package transports

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/golioth/coap-over-quic-poc/common"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/udp/coder"
	"github.com/quic-go/quic-go"
)

// QUIC0RTTClient implements AdvancedTransportClient for testing 0-RTT connection resumption
type QUIC0RTTClient struct {
	conn      *quic.Conn
	transport *quic.Transport
	udpConn   *net.UDPConn
	tlsConfig *tls.Config
	quicConfig *quic.Config
	serverAddr *net.UDPAddr
	codec     coder.Coder
}

// NewQUIC0RTTClient creates a new QUIC 0-RTT client
func NewQUIC0RTTClient() *QUIC0RTTClient {
	return &QUIC0RTTClient{
		codec: coder.Coder{},
	}
}

// Name returns the transport name
func (c *QUIC0RTTClient) Name() string {
	return "quic-0rtt"
}

// Connect establishes initial connection and caches session
func (c *QUIC0RTTClient) Connect(addr string) error {
	// Create TLS config with session cache for 0-RTT
	c.tlsConfig = &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"coap-quic-poc"},
		ClientSessionCache: tls.NewLRUClientSessionCache(100), // Enable session caching
	}

	c.quicConfig = &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
		EnableDatagrams: true,
	}

	// Create UDP connection
	var err error
	c.udpConn, err = net.ListenUDP("udp", nil)
	if err != nil {
		return fmt.Errorf("failed to create UDP connection: %w", err)
	}

	// Create Transport
	c.transport = &quic.Transport{
		Conn: c.udpConn,
	}

	// Resolve server address
	c.serverAddr, err = net.ResolveUDPAddr("udp", addr)
	if err != nil {
		c.udpConn.Close()
		return fmt.Errorf("failed to resolve address: %w", err)
	}

	// Initial connection (cold start)
	c.conn, err = c.transport.Dial(context.Background(), c.serverAddr, c.tlsConfig, c.quicConfig)
	if err != nil {
		c.udpConn.Close()
		return fmt.Errorf("failed to connect: %w", err)
	}

	return nil
}

// Close closes the connection
func (c *QUIC0RTTClient) Close() error {
	if c.conn != nil {
		c.conn.CloseWithError(0, "benchmark complete")
	}
	if c.transport != nil {
		c.transport.Close()
	}
	if c.udpConn != nil {
		c.udpConn.Close()
	}
	return nil
}

// SendRequest sends a CoAP request over a QUIC stream
func (c *QUIC0RTTClient) SendRequest(ctx context.Context, method, path string, payload []byte) ([]byte, int, time.Duration, error) {
	start := time.Now()

	// Open stream
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, 0, time.Since(start), fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	// Build CoAP request
	token := []byte{0x01, 0x02, 0x03, 0x04}
	request := common.NewMessage(common.Confirmable, common.MethodFromString(method), token)
	common.AddPathOption(request, path)
	if len(payload) > 0 {
		common.SetJSONPayload(request, payload)
	}

	// Encode and send request
	requestData := make([]byte, 65535)
	requestLen, err := c.codec.Encode(*request, requestData)
	if err != nil {
		return nil, 0, time.Since(start), fmt.Errorf("failed to encode request: %w", err)
	}

	bytesSent := requestLen
	if _, err := stream.Write(requestData[:requestLen]); err != nil {
		return nil, 0, time.Since(start), fmt.Errorf("failed to write request: %w", err)
	}

	stream.Close()

	// Read response
	responseData, err := io.ReadAll(stream)
	if err != nil {
		return nil, 0, time.Since(start), fmt.Errorf("failed to read response: %w", err)
	}

	bytesReceived := len(responseData)
	totalBytes := bytesSent + bytesReceived

	// Decode response
	response := message.Message{
		Options: make(message.Options, 0, 16),
	}
	if _, err := c.codec.Decode(responseData, &response); err != nil {
		return nil, totalBytes, time.Since(start), fmt.Errorf("failed to decode response: %w", err)
	}

	duration := time.Since(start)
	return response.Payload, totalBytes, duration, nil
}

// SendNON sends a non-confirmable message
func (c *QUIC0RTTClient) SendNON(ctx context.Context, method, path string, payload []byte) (int, time.Duration, error) {
	start := time.Now()
	return 0, time.Since(start), fmt.Errorf("NON messages not primary focus for 0-RTT testing")
}

// TestConnectionResumption tests 0-RTT connection resumption
// Returns (cold connection duration, warm connection duration, error)
func (c *QUIC0RTTClient) TestConnectionResumption(addr string) (time.Duration, time.Duration, error) {
	// Cold start - initial connection (already done in Connect, but measure it)
	coldStart := time.Now()
	coldConn, err := c.transport.Dial(context.Background(), c.serverAddr, c.tlsConfig, c.quicConfig)
	if err != nil {
		return 0, 0, fmt.Errorf("cold start failed: %w", err)
	}
	coldDuration := time.Since(coldStart)

	// Send a dummy request to complete handshake and cache session
	stream, err := coldConn.OpenStreamSync(context.Background())
	if err != nil {
		coldConn.CloseWithError(0, "test")
		return coldDuration, 0, fmt.Errorf("failed to open stream for cold start: %w", err)
	}
	stream.Close()

	// Close connection
	coldConn.CloseWithError(0, "cold start complete")
	time.Sleep(100 * time.Millisecond)

	// Warm start - 0-RTT resumption
	warmStart := time.Now()
	warmConn, err := c.transport.DialEarly(context.Background(), c.serverAddr, c.tlsConfig, c.quicConfig)
	if err != nil {
		return coldDuration, 0, fmt.Errorf("0-RTT resumption failed: %w", err)
	}
	warmDuration := time.Since(warmStart)

	// Send a dummy request with 0-RTT
	stream, err = warmConn.OpenStreamSync(context.Background())
	if err != nil {
		warmConn.CloseWithError(0, "test")
		return coldDuration, warmDuration, fmt.Errorf("failed to open stream for warm start: %w", err)
	}
	stream.Close()

	// Keep this connection as the active one
	if c.conn != nil {
		c.conn.CloseWithError(0, "replacing")
	}
	c.conn = warmConn

	return coldDuration, warmDuration, nil
}

// TestMigration is not applicable for 0-RTT testing
func (c *QUIC0RTTClient) TestMigration() (time.Duration, error) {
	return 0, fmt.Errorf("migration not supported by 0-RTT client")
}

// SubscribeObserve is not applicable for 0-RTT testing
func (c *QUIC0RTTClient) SubscribeObserve(ctx context.Context, path string) (time.Duration, error) {
	return 0, fmt.Errorf("observe not supported by 0-RTT client")
}

// WaitForNotifications is not applicable for 0-RTT testing
func (c *QUIC0RTTClient) WaitForNotifications(ctx context.Context, count int) ([]time.Duration, error) {
	return nil, fmt.Errorf("observe not supported by 0-RTT client")
}
