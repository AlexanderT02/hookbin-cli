# Hookbin agent protocol v1

The CLI connects to `ws(s)://<server>/ws/agent?slug=<slug>` with the HTTP headers
`Authorization: Bearer <secret>` and `Hookbin-Protocol: 1`. The server rejects an
unknown slug with 404, an expired bin with 410, a wrong secret with 401, and an
unsupported protocol with 426. All WebSocket messages are JSON text frames.

## Server to CLI

The first message is `hello` with `binSlug`, `serverVersion`,
`protocolVersion`, and `expiresAt` (UTC ISO 8601). The server then sends:

- `request`: `attemptId`, `requestId`, `replay`, `method`, `path` (starting with
  `/`), `query` (without `?`), `headers` (names to arrays of values), and
  `bodyBase64` (empty string for no body).
- `ping`: expects a `pong` within 10 seconds. Sent every 30 seconds.
- `expired`: the bin has expired; the CLI exits without reconnecting.

Every delivery, including a replay of the same captured request, receives a
new `attemptId`. This lets the server match each result to the right delivery
when several CLIs are connected or requests are replayed concurrently.

## CLI to server

After `hello`, the CLI sends `ready` with `cliVersion` and `targetUrl`.
For each `request`, it sends exactly one of:

- `response`: `attemptId`, `requestId`, `status`, `headers`, `bodyBase64`,
  `durationMs`.
- `error`: `attemptId`, `requestId`, `message`, `durationMs`.

It also sends `pong` after each `ping`. The server accepts results only for an
outstanding `attemptId` belonging to that WebSocket session. Unknown fields
and unknown message types are ignored on both sides.

The CLI forwards the original method, path, query, headers, and decoded body
to the configured target. Hop-by-hop headers are removed, while `Host` and
`Content-Length` are set by the local HTTP client. The local timeout defaults
to 10 seconds; at most 256 KB of each local response body is reported.
The CLI reconnects with exponential backoff (1 to 30 seconds) after a transient
connection loss, but exits on invalid credentials, unsupported protocol, an
unknown bin, or expiry.

The examples in `testdata/protocol/` are contract fixtures for both the CLI
and server repositories. Adding required fields or changing existing meanings
requires a new protocol version.
