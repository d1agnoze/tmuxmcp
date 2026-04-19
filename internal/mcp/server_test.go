package mcp

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/d1agnoze/tmuxmcp/internal/state"
	"github.com/d1agnoze/tmuxmcp/internal/tmux"
)

type fakeRunner struct {
	exists  map[string]bool
	capture string
	err     error
}

func (f fakeRunner) ListPanes(context.Context) ([]tmux.Pane, error) {
	return nil, nil
}

func (f fakeRunner) CapturePane(context.Context, string, int) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.capture, nil
}

func (f fakeRunner) PaneExists(_ context.Context, paneID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.exists[paneID], nil
}

func TestReadActivePaneWithoutSelection(t *testing.T) {
	server := New(state.NewStore(), fakeRunner{capture: "captured output"}, 500, nil)
	session := connectClient(t, server)

	res, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "read_active_pane", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}

	text := firstText(t, res)
	if !strings.Contains(text, "No tmux pane is currently shared") {
		t.Fatalf("unexpected tool response text: %q", text)
	}
}

func TestReadActivePaneReturnsCapture(t *testing.T) {
	store := state.NewStore()
	store.Set("%3")
	server := New(store, fakeRunner{exists: map[string]bool{"%3": true}, capture: "captured output"}, 500, nil)
	session := connectClient(t, server)

	res, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "read_active_pane", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}

	if got := firstText(t, res); got != "captured output" {
		t.Fatalf("expected capture, got %q", got)
	}
}

func TestGetActivePaneClearsStaleSelection(t *testing.T) {
	store := state.NewStore()
	store.Set("%9")
	server := New(store, fakeRunner{exists: map[string]bool{}}, 500, nil)
	session := connectClient(t, server)

	res, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "get_active_pane", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}

	text := firstText(t, res)
	if !strings.Contains(text, "selection has been cleared") {
		t.Fatalf("unexpected tool response text: %q", text)
	}
	if active := store.Get(); active != nil {
		t.Fatalf("expected stale selection to be cleared, got %+v", active)
	}
}

func TestReadActivePaneReturnsToolError(t *testing.T) {
	store := state.NewStore()
	store.Set("%3")
	server := New(store, fakeRunner{exists: map[string]bool{"%3": true}, err: errors.New("tmux failed")}, 500, nil)
	session := connectClient(t, server)

	res, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "read_active_pane", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected tool error result, got %+v", res)
	}
	if got := firstText(t, res); !strings.Contains(got, "tmux failed") {
		t.Fatalf("expected tmux failure in tool response, got %q", got)
	}
}

func TestInitializeResultIncludesServerMetadata(t *testing.T) {
	server := New(state.NewStore(), fakeRunner{}, 500, nil)
	session := connectClient(t, server)

	init := session.InitializeResult()
	if init == nil {
		t.Fatal("expected initialize result")
	}
	if init.ServerInfo == nil {
		t.Fatal("expected server info")
	}
	if init.ServerInfo.Name != "tmuxmcpd" {
		t.Fatalf("unexpected server name %q", init.ServerInfo.Name)
	}
	if init.ServerInfo.Title != "tmuxmcp Shared Pane Server" {
		t.Fatalf("unexpected server title %q", init.ServerInfo.Title)
	}
	if !strings.Contains(init.Instructions, "Call get_active_pane first") {
		t.Fatalf("expected initialize instructions to mention get_active_pane, got %q", init.Instructions)
	}
	if !strings.Contains(init.Instructions, "read_active_pane") {
		t.Fatalf("expected initialize instructions to mention read_active_pane, got %q", init.Instructions)
	}
}

func TestListToolsIncludesToolMetadata(t *testing.T) {
	server := New(state.NewStore(), fakeRunner{}, 500, nil)
	session := connectClient(t, server)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(res.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(res.Tools))
	}

	getTool := slices.IndexFunc(res.Tools, func(tool *sdkmcp.Tool) bool { return tool.Name == "get_active_pane" })
	readTool := slices.IndexFunc(res.Tools, func(tool *sdkmcp.Tool) bool { return tool.Name == "read_active_pane" })
	if getTool < 0 || readTool < 0 {
		t.Fatalf("expected get_active_pane and read_active_pane tools, got %+v", res.Tools)
	}

	assertToolMetadata(t, res.Tools[getTool], "Get Active Shared Pane", "currently shared", false)
	assertToolMetadata(t, res.Tools[readTool], "Read Shared Pane Output", "logs, command output", false)
}

func connectClient(t *testing.T, server *Server) *sdkmcp.ClientSession {
	t.Helper()

	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	if _, err := server.server.Connect(context.Background(), serverTransport, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "0.1.0"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func firstText(t *testing.T, res *sdkmcp.CallToolResult) string {
	t.Helper()

	if len(res.Content) == 0 {
		t.Fatalf("expected content in tool result")
	}
	text, ok := res.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", res.Content[0])
	}
	return text.Text
}

func assertToolMetadata(t *testing.T, tool *sdkmcp.Tool, wantTitle, wantDescription string, wantOpenWorld bool) {
	t.Helper()

	if tool.Title != wantTitle {
		t.Fatalf("unexpected tool title for %s: %q", tool.Name, tool.Title)
	}
	if !strings.Contains(tool.Description, wantDescription) {
		t.Fatalf("unexpected tool description for %s: %q", tool.Name, tool.Description)
	}
	if tool.Annotations == nil {
		t.Fatalf("expected annotations for %s", tool.Name)
	}
	if !tool.Annotations.ReadOnlyHint {
		t.Fatalf("expected read-only annotation for %s", tool.Name)
	}
	if !tool.Annotations.IdempotentHint {
		t.Fatalf("expected idempotent annotation for %s", tool.Name)
	}
	if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
		t.Fatalf("expected non-destructive annotation for %s", tool.Name)
	}
	if tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint != wantOpenWorld {
		t.Fatalf("unexpected open-world annotation for %s", tool.Name)
	}
}
