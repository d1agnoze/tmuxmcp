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

	return strings.TrimRight(sanitizeANSI(out), "\n"), nil
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

func sanitizeANSI(s string) string {
	var out strings.Builder
	out.Grow(len(s))

	for i := 0; i < len(s); {
		if s[i] != '\x1b' {
			out.WriteByte(s[i])
			i++
			continue
		}

		if i+1 >= len(s) {
			break
		}

		switch s[i+1] {
		case '[':
			j := i + 2
			for j < len(s) && !isCSIFinal(s[j]) {
				j++
			}
			if j >= len(s) {
				return out.String()
			}
			if s[j] == 'm' && isSafeSGR(s[i+2:j]) {
				out.WriteString(s[i : j+1])
			}
			i = j + 1
		case ']':
			i = skipOSC(s, i+2)
		default:
			i += 2
		}
	}

	return out.String()
}

func isCSIFinal(b byte) bool {
	return b >= 0x40 && b <= 0x7e
}

func isSafeSGR(s string) bool {
	for i := 0; i < len(s); i++ {
		if (s[i] < '0' || s[i] > '9') && s[i] != ';' && s[i] != ':' {
			return false
		}
	}
	return true
}

func skipOSC(s string, start int) int {
	for i := start; i < len(s); i++ {
		if s[i] == '\a' {
			return i + 1
		}
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
			return i + 2
		}
	}
	return len(s)
}
