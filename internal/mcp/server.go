package mcp

import (
	"context"
	"fmt"
	"log/slog"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/d1agnoze/tmuxmcp/internal/state"
	"github.com/d1agnoze/tmuxmcp/internal/tmux"
)

type Server struct {
	store        *state.Store
	tmux         tmux.Runner
	historyLines int
	server       *sdkmcp.Server
}

func New(store *state.Store, runner tmux.Runner, historyLines int, logger *slog.Logger) *Server {
	s := &Server{
		store:        store,
		tmux:         runner,
		historyLines: historyLines,
	}

	s.server = sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "tmuxmcpd",
		Version: "0.1.0",
	}, &sdkmcp.ServerOptions{Logger: logger})

	s.registerTools()
	return s
}

func (s *Server) Run(ctx context.Context) error {
	return s.server.Run(ctx, &sdkmcp.StdioTransport{})
}

func (s *Server) registerTools() {
	sdkmcp.AddTool(s.server, &sdkmcp.Tool{
		Name:        "get_active_pane",
		Description: "Get the currently shared tmux pane id and selection timestamp.",
	}, s.handleGetActivePane)

	sdkmcp.AddTool(s.server, &sdkmcp.Tool{
		Name:        "read_active_pane",
		Description: "Read a fresh plain-text snapshot of the currently shared tmux pane.",
	}, s.handleReadActivePane)
}

func (s *Server) handleGetActivePane(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
	active := s.store.Get()
	if active == nil {
		return textResult("No tmux pane is currently shared."), nil, nil
	}

	exists, err := s.tmux.PaneExists(ctx, active.PaneID)
	if err != nil {
		return nil, nil, err
	}

	if !exists {
		s.store.Clear()
		return textResult(fmt.Sprintf("Shared tmux pane %s is no longer available. The selection has been cleared.", active.PaneID)), nil, nil
	}

	text := fmt.Sprintf("Shared pane: %s\nSelected at: %s", active.PaneID, active.SelectedAt.Format("2006-01-02T15:04:05Z07:00"))
	return textResult(text), nil, nil
}

func (s *Server) handleReadActivePane(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
	active := s.store.Get()
	if active == nil {
		return textResult("No tmux pane is currently shared."), nil, nil
	}

	exists, err := s.tmux.PaneExists(ctx, active.PaneID)
	if err != nil {
		return nil, nil, err
	}

	if !exists {
		s.store.Clear()
		return textResult(fmt.Sprintf("Shared tmux pane %s is no longer available. The selection has been cleared.", active.PaneID)), nil, nil
	}

	capture, err := s.tmux.CapturePane(ctx, active.PaneID, s.historyLines)
	if err != nil {
		return nil, nil, err
	}

	return textResult(capture), nil, nil
}

func textResult(text string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: text}},
	}
}
