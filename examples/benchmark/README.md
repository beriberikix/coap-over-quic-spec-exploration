# CoAP Transport Benchmark Tool

This tool provides comprehensive performance benchmarking for CoAP over different transport protocols:
- **CoAP/QUIC Streams** - Reliable bidirectional streams
- **CoAP/QUIC Datagrams** - Hybrid reliable/unreliable (RFC 9221)
- **CoAP/UDP** - Traditional CoAP baseline

## Features

- **Latency Measurement**: Round-trip time (RTT) with full statistics (min, mean, median, P95, P99, max)
- **Throughput Analysis**: Bytes transferred over the wire including protocol overhead
- **Multiple Payload Sizes**: Small (sensor telemetry), medium (config), large (OTA)
- **Request Types**: GET, POST, PUT with both CON and NON messages
- **Concurrent Testing**: Sequential and concurrent request patterns
- **CSV Export**: Detailed metrics for further analysis

## Quick Start

### 1. Start a Server

Choose one of the three server implementations:

```bash
# CoAP/QUIC with datagrams (recommended for full feature testing)
cd ../poc/server-datagram
go run main.go

# Or CoAP/QUIC with streams only
cd ../poc/server
go run main.go

# Or CoAP/UDP baseline
cd ../poc/udp-server
go run main.go
```

### 2. Run Benchmark

```bash
# Build the benchmark tool
go build

# Run benchmark against all transports
./benchmark --server localhost:5683 --transport all

# Or test a specific transport
./benchmark --server localhost:5683 --transport quic-stream
./benchmark --server localhost:5683 --transport quic-datagram
./benchmark --server localhost:5683 --transport udp
```

## Command-Line Options

```
-server string
    Server address (default "localhost:5683")

-transport string
    Transport to test: quic-stream, quic-datagram, udp, or all (default "all")

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
| transport | quic-stream, quic-datagram, or udp |
| operation | GET, POST, PUT |
| path | Resource path |
| metric_type | latency or bytes |
| value | Measurement value (ms for latency, bytes for bytes) |
| payload_size | Size of request payload |
| success | true/false |
| error | Error message if failed |

## Analysis Tips

### Comparing Transports

1. **Latency Comparison**:
   ```bash
   # Filter CSV for latency metrics
   grep "latency" results/benchmark-*.csv | grep "GET"
   ```

2. **Protocol Overhead**:
   - Compare `Total bytes` vs `payload_size` × `count`
   - UDP typically has lowest overhead for small payloads
   - QUIC has higher initial overhead but better for large payloads

3. **Reliability vs Speed**:
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
- [ ] Support for 0-RTT connection resumption testing
- [ ] Connection migration testing for QUIC
- [ ] DTLS support for encrypted UDP baseline
- [ ] Prometheus metrics export
- [ ] Real-time charts and visualization

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
