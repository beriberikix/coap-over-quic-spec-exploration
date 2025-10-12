package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/golioth/coap-over-quic-poc/common"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/udp/coder"
	"github.com/quic-go/quic-go"
)

var codec = coder.DefaultCoder

func main() {
	log.Println("CoAP over QUIC Client Starting (using go-coap)...")

	// Configure TLS (skip verification for self-signed cert in demo)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // Don't do this in production!
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		NextProtos:         []string{"coap-quic-poc"},
	}

	// Configure QUIC
	quicConfig := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
	}

	// Establish QUIC connection to server
	// Per spec: client initiates connection with TLS 1.3
	addr := fmt.Sprintf("localhost:%d", common.DefaultPort)
	log.Printf("Connecting to coaps+quic://%s", addr)

	conn, err := quic.DialAddr(context.Background(), addr, tlsConfig, quicConfig)
	if err != nil {
		log.Fatalf("Failed to establish QUIC connection: %v", err)
	}
	defer conn.CloseWithError(0, "")

	log.Printf("Connected to %s", conn.RemoteAddr())
	log.Println()

	// Demo 1: Resource Discovery (GET /.well-known/core)
	log.Println("=== Demo 1: Resource Discovery ===")
	discoverResources(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 2: GET /temp (temperature reading)
	log.Println("\n=== Demo 2: Get Temperature ===")
	getTemperature(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 3: GET /led (check LED state)
	log.Println("\n=== Demo 3: Get LED State ===")
	getLEDState(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 4: PUT /led (toggle LED)
	log.Println("\n=== Demo 4: Toggle LED ===")
	toggleLED(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 5: Multiple concurrent requests (no head-of-line blocking)
	log.Println("\n=== Demo 5: Concurrent Requests (No Head-of-Line Blocking) ===")
	concurrentRequests(conn)

	log.Println("\n=== All demos completed successfully ===")
}

// sendRequest sends a CoAP request over a bidirectional QUIC stream
// Per spec (Section 3.1): Request/response pair on single bidirectional stream
func sendRequest(conn *quic.Conn, request *message.Message) (*message.Message, error) {
	// Open a new bidirectional stream for this request
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	// Marshal the CoAP request using go-coap UDP coder (same format for QUIC)
	requestData := make([]byte, 2048)
	requestLen, err := codec.Encode(*request, requestData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	requestData = requestData[:requestLen]

	// Send request and set FIN bit (implicit with Write)
	// Per spec: client MUST set FIN on last STREAM frame
	path := common.GetPath(request)
	log.Printf("  -> Stream %d: Sending %s %s (%d bytes)",
		stream.StreamID(), common.MethodToString(request.Code), path, len(requestData))

	_, err = stream.Write(requestData)
	if err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Close to signal end of request (sends FIN on write side)
	// Per spec: client MUST set FIN bit
	stream.Close()

	// Read response from same stream
	// Per spec: server sends response on same stream with FIN
	responseData, err := io.ReadAll(stream)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("  <- Stream %d: Received %d bytes", stream.StreamID(), len(responseData))

	// Parse CoAP response using go-coap UDP coder
	response := message.Message{
		Options: make(message.Options, 0, 16), // Pre-allocate options buffer
	}
	_, err = codec.Decode(responseData, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// discoverResources performs CoAP resource discovery
func discoverResources(conn *quic.Conn) {
	token := generateToken()
	request := common.NewMessage(common.Confirmable, common.GET, token)

	// Add URI path options for /.well-known/core
	common.AddPathOption(request, ".well-known")
	common.AddPathOption(request, "core")

	response, err := sendRequest(conn, request)
	if err != nil {
		log.Printf("  ERROR: %v", err)
		return
	}

	log.Printf("  Response Code: %s", common.CodeToString(response.Code))
	if len(response.Payload) > 0 {
		log.Printf("  Available Resources:\n    %s", string(response.Payload))
	}
}

// getTemperature requests temperature reading from /temp
func getTemperature(conn *quic.Conn) {
	token := generateToken()
	request := common.NewMessage(common.Confirmable, common.GET, token)
	common.AddPathOption(request, "temp")

	response, err := sendRequest(conn, request)
	if err != nil {
		log.Printf("  ERROR: %v", err)
		return
	}

	log.Printf("  Response Code: %s", common.CodeToString(response.Code))
	if len(response.Payload) > 0 {
		log.Printf("  Temperature: %s", string(response.Payload))
	}
}

// getLEDState requests current LED state from /led
func getLEDState(conn *quic.Conn) {
	token := generateToken()
	request := common.NewMessage(common.Confirmable, common.GET, token)
	common.AddPathOption(request, "led")

	response, err := sendRequest(conn, request)
	if err != nil {
		log.Printf("  ERROR: %v", err)
		return
	}

	log.Printf("  Response Code: %s", common.CodeToString(response.Code))
	if len(response.Payload) > 0 {
		log.Printf("  LED State: %s", string(response.Payload))
	}
}

// toggleLED sends PUT request to toggle LED state
func toggleLED(conn *quic.Conn) {
	token := generateToken()
	request := common.NewMessage(common.Confirmable, common.PUT, token)
	common.AddPathOption(request, "led")

	response, err := sendRequest(conn, request)
	if err != nil {
		log.Printf("  ERROR: %v", err)
		return
	}

	log.Printf("  Response Code: %s", common.CodeToString(response.Code))
	if len(response.Payload) > 0 {
		log.Printf("  New LED State: %s", string(response.Payload))
	}
}

// concurrentRequests demonstrates multiple concurrent requests without head-of-line blocking
// Per spec (Section 3): Multiple concurrent streams avoid head-of-line blocking
func concurrentRequests(conn *quic.Conn) {
	log.Println("  Sending 3 concurrent requests on separate streams...")

	// Channel to collect results
	done := make(chan string, 3)

	// Request 1: Temperature
	go func() {
		token := generateToken()
		request := common.NewMessage(common.Confirmable, common.GET, token)
		common.AddPathOption(request, "temp")

		response, err := sendRequest(conn, request)
		if err != nil {
			done <- fmt.Sprintf("Temperature request failed: %v", err)
			return
		}

		done <- fmt.Sprintf("Temperature: %s (Code: %s)",
			string(response.Payload), common.CodeToString(response.Code))
	}()

	// Request 2: LED state
	go func() {
		token := generateToken()
		request := common.NewMessage(common.Confirmable, common.GET, token)
		common.AddPathOption(request, "led")

		response, err := sendRequest(conn, request)
		if err != nil {
			done <- fmt.Sprintf("LED request failed: %v", err)
			return
		}

		done <- fmt.Sprintf("LED: %s (Code: %s)",
			string(response.Payload), common.CodeToString(response.Code))
	}()

	// Request 3: Resource discovery
	go func() {
		token := generateToken()
		request := common.NewMessage(common.Confirmable, common.GET, token)
		common.AddPathOption(request, ".well-known")
		common.AddPathOption(request, "core")

		response, err := sendRequest(conn, request)
		if err != nil {
			done <- fmt.Sprintf("Discovery request failed: %v", err)
			return
		}

		done <- fmt.Sprintf("Discovery: %s (Code: %s)",
			string(response.Payload), common.CodeToString(response.Code))
	}()

	// Wait for all responses
	for i := 0; i < 3; i++ {
		result := <-done
		log.Printf("  [%d] %s", i+1, result)
	}

	log.Println("  All concurrent requests completed successfully!")
}

// generateToken creates a random CoAP token
// Per spec (Section 5.2): Token MUST be used to match request/response
func generateToken() []byte {
	token := make([]byte, 4)
	rand.Read(token)
	return token
}
