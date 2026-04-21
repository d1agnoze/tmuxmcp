package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultLogFilePath(t *testing.T) {
	t.Run("uses XDG_DATA_HOME when set", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "/custom/data")
		got := DefaultLogFilePath()
		want := filepath.Join("/custom/data", "tmuxmcp", "tmuxmcpd.log")
		if got != want {
			t.Fatalf("DefaultLogFilePath() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to ~/.local/share when XDG_DATA_HOME unset", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "")
		got := DefaultLogFilePath()
		if !strings.HasSuffix(got, filepath.Join("tmuxmcp", "tmuxmcpd.log")) {
			t.Fatalf("DefaultLogFilePath() = %q, expected suffix %q", got, filepath.Join("tmuxmcp", "tmuxmcpd.log"))
		}
		home, err := os.UserHomeDir()
		if err == nil {
			want := filepath.Join(home, ".local", "share", "tmuxmcp", "tmuxmcpd.log")
			if got != want {
				t.Fatalf("DefaultLogFilePath() = %q, want %q", got, want)
			}
		}
	})
}

func TestServerValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Server
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Server{
				ListenAddr:   DefaultListenAddr,
				HistoryLines: DefaultHistoryLine,
			},
		},
		{
			name: "history too small",
			cfg: Server{
				ListenAddr:   DefaultListenAddr,
				HistoryLines: MinHistoryLine - 1,
			},
			wantErr: true,
		},
		{
			name: "history too large",
			cfg: Server{
				ListenAddr:   DefaultListenAddr,
				HistoryLines: MaxHistoryLine + 1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}
