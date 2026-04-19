package config

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	DefaultListenAddr  = "127.0.0.1:46321"
	DefaultHistoryLine = 500
	MaxHistoryLine     = 2000
	MinHistoryLine     = 500
	DefaultPreviewLine = 8
	DefaultLogFile     = "tmuxmcpd.log"
)

type Server struct {
	ListenAddr   string
	HistoryLines int
	LogFile      string
}

func (c Server) Validate() error {
	if strings.TrimSpace(c.ListenAddr) == "" {
		return fmt.Errorf("listen address is required")
	}

	if c.HistoryLines < MinHistoryLine || c.HistoryLines > MaxHistoryLine {
		return fmt.Errorf("history-lines must be between %d and %d", MinHistoryLine, MaxHistoryLine)
	}

	return nil
}

type Client struct {
	ServerURL    string
	PreviewLines int
}

func (c Client) Validate() error {
	if strings.TrimSpace(c.ServerURL) == "" {
		return fmt.Errorf("server URL is required")
	}

	parsed, err := url.Parse(c.ServerURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("server URL must be a valid absolute URL")
	}

	if c.PreviewLines <= 0 {
		return fmt.Errorf("preview-lines must be greater than 0")
	}

	return nil
}
