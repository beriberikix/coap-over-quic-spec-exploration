# CoAP Transport Benchmark Tool

This tool provides comprehensive performance benchmarking for CoAP over different transport protocols.

## Transport Types

### Basic Transports
- **CoAP/QUIC Streams** (`quic-stream`) - Reliable bidirectional streams with TLS 1.3
- **CoAP/QUIC Datagrams** (`quic-datagram`) - Hybrid reliable/unreliable (RFC 9221) with TLS 1.3
- **CoAP/UDP** (`udp`) - Traditional CoAP baseline (unencrypted)
- **CoAP/DTLS** (`dtls`) - CoAP over UDP with DTLS 1.2 encryption (encrypted baseline)

### Advanced Transports
- **QUIC Streaming** (`quic-streaming`) - Native QUIC streaming vs traditional block-wise transfers
- **UDP Block-wise** (`udp-blockwise`) - Traditional CoAP with RFC 7959 block-wise transfers (for comparison)
- **QUIC 0-RTT** (`quic-0rtt`) - Connection resumption and 0-RTT performance testing
- **QUIC Migration** (`quic-migration`) - Connection migration and network handoff testing
- **QUIC Observe** (`quic-observe`) - RFC 7641 Observe pattern over long-lived streams

## Features

- **Latency Measurement**: Round-trip time (RTT) with full statistics (min, mean, median, P95, P99, max)
- **Throughput Analysis**: Bytes transferred over the wire including protocol overhead
- **Multiple Payload Sizes**: Small (sensor telemetry), medium (config), large (OTA)
- **Request Types**: GET, POST, PUT with both CON and NON messages
- **Concurrent Testing**: Sequential and concurrent request patterns
- **Advanced Metrics**: Connection resumption, migration overhead, observe notifications, streaming efficiency
- **CSV Export**: Detailed metrics for further analysis
- **Python Analysis**: Automated comparison and insights generation

## Quick Start

### 1. Start a Server

**Recommended**: Use `server-datagram` for all QUIC-based transports:

```bash
# Start the server (supports all QUIC transports)
cd ../poc/server-datagram
go run main.go
```

This server works with: `quic-stream`, `quic-datagram`, `quic-streaming`, `quic-0rtt`, `quic-migration`, `quic-observe`

**Alternative servers**:

```bash
# For UDP baseline
cd ../poc/udp-server
go run main.go

# For DTLS baseline
cd ../poc/udp-server-dtls
go run main.go
```

### 2. Run Benchmark

**Important**: Make sure the appropriate server is running for the transport you want to test.

```bash
# Build the benchmark tool
go build

# Basic transports
./benchmark -transport quic-stream -server localhost:5683
./benchmark -transport quic-datagram -server localhost:5683
./benchmark -transport udp -server localhost:5683
./benchmark -transport dtls -server localhost:5683

# Advanced transports
./benchmark -transport quic-streaming -server localhost:5683
./benchmark -transport udp-blockwise -server localhost:5683
./benchmark -transport quic-0rtt -server localhost:5683
./benchmark -transport quic-migration -server localhost:5683
./benchmark -transport quic-observe -server localhost:5683
```

**Transport → Server Mapping**:
- All `quic-*` transports → Use `server-datagram` (recommended)
- `udp` → Use `udp-server`
- `udp-blockwise` → Use `server-blockwise`
- `dtls` → Use `udp-server-dtls`

## Command-Line Options

```
-transport string (required)
    Transport to test (run without this flag to see all options)
    Basic: quic-stream, quic-datagram, udp, dtls
    Advanced: quic-streaming, udp-blockwise, quic-0rtt, quic-migration, quic-observe

-server string
    Server address (default "localhost:5683")

-output string
    Output directory for CSV results (default "./results")
```

## Test Scenarios

The benchmark runs the following scenarios for each transport:

### 1. Small GET (Temperature Reading)
- **Payload**: 0 bytes
- **Count**: 100 requests
- **Purpose**: Simulates frequent sensor readings
- **Type**: CON (reliable)

### 2. Small POST (Telemetry)
- **Payload**: 50 bytes
- **Count**: 100 requests
- **Purpose**: Simulates small telemetry uploads
- **Type**: CON (reliable)

### 3. Small NON (Fire-and-forget Telemetry)
- **Payload**: 50 bytes
- **Count**: 100 requests
- **Purpose**: Simulates unreliable telemetry
- **Type**: NON (unreliable)
- **Note**: Skipped for quic-stream (not applicable)

### 4. Medium POST (Config Update)
- **Payload**: 1 KB
- **Count**: 50 requests
- **Purpose**: Simulates configuration updates
- **Type**: CON (reliable)

### 5. Large POST (OTA Chunk)
- **Payload**: 10 KB
- **Count**: 20 requests
- **Purpose**: Simulates OTA firmware chunks
- **Type**: CON (reliable)

### 6. Very Large POST (Large OTA Chunk)
- **Payload**: 100 KB
- **Count**: 10 requests
- **Purpose**: Simulates larger OTA chunks
- **Type**: CON (reliable)

### 7. Burst Small NON (Rapid Sensor Data)
- **Payload**: 50 bytes
- **Count**: 50 requests
- **Concurrent**: 5
- **Purpose**: Simulates rapid sensor bursts
- **Type**: NON (unreliable)

## Advanced Transport Features

### QUIC Streaming (`quic-streaming`) vs UDP Block-wise (`udp-blockwise`)
Direct comparison of QUIC's native streaming capabilities against traditional CoAP block-wise transfers (RFC 7959).

**Test Setup**:
- Both transports fetch the same 51KB firmware resource
- UDP Block-wise uses 1024-byte blocks (RFC 7959)
- QUIC Streaming uses native QUIC streams (no fragmentation)

**Streaming Scenarios**:
- 1KB Transfer (50 requests)
- 10KB Transfer (30 requests)
- 50KB Transfer (20 requests)
- 100KB Transfer (10 requests)

**Actual Results** (localhost testing):
- **1KB payloads**: QUIC is **8.3x faster** (0.75ms vs 6.18ms)
- **10KB payloads**: QUIC is **15.0x faster** (0.44ms vs 6.64ms)
- **50KB payloads**: QUIC is **5.7x faster** (1.18ms vs 6.69ms)
- **Overall average**: QUIC is **9.5x faster** than block-wise

**Why is QUIC streaming faster?**
- Single request/response instead of ~50 block-wise round trips
- No block negotiation overhead (Block1/Block2 options)
- QUIC's native flow control eliminates fragmentation
- No CoAP block assembly/reassembly overhead

### QUIC 0-RTT (`quic-0rtt`)
Tests 0-RTT connection resumption performance, comparing cold start (1-RTT) against warm reconnect (0-RTT).

**What it measures**:
- Cold connection time (initial 1-RTT handshake)
- Warm connection time (0-RTT resumption with cached session)
- Speedup factor

**Expected Results**: 0-RTT should be **~3x faster** than cold start for:
- Device wake-from-sleep scenarios
- Frequent reconnection patterns
- Battery-constrained IoT devices

### QUIC Migration (`quic-migration`)
Tests connection migration capabilities, simulating network handoff (e.g., WiFi → Cellular).

**What it measures**:
- Migration overhead (time to establish new path)
- Connection continuity (verify no packet loss)
- Seamless failover

**Expected Results**:
- Minimal migration overhead (<5ms)
- Zero packet loss during migration
- Connection survives network changes

### QUIC Observe (`quic-observe`)
Tests RFC 7641 Observe pattern over long-lived QUIC streams for push notifications.

**What it measures**:
- Initial subscription latency
- Notification delivery timing
- Stream overhead vs traditional CON/ACK

**Expected Results**:
- Lower overhead than traditional polling
- Real-time push notifications
- Single long-lived connection maintained

## Output

### Console Output

The tool prints real-time progress and a summary at the end:

```
=== Benchmark Results Summary ===

## quic-stream

Latency (ms):
  Count:   370
  Min:     1.234
  Mean:    2.456
  Median:  2.301
  P95:     3.789
  P99:     4.123
  Max:     5.678

Bytes Transferred:
  Requests:    370
  Total:       45678 bytes
  Mean/req:    123 bytes
  Min/req:     45 bytes
  Max/req:     256 bytes

## quic-datagram
...

## udp
...
```

### CSV Export

Detailed metrics are exported to `results/benchmark-YYYYMMDD-HHMMSS.csv`:

| Column | Description |
|--------|-------------|
| timestamp | ISO 8601 timestamp |
| transport | Transport type (quic-stream, quic-datagram, udp, dtls, quic-streaming, udp-blockwise, quic-0rtt, quic-migration, quic-observe) |
| operation | GET, POST, PUT, CONNECT, MIGRATE, OBSERVE |
| path | Resource path |
| metric_type | latency or bytes |
| value | Measurement value (ms for latency, bytes for bytes) |
| payload_size | Size of request payload |
| success | true/false |
| error | Error message if failed |
| connection_type | "cold", "warm", or "" (for 0-RTT testing) |
| migration_event | "before", "during", "after", or "" (for migration testing) |
| observe_seq | Observation sequence number (0 for non-observe) |
| transfer_method | "stream", "blockwise", or "" (for streaming comparison) |

## Analyzing Results

### Python Analysis Script

The included `analyze_results.py` script automatically analyzes benchmark results:

```bash
python3 analyze_results.py
```

**Output includes**:
- Latency comparison table across all transports
- Performance vs UDP baseline
- Encrypted transport comparison (QUIC vs DTLS)
- 0-RTT connection resumption speedup
- Connection migration overhead
- Observe notification timing
- Streaming vs block-wise efficiency

**Example output**:
```
=== Advanced Features Analysis ===

0-RTT Connection Resumption:
  Cold start (1-RTT):  11.329 ms
  Warm reconnect (0-RTT): 3.369 ms
  Speedup: 3.4x faster!

Connection Migration:
  Migration overhead: 0.633 ms (min: 0.302, max: 1.087)
  Connection maintained throughout network changes

=== Streaming vs Block-wise Transfer Comparison ===

QUIC Streaming:    0.681 ms (single stream, no fragmentation)
UDP Block-wise:    6.471 ms (RFC 7959, 1024-byte blocks)
Speedup:           9.5x faster with QUIC streaming!

Why is QUIC streaming faster?
  • No block-wise negotiation overhead
  • Single request/response instead of multiple round trips
  • QUIC's built-in flow control and reliability
  • Eliminates CoAP Block1/Block2 option processing
```

### Manual Analysis

1. **Latency Comparison**:
   ```bash
   # Filter CSV for latency metrics
   grep "latency" results/benchmark-*.csv | grep "GET"
   ```

2. **0-RTT Analysis**:
   ```bash
   # Compare cold vs warm connection times
   grep "connection_type" results/benchmark-*.csv
   ```

3. **Protocol Overhead**:
   - Compare `Total bytes` vs `payload_size` × `count`
   - UDP typically has lowest overhead for small payloads
   - QUIC has higher initial overhead but better for large payloads

4. **Reliability vs Speed**:
   - Compare CON (reliable) vs NON (unreliable) for same payload
   - QUIC datagrams provide encryption without reliability overhead

### Import to Analysis Tools

```python
import pandas as pd
import matplotlib.pyplot as plt

# Load results
df = pd.read_csv('results/benchmark-20250112-143000.csv')

# Filter latency metrics
latency = df[df['metric_type'] == 'latency']

# Plot latency by transport
latency.boxplot(column='value', by='transport')
plt.ylabel('Latency (ms)')
plt.title('CoAP Transport Latency Comparison')
plt.show()
```

## Architecture

### Components

1. **harness/metrics.go** - Metrics collection and statistics
2. **harness/runner.go** - Test execution and scenario management
3. **transports/** - Transport-specific client implementations
4. **main.go** - CLI and orchestration

### Adding Custom Scenarios

Edit `harness/runner.go` to add new scenarios:

```go
CustomScenario := TestScenario{
    Name:        "My Custom Test",
    Description: "Description here",
    PayloadSize: 512,
    Count:       100,
    Concurrent:  1,
    Method:      "POST",
    Path:        "/custom",
    IsNON:       false,
}
```

## Limitations

### Current Limitations

1. **No Packet Loss Testing**: Requires network simulation tools (tc, netem)
2. **Byte Counting**: Estimates based on CoAP message structure, not actual wire bytes
3. **Single Connection**: Each transport uses one connection (no connection pooling)
4. **Local Testing**: Optimized for localhost, may need tuning for WAN

### Future Enhancements

- [ ] Integrate with tcpdump/pcap for actual byte counting
- [ ] Add packet loss scenarios using netem
- [x] Support for 0-RTT connection resumption testing (✅ Completed)
- [x] Connection migration testing for QUIC (✅ Completed)
- [x] Observe pattern testing for QUIC (✅ Completed)
- [x] Streaming vs block-wise comparison (✅ Completed)
- [x] DTLS support for encrypted UDP baseline (✅ Completed)
- [x] Python analysis script (✅ Completed)
- [ ] Prometheus metrics export
- [ ] Real-time charts and visualization
- [ ] DTLS session resumption comparison

## Troubleshooting

### "connection refused"
Make sure a server is running on the specified address.

### "context deadline exceeded"
Server may be overloaded. Try reducing concurrent requests or count.

### "too many open files"
Increase system limits:
```bash
ulimit -n 4096
```

### Inconsistent results
- Run multiple times and average
- Ensure no other processes are competing for resources
- Use `nice` to prioritize: `nice -n -10 ./benchmark`

## References

- [CoAP Specification (RFC 7252)](https://www.rfc-editor.org/rfc/rfc7252)
- [QUIC Protocol (RFC 9000)](https://www.rfc-editor.org/rfc/rfc9000)
- [QUIC Datagrams (RFC 9221)](https://www.rfc-editor.org/rfc/rfc9221)
- [Project PoC Documentation](../poc/README.md)
