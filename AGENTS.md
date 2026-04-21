# AGENTS

## Entrypoints

- `cmd/tmuxmcpd` is the main daemon: it serves the local HTTP control API and the MCP stdio server in the same process.
- `cmd/tmuxmcp` is not a generic standalone TUI; v1 is a Bubble Tea popup client intended to run inside `tmux popup`.

## Verified Commands

- Build everything: `go build ./...`
- Run tests and build in one pass: `go test ./... && go build ./...`
- Install binaries: `go install github.com/d1agnoze/tmuxmcp/cmd/tmuxmcpd@latest && go install github.com/d1agnoze/tmuxmcp/cmd/tmuxmcp@latest`
- Run the server locally: `tmuxmcpd --listen 127.0.0.1:46321 --history-lines 500`
- Run the popup client from inside tmux: `tmux popup -w 90% -h 85% -E 'tmuxmcp --server http://127.0.0.1:46321'`

## Runtime Boundaries

- MCP is `stdio` only in v1, implemented with the official `github.com/modelcontextprotocol/go-sdk`. The HTTP server is only for the local tmux client and exposes `GET/POST/DELETE /active-pane` plus `GET /healthz`.
- The shared pane selection is in-memory only. Restarting `tmuxmcpd` clears it.
- MCP is tools-only in v1: `get_active_pane` and `read_active_pane`.
- The server also publishes initialize-time instructions that describe the server as a read-only shared-pane inspection surface and tell clients to check `get_active_pane` before using `read_active_pane`.
- Treat `read_active_pane` as the tool for reading the current shared terminal output, including logs, test failures, command output, or a running program's screen during debugging.
- Both tools advertise read-only, idempotent, non-destructive, closed-world annotations plus human-readable titles.
- `--history-lines` on `tmuxmcpd` is validated to `500..2000`; default is `500`.
- `--log-file` on `tmuxmcpd` defaults to `~/.local/share/tmuxmcp/tmuxmcpd.log` (honours `$XDG_DATA_HOME`; the directory is created automatically). Keep daemon and SDK logs off stdout because stdout is reserved for MCP stdio traffic.
- `tmuxmcp` also has `--preview-lines`; default is `8`.
- Do not reintroduce custom MCP framing unless there is a strong reason. The SDK `StdioTransport` is the source of truth for stdio behavior and uses newline-delimited JSON rather than `Content-Length` framing.

## tmux Integration Notes

- Pane previews are captured directly by the client with tmux commands, preserving ANSI styling while stripping non-styling control sequences; the popup does not ask the daemon for previews.
- The popup renders panes in a table and updates the preview panel below the table from the highlighted row.
- The preview panel keeps pane metadata above the scrollable viewport and opens from the bottom of the captured pane content.
- The popup supports focus switching between the table and preview; mouse wheel scrolling is routed to the preview when it is hovered or focused.
- The client captures previews concurrently with a small worker limit and short timeout. Keep that behavior if you change preview loading; serial per-pane capture makes the popup sluggish.
- `internal/tmux.ListPanes` parses tab-delimited `tmux list-panes` output. Be careful changing pane metadata fields because titles or names with unexpected delimiters can break parsing.

## Verification Expectations

- There is no Makefile, task runner, CI workflow, or linter config in this repo right now. The checked-in verification path is the Go toolchain: `go test ./... && go build ./...`.
- Focused verification commands that exist today: `go test ./internal/config`, `go test ./internal/httpapi`, `go test ./internal/mcp`, and `go test -cover ./...`.
- For end-to-end validation, run `tmuxmcpd`, open `tmuxmcp` inside `tmux popup`, verify table navigation, preview updates, sticky preview metadata, preview scrolling from the latest lines, pane select/unshare behavior, then hit `GET /healthz`, `GET/POST/DELETE /active-pane`, and finally verify `get_active_pane` and `read_active_pane` from an MCP host/client, including a check that `read_active_pane` returns the current shared pane output.

## Documentation Sync

- Treat `README.md` and `AGENTS.md` as paired docs. When behavior, commands, flags, runtime boundaries, transport details, testing steps, or verification expectations change, update both files in the same session.
- Do not leave one of these files reflecting old behavior after code changes. If only one file needs a wording cleanup with no code or workflow change, keep the other unchanged.
