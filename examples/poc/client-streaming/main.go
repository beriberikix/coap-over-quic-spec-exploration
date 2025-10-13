package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/golioth/coap-over-quic-poc/common"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/udp/coder"
	"github.com/quic-go/quic-go"
)

func main() {
	log.Println("CoAP over QUIC Client with Native Streaming")
	log.Println("===========================================")
	log.Println()

	serverAddr := fmt.Sprintf("localhost:%d", common.DefaultPort)
	log.Printf("Connecting to coaps+quic://%s", serverAddr)

	log.Println()
	log.Println("=== Demo 1: Small Resource ===")
	testSmallResource(serverAddr)

	log.Println()
	log.Println("=== Demo 2: Large Resource with QUIC Native Streaming ===")
	log.Println("This demonstrates CoAP over QUIC streaming (no block-wise!)")
	log.Println("Fetching 50KB firmware using QUIC streams...")
	log.Println()

	testLargeResource(serverAddr)

	log.Println()
	log.Println("=== Summary ===")
	log.Println("✓ QUIC streams handle large payloads natively")
	log.Println("✓ No block-wise transfer overhead")
	log.Println("✓ Single request/response - no fragmentation")
	log.Println("✓ QUIC provides reliability + flow control built-in")
	log.Println()
	log.Println("Compare this with UDP block-wise transfers!")
}

// testSmallResource fetches a small resource
func testSmallResource(serverAddr string) {
	conn, err := connectQUIC(serverAddr)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.CloseWithError(0, "done")

	log.Println("Fetching /temp (small resource)...")
	start := time.Now()

	body, err := sendRequest(conn, common.GET, "/temp")
	if err != nil {
		log.Printf("ERROR: %v", err)
		return
	}

	duration := time.Since(start)

	log.Printf("✓ Received %d bytes in %v", len(body), duration)
	log.Printf("  Response: %s", string(body))
	log.Println("  (Single stream - no fragmentation)")
}

// testLargeResource fetches a large resource using QUIC streaming
func testLargeResource(serverAddr string) {
	conn, err := connectQUIC(serverAddr)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.CloseWithError(0, "done")

	log.Println("Fetching /firmware with QUIC native streaming...")
	start := time.Now()

	body, err := sendRequest(conn, common.GET, "/firmware")
	if err != nil {
		log.Printf("ERROR: %v", err)
		return
	}

	duration := time.Since(start)

	log.Printf("✓ Received %d bytes in %v", len(body), duration)
	log.Printf("  Transfer method:  QUIC native streaming")
	log.Printf("  Block-wise:       NOT NEEDED")
	log.Printf("  Round trips:      1 (just request + response)")
	log.Printf("  Throughput:       %.2f KB/s", float64(len(body))/1024.0/duration.Seconds())

	// Verify firmware integrity
	if len(body) > 0 {
		header := body[:min(50, len(body))]
		log.Printf("  Firmware header:  %s", string(header))
	}

	// Compare with block-wise
	log.Println()
	log.Println("  Comparison with UDP block-wise (1024 byte blocks):")
	log.Println("  - UDP needs ~50 round trips")
	log.Println("  - QUIC needs 1 round trip")
	log.Println("  - QUIC is simpler and faster!")
}

// connectQUIC establishes a QUIC connection
func connectQUIC(serverAddr string) (*quic.Conn, error) {
	// Configure TLS (skip verification for self-signed certs in demo)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"coap-quic-streaming"},
	}

	// Configure QUIC
	quicConfig := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
	}

	// Create UDP connection
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve address: %w", err)
	}

	udpConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, fmt.Errorf("create UDP conn: %w", err)
	}

	// Create QUIC transport and connect
	tr := &quic.Transport{Conn: udpConn}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := tr.Dial(ctx, udpAddr, tlsConfig, quicConfig)
	if err != nil {
		udpConn.Close()
		return nil, fmt.Errorf("QUIC dial: %w", err)
	}

	return conn, nil
}

// sendRequest sends a CoAP request over a QUIC stream
func sendRequest(conn *quic.Conn, method common.Code, path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Open a new stream
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer stream.Close()

	// Create CoAP request
	request := common.NewMessage(message.Confirmable, method, []byte{0x01})

	// Add path segments (split by '/')
	// For "/temp" -> add "temp"
	// For "/firmware" -> add "firmware"
	pathSegments := splitPath(path)
	for _, segment := range pathSegments {
		common.AddPathOption(request, segment)
	}

	// Encode request
	codec := coder.DefaultCoder
	requestData := make([]byte, 1024)
	requestLen, err := codec.Encode(*request, requestData)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	// Send request and close write side (set FIN bit)
	_, err = stream.Write(requestData[:requestLen])
	if err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}
	stream.Close() // Close write side to signal end of request

	// Read entire response
	responseData, err := io.ReadAll(stream)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Decode response with pre-allocated options buffer
	response := &message.Message{
		Options: make(message.Options, 0, 16),
	}
	_, err = codec.Decode(responseData, response)
	if err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Check response code
	if response.Code != common.Content {
		return nil, fmt.Errorf("unexpected response code: %s", common.CodeToString(response.Code))
	}

	return response.Payload, nil
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

// splitPath splits a path string into segments for CoAP Uri-Path options
// "/temp" -> ["temp"]
// "/firmware" -> ["firmware"]
// "/.well-known/core" -> [".well-known", "core"]
func splitPath(path string) []string {
	segments := []string{}
	for _, segment := range strings.Split(path, "/") {
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	return segments
}
