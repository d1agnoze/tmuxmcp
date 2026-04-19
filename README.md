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
tmux popup -E 'tmuxmcp --server http://127.0.0.1:46321'
```

Flags:

- `--server`: local HTTP base URL for `tmuxmcpd`
- `--preview-lines`: preview line count per pane, default `8`

The client:

- lists panes across tmux sessions in a scrollable table
- shows a live ANSI-colored preview for the highlighted pane in a panel below the table, with pane metadata kept above the scrollable viewport
- marks the currently shared pane with `*`
- lets you switch focus with `Tab`, move the table with arrow keys or `j`/`k`, scroll the preview with the same keys or the mouse wheel, share with `Enter`, unshare with `u`, or quit with `q`

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

### `get_active_pane`

Returns the current shared pane id and selection time, or a clear message if no pane is shared.

### `read_active_pane`

Captures a fresh plain-text snapshot of the shared pane. If the pane disappeared, the server clears the selection and returns a clear message.

## tmux keybinding example

Add something like this to your tmux config after installing the binaries on your `PATH`:

```tmux
bind-key s popup -E 'tmuxmcp --server http://127.0.0.1:46321'
```

## Manual Testing

1. Start the daemon:

```bash
go run ./cmd/tmuxmcpd --listen 127.0.0.1:46321 --history-lines 500 --log-file tmuxmcpd.log
```

2. Inside tmux, open a few panes with visible output.

3. Launch the popup client:

```bash
tmux popup -E 'go run ./cmd/tmuxmcp --server http://127.0.0.1:46321'
```

4. Verify the popup can:

- scroll through the pane table
- update the preview when the highlighted row changes
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

6. Verify MCP from a host/client by calling `get_active_pane` and `read_active_pane` against the stdio server.

7. Inspect daemon logs if anything fails:

```bash
less tmuxmcpd.log
```

## Notes

- The popup client talks to the local HTTP server only.
- The popup UI uses `github.com/charmbracelet/bubbletea` with `bubbles/table` and `bubbles/viewport`.
- MCP traffic uses `stdio` only in v1.
- MCP transport behavior comes from the official Go SDK `StdioTransport`, which uses newline-delimited JSON on stdin/stdout.
- `tmuxmcpd` logs to a file so stdout stays reserved for MCP traffic. The default log path is `tmuxmcpd.log`.
- No localhost auth is implemented in v1.
