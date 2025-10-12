# CoAP over QUIC with 0-RTT Connection Resumption

This demo demonstrates QUIC's 0-RTT (Zero Round-Trip Time) connection resumption feature, which significantly speeds up reconnections for IoT devices.

## What is 0-RTT?

0-RTT allows a client that has previously connected to a server to send application data in the very first packet of a new connection, without waiting for the TLS handshake to complete. This is achieved by caching session tickets from previous connections.

## Benefits for IoT

- **Faster reconnection**: 2-3x faster than full handshake
- **Battery savings**: Less time with radio on
- **Better user experience**: Immediate data transmission
- **Server efficiency**: Lower CPU usage for reconnects

## Running the Demo

### Start the Server

```bash
cd ../server-datagram
go run main.go
```

### Run the 0-RTT Client

```bash
cd ../client-0rtt
go run main.go
```

## Demo Scenarios

### 1. Cold Start (Initial Connection)
First connection with no cached session - performs full 1-RTT handshake.

###  2. Warm Reconnect
Reconnection using cached session ticket - uses 0-RTT!

### 3. Multiple Reconnects
Simulates device repeatedly reconnecting (e.g., waking from sleep).

## Typical Results

```
Cold Start (1-RTT):          ~10ms
Warm Reconnect (0-RTT):      ~4ms
Average 0-RTT reconnect:     ~3-4ms

🚀 0-RTT provides 2-3x faster reconnection!
```

## Security Considerations

0-RTT data is **not forward secret** and can be replayed. Therefore:
- Only send idempotent requests in 0-RTT
- Avoid state-changing operations (use regular 1-RTT for those)
- Server should implement replay protection

## How It Works

1. **Initial Connection**: Client connects, server sends session ticket
2. **Session Cache**: Client stores ticket in `tls.ClientSessionCache`
3. **Reconnection**: Client uses `tr.DialEarly()` to attempt 0-RTT
4. **0-RTT Data**: Application data sent in first flight (before handshake completes)
5. **Fallback**: If server rejects 0-RTT, falls back to 1-RTT

## References

- [RFC 9001: QUIC + TLS 1.3](https://www.rfc-editor.org/rfc/rfc9001.html#section-4.6)
- [RFC 8446: TLS 1.3 0-RTT](https://www.rfc-editor.org/rfc/rfc8446.html#section-2.3)
- [quic-go 0-RTT Documentation](https://quic-go.net/docs/quic/client/)
