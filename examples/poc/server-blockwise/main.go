package main

import (
	"bytes"
	"fmt"
	"log"

	coap "github.com/plgd-dev/go-coap/v3"
	"github.com/golioth/coap-over-quic-poc/common"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/mux"
)

func main() {
	log.Println("CoAP over UDP Server with Block-wise Transfer Support (RFC 7959)")
	log.Println("==============================================================")
	log.Printf("Listening on port %d (coap://localhost:%d)", common.DefaultPort, common.DefaultPort)
	log.Println()
	log.Println("Supports:")
	log.Println("  - Block-wise transfers (RFC 7959) - automatic")
	log.Println("  - Large resource: /firmware (50KB simulated firmware)")
	log.Println("  - Negotiable block sizes (16 to 1024 bytes)")
	log.Println()

	// Create CoAP router
	router := mux.NewRouter()
	router.Use(loggingMiddleware)

	// Register resource handlers
	router.Handle("/.well-known/core", mux.HandlerFunc(handleResourceDiscovery))
	router.Handle("/firmware", mux.HandlerFunc(handleFirmware))
	router.Handle("/temp", mux.HandlerFunc(handleTemp))

	log.Println("Server ready. Block-wise transfers enabled by default.")
	log.Println()

	// Start UDP server (block-wise is enabled by default for UDP)
	addr := fmt.Sprintf(":%d", common.DefaultPort)
	log.Fatal(coap.ListenAndServe("udp", addr, router))
}

// loggingMiddleware logs all incoming requests
func loggingMiddleware(next mux.Handler) mux.Handler {
	return mux.HandlerFunc(func(w mux.ResponseWriter, r *mux.Message) {
		path, _ := r.Options().Path()

		// Check for Block2 option to detect block-wise request
		blockOpt, err := r.Options().GetUint32(message.Block2)
		if err == nil {
			blockNum := blockOpt >> 4
			more := (blockOpt & 0x08) != 0
			szx := blockOpt & 0x07
			blockSize := 16 << szx
			log.Printf("[%s] %s /%s (Block2: num=%d, more=%t, size=%d bytes)",
				r.Type().String(),
				common.CodeToString(r.Code()),
				path,
				blockNum,
				more,
				blockSize)
		} else {
			log.Printf("[%s] %s /%s from %v",
				r.Type().String(),
				common.CodeToString(r.Code()),
				path,
				w.Conn().RemoteAddr())
		}

		next.ServeCOAP(w, r)
	})
}

// handleResourceDiscovery serves /.well-known/core
func handleResourceDiscovery(w mux.ResponseWriter, r *mux.Message) {
	if r.Code() != common.GET {
		w.SetResponse(common.MethodNotAllowed, message.TextPlain, nil)
		return
	}

	payload := []byte(`</firmware>;rt="firmware";sz="51200",</temp>;rt="temperature"`)
	err := w.SetResponse(common.Content, message.AppLinkFormat, bytes.NewReader(payload))
	if err != nil {
		log.Printf("  ERROR: cannot set response: %v", err)
	}
	log.Println("  -> Serving resource discovery")
}

// handleFirmware serves /firmware - a large resource for block-wise demo
func handleFirmware(w mux.ResponseWriter, r *mux.Message) {
	if r.Code() != common.GET {
		w.SetResponse(common.MethodNotAllowed, message.TextPlain, nil)
		return
	}

	// Generate 50KB simulated firmware
	firmwareSize := 50 * 1024 // 50KB
	firmware := generateFirmware(firmwareSize)

	log.Printf("  -> Serving firmware (%d bytes)", len(firmware))
	log.Println("     go-coap will automatically handle block-wise transfer")

	// go-coap automatically handles block-wise transfers
	// Just set the full payload and the library fragments it
	err := w.SetResponse(common.Content, common.AppOctets, bytes.NewReader(firmware))
	if err != nil {
		log.Printf("  ERROR: cannot set response: %v", err)
	}
}

// handleTemp serves /temp - small resource for comparison
func handleTemp(w mux.ResponseWriter, r *mux.Message) {
	if r.Code() != common.GET {
		w.SetResponse(common.MethodNotAllowed, message.TextPlain, nil)
		return
	}

	payload := []byte(`{"value":23.5,"unit":"celsius"}`)
	err := w.SetResponse(common.Content, message.AppJSON, bytes.NewReader(payload))
	if err != nil {
		log.Printf("  ERROR: cannot set response: %v", err)
	}
	log.Println("  -> Serving temperature (no block-wise needed)")
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
