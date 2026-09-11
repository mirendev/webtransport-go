# Miren v0.9 maintenance patch

This branch starts at upstream v0.9.0. It removes closed WebTransport sessions
from the session registry so their HTTP/3 connections and buffers can be
collected. It changes no protocol, public API, or dependency versions.

Miren Runtime v0.15 also closes each streaming RPC's dedicated QUIC connection
when its session ends, and removes closed connections from client diagnostics.
Those runtime fixes need this registry cleanup to stop retained heap growth.
Tracked in MIR-1820.

This branch is only for the v0.15 patch release. Runtime main already uses the
newer transport on the separate miren-v0.13 branch.
