package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/d1agnoze/tmuxmcp/internal/config"
	"github.com/d1agnoze/tmuxmcp/internal/state"
	"github.com/d1agnoze/tmuxmcp/internal/tmux"
)

var (
	headerStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	helpStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	infoStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	labelStyle         = lipgloss.NewStyle().Bold(true)
	metaTitleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	metaDetailStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	panelStyle         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	panelFocusedStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("86"))
	panelMutedStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238"))
	previewHeaderStyle = lipgloss.NewStyle().Padding(0, 0, 1, 0).BorderBottom(true).BorderForeground(lipgloss.Color("240"))
)

type activePaneResponse struct {
	Shared     bool              `json:"shared"`
	ActivePane *state.ActivePane `json:"active_pane,omitempty"`
	Error      string            `json:"error,omitempty"`
}

type panePreview struct {
	text string
	err  error
}

type rect struct {
	x int
	y int
	w int
	h int
}

type statusKind int

type focusTarget int

const (
	statusInfo statusKind = iota
	statusError
	focusTable focusTarget = iota
	focusPreview
)

type tuiModel struct {
	serverURL  string
	panes      []tmux.Pane
	previews   map[string]panePreview
	active     *state.ActivePane
	table      table.Model
	previewHdr string
	preview    viewport.Model
	width      int
	height     int
	status     string
	statusTyp  statusKind
	focus      focusTarget
	previewBox rect
	quitting   bool
}

func main() {
	serverURL := flag.String("server", "http://"+config.DefaultListenAddr, "HTTP base URL for tmuxmcpd")
	previewLines := flag.Int("preview-lines", config.DefaultPreviewLine, "Number of preview lines shown per pane")
	flag.Parse()

	cfg := config.Client{ServerURL: *serverURL, PreviewLines: *previewLines}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	runner := tmux.New()
	listCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	panes, err := runner.ListPanes(listCtx)
	if err != nil {
		exitf("list panes: %v", err)
	}
	if len(panes) == 0 {
		exitf("no tmux panes found")
	}

	active, err := getActivePane(cfg.ServerURL)
	if err != nil {
		exitf("query active pane: %v", err)
	}

	model := newTUIModel(cfg.ServerURL, panes, capturePreviews(runner, panes, cfg.PreviewLines), active)
	if _, err := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		exitf("run popup UI: %v", err)
	}
}

func newTUIModel(serverURL string, panes []tmux.Pane, previews map[string]panePreview, active *state.ActivePane) tuiModel {
	columns := []table.Column{
		{Title: "*", Width: 3},
		{Title: "Pane", Width: 7},
		{Title: "Session", Width: 18},
		{Title: "Window", Width: 18},
		{Title: "Title", Width: 28},
	}

	rows := make([]table.Row, 0, len(panes))
	selected := 0
	for i, pane := range panes {
		shared := ""
		if active != nil && active.PaneID == pane.ID {
			shared = "*"
			selected = i
		}

		window := pane.WindowIndex + "." + pane.PaneIndex
		if pane.WindowName != "" {
			window += " " + pane.WindowName
		}

		rows = append(rows, table.Row{
			shared,
			pane.ID,
			pane.SessionName,
			window,
			pane.PaneTitle,
		})
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(8),
	)
	t.SetCursor(selected)
	t.SetStyles(tableStyles(true))

	vp := viewport.New(20, 10)
	vp.MouseWheelEnabled = true

	m := tuiModel{
		serverURL: serverURL,
		panes:     panes,
		previews:  previews,
		active:    active,
		table:     t,
		preview:   vp,
		status:    "Enter shares the highlighted pane. Press u to unshare.",
		statusTyp: statusInfo,
		focus:     focusTable,
	}
	m.syncFocus()
	m.syncPreview(true)
	return m
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "tab":
			if m.focus == focusTable {
				m.focus = focusPreview
			} else {
				m.focus = focusTable
			}
			m.syncFocus()
			return m, nil
		case "enter":
			if m.focus != focusTable {
				break
			}
			pane, ok := m.selectedPane()
			if !ok {
				return m, nil
			}
			if err := setActivePane(m.serverURL, pane.ID); err != nil {
				m.status = fmt.Sprintf("share pane %s: %v", pane.ID, err)
				m.statusTyp = statusError
				return m, nil
			}
			m.active = &state.ActivePane{PaneID: pane.ID, SelectedAt: time.Now().UTC()}
			m.refreshRows()
			m.status = fmt.Sprintf("Shared pane %s", pane.ID)
			m.statusTyp = statusInfo
			return m, nil
		case "u":
			if err := clearActivePane(m.serverURL); err != nil {
				m.status = fmt.Sprintf("unshare pane: %v", err)
				m.statusTyp = statusError
				return m, nil
			}
			m.active = nil
			m.refreshRows()
			m.status = "Shared pane cleared"
			m.statusTyp = statusInfo
			return m, nil
		}
	case tea.MouseMsg:
		if tea.MouseEvent(msg).IsWheel() && m.isInsidePreview(tea.MouseEvent(msg).X, tea.MouseEvent(msg).Y) {
			m.focus = focusPreview
			m.syncFocus()
			var cmd tea.Cmd
			m.preview, cmd = m.preview.Update(msg)
			return m, cmd
		}
	}

	if m.focus == focusPreview {
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		return m, cmd
	}

	prev := m.table.Cursor()
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	if m.table.Cursor() != prev {
		m.syncPreview(true)
	}
	return m, cmd
}

func (m tuiModel) View() string {
	if m.quitting {
		return ""
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		headerStyle.Render("tmuxmcp"),
		m.bodyView(),
		m.footerView(),
	)

	return lipgloss.NewStyle().Padding(0, 1).Render(content)
}

func (m *tuiModel) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	panelWidth := max(1, m.width-2)
	headerHeight := lipgloss.Height(headerStyle.Render("tmuxmcp"))
	footerHeight := lipgloss.Height(m.footerView())
	bodyHeight := max(2, m.height-headerHeight-footerHeight)
	tableInnerWidth := max(1, panelWidth-2)
	previewHeaderHeight := lipgloss.Height(renderPreviewHeader(m.previewHdr, tableInnerWidth))
	scrollBudget := max(2, bodyHeight-(2+2+previewHeaderHeight))
	tableHeight := clamp(scrollBudget/3, 1, max(1, scrollBudget-1))
	previewHeight := max(1, scrollBudget-tableHeight)
	prevOffset := m.preview.YOffset

	m.table.SetWidth(tableInnerWidth)
	m.table.SetHeight(tableHeight)
	m.table.SetColumns(tableColumns(tableInnerWidth))

	m.preview.Width = tableInnerWidth
	m.preview.Height = previewHeight
	m.preview.SetYOffset(clamp(prevOffset, 0, max(0, m.preview.TotalLineCount()-m.preview.VisibleLineCount())))
	m.previewBox = rect{x: 1, y: headerHeight + tableHeight + 2, w: panelWidth, h: previewHeight + previewHeaderHeight + 2}
}

func (m *tuiModel) bodyView() string {
	panelWidth := m.table.Width() + 2
	if panelWidth <= 2 {
		panelWidth = max(1, m.width-2)
	}

	tablePanelStyle := panelMutedStyle
	previewPanelStyle := panelMutedStyle
	if m.focus == focusTable {
		tablePanelStyle = panelFocusedStyle
		previewPanelStyle = panelStyle
	} else {
		tablePanelStyle = panelStyle
		previewPanelStyle = panelFocusedStyle
	}

	tableBody := lipgloss.JoinVertical(lipgloss.Left, m.table.View())
	previewBody := lipgloss.JoinVertical(lipgloss.Left, renderPreviewHeader(m.previewHdr, m.preview.Width), m.preview.View())
	tablePanel := tablePanelStyle.Width(panelWidth).Render(tableBody)
	previewPanel := previewPanelStyle.Width(panelWidth).Render(previewBody)
	return lipgloss.JoinVertical(lipgloss.Left, tablePanel, previewPanel)
}

func (m tuiModel) footerView() string {
	shared := "No pane shared"
	if m.active != nil {
		shared = "Shared pane: " + m.active.PaneID
	}

	statusRenderer := infoStyle.Render
	if m.statusTyp == statusError {
		statusRenderer = errorStyle.Render
	}

	focusLabel := "Focus: table"
	if m.focus == focusPreview {
		focusLabel = "Focus: preview"
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		labelStyle.Render(shared),
		labelStyle.Render(focusLabel),
		statusRenderer(m.status),
		helpStyle.Render("Keys: tab switch focus • up/down or j/k move/scroll • enter share • u unshare • q quit • mouse wheel scrolls preview when hovered or focused"),
	)
}

func (m *tuiModel) syncPreview(resetScroll bool) {
	pane, ok := m.selectedPane()
	if !ok {
		m.previewHdr = "Preview"
		m.preview.SetContent("No pane selected")
		if resetScroll {
			m.preview.GotoTop()
		}
		return
	}

	meta := []string{fmt.Sprintf("Pane %s", pane.ID), fmt.Sprintf("Session %s", pane.SessionName)}
	window := pane.WindowIndex + "." + pane.PaneIndex
	if pane.WindowName != "" {
		window += " " + pane.WindowName
	}
	meta = append(meta, fmt.Sprintf("Window %s", window))
	if pane.PaneTitle != "" {
		meta = append(meta, fmt.Sprintf("Title %s", pane.PaneTitle))
	}
	m.previewHdr = strings.Join(meta, "  |  ")

	preview := m.previews[pane.ID]
	if preview.err != nil {
		m.preview.SetContent("preview unavailable: " + preview.err.Error())
		if resetScroll {
			m.preview.GotoTop()
		}
		return
	}

	body := preview.text
	if strings.TrimSpace(body) == "" {
		body = "(empty pane)"
	}
	prevOffset := m.preview.YOffset
	m.preview.SetContent(body)
	if resetScroll {
		m.preview.GotoBottom()
		return
	}
	m.preview.SetYOffset(clamp(prevOffset, 0, max(0, m.preview.TotalLineCount()-m.preview.VisibleLineCount())))
}

func (m *tuiModel) selectedPane() (tmux.Pane, bool) {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.panes) {
		return tmux.Pane{}, false
	}
	return m.panes[idx], true
}

func (m *tuiModel) refreshRows() {
	rows := make([]table.Row, 0, len(m.panes))
	for _, pane := range m.panes {
		shared := ""
		if m.active != nil && m.active.PaneID == pane.ID {
			shared = "*"
		}

		window := pane.WindowIndex + "." + pane.PaneIndex
		if pane.WindowName != "" {
			window += " " + pane.WindowName
		}

		rows = append(rows, table.Row{
			shared,
			pane.ID,
			pane.SessionName,
			window,
			pane.PaneTitle,
		})
	}
	m.table.SetRows(rows)
	m.syncPreview(false)
}

func (m *tuiModel) syncFocus() {
	if m.focus == focusPreview {
		m.table.SetStyles(tableStyles(false))
		m.table.Blur()
		return
	}

	m.table.SetStyles(tableStyles(true))
	m.table.Focus()
}

func capturePreviews(runner tmux.Runner, panes []tmux.Pane, previewLines int) map[string]panePreview {
	previews := make(map[string]panePreview, len(panes))
	if len(panes) == 0 {
		return previews
	}

	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, 4)
	)

	for _, pane := range panes {
		wg.Add(1)
		go func(pane tmux.Pane) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
			defer cancel()

			text, err := runner.CapturePane(ctx, pane.ID, previewLines)

			mu.Lock()
			previews[pane.ID] = panePreview{text: text, err: err}
			mu.Unlock()
		}(pane)
	}

	wg.Wait()
	return previews
}

func getActivePane(serverURL string) (*state.ActivePane, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(serverURL, "/")+"/active-pane", nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body activePaneResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		if body.Error != "" {
			return nil, errors.New(body.Error)
		}
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}

	return body.ActivePane, nil
}

func setActivePane(serverURL, paneID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payload, err := json.Marshal(map[string]string{"pane_id": paneID})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(serverURL, "/")+"/active-pane", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var body activePaneResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		if body.Error != "" {
			return errors.New(body.Error)
		}
		return fmt.Errorf("server returned %s", resp.Status)
	}

	return nil
}

func clearActivePane(serverURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, strings.TrimRight(serverURL, "/")+"/active-pane", nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var body activePaneResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err == nil && body.Error != "" {
			return errors.New(body.Error)
		}
		return fmt.Errorf("server returned %s", resp.Status)
	}

	return nil
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func clamp(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func tableColumns(width int) []table.Column {
	usable := max(5, width-4)
	mins := []int{1, 4, 4, 4, 5}
	wants := []int{3, 7, 18, 18, 28}
	widths := []int{1, 1, 1, 1, 1}
	extra := usable - sumInts(widths)
	for _, idx := range []int{1, 2, 3, 4, 0} {
		for widths[idx] < mins[idx] && extra > 0 {
			widths[idx]++
			extra--
		}
	}
	for extra > 0 {
		advanced := false
		for i := range widths {
			if widths[i] >= wants[i] || extra == 0 {
				continue
			}
			widths[i]++
			extra--
			advanced = true
		}
		if !advanced {
			break
		}
	}

	return []table.Column{
		{Title: "*", Width: widths[0]},
		{Title: "Pane", Width: widths[1]},
		{Title: "Session", Width: widths[2]},
		{Title: "Window", Width: widths[3]},
		{Title: "Title", Width: widths[4]},
	}
}

func tableStyles(focused bool) table.Styles {
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Bold(true).Foreground(lipgloss.Color("252")).BorderForeground(lipgloss.Color("240")).BorderBottom(true)
	styles.Selected = styles.Selected.Foreground(lipgloss.Color("230")).Background(lipgloss.Color("238")).Bold(false)
	if focused {
		styles.Selected = styles.Selected.Background(lipgloss.Color("62"))
	}
	return styles
}

func renderPreviewHeader(header string, width int) string {
	parts := strings.Split(header, "  |  ")
	if len(parts) == 0 {
		return previewHeaderStyle.Render(metaTitleStyle.Render("Preview"))
	}
	if width <= 0 {
		width = 1
	}

	parts = fitPreviewHeader(parts, width)

	out := make([]string, 0, len(parts))
	for i, part := range parts {
		if i == 0 {
			out = append(out, metaTitleStyle.Render(part))
			continue
		}
		out = append(out, metaDetailStyle.Render(part))
	}

	return previewHeaderStyle.Render(strings.Join(out, "  |  "))
}

func fitPreviewHeader(parts []string, width int) []string {
	if len(parts) == 0 {
		return []string{"Preview"}
	}

	fit := make([]string, 0, len(parts))
	for _, part := range parts {
		candidate := append(append([]string(nil), fit...), part)
		if lipgloss.Width(strings.Join(candidate, "  |  ")) <= width {
			fit = candidate
			continue
		}
		if len(fit) == 0 {
			return []string{truncateText(part, width)}
		}
		remaining := width - lipgloss.Width(strings.Join(fit, "  |  ")) - lipgloss.Width("  |  ")
		if remaining > 1 {
			fit = append(fit, truncateText(part, remaining))
		}
		return fit
	}

	return fit
}

func truncateText(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}

	runes := []rune(s)
	for i := len(runes); i > 0; i-- {
		candidate := string(runes[:i]) + "…"
		if lipgloss.Width(candidate) <= width {
			return candidate
		}
	}

	return "…"
}

func sumInts(v []int) int {
	out := 0
	for _, n := range v {
		out += n
	}
	return out
}

func (m tuiModel) isInsidePreview(x, y int) bool {
	return x >= m.previewBox.x && x < m.previewBox.x+m.previewBox.w && y >= m.previewBox.y && y < m.previewBox.y+m.previewBox.h
}
