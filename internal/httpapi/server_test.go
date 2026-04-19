package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/d1agnoze/tmuxmcp/internal/state"
	"github.com/d1agnoze/tmuxmcp/internal/tmux"
)

type fakeRunner struct {
	exists map[string]bool
	err    error
}

func (f fakeRunner) ListPanes(context.Context) ([]tmux.Pane, error) {
	return nil, nil
}

func (f fakeRunner) CapturePane(context.Context, string, int) (string, error) {
	return "", nil
}

func (f fakeRunner) PaneExists(_ context.Context, paneID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}

	return f.exists[paneID], nil
}

func TestHandleSetActivePane(t *testing.T) {
	server := New("127.0.0.1:0", state.NewStore(), fakeRunner{exists: map[string]bool{"%3": true}})
	req := httptest.NewRequest(http.MethodPost, "/active-pane", strings.NewReader(`{"pane_id":"%3"}`))
	rec := httptest.NewRecorder()

	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rec.Code)
	}

	var body activePaneResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !body.Shared || body.ActivePane == nil || body.ActivePane.PaneID != "%3" {
		t.Fatalf("unexpected response body: %+v", body)
	}
}

func TestHandleSetActivePaneNotFound(t *testing.T) {
	server := New("127.0.0.1:0", state.NewStore(), fakeRunner{exists: map[string]bool{}})
	req := httptest.NewRequest(http.MethodPost, "/active-pane", strings.NewReader(`{"pane_id":"%99"}`))
	rec := httptest.NewRecorder()

	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestHandleGetActivePaneEmpty(t *testing.T) {
	server := New("127.0.0.1:0", state.NewStore(), fakeRunner{exists: map[string]bool{}})
	req := httptest.NewRequest(http.MethodGet, "/active-pane", nil)
	rec := httptest.NewRecorder()

	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rec.Code)
	}

	var body activePaneResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Shared {
		t.Fatalf("expected no active pane, got %+v", body)
	}
}
