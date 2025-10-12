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
	transport := flag.String("transport", "all", "Transport to test (quic-stream, quic-datagram, udp, or all)")
	flag.Parse()

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

	// Get test scenarios
	scenarios := harness.DefaultScenarios()

	// Determine which transports to test
	var clients []harness.TransportClient
	switch *transport {
	case "quic-stream":
		clients = []harness.TransportClient{transports.NewQUICStreamClient()}
	case "quic-datagram":
		clients = []harness.TransportClient{transports.NewQUICDatagramClient()}
	case "udp":
		clients = []harness.TransportClient{transports.NewUDPClient()}
	case "all":
		clients = []harness.TransportClient{
			transports.NewQUICStreamClient(),
			transports.NewQUICDatagramClient(),
			transports.NewUDPClient(),
		}
	default:
		log.Fatalf("Unknown transport: %s", *transport)
	}

	// Run benchmarks
	log.Println("=== Starting Benchmark Suite ===\n")
	startTime := time.Now()

	for _, client := range clients {
		log.Printf("\n--- Testing Transport: %s ---\n", client.Name())

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
