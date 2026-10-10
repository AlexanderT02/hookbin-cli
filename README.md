# hookbin-cli

The Hookbin CLI connects to a bin and forwards incoming webhooks to a local HTTP endpoint. It also reports the local response so you can inspect the full request and forwarding result in Hookbin.

## Quick start

Build the CLI with Go 1.22 or later:

```sh
go build -o hookbin ./cmd/hookbin
```

Set the bin secret and start forwarding requests to your local app:

```sh
export HOOKBIN_SECRET='<secret shown in the inspector>'
./hookbin listen <slug> --forward-to http://localhost:3000/webhook
```

The CLI connects to `https://hookbin.madebyflow.de` by default. For a local Hookbin server, pass `--server http://localhost:8080`.

On PowerShell, set the secret with `$env:HOOKBIN_SECRET = '<secret>'` and run `./hookbin.exe listen <slug> --forward-to http://localhost:3000/webhook`.

## Development

```sh
go test ./...
go run ./cmd/hookbin listen <slug> --secret <secret> --forward-to http://localhost:3000/webhook
```

The CLI reconnects after transient connection failures and exits when the bin expires. See [PROTOCOL.md](PROTOCOL.md) for the agent protocol and fixtures.

## Releases

Push a version tag such as `v0.1.0` to run the release workflow. It tests the source, builds archives for Linux, macOS, and Windows, and publishes the archives with SHA-256 checksums as a GitHub Release.

```sh
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```
