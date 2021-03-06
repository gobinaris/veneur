package veneur

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	vhttp "github.com/stripe/veneur/http"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"
)

// Flush collects sampler's metrics and passes them to sinks.
func (s *Server) Flush(ctx context.Context) {
	span := tracer.StartSpan("flush").(*trace.Span)
	defer span.ClientFinish(s.TraceClient)

	mem := &runtime.MemStats{}
	runtime.ReadMemStats(mem)

	span.Add(ssf.Gauge("mem.heap_alloc_bytes", float32(mem.HeapAlloc), nil),
		ssf.Gauge("gc.number", float32(mem.NumGC), nil),
		ssf.Gauge("gc.pause_total_ns", float32(mem.PauseTotalNs), nil),
		ssf.Gauge("worker.span_chan.total_elements", float32(len(s.SpanChan)), nil),
		ssf.Gauge("worker.span_chan.total_capacity", float32(cap(s.SpanChan)), nil),
	)

	// TODO Why is this not in the worker the way the trace worker is set up?
	events, checks := s.EventWorker.Flush()
	span.Add(ssf.Count("worker.events_flushed_total", float32(len(events)), nil),
		ssf.Count("worker.checks_flushed_total", float32(len(checks)), nil))

	// TODO Concurrency
	for _, sink := range s.metricSinks {
		sink.FlushEventsChecks(span.Attach(ctx), events, checks)
	}

	go s.flushTraces(span.Attach(ctx))

	// don't publish percentiles if we're a local veneur; that's the global
	// veneur's job
	var percentiles []float64
	var finalMetrics []samplers.InterMetric

	if !s.IsLocal() {
		percentiles = s.HistogramPercentiles
	}

	tempMetrics, ms := s.tallyMetrics(percentiles)

	finalMetrics = s.generateInterMetrics(span.Attach(ctx), percentiles, tempMetrics, ms)

	span.Add(s.computeMetricsFlushCounts(ms)...)

	if s.IsLocal() {
		go s.flushForward(span.Attach(ctx), tempMetrics)
	} else {
		span.Add(s.computeGlobalMetricsFlushCounts(ms)...)
	}

	// If there's nothing to flush, don't bother calling the plugins and stuff.
	if len(finalMetrics) == 0 {
		return
	}

	wg := sync.WaitGroup{}
	for _, sink := range s.metricSinks {
		wg.Add(1)
		go func(ms sinks.MetricSink) {
			err := ms.Flush(span.Attach(ctx), finalMetrics)
			if err != nil {
				log.WithError(err).WithField("sink", ms.Name()).Warn("Error flushing sink")
			}
			wg.Done()
		}(sink)
	}
	wg.Wait()

	go func() {
		samples := &ssf.Samples{}
		defer metrics.Report(s.TraceClient, samples)

		tags := map[string]string{"part": "post"}
		for _, p := range s.getPlugins() {
			start := time.Now()
			err := p.Flush(span.Attach(ctx), finalMetrics)
			samples.Add(ssf.Timing(fmt.Sprintf("flush.plugins.%s.total_duration_ns", p.Name()), time.Since(start), time.Nanosecond, tags))
			if err != nil {
				samples.Add(ssf.Count(fmt.Sprintf("flush.plugins.%s.error_total", p.Name()), 1, nil))
			}
			samples.Add(ssf.Gauge(fmt.Sprintf("flush.plugins.%s.post_metrics_total", p.Name()), float32(len(finalMetrics)), nil))
		}
	}()
}

type metricsSummary struct {
	totalCounters   int
	totalGauges     int
	totalHistograms int
	totalSets       int
	totalTimers     int

	totalGlobalCounters int
	totalGlobalGauges   int

	totalLocalHistograms int
	totalLocalSets       int
	totalLocalTimers     int

	totalLength int
}

// tallyMetrics gives a slight overestimate of the number
// of metrics we'll be reporting, so that we can pre-allocate
// a slice of the correct length instead of constantly appending
// for performance
func (s *Server) tallyMetrics(percentiles []float64) ([]WorkerMetrics, metricsSummary) {
	// allocating this long array to count up the sizes is cheaper than appending
	// the []WorkerMetrics together one at a time
	tempMetrics := make([]WorkerMetrics, 0, len(s.Workers))

	gatherStart := time.Now()
	ms := metricsSummary{}

	for i, w := range s.Workers {
		log.WithField("worker", i).Debug("Flushing")
		wm := w.Flush()
		tempMetrics = append(tempMetrics, wm)

		ms.totalCounters += len(wm.counters)
		ms.totalGauges += len(wm.gauges)
		ms.totalHistograms += len(wm.histograms)
		ms.totalSets += len(wm.sets)
		ms.totalTimers += len(wm.timers)

		ms.totalGlobalCounters += len(wm.globalCounters)
		ms.totalGlobalGauges += len(wm.globalGauges)

		ms.totalLocalHistograms += len(wm.localHistograms)
		ms.totalLocalSets += len(wm.localSets)
		ms.totalLocalTimers += len(wm.localTimers)
	}

	metrics.ReportOne(s.TraceClient, ssf.Timing("flush.total_duration_ns", time.Since(gatherStart), time.Nanosecond, map[string]string{"part": "gather"}))

	ms.totalLength = ms.totalCounters + ms.totalGauges +
		// histograms and timers each report a metric point for each percentile
		// plus a point for each of their aggregates
		(ms.totalTimers+ms.totalHistograms)*(s.HistogramAggregates.Count+len(percentiles)) +
		// local-only histograms will be flushed with percentiles, so we intentionally
		// use the original percentile list here.
		// remember that both the global veneur and the local instances have
		// 'local-only' histograms.
		ms.totalLocalSets + (ms.totalLocalTimers+ms.totalLocalHistograms)*(s.HistogramAggregates.Count+len(s.HistogramPercentiles))

	// Global instances also flush sets and global counters, so be sure and add
	// them to the total size
	if !s.IsLocal() {
		ms.totalLength += ms.totalSets
		ms.totalLength += ms.totalGlobalCounters
		ms.totalLength += ms.totalGlobalGauges
	}

	return tempMetrics, ms
}

// generateInterMetrics calls the Flush method on each
// counter/gauge/histogram/timer/set in order to
// generate an InterMetric corresponding to that value
func (s *Server) generateInterMetrics(ctx context.Context, percentiles []float64, tempMetrics []WorkerMetrics, ms metricsSummary) []samplers.InterMetric {

	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(s.TraceClient)

	finalMetrics := make([]samplers.InterMetric, 0, ms.totalLength)
	for _, wm := range tempMetrics {
		for _, c := range wm.counters {
			finalMetrics = append(finalMetrics, c.Flush(s.interval)...)
		}
		for _, g := range wm.gauges {
			finalMetrics = append(finalMetrics, g.Flush()...)
		}
		// if we're a local veneur, then percentiles=nil, and only the local
		// parts (count, min, max) will be flushed
		for _, h := range wm.histograms {
			finalMetrics = append(finalMetrics, h.Flush(s.interval, percentiles, s.HistogramAggregates)...)
		}
		for _, t := range wm.timers {
			finalMetrics = append(finalMetrics, t.Flush(s.interval, percentiles, s.HistogramAggregates)...)
		}

		// local-only samplers should be flushed in their entirety, since they
		// will not be forwarded
		// we still want percentiles for these, even if we're a local veneur, so
		// we use the original percentile list when flushing them
		for _, h := range wm.localHistograms {
			finalMetrics = append(finalMetrics, h.Flush(s.interval, s.HistogramPercentiles, s.HistogramAggregates)...)
		}
		for _, s := range wm.localSets {
			finalMetrics = append(finalMetrics, s.Flush()...)
		}
		for _, t := range wm.localTimers {
			finalMetrics = append(finalMetrics, t.Flush(s.interval, s.HistogramPercentiles, s.HistogramAggregates)...)
		}

		// TODO (aditya) refactor this out so we don't
		// have to call IsLocal again
		if !s.IsLocal() {
			// sets have no local parts, so if we're a local veneur, there's
			// nothing to flush at all
			for _, s := range wm.sets {
				finalMetrics = append(finalMetrics, s.Flush()...)
			}

			// also do this for global counters
			// global counters have no local parts, so if we're a local veneur,
			// there's nothing to flush
			for _, gc := range wm.globalCounters {
				finalMetrics = append(finalMetrics, gc.Flush(s.interval)...)
			}

			// and global gauges
			for _, gg := range wm.globalGauges {
				finalMetrics = append(finalMetrics, gg.Flush()...)
			}
		}
	}

	metrics.ReportOne(s.TraceClient, ssf.Timing("flush.total_duration_ns", time.Since(span.Start), time.Nanosecond, map[string]string{"part": "combine"}))
	return finalMetrics
}

const flushTotalMetric = "worker.metrics_flushed_total"

// computeMetricsFlushCounts reports the counts of
// Counters, Gauges, LocalHistograms, LocalSets, and LocalTimers
// as metrics. These are shared by both global and local flush operations.
// It does *not* report the totalHistograms, totalSets, or totalTimers
// because those are only performed by the global veneur instance.
// It also does not report the total metrics posted, because on the local veneur,
// that should happen *after* the flush-forward operation.
func (s *Server) computeMetricsFlushCounts(ms metricsSummary) []*ssf.SSFSample {
	return []*ssf.SSFSample{
		ssf.Count(flushTotalMetric, float32(ms.totalCounters), map[string]string{"metric_type": "counter"}),
		ssf.Count(flushTotalMetric, float32(ms.totalGauges), map[string]string{"metric_type": "gauge"}),
		ssf.Count(flushTotalMetric, float32(ms.totalLocalHistograms), map[string]string{"metric_type": "local_histogram"}),
		ssf.Count(flushTotalMetric, float32(ms.totalLocalSets), map[string]string{"metric_type": "local_set"}),
		ssf.Count(flushTotalMetric, float32(ms.totalLocalTimers), map[string]string{"metric_type": "local_timer"}),
	}
}

// computeGlobalMetricsFlushCounts reports the counts of
// globalCounters, globalGauges, totalHistograms, totalSets, and totalTimers,
// which are the three metrics reported *only* by the global
// veneur instance.
func (s *Server) computeGlobalMetricsFlushCounts(ms metricsSummary) []*ssf.SSFSample {
	// we only report these lengths in FlushGlobal
	// since if we're the global veneur instance responsible for flushing them
	// this avoids double-counting problems where a local veneur reports
	// histograms that it received, and then a global veneur reports them
	// again
	return []*ssf.SSFSample{
		ssf.Count(flushTotalMetric, float32(ms.totalGlobalCounters), map[string]string{"metric_type": "global_counter"}),
		ssf.Count(flushTotalMetric, float32(ms.totalGlobalGauges), map[string]string{"metric_type": "global_gauge"}),
		ssf.Count(flushTotalMetric, float32(ms.totalHistograms), map[string]string{"metric_type": "histogram"}),
		ssf.Count(flushTotalMetric, float32(ms.totalSets), map[string]string{"metric_type": "set"}),
		ssf.Count(flushTotalMetric, float32(ms.totalTimers), map[string]string{"metric_type": "timer"}),
	}
}

func (s *Server) flushForward(ctx context.Context, wms []WorkerMetrics) {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(s.TraceClient)
	jmLength := 0
	for _, wm := range wms {
		jmLength += len(wm.histograms)
		jmLength += len(wm.sets)
		jmLength += len(wm.timers)
	}

	jsonMetrics := make([]samplers.JSONMetric, 0, jmLength)
	exportStart := time.Now()
	for _, wm := range wms {
		for _, count := range wm.globalCounters {
			jm, err := count.Export()
			if err != nil {
				log.WithFields(logrus.Fields{
					logrus.ErrorKey: err,
					"type":          "counter",
					"name":          count.Name,
				}).Error("Could not export metric")
				continue
			}
			jsonMetrics = append(jsonMetrics, jm)
		}
		for _, gauge := range wm.globalGauges {
			jm, err := gauge.Export()
			if err != nil {
				log.WithFields(logrus.Fields{
					logrus.ErrorKey: err,
					"type":          "gauge",
					"name":          gauge.Name,
				}).Error("Could not export metric")
				continue
			}
			jsonMetrics = append(jsonMetrics, jm)
		}
		for _, histo := range wm.histograms {
			jm, err := histo.Export()
			if err != nil {
				log.WithFields(logrus.Fields{
					logrus.ErrorKey: err,
					"type":          "histogram",
					"name":          histo.Name,
				}).Error("Could not export metric")
				continue
			}
			jsonMetrics = append(jsonMetrics, jm)
		}
		for _, set := range wm.sets {
			jm, err := set.Export()
			if err != nil {
				log.WithFields(logrus.Fields{
					logrus.ErrorKey: err,
					"type":          "set",
					"name":          set.Name,
				}).Error("Could not export metric")
				continue
			}
			jsonMetrics = append(jsonMetrics, jm)
		}
		for _, timer := range wm.timers {
			jm, err := timer.Export()
			if err != nil {
				log.WithFields(logrus.Fields{
					logrus.ErrorKey: err,
					"type":          "timer",
					"name":          timer.Name,
				}).Error("Could not export metric")
				continue
			}
			// the exporter doesn't know that these two are "different"
			jm.Type = "timer"
			jsonMetrics = append(jsonMetrics, jm)
		}
	}
	span.Add(ssf.Timing("forward.duration_ns", time.Since(exportStart), time.Nanosecond, map[string]string{"part": "export"}),
		ssf.Gauge("forward.post_metrics_total", float32(len(jsonMetrics)), nil))
	if len(jsonMetrics) == 0 {
		log.Debug("Nothing to forward, skipping.")
		return
	}

	// the error has already been logged (if there was one), so we only care
	// about the success case
	endpoint := fmt.Sprintf("%s/import", s.ForwardAddr)
	if vhttp.PostHelper(span.Attach(ctx), s.HTTPClient, s.TraceClient, http.MethodPost, endpoint, jsonMetrics, "forward", true, log) == nil {
		verbose(log.WithFields(logrus.Fields{
			"metrics":     len(jsonMetrics),
			"endpoint":    endpoint,
			"forwardAddr": s.ForwardAddr,
		}), "Completed forward to upstream Veneur")
	}
}

func (s *Server) flushTraces(ctx context.Context) {
	for _, w := range s.SpanWorkers {
		w.Flush()
	}
}
