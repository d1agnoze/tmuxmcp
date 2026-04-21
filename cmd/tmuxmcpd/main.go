package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/d1agnoze/tmuxmcp/internal/config"
	"github.com/d1agnoze/tmuxmcp/internal/httpapi"
	"github.com/d1agnoze/tmuxmcp/internal/mcp"
	"github.com/d1agnoze/tmuxmcp/internal/state"
	"github.com/d1agnoze/tmuxmcp/internal/tmux"
)

func main() {
	listenAddr := flag.String("listen", config.DefaultListenAddr, "HTTP listen address for local control API")
	historyLines := flag.Int("history-lines", config.DefaultHistoryLine, "Number of pane history lines exposed to MCP (500-2000)")
	logFile := flag.String("log-file", config.DefaultLogFilePath(), "Path to tmuxmcpd log file")
	flag.Parse()

	cfg := config.Server{
		ListenAddr:   *listenAddr,
		HistoryLines: *historyLines,
		LogFile:      *logFile,
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.LogFile), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create log directory %q: %v\n", filepath.Dir(cfg.LogFile), err)
		os.Exit(1)
	}

	logWriter, err := os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open log file %q: %v\n", cfg.LogFile, err)
		os.Exit(1)
	}
	defer logWriter.Close()

	logger := slog.New(slog.NewTextHandler(logWriter, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store := state.NewStore()
	runner := tmux.New()
	httpServer := httpapi.New(cfg.ListenAddr, store, runner)
	mcpServer := mcp.New(store, runner, cfg.HistoryLines, logger)
	logger.Info("tmuxmcpd starting", "listen_addr", cfg.ListenAddr, "history_lines", cfg.HistoryLines, "log_file", cfg.LogFile)

	httpErrCh := make(chan error, 1)
	go func() {
		httpErrCh <- httpServer.Start()
	}()

	mcpErrCh := make(chan error, 1)
	go func() {
		mcpErrCh <- mcpServer.Run(ctx)
	}()

	select {
	case err := <-httpErrCh:
		if err != nil {
			logger.Error("http server failed", "error", err)
			os.Exit(1)
		}
	case err := <-mcpErrCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("mcp server failed", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown failed", "error", err)
	}
	logger.Info("tmuxmcpd stopped")
}
