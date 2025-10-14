package main

import (
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
	log.Println("CoAP over QUIC Client with Observe Support (RFC 7641)")
	log.Println("=====================================================")
	log.Println()

	serverAddr := fmt.Sprintf("localhost:%d", common.DefaultPort)
	log.Printf("Connecting to coaps+quic://%s", serverAddr)
	log.Println()

	// Connect to server
	conn, err := connectQUIC(serverAddr)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.CloseWithError(0, "done")

	log.Println("=== Demo: Observe Temperature Resource ===")
	log.Println("This demonstrates RFC 7641 Observe pattern over QUIC")
	log.Println()

	// Register observation
	log.Println("Step 1: Registering observation on /temp...")
	stream, initialTemp, err := registerObservation(conn, "/temp")
	if err != nil {
		log.Fatalf("Failed to register observation: %v", err)
	}

	log.Printf("✓ Observation registered! Initial temperature: %s", initialTemp)
	log.Println()
	log.Println("Step 2: Waiting for notifications...")
	log.Println("(Server updates temperature every 3 seconds)")
	log.Println()

	// Listen for notifications
	notificationCount := 0
	maxNotifications := 5

	for notificationCount < maxNotifications {
		notification, err := receiveNotification(stream)
		if err != nil {
			if err == io.EOF {
				log.Println("Stream closed by server")
				break
			}
			log.Printf("ERROR receiving notification: %v", err)
			break
		}

		notificationCount++
		log.Printf("[NOTIFICATION #%d] Temperature update: %s",
			notificationCount, notification)
	}

	log.Println()
	log.Println("=== Summary ===")
	log.Printf("✓ Received %d temperature notifications", notificationCount)
	log.Println("✓ Observe pattern successfully demonstrated over QUIC")
	log.Println("✓ Long-lived QUIC stream maintained throughout observations")
	log.Println("✓ Server pushed updates without polling!")
	log.Println()
	log.Println("Benefits of Observe over QUIC:")
	log.Println("  - No polling needed (battery efficient)")
	log.Println("  - Real-time updates as they happen")
	log.Println("  - Single long-lived connection")
	log.Println("  - QUIC ensures reliable, ordered delivery")
}

// connectQUIC establishes a QUIC connection
func connectQUIC(serverAddr string) (*quic.Conn, error) {
	// Configure TLS (skip verification for self-signed certs in demo)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"coap-quic-observe"},
	}

	// Configure QUIC with longer timeout for observations
	quicConfig := &quic.Config{
		MaxIdleTimeout:  120 * time.Second,
		KeepAlivePeriod: 30 * time.Second,
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

// registerObservation registers an observation and returns the stream and initial value
func registerObservation(conn *quic.Conn, path string) (*quic.Stream, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Open a new stream
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("open stream: %w", err)
	}

	// Create CoAP GET request with Observe option
	token := []byte{0x01, 0x02, 0x03, 0x04}
	request := common.NewMessage(message.Confirmable, common.GET, token)

	// IMPORTANT: Add Observe option FIRST (option 6 must come before URIPath option 11)
	// CoAP requires options in ascending order by option number
	common.AddObserveOption(request, 0)

	// Add path segments
	pathSegments := splitPath(path)
	for _, segment := range pathSegments {
		common.AddPathOption(request, segment)
	}

	// Encode request
	codec := coder.DefaultCoder
	requestData := make([]byte, 1024)
	requestLen, err := codec.Encode(*request, requestData)
	if err != nil {
		stream.Close()
		return nil, "", fmt.Errorf("encode request: %w", err)
	}

	// Send request (don't close stream - we need it for notifications)
	_, err = stream.Write(requestData[:requestLen])
	if err != nil {
		stream.Close()
		return nil, "", fmt.Errorf("write request: %w", err)
	}

	log.Printf("  -> Sent GET %s with Observe:0 (register)", path)

	// Read initial response
	responseData := make([]byte, 1024)
	n, err := stream.Read(responseData)
	if err != nil {
		stream.Close()
		return nil, "", fmt.Errorf("read initial response: %w", err)
	}

	// Decode initial response
	response := &message.Message{
		Options: make(message.Options, 0, 16),
	}
	_, err = codec.Decode(responseData[:n], response)
	if err != nil {
		stream.Close()
		return nil, "", fmt.Errorf("decode response: %w", err)
	}

	// Check response code
	if response.Code != common.Content {
		stream.Close()
		return nil, "", fmt.Errorf("unexpected response code: %s", common.CodeToString(response.Code))
	}

	// Check for Observe option in response
	observeSeq, hasObserve := common.GetObserveOption(response)
	if !hasObserve {
		stream.Close()
		return nil, "", fmt.Errorf("response missing Observe option")
	}

	log.Printf("  <- Initial response (seq=%d): %s", observeSeq, string(response.Payload))

	// Return stream (keep open) and initial payload
	return stream, string(response.Payload), nil
}

// receiveNotification waits for and receives an observe notification
func receiveNotification(stream *quic.Stream) (string, error) {
	// Read notification from stream
	responseData := make([]byte, 1024)
	n, err := stream.Read(responseData)
	if err != nil {
		return "", err
	}

	// Decode notification
	codec := coder.DefaultCoder
	response := &message.Message{
		Options: make(message.Options, 0, 16),
	}
	_, err = codec.Decode(responseData[:n], response)
	if err != nil {
		return "", fmt.Errorf("decode notification: %w", err)
	}

	// Check for Observe option
	observeSeq, hasObserve := common.GetObserveOption(response)
	if !hasObserve {
		return "", fmt.Errorf("notification missing Observe option")
	}

	log.Printf("  <- Notification (seq=%d): %s", observeSeq, string(response.Payload))

	return string(response.Payload), nil
}

// splitPath splits a path string into segments for CoAP Uri-Path options
func splitPath(path string) []string {
	segments := []string{}
	for _, segment := range strings.Split(path, "/") {
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	return segments
}
