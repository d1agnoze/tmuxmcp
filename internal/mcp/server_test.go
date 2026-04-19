package mcp

import (
	"context"
	"errors"
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
