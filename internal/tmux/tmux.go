package tmux

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

var ErrPaneNotFound = errors.New("tmux pane not found")

type Pane struct {
	ID          string
	SessionName string
	WindowIndex string
	WindowName  string
	PaneIndex   string
	PaneTitle   string
	Active      bool
}

type Runner interface {
	ListPanes(ctx context.Context) ([]Pane, error)
	CapturePane(ctx context.Context, paneID string, lines int) (string, error)
	PaneExists(ctx context.Context, paneID string) (bool, error)
}

type CommandRunner struct{}

func New() *CommandRunner {
	return &CommandRunner{}
}

func (r *CommandRunner) ListPanes(ctx context.Context) ([]Pane, error) {
	format := "#{pane_id}\t#{session_name}\t#{window_index}\t#{window_name}\t#{pane_index}\t#{pane_title}\t#{pane_active}"
	out, err := runTmux(ctx, "list-panes", "-a", "-F", format)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	panes := make([]Pane, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) != 7 {
			return nil, fmt.Errorf("unexpected tmux list-panes output: %q", line)
		}

		panes = append(panes, Pane{
			ID:          parts[0],
			SessionName: parts[1],
			WindowIndex: parts[2],
			WindowName:  parts[3],
			PaneIndex:   parts[4],
			PaneTitle:   parts[5],
			Active:      parts[6] == "1",
		})
	}

	return panes, nil
}

func (r *CommandRunner) CapturePane(ctx context.Context, paneID string, lines int) (string, error) {
	if lines <= 0 {
		return "", fmt.Errorf("lines must be greater than 0")
	}

	start := strconv.Itoa(-lines)
	out, err := runTmux(ctx, "capture-pane", "-e", "-p", "-t", paneID, "-S", start)
	if err != nil {
		if isPaneMissing(err) {
			return "", ErrPaneNotFound
		}

		return "", err
	}

	return strings.TrimRight(out, "\n"), nil
}

func (r *CommandRunner) PaneExists(ctx context.Context, paneID string) (bool, error) {
	_, err := runTmux(ctx, "display-message", "-p", "-t", paneID, "#{pane_id}")
	if err == nil {
		return true, nil
	}

	if isPaneMissing(err) {
		return false, nil
	}

	return false, err
}

func runTmux(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}

	return string(out), nil
}

func isPaneMissing(err error) bool {
	return strings.Contains(err.Error(), "can't find pane") || strings.Contains(err.Error(), "can't find window")
}
