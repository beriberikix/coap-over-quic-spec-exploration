# CoAP over QUIC with Connection Migration

This demo demonstrates QUIC's connection migration feature, which allows mobile IoT devices to maintain active sessions when switching between networks (e.g., WiFi ↔ Cellular).

## What is Connection Migration?

Connection migration is a QUIC feature that allows a connection to survive network changes by using Connection IDs instead of the traditional IP:port tuple for connection identification. When a device's IP address or port changes, the QUIC connection remains intact.

## Key Benefits for IoT

- **Seamless network handoff**: Switch WiFi ↔ Cellular without dropping session
- **No re-authentication**: Connection and security context maintained
- **Zero packet loss**: All active streams preserved during migration
- **Better than TCP**: TCP connections break when IP changes
- **Mobile-friendly**: Critical for roaming devices

## How It Works

### Traditional TCP Problem
```
Device on WiFi (IP: 192.168.1.100) → Server
WiFi signal lost, switches to Cellular (IP: 10.0.0.50)
❌ TCP connection breaks (new IP = new connection)
❌ Must re-establish connection, re-authenticate
```

### QUIC Solution
```
Device on WiFi (IP: 192.168.1.100, Connection ID: ABC123) → Server
WiFi signal lost, switches to Cellular (IP: 10.0.0.50, Connection ID: ABC123)
✅ QUIC connection continues (same Connection ID)
✅ Server validates new path, switches automatically
✅ All streams continue without interruption
```

## Running the Demo

### Start the Server

```bash
cd ../server-datagram
go run main.go
```

### Run the Migration Client

```bash
cd ../client-migration
go run main.go
```

## Demo Scenarios

### 1. Initial Connection (WiFi Simulation)
Creates first UDP socket to simulate WiFi interface and establishes QUIC connection.

### 2. Active Communication (WiFi)
Sends several CoAP requests over the "WiFi" connection to establish baseline.

### 3. Network Change Detected
Creates second UDP socket to simulate cellular interface becoming available.

### 4. Connection Migration
Demonstrates QUIC's transparent handling of network changes.

### 5. Verify Seamless Migration
Continues sending requests - all succeed without connection drops!

## Typical Results

```
=== Demo 2: Active Communication (WiFi) ===
  ✓ [Request 1 (WiFi)] Response: 2.05 - Success!
  ✓ [Request 2 (WiFi)] Response: 2.05 - Success!
  ✓ [Request 3 (WiFi)] Response: 2.05 - Success!

=== Demo 5: Verify Seamless Migration ===
  ✓ [Request 4 (post-migration)] Response: 2.05 - Success!
  ✓ [Request 5 (post-migration)] Response: 2.05 - Success!
  ✓ [Request 6 (post-migration)] Response: 2.05 - Success!

✓ Zero packet loss during migration
✓ All streams continued without interruption
```

## QUIC Features Demonstrated

### 1. Connection ID
- Identifies connection independent of IP/port tuple
- Allows server to recognize connection after address change
- Encoded in every QUIC packet

### 2. Path Validation
- Server validates new path before accepting it
- Prevents address spoofing attacks
- Uses PATH_CHALLENGE/PATH_RESPONSE frames

### 3. Seamless Failover
- Automatic retry on new path if old path fails
- No application-level handling needed
- Maintains all active streams and datagrams

### 4. Stream Continuity
- All active streams preserved during migration
- Stream offsets and flow control maintained
- No data loss or reordering

## Real-World Use Cases

### Mobile Sensor Devices
```
Warehouse robot moving between WiFi zones
→ Maintains telemetry stream while roaming
```

### Vehicle Telematics
```
Connected car driving through cellular coverage areas
→ CoAP session survives cell tower handoffs
```

### IoT Gateway Failover
```
Gateway has both Ethernet and 4G backup
→ Seamless failover if primary link fails
```

### Mobile Health Devices
```
Wearable device switches networks while transmitting health data
→ Critical data stream uninterrupted
```

## Implementation Notes

### Current quic-go Behavior

quic-go handles connection migration **automatically**. The application doesn't need to:
- Detect path changes
- Manually switch paths
- Re-establish streams

The QUIC implementation handles it transparently when:
- Client's source IP changes
- Client's source port changes
- Network interface switches

### Future API Enhancements

The quic-go project is working on explicit path management APIs:
- `conn.AddPath(transport)` - Proactively create alternative paths
- `path.Commit()` - Explicitly switch to new path
- Path metrics and status monitoring

## Comparison with Other Protocols

| Feature | QUIC | TCP | UDP |
|---------|------|-----|-----|
| Connection ID | ✅ Yes | ❌ No (IP:port only) | ❌ Connectionless |
| Survives IP change | ✅ Yes | ❌ No | N/A |
| Path validation | ✅ Built-in | ❌ None | ❌ None |
| Stream continuity | ✅ Preserved | ❌ Breaks | N/A |
| Mobile-friendly | ✅ Excellent | ❌ Poor | ⚠️ Manual handling |

## Security Considerations

### Path Validation
- Prevents address spoofing (attacker can't hijack connection)
- Server validates new path before switching
- Uses challenge-response mechanism

### Connection ID Privacy
- Connection IDs can be rotated for privacy
- Prevents passive observation of migration
- Helps protect against tracking

## References

- [RFC 9000: QUIC Transport](https://www.rfc-editor.org/rfc/rfc9000.html#section-9)
- [RFC 9000 Section 9: Connection Migration](https://www.rfc-editor.org/rfc/rfc9000.html#name-connection-migration)
- [quic-go Connection Migration Docs](https://quic-go.net/docs/quic/connection-migration/)
- [Connection Migration API Issue](https://github.com/quic-go/quic-go/issues/3990)
