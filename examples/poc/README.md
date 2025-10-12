# CoAP over QUIC - Proof of Concept

This is a basic implementation demonstrating the core concepts of the CoAP over QUIC specification defined in [spec.md](../../spec.md).

## Overview

This PoC implements a simple CoAP over QUIC client and server using:
- **QUIC transport**: Using [quic-go](https://github.com/quic-go/quic-go) for QUIC protocol support
- **TLS 1.3 encryption**: Built into QUIC (no separate DTLS needed)
- **Bidirectional streams**: For CoAP request/response pairs
- **CoAP messaging**: RFC 7252 compliant message format

## Key Spec Features Demonstrated

### 1. CoAP over QUIC Streams (Section 3.1)
- Each request/response pair uses a single **bidirectional QUIC stream**
- Client sends request and sets **FIN bit** to indicate end of request
- Server sends response on same stream with **FIN bit**
- Multiple **concurrent streams** avoid head-of-line blocking

### 2. Message Types (Section 5.1)
- **CON messages**: Implicit reliability via QUIC (no ACK messages needed)
- **Token matching**: Used to correlate requests and responses
- **Message ID**: Present but not critical for reliability (QUIC handles that)

### 3. Protocol Stack (Section 3.2)
```
+----------------------+
|    Application       |
+----------------------+
|        CoAP          |
+----------------------+
|        QUIC          |  <- TLS 1.3 built-in
+----------------------+
|         UDP          |
+----------------------+
```

### 4. Security (Section 9)
- All traffic encrypted with **TLS 1.3** (built into QUIC)
- No need for separate DTLS layer
- Better privacy than TCP+TLS (encrypted QUIC headers)

### 5. Benefits Demonstrated (Section 8.1)
- **Fast connection setup**: 1-RTT QUIC handshake
- **No head-of-line blocking**: Multiple concurrent requests on different streams
- **Automatic congestion control**: Provided by QUIC
- **Reliable delivery**: No need for CON/ACK mechanisms

## Project Structure

```
examples/poc/
├── README.md           # This file
├── go.mod              # Go module definition
├── certs/              # TLS certificates
│   ├── generate.sh     # Certificate generation script
│   ├── server.crt      # Server certificate
│   └── server.key      # Server private key
├── common/             # Shared CoAP utilities
│   └── coap.go         # CoAP message encoding/decoding
├── server/             # Server implementation
│   └── main.go         # CoAP/QUIC server
└── client/             # Client implementation
    └── main.go         # CoAP/QUIC client
```

## Prerequisites

- Go 1.21 or later
- OpenSSL (for certificate generation)

## Setup

1. **Navigate to the PoC directory**:
   ```bash
   cd examples/poc
   ```

2. **Install dependencies**:
   ```bash
   go mod tidy
   ```

3. **Generate TLS certificates** (already done if certs exist):
   ```bash
   cd certs
   ./generate.sh
   cd ..
   ```

## Running the Demo

### Terminal 1: Start the Server

```bash
cd server
go run main.go
```

Expected output:
```
Starting CoAP over QUIC Server...
Listening on port 5683 (coaps+quic://localhost:5683)
Server ready. Waiting for connections...
```

### Terminal 2: Run the Client

```bash
cd client
go run main.go
```

The client will run through 5 demos:

1. **Resource Discovery**: GET `/.well-known/core`
2. **Temperature Reading**: GET `/temp`
3. **LED State Query**: GET `/led`
4. **LED Toggle**: PUT `/led`
5. **Concurrent Requests**: Multiple simultaneous requests on different streams

## Server Endpoints

The server implements the following CoAP resources:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/.well-known/core` | GET | Resource discovery (returns available resources) |
| `/temp` | GET | Returns simulated temperature reading in JSON |
| `/led` | GET | Returns current LED state |
| `/led` | PUT | Toggles LED state and returns new state |

## Example Output

### Server Side:
```
Starting CoAP over QUIC Server...
Listening on port 5683 (coaps+quic://localhost:5683)
Server ready. Waiting for connections...
New connection from 127.0.0.1:xxxxx
New bidirectional stream 0 opened
Received 23 bytes on stream 0
CoAP Request: Type=0, Code=0.01, Token=a1b2c3d4, MsgID=0
Request path: /.well-known/core
Serving resource discovery
Sent CoAP Response: Code=2.05, 67 bytes
```

### Client Side:
```
CoAP over QUIC Client Starting...
Connecting to coaps+quic://localhost:5683
Connected to 127.0.0.1:5683

=== Demo 1: Resource Discovery ===
  -> Stream 0: Sending GET /.well-known/core (23 bytes)
  <- Stream 0: Received 67 bytes
  Response Code: 2.05
  Available Resources:
    </temp>;rt="temperature",</led>;rt="actuator"

=== Demo 2: Get Temperature ===
  -> Stream 4: Sending GET /temp (18 bytes)
  <- Stream 4: Received 51 bytes
  Response Code: 2.05
  Temperature: {"value":24.37,"unit":"celsius"}
```

## Implementation Details

### CoAP Message Format

The implementation uses the standard CoAP message format from RFC 7252:

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|Ver| T |  TKL  |      Code     |          Message ID           |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|   Token (if any, TKL bytes) ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|   Options (if any) ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|1 1 1 1 1 1 1 1|    Payload (if any) ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

### QUIC Configuration

- **Port**: 5683 (traditional CoAP port)
- **TLS Version**: 1.3 (required by QUIC)
- **ALPN**: `coap-quic-poc`
- **Max Idle Timeout**: 30 seconds
- **Keep-Alive Period**: 10 seconds

## Differences from CoAP over UDP

As specified in the spec, several CoAP/UDP features are not needed:

1. **No ACK messages**: QUIC provides reliable delivery
2. **No retransmission**: QUIC handles packet loss
3. **No deduplication**: QUIC ensures exactly-once delivery
4. **No multicast**: QUIC is unicast only
5. **Simplified congestion control**: QUIC provides advanced congestion control

## Future Enhancements

This PoC demonstrates basic functionality. The spec defines additional features:

- **Unidirectional streams** for NON messages (Section 5.3)
- **QUIC Datagrams** for unreliable transport (RFC 9221)
- **Block-wise transfers** leveraging QUIC stream reliability
- **Observe** pattern for pub/sub (RFC 7641)
- **Connection migration** for mobile devices

## References

- [CoAP over QUIC Spec](../../spec.md)
- [RFC 7252: CoAP](https://www.rfc-editor.org/rfc/rfc7252)
- [RFC 9000: QUIC](https://www.rfc-editor.org/rfc/rfc9000)
- [quic-go Library](https://github.com/quic-go/quic-go)

## Troubleshooting

### Port already in use
If you see "bind: address already in use", another process is using port 5683:
```bash
# Find the process
lsof -i :5683
# Kill it or change the port in common/coap.go
```

### Certificate errors
If you see certificate-related errors, regenerate the certificates:
```bash
cd certs
rm server.crt server.key
./generate.sh
```

### Connection refused
Make sure the server is running before starting the client.

## License

This is a proof-of-concept implementation for educational purposes.
