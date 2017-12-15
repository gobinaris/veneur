package veneur

type Config struct {
	Aggregates                    []string  `yaml:"aggregates"`
	APIHostname                   string    `yaml:"api_hostname"`
	AwsAccessKeyID                string    `yaml:"aws_access_key_id"`
	AwsRegion                     string    `yaml:"aws_region"`
	AwsS3Bucket                   string    `yaml:"aws_s3_bucket"`
	AwsSecretAccessKey            string    `yaml:"aws_secret_access_key"`
	DatadogAPIHostname            string    `yaml:"datadog_api_hostname"`
	DatadogAPIKey                 string    `yaml:"datadog_api_key"`
	DatadogTraceAPIAddress        string    `yaml:"datadog_trace_api_address"`
	Debug                         bool      `yaml:"debug"`
	EnableProfiling               bool      `yaml:"enable_profiling"`
	FlushFile                     string    `yaml:"flush_file"`
	FlushMaxPerBody               int       `yaml:"flush_max_per_body"`
	ForwardAddress                string    `yaml:"forward_address"`
	Hostname                      string    `yaml:"hostname"`
	HTTPAddress                   string    `yaml:"http_address"`
	IndicatorSpanTimerName        string    `yaml:"indicator_span_timer_name"`
	Interval                      string    `yaml:"interval"`
	KafkaBroker                   string    `yaml:"kafka_broker"`
	KafkaCheckTopic               string    `yaml:"kafka_check_topic"`
	KafkaEventTopic               string    `yaml:"kafka_event_topic"`
	KafkaMetricBufferBytes        int       `yaml:"kafka_metric_buffer_bytes"`
	KafkaMetricBufferFrequency    string    `yaml:"kafka_metric_buffer_frequency"`
	KafkaMetricBufferMessages     int       `yaml:"kafka_metric_buffer_messages"`
	KafkaMetricRequireAcks        string    `yaml:"kafka_metric_require_acks"`
	KafkaMetricTopic              string    `yaml:"kafka_metric_topic"`
	KafkaPartitioner              string    `yaml:"kafka_partitioner"`
	KafkaRetryMax                 int       `yaml:"kafka_retry_max"`
	KafkaSpanBufferBytes          int       `yaml:"kafka_span_buffer_bytes"`
	KafkaSpanBufferFrequency      string    `yaml:"kafka_span_buffer_frequency"`
	KafkaSpanBufferMesages        int       `yaml:"kafka_span_buffer_mesages"`
	KafkaSpanRequireAcks          string    `yaml:"kafka_span_require_acks"`
	KafkaSpanSerializationFormat  string    `yaml:"kafka_span_serialization_format"`
	KafkaSpanTopic                string    `yaml:"kafka_span_topic"`
	Key                           string    `yaml:"key"`
	MetricMaxLength               int       `yaml:"metric_max_length"`
	NumReaders                    int       `yaml:"num_readers"`
	NumWorkers                    int       `yaml:"num_workers"`
	OmitEmptyHostname             bool      `yaml:"omit_empty_hostname"`
	Percentiles                   []float64 `yaml:"percentiles"`
	ReadBufferSizeBytes           int       `yaml:"read_buffer_size_bytes"`
	SentryDsn                     string    `yaml:"sentry_dsn"`
	SignalfxAPIKey                string    `yaml:"signalfx_api_key"`
	SignalfxHostname              string    `yaml:"signalfx_hostname"`
	SsfAddress                    string    `yaml:"ssf_address"`
	SsfBufferSize                 int       `yaml:"ssf_buffer_size"`
	SsfListenAddresses            []string  `yaml:"ssf_listen_addresses"`
	StatsAddress                  string    `yaml:"stats_address"`
	StatsdListenAddresses         []string  `yaml:"statsd_listen_addresses"`
	SynchronizeWithInterval       bool      `yaml:"synchronize_with_interval"`
	Tags                          []string  `yaml:"tags"`
	TcpAddress                    string    `yaml:"tcp_address"`
	TLSAuthorityCertificate       string    `yaml:"tls_authority_certificate"`
	TLSCertificate                string    `yaml:"tls_certificate"`
	TLSKey                        string    `yaml:"tls_key"`
	TraceAddress                  string    `yaml:"trace_address"`
	TraceAPIAddress               string    `yaml:"trace_api_address"`
	TraceLightstepAccessToken     string    `yaml:"trace_lightstep_access_token"`
	TraceLightstepCollectorHost   string    `yaml:"trace_lightstep_collector_host"`
	TraceLightstepMaximumSpans    int       `yaml:"trace_lightstep_maximum_spans"`
	TraceLightstepNumClients      int       `yaml:"trace_lightstep_num_clients"`
	TraceLightstepReconnectPeriod string    `yaml:"trace_lightstep_reconnect_period"`
	TraceMaxLengthBytes           int       `yaml:"trace_max_length_bytes"`
	UdpAddress                    string    `yaml:"udp_address"`
}
