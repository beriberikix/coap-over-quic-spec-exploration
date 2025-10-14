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

// QUICObserveClient implements AdvancedTransportClient for testing Observe pattern (RFC 7641)
type QUICObserveClient struct {
	conn       *quic.Conn
	stream     *quic.Stream
	codec      coder.Coder
	lastNotificationTime time.Time
}

// NewQUICObserveClient creates a new QUIC Observe client
func NewQUICObserveClient() *QUICObserveClient {
	return &QUICObserveClient{
		codec: coder.Coder{},
	}
}

// Name returns the transport name
func (c *QUICObserveClient) Name() string {
	return "quic-observe"
}

// Connect establishes a QUIC connection with longer timeout for observations
func (c *QUICObserveClient) Connect(addr string) error {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"coap-quic-poc"},
	}

	// Configure QUIC with longer timeout for long-lived observations
	quicConfig := &quic.Config{
		MaxIdleTimeout:  120 * time.Second,
		KeepAlivePeriod: 30 * time.Second,
	}

	// Create UDP connection
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve address: %w", err)
	}

	udpConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return fmt.Errorf("failed to create UDP connection: %w", err)
	}

	// Create QUIC transport and connect
	tr := &quic.Transport{Conn: udpConn}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c.conn, err = tr.Dial(ctx, udpAddr, tlsConfig, quicConfig)
	if err != nil {
		udpConn.Close()
		return fmt.Errorf("failed to dial QUIC: %w", err)
	}

	return nil
}

// Close closes the connection
func (c *QUICObserveClient) Close() error {
	if c.stream != nil {
		c.stream.Close()
	}
	if c.conn != nil {
		return c.conn.CloseWithError(0, "benchmark complete")
	}
	return nil
}

// SendRequest sends a regular CoAP request (not observe)
func (c *QUICObserveClient) SendRequest(ctx context.Context, method, path string, payload []byte) ([]byte, int, time.Duration, error) {
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

// SendNON is not applicable for Observe testing
func (c *QUICObserveClient) SendNON(ctx context.Context, method, path string, payload []byte) (int, time.Duration, error) {
	start := time.Now()
	return 0, time.Since(start), fmt.Errorf("NON messages not applicable for Observe testing")
}

// SubscribeObserve subscribes to an Observe resource and returns initial response time
func (c *QUICObserveClient) SubscribeObserve(ctx context.Context, path string) (time.Duration, error) {
	start := time.Now()

	// Open a new stream (keep it open for notifications)
	var err error
	c.stream, err = c.conn.OpenStreamSync(ctx)
	if err != nil {
		return time.Since(start), fmt.Errorf("failed to open stream: %w", err)
	}

	// Create CoAP GET request with Observe option
	token := []byte{0x01, 0x02, 0x03, 0x04}
	request := common.NewMessage(message.Confirmable, common.GET, token)

	// IMPORTANT: Add Observe option FIRST (option 6 must come before URIPath option 11)
	common.AddObserveOption(request, 0)
	common.AddPathOption(request, path)

	// Encode and send request
	requestData := make([]byte, 1024)
	requestLen, err := c.codec.Encode(*request, requestData)
	if err != nil {
		c.stream.Close()
		c.stream = nil
		return time.Since(start), fmt.Errorf("failed to encode request: %w", err)
	}

	// Send request (don't close stream - we need it for notifications)
	if _, err := c.stream.Write(requestData[:requestLen]); err != nil {
		c.stream.Close()
		c.stream = nil
		return time.Since(start), fmt.Errorf("failed to write request: %w", err)
	}

	// Read initial response
	responseData := make([]byte, 1024)
	n, err := c.stream.Read(responseData)
	if err != nil {
		c.stream.Close()
		c.stream = nil
		return time.Since(start), fmt.Errorf("failed to read initial response: %w", err)
	}

	// Decode initial response
	response := &message.Message{
		Options: make(message.Options, 0, 16),
	}
	if _, err := c.codec.Decode(responseData[:n], response); err != nil {
		c.stream.Close()
		c.stream = nil
		return time.Since(start), fmt.Errorf("failed to decode response: %w", err)
	}

	// Verify Observe option is present
	if _, hasObserve := common.GetObserveOption(response); !hasObserve {
		c.stream.Close()
		c.stream = nil
		return time.Since(start), fmt.Errorf("response missing Observe option")
	}

	c.lastNotificationTime = time.Now()
	return time.Since(start), nil
}

// WaitForNotifications waits for Observe notifications and returns their timings
func (c *QUICObserveClient) WaitForNotifications(ctx context.Context, count int) ([]time.Duration, error) {
	if c.stream == nil {
		return nil, fmt.Errorf("no active observation - call SubscribeObserve first")
	}

	timings := make([]time.Duration, 0, count)

	for i := 0; i < count; i++ {
		// Wait for notification
		responseData := make([]byte, 1024)
		n, err := c.stream.Read(responseData)
		if err != nil {
			if err == io.EOF {
				break
			}
			return timings, fmt.Errorf("failed to read notification: %w", err)
		}

		// Record timing since last notification
		now := time.Now()
		timeSinceLastNotification := now.Sub(c.lastNotificationTime)
		timings = append(timings, timeSinceLastNotification)
		c.lastNotificationTime = now

		// Decode notification
		response := &message.Message{
			Options: make(message.Options, 0, 16),
		}
		if _, err := c.codec.Decode(responseData[:n], response); err != nil {
			return timings, fmt.Errorf("failed to decode notification: %w", err)
		}

		// Verify it's an observe notification
		if _, hasObserve := common.GetObserveOption(response); !hasObserve {
			return timings, fmt.Errorf("notification missing Observe option")
		}
	}

	return timings, nil
}

// TestConnectionResumption is not applicable for Observe testing
func (c *QUICObserveClient) TestConnectionResumption(addr string) (time.Duration, time.Duration, error) {
	return 0, 0, fmt.Errorf("0-RTT not supported by Observe client")
}

// TestMigration is not applicable for Observe testing
func (c *QUICObserveClient) TestMigration() (time.Duration, error) {
	return 0, fmt.Errorf("migration not supported by Observe client")
}
