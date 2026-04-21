# tmuxmcp

**Share a tmux pane with your AI coding agent — in real time.**

`tmuxmcp` lets you point a coding agent (e.g. Claude, Cursor, Copilot) at a specific tmux pane so it can read live terminal output — logs, test failures, command results — without you having to copy-paste anything.

---

## Table of Contents

- [How It Works](#how-it-works)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Popup Client Usage](#popup-client-usage)
- [MCP Server Setup](#mcp-server-setup)
- [Configuration Reference](#configuration-reference)
- [HTTP API Reference](#http-api-reference)
- [MCP Tools Reference](#mcp-tools-reference)
- [Development](#development)

---

## How It Works

```
┌──────────────────────────────────────────────────────┐
│  tmux                                                │
│  ┌─────────────────┐   ┌──────────────────────────┐  │
│  │  your terminal  │   │  tmuxmcp popup (s key)   │  │
│  │  (logs, tests)  │   │  [select pane to share]  │  │
│  └────────┬────────┘   └────────────┬─────────────┘  │
│           │ pane output             │ HTTP           │
│           └──────────┐  ┌───────────┘                │
│                      ▼  ▼                            │
│               ┌─────────────┐                        │
│               │  tmuxmcpd   │ ◄── MCP stdio ──► AI   │
│               │   (daemon)  │                        │
│               └─────────────┘                        │
└──────────────────────────────────────────────────────┘
```

1. **`tmuxmcpd`** is a background daemon. It runs an MCP server (over `stdio`) and a small HTTP control API (local only).
2. **`tmuxmcp`** is a popup TUI client (launched with a tmux keybinding). Use it to pick which pane to share.
3. Once a pane is shared, your AI agent can call `get_active_pane` / `read_active_pane` via MCP to read the live terminal output.

> **v1 limits:** one shared pane at a time · read-only · in-memory only (cleared on restart) · Linux first

---

## Requirements

- **Go 1.22+** (for `go install`)
- **tmux** (any reasonably modern version)
- **Linux** (macOS may work but is untested)
- An MCP-capable AI host (Claude Desktop, Cursor, VS Code with Copilot, etc.)

---

## Installation

Install both binaries into your Go bin directory:

```bash
go install github.com/d1agnoze/tmuxmcp/cmd/tmuxmcpd@latest
go install github.com/d1agnoze/tmuxmcp/cmd/tmuxmcp@latest
```

Make sure the Go bin directory is on your `PATH` (add to `~/.bashrc` or `~/.zshrc`):

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

Verify the installation:

```bash
tmuxmcpd --help
tmuxmcp --help
```

---

## Quick Start

**Step 1 — Start the daemon** (run this once, e.g. in a background tmux pane):

```bash
tmuxmcpd --listen 127.0.0.1:46321 --history-lines 500
```

**Step 2 — Add a tmux keybinding** to open the popup client (`~/.tmux.conf`):

```tmux
bind-key s popup -w 90% -h 85% -E 'tmuxmcp --server http://127.0.0.1:46321'
```

Reload your tmux config:

```bash
tmux source-file ~/.tmux.conf
```

**Step 3 — Share a pane**: press `s` inside tmux, highlight the pane you want to share, press `Enter`.

**Step 4 — Configure your AI host** — see [MCP Server Setup](#mcp-server-setup) below.

**Step 5 — Ask your agent** to call `read_active_pane` to inspect the terminal output.

---

## Popup Client Usage

Open the popup with your keybinding (e.g. `s`) from inside tmux.

### Keybindings

| Key | Action |
|-----|--------|
| `↑` / `k` | Move selection up in the pane table |
| `↓` / `j` | Move selection down in the pane table |
| `Enter` | Share the highlighted pane |
| `u` | Unshare the currently shared pane |
| `Tab` | Switch focus between the table and preview panel |
| `↑` / `↓` / scroll | Scroll the preview panel (when focused or hovered) |
| `q` / `Ctrl+C` | Quit the popup |

### What you see

- **Pane table** — all tmux panes across all sessions. The currently shared pane is marked with `*`.
- **Preview panel** — live ANSI-colored snapshot of the highlighted pane, anchored to the latest lines. Scroll up to see older output.

---

## MCP Server Setup

`tmuxmcpd` speaks MCP over `stdio`. Configure your AI host to launch it as a subprocess.

### Generic MCP config (`mcpServers`)

```json
{
  "mcpServers": {
    "tmuxmcpd": {
      "command": "tmuxmcpd",
      "args": [
        "--listen", "127.0.0.1:46321",
        "--history-lines", "500"
      ]
    }
  }
}
```

> If your host does not inherit your shell `PATH`, replace `"tmuxmcpd"` with the full path (e.g. `/home/you/go/bin/tmuxmcpd`).

After adding the config, restart or reload your MCP host. The agent will have access to two tools: `get_active_pane` and `read_active_pane`.

**Typical agent flow:**

1. Agent calls `get_active_pane` → confirms a pane is shared and gets its ID.
2. Agent calls `read_active_pane` → reads the latest terminal output from that pane.

---

## Configuration Reference

### `tmuxmcpd` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--listen` | `127.0.0.1:46321` | Local HTTP control API address |
| `--history-lines` | `500` | Lines of pane history exposed via MCP (range: 500–2000) |
| `--log-file` | `~/.local/share/tmuxmcp/tmuxmcpd.log` | Daemon log file path — honours `$XDG_DATA_HOME`; directory is created automatically (stdout is reserved for MCP stdio traffic) |

### `tmuxmcp` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--server` | *(required)* | HTTP base URL of the running `tmuxmcpd` instance |
| `--preview-lines` | `8` | Number of lines shown in the pane preview panel |

---

## HTTP API Reference

The local HTTP API is used by the popup client and can also be scripted with `curl`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/healthz` | Health check |
| `GET` | `/active-pane` | Returns the currently shared pane (or empty if none) |
| `POST` | `/active-pane` | Set the shared pane — body: `{"pane_id": "%3"}` |
| `DELETE` | `/active-pane` | Clear the shared pane |

**Example:**

```bash
# Check health
curl http://127.0.0.1:46321/healthz

# See current shared pane
curl http://127.0.0.1:46321/active-pane

# Share pane %3
curl -X POST http://127.0.0.1:46321/active-pane \
  -H 'Content-Type: application/json' \
  -d '{"pane_id": "%3"}'

# Clear the shared pane
curl -X DELETE http://127.0.0.1:46321/active-pane
```

---

## MCP Tools Reference

The server advertises two read-only tools to connected MCP clients.

### `get_active_pane`

Returns the currently shared pane ID and selection time, or a message indicating no pane is shared.

- **Use this first** to confirm a pane is available before reading output.
- Annotations: read-only, idempotent, non-destructive.

### `read_active_pane`

Returns the latest plain-text output from the shared pane (logs, test results, command output, live terminal state). If the pane has disappeared, the selection is cleared automatically.

- **Use this** when the user asks about terminal output, logs, or test failures.
- Annotations: read-only, idempotent, non-destructive.

---

## Development

**Build:**

```bash
go build ./...
```

**Test:**

```bash
go test ./... && go build ./...
```

**Focused package tests:**

```bash
go test ./internal/config
go test ./internal/httpapi
go test ./internal/mcp
go test -cover ./...
```

**End-to-end manual check:**

1. Start `tmuxmcpd`, open `tmuxmcp` in a tmux popup, share a pane.
2. Verify table navigation, preview updates, pane select/unshare.
3. Exercise the HTTP API with `curl` (see [HTTP API Reference](#http-api-reference)).
4. Verify MCP tools from your AI host — `get_active_pane` then `read_active_pane`.
5. Check `~/.local/share/tmuxmcp/tmuxmcpd.log` if anything looks wrong.
