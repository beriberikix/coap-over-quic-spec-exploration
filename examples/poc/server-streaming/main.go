package main

import (
	"context"
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

// Server implements a CoAP over QUIC server with native streaming
type Server struct {
	coder *coder.Coder
}

func main() {
	log.Println("CoAP over QUIC Server with Native Streaming")
	log.Println("===========================================")
	log.Printf("Listening on port %d (coaps+quic://localhost:%d)", common.DefaultPort, common.DefaultPort)
	log.Println()
	log.Println("Supports:")
	log.Println("  - QUIC native streaming (no block-wise needed)")
	log.Println("  - Large resource: /firmware (50KB simulated firmware)")
	log.Println("  - Full reliability and ordering from QUIC")
	log.Println()

	// Load TLS certificate
	cert, err := tls.LoadX509KeyPair("../certs/server.crt", "../certs/server.key")
	if err != nil {
		log.Fatalf("Failed to load TLS certificate: %v", err)
	}

	// Configure TLS with TLS 1.3 (required by QUIC)
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		NextProtos:   []string{"coap-quic-streaming"},
	}

	// Configure QUIC
	quicConfig := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
	}

	// Create QUIC listener
	addr := fmt.Sprintf(":%d", common.DefaultPort)
	listener, err := quic.ListenAddr(addr, tlsConfig, quicConfig)
	if err != nil {
		log.Fatalf("Failed to create QUIC listener: %v", err)
	}
	defer listener.Close()

	log.Println("Server ready. Waiting for requests...")
	log.Println()

	server := &Server{
		coder: coder.DefaultCoder,
	}

	// Accept connections
	for {
		conn, err := listener.Accept(context.Background())
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		log.Printf("New connection from %s", conn.RemoteAddr())

		// Handle connection in a goroutine
		go server.handleConnection(conn)
	}
}

// handleConnection handles incoming QUIC streams
func (s *Server) handleConnection(conn *quic.Conn) {
	defer conn.CloseWithError(0, "connection closed")

	for {
		// Accept a new stream
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			log.Printf("Connection closed: %v", err)
			return
		}

		// Handle stream in a goroutine
		go s.handleStream(stream)
	}
}

// handleStream handles a single QUIC stream
func (s *Server) handleStream(stream *quic.Stream) {
	defer stream.Close()

	// Read entire CoAP request from stream
	requestData, err := io.ReadAll(stream)
	if err != nil {
		log.Printf("ERROR reading stream: %v", err)
		return
	}

	log.Printf("[STREAM %d] Received %d bytes", stream.StreamID(), len(requestData))

	// Decode CoAP message with pre-allocated options buffer
	request := &message.Message{
		Options: make(message.Options, 0, 16), // Pre-allocate to avoid "too small options buffer"
	}
	_, err = s.coder.Decode(requestData, request)
	if err != nil {
		log.Printf("ERROR decoding message: %v", err)
		return
	}

	// Get path
	path := common.GetPath(request)
	log.Printf("[Stream] %s %s", common.MethodToString(request.Code), path)

	// Route request
	var response *message.Message
	switch path {
	case "/.well-known/core":
		response = s.handleResourceDiscovery(request)
	case "/firmware":
		response = s.handleFirmware(request)
	case "/temp":
		response = s.handleTemp(request)
	default:
		response = s.createResponse(request, common.NotFound, message.TextPlain, []byte("Not Found"))
	}

	// Encode and send response
	responseData := make([]byte, 65536) // Large buffer for firmware
	responseLen, err := s.coder.Encode(*response, responseData)
	if err != nil {
		log.Printf("ERROR encoding response: %v", err)
		return
	}

	_, err = stream.Write(responseData[:responseLen])
	if err != nil {
		log.Printf("ERROR writing response: %v", err)
		return
	}

	log.Printf("  -> Response sent (%d bytes over QUIC stream)", responseLen)
}

// handleResourceDiscovery serves /.well-known/core
func (s *Server) handleResourceDiscovery(request *message.Message) *message.Message {
	if request.Code != common.GET {
		return s.createResponse(request, common.MethodNotAllowed, message.TextPlain, nil)
	}

	payload := []byte(`</firmware>;rt="firmware";sz="51200",</temp>;rt="temperature"`)
	return s.createResponse(request, common.Content, message.AppLinkFormat, payload)
}

// handleFirmware serves /firmware - a large resource streamed over QUIC
func (s *Server) handleFirmware(request *message.Message) *message.Message {
	if request.Code != common.GET {
		return s.createResponse(request, common.MethodNotAllowed, message.TextPlain, nil)
	}

	// Generate 50KB simulated firmware
	firmwareSize := 50 * 1024 // 50KB
	firmware := generateFirmware(firmwareSize)

	log.Printf("  -> Serving firmware (%d bytes)", len(firmware))
	log.Println("     QUIC streams handle large payloads natively")
	log.Println("     No block-wise transfer needed!")

	return s.createResponse(request, common.Content, common.AppOctets, firmware)
}

// handleTemp serves /temp - small resource for comparison
func (s *Server) handleTemp(request *message.Message) *message.Message {
	if request.Code != common.GET {
		return s.createResponse(request, common.MethodNotAllowed, message.TextPlain, nil)
	}

	payload := []byte(`{"value":23.5,"unit":"celsius"}`)
	return s.createResponse(request, common.Content, message.AppJSON, payload)
}

// createResponse creates a CoAP response message
func (s *Server) createResponse(request *message.Message, code common.Code, contentFormat message.MediaType, payload []byte) *message.Message {
	response := common.NewMessage(message.Acknowledgement, code, request.Token)

	if len(payload) > 0 {
		// Add content format option
		cfValue := uint32(contentFormat)
		response.Options = append(response.Options, message.Option{
			ID:    message.ContentFormat,
			Value: encodeUint32(cfValue),
		})
		response.Payload = payload
	}

	return response
}

// encodeUint32 encodes a uint32 value for CoAP options
func encodeUint32(value uint32) []byte {
	if value == 0 {
		return []byte{}
	}
	buf := make([]byte, 4)
	i := 0
	for value > 0 {
		buf[i] = byte(value)
		value >>= 8
		i++
	}
	// Reverse for big-endian
	for j := 0; j < i/2; j++ {
		buf[j], buf[i-1-j] = buf[i-1-j], buf[j]
	}
	return buf[i-1:]
}

// generateFirmware creates simulated firmware data
func generateFirmware(size int) []byte {
	// Create realistic-looking firmware with headers and data sections
	firmware := make([]byte, size)

	// Header section (first 256 bytes)
	header := []byte("FIRMWARE v2.1.5 - CoAP/QUIC Demo - ")
	copy(firmware, header)
	for i := len(header); i < 256; i++ {
		firmware[i] = byte(i % 256)
	}

	// Data section (repeating pattern)
	pattern := []byte("0123456789ABCDEF")
	for i := 256; i < size; i++ {
		firmware[i] = pattern[i%len(pattern)]
	}

	// Footer (last 64 bytes)
	footer := []byte(" END_OF_FIRMWARE ")
	copy(firmware[size-len(footer):], footer)

	return firmware
}
