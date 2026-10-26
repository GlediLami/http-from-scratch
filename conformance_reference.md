# RFC 9112 & RFC 9110 Conformance Reference: `parser.go`

This document maps all implemented RFC 9112 and RFC 9110 requirements to their corresponding methods and logic in `parser.go`.

---

## Conformance Matrix

| RFC Rule | Parser Feature | Status | Implementation Target |
| --- | --- | --- | --- |
| [RFC 9112 §2.2](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-2.2) | Bare `\r` (CR) rejection | Verified | `readLine()` |
| [RFC 9112 §2.2](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-2.2) | Ignore leading empty lines | Verified | `parse()` (`stateRequestLine`) |
| [RFC 9112 §3.0](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-3) | Strict single `SP` delimiter check | Verified | `parseRequestLine()` |
| [RFC 9112 §5.1](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-5.1) | Reject whitespace before colon (`field-name :`) | Verified | `parseFieldLine()` |
| [RFC 9112 §5.2](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-5.2) | Reject obsolete line folding (`obs-fold`) | Verified | `parseFieldLine()` |
| [RFC 9112 §5.5](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-5.5) | Strip leading/trailing `OWS` | Verified | `parseFieldLine()` via `bytes.Trim` |
| [RFC 9112 §3.2.1](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-3.2.1) | Mandatory single `Host` header for HTTP/1.1 | Verified | `setupBodyReader()` |
| [RFC 9112 §6.1](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-6.1) | Reject dual `Transfer-Encoding` + `Content-Length` | Verified | `setupBodyReader()` |
| [RFC 9112 §7.1](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-7.1) | Chunked transfer decoder + trailers handling | Verified | `chunkedReader.Read()` |
| [RFC 9110 §5.6.2](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9110%23section-5.6.2) | `tchar` byte verification for methods/headers | Verified | `validateToken()` / `isTChar()` |

---

## Detailed Rule Breakdowns

### 1. Bare CR Rejection

* **Specification:** [RFC 9112 §2.2: Message Parsing](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-2.2)
* **Requirement:** A recipient MUST parse lines terminated by `CRLF`. A recipient receiving a bare `CR` (not followed by `LF`) MUST reject the message with a `400 Bad Request`.
* **Method:** `readLine()`
* **Code Reference:**
```go
if bytes.IndexByte(line, '\r') != -1 {
    return nil, badRequest("bare CR character in input line is forbidden")
}

```



---

### 2. Ignore Leading Empty Lines

* **Specification:** [RFC 9112 §2.2: Message Parsing](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-2.2)
* **Requirement:** In the interest of robustness, a server that expects to receive a request-line on a persistent connection MAY ignore at least one empty line (`CRLF`) received prior to that request-line.
* **Method:** `parse()`
* **Code Reference:**
```go
case stateRequestLine:
    line, err := p.readLine()
    if err != nil {
        return nil, err
    }
    if len(line) == 0 {
        continue // Ignore leading empty line
    }

```



---

### 3. Strict Single Space (`SP`) Delimiter Check

* **Specification:** [RFC 9112 §3: Request Line](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-3)
* **Requirement:** `request-line = method SP request-target SP HTTP-version CRLF`. Senders MUST NOT send multiple spaces or tab characters (`HTAB`) between elements. A server receiving multiple spaces or tabs must reject the line with `400 Bad Request`.
* **Method:** `parseRequestLine()`
* **Code Reference:**
```go
parts := bytes.Split(line, []byte(" "))
if len(parts) != 3 || len(parts[0]) == 0 || len(parts[1]) == 0 || len(parts[2]) == 0 {
    return badRequest("malformed request-line: must contain exactly method, target, and version separated by single SP")
}

```



---

### 4. Whitespace Rejection Before Colon

* **Specification:** [RFC 9112 §5.1: Field Line Parsing](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-5.1)
* **Requirement:** No whitespace is allowed between the header field-name and colon. A server MUST reject any received request message that contains whitespace between a header field-name and colon with a `400 Bad Request` to prevent request smuggling.
* **Method:** `parseFieldLine()`
* **Code Reference:**
```go
fieldName := line[:colonIdx]
if fieldName[len(fieldName)-1] == ' ' || fieldName[len(fieldName)-1] == '\t' {
    return badRequest("whitespace before colon is prohibited (smuggling defense)")
}

```



---

### 5. Obsolete Line Folding (`obs-fold`) Rejection

* **Specification:** [RFC 9112 §5.2: Obsolete Line Folding](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-5.2)
* **Requirement:** Historically, headers could span multiple lines if continued lines began with `SP` or `HTAB`. A server that receives an `obs-fold` in a request message MUST reject the message with a `400 Bad Request`.
* **Method:** `parseFieldLine()`
* **Code Reference:**
```go
if line[0] == ' ' || line[0] == '\t' {
    return badRequest("obsolete line folding (obs-fold) is prohibited")
}

```



---

### 6. Strip Leading and Trailing Optional White Space (`OWS`)

* **Specification:** [RFC 9112 §5.5: Field Values](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-5.5)
* **Requirement:** `field-line = field-name ":" OWS field-value OWS`. Field values are surrounded by optional whitespace (`SP` or `HTAB`), which must be trimmed prior to value interpretation or storage.
* **Method:** `parseFieldLine()`
* **Code Reference:**
```go
fieldVal := bytes.Trim(line[colonIdx+1:], " \t")

```



---

### 7. Mandatory Single `Host` Header

* **Specification:** [RFC 9112 §3.2.1: Origin Form](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-3.2.1)
* **Requirement:** A client MUST send a `Host` header field in all HTTP/1.1 request messages. A server MUST respond with a `400 Bad Request` to any HTTP/1.1 request message that lacks a `Host` header field and to any request message that contains more than one `Host` header field.
* **Method:** `setupBodyReader()`
* **Code Reference:**
```go
if p.req.ProtoMajor == 1 && p.req.ProtoMinor >= 1 {
    if p.req.Host == "" || len(p.req.Header.Values("Host")) != 1 {
        return badRequest("HTTP/1.1 requests require exactly one Host header")
    }
}

```



---

### 8. Dual `Transfer-Encoding` + `Content-Length` Rejection

* **Specification:** [RFC 9112 §6.1: Transfer-Encoding](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-6.1)
* **Requirement:** A sender MUST NOT send a `Content-Length` header field in any message that contains a `Transfer-Encoding` header field. If received, the server MUST reject the message with a `400 Bad Request` to neutralize HTTP request smuggling vulnerabilities.
* **Method:** `setupBodyReader()`
* **Code Reference:**
```go
teHeaders := p.req.Header.Values("Transfer-Encoding")
clHeaders := p.req.Header.Values("Content-Length")

if len(teHeaders) > 0 && len(clHeaders) > 0 {
    return badRequest("request contains both Transfer-Encoding and Content-Length")
}

```



---

### 9. Chunked Transfer Decoding & Trailer Processing

* **Specification:** [RFC 9112 §7.1: Chunked Transfer Coding](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9112%23section-7.1)
* **Requirement:** Handles stream payloads encoded as `chunk-size [chunk-ext] CRLF chunk-data CRLF`, parsing hex chunk lengths, consuming trailing CRLFs, and closing on a zero-size terminal chunk (`0 CRLF [trailer-section] CRLF`).
* **Method:** `chunkedReader.Read()`
* **Code Reference:**
```go
size, err := strconv.ParseInt(string(line), 16, 64)
if err != nil || size < 0 {
    return 0, badRequest("invalid hexadecimal chunk size")
}
if size == 0 {
    cr.done = true
    // Consume trailer-section until final CRLF
    for {
        trailerLine, err := cr.r.ReadBytes('\n')
        ...
        if len(bytes.TrimRight(trailerLine, "\r\n")) == 0 { break }
    }
    return 0, io.EOF
}

```



---

### 10. `tchar` Byte Verification

* **Specification:** [RFC 9110 §5.6.2: Tokens](https://www.google.com/search?q=https://www.rfc-editor.org/rfc/rfc9110%23section-5.6.2)
* **Requirement:** Tokens (used in HTTP methods and field names) must contain 1 or more characters belonging exclusively to the `tchar` subset (`ALPHA`, `DIGIT`, or `!#\$%&'*+-.^_`|~`). Senders MUST NOT send unescaped delimiters, whitespace, or control characters.
* **Method:** `validateToken()`, `isTChar()`
* **Code Reference:**
```go
func (p *requestParser) isTChar(b byte) bool {
    return (b >= 'a' && b <= 'z') ||
        (b >= 'A' && b <= 'Z') ||
        (b >= '0' && b <= '9') ||
        bytes.IndexByte([]byte("!#$%&'*+-.^_`|~"), b) != -1
}

```