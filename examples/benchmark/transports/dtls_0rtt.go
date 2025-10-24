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

// DTLS0RTTClient implements AdvancedTransportClient for testing DTLS session resumption
// Uses pion/dtls SessionStore to cache sessions for abbreviated handshake
type DTLS0RTTClient struct {
	conn         *client.Conn
	dtlsConfig   *piondtls.Config
	sessionStore *InMemorySessionStore
	serverAddr   string
}

// NewDTLS0RTTClient creates a new DTLS 0-RTT client
func NewDTLS0RTTClient() *DTLS0RTTClient {
	return &DTLS0RTTClient{
		sessionStore: NewInMemorySessionStore(),
	}
}

// Name returns the transport name
func (c *DTLS0RTTClient) Name() string {
	return "dtls-0rtt"
}

// Connect establishes initial connection with session caching enabled
func (c *DTLS0RTTClient) Connect(addr string) error {
	c.serverAddr = addr

	// Create DTLS configuration with session store
	var err error
	c.dtlsConfig, err = createDTLSConfigWithSessionStore(c.sessionStore)
	if err != nil {
		return fmt.Errorf("failed to create DTLS config: %w", err)
	}

	// Connect with DTLS (this will cache the session)
	c.conn, err = dtls.Dial(addr, c.dtlsConfig)
	if err != nil {
		return fmt.Errorf("failed to dial DTLS: %w", err)
	}

	return nil
}

// Close closes the connection
func (c *DTLS0RTTClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// SendRequest sends a CoAP request over DTLS
func (c *DTLS0RTTClient) SendRequest(ctx context.Context, method, path string, payload []byte) ([]byte, int, time.Duration, error) {
	start := time.Now()

	var resp *pool.Message
	var err error

	// Estimate bytes sent (CoAP header + options + payload + DTLS overhead)
	baseCoAPSize := 4 + 4 + len(path) + 3
	if len(payload) > 0 {
		baseCoAPSize += len(payload) + 1
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
	body := readBodyDTLS0RTT(resp.Body())

	// Estimate bytes received
	baseResponseSize := 4 + len(resp.Token()) + len(body) + 1
	bytesReceived := baseResponseSize + 25 // +25 bytes for DTLS overhead

	duration := time.Since(start)
	totalBytes := bytesSent + bytesReceived

	return body, totalBytes, duration, nil
}

// SendNON sends a non-confirmable CoAP message over DTLS
func (c *DTLS0RTTClient) SendNON(ctx context.Context, method, path string, payload []byte) (int, time.Duration, error) {
	start := time.Now()

	// Create NON message
	req := c.conn.AcquireMessage(ctx)
	defer c.conn.ReleaseMessage(req)

	req.SetCode(methodToCodeDTLS0RTT(method))
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

	// Estimate bytes sent
	baseSize := 4 + len(req.Token()) + len(path) + 3
	if len(payload) > 0 {
		baseSize += len(payload) + 1
	}
	bytesSent := baseSize + 25 // +25 bytes for DTLS overhead

	duration := time.Since(start)
	return bytesSent, duration, nil
}

// TestConnectionResumption tests DTLS session resumption (abbreviated handshake)
// Returns (cold connection duration, warm connection duration, error)
func (c *DTLS0RTTClient) TestConnectionResumption(addr string) (time.Duration, time.Duration, error) {
	c.serverAddr = addr

	// Create DTLS configuration with session store
	var err error
	c.dtlsConfig, err = createDTLSConfigWithSessionStore(c.sessionStore)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create DTLS config: %w", err)
	}

	// Clear any existing sessions for clean test
	c.sessionStore.Clear()

	// Cold start - full DTLS handshake
	coldStart := time.Now()
	coldConn, err := dtls.Dial(addr, c.dtlsConfig)
	if err != nil {
		return 0, 0, fmt.Errorf("cold start failed: %w", err)
	}
	coldDuration := time.Since(coldStart)

	// Send a dummy request to ensure session is fully established and cached
	ctx := context.Background()
	_, err = coldConn.Get(ctx, "/temp")
	if err != nil {
		coldConn.Close()
		return coldDuration, 0, fmt.Errorf("failed to send request during cold start: %w", err)
	}

	// Close connection (session should be cached now)
	coldConn.Close()
	time.Sleep(100 * time.Millisecond)

	// Warm start - abbreviated handshake using cached session
	warmStart := time.Now()
	warmConn, err := dtls.Dial(addr, c.dtlsConfig)
	if err != nil {
		return coldDuration, 0, fmt.Errorf("warm start failed: %w", err)
	}
	warmDuration := time.Since(warmStart)

	// Send a dummy request to verify abbreviated handshake worked
	_, err = warmConn.Get(ctx, "/temp")
	if err != nil {
		warmConn.Close()
		return coldDuration, warmDuration, fmt.Errorf("failed to send request during warm start: %w", err)
	}

	// Keep this connection as the active one
	if c.conn != nil {
		c.conn.Close()
	}
	c.conn = warmConn

	return coldDuration, warmDuration, nil
}

// TestMigration is not supported by DTLS 0-RTT client (use dtls-migration instead)
func (c *DTLS0RTTClient) TestMigration() (time.Duration, error) {
	return 0, fmt.Errorf("migration not supported by dtls-0rtt client (use dtls-migration)")
}

// SubscribeObserve is not applicable for DTLS 0-RTT testing
func (c *DTLS0RTTClient) SubscribeObserve(ctx context.Context, path string) (time.Duration, error) {
	return 0, fmt.Errorf("observe not supported by dtls-0rtt client")
}

// WaitForNotifications is not applicable for DTLS 0-RTT testing
func (c *DTLS0RTTClient) WaitForNotifications(ctx context.Context, count int) ([]time.Duration, error) {
	return nil, fmt.Errorf("observe not supported by dtls-0rtt client")
}

// Helper functions

// createDTLSConfigWithSessionStore creates DTLS configuration with session caching enabled
func createDTLSConfigWithSessionStore(sessionStore *InMemorySessionStore) (*piondtls.Config, error) {
	// Find the certs directory (relative to benchmark binary location)
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
		SessionStore:         sessionStore, // Enable session caching for resumption
	}, nil
}

func readBodyDTLS0RTT(r io.ReadSeeker) []byte {
	if r == nil {
		return nil
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	return buf.Bytes()
}

func methodToCodeDTLS0RTT(method string) codes.Code {
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
