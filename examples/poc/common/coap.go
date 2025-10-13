package common

import (
	"fmt"

	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/message/codes"
)

// Default port for CoAP over QUIC (TBD in spec, using 5683 for now)
const DefaultPort = 5683

// Re-export go-coap types for convenience
type (
	Message = message.Message
	Option  = message.Option
	Code    = codes.Code
)

// Common CoAP codes (re-exported from go-coap)
const (
	// Request codes
	GET    = codes.GET
	POST   = codes.POST
	PUT    = codes.PUT
	DELETE = codes.DELETE

	// Response codes
	Created = codes.Created // 2.01
	Deleted = codes.Deleted // 2.02
	Valid   = codes.Valid   // 2.03
	Changed = codes.Changed // 2.04
	Content = codes.Content // 2.05

	BadRequest           = codes.BadRequest         // 4.00
	Unauthorized         = codes.Unauthorized       // 4.01
	BadOption            = codes.BadOption          // 4.02
	Forbidden            = codes.Forbidden          // 4.03
	NotFound             = codes.NotFound           // 4.04
	MethodNotAllowed     = codes.MethodNotAllowed   // 4.05
	NotAcceptable        = codes.NotAcceptable      // 4.06
	PreconditionFailed   = codes.PreconditionFailed // 4.12
	InternalServerError  = codes.InternalServerError     // 5.00
	NotImplemented       = codes.NotImplemented          // 5.01
	BadGateway           = codes.BadGateway              // 5.02
	ServiceUnavailable   = codes.ServiceUnavailable      // 5.03
	GatewayTimeout       = codes.GatewayTimeout          // 5.04
)

// Message types (re-exported from go-coap)
const (
	Confirmable     = message.Confirmable     // CON - implicit with QUIC reliability
	NonConfirmable  = message.NonConfirmable  // NON - for unidirectional streams
	Acknowledgement = message.Acknowledgement // ACK - not used in CoAP/QUIC
	Reset           = message.Reset           // RST - handled via QUIC stream reset
)

// Option numbers (re-exported from go-coap)
const (
	URIPath       = message.URIPath
	URIQuery      = message.URIQuery
	ContentFormat = message.ContentFormat
	Accept        = message.Accept
)

// Content formats (MediaType values)
var (
	TextPlain     = message.TextPlain
	AppLinkFormat = message.AppLinkFormat
	AppJSON       = message.AppJSON
	AppCBOR       = message.AppCBOR
	AppOctets     = message.AppOctets
)

// NewMessage creates a new CoAP message using go-coap
func NewMessage(msgType message.Type, code codes.Code, token []byte) *message.Message {
	msg := &message.Message{
		Type:    msgType,
		Code:    code,
		Token:   token,
		Options: make(message.Options, 0, 8),
	}
	return msg
}

// AddPathOption adds a URI-Path option to the message
func AddPathOption(msg *message.Message, path string) {
	msg.Options = append(msg.Options, message.Option{
		ID:    message.URIPath,
		Value: []byte(path),
	})
}

// AddPathSegments adds multiple URI-Path options (one per segment)
func AddPathSegments(msg *message.Message, segments []string) {
	for _, seg := range segments {
		AddPathOption(msg, seg)
	}
}

// GetPath extracts the full URI path from a message
func GetPath(msg *message.Message) string {
	path, err := msg.Options.Path()
	if err != nil || path == "" {
		return "/"
	}
	// Options.Path() already includes the leading slash
	if path[0] != '/' {
		return "/" + path
	}
	return path
}

// SetJSONPayload sets the payload and content format for JSON
func SetJSONPayload(msg *message.Message, payload []byte) {
	msg.Options = append(msg.Options, message.Option{
		ID:    message.ContentFormat,
		Value: []byte{byte(message.AppJSON >> 8), byte(message.AppJSON)},
	})
	msg.Payload = payload
}

// SetLinkFormatPayload sets the payload and content format for CoRE Link Format
func SetLinkFormatPayload(msg *message.Message, payload []byte) {
	msg.Options = append(msg.Options, message.Option{
		ID:    message.ContentFormat,
		Value: []byte{byte(message.AppLinkFormat >> 8), byte(message.AppLinkFormat)},
	})
	msg.Payload = payload
}

// CodeToString converts a CoAP code to a human-readable string
func CodeToString(code codes.Code) string {
	class := code / 32
	detail := code % 32
	return fmt.Sprintf("%d.%02d", class, detail)
}

// MethodToString returns a human-readable method name
func MethodToString(code codes.Code) string {
	switch code {
	case codes.GET:
		return "GET"
	case codes.POST:
		return "POST"
	case codes.PUT:
		return "PUT"
	case codes.DELETE:
		return "DELETE"
	default:
		return fmt.Sprintf("0x%02x", code)
	}
}

// MethodFromString converts a method string to a CoAP code
func MethodFromString(method string) codes.Code {
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
