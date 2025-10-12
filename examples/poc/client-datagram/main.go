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
	log.Println("CoAP over QUIC Client with Datagram Support (RFC 9221)...")

	// Configure TLS (skip verification for self-signed cert in demo)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // Don't do this in production!
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		NextProtos:         []string{"coap-quic-poc"},
	}

	// Configure QUIC with Datagram support
	quicConfig := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
		EnableDatagrams: true, // Enable RFC 9221 QUIC Datagrams
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
	log.Println("Supports:")
	log.Println("  - Bidirectional streams (reliable CON messages)")
	log.Println("  - QUIC Datagrams (unreliable NON messages, RFC 9221)")
	log.Println()

	// Demo 1: Resource Discovery (GET /.well-known/core) - Stream
	log.Println("=== Demo 1: Resource Discovery (Stream) ===")
	discoverResources(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 2: GET /temp (temperature reading) - Stream
	log.Println("\n=== Demo 2: Get Temperature (Stream) ===")
	getTemperature(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 3: Send telemetry via Datagram (NON message)
	log.Println("\n=== Demo 3: Send Telemetry via Datagram (NON) ===")
	sendTelemetryDatagram(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 4: Send log message via Datagram (NON message)
	log.Println("\n=== Demo 4: Send Log via Datagram (NON) ===")
	sendLogDatagram(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 5: Multiple datagrams rapidly (simulating sensor burst)
	log.Println("\n=== Demo 5: Rapid Sensor Burst (Multiple Datagrams) ===")
	sendSensorBurst(conn, 10)
	time.Sleep(500 * time.Millisecond)

	// Demo 6: GET /led (check LED state) - Stream
	log.Println("\n=== Demo 6: Get LED State (Stream) ===")
	getLEDState(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 7: PUT /led (toggle LED) - Stream
	log.Println("\n=== Demo 7: Toggle LED (Stream) ===")
	toggleLED(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 8: Concurrent requests (streams + datagrams)
	log.Println("\n=== Demo 8: Mixed Streams and Datagrams ===")
	mixedRequests(conn)

	log.Println("\n=== All demos completed successfully ===")
}

// sendRequest sends a CoAP request over a bidirectional QUIC stream (CON message)
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

// sendDatagram sends a CoAP NON message via QUIC datagram (RFC 9221)
// Per spec (Section 5.3): NON messages can use QUIC datagrams for unreliable delivery
func sendDatagram(conn *quic.Conn, request *message.Message) error {
	// Marshal the CoAP NON message
	requestData := make([]byte, 1200) // Max datagram size
	requestLen, err := codec.Encode(*request, requestData)
	if err != nil {
		return fmt.Errorf("failed to marshal datagram: %w", err)
	}
	requestData = requestData[:requestLen]

	path := common.GetPath(request)
	log.Printf("  -> Datagram: Sending %s %s (%d bytes)",
		common.MethodToString(request.Code), path, len(requestData))

	// Send datagram (fire-and-forget, no response expected)
	err = conn.SendDatagram(requestData)
	if err != nil {
		return fmt.Errorf("failed to send datagram: %w", err)
	}

	log.Printf("  <- Datagram sent (no response expected)")
	return nil
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

// sendTelemetryDatagram sends telemetry data via QUIC datagram (NON message)
func sendTelemetryDatagram(conn *quic.Conn) {
	token := generateToken()
	request := common.NewMessage(common.NonConfirmable, common.POST, token)
	common.AddPathOption(request, "telemetry")

	// Add some sensor data as payload
	payload := []byte(`{"sensor":"temp","value":22.5,"timestamp":1234567890}`)
	common.SetJSONPayload(request, payload)

	err := sendDatagram(conn, request)
	if err != nil {
		log.Printf("  ERROR: %v", err)
	}
}

// sendLogDatagram sends a log message via QUIC datagram (NON message)
func sendLogDatagram(conn *quic.Conn) {
	token := generateToken()
	request := common.NewMessage(common.NonConfirmable, common.POST, token)
	common.AddPathOption(request, "log")

	// Add log message as payload
	payload := []byte("Device started successfully at " + time.Now().Format(time.RFC3339))
	request.Payload = payload

	err := sendDatagram(conn, request)
	if err != nil {
		log.Printf("  ERROR: %v", err)
	}
}

// sendSensorBurst sends multiple datagrams rapidly to simulate sensor burst
func sendSensorBurst(conn *quic.Conn, count int) {
	log.Printf("  Sending %d sensor readings rapidly...", count)

	for i := 0; i < count; i++ {
		token := generateToken()
		request := common.NewMessage(common.NonConfirmable, common.POST, token)
		common.AddPathOption(request, "telemetry")

		// Simulate sensor reading
		payload := []byte(fmt.Sprintf(`{"reading":%d,"value":%.2f}`, i, 20.0+float64(i)*0.5))
		common.SetJSONPayload(request, payload)

		err := sendDatagram(conn, request)
		if err != nil {
			log.Printf("  ERROR sending datagram %d: %v", i, err)
		}

		// Small delay between datagrams
		time.Sleep(10 * time.Millisecond)
	}

	log.Printf("  Sent %d datagrams", count)
}

// mixedRequests demonstrates mixing streams and datagrams
func mixedRequests(conn *quic.Conn) {
	log.Println("  Mixing streams (reliable) and datagrams (unreliable)...")

	// Send 3 datagrams (fire-and-forget)
	for i := 0; i < 3; i++ {
		token := generateToken()
		request := common.NewMessage(common.NonConfirmable, common.POST, token)
		common.AddPathOption(request, "telemetry")
		payload := []byte(fmt.Sprintf(`{"burst":%d}`, i))
		common.SetJSONPayload(request, payload)
		sendDatagram(conn, request)
	}

	// Send 1 stream request (reliable, with response)
	token := generateToken()
	request := common.NewMessage(common.Confirmable, common.GET, token)
	common.AddPathOption(request, "temp")

	response, err := sendRequest(conn, request)
	if err != nil {
		log.Printf("  ERROR: %v", err)
		return
	}

	log.Printf("  Stream response: %s - %s",
		common.CodeToString(response.Code), string(response.Payload))

	log.Println("  Mixed traffic completed!")
}

// generateToken creates a random CoAP token
// Per spec (Section 5.2): Token MUST be used to match request/response
func generateToken() []byte {
	token := make([]byte, 4)
	rand.Read(token)
	return token
}
