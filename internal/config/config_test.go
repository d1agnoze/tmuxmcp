package config

import "testing"

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
