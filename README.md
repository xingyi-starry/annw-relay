# XingyiStarry MP Relay

TCP relay and public room directory for XingyiStarry MP. The relay keeps rooms in memory and forwards protobuf envelopes; it does not simulate games or retain authority logs and snapshots.

The envelope has named protobuf `oneof` branches. The relay handles relay-control
branches and validates game envelopes only for connection identity, room, and
delivery direction. It does not interpret game rules. The game protocol version
lives in `Hello.protocol_version` and is checked by the host, not by the relay.

Playing rooms may remain joinable while they report available seats. A dropped
client identity can be rebound to a new physical connection for 15 seconds when
the room and match IDs agree; active connections cannot be replaced. Explicit
leave skips this grace period.

## Build and test

```powershell
go test ./...
go vet ./...
```

The server is built from source on its target machine with `go build`. No local
cross-build script or `dist` output is maintained.

The relay uses logrus v1.10.2. It writes single-line UTC
events to standard output with connection, room, client, match, and request IDs
when available. Heartbeats and forwarded game payloads are not logged at Info;
passwords and payload bytes are never included. Use `go test -race ./...` on a
platform with a working race-enabled Go toolchain before release.

Each connection has a bounded Go channel send queue, configured by
`write_queue_messages`. Its payload remains a `WireEnvelope`; the small queue
item wrapper only marks an optional graceful close after that envelope. A full
queue applies normal channel backpressure. The relay does not pre-encode
messages, maintain a second byte counter, or turn a temporary full queue into a
protocol error. Encoding happens in the connection's single writer goroutine.

Transport state is explicit (`Open`, `Draining`, `Closed`). Connection contexts
stop blocked producers and I/O loops; graceful close lets the writer flush its
final envelope, while server shutdown cancels transports without creating
reconnect reservations or normal runtime peer notifications.

`generate.ps1` reads the canonical protocol directly from the sibling plugin
project at `../annw-mp/src/XingyiStarry.Mp.Protocol/Protos/multiplayer.proto`
and regenerates `internal/wire/multiplayer.pb.go`. The relay repository does not
keep a second `.proto` copy. Generated Go code is committed so a standalone
server clone does not need the plugin repository or `protoc` to build.
The plugin tool `tools/generate_authority_hash.py` regenerates both explicit field
hash encoders. Shared vectors in `internal/wire/testdata/hash-v21.json` check
compatibility with C#; the relay does not validate gameplay hashes when routing.

## Run

Copy `relay.example.json` to `relay.json`, adjust limits, then run:

```sh
go build -trimpath -o ~/.local/bin/xingyistarry-mp-relay ./cmd/mprelay
~/.local/bin/xingyistarry-mp-relay -config ./relay.json
```

Rooms and identities are intentionally lost on restart. Public transport is plain TCP; terminate it behind a secured TCP proxy if transport confidentiality is required.

`deploy/xingyistarry-mp-relay.service` is the optional systemd user-service
definition used on the gateway. It expects this repository at
`~/code/annw-relay`, the binary at
`~/.local/bin/xingyistarry-mp-relay`, and configuration in the repository's
ignored `relay.json`.
