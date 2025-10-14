package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/golioth/coap-over-quic-spec-exploration/examples/benchmark/harness"
	"github.com/golioth/coap-over-quic-spec-exploration/examples/benchmark/transports"
)

func main() {
	// Command-line flags
	serverAddr := flag.String("server", "localhost:5683", "Server address")
	outputDir := flag.String("output", "./results", "Output directory for results")
	transport := flag.String("transport", "", "Transport to test (required)")
	flag.Parse()

	// Validate transport flag
	if *transport == "" {
		log.Println("Error: -transport flag is required")
		log.Println("Usage: ./benchmark -transport <type> [-server <addr>] [-output <dir>]")
		log.Println("\nAvailable transports:")
		log.Println("  Basic transports:")
		log.Println("    quic-stream      - CoAP over QUIC with bidirectional streams")
		log.Println("    quic-datagram    - CoAP over QUIC with streams + datagrams (RFC 9221)")
		log.Println("    udp              - Traditional CoAP over UDP")
		log.Println("    dtls             - CoAP over UDP with DTLS 1.2")
		log.Println("  Advanced transports:")
		log.Println("    quic-streaming   - QUIC native streaming (vs block-wise)")
		log.Println("    quic-0rtt        - 0-RTT connection resumption testing")
		log.Println("    quic-migration   - Connection migration testing")
		log.Println("    quic-observe     - Observe pattern (RFC 7641) testing")
		os.Exit(1)
	}

	log.Printf("CoAP Transport Benchmark")
	log.Printf("Server: %s", *serverAddr)
	log.Printf("Transport: %s", *transport)
	log.Println()

	// Create output directory
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Create benchmark runner
	runner := harness.NewBenchmarkRunner(*serverAddr)

	// Select transport client
	var client harness.TransportClient
	switch *transport {
	case "quic-stream":
		client = transports.NewQUICStreamClient()
	case "quic-datagram":
		client = transports.NewQUICDatagramClient()
	case "udp":
		client = transports.NewUDPClient()
	case "dtls":
		client = transports.NewDTLSClient()
	case "quic-streaming":
		client = transports.NewQUICStreamingClient()
	case "quic-0rtt":
		client = transports.NewQUIC0RTTClient()
	case "quic-migration":
		client = transports.NewQUICMigrationClient()
	case "quic-observe":
		client = transports.NewQUICObserveClient()
	default:
		log.Fatalf("Unknown transport: %s (run without -transport to see options)", *transport)
	}

	// Select appropriate scenarios based on transport
	var scenarios []harness.TestScenario
	if *transport == "quic-streaming" {
		scenarios = harness.StreamingScenarios()
	} else {
		scenarios = harness.DefaultScenarios()
	}

	// Run benchmarks
	log.Println("=== Starting Benchmark Suite ===\n")
	log.Printf("--- Testing Transport: %s ---\n", client.Name())
	startTime := time.Now()

	for _, scenario := range scenarios {
		// Skip NON scenarios for transports that don't support them well
		if scenario.IsNON && client.Name() == "quic-stream" {
			log.Printf("Skipping %s (NON not applicable for streams)\n", scenario.Name)
			continue
		}

		if err := runner.RunScenario(client, scenario); err != nil {
			log.Printf("ERROR in scenario %s: %v\n", scenario.Name, err)
		}

		// Small delay between scenarios
		time.Sleep(1 * time.Second)
	}

	elapsed := time.Since(startTime)
	log.Printf("\n=== Benchmark Suite Complete ===")
	log.Printf("Total time: %v\n", elapsed)

	// Get metrics collector
	collector := runner.GetCollector()

	// Print summary
	collector.PrintSummary()

	// Export results to CSV
	timestamp := time.Now().Format("20060102-150405")
	csvFile := filepath.Join(*outputDir, fmt.Sprintf("benchmark-%s.csv", timestamp))
	log.Printf("\nExporting results to %s...", csvFile)
	if err := collector.ExportCSV(csvFile); err != nil {
		log.Fatalf("Failed to export CSV: %v", err)
	}

	log.Println("Done!")
}
