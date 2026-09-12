# XingyiStarry MP Relay

TCP relay and public room directory for XingyiStarry MP. The relay keeps rooms in memory and forwards protobuf envelopes; it does not simulate games or retain authority logs and snapshots.

## Build and test

```powershell
go test ./...
go vet ./...
```

The server is built from source on its target machine with `go build`. No local
cross-build script or `dist` output is maintained.

`generate.ps1` reads the canonical protocol directly from the sibling plugin
project at `../xingyistarry-mp/src/XingyiStarry.Mp.Protocol/Protos/multiplayer.proto`
and regenerates `internal/wire/multiplayer.pb.go`. The relay repository does not
keep a second `.proto` copy. Generated Go code is committed so a standalone
server clone does not need the plugin repository or `protoc` to build.

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
