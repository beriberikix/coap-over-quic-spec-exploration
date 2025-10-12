package harness

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"time"
)

// MetricType represents the type of metric being measured
type MetricType string

const (
	LatencyMetric    MetricType = "latency"
	ThroughputMetric MetricType = "throughput"
	BytesMetric      MetricType = "bytes"
)

// Metric represents a single measurement
type Metric struct {
	Timestamp   time.Time
	Transport   string // "quic-stream", "quic-datagram", "udp"
	Operation   string // "GET", "POST", "PUT"
	Path        string
	Type        MetricType
	Value       float64 // milliseconds for latency, bytes for bytes
	PayloadSize int
	Success     bool
	Error       string
}

// Stats represents statistical summary of metrics
type Stats struct {
	Count      int
	Min        float64
	Max        float64
	Mean       float64
	Median     float64
	P95        float64
	P99        float64
	TotalBytes int64
	Errors     int
}

// MetricsCollector collects and analyzes metrics
type MetricsCollector struct {
	metrics []Metric
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		metrics: make([]Metric, 0, 1000),
	}
}

// Record adds a metric to the collector
func (mc *MetricsCollector) Record(m Metric) {
	mc.metrics = append(mc.metrics, m)
}

// RecordLatency is a convenience method for recording latency
func (mc *MetricsCollector) RecordLatency(transport, operation, path string, duration time.Duration, payloadSize int, err error) {
	metric := Metric{
		Timestamp:   time.Now(),
		Transport:   transport,
		Operation:   operation,
		Path:        path,
		Type:        LatencyMetric,
		Value:       float64(duration.Microseconds()) / 1000.0, // Convert to milliseconds
		PayloadSize: payloadSize,
		Success:     err == nil,
	}
	if err != nil {
		metric.Error = err.Error()
	}
	mc.Record(metric)
}

// RecordBytes is a convenience method for recording bytes transferred
func (mc *MetricsCollector) RecordBytes(transport, operation, path string, bytesTransferred int, payloadSize int) {
	mc.Record(Metric{
		Timestamp:   time.Now(),
		Transport:   transport,
		Operation:   operation,
		Path:        path,
		Type:        BytesMetric,
		Value:       float64(bytesTransferred),
		PayloadSize: payloadSize,
		Success:     true,
	})
}

// GetStats calculates statistics for a filtered set of metrics
func (mc *MetricsCollector) GetStats(transport string, metricType MetricType) Stats {
	var values []float64
	var totalBytes int64
	var errors int

	for _, m := range mc.metrics {
		if m.Transport == transport && m.Type == metricType {
			if m.Success {
				values = append(values, m.Value)
				if metricType == BytesMetric {
					totalBytes += int64(m.Value)
				}
			} else {
				errors++
			}
		}
	}

	if len(values) == 0 {
		return Stats{Count: 0, Errors: errors}
	}

	sort.Float64s(values)

	sum := 0.0
	for _, v := range values {
		sum += v
	}

	stats := Stats{
		Count:      len(values),
		Min:        values[0],
		Max:        values[len(values)-1],
		Mean:       sum / float64(len(values)),
		Median:     percentile(values, 50),
		P95:        percentile(values, 95),
		P99:        percentile(values, 99),
		TotalBytes: totalBytes,
		Errors:     errors,
	}

	return stats
}

// GetAllStats returns stats grouped by transport and metric type
func (mc *MetricsCollector) GetAllStats() map[string]map[MetricType]Stats {
	result := make(map[string]map[MetricType]Stats)

	transports := []string{"quic-stream", "quic-datagram", "udp"}
	metricTypes := []MetricType{LatencyMetric, BytesMetric}

	for _, transport := range transports {
		result[transport] = make(map[MetricType]Stats)
		for _, metricType := range metricTypes {
			result[transport][metricType] = mc.GetStats(transport, metricType)
		}
	}

	return result
}

// ExportCSV exports all metrics to a CSV file
func (mc *MetricsCollector) ExportCSV(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		"timestamp",
		"transport",
		"operation",
		"path",
		"metric_type",
		"value",
		"payload_size",
		"success",
		"error",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data
	for _, m := range mc.metrics {
		record := []string{
			m.Timestamp.Format(time.RFC3339Nano),
			m.Transport,
			m.Operation,
			m.Path,
			string(m.Type),
			fmt.Sprintf("%.3f", m.Value),
			fmt.Sprintf("%d", m.PayloadSize),
			fmt.Sprintf("%t", m.Success),
			m.Error,
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	return nil
}

// PrintSummary prints a summary of statistics to stdout
func (mc *MetricsCollector) PrintSummary() {
	fmt.Println("\n=== Benchmark Results Summary ===\n")

	allStats := mc.GetAllStats()

	for _, transport := range []string{"quic-stream", "quic-datagram", "udp"} {
		stats, ok := allStats[transport]
		if !ok {
			continue
		}

		fmt.Printf("## %s\n", transport)

		// Latency stats
		if latency, ok := stats[LatencyMetric]; ok && latency.Count > 0 {
			fmt.Println("\nLatency (ms):")
			fmt.Printf("  Count:   %d\n", latency.Count)
			fmt.Printf("  Min:     %.3f\n", latency.Min)
			fmt.Printf("  Mean:    %.3f\n", latency.Mean)
			fmt.Printf("  Median:  %.3f\n", latency.Median)
			fmt.Printf("  P95:     %.3f\n", latency.P95)
			fmt.Printf("  P99:     %.3f\n", latency.P99)
			fmt.Printf("  Max:     %.3f\n", latency.Max)
			if latency.Errors > 0 {
				fmt.Printf("  Errors:  %d\n", latency.Errors)
			}
		}

		// Bytes stats
		if bytes, ok := stats[BytesMetric]; ok && bytes.Count > 0 {
			fmt.Println("\nBytes Transferred:")
			fmt.Printf("  Requests:    %d\n", bytes.Count)
			fmt.Printf("  Total:       %d bytes\n", bytes.TotalBytes)
			fmt.Printf("  Mean/req:    %.0f bytes\n", bytes.Mean)
			fmt.Printf("  Min/req:     %.0f bytes\n", bytes.Min)
			fmt.Printf("  Max/req:     %.0f bytes\n", bytes.Max)
		}

		fmt.Println()
	}
}

// percentile calculates the nth percentile of a sorted slice
func percentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}

	index := float64(p) / 100.0 * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1

	if upper >= len(sorted) {
		return sorted[lower]
	}

	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}
