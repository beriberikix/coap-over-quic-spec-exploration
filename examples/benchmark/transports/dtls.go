package transports

import (
	"bytes"
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	piondtls "github.com/pion/dtls/v3"
	"github.com/plgd-dev/go-coap/v3/dtls"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/message/codes"
	"github.com/plgd-dev/go-coap/v3/message/pool"
	"github.com/plgd-dev/go-coap/v3/udp/client"
)

// DTLSClient implements TransportClient for CoAP over UDP with DTLS encryption
type DTLSClient struct {
	conn *client.Conn
}

// NewDTLSClient creates a new DTLS client
func NewDTLSClient() *DTLSClient {
	return &DTLSClient{}
}

// Name returns the transport name
func (c *DTLSClient) Name() string {
	return "dtls"
}

// Connect establishes a DTLS connection
func (c *DTLSClient) Connect(addr string) error {
	// Create DTLS configuration
	dtlsConfig, err := createDTLSConfig()
	if err != nil {
		return fmt.Errorf("failed to create DTLS config: %w", err)
	}

	// Connect with DTLS
	conn, err := dtls.Dial(addr, dtlsConfig)
	if err != nil {
		return fmt.Errorf("failed to dial DTLS: %w", err)
	}

	c.conn = conn
	return nil
}

// Close closes the connection
func (c *DTLSClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// SendRequest sends a CoAP request over DTLS
func (c *DTLSClient) SendRequest(ctx context.Context, method, path string, payload []byte) ([]byte, int, time.Duration, error) {
	start := time.Now()

	var resp *pool.Message
	var err error

	// Estimate bytes sent (CoAP header + options + payload)
	// DTLS adds encryption overhead (~13-29 bytes per record depending on cipher suite)
	// We'll estimate +25 bytes for DTLS record header and MAC
	baseCoAPSize := 4 + 4 + len(path) + 3 // CoAP header + token + options
	if len(payload) > 0 {
		baseCoAPSize += len(payload) + 1 // +1 for payload marker
	}
	bytesSent := baseCoAPSize + 25 // +25 bytes for DTLS overhead

	switch method {
	case "GET":
		resp, err = c.conn.Get(ctx, path)
	case "POST":
		if len(payload) > 0 {
			resp, err = c.conn.Post(ctx, path, message.AppJSON, bytes.NewReader(payload))
		} else {
			resp, err = c.conn.Post(ctx, path, message.TextPlain, nil)
		}
	case "PUT":
		if len(payload) > 0 {
			resp, err = c.conn.Put(ctx, path, message.AppJSON, bytes.NewReader(payload))
		} else {
			resp, err = c.conn.Put(ctx, path, message.TextPlain, nil)
		}
	default:
		return nil, 0, time.Since(start), fmt.Errorf("unsupported method: %s", method)
	}

	if err != nil {
		return nil, bytesSent, time.Since(start), fmt.Errorf("request failed: %w", err)
	}

	// Read response body
	body := readBodyDTLS(resp.Body())

	// Estimate bytes received (CoAP response + DTLS overhead)
	baseResponseSize := 4 + len(resp.Token()) + len(body) + 1
	bytesReceived := baseResponseSize + 25 // +25 bytes for DTLS overhead

	duration := time.Since(start)
	totalBytes := bytesSent + bytesReceived

	return body, totalBytes, duration, nil
}

// SendNON sends a non-confirmable CoAP message over DTLS
func (c *DTLSClient) SendNON(ctx context.Context, method, path string, payload []byte) (int, time.Duration, error) {
	start := time.Now()

	// Create NON message
	req := c.conn.AcquireMessage(ctx)
	defer c.conn.ReleaseMessage(req)

	req.SetCode(methodToCodeDTLS(method))
	req.SetPath(path)
	if len(payload) > 0 {
		req.SetContentFormat(message.AppJSON)
		req.SetBody(bytes.NewReader(payload))
	}
	req.SetType(message.NonConfirmable)

	// Send message
	if err := c.conn.WriteMessage(req); err != nil {
		return 0, time.Since(start), fmt.Errorf("failed to send NON message: %w", err)
	}

	// Estimate bytes sent (CoAP + DTLS overhead)
	baseSize := 4 + len(req.Token()) + len(path) + 3
	if len(payload) > 0 {
		baseSize += len(payload) + 1
	}
	bytesSent := baseSize + 25 // +25 bytes for DTLS overhead

	duration := time.Since(start)
	return bytesSent, duration, nil
}

// Helper functions

// createDTLSConfig creates DTLS configuration with certificates
func createDTLSConfig() (*piondtls.Config, error) {
	// Find the certs directory (relative to benchmark binary location)
	// Try several possible paths
	certPaths := []string{
		"../poc/certs/server.crt",
		"../../poc/certs/server.crt",
		"../../../poc/certs/server.crt",
		"poc/certs/server.crt",
	}

	var certPEM []byte
	var err error
	for _, path := range certPaths {
		certPEM, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		// Try to find it relative to current working directory
		wd, _ := os.Getwd()
		relPath := filepath.Join(wd, "..", "poc", "certs", "server.crt")
		certPEM, err = os.ReadFile(relPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read server certificate from any known location: %w", err)
		}
	}

	// Create certificate pool
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(certPEM) {
		return nil, fmt.Errorf("failed to parse server certificate")
	}

	return &piondtls.Config{
		RootCAs:              certPool,
		InsecureSkipVerify:   true, // Skip hostname verification for localhost testing
		ExtendedMasterSecret: piondtls.RequireExtendedMasterSecret,
	}, nil
}

func readBodyDTLS(r io.ReadSeeker) []byte {
	if r == nil {
		return nil
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	return buf.Bytes()
}

func methodToCodeDTLS(method string) codes.Code {
	switch method {
	case "GET":
		return codes.GET
	case "POST":
		return codes.POST
	case "PUT":
		return codes.PUT
	case "DELETE":
		return codes.DELETE
	default:
		return codes.GET
	}
}
