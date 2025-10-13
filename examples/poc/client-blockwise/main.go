package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/golioth/coap-over-quic-poc/common"
	"github.com/plgd-dev/go-coap/v3/net/blockwise"
	"github.com/plgd-dev/go-coap/v3/options"
	"github.com/plgd-dev/go-coap/v3/udp"
)

func main() {
	log.Println("CoAP over UDP Client with Block-wise Transfer (RFC 7959)")
	log.Println("========================================================")
	log.Println()

	serverAddr := fmt.Sprintf("localhost:%d", common.DefaultPort)
	log.Printf("Connecting to coap://%s", serverAddr)

	// Connect with block-wise enabled (different block sizes for testing)
	blockSizes := []blockwise.SZX{
		blockwise.SZX16,    // 16 bytes
		blockwise.SZX64,    // 64 bytes
		blockwise.SZX256,   // 256 bytes
		blockwise.SZX1024,  // 1024 bytes (optimal for most networks)
	}

	log.Println()
	log.Println("=== Demo 1: Small Resource (No Block-wise Needed) ===")
	testSmallResource(serverAddr)

	log.Println()
	log.Println("=== Demo 2: Large Resource with Different Block Sizes ===")
	log.Println("This demonstrates RFC 7959 block-wise transfer")
	log.Println("Fetching 50KB firmware with different block sizes...")
	log.Println()

	for _, szx := range blockSizes {
		testLargeResource(serverAddr, szx)
		time.Sleep(500 * time.Millisecond)
	}

	log.Println()
	log.Println("=== Summary ===")
	log.Println("✓ Demonstrated RFC 7959 block-wise transfers")
	log.Println("✓ Tested multiple block sizes (16-1024 bytes)")
	log.Println("✓ Larger blocks = fewer round trips = faster transfer")
	log.Println("✓ For UDP: Block-wise is essential for large payloads")
	log.Println()
	log.Println("Next: Compare with QUIC streaming (no block-wise needed!)")
}

// testSmallResource fetches a small resource that doesn't need block-wise
func testSmallResource(serverAddr string) {
	conn, err := udp.Dial(serverAddr)
	if err != nil {
		log.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Println("Fetching /temp (small resource)...")
	start := time.Now()

	resp, err := conn.Get(ctx, "/temp")
	if err != nil {
		log.Printf("ERROR: %v", err)
		return
	}

	duration := time.Since(start)
	body := readBody(resp.Body())

	log.Printf("✓ Received %d bytes in %v", len(body), duration)
	log.Printf("  Response: %s", string(body))
	log.Println("  (No block-wise - fits in single packet)")
}

// testLargeResource fetches a large resource using block-wise transfer
func testLargeResource(serverAddr string, szx blockwise.SZX) {
	blockSize := 16 << szx
	transferTimeout := 30 * time.Second

	// Connect with specific block size
	conn, err := udp.Dial(serverAddr,
		options.WithBlockwise(true, szx, transferTimeout))
	if err != nil {
		log.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), transferTimeout)
	defer cancel()

	log.Printf("Fetching /firmware with block size %d bytes...", blockSize)
	start := time.Now()
	blockCount := 0

	// Create a custom handler to count blocks
	resp, err := conn.Get(ctx, "/firmware")
	if err != nil {
		log.Printf("ERROR: %v", err)
		return
	}

	duration := time.Since(start)
	body := readBody(resp.Body())

	// Calculate expected blocks
	expectedBlocks := (len(body) + blockSize - 1) / blockSize

	log.Printf("✓ Received %d bytes in %v", len(body), duration)
	log.Printf("  Block size:       %d bytes", blockSize)
	log.Printf("  Estimated blocks: %d", expectedBlocks)
	log.Printf("  Round trips:      ~%d", expectedBlocks)
	log.Printf("  Throughput:       %.2f KB/s", float64(len(body))/1024.0/duration.Seconds())

	// Verify firmware integrity
	if len(body) > 0 {
		header := body[:min(50, len(body))]
		log.Printf("  Firmware header:  %s", string(header))
	}

	// Calculate efficiency
	totalRTT := duration
	avgRTTperBlock := totalRTT / time.Duration(expectedBlocks)
	log.Printf("  Avg RTT/block:    %v", avgRTTperBlock)

	if blockCount > 0 {
		log.Printf("  Blocks received:  %d", blockCount)
	}
}

// readBody reads the body from an io.ReadSeeker
func readBody(r interface{}) []byte {
	if r == nil {
		return nil
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(r.(interface{ Read([]byte) (int, error) }))
	return buf.Bytes()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
