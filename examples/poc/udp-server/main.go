package main

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"

	coap "github.com/plgd-dev/go-coap/v3"
	"github.com/golioth/coap-over-quic-poc/common"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/mux"
)

// Server state
type Server struct {
	ledState bool
}

func main() {
	log.Println("Starting CoAP over UDP Server (RFC 7252)...")
	log.Printf("Listening on port %d (coap://localhost:%d)", common.DefaultPort, common.DefaultPort)

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
	log.Println("  - CON (confirmable) messages with ACK")
	log.Println("  - NON (non-confirmable) messages")
	log.Println("  - Retransmission and deduplication")

	// Start UDP server
	addr := fmt.Sprintf(":%d", common.DefaultPort)
	log.Fatal(coap.ListenAndServe("udp", addr, router))
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
