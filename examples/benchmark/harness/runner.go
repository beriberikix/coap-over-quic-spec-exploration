package harness

import (
	"context"
	"fmt"
	"log"
	"time"
)

// TestScenario defines a benchmark test scenario
type TestScenario struct {
	Name        string
	Description string
	PayloadSize int    // Size of payload in bytes
	Count       int    // Number of requests to send
	Concurrent  int    // Number of concurrent requests (0 = sequential)
	Method      string // GET, POST, PUT
	Path        string
	IsNON       bool // For UDP: use NON messages; For QUIC: use datagrams
}

// TransportClient is an interface that different transports must implement
type TransportClient interface {
	// Name returns the transport name (e.g., "quic-stream", "udp")
	Name() string

	// Connect establishes connection to the server
	Connect(addr string) error

	// Close closes the connection
	Close() error

	// SendRequest sends a request and returns (response bytes, total bytes sent+received, duration, error)
	SendRequest(ctx context.Context, method, path string, payload []byte) ([]byte, int, time.Duration, error)

	// SendNON sends a non-confirmable message (fire-and-forget)
	SendNON(ctx context.Context, method, path string, payload []byte) (int, time.Duration, error)
}

// AdvancedTransportClient extends TransportClient with advanced features
// Not all transports need to implement these - they can return ErrNotSupported
type AdvancedTransportClient interface {
	TransportClient

	// TestConnectionResumption tests 0-RTT connection resumption
	// Returns (cold connection duration, warm connection duration, error)
	TestConnectionResumption(addr string) (time.Duration, time.Duration, error)

	// TestMigration tests connection migration
	// Returns (migration overhead duration, error)
	TestMigration() (time.Duration, error)

	// SubscribeObserve subscribes to an Observe resource
	// Returns (initial response time, error)
	SubscribeObserve(ctx context.Context, path string) (time.Duration, error)

	// WaitForNotifications waits for Observe notifications and returns their timings
	// Returns (notification timings array, error)
	WaitForNotifications(ctx context.Context, count int) ([]time.Duration, error)
}

// BenchmarkRunner runs benchmark scenarios
type BenchmarkRunner struct {
	collector *MetricsCollector
	serverAddr string
}

// NewBenchmarkRunner creates a new benchmark runner
func NewBenchmarkRunner(serverAddr string) *BenchmarkRunner {
	return &BenchmarkRunner{
		collector:  NewMetricsCollector(),
		serverAddr: serverAddr,
	}
}

// RunScenario runs a single test scenario with a specific transport
func (br *BenchmarkRunner) RunScenario(client TransportClient, scenario TestScenario) error {
	log.Printf("\n=== Running: %s (%s) ===", scenario.Name, client.Name())
	log.Printf("Description: %s", scenario.Description)
	log.Printf("Payload: %d bytes, Count: %d, Concurrent: %d",
		scenario.PayloadSize, scenario.Count, scenario.Concurrent)

	// Connect to server
	if err := client.Connect(br.serverAddr); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()

	// Generate payload
	payload := generatePayload(scenario.PayloadSize)

	// Run requests
	startTime := time.Now()

	if scenario.Concurrent > 1 {
		// Concurrent execution
		br.runConcurrent(client, scenario, payload)
	} else {
		// Sequential execution
		br.runSequential(client, scenario, payload)
	}

	elapsed := time.Since(startTime)
	log.Printf("Completed in %v (%.2f req/sec)",
		elapsed, float64(scenario.Count)/elapsed.Seconds())

	return nil
}

// runSequential executes requests sequentially
func (br *BenchmarkRunner) runSequential(client TransportClient, scenario TestScenario, payload []byte) {
	ctx := context.Background()

	for i := 0; i < scenario.Count; i++ {
		if scenario.IsNON {
			// Send NON message (fire-and-forget)
			bytes, duration, err := client.SendNON(ctx, scenario.Method, scenario.Path, payload)
			br.collector.RecordLatency(client.Name(), scenario.Method, scenario.Path, duration, scenario.PayloadSize, err)
			if err == nil {
				br.collector.RecordBytes(client.Name(), scenario.Method, scenario.Path, bytes, scenario.PayloadSize)
			}
		} else {
			// Send regular request with response
			_, bytes, duration, err := client.SendRequest(ctx, scenario.Method, scenario.Path, payload)
			br.collector.RecordLatency(client.Name(), scenario.Method, scenario.Path, duration, scenario.PayloadSize, err)
			if err == nil {
				br.collector.RecordBytes(client.Name(), scenario.Method, scenario.Path, bytes, scenario.PayloadSize)
			}
		}

		// Small delay between requests to avoid overwhelming the server
		time.Sleep(10 * time.Millisecond)
	}
}

// runConcurrent executes requests concurrently
func (br *BenchmarkRunner) runConcurrent(client TransportClient, scenario TestScenario, payload []byte) {
	sem := make(chan struct{}, scenario.Concurrent)
	done := make(chan struct{})

	for i := 0; i < scenario.Count; i++ {
		sem <- struct{}{} // Acquire semaphore

		go func() {
			defer func() { <-sem }() // Release semaphore
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if scenario.IsNON {
				bytes, duration, err := client.SendNON(ctx, scenario.Method, scenario.Path, payload)
				br.collector.RecordLatency(client.Name(), scenario.Method, scenario.Path, duration, scenario.PayloadSize, err)
				if err == nil {
					br.collector.RecordBytes(client.Name(), scenario.Method, scenario.Path, bytes, scenario.PayloadSize)
				}
			} else {
				_, bytes, duration, err := client.SendRequest(ctx, scenario.Method, scenario.Path, payload)
				br.collector.RecordLatency(client.Name(), scenario.Method, scenario.Path, duration, scenario.PayloadSize, err)
				if err == nil {
					br.collector.RecordBytes(client.Name(), scenario.Method, scenario.Path, bytes, scenario.PayloadSize)
				}
			}
		}()
	}

	// Wait for all requests to complete
	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}
	close(done)
}

// GetCollector returns the metrics collector
func (br *BenchmarkRunner) GetCollector() *MetricsCollector {
	return br.collector
}

// generatePayload creates a payload of the specified size
func generatePayload(size int) []byte {
	if size == 0 {
		return nil
	}

	// Generate realistic JSON payload for sensor data
	if size < 50 {
		return []byte(fmt.Sprintf(`{"v":%d}`, size))
	}

	// For larger payloads, create a JSON with a data field
	dataSize := size - 20 // Account for JSON structure
	if dataSize < 0 {
		dataSize = 0
	}

	data := make([]byte, dataSize)
	for i := range data {
		data[i] = byte('A' + (i % 26))
	}

	return []byte(fmt.Sprintf(`{"data":"%s"}`, string(data)))
}

// Common test scenarios
var (
	// Small payloads (sensor telemetry)
	SmallPayloadGET = TestScenario{
		Name:        "Small GET (Temperature Reading)",
		Description: "Simulates frequent temperature sensor readings",
		PayloadSize: 0,
		Count:       100,
		Concurrent:  1,
		Method:      "GET",
		Path:        "/temp",
		IsNON:       false,
	}

	SmallPayloadPOST = TestScenario{
		Name:        "Small POST (Telemetry)",
		Description: "Simulates small telemetry data uploads",
		PayloadSize: 50,
		Count:       100,
		Concurrent:  1,
		Method:      "POST",
		Path:        "/telemetry",
		IsNON:       false,
	}

	SmallPayloadNON = TestScenario{
		Name:        "Small NON (Fire-and-forget Telemetry)",
		Description: "Simulates unreliable telemetry uploads",
		PayloadSize: 50,
		Count:       100,
		Concurrent:  1,
		Method:      "POST",
		Path:        "/telemetry",
		IsNON:       true,
	}

	// Medium payloads (configuration updates)
	MediumPayloadPOST = TestScenario{
		Name:        "Medium POST (Config Update)",
		Description: "Simulates configuration updates",
		PayloadSize: 1024, // 1KB
		Count:       50,
		Concurrent:  1,
		Method:      "POST",
		Path:        "/telemetry",
		IsNON:       false,
	}

	// Large payloads (OTA updates)
	LargePayloadPOST = TestScenario{
		Name:        "Large POST (OTA Chunk)",
		Description: "Simulates OTA firmware update chunks",
		PayloadSize: 10240, // 10KB
		Count:       20,
		Concurrent:  1,
		Method:      "POST",
		Path:        "/telemetry",
		IsNON:       false,
	}

	VeryLargePayloadPOST = TestScenario{
		Name:        "Very Large POST (Large OTA Chunk)",
		Description: "Simulates larger OTA firmware chunks",
		PayloadSize: 102400, // 100KB
		Count:       10,
		Concurrent:  1,
		Method:      "POST",
		Path:        "/telemetry",
		IsNON:       false,
	}

	// Burst scenarios
	BurstSmallPayload = TestScenario{
		Name:        "Burst Small NON (Rapid Sensor Data)",
		Description: "Simulates rapid sensor burst uploads",
		PayloadSize: 50,
		Count:       50,
		Concurrent:  5,
		Method:      "POST",
		Path:        "/telemetry",
		IsNON:       true,
	}

	// Streaming scenarios (for QUIC streaming comparison)
	StreamingSmallPayload = TestScenario{
		Name:        "Streaming 1KB Transfer",
		Description: "Tests streaming transfer for small payloads",
		PayloadSize: 1024, // 1KB
		Count:       50,
		Concurrent:  1,
		Method:      "GET",
		Path:        "/firmware",
		IsNON:       false,
	}

	StreamingMediumPayload = TestScenario{
		Name:        "Streaming 10KB Transfer",
		Description: "Tests streaming transfer for medium payloads",
		PayloadSize: 10240, // 10KB
		Count:       30,
		Concurrent:  1,
		Method:      "GET",
		Path:        "/firmware",
		IsNON:       false,
	}

	StreamingLargePayload = TestScenario{
		Name:        "Streaming 50KB Transfer",
		Description: "Tests streaming transfer for large payloads",
		PayloadSize: 51200, // 50KB
		Count:       20,
		Concurrent:  1,
		Method:      "GET",
		Path:        "/firmware",
		IsNON:       false,
	}

	StreamingVeryLargePayload = TestScenario{
		Name:        "Streaming 100KB Transfer",
		Description: "Tests streaming transfer for very large payloads",
		PayloadSize: 102400, // 100KB
		Count:       10,
		Concurrent:  1,
		Method:      "GET",
		Path:        "/firmware",
		IsNON:       false,
	}
)

// DefaultScenarios returns a standard set of test scenarios
func DefaultScenarios() []TestScenario {
	return []TestScenario{
		SmallPayloadGET,
		SmallPayloadPOST,
		SmallPayloadNON,
		MediumPayloadPOST,
		LargePayloadPOST,
		VeryLargePayloadPOST,
		BurstSmallPayload,
	}
}

// StreamingScenarios returns scenarios for testing streaming transfers
func StreamingScenarios() []TestScenario {
	return []TestScenario{
		StreamingSmallPayload,
		StreamingMediumPayload,
		StreamingLargePayload,
		StreamingVeryLargePayload,
	}
}
