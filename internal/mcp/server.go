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

var falseBool = false

func New(store *state.Store, runner tmux.Runner, historyLines int, logger *slog.Logger) *Server {
	s := &Server{
		store:        store,
		tmux:         runner,
		historyLines: historyLines,
	}

	s.server = sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "tmuxmcpd",
		Title:   "tmuxmcp Shared Pane Server",
		Version: "0.1.0",
	}, &sdkmcp.ServerOptions{
		Instructions: "Use this server to inspect one user-selected tmux pane. Call get_active_pane first to confirm a pane is currently shared. Call read_active_pane to read the latest plain-text terminal output from that pane, such as logs, command output, test failures, or a running program's screen during debugging. MCP access is read-only, and the shared-pane selection is in-memory only, so restarting tmuxmcpd clears it.",
		Logger:       logger,
	})

	s.registerTools()
	return s
}

func (s *Server) Run(ctx context.Context) error {
	return s.server.Run(ctx, &sdkmcp.StdioTransport{})
}

func (s *Server) registerTools() {
	sdkmcp.AddTool(s.server, &sdkmcp.Tool{
		Name:        "get_active_pane",
		Title:       "Get Active Shared Pane",
		Description: "Check whether a tmux pane is currently shared and return its pane id plus selection timestamp.",
		Annotations: &sdkmcp.ToolAnnotations{
			Title:           "Get Active Shared Pane",
			ReadOnlyHint:    true,
			IdempotentHint:  true,
			DestructiveHint: &falseBool,
			OpenWorldHint:   &falseBool,
		},
	}, s.handleGetActivePane)

	sdkmcp.AddTool(s.server, &sdkmcp.Tool{
		Name:        "read_active_pane",
		Title:       "Read Shared Pane Output",
		Description: "Read the latest plain-text output from the currently shared tmux pane, such as logs, command output, or a running program's screen.",
		Annotations: &sdkmcp.ToolAnnotations{
			Title:           "Read Shared Pane Output",
			ReadOnlyHint:    true,
			IdempotentHint:  true,
			DestructiveHint: &falseBool,
			OpenWorldHint:   &falseBool,
		},
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
