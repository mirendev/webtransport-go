# Miren compatibility fork

This fork carries one opt-in client patch on webtransport-go v0.13.0. Miren
coordinators, runners, and CLIs can be upgraded independently, so a new client
must still be able to establish a WebTransport session with a v0.9 server.

Set `Transport.AllowLegacyDraft06` to allow the older handshake. Before sending
CONNECT, the client checks the server's HTTP/3 settings. If the modern setting
is absent and the draft-06 setting is enabled, it sends the old protocol name
and draft header and accepts a peer without partial stream resets. A modern
advertisement always takes precedence, including an explicitly disabled one.
The default behavior is unchanged. There is no reconnect or RPC replay.

Legacy sessions retain ordinary QUIC stream-reset semantics: a cancellation can
still discard a stream header. Modern peers keep the partial-reset requirement.
This bridge is intended for Miren's single-session connections without
WebTransport session flow control or application-protocol negotiation. It does
not add graceful shutdown; Miren handles RPC drain separately.

Remove the fork and the opt-in together once supported coordinators and runners
no longer include v0.9 servers. Old clients already work with v0.13 servers, so
retiring old CLIs is not a prerequisite.

`go test -race ./...` includes handshake selection and rejection tests. Runtime's
`hack/test-rpc-compat` additionally builds separate old/new RPC peers and exercises
the generated watch and exec APIs, including callbacks, cancellation, forwarding,
and simulated packet loss and reordering.
