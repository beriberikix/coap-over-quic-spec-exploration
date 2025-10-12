package transports

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/message/codes"
	"github.com/plgd-dev/go-coap/v3/message/pool"
	"github.com/plgd-dev/go-coap/v3/udp"
	"github.com/plgd-dev/go-coap/v3/udp/client"
)

// UDPClient implements TransportClient for CoAP over UDP
type UDPClient struct {
	conn *client.Conn
}

// NewUDPClient creates a new UDP client
func NewUDPClient() *UDPClient {
	return &UDPClient{}
}

// Name returns the transport name
func (c *UDPClient) Name() string {
	return "udp"
}

// Connect establishes a UDP connection
func (c *UDPClient) Connect(addr string) error {
	conn, err := udp.Dial(addr)
	if err != nil {
		return fmt.Errorf("failed to dial UDP: %w", err)
	}
	c.conn = conn
	return nil
}

// Close closes the connection
func (c *UDPClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// SendRequest sends a CoAP request over UDP
func (c *UDPClient) SendRequest(ctx context.Context, method, path string, payload []byte) ([]byte, int, time.Duration, error) {
	start := time.Now()

	var resp *pool.Message
	var err error

	// Estimate bytes sent (CoAP header + options + payload)
	// CoAP header: 4 bytes + token (4 bytes) + URI-Path option + Content-Format option
	bytesSent := 4 + 4 + len(path) + 3 // Rough estimate
	if len(payload) > 0 {
		bytesSent += len(payload) + 1 // +1 for payload marker
	}

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
	body := readBody(resp.Body())

	// Estimate bytes received (CoAP response header + payload)
	bytesReceived := 4 + len(resp.Token()) + len(body) + 1 // Rough estimate

	duration := time.Since(start)
	totalBytes := bytesSent + bytesReceived

	return body, totalBytes, duration, nil
}

// SendNON sends a non-confirmable CoAP message
func (c *UDPClient) SendNON(ctx context.Context, method, path string, payload []byte) (int, time.Duration, error) {
	start := time.Now()

	// Create NON message
	req := c.conn.AcquireMessage(ctx)
	defer c.conn.ReleaseMessage(req)

	req.SetCode(methodToCode(method))
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
	bytesSent := 4 + len(req.Token()) + len(path) + 3
	if len(payload) > 0 {
		bytesSent += len(payload) + 1
	}

	duration := time.Since(start)
	return bytesSent, duration, nil
}

// Helper functions

func readBody(r io.ReadSeeker) []byte {
	if r == nil {
		return nil
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	return buf.Bytes()
}

func methodToCode(method string) codes.Code {
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
