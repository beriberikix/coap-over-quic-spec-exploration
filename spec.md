# **Constrained Application Protocol (CoAP) over QUIC**

## **Abstract**

The Constrained Application Protocol (CoAP) is a specialized web transfer protocol for use with constrained nodes and constrained networks in the Internet of Things. While CoAP is typically used over UDP and DTLS to provide a lightweight, RESTful interface, there is a growing need for a transport that offers reliability, multiplexing, and reduced connection setup overhead without the complexity of TCP.

This document specifies the use of the QUIC transport protocol to carry CoAP messages. QUIC provides a secure, reliable, stream-based transport with built-in multiplexing, flow control, and faster session establishment. By layering CoAP over QUIC, we gain the benefits of a modern transport protocol while retaining the simplicity and efficiency of the CoAP application layer. This mapping leverages the stream abstraction of QUIC to provide a more efficient and robust transport for CoAP, especially in environments with high packet loss or where multiple concurrent requests are common.

## **1\. Introduction**

The Constrained Application Protocol (CoAP) ([RFC7252](https://www.rfc-editor.org/info/rfc7252)) is designed for machine-to-machine (M2M) applications such as smart energy and building automation. It provides a request/response interaction model between application endpoints, supports built-in discovery of services and resources, and includes key concepts of the Web such as URIs and Internet media types.

CoAP is typically bound to UDP ([RFC0768](https://www.rfc-editor.org/info/rfc768)) and secured with DTLS ([RFC6347](https://www.rfc-editor.org/info/rfc6347)). This provides a very lightweight solution but lacks certain features of more robust transport protocols. [RFC8323](https://www.rfc-editor.org/info/rfc8323) specifies CoAP over TCP, TLS, and WebSockets, providing a reliable transport option.

QUIC ([RFC9000](https://www.rfc-editor.org/info/rfc9000)) is a modern transport protocol that provides stream-multiplexing, per-stream flow control, and low-latency connection establishment, all while being encrypted by default with TLS 1.3 ([RFC8446](https://www.rfc-editor.org/info/rfc8446)). These features make QUIC an attractive transport for CoAP, offering a middle ground between the minimalism of UDP and the overhead of TCP.

This document defines how CoAP can be transported over QUIC. It leverages QUIC streams to handle CoAP request/response exchanges, provides a mechanism for non-confirmable messages, and discusses the implications for constrained devices.

## **2\. Terminology**

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in BCP 14 ([RFC2119](https://www.rfc-editor.org/info/rfc2119)) ([RFC8174](https://www.rfc-editor.org/info/rfc8174)) when, and only when, they appear in all capitals, as shown here.

This document also uses the following terminology:

* **CoAP/QUIC**: CoAP over a QUIC transport.  
* **Endpoint**: A CoAP/QUIC endpoint is a host that can initiate or receive CoAP messages.  
* **QUIC Stream**: A unidirectional or bidirectional channel of ordered bytes within a QUIC connection.

## **3\. Protocol Overview**

CoAP over QUIC uses a single QUIC connection between two endpoints. Within this connection, multiple QUIC streams are used to carry CoAP request/response pairs. This avoids head-of-line blocking between independent CoAP requests.

The protocol operates in a client-server model. A CoAP client initiates a QUIC connection to a CoAP server. Once the connection is established, the client can send CoAP requests on QUIC streams.

```
+--------+                                  +--------+  
| Client |                                  | Server |  
+--------+                                  +--------+  
    |                                           |  
    |  QUIC Connection Establishment (TLS 1.3)  |  
    |------------------------------------------>|  
    |                                           |  
    |  Stream 1: CoAP Request (GET /temp)       |  
    |------------------------------------------>|  
    |                                           |  
    |  Stream 1: CoAP Response (2.05 Content)   |  
    |<------------------------------------------|  
    |                                           |  
    |  Stream 2: CoAP Request (PUT /led)        |  
    |------------------------------------------>|  
    |                                           |  
    |  Stream 2: CoAP Response (2.04 Changed)   |  
    |<------------------------------------------|  
    |                                           |  
    |  Stream 3 (Unidirectional): NON Message   |  
    |------------------------------------------>|  
    |                                           |
```

*Figure 1: Example CoAP/QUIC Exchange*

### **3.1. Mapping CoAP to QUIC Streams**

* A CoAP request/response pair SHOULD be sent over a single bidirectional QUIC stream. The client initiates the stream and sends the request, and the server sends the response on the same stream.  
* The client MUST indicate the end of its request on the stream by setting the FIN bit on the last QUIC STREAM frame containing the CoAP message.  
* The server MUST indicate the end of its response on the stream by setting the FIN bit on the last QUIC STREAM frame.  
* A client MAY open multiple concurrent streams to send multiple CoAP requests without waiting for responses to previous requests.  
* For Non-confirmable (NON) messages where no response is expected, a unidirectional QUIC stream initiated by the sender SHOULD be used.  
* Block-wise transfers ([RFC7959](https://www.rfc-editor.org/info/rfc7959)) are handled within a single stream. The inherent reliability and ordering of QUIC streams obviate the need for the block-wise transfer mechanisms at the CoAP layer. A large CoAP payload can be fragmented into multiple QUIC STREAM frames.

### **3.2. Protocol Stack**

The protocol stack for CoAP over QUIC is as follows:

```
   +----------------------+  
   |      Application     |  
   +----------------------+  
   |         CoAP         |  
   +----------------------+  
   |         QUIC         |  
   +----------------------+  
   |          UDP         |  
   +----------------------+  
   |          IP          |  
   +----------------------+
```

*Figure 2: CoAP/QUIC Protocol Stack*

QUIC itself is encrypted using TLS 1.3, providing a secure channel for CoAP messages.

## **4\. The `coaps+quic` URI Scheme**

This specification defines a new URI scheme, coaps+quic, to identify CoAP resources that are available over a QUIC transport.

The syntax of the coaps+quic URI is defined as follows, following the ABNF syntax of [RFC3986](https://www.rfc-editor.org/info/rfc3986):

`coaps+quic-URI = "coaps+quic:" "//" host [ ":" port ] path-abnf`

* `host`: The IP address or registered name of the CoAP server.  
* `port`: The UDP port number of the server. If omitted, the default port for CoAP over QUIC is TBD1.

For example: `coaps+quic://[2001:db8::1]:5683/sensors/temperature`

The `coaps` prefix is used to indicate that the communication is secured, which is inherent with QUIC. The \+quic suffix denotes the use of QUIC as the transport.

## **5\. CoAP Messaging over QUIC**

### **5.1. Message Types and Reliability**

CoAP defines four message types: Confirmable (CON), Non-confirmable (NON), Acknowledgement (ACK), and Reset (RST).

With QUIC as the transport, the reliability is handled at the transport layer. Therefore, the CoAP message layer reliability is redundant.

* **Confirmable (CON) Messages**: The concept of a CON message is replaced by the reliable delivery of the QUIC stream. A CoAP request sent on a bidirectional stream is implicitly "confirmable" in that its delivery is guaranteed by QUIC. A successful response from the server serves as an application-level acknowledgment. If a response is not received within a configured time, the client MAY assume the request failed and close the stream.  
* **Acknowledgement (ACK) Messages**: ACK messages are not used in CoAP/QUIC. The QUIC transport provides its own acknowledgment mechanisms. A CoAP response message itself serves as the acknowledgment of the request. Piggybacked responses are the default model. Separate responses (empty ACKs followed by a CON response) are not used.  
* **Reset (RST) Messages**: If a server receives a CoAP message that it cannot process (e.g., malformed), it SHOULD close the QUIC stream with an application-specific error code. The `STOP\_SENDING` and `RESET\_STREAM` frames in QUIC can be used for this purpose.

### **5.2. Request/Response Model**

A CoAP request is sent on a new or existing bidirectional QUIC stream.

```
      Client                                      Server  
        |                                             |  
        |  (QUIC connection established)              |  
        |                                             |  
        |----------- STREAM (ID=0, FIN=1) ----------->|  
        |  CoAP Header (GET, T=CON, Code=0.01, ...)   |  
        |  Token                                      |  
        |  Options (Uri-Path: "temp")                 |  
        |                                             |  
        |                                             |  
        |  <---------- STREAM (ID=0, FIN=1) ----------|  
        |  CoAP Header (2.05 Content, T=ACK, ...)     |  
        |  Token (same)                               |  
        |  Options (Content-Format: app/json)         |  
        |  Payload: {"value": 22.5}                   |  
        |                                             |
```

*Figure 3: Request/Response Flow on a QUIC Stream*

The CoAP Message ID, used for matching CON/ACK/RST messages in CoAP over UDP, is still present in the header but its significance is reduced. It can be used for application-level tracking but is not needed for transport reliability. Implementations MAY set it to a fixed value (e.g., zero) or use it as a sequence number within the stream.

The CoAP Token is used to match a request and response and MUST be used as specified in [RFC7252](https://www.rfc-editor.org/info/rfc7252).

### **5.3. Non-confirmable (NON) Messages**

For applications that require sending data without requiring a direct response, such as periodic sensor readings, CoAP NON messages are useful. In CoAP/QUIC, these can be mapped to unidirectional QUIC streams. This is analogous to the concept of "unreliable datagrams" in [RFC9221](https://www.rfc-editor.org/info/rfc9221).

To send a NON message, a client or server initiates a unidirectional stream and sends the CoAP message on it. The stream is closed with a FIN bit once the message is sent.

```
      Client                                     Server  
        |                                            |  
        |  (QUIC connection established)             |  
        |                                            |  
        |----------- STREAM (ID=2, UNI, FIN=1) ----->|  
        |  CoAP Header (POST, T=NON, Code=0.02, ...) |  
        |  Token                                     |  
        |  Options (Uri-Path: "log")                 |  
        |  Payload: "Device event occurred"          |  
        |                                            |
```

*Figure 4: NON Message on a Unidirectional Stream*

While QUIC streams are reliable, the use of a unidirectional stream signals to the application that no response is expected. If the application requires an unreliable transport, QUIC Datagrams ([RFC9221](https://www.rfc-editor.org/info/rfc9221)) can be used. When using QUIC Datagrams, the CoAP message is sent as the payload of a DATAGRAM frame. This provides a true non-confirmable, unreliable service similar to CoAP over UDP. The choice between unidirectional streams and datagrams depends on the application's tolerance for data loss.

### **5.4. Message Format**

The CoAP message format as defined in [RFC7252](https://www.rfc-editor.org/info/rfc7252) is used without modification. The entire CoAP message, including the header, token, options, and payload, is sent as the data on a QUIC stream.

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

*Figure 5: CoAP Message Format*

The message is then placed into one or more QUIC STREAM frames.

```
   +-----------------+  
   |  QUIC STREAM    |  
   |  Frame Header   |  
   +-----------------+  
   |                 |  
   ~  CoAP Message   ~  
   |                 |  
   +-----------------+
```

*Figure 6: CoAP Message in a QUIC Stream Frame*

## **6\. Features of CoAP over UDP Not Present in CoAP over QUIC**

* **Multicast**: CoAP multicast support is not applicable to CoAP/QUIC, as QUIC is a unicast, connection-oriented protocol.  
* **Message Deduplication**: The Message ID based deduplication in CoAP is not needed, as QUIC provides exactly-once delivery of stream data.  
* **Congestion Control**: CoAP's simple exponential backoff congestion control is replaced by QUIC's more sophisticated mechanisms (e.g., NewReno, CUBIC).

## **7\. Resource Discovery**

Resource discovery is performed as described in Section 7 of [RFC7252](https://www.rfc-editor.org/info/rfc7252). A client sends a `GET` request to `/.well-known/core` to retrieve a list of resources from the server. This request is sent over a bidirectional QUIC stream like any other request.

## **8\. Performance and Energy Considerations**

### **8.1. Benefits**

* **Reduced Connection Overhead**: QUIC supports 0-RTT and 1-RTT connection establishment, which is significantly faster than the multi-roundtrip handshakes of DTLS over UDP or TCP with TLS. This is highly beneficial for sleepy devices that need to connect, send data, and disconnect quickly.  
* **No Head-of-Line Blocking**: Multiple CoAP requests can be sent concurrently over different streams. If one request is delayed (e.g., waiting for a slow sensor reading on the server), it does not block other requests.  
* **Improved Congestion Control**: QUIC's advanced congestion control can lead to better network utilization and performance, especially in lossy environments like wireless networks.  
* **Connection Migration**: QUIC's ability to migrate connections across IP addresses is a major advantage for mobile IoT devices that may switch between cellular and Wi-Fi networks.

### **8.2. Tradeoffs**

* **Increased Complexity**: A full QUIC implementation is more complex and may have a larger code size and memory footprint than a simple CoAP/UDP/DTLS stack. This may be a concern for the most constrained devices (e.g., with less than 100KB of RAM). However, optimized implementations for embedded systems are emerging.  
* **Stateful Connections**: QUIC is connection-oriented, meaning both client and server must maintain connection state. This contrasts with the largely stateless nature of CoAP over UDP, which can be simpler for some server architectures.  
* **Processing Overhead**: The stream management and more complex congestion control of QUIC may incur a higher CPU overhead per-packet compared to UDP.

### **8.3. Comparison with HTTP/3**

HTTP/3 presents a potential alternative application mapping for QUIC in IoT. The primary tradeoff between CoAP/QUIC and HTTP/3 lies in application-layer overhead and complexity.

CoAP is designed for minimalism. Its header is a compact binary format with a predictable and consistently low overhead, making it well-suited for environments where every byte is critical.

HTTP/3 uses QPACK for header compression. This mechanism is highly efficient for subsequent requests within the same connection, as it can replace previously seen header fields with short indexes from a dynamic table. However, the first request on a "cold" connection bears a higher cost, as header fields must be transmitted as string literals.

This leads to a clear performance distinction:

* CoAP/QUIC is optimized for use cases involving sleepy devices or infrequent transmissions, where the low, consistent header overhead is paramount.  
* HTTP/3 is better suited for devices with long-lived connections that can amortize the initial header compression cost over many requests, and for applications that benefit from direct integration with the broader web ecosystem.

Furthermore, CoAP provides IoT-native features like Observe ([RFC7641](https://www.rfc-editor.org/info/rfc7641)) for pub/sub interactions, which do not have a direct equivalent in the core HTTP/3 protocol.

## **9\. Security Considerations**

All CoAP messages exchanged over QUIC are protected by the mechanisms provided by QUIC, which incorporates TLS 1.3. This provides:

* **Confidentiality and Integrity**: All CoAP data is encrypted and authenticated.  
* **Peer Authentication**: The client and server can authenticate each other using X.509 certificates or pre-shared keys (PSKs).

Since QUIC encrypts most of its headers, it offers better privacy protection against passive observers than TCP with TLS.

The security considerations for CoAP ([RFC7252](https://www.rfc-editor.org/info/rfc7252)) and QUIC ([RFC9000](https://www.rfc-editor.org/info/rfc9000)) apply. Implementers MUST follow the security best practices for both protocols.

Application-layer security mechanisms for CoAP, such as OSCORE ([RFC8613](https://www.rfc-editor.org/info/rfc8613)), can still be used over CoAP/QUIC to provide end-to-end security, which may be desirable in scenarios involving untrusted intermediaries or proxies.

## **10\. IANA Considerations**

This document requests IANA to register the `coaps+quic` URI scheme in the "Uniform Resource Identifier (URI) Schemes" registry.

This document also requests IANA to assign a default port number for CoAP over QUIC in the "Service Name and Transport Protocol Port Number Registry". The suggested port is TBD1.

## **11\. References**

### **11.1. Normative References**

* [RFC2119](https://www.rfc-editor.org/info/rfc2119)  
* [RFC7252](https://www.rfc-editor.org/info/rfc7252)  
* [RFC7641](https://www.rfc-editor.org/info/rfc7641)  
* [RFC8174](https://www.rfc-editor.org/info/rfc8174)  
* [RFC9000](https://www.rfc-editor.org/info/rfc9000)  
* [RFC9221](https://www.rfc-editor.org/info/rfc9221)

### **11.2. Informative References**

* [RFC0768](https://www.rfc-editor.org/info/rfc768)  
* [RFC3986](https://www.rfc-editor.org/info/rfc3986)  
* [RFC6347](https://www.rfc-editor.org/info/rfc6347)  
* [RFC7959](https://www.rfc-editor.org/info/rfc7959)  
* [RFC8323](https://www.rfc-editor.org/info/rfc8323)  
* [RFC8446](https://www.rfc-editor.org/info/rfc8446)  
* [RFC8613](https://www.rfc-editor.org/info/rfc8613)
