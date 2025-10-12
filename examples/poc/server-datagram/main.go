package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"math/rand"
	"time"

	"github.com/golioth/coap-over-quic-poc/common"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/udp/coder"
	"github.com/quic-go/quic-go"
)

// Server state
type Server struct {
	ledState bool
	coder    *coder.Coder
}

func main() {
	log.Println("Starting CoAP over QUIC Server with Datagram Support (RFC 9221)...")
	log.Printf("Listening on port %d (coaps+quic://localhost:%d)", common.DefaultPort, common.DefaultPort)

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
		NextProtos:   []string{"coap-quic-poc"},
	}

	// Configure QUIC with Datagram support (RFC 9221)
	quicConfig := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
		EnableDatagrams: true, // Enable RFC 9221 QUIC Datagrams
	}

	// Create QUIC listener
	addr := fmt.Sprintf(":%d", common.DefaultPort)
	listener, err := quic.ListenAddr(addr, tlsConfig, quicConfig)
	if err != nil {
		log.Fatalf("Failed to create QUIC listener: %v", err)
	}
	defer listener.Close()

	log.Println("Server ready. Waiting for connections...")
	log.Println("Supports:")
	log.Println("  - Bidirectional streams (reliable request/response)")
	log.Println("  - QUIC Datagrams (unreliable NON messages, RFC 9221)")

	server := &Server{
		ledState: false,
		coder:    coder.DefaultCoder,
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

// handleConnection manages a QUIC connection, handling both streams and datagrams
func (s *Server) handleConnection(conn *quic.Conn) {
	defer func() {
		log.Printf("Connection closed from %s", conn.RemoteAddr())
		conn.CloseWithError(0, "")
	}()

	// Handle streams in one goroutine
	go s.handleStreams(conn)

	// Handle datagrams in another goroutine
	s.handleDatagrams(conn)
}

// handleStreams accepts and processes bidirectional streams
func (s *Server) handleStreams(conn *quic.Conn) {
	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			log.Printf("Stream handler terminated: %v", err)
			return
		}

		log.Printf("New bidirectional stream %d opened", stream.StreamID())

		// Handle each stream in a goroutine (enables concurrent requests)
		go s.handleStream(stream)
	}
}

// handleStream processes a single QUIC stream containing a CoAP request/response
func (s *Server) handleStream(stream *quic.Stream) {
	// Read the CoAP request from the stream
	// Per spec: client sends request and sets FIN bit
	requestData, err := io.ReadAll(stream)
	if err != nil {
		log.Printf("Failed to read from stream: %v", err)
		stream.Close()
		return
	}

	log.Printf("[STREAM %d] Received %d bytes", stream.StreamID(), len(requestData))

	// Parse CoAP message using go-coap UDP coder (same format for QUIC)
	request := message.Message{
		Options: make(message.Options, 0, 16), // Pre-allocate options buffer
	}
	_, err2 := s.coder.Decode(requestData, &request)
	if err2 != nil {
		log.Printf("Failed to parse CoAP message: %v", err2)
		// Per spec: send RESET_STREAM on malformed messages
		stream.CancelRead(1) // Application error code
		stream.Close()
		return
	}

	log.Printf("[STREAM %d] CoAP Request: Type=%v, Code=%s, Token=%x",
		stream.StreamID(), request.Type, common.CodeToString(request.Code), request.Token)

	// Route the request and generate response
	response := s.routeRequest(&request)

	// Marshal the response using go-coap UDP coder
	responseData := make([]byte, 2048)
	responseLen, err := s.coder.Encode(*response, responseData)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		stream.Close()
		return
	}
	responseData = responseData[:responseLen]

	// Send response on the same stream
	// Per spec: server sets FIN bit to indicate end of response
	_, err = stream.Write(responseData)
	if err != nil {
		log.Printf("Failed to write response: %v", err)
		stream.Close()
		return
	}

	log.Printf("[STREAM %d] Sent CoAP Response: Code=%s, %d bytes",
		stream.StreamID(), common.CodeToString(response.Code), len(responseData))

	// Close stream after sending response
	stream.Close()
}

// handleDatagrams processes QUIC datagrams (RFC 9221)
// Used for NON (non-confirmable) CoAP messages
func (s *Server) handleDatagrams(conn *quic.Conn) {
	for {
		// Receive datagram (blocks until datagram arrives or connection closes)
		datagramData, err := conn.ReceiveDatagram(context.Background())
		if err != nil {
			log.Printf("Datagram handler terminated: %v", err)
			return
		}

		log.Printf("[DATAGRAM] Received %d bytes", len(datagramData))

		// Process datagram in goroutine to avoid blocking
		go s.handleDatagram(conn, datagramData)
	}
}

// handleDatagram processes a single QUIC datagram containing a CoAP NON message
func (s *Server) handleDatagram(conn *quic.Conn, datagramData []byte) {
	// Parse CoAP message
	request := message.Message{
		Options: make(message.Options, 0, 16),
	}
	_, err := s.coder.Decode(datagramData, &request)
	if err != nil {
		log.Printf("[DATAGRAM] Failed to parse CoAP message: %v", err)
		return
	}

	log.Printf("[DATAGRAM] CoAP NON: Type=%v, Code=%s, Token=%x",
		request.Type, common.CodeToString(request.Code), request.Token)

	// Extract path
	path := common.GetPath(&request)
	log.Printf("[DATAGRAM] Request path: %s", path)

	// For NON messages, typically no response is sent
	// But we can optionally send a response datagram for certain endpoints
	// For now, just log and update state if applicable

	switch path {
	case "/telemetry":
		log.Printf("[DATAGRAM] Telemetry data received (fire-and-forget)")
		// No response for telemetry

	case "/log":
		if len(request.Payload) > 0 {
			log.Printf("[DATAGRAM] Log message: %s", string(request.Payload))
		}
		// No response for logs

	default:
		log.Printf("[DATAGRAM] NON message to: %s (no response)", path)
	}
}

// routeRequest handles CoAP request routing and returns a response
func (s *Server) routeRequest(req *message.Message) *message.Message {
	// Extract URI path using go-coap helper
	path := common.GetPath(req)
	log.Printf("Request path: %s", path)

	// Create response with same token (RFC 7252 requirement)
	// Type is implicitly ACK due to QUIC reliability
	response := common.NewMessage(common.Acknowledgement, common.Content, req.Token)

	switch path {
	case "/.well-known/core":
		// Resource discovery (Section 7 of spec)
		if req.Code == common.GET {
			response.Code = common.Content
			payload := []byte("</temp>;rt=\"temperature\",</led>;rt=\"actuator\",</telemetry>;rt=\"telemetry\",</log>;rt=\"log\"")
			common.SetLinkFormatPayload(response, payload)
			log.Println("Serving resource discovery")
		} else {
			response.Code = common.MethodNotAllowed
		}

	case "/temp":
		// GET /temp - return simulated temperature
		if req.Code == common.GET {
			temp := 20.0 + rand.Float64()*10.0 // Random temp between 20-30°C
			response.Code = common.Content
			payload := []byte(fmt.Sprintf(`{"value":%.2f,"unit":"celsius"}`, temp))
			common.SetJSONPayload(response, payload)
			log.Printf("Serving temperature: %.2f°C", temp)
		} else {
			response.Code = common.MethodNotAllowed
		}

	case "/led":
		// PUT /led - toggle LED state
		if req.Code == common.PUT {
			s.ledState = !s.ledState
			response.Code = common.Changed
			payload := []byte(fmt.Sprintf(`{"state":%t}`, s.ledState))
			common.SetJSONPayload(response, payload)
			log.Printf("LED state changed to: %t", s.ledState)
		} else if req.Code == common.GET {
			response.Code = common.Content
			payload := []byte(fmt.Sprintf(`{"state":%t}`, s.ledState))
			common.SetJSONPayload(response, payload)
			log.Printf("LED state requested: %t", s.ledState)
		} else {
			response.Code = common.MethodNotAllowed
		}

	default:
		response.Code = common.NotFound
		response.Payload = []byte("Resource not found")
		log.Printf("Resource not found: %s", path)
	}

	return response
}
