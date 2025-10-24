package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"log"
	"math/rand"
	"sync"

	piondtls "github.com/pion/dtls/v3"
	"github.com/golioth/coap-over-quic-poc/common"
	"github.com/plgd-dev/go-coap/v3/dtls"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/mux"
	"github.com/plgd-dev/go-coap/v3/net"
	"github.com/plgd-dev/go-coap/v3/options"
)

// Server state
type Server struct {
	ledState bool
}

// InMemorySessionStore implements pion/dtls SessionStore interface for session caching
// Enables DTLS session resumption (abbreviated handshake) for faster reconnections
type InMemorySessionStore struct {
	sessions map[string]*piondtls.Session
	mutex    sync.RWMutex
}

func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		sessions: make(map[string]*piondtls.Session),
	}
}

func (s *InMemorySessionStore) Set(key []byte, session piondtls.Session) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	keyStr := string(key)
	s.sessions[keyStr] = &session
	log.Printf("  [Session Store] Cached session for key: %x (total: %d sessions)", key[:min(8, len(key))], len(s.sessions))

	return nil
}

func (s *InMemorySessionStore) Get(key []byte) (piondtls.Session, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	keyStr := string(key)
	if session, exists := s.sessions[keyStr]; exists {
		log.Printf("  [Session Store] Retrieved cached session for key: %x (abbreviated handshake)", key[:min(8, len(key))])
		return *session, nil
	}

	return piondtls.Session{}, fmt.Errorf("session not found")
}

func (s *InMemorySessionStore) Del(key []byte) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	keyStr := string(key)
	delete(s.sessions, keyStr)
	log.Printf("  [Session Store] Deleted session for key: %x", key[:min(8, len(key))])

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	log.Println("CoAP over UDP with DTLS Server (RFC 7252 + RFC 6347)")
	log.Println("======================================================")
	log.Printf("Listening on port %d (coaps://localhost:%d)", common.DefaultPort, common.DefaultPort)
	log.Println()

	server := &Server{
		ledState: false,
	}

	// Create CoAP router
	router := mux.NewRouter()
	router.Use(loggingMiddleware)

	// Register resource handlers
	router.Handle("/.well-known/core", mux.HandlerFunc(server.handleResourceDiscovery))
	router.Handle("/temp", mux.HandlerFunc(server.handleTemperature))
	router.Handle("/led", mux.HandlerFunc(server.handleLED))
	router.Handle("/telemetry", mux.HandlerFunc(server.handleTelemetry))
	router.Handle("/log", mux.HandlerFunc(server.handleLog))

	log.Println("Server ready. Waiting for requests...")
	log.Println("Supports:")
	log.Println("  - DTLS 1.2 encryption (RFC 6347)")
	log.Println("  - TLS certificate-based authentication")
	log.Println("  - Session resumption (abbreviated handshake) for faster reconnection")
	log.Println("  - Connection ID (RFC 9146) for connection migration support")
	log.Println("  - CON (confirmable) messages with ACK")
	log.Println("  - NON (non-confirmable) messages")
	log.Println("  - Retransmission and deduplication")
	log.Println()

	// Load TLS certificate
	certificate, err := tls.LoadX509KeyPair(
		"../certs/server.crt",
		"../certs/server.key",
	)
	if err != nil {
		log.Fatalf("Failed to load certificates: %v", err)
	}

	// Create session store for session resumption (abbreviated handshake)
	sessionStore := NewInMemorySessionStore()

	// Create DTLS config with session resumption and Connection ID (RFC 9146) support
	dtlsConfig := &piondtls.Config{
		Certificates:         []tls.Certificate{certificate},
		ExtendedMasterSecret: piondtls.RequireExtendedMasterSecret,
		// Don't require client certificates for this demo
		ClientAuth: piondtls.NoClientCert,
		// Enable session caching for faster reconnection (abbreviated handshake)
		SessionStore: sessionStore,
		// Enable Connection ID (RFC 9146) for connection migration support
		// Allows connections to survive IP/port changes (e.g., NAT rebinding, network handoff)
		ConnectionIDGenerator: piondtls.RandomCIDGenerator(8), // 8-byte Connection ID
	}

	// Start DTLS server
	addr := fmt.Sprintf(":%d", common.DefaultPort)
	if err := listenAndServeDTLS("udp", addr, dtlsConfig, router); err != nil {
		log.Fatal(err)
	}
}

// listenAndServeDTLS starts a DTLS server with the given configuration
func listenAndServeDTLS(network string, addr string, config *piondtls.Config, handler mux.Handler) error {
	listener, err := net.NewDTLSListener(network, addr, config)
	if err != nil {
		return fmt.Errorf("failed to create DTLS listener: %w", err)
	}
	defer listener.Close()

	server := dtls.NewServer(options.WithMux(handler))
	return server.Serve(listener)
}

// loggingMiddleware logs all incoming requests
func loggingMiddleware(next mux.Handler) mux.Handler {
	return mux.HandlerFunc(func(w mux.ResponseWriter, r *mux.Message) {
		path, _ := r.Options().Path()
		log.Printf("[%s] %s /%s from %v",
			r.Type().String(),
			common.CodeToString(r.Code()),
			path,
			w.Conn().RemoteAddr())
		next.ServeCOAP(w, r)
	})
}

// handleResourceDiscovery serves /.well-known/core (resource discovery)
func (s *Server) handleResourceDiscovery(w mux.ResponseWriter, r *mux.Message) {
	if r.Code() != common.GET {
		w.SetResponse(common.MethodNotAllowed, message.TextPlain, nil)
		return
	}

	payload := []byte(`</temp>;rt="temperature",</led>;rt="actuator",</telemetry>;rt="telemetry",</log>;rt="log"`)
	err := w.SetResponse(common.Content, message.AppLinkFormat, bytes.NewReader(payload))
	if err != nil {
		log.Printf("  ERROR: cannot set response: %v", err)
	}
	log.Println("  -> Serving resource discovery")
}

// handleTemperature serves /temp (simulated temperature readings)
func (s *Server) handleTemperature(w mux.ResponseWriter, r *mux.Message) {
	if r.Code() != common.GET {
		w.SetResponse(common.MethodNotAllowed, message.TextPlain, nil)
		return
	}

	temp := 20.0 + rand.Float64()*10.0 // Random temp between 20-30°C
	payload := []byte(fmt.Sprintf(`{"value":%.2f,"unit":"celsius"}`, temp))
	err := w.SetResponse(common.Content, message.AppJSON, bytes.NewReader(payload))
	if err != nil {
		log.Printf("  ERROR: cannot set response: %v", err)
	}
	log.Printf("  -> Serving temperature: %.2f°C", temp)
}

// handleLED serves /led (GET for state query, PUT to toggle)
func (s *Server) handleLED(w mux.ResponseWriter, r *mux.Message) {
	switch r.Code() {
	case common.GET:
		payload := []byte(fmt.Sprintf(`{"state":%t}`, s.ledState))
		err := w.SetResponse(common.Content, message.AppJSON, bytes.NewReader(payload))
		if err != nil {
			log.Printf("  ERROR: cannot set response: %v", err)
		}
		log.Printf("  -> LED state requested: %t", s.ledState)

	case common.PUT:
		s.ledState = !s.ledState
		payload := []byte(fmt.Sprintf(`{"state":%t}`, s.ledState))
		err := w.SetResponse(common.Changed, message.AppJSON, bytes.NewReader(payload))
		if err != nil {
			log.Printf("  ERROR: cannot set response: %v", err)
		}
		log.Printf("  -> LED state changed to: %t", s.ledState)

	default:
		w.SetResponse(common.MethodNotAllowed, message.TextPlain, nil)
	}
}

// handleTelemetry serves /telemetry (accepts POST for telemetry data)
func (s *Server) handleTelemetry(w mux.ResponseWriter, r *mux.Message) {
	if r.Code() != common.POST {
		w.SetResponse(common.MethodNotAllowed, message.TextPlain, nil)
		return
	}

	// Read body
	bodyReader := r.Body()
	if bodyReader != nil {
		buf := new(bytes.Buffer)
		buf.ReadFrom(bodyReader)
		log.Printf("  -> Telemetry data received (%d bytes)", buf.Len())
	}

	// For NON messages, typically no response is sent
	// For CON messages, send ACK with Created status
	if r.Type() == message.Confirmable {
		w.SetResponse(common.Created, message.TextPlain, nil)
	}
}

// handleLog serves /log (accepts POST for log messages)
func (s *Server) handleLog(w mux.ResponseWriter, r *mux.Message) {
	if r.Code() != common.POST {
		w.SetResponse(common.MethodNotAllowed, message.TextPlain, nil)
		return
	}

	// Read body
	bodyReader := r.Body()
	if bodyReader != nil {
		buf := new(bytes.Buffer)
		buf.ReadFrom(bodyReader)
		if buf.Len() > 0 {
			log.Printf("  -> Log message: %s", buf.String())
		}
	}

	// For NON messages, typically no response is sent
	// For CON messages, send ACK with Created status
	if r.Type() == message.Confirmable {
		w.SetResponse(common.Created, message.TextPlain, nil)
	}
}
