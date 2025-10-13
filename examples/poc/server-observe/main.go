package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/golioth/coap-over-quic-poc/common"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/udp/coder"
	"github.com/quic-go/quic-go"
)

// Observer represents a client observing a resource
type Observer struct {
	stream         *quic.Stream
	token          []byte
	sequenceNumber uint32
}

// ObservableResource represents a resource that can be observed
type ObservableResource struct {
	path      string
	value     interface{}
	observers map[string]*Observer // key: token as string
	mu        sync.RWMutex
}

// Server implements a CoAP over QUIC server with Observe support (RFC 7641)
type Server struct {
	coder     *coder.Coder
	resources map[string]*ObservableResource
	mu        sync.RWMutex
}

func main() {
	log.Println("CoAP over QUIC Server with Observe Support (RFC 7641)")
	log.Println("=====================================================")
	log.Printf("Listening on port %d (coaps+quic://localhost:%d)", common.DefaultPort, common.DefaultPort)
	log.Println()
	log.Println("Supports:")
	log.Println("  - QUIC bidirectional streams")
	log.Println("  - Observe pattern (RFC 7641) for pub/sub")
	log.Println("  - Long-lived observations over QUIC")
	log.Println("  - Automatic notifications on resource changes")
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
		NextProtos:   []string{"coap-quic-observe"},
	}

	// Configure QUIC
	quicConfig := &quic.Config{
		MaxIdleTimeout:  120 * time.Second, // Longer timeout for observations
		KeepAlivePeriod: 30 * time.Second,
	}

	// Create QUIC listener
	addr := fmt.Sprintf(":%d", common.DefaultPort)
	listener, err := quic.ListenAddr(addr, tlsConfig, quicConfig)
	if err != nil {
		log.Fatalf("Failed to create QUIC listener: %v", err)
	}
	defer listener.Close()

	// Initialize server
	server := &Server{
		coder:     coder.DefaultCoder,
		resources: make(map[string]*ObservableResource),
	}

	// Create observable temperature resource
	tempResource := &ObservableResource{
		path:      "/temp",
		value:     20.0 + rand.Float64()*10.0, // Initial temp between 20-30°C
		observers: make(map[string]*Observer),
	}
	server.resources["/temp"] = tempResource

	// Start background temperature simulator
	go server.simulateTemperatureChanges(tempResource)

	log.Println("Server ready. Waiting for requests...")
	log.Println()

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

// simulateTemperatureChanges updates the temperature resource periodically
func (s *Server) simulateTemperatureChanges(resource *ObservableResource) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		resource.mu.Lock()

		// Simulate temperature change (±2°C with some drift)
		currentTemp := resource.value.(float64)
		change := (rand.Float64() - 0.5) * 4.0 // -2 to +2°C
		newTemp := currentTemp + change

		// Keep within reasonable bounds (15-35°C)
		if newTemp < 15.0 {
			newTemp = 15.0
		} else if newTemp > 35.0 {
			newTemp = 35.0
		}

		resource.value = newTemp
		log.Printf("[TEMP UPDATE] Temperature changed: %.1f°C -> %.1f°C", currentTemp, newTemp)

		// Notify all observers
		observerCount := len(resource.observers)
		if observerCount > 0 {
			log.Printf("[NOTIFY] Sending notifications to %d observer(s)", observerCount)
			for tokenStr, observer := range resource.observers {
				go s.sendNotification(observer, resource, tokenStr)
			}
		}

		resource.mu.Unlock()
	}
}

// sendNotification sends an observe notification to a client
func (s *Server) sendNotification(observer *Observer, resource *ObservableResource, tokenStr string) {
	observer.sequenceNumber++

	// Create notification response
	response := common.NewMessage(message.Acknowledgement, common.Content, observer.token)

	// Add Observe option with sequence number
	common.AddObserveOption(response, observer.sequenceNumber)

	// Add temperature payload
	temp := resource.value.(float64)
	payload := []byte(fmt.Sprintf(`{"value":%.1f,"unit":"celsius"}`, temp))
	response.Options = append(response.Options, message.Option{
		ID:    message.ContentFormat,
		Value: []byte{byte(message.AppJSON)},
	})
	response.Payload = payload

	// Encode response
	responseData := make([]byte, 1024)
	responseLen, err := s.coder.Encode(*response, responseData)
	if err != nil {
		log.Printf("[NOTIFY] ERROR encoding notification: %v", err)
		return
	}

	// Send notification on the stream
	_, err = observer.stream.Write(responseData[:responseLen])
	if err != nil {
		log.Printf("[NOTIFY] ERROR sending to observer (token=%s): %v", tokenStr, err)
		// Remove dead observer
		resource.mu.Lock()
		delete(resource.observers, tokenStr)
		resource.mu.Unlock()
		return
	}

	log.Printf("[NOTIFY] Sent notification #%d to observer (token=%s): %.1f°C",
		observer.sequenceNumber, tokenStr, temp)
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
	// Read CoAP request from stream
	// For Observe, the stream stays open, so we read one message at a time
	requestData := make([]byte, 2048)
	n, err := stream.Read(requestData)
	if err != nil {
		if err != io.EOF {
			log.Printf("ERROR reading stream: %v", err)
		}
		stream.Close()
		return
	}

	log.Printf("[STREAM %d] Received %d bytes", stream.StreamID(), n)

	// Decode CoAP message with pre-allocated options buffer
	request := &message.Message{
		Options: make(message.Options, 0, 16),
	}
	_, err = s.coder.Decode(requestData[:n], request)
	if err != nil {
		log.Printf("ERROR decoding message: %v", err)
		stream.Close()
		return
	}

	// Get path
	path := common.GetPath(request)
	log.Printf("[STREAM %d] %s %s", stream.StreamID(), common.MethodToString(request.Code), path)

	// Debug: Log all options
	log.Printf("[DEBUG] Request has %d options", len(request.Options))
	for i, opt := range request.Options {
		log.Printf("[DEBUG]   Option %d: ID=%d, Value=%v", i, opt.ID, opt.Value)
	}

	// Check for Observe option
	observeValue, hasObserve := common.GetObserveOption(request)
	log.Printf("[DEBUG] Observe option check: hasObserve=%v, value=%d", hasObserve, observeValue)

	if hasObserve {
		if observeValue == 0 {
			// Register observation
			log.Printf("[OBSERVE] Client wants to observe %s (token=%x)", path, request.Token)
			s.handleObserveRegistration(stream, request, path)
			// Don't close stream - keep it open for notifications
			return
		} else {
			// Deregister observation
			log.Printf("[OBSERVE] Client deregistering from %s (token=%x)", path, request.Token)
			s.handleObserveDeregistration(request, path)
		}
	}

	// Handle regular (non-observe) request
	response := s.routeRequest(request, path)

	// Encode and send response
	responseData := make([]byte, 1024)
	responseLen, err := s.coder.Encode(*response, responseData)
	if err != nil {
		log.Printf("ERROR encoding response: %v", err)
		stream.Close()
		return
	}

	_, err = stream.Write(responseData[:responseLen])
	if err != nil {
		log.Printf("ERROR writing response: %v", err)
	}

	log.Printf("  -> Response sent (%d bytes)", responseLen)
	stream.Close()
}

// handleObserveRegistration registers a client to observe a resource
func (s *Server) handleObserveRegistration(stream *quic.Stream, request *message.Message, path string) {
	// Check if resource exists and is observable
	s.mu.RLock()
	resource, exists := s.resources[path]
	s.mu.RUnlock()

	if !exists {
		// Resource not found or not observable
		response := common.NewMessage(message.Acknowledgement, common.NotFound, request.Token)
		responseData := make([]byte, 256)
		responseLen, _ := s.coder.Encode(*response, responseData)
		stream.Write(responseData[:responseLen])
		stream.Close()
		return
	}

	// Send initial response with current value
	response := common.NewMessage(message.Acknowledgement, common.Content, request.Token)
	common.AddObserveOption(response, 0) // Sequence number 0 for initial response

	temp := resource.value.(float64)
	payload := []byte(fmt.Sprintf(`{"value":%.1f,"unit":"celsius"}`, temp))
	response.Options = append(response.Options, message.Option{
		ID:    message.ContentFormat,
		Value: []byte{byte(message.AppJSON)},
	})
	response.Payload = payload

	responseData := make([]byte, 1024)
	responseLen, err := s.coder.Encode(*response, responseData)
	if err != nil {
		log.Printf("ERROR encoding initial response: %v", err)
		stream.Close()
		return
	}

	_, err = stream.Write(responseData[:responseLen])
	if err != nil {
		log.Printf("ERROR writing initial response: %v", err)
		stream.Close()
		return
	}

	log.Printf("[OBSERVE] Initial response sent: %.1f°C", temp)

	// Register observer
	resource.mu.Lock()
	tokenStr := string(request.Token)
	resource.observers[tokenStr] = &Observer{
		stream:         stream,
		token:          request.Token,
		sequenceNumber: 0,
	}
	resource.mu.Unlock()

	log.Printf("[OBSERVE] Registered observer for %s (token=%x), total=%d",
		path, request.Token, len(resource.observers))
}

// handleObserveDeregistration removes a client from observing a resource
func (s *Server) handleObserveDeregistration(request *message.Message, path string) {
	s.mu.RLock()
	resource, exists := s.resources[path]
	s.mu.RUnlock()

	if !exists {
		return
	}

	resource.mu.Lock()
	tokenStr := string(request.Token)
	delete(resource.observers, tokenStr)
	count := len(resource.observers)
	resource.mu.Unlock()

	log.Printf("[OBSERVE] Deregistered observer for %s (token=%x), remaining=%d",
		path, request.Token, count)
}

// routeRequest handles non-observe requests
func (s *Server) routeRequest(request *message.Message, path string) *message.Message {
	switch path {
	case "/.well-known/core":
		return s.handleResourceDiscovery(request)
	case "/temp":
		return s.handleTempGet(request)
	default:
		return common.NewMessage(message.Acknowledgement, common.NotFound, request.Token)
	}
}

// handleResourceDiscovery serves /.well-known/core
func (s *Server) handleResourceDiscovery(request *message.Message) *message.Message {
	if request.Code != common.GET {
		return common.NewMessage(message.Acknowledgement, common.MethodNotAllowed, request.Token)
	}

	payload := []byte(`</temp>;rt="temperature";obs`)
	response := common.NewMessage(message.Acknowledgement, common.Content, request.Token)
	response.Options = append(response.Options, message.Option{
		ID:    message.ContentFormat,
		Value: []byte{byte(message.AppLinkFormat)},
	})
	response.Payload = payload

	return response
}

// handleTempGet serves /temp (non-observe GET)
func (s *Server) handleTempGet(request *message.Message) *message.Message {
	if request.Code != common.GET {
		return common.NewMessage(message.Acknowledgement, common.MethodNotAllowed, request.Token)
	}

	s.mu.RLock()
	resource := s.resources["/temp"]
	s.mu.RUnlock()

	resource.mu.RLock()
	temp := resource.value.(float64)
	resource.mu.RUnlock()

	payload := []byte(fmt.Sprintf(`{"value":%.1f,"unit":"celsius"}`, temp))
	response := common.NewMessage(message.Acknowledgement, common.Content, request.Token)
	response.Options = append(response.Options, message.Option{
		ID:    message.ContentFormat,
		Value: []byte{byte(message.AppJSON)},
	})
	response.Payload = payload

	return response
}
