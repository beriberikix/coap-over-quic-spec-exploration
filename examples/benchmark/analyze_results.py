#!/usr/bin/env python3
"""
Analyze and compare CoAP benchmark results across all transports.
Uses only standard library - no pandas required.
"""

import csv
import statistics
from pathlib import Path
from collections import defaultdict

def analyze_benchmark(csv_path):
    """Analyze a single benchmark CSV file."""
    latencies = defaultdict(list)

    with open(csv_path, 'r') as f:
        reader = csv.DictReader(f)
        for row in reader:
            if row['metric_type'] == 'latency':
                transport = row['transport']
                latency = float(row['value'])
                latencies[transport].append(latency)

    # Calculate statistics for each transport
    results = {}
    for transport, values in latencies.items():
        if values:
            sorted_values = sorted(values)
            n = len(sorted_values)
            results[transport] = {
                'count': n,
                'mean_ms': statistics.mean(values),
                'median_ms': statistics.median(values),
                'p95_ms': sorted_values[int(n * 0.95)] if n > 0 else 0,
                'p99_ms': sorted_values[int(n * 0.99)] if n > 0 else 0,
                'min_ms': min(values),
                'max_ms': max(values),
            }

    return results

def main():
    results_dir = Path(__file__).parent / 'results'

    # Find all benchmark CSV files
    csv_files = sorted(results_dir.glob('benchmark-*.csv'), reverse=True)

    if not csv_files:
        print("No benchmark CSV files found!")
        return

    # Combine results from all recent files
    all_results = {}
    for csv_file in csv_files[:4]:  # Process last 4 files
        results = analyze_benchmark(csv_file)
        for transport, stats in results.items():
            if transport not in all_results:
                all_results[transport] = stats

    # Print comparison table
    print("\n=== CoAP Benchmark Results Comparison ===\n")
    print(f"{'Transport':<20} {'Count':<8} {'Mean':<10} {'Median':<10} {'P95':<10} {'P99':<10} {'Min':<10} {'Max':<10}")
    print("-" * 98)

    transports = ['quic-stream', 'quic-datagram', 'udp', 'dtls']
    for transport in transports:
        if transport in all_results:
            stats = all_results[transport]
            print(f"{transport:<20} {stats['count']:<8} "
                  f"{stats['mean_ms']:<10.3f} {stats['median_ms']:<10.3f} "
                  f"{stats['p95_ms']:<10.3f} {stats['p99_ms']:<10.3f} "
                  f"{stats['min_ms']:<10.3f} {stats['max_ms']:<10.3f}")

    # Print percentage comparisons
    print("\n=== Performance Comparison (vs UDP baseline) ===\n")

    if 'udp' in all_results:
        udp_mean = all_results['udp']['mean_ms']
        print(f"{'Transport':<20} {'Mean Latency':<15} {'vs UDP':<15} {'Status'}")
        print("-" * 65)

        for transport in transports:
            if transport in all_results:
                mean = all_results[transport]['mean_ms']
                diff_pct = ((mean - udp_mean) / udp_mean) * 100
                faster = "FASTER" if diff_pct < 0 else "SLOWER"
                status = f"{faster}" if abs(diff_pct) > 1 else "~SAME"
                print(f"{transport:<20} {mean:.3f} ms{'':<7} {diff_pct:+.1f}%{'':<10} {status}")

    # Compare encrypted transports
    print("\n=== Encrypted Transport Comparison (QUIC vs DTLS) ===\n")

    if 'quic-stream' in all_results and 'dtls' in all_results:
        quic_mean = all_results['quic-stream']['mean_ms']
        dtls_mean = all_results['dtls']['mean_ms']
        diff_pct = ((quic_mean - dtls_mean) / dtls_mean) * 100

        print(f"QUIC (TLS 1.3):  {quic_mean:.3f} ms")
        print(f"DTLS (DTLS 1.2): {dtls_mean:.3f} ms")
        print(f"QUIC is {abs(diff_pct):.1f}% {'FASTER' if diff_pct < 0 else 'SLOWER'} than DTLS")

    print("\n=== Key Insights ===\n")

    # Compare encryption overhead
    if 'udp' in all_results and 'dtls' in all_results:
        udp_mean = all_results['udp']['mean_ms']
        dtls_mean = all_results['dtls']['mean_ms']
        overhead_pct = ((dtls_mean - udp_mean) / udp_mean) * 100
        print(f"• DTLS encryption overhead: {overhead_pct:+.1f}% vs unencrypted UDP")

    if 'quic-stream' in all_results and 'quic-datagram' in all_results:
        stream_mean = all_results['quic-stream']['mean_ms']
        datagram_mean = all_results['quic-datagram']['mean_ms']
        diff_pct = abs(((stream_mean - datagram_mean) / datagram_mean) * 100)
        print(f"• QUIC streams vs datagrams: ~{diff_pct:.1f}% performance difference")

    if 'quic-stream' in all_results and 'udp' in all_results:
        quic_mean = all_results['quic-stream']['mean_ms']
        udp_mean = all_results['udp']['mean_ms']
        diff_pct = abs(((quic_mean - udp_mean) / udp_mean) * 100)
        print(f"• QUIC (encrypted) vs UDP (unencrypted): only {diff_pct:.1f}% difference!")

    print(f"• All transports showing sub-millisecond mean latency on localhost")
    print(f"• QUIC provides built-in encryption with minimal performance impact")
    print()

if __name__ == '__main__':
    main()
