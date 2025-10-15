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
	"github.com/plgd-dev/go-coap/v3/net/blockwise"
	"github.com/plgd-dev/go-coap/v3/options"
	"github.com/plgd-dev/go-coap/v3/udp"
	"github.com/plgd-dev/go-coap/v3/udp/client"
)

// UDPBlockwiseClient implements TransportClient for CoAP over UDP with block-wise transfers (RFC 7959)
// This demonstrates traditional CoAP approach for large payloads
type UDPBlockwiseClient struct {
	conn      *client.Conn
	blockSize blockwise.SZX
}

// NewUDPBlockwiseClient creates a new UDP block-wise client
func NewUDPBlockwiseClient() *UDPBlockwiseClient {
	return &UDPBlockwiseClient{
		blockSize: blockwise.SZX1024, // 1024 bytes - optimal for most networks
	}
}

// Name returns the transport name
func (c *UDPBlockwiseClient) Name() string {
	return "udp-blockwise"
}

// Connect establishes a UDP connection with block-wise enabled
func (c *UDPBlockwiseClient) Connect(addr string) error {
	transferTimeout := 30 * time.Second

	// Connect with block-wise enabled
	conn, err := udp.Dial(addr,
		options.WithBlockwise(true, c.blockSize, transferTimeout))
	if err != nil {
		return fmt.Errorf("failed to dial UDP: %w", err)
	}

	c.conn = conn
	return nil
}

// Close closes the connection
func (c *UDPBlockwiseClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// SendRequest sends a CoAP request over UDP using block-wise transfer if needed
func (c *UDPBlockwiseClient) SendRequest(ctx context.Context, method, path string, payload []byte) ([]byte, int, time.Duration, error) {
	start := time.Now()

	var resp *pool.Message
	var err error

	// Send request based on method
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
		return nil, 0, time.Since(start), fmt.Errorf("request failed: %w", err)
	}

	// Read response body
	responseBody := readBlockwiseBody(resp.Body())

	// Estimate total bytes (request + response)
	// For block-wise, this is approximate as we don't have exact protocol overhead
	blockSizeBytes := 16 << c.blockSize
	payloadBlocks := (len(payload) + blockSizeBytes - 1) / blockSizeBytes
	responseBlocks := (len(responseBody) + blockSizeBytes - 1) / blockSizeBytes

	// Rough estimate: each block has ~20 bytes CoAP header overhead
	estimatedBytes := len(payload) + len(responseBody) + (payloadBlocks+responseBlocks)*20

	duration := time.Since(start)
	return responseBody, estimatedBytes, duration, nil
}

// SendNON sends a non-confirmable message (block-wise doesn't apply to NON)
func (c *UDPBlockwiseClient) SendNON(ctx context.Context, method, path string, payload []byte) (int, time.Duration, error) {
	start := time.Now()

	// Create NON message
	req := c.conn.AcquireMessage(ctx)
	defer c.conn.ReleaseMessage(req)

	req.SetCode(methodToBlockwiseCode(method))
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

// readBlockwiseBody reads the body from a response
func readBlockwiseBody(r io.ReadSeeker) []byte {
	if r == nil {
		return nil
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	return buf.Bytes()
}

// methodToBlockwiseCode converts method string to CoAP code
func methodToBlockwiseCode(method string) codes.Code {
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
