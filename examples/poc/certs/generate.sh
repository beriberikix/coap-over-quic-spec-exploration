#!/bin/bash
# Generate self-signed certificates for testing CoAP over QUIC

set -e

CERT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Generate private key
openssl ecparam -genkey -name prime256v1 -out "$CERT_DIR/server.key"

# Generate self-signed certificate
openssl req -new -x509 -key "$CERT_DIR/server.key" \
    -out "$CERT_DIR/server.crt" -days 365 \
    -subj "/C=US/ST=Test/L=Test/O=CoAP-QUIC-Test/CN=localhost"

echo "Certificates generated successfully in $CERT_DIR"
echo "  - server.key: Private key"
echo "  - server.crt: Certificate"
