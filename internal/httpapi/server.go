package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/d1agnoze/tmuxmcp/internal/state"
	"github.com/d1agnoze/tmuxmcp/internal/tmux"
)

type Server struct {
	store  *state.Store
	tmux   tmux.Runner
	server *http.Server
}

type setActivePaneRequest struct {
	PaneID string `json:"pane_id"`
}

type activePaneResponse struct {
	Shared     bool              `json:"shared"`
	ActivePane *state.ActivePane `json:"active_pane,omitempty"`
	Error      string            `json:"error,omitempty"`
}

func New(addr string, store *state.Store, runner tmux.Runner) *Server {
	muxServer := &Server{store: store, tmux: runner}
	muxServer.server = &http.Server{
		Addr:              addr,
		Handler:           muxServer.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	return muxServer
}

func (s *Server) Start() error {
	err := s.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) routes() http.Handler {
	muxMux := http.NewServeMux()
	muxMux.HandleFunc("/healthz", s.handleHealth)
	muxMux.HandleFunc("/active-pane", s.handleActivePane)
	return muxMux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleActivePane(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetActivePane(w, r)
	case http.MethodPost:
		s.handleSetActivePane(w, r)
	case http.MethodDelete:
		s.store.Clear()
		writeJSON(w, http.StatusOK, activePaneResponse{Shared: false})
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		writeJSON(w, http.StatusMethodNotAllowed, activePaneResponse{Error: fmt.Sprintf("method %s not allowed", r.Method)})
	}
}

func (s *Server) handleGetActivePane(w http.ResponseWriter, r *http.Request) {
	active := s.store.Get()
	if active == nil {
		writeJSON(w, http.StatusOK, activePaneResponse{Shared: false})
		return
	}

	exists, err := s.tmux.PaneExists(r.Context(), active.PaneID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, activePaneResponse{Error: "failed to validate pane state"})
		return
	}

	if !exists {
		s.store.Clear()
		writeJSON(w, http.StatusOK, activePaneResponse{Shared: false})
		return
	}

	writeJSON(w, http.StatusOK, activePaneResponse{Shared: true, ActivePane: active})
}

func (s *Server) handleSetActivePane(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req setActivePaneRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, activePaneResponse{Error: "invalid JSON body"})
		return
	}

	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, activePaneResponse{Error: "invalid JSON body"})
		return
	}

	req.PaneID = strings.TrimSpace(req.PaneID)
	if req.PaneID == "" {
		writeJSON(w, http.StatusBadRequest, activePaneResponse{Error: "pane_id is required"})
		return
	}

	exists, err := s.tmux.PaneExists(r.Context(), req.PaneID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, activePaneResponse{Error: "failed to validate pane"})
		return
	}

	if !exists {
		writeJSON(w, http.StatusNotFound, activePaneResponse{Error: "pane not found"})
		return
	}

	active := s.store.Set(req.PaneID)
	writeJSON(w, http.StatusOK, activePaneResponse{Shared: true, ActivePane: active})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
