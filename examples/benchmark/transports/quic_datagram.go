package transports

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"time"

	"github.com/golioth/coap-over-quic-poc/common"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/udp/coder"
	"github.com/quic-go/quic-go"
)

// QUICDatagramClient implements TransportClient for CoAP over QUIC datagrams
type QUICDatagramClient struct {
	conn  *quic.Conn
	codec coder.Coder
}

// NewQUICDatagramClient creates a new QUIC datagram client
func NewQUICDatagramClient() *QUICDatagramClient {
	return &QUICDatagramClient{
		codec: coder.Coder{},
	}
}

// Name returns the transport name
func (c *QUICDatagramClient) Name() string {
	return "quic-datagram"
}

// Connect establishes a QUIC connection with datagram support
func (c *QUICDatagramClient) Connect(addr string) error {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"coap-quic-poc"},
	}

	quicConfig := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
		EnableDatagrams: true, // Enable RFC 9221
	}

	conn, err := quic.DialAddr(context.Background(), addr, tlsConfig, quicConfig)
	if err != nil {
		return fmt.Errorf("failed to dial QUIC: %w", err)
	}

	c.conn = conn
	return nil
}

// Close closes the connection
func (c *QUICDatagramClient) Close() error {
	if c.conn != nil {
		return c.conn.CloseWithError(0, "benchmark complete")
	}
	return nil
}

// SendRequest sends a CoAP request over a QUIC stream (reliable)
func (c *QUICDatagramClient) SendRequest(ctx context.Context, method, path string, payload []byte) ([]byte, int, time.Duration, error) {
	start := time.Now()

	// Open stream for reliable request/response
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

// SendNON sends a non-confirmable message via QUIC datagram
func (c *QUICDatagramClient) SendNON(ctx context.Context, method, path string, payload []byte) (int, time.Duration, error) {
	start := time.Now()

	// Build CoAP NON message
	token := []byte{0x01, 0x02, 0x03, 0x04}
	request := common.NewMessage(common.NonConfirmable, common.MethodFromString(method), token)
	common.AddPathOption(request, path)
	if len(payload) > 0 {
		common.SetJSONPayload(request, payload)
	}

	// Encode message
	requestData := make([]byte, 1200) // Max datagram size
	requestLen, err := c.codec.Encode(*request, requestData)
	if err != nil {
		return 0, time.Since(start), fmt.Errorf("failed to encode request: %w", err)
	}

	// Send via datagram
	if err := c.conn.SendDatagram(requestData[:requestLen]); err != nil {
		return 0, time.Since(start), fmt.Errorf("failed to send datagram: %w", err)
	}

	duration := time.Since(start)
	return requestLen, duration, nil
}
