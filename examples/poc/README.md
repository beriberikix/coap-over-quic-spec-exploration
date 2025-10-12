# CoAP over QUIC - Proof of Concept

This is a basic implementation demonstrating the core concepts of the CoAP over QUIC specification defined in [spec.md](../../spec.md).

## Overview

This PoC provides multiple implementations to demonstrate and compare CoAP transport options:

### CoAP over QUIC (Primary Focus)
- **QUIC transport**: Using [quic-go](https://github.com/quic-go/quic-go) for QUIC protocol support
- **TLS 1.3 encryption**: Built into QUIC (no separate DTLS needed)
- **Bidirectional streams**: For reliable CoAP request/response pairs
- **QUIC Datagrams (RFC 9221)**: For unreliable NON messages
- **CoAP messaging**: RFC 7252 compliant message format using [go-coap](https://github.com/plgd-dev/go-coap)

### CoAP over UDP (Baseline Comparison)
- **Native UDP transport**: Using go-coap's built-in UDP support
- **Standard CoAP**: RFC 7252 with CON/NON messages, retransmission, deduplication
- **Optional DTLS**: Can be added for encrypted comparison (not implemented yet)

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
├── README.md              # This file
├── go.mod                 # Go module definition
├── certs/                 # TLS certificates (for QUIC)
│   ├── generate.sh        # Certificate generation script
│   ├── server.crt         # Server certificate
│   └── server.key         # Server private key
├── common/                # Shared CoAP utilities
│   └── coap.go            # CoAP message helpers (wraps go-coap)
├── server/                # CoAP/QUIC server (streams only)
│   └── main.go
├── client/                # CoAP/QUIC client (streams only)
│   └── main.go
├── server-datagram/       # CoAP/QUIC server (streams + RFC 9221 datagrams)
│   └── main.go
├── client-datagram/       # CoAP/QUIC client (streams + RFC 9221 datagrams)
│   └── main.go
├── udp-server/            # CoAP/UDP server (baseline comparison)
│   └── main.go
└── udp-client/            # CoAP/UDP client (baseline comparison)
    └── main.go
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

## Running the Demos

This PoC includes three implementations that can be run independently:

### 1. CoAP over QUIC (Streams Only)

Basic QUIC implementation using bidirectional streams for all messages.

**Terminal 1: Start the Server**
```bash
cd server
go run main.go
```

**Terminal 2: Run the Client**
```bash
cd client
go run main.go
```

The client runs 5 demos with stream-based requests.

---

### 2. CoAP over QUIC with Datagrams (RFC 9221)

Full QUIC implementation with both streams (reliable) and datagrams (unreliable).

**Terminal 1: Start the Server**
```bash
cd server-datagram
go run main.go
```

**Terminal 2: Run the Client**
```bash
cd client-datagram
go run main.go
```

The client runs 8 demos including:
1. Resource Discovery (stream)
2. Temperature Reading (stream)
3. Telemetry via Datagram (NON)
4. Log via Datagram (NON)
5. Rapid Sensor Burst (10 datagrams)
6. LED State Query (stream)
7. LED Toggle (stream)
8. Mixed Streams and Datagrams

---

### 3. CoAP over UDP (Baseline)

Standard CoAP over UDP for performance comparison.

**Terminal 1: Start the Server**
```bash
cd udp-server
go run main.go
```

**Terminal 2: Run the Client**
```bash
cd udp-client
go run main.go
```

The client runs 8 demos using CON (confirmable) and NON (non-confirmable) messages:
1. Resource Discovery (CON)
2. Temperature Reading (CON)
3. Telemetry via NON message
4. Log via NON message
5. Rapid Sensor Burst (10 NON messages)
6. LED State Query (CON)
7. LED Toggle (CON)
8. Mixed CON and NON Messages

**Note**: All three implementations use the same port (5683), so only run one server at a time.

## Server Endpoints

All server implementations expose the following CoAP resources:

| Endpoint | Method | Description | Transport Support |
|----------|--------|-------------|-------------------|
| `/.well-known/core` | GET | Resource discovery (returns available resources) | All |
| `/temp` | GET | Returns simulated temperature reading in JSON | All |
| `/led` | GET | Returns current LED state | All |
| `/led` | PUT | Toggles LED state and returns new state | All |
| `/telemetry` | POST | Accepts telemetry data (fire-and-forget) | All |
| `/log` | POST | Accepts log messages (fire-and-forget) | All |

## Example Output

### CoAP/QUIC with Datagrams (server-datagram)

**Server Side:**
```
Starting CoAP over QUIC Server with Datagram Support (RFC 9221)...
Listening on port 5683 (coaps+quic://localhost:5683)
Server ready. Waiting for connections...
Supports:
  - Bidirectional streams (reliable request/response)
  - QUIC Datagrams (unreliable NON messages, RFC 9221)
New connection from 127.0.0.1:xxxxx
New bidirectional stream 0 opened
[STREAM 0] Received 25 bytes
[STREAM 0] CoAP Request: Type=0, Code=0.01, Token=a1b2c3d4
Request path: /.well-known/core
[STREAM 0] Sent CoAP Response: Code=2.05, 67 bytes
[DATAGRAM] Received 75 bytes
[DATAGRAM] CoAP NON: Type=1, Code=0.02, Token=xyz123
[DATAGRAM] Request path: /telemetry
[DATAGRAM] Telemetry data received (fire-and-forget)
```

**Client Side:**
```
CoAP over QUIC Client with Datagram Support (RFC 9221)...
Connecting to coaps+quic://localhost:5683
Connected to 127.0.0.1:5683
Supports:
  - Bidirectional streams (reliable CON messages)
  - QUIC Datagrams (unreliable NON messages, RFC 9221)

=== Demo 1: Resource Discovery (Stream) ===
  -> Stream 0: Sending GET /.well-known/core (25 bytes)
  <- Stream 0: Received 67 bytes
  Response Code: 2.05
  Available Resources:
    </temp>;rt="temperature",</led>;rt="actuator",</telemetry>;rt="telemetry",</log>;rt="log"

=== Demo 3: Send Telemetry via Datagram (NON) ===
  -> Datagram: Sending POST /telemetry (75 bytes)
  <- Datagram sent (no response expected)
```

### CoAP/UDP Baseline

**Server Side:**
```
Starting CoAP over UDP Server (RFC 7252)...
Listening on port 5683 (coap://localhost:5683)
Server ready. Waiting for requests...
Supports:
  - CON (confirmable) messages with ACK
  - NON (non-confirmable) messages
  - Retransmission and deduplication
[CON] GET /.well-known/core from 127.0.0.1:xxxxx
  -> Serving resource discovery
[NON] POST /telemetry from 127.0.0.1:xxxxx
  -> Telemetry data received (60 bytes)
```

**Client Side:**
```
CoAP over UDP Client (RFC 7252)...
Connecting to coap://localhost:5683
Connected to 127.0.0.1:5683
Supports:
  - CON (confirmable) messages with retransmission
  - NON (non-confirmable) messages

=== Demo 1: Resource Discovery (CON) ===
  -> Sending CON GET /.well-known/core
  <- Response Code: 2.05
  Available Resources:
    </temp>;rt="temperature",</led>;rt="actuator",</telemetry>;rt="telemetry",</log>;rt="log"

=== Demo 3: Send Telemetry (NON) ===
  -> Sending NON POST /telemetry
  <- NON message sent (no response expected)
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

## Key Differences: CoAP/QUIC vs CoAP/UDP

As specified in the spec, CoAP over QUIC simplifies several aspects:

### CoAP over QUIC (Streams)
1. **No ACK messages**: QUIC streams provide reliable delivery
2. **No retransmission logic**: QUIC handles packet loss automatically
3. **No deduplication**: QUIC ensures exactly-once delivery per stream
4. **No multicast**: QUIC is unicast only
5. **Advanced congestion control**: Built into QUIC

### CoAP over QUIC (Datagrams - RFC 9221)
- **Similar to UDP**: Unreliable, unordered delivery (like NON messages)
- **Encrypted**: Unlike plain UDP, datagrams are still encrypted via QUIC
- **Connection context**: Datagrams tied to existing QUIC connection
- **No head-of-line blocking**: Unlike QUIC streams

### CoAP over UDP (Traditional)
- **CON messages**: Require explicit ACK and retransmission
- **NON messages**: Fire-and-forget, no reliability guarantees
- **Message deduplication**: Required to handle retransmissions
- **Optional DTLS**: Adds overhead and complexity for encryption

## Implementation Status

### Completed Features
- ✅ CoAP over QUIC with bidirectional streams (Section 3.1)
- ✅ QUIC Datagrams for NON messages (RFC 9221)
- ✅ CoAP over UDP baseline for comparison
- ✅ Resource discovery (/.well-known/core)
- ✅ GET/PUT/POST methods
- ✅ CON and NON message types
- ✅ Token-based request/response matching
- ✅ TLS 1.3 encryption (built into QUIC)
- ✅ Concurrent request handling

### Future Enhancements
The spec defines additional features that could be implemented:

- **Unidirectional streams** for NON messages (Section 5.3) - alternative to datagrams
- **Block-wise transfers** (RFC 7959) - leveraging QUIC stream reliability
- **Observe** pattern (RFC 7641) - for pub/sub and real-time updates
- **Connection migration** - for mobile devices changing networks
- **0-RTT connection resumption** - for ultra-low latency reconnects
- **DTLS for UDP** - encrypted UDP baseline for fair comparison
- **Performance benchmarking framework** - automated latency and throughput testing

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
