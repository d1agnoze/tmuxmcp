# tmuxmcp

`tmuxmcp` is a small Linux-first prototype for sharing one tmux pane with a coding agent through MCP.

## Components

- `tmuxmcpd`: Go daemon that exposes:
  - MCP over `stdio` using the official `github.com/modelcontextprotocol/go-sdk`
  - local HTTP control API on `127.0.0.1`
- `tmuxmcp`: tmux popup client built with Bubble Tea that lists panes in a table, previews the highlighted pane, and selects which pane is shared

## v1 Scope

- one shared pane at a time
- read-only MCP tools
- plain-text pane snapshots
- no persistence across restarts
- Linux first

## Build

```bash
go build ./...
```

## Install

Install both binaries into your Go bin directory:

```bash
go install ./cmd/tmuxmcpd ./cmd/tmuxmcp
```

Make sure `$(go env GOPATH)/bin` or `$(go env GOBIN)` is on your `PATH`.

## tmux Setup

Add a binding like this to `~/.tmux.conf`:

```tmux
bind-key s popup -w 90% -h 85% -E 'go run ./cmd/tmuxmcp --server http://127.0.0.1:46321'
```

Reload tmux after saving:

```bash
tmux source-file ~/.tmux.conf
```

If you prefer installed binaries instead of `go run`, use:

```tmux
bind-key s popup -w 90% -h 85% -E 'tmuxmcp --server http://127.0.0.1:46321'
```

## Verify

```bash
go test ./... && go build ./...
```

Focused checks:

```bash
go test ./internal/config
go test ./internal/httpapi
go test ./internal/mcp
go test -cover ./...
```

## Run the server

```bash
go run ./cmd/tmuxmcpd --listen 127.0.0.1:46321 --history-lines 500 --log-file tmuxmcpd.log
```

Flags:

- `--listen`: local HTTP control address
- `--history-lines`: MCP pane snapshot line limit, valid range `500..2000`
- `--log-file`: daemon log path, default `tmuxmcpd.log`

## Run the popup client

Inside tmux:

```bash
tmux popup -w 90% -h 85% -E 'go run ./cmd/tmuxmcp --server http://127.0.0.1:46321'
```

Flags:

- `--server`: local HTTP base URL for `tmuxmcpd`
- `--preview-lines`: preview line count per pane, default `8`

The client:

- lists panes across tmux sessions in a scrollable table
- shows a live ANSI-colored preview for the highlighted pane in a panel below the table, with pane metadata kept above the scrollable viewport
- opens the preview at the bottom of the captured pane content so older lines are reached by scrolling up
- marks the currently shared pane with `*`
- lets you switch focus with `Tab`, move the table with arrow keys or `j`/`k`, scroll the preview with the same keys or the mouse wheel when it is hovered or focused, share with `Enter`, unshare with `u`, or quit with `q`

## HTTP control API

### `GET /active-pane`

Returns whether a pane is currently shared.

### `POST /active-pane`

Request body:

```json
{ "pane_id": "%3" }
```

Sets the shared pane if that pane exists.

### `DELETE /active-pane`

Clears the shared pane.

## MCP tools

During MCP initialization, the server also advertises server-level instructions that tell clients to use `get_active_pane` to confirm a pane is shared and `read_active_pane` to inspect the latest shared terminal output during debugging.

### `get_active_pane`

Checks whether a pane is currently shared and returns its pane id plus selection time, or a clear message if no pane is shared.

Advertised tool metadata:

- title: `Get Active Shared Pane`
- description: explicitly frames this as the check for whether a shared pane exists
- annotations: read-only, idempotent, non-destructive, closed-world

### `read_active_pane`

Reads the latest plain-text output from the shared pane. This is intended for things like live logs, command output, test failures, or the current terminal screen while debugging. If the pane disappeared, the server clears the selection and returns a clear message.

Advertised tool metadata:

- title: `Read Shared Pane Output`
- description: explicitly frames this as the entry point for logs, command output, test failures, and live terminal state
- annotations: read-only, idempotent, non-destructive, closed-world

Typical host/client behavior:

- call `get_active_pane` first to confirm that a pane is currently shared
- call `read_active_pane` when the user asks to inspect logs, terminal output, or the current state of the shared session

## MCP metadata

The server exposes the following MCP metadata to connected clients during initialization:

- server name: `tmuxmcpd`
- server title: `tmuxmcp Shared Pane Server`
- server version: `0.1.0`
- server instructions: describe that the server is for inspecting one user-selected tmux pane, that clients should usually call `get_active_pane` before `read_active_pane`, and that MCP access is read-only and the shared selection is in-memory only

This matters because many MCP hosts use server instructions plus each tool's name, description, schema, title, and annotations to decide when a tool is relevant.

## MCP Server Setup

`tmuxmcpd` is a stdio MCP server. Start it from your MCP host by pointing the host at the `tmuxmcpd` binary.

1. Install the binaries:

```bash
go install ./cmd/tmuxmcpd ./cmd/tmuxmcp
```

2. Confirm the binary is available:

```bash
tmuxmcpd --help
```

3. Add an MCP server entry in your host that runs:

```bash
tmuxmcpd --listen 127.0.0.1:46321 --history-lines 500 --log-file tmuxmcpd.log
```

Example generic MCP config shape:

```json
{
  "mcpServers": {
    "tmuxmcpd": {
      "command": "tmuxmcpd",
      "args": [
        "--listen",
        "127.0.0.1:46321",
        "--history-lines",
        "500",
        "--log-file",
        "tmuxmcpd.log"
      ]
    }
  }
}
```

If your MCP host does not inherit your shell `PATH`, replace `tmuxmcpd` with its absolute path.

4. Start or reload your MCP host.

5. Inside tmux, open the popup client and share a pane before asking the host to inspect terminal output.

Typical MCP flow:

- call `get_active_pane` first
- call `read_active_pane` after a pane has been shared

## tmux keybinding example

Add something like this to your tmux config after installing the binaries on your `PATH`:

```tmux
bind-key s popup -w 90% -h 85% -E 'tmuxmcp --server http://127.0.0.1:46321'
```

## Manual Testing

1. Start the daemon:

```bash
go run ./cmd/tmuxmcpd --listen 127.0.0.1:46321 --history-lines 500 --log-file tmuxmcpd.log
```

2. Inside tmux, open a few panes with visible output.

3. Launch the popup client:

```bash
tmux popup -w 90% -h 85% -E 'go run ./cmd/tmuxmcp --server http://127.0.0.1:46321'
```

4. Verify the popup can:

- scroll through the pane table
- update the preview when the highlighted row changes
- keep preview metadata fixed above the scrollable preview body
- open the preview at the latest lines and let you scroll upward for older output
- mark the currently shared pane with `*`
- select the highlighted pane to share with `Enter`
- clear the shared pane with `u`

5. Verify the local HTTP control API:

```bash
curl http://127.0.0.1:46321/healthz
curl http://127.0.0.1:46321/active-pane
curl -X POST http://127.0.0.1:46321/active-pane -H 'Content-Type: application/json' -d '{"pane_id":"%3"}'
curl -X DELETE http://127.0.0.1:46321/active-pane
```

6. Verify MCP from a host/client by calling `get_active_pane` and `read_active_pane` against the stdio server, and confirm that `read_active_pane` returns the current shared pane output.

7. Inspect daemon logs if anything fails:

```bash
less tmuxmcpd.log
```

## Notes

- The popup client talks to the local HTTP server only.
- The popup UI uses `github.com/charmbracelet/bubbletea` with `bubbles/table` and `bubbles/viewport`.
- Popup previews preserve ANSI styling from tmux but strip non-styling control sequences before rendering.
- MCP traffic uses `stdio` only in v1.
- MCP transport behavior comes from the official Go SDK `StdioTransport`, which uses newline-delimited JSON on stdin/stdout.
- `read_active_pane` is the MCP entry point for inspecting the current shared terminal output, including logs and command results.
- The server also publishes initialize-time instructions so MCP hosts can infer that `get_active_pane` is the existence check and `read_active_pane` is the log/output reader.
- `tmuxmcpd` logs to a file so stdout stays reserved for MCP traffic. The default log path is `tmuxmcpd.log`.
- No localhost auth is implemented in v1.
