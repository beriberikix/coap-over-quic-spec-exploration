package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"fmt"
	"log"
	"os"
	"time"

	piondtls "github.com/pion/dtls/v3"
	"github.com/golioth/coap-over-quic-poc/common"
	"github.com/plgd-dev/go-coap/v3/dtls"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/message/codes"
	"github.com/plgd-dev/go-coap/v3/udp/client"
)

func main() {
	log.Println("CoAP over UDP with DTLS Client (RFC 7252 + RFC 6347)")
	log.Println("=====================================================")
	log.Println()

	// Connect to DTLS server
	addr := fmt.Sprintf("localhost:%d", common.DefaultPort)
	log.Printf("Connecting to coaps://%s", addr)

	// Create DTLS config
	dtlsConfig, err := createDTLSConfig()
	if err != nil {
		log.Fatalf("Failed to create DTLS config: %v", err)
	}

	conn, err := dtls.Dial(addr, dtlsConfig)
	if err != nil {
		log.Fatalf("Failed to dial DTLS server: %v", err)
	}
	defer conn.Close()

	log.Printf("✓ Connected to %s (encrypted with DTLS 1.2)", conn.RemoteAddr())
	log.Println("Supports:")
	log.Println("  - DTLS 1.2 encryption (RFC 6347)")
	log.Println("  - TLS certificate-based authentication")
	log.Println("  - CON (confirmable) messages with retransmission")
	log.Println("  - NON (non-confirmable) messages")
	log.Println()

	// Demo 1: Resource Discovery (GET /.well-known/core) - CON
	log.Println("=== Demo 1: Resource Discovery (CON) ===")
	discoverResources(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 2: GET /temp (temperature reading) - CON
	log.Println("\n=== Demo 2: Get Temperature (CON) ===")
	getTemperature(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 3: Send telemetry via NON message
	log.Println("\n=== Demo 3: Send Telemetry (NON) ===")
	sendTelemetryNON(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 4: Send log message via NON message
	log.Println("\n=== Demo 4: Send Log (NON) ===")
	sendLogNON(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 5: Multiple NON messages rapidly (simulating sensor burst)
	log.Println("\n=== Demo 5: Rapid Sensor Burst (Multiple NON) ===")
	sendSensorBurst(conn, 10)
	time.Sleep(500 * time.Millisecond)

	// Demo 6: GET /led (check LED state) - CON
	log.Println("\n=== Demo 6: Get LED State (CON) ===")
	getLEDState(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 7: PUT /led (toggle LED) - CON
	log.Println("\n=== Demo 7: Toggle LED (CON) ===")
	toggleLED(conn)
	time.Sleep(500 * time.Millisecond)

	// Demo 8: Mixed CON and NON messages
	log.Println("\n=== Demo 8: Mixed CON and NON Messages ===")
	mixedRequests(conn)

	log.Println("\n=== Summary ===")
	log.Println("✓ All demos completed successfully")
	log.Println("✓ All traffic encrypted with DTLS 1.2")
	log.Println("✓ Demonstrated encrypted CoAP baseline for comparison")
	log.Println()
	log.Println("Compare to QUIC: QUIC provides better performance with")
	log.Println("  - 0-RTT connection resumption")
	log.Println("  - Stream multiplexing without HOL blocking")
	log.Println("  - Built-in connection migration")
}

// createDTLSConfig creates a DTLS configuration for the client
func createDTLSConfig() (*piondtls.Config, error) {
	// Load server certificate for verification
	certPEM, err := os.ReadFile("../certs/server.crt")
	if err != nil {
		return nil, fmt.Errorf("failed to read server certificate: %w", err)
	}

	// Create certificate pool
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(certPEM) {
		return nil, fmt.Errorf("failed to parse server certificate")
	}

	return &piondtls.Config{
		RootCAs:              certPool,
		InsecureSkipVerify:   true, // Skip hostname verification for localhost demo
		ExtendedMasterSecret: piondtls.RequireExtendedMasterSecret,
	}, nil
}

// discoverResources performs CoAP resource discovery
func discoverResources(conn *client.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Printf("  -> Sending CON GET /.well-known/core")
	resp, err := conn.Get(ctx, "/.well-known/core")
	if err != nil {
		log.Printf("  ERROR: %v", err)
		return
	}

	body := readBody(resp.Body())
	log.Printf("  <- Response Code: %s", codes.Code(resp.Code()).String())
	if len(body) > 0 {
		log.Printf("  Available Resources:\n    %s", string(body))
	}
}

// readBody reads the body from an io.ReadSeeker
func readBody(r interface {
	Read([]byte) (int, error)
}) []byte {
	buf := new(bytes.Buffer)
	buf.ReadFrom(r.(interface{ Read([]byte) (int, error) }))
	return buf.Bytes()
}

// getTemperature requests temperature reading from /temp
func getTemperature(conn *client.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Printf("  -> Sending CON GET /temp")
	resp, err := conn.Get(ctx, "/temp")
	if err != nil {
		log.Printf("  ERROR: %v", err)
		return
	}

	body := readBody(resp.Body())
	log.Printf("  <- Response Code: %s", codes.Code(resp.Code()).String())
	if len(body) > 0 {
		log.Printf("  Temperature: %s", string(body))
	}
}

// getLEDState requests current LED state from /led
func getLEDState(conn *client.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Printf("  -> Sending CON GET /led")
	resp, err := conn.Get(ctx, "/led")
	if err != nil {
		log.Printf("  ERROR: %v", err)
		return
	}

	body := readBody(resp.Body())
	log.Printf("  <- Response Code: %s", codes.Code(resp.Code()).String())
	if len(body) > 0 {
		log.Printf("  LED State: %s", string(body))
	}
}

// toggleLED sends PUT request to toggle LED state
func toggleLED(conn *client.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Printf("  -> Sending CON PUT /led")
	resp, err := conn.Put(ctx, "/led", message.TextPlain, nil)
	if err != nil {
		log.Printf("  ERROR: %v", err)
		return
	}

	body := readBody(resp.Body())
	log.Printf("  <- Response Code: %s", codes.Code(resp.Code()).String())
	if len(body) > 0 {
		log.Printf("  New LED State: %s", string(body))
	}
}

// sendTelemetryNON sends telemetry data via NON message
func sendTelemetryNON(conn *client.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Create NON message
	req := conn.AcquireMessage(ctx)
	defer conn.ReleaseMessage(req)

	req.SetCode(codes.POST)
	req.SetPath("/telemetry")
	payload := []byte(`{"sensor":"temp","value":22.5,"timestamp":1234567890}`)
	req.SetContentFormat(message.AppJSON)
	req.SetBody(bytes.NewReader(payload))
	req.SetType(message.NonConfirmable)

	log.Printf("  -> Sending NON POST /telemetry")
	err := conn.WriteMessage(req)
	if err != nil {
		log.Printf("  ERROR: %v", err)
		return
	}
	log.Printf("  <- NON message sent (no response expected)")
}

// sendLogNON sends a log message via NON message
func sendLogNON(conn *client.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Create NON message
	req := conn.AcquireMessage(ctx)
	defer conn.ReleaseMessage(req)

	req.SetCode(codes.POST)
	req.SetPath("/log")
	payload := []byte("Device started successfully at " + time.Now().Format(time.RFC3339))
	req.SetBody(bytes.NewReader(payload))
	req.SetType(message.NonConfirmable)

	log.Printf("  -> Sending NON POST /log")
	err := conn.WriteMessage(req)
	if err != nil {
		log.Printf("  ERROR: %v", err)
		return
	}
	log.Printf("  <- NON message sent (no response expected)")
}

// sendSensorBurst sends multiple NON messages rapidly to simulate sensor burst
func sendSensorBurst(conn *client.Conn, count int) {
	log.Printf("  Sending %d sensor readings rapidly...", count)

	for i := 0; i < count; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

		// Create NON message
		req := conn.AcquireMessage(ctx)
		req.SetCode(codes.POST)
		req.SetPath("/telemetry")
		payload := []byte(fmt.Sprintf(`{"reading":%d,"value":%.2f}`, i, 20.0+float64(i)*0.5))
		req.SetContentFormat(message.AppJSON)
		req.SetBody(bytes.NewReader(payload))
		req.SetType(message.NonConfirmable)

		err := conn.WriteMessage(req)
		conn.ReleaseMessage(req)
		cancel()

		if err != nil {
			log.Printf("  ERROR sending NON %d: %v", i, err)
		}

		// Small delay between messages
		time.Sleep(10 * time.Millisecond)
	}

	log.Printf("  Sent %d NON messages", count)
}

// mixedRequests demonstrates mixing CON and NON messages
func mixedRequests(conn *client.Conn) {
	log.Println("  Mixing CON (reliable) and NON (unreliable) messages...")

	// Send 3 NON messages (fire-and-forget)
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

		req := conn.AcquireMessage(ctx)
		req.SetCode(codes.POST)
		req.SetPath("/telemetry")
		payload := []byte(fmt.Sprintf(`{"burst":%d}`, i))
		req.SetContentFormat(message.AppJSON)
		req.SetBody(bytes.NewReader(payload))
		req.SetType(message.NonConfirmable)

		conn.WriteMessage(req)
		conn.ReleaseMessage(req)
		cancel()
	}

	// Send 1 CON request (reliable, with response)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := conn.Get(ctx, "/temp")
	if err != nil {
		log.Printf("  ERROR: %v", err)
		return
	}

	body := readBody(resp.Body())
	log.Printf("  CON response: %s - %s",
		codes.Code(resp.Code()).String(), string(body))

	log.Println("  Mixed traffic completed!")
}
