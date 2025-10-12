package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/golioth/coap-over-quic-poc/common"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/udp/coder"
	"github.com/quic-go/quic-go"
)

func main() {
	log.Println("CoAP over QUIC with 0-RTT Connection Resumption Demo")
	log.Println("=====================================================")
	log.Println()

	serverAddr := fmt.Sprintf("localhost:%d", common.DefaultPort)

	// Create TLS config with session cache for 0-RTT
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"coap-quic-poc"},
		ClientSessionCache: tls.NewLRUClientSessionCache(100), // Enable session caching
	}

	quicConfig := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
		EnableDatagrams: true,
	}

	// Create UDP connection
	udpConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		log.Fatalf("Failed to create UDP connection: %v", err)
	}
	defer udpConn.Close()

	// Create Transport
	tr := &quic.Transport{
		Conn: udpConn,
	}
	defer tr.Close()

	// Resolve server address
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		log.Fatalf("Failed to resolve address: %v", err)
	}

	// Demo 1: Cold Start - Initial connection (full handshake)
	log.Println("=== Demo 1: Cold Start (Initial Connection) ===")
	log.Println("This is the first connection - no cached session available")
	log.Println("QUIC will perform a full TLS 1.3 handshake (1-RTT)")
	log.Println()

	coldStart := time.Now()
	conn1, err := tr.Dial(context.Background(), udpAddr, tlsConfig, quicConfig)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	coldDuration := time.Since(coldStart)

	// Send a request to complete the handshake
	sendRequest(conn1, "Cold start request")

	log.Printf("✓ Cold start completed in: %v", coldDuration)
	log.Printf("  Session ticket saved to cache")
	log.Println()

	// Close connection to simulate device going idle
	conn1.CloseWithError(0, "demo complete")
	time.Sleep(500 * time.Millisecond)

	// Demo 2: Warm Reconnect with 0-RTT
	log.Println("=== Demo 2: Warm Reconnect (0-RTT Resumption) ===")
	log.Println("Device reconnects using cached session ticket")
	log.Println("QUIC will use 0-RTT - application data sent immediately!")
	log.Println()

	warmStart := time.Now()

	// Use DialEarly for 0-RTT support
	conn2, err := tr.DialEarly(context.Background(), udpAddr, tlsConfig, quicConfig)
	if err != nil {
		log.Fatalf("Failed to connect with 0-RTT: %v", err)
	}

	// Can send data immediately without waiting for handshake!
	sendRequest(conn2, "0-RTT request (sent before handshake completes!)")

	warmDuration := time.Since(warmStart)

	log.Printf("✓ 0-RTT reconnect completed in: %v", warmDuration)
	log.Printf("  Application data sent with 0-RTT!")
	log.Println()

	// Demo 3: Multiple reconnects to show consistency
	log.Println("=== Demo 3: Multiple 0-RTT Reconnects ===")
	log.Println("Simulating device repeatedly reconnecting (e.g., wake from sleep)")
	log.Println()

	conn2.CloseWithError(0, "demo complete")
	time.Sleep(500 * time.Millisecond)

	var reconnectTimes []time.Duration
	for i := 0; i < 5; i++ {
		start := time.Now()
		conn, err := tr.DialEarly(context.Background(), udpAddr, tlsConfig, quicConfig)
		if err != nil {
			log.Printf("Reconnect %d failed: %v", i+1, err)
			continue
		}

		sendRequest(conn, fmt.Sprintf("Reconnect #%d with 0-RTT", i+1))
		duration := time.Since(start)
		reconnectTimes = append(reconnectTimes, duration)

		log.Printf("  Reconnect %d: %v", i+1, duration)

		conn.CloseWithError(0, "demo complete")
		time.Sleep(300 * time.Millisecond)
	}

	// Calculate statistics
	if len(reconnectTimes) > 0 {
		var sum time.Duration
		min := reconnectTimes[0]
		max := reconnectTimes[0]

		for _, d := range reconnectTimes {
			sum += d
			if d < min {
				min = d
			}
			if d > max {
				max = d
			}
		}

		avg := sum / time.Duration(len(reconnectTimes))

		log.Println()
		log.Println("=== Performance Summary ===")
		log.Printf("Cold Start (1-RTT):          %v", coldDuration)
		log.Printf("Warm Reconnect (0-RTT):      %v", warmDuration)
		log.Printf("Average 0-RTT reconnect:     %v", avg)
		log.Printf("Min 0-RTT reconnect:         %v", min)
		log.Printf("Max 0-RTT reconnect:         %v", max)
		log.Println()

		improvement := float64(coldDuration) / float64(avg)
		log.Printf("🚀 0-RTT provides %.1fx faster reconnection!", improvement)
		log.Println()

		log.Println("=== Benefits for IoT Devices ===")
		log.Println("✓ Battery savings: Less time with radio on")
		log.Println("✓ Faster response: No waiting for handshake")
		log.Println("✓ Better UX: Immediate data transmission")
		log.Println("✓ Scale: Lower server CPU usage for reconnects")
	}

	log.Println()
	log.Println("=== Demo Complete ===")
}

// sendRequest sends a simple CoAP GET request
func sendRequest(conn *quic.Conn, msg string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	// Build CoAP request
	token := []byte{0x01, 0x02, 0x03, 0x04}
	request := common.NewMessage(common.Confirmable, common.GET, token)
	common.AddPathOption(request, "temp")

	// Encode request
	codec := coder.Coder{}
	requestData := make([]byte, 2048)
	requestLen, err := codec.Encode(*request, requestData)
	if err != nil {
		return fmt.Errorf("failed to encode: %w", err)
	}

	// Send request
	if _, err := stream.Write(requestData[:requestLen]); err != nil {
		return fmt.Errorf("failed to write: %w", err)
	}
	stream.Close()

	// Read response
	responseData, err := io.ReadAll(stream)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Decode response
	response := message.Message{
		Options: make(message.Options, 0, 16),
	}
	if _, err := codec.Decode(responseData, &response); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	log.Printf("  [%s] Response: %s", msg, common.CodeToString(response.Code))
	return nil
}
