package base

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	labels         = []string{"chain_name", "hostname"}
	labelsWithInfo = []string{"chain_name", "hostname", "chain_id", "node_version", "protocol_name"}

	// BlockLastUpdateTime tracks the last time a block was processed (seconds since epoch)
	BlockLastUpdateTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "story_node_last_block_timestamp_seconds",
		Help: "Timestamp of the last processed block in seconds since epoch",
	}, labelsWithInfo)

	// BlockProcessingDelay measures the delay between block creation and processing
	BlockProcessingDelay = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "story_node_block_processing_delay_seconds",
		Help: "Delay between block creation and processing in seconds",
	}, labels)

	// BlockProcessingDelayHistogram provides histogram of block processing delays
	BlockProcessingDelayHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "story_node_block_processing_delay_histogram_seconds",
		Help:    "Histogram of block processing delays in seconds",
		Buckets: []float64{0.1, 0.3, 0.5, 1, 3, 5, 10, 30, 60, 120, 180},
	}, labels)

	// RPCConnectionAttempts counts successful and failed RPC connection attempts
	RPCConnectionAttempts = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "story_node_rpc_connections_count",
		Help: "Total number of RPC connection attempts by type and result",
	}, append(labels, "connection_type", "result"))

	// NodeHealthStatus indicates the health status of various node endpoints
	NodeHealthStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "story_node_health_status",
		Help: "Health status of node endpoints (1=healthy, 0=unhealthy)",
	}, append(labels, "endpoint_type"))

	// EndpointResponseTime measures current response time for different endpoints
	EndpointResponseTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "story_node_endpoint_response_time_milliseconds",
		Help: "Current response time for node endpoints in milliseconds",
	}, append(labels, "endpoint_type"))

	// EndpointResponseTimeHistogram provides histogram of endpoint response times
	EndpointResponseTimeHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "story_node_endpoint_response_time_histogram_milliseconds",
		Help:    "Histogram of endpoint response times in milliseconds",
		Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
	}, append(labels, "endpoint_type"))
)

func init() {
	prometheus.MustRegister(BlockLastUpdateTime)
	prometheus.MustRegister(BlockProcessingDelay)
	prometheus.MustRegister(BlockProcessingDelayHistogram)
	prometheus.MustRegister(RPCConnectionAttempts)
	prometheus.MustRegister(NodeHealthStatus)
	prometheus.MustRegister(EndpointResponseTime)
	prometheus.MustRegister(EndpointResponseTimeHistogram)
}

type CheckerTrait interface {
	Start()

	GetChainName() string
	GetHostName() string
	GetChainId() string
	GetNodeVersion() string
	GetProtocolName() string
}

// BaseChecker provides common functionality for all checker implementations
type BaseChecker struct {
	ChainName    string
	HostName     string
	ChainId      string
	NodeVersion  string
	ProtocolName string
}

// AddLabelValues creates label values array for basic metrics (chain_name, hostname)
func (b *BaseChecker) AddLabelValues(extraLabels ...string) []string {
	values := []string{b.ChainName, b.HostName}
	return append(values, extraLabels...)
}

// AddLabelValuesWithInfo creates label values array including full chain info (chain_name, hostname, chain_id, node_version, protocol_name)
func (b *BaseChecker) AddLabelValuesWithInfo(extraLabels ...string) []string {
	values := []string{b.ChainName, b.HostName, b.ChainId, b.NodeVersion, b.ProtocolName}
	return append(values, extraLabels...)
}

// RecordConnectionAttempt records connection attempt metrics
func (b *BaseChecker) RecordConnectionAttempt(connectionType string, success bool) {
	result := "fail"
	if success {
		result = "success"
	}
	RPCConnectionAttempts.WithLabelValues(b.AddLabelValues(connectionType, result)...).Add(1)
}

// RecordHealthStatus records health status for an endpoint type
func (b *BaseChecker) RecordHealthStatus(endpointType string, healthy bool) {
	status := float64(0)
	if healthy {
		status = 1
	}
	NodeHealthStatus.WithLabelValues(b.AddLabelValues(endpointType)...).Set(status)
}

// RecordResponseTime records response time metrics for an endpoint
func (b *BaseChecker) RecordResponseTime(endpointType string, duration time.Duration) {
	milliseconds := float64(duration.Milliseconds())
	EndpointResponseTime.WithLabelValues(b.AddLabelValues(endpointType)...).Set(milliseconds)
	EndpointResponseTimeHistogram.WithLabelValues(b.AddLabelValues(endpointType)...).Observe(milliseconds)
}

// RecordBlockProcessingDelay records block processing delay metrics
func (b *BaseChecker) RecordBlockProcessingDelay(delaySeconds float64) {
	BlockProcessingDelay.WithLabelValues(b.AddLabelValues()...).Set(delaySeconds)
	BlockProcessingDelayHistogram.WithLabelValues(b.AddLabelValues()...).Observe(delaySeconds)
}

// UpdateLastBlockTime updates the last block timestamp
func (b *BaseChecker) UpdateLastBlockTime() {
	BlockLastUpdateTime.WithLabelValues(b.AddLabelValuesWithInfo()...).Set(0)
}

// HealthCheckOperation represents a health check operation with timing
func (b *BaseChecker) HealthCheckOperation(endpointType string, operation func() error) {
	startTime := time.Now()
	err := operation()
	duration := time.Since(startTime)

	b.RecordHealthStatus(endpointType, err == nil)
	b.RecordResponseTime(endpointType, duration)
}

// CheckSecondToTicker converts check_second to ticker, with default fallback
func CheckSecondToTicker(checkSecond int, defaultSeconds int) *time.Ticker {
	if checkSecond <= 0 {
		checkSecond = defaultSeconds
	}
	return time.NewTicker(time.Duration(checkSecond) * time.Second)
}

// WaitForContextOrTicker waits for either context cancellation or ticker
func WaitForContextOrTicker(ctx context.Context, ticker *time.Ticker) bool {
	select {
	case <-ctx.Done():
		return false // context cancelled
	case <-ticker.C:
		return true // ticker fired
	}
}
