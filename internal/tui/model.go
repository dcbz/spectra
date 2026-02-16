package tui

import (
	"fmt"
	"io"
	"os/exec"
	goruntime "runtime"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"watcher/internal/config"
	"watcher/internal/highlight"
	"watcher/internal/pipeline"
	"watcher/internal/rules"
	"watcher/internal/runtime"
)

// ModelConfig wires the data stream into the UI.
type ModelConfig struct {
	Events      <-chan pipeline.HighlightedEvent
	ThemeName   string
	Scrollback  int
	Files       []string
	ShowAll     bool
	MinSeverity rules.Severity
	Controller  *runtime.Controller
	Presets     []config.LogPreset
	RuleGroups  []runtime.RuleGroup
}

// Model renders a colorful monitoring dashboard.
type Model struct {
	cfg            ModelConfig
	viewport       viewport.Model
	theme          Theme
	events         <-chan pipeline.HighlightedEvent
	lines          []displayLine
	scrollback     int
	paused         bool
	follow         bool
	shimmer        bool
	eyeFrame       int
	sidebarWidth   int
	activeFiles    []string
	activeTags     []string
	counts         map[rules.Severity]int
	lastRule       string
	notification   string
	notificationT  time.Time
	selectedIndex  int
	detailOpen     bool
	detailViewport viewport.Model
	detailContent  string
	detailLine     displayLine
	helpOpen       bool
	helpViewport   viewport.Model
	config         configState
	windowWidth    int
	windowHeight   int
	showHeader     bool
	showStatus     bool
	filteredRules  map[string]bool
	hiddenIndices  map[int]bool
}

type displayLine struct {
	Severity  rules.Severity
	RuleName  string
	Path      string
	Timestamp time.Time
	Fragments []highlight.Fragment
	Tags      []string
	Text      string
	Index     int
}

type logMsg pipeline.HighlightedEvent
type tickMsg time.Time
type streamClosedMsg struct{}

const (
	modalPaddingX    = 2
	modalPaddingY    = 1
	modalChromeLines = 2
)

// NewModel returns a configured Bubble Tea model.
func NewModel(cfg ModelConfig) Model {
	scrollback := cfg.Scrollback
	if scrollback <= 0 {
		scrollback = 600
	}
	theme := themeByName(cfg.ThemeName)
	vp := viewport.New(80, 24)
	vp.SetContent("booting logstream…")
	detailVP := viewport.New(60, 20)
	helpVP := viewport.New(60, 20)
	return Model{
		cfg:            cfg,
		viewport:       vp,
		theme:          theme,
		events:         cfg.Events,
		scrollback:     scrollback,
		follow:         true,
		sidebarWidth:   30,
		activeFiles:    append([]string{}, cfg.Files...),
		activeTags:     nil,
		counts:         make(map[rules.Severity]int),
		selectedIndex:  -1,
		detailViewport: detailVP,
		helpViewport:   helpVP,
		config:         newConfigState(),
		windowWidth:    80,
		windowHeight:   24,
		showHeader:     true,
		showStatus:     true,
		filteredRules:  make(map[string]bool),
		hiddenIndices:  make(map[int]bool),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.listen(), pulse(), tea.EnterAltScreen)
}

func (m Model) listen() tea.Cmd {
	if m.events == nil {
		return nil
	}
	return func() tea.Msg {
		evt, ok := <-m.events
		if !ok {
			return streamClosedMsg{}
		}
		return logMsg(evt)
	}
}

func pulse() tea.Cmd {
	return tea.Tick(750*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height

		if msg.Width < 10 {
			msg.Width = 80
		}
		if msg.Height < 5 {
			msg.Height = 24
		}

		if m.windowWidth < m.sidebarWidth+20 {
			m.sidebarWidth = clamp(m.windowWidth/3, 18, 40)
		}
		paneFrameW, paneFrameH := m.theme.Pane.GetFrameSize()
		sidebarFrameW, _ := m.theme.Sidebar.GetFrameSize()
		sidebarTotal := m.sidebarWidth + sidebarFrameW
		totalWidth := msg.Width - sidebarTotal
		if totalWidth < paneFrameW+1 {
			totalWidth = paneFrameW + 1
		}
		contentWidth := totalWidth - paneFrameW
		if contentWidth < 1 {
			contentWidth = 1
		}
		m.viewport.Width = contentWidth

		m.showHeader = true
		m.showStatus = true
		headerHeight := lipgloss.Height(m.renderHeader())
		statusHeight := lipgloss.Height(m.renderStatus())
		minBody := 3
		availableHeight := msg.Height
		if headerHeight+statusHeight+minBody > availableHeight {
			m.showHeader = false
			headerHeight = 0
			if statusHeight+minBody > availableHeight {
				m.showStatus = false
				statusHeight = 0
			}
		}
		totalHeight := availableHeight - headerHeight - statusHeight
		if totalHeight < minBody {
			totalHeight = minBody
		}
		contentHeight := totalHeight - paneFrameH
		if contentHeight < 1 {
			contentHeight = 1
		}
		m.viewport.Height = contentHeight
		m.viewport.SetContent(m.renderLogContent())
		m.ensureSelectionVisible()
		if m.detailOpen {
			m.updateDetailViewportSize()
		}
		if m.helpOpen {
			m.updateHelpViewportSize()
		}
	case tea.KeyMsg:
		if m.config.open {
			return m.handleConfigKey(msg)
		}
		if m.helpOpen {
			switch msg.String() {
			case "q", "esc", "enter", "?":
				m.helpOpen = false
				return m, nil
			default:
				var cmd tea.Cmd
				m.helpViewport, cmd = m.helpViewport.Update(msg)
				return m, cmd
			}
		}
		if m.detailOpen {
			switch msg.String() {
			case "enter", "esc", "q":
				m.closeDetail()
			case "y", "c":
				m.copyDetailToClipboard()
			default:
				var cmd tea.Cmd
				m.detailViewport, cmd = m.detailViewport.Update(msg)
				return m, cmd
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "?":
			m.openHelp()
			return m, nil
		case "up":
			m.moveSelection(-1)
		case "down":
			m.moveSelection(1)
		case "pgup", "pageup":
			m.pageSelection(-1)
		case "pgdown", "pagedown":
			m.pageSelection(1)
		case "enter":
			m.openDetail()
		case "h":
			m.hideCurrentLine()
		case "x":
			m.filterCurrentRule()
		case "r":
			m.resetFilters()
		case "p":
			m.paused = !m.paused
			if !m.paused {
				m.viewport.SetContent(m.renderLogContent())
				if m.follow {
					m.viewport.GotoBottom()
				}
			}
		case "f":
			m.follow = !m.follow
		case "t":
			m.theme = themeByName(nextTheme(m.theme.Name))
		case "c":
			m.openConfig()
		}
	case logMsg:
		return m.consumeLog(msg)
	case tickMsg:
		m.shimmer = !m.shimmer
		if len(eyeFrames) > 0 {
			m.eyeFrame = (m.eyeFrame + 1) % len(eyeFrames)
		}
		if time.Since(m.notificationT) > 5*time.Second {
			m.notification = ""
		}
		return m, pulse()
	case streamClosedMsg:
		m.notification = "stream closed"
	case configResultMsg:
		m.config.applying = false
		if msg.err != nil {
			m.config.errorMsg = msg.err.Error()
			return m, nil
		}
		m.config.errorMsg = ""
		m.config.open = false
		m.activeFiles = append([]string{}, msg.files...)
		m.activeTags = append([]string{}, msg.tags...)
		m.notification = fmt.Sprintf("watching %d files", len(msg.files))
		m.notificationT = time.Now()
	}

	var cmd tea.Cmd
	if !m.paused {
		m.viewport, cmd = m.viewport.Update(msg)
	}
	return m, cmd
}

func (m Model) consumeLog(evt logMsg) (tea.Model, tea.Cmd) {
	if evt.Err != nil {
		m.notification = evt.Err.Error()
		m.notificationT = time.Now()
		return m, m.listen()
	}

	dl := displayLine{
		Severity:  evt.Severity,
		RuleName:  evt.RuleName,
		Path:      evt.Path,
		Timestamp: evt.Timestamp,
		Fragments: evt.Fragments,
		Tags:      append([]string{}, evt.Tags...),
		Text:      evt.Line,
		Index:     len(m.lines),
	}
	m.lines = append(m.lines, dl)
	if len(m.lines) > m.scrollback {
		trim := len(m.lines) - m.scrollback
		m.lines = m.lines[trim:]
		newHidden := make(map[int]bool)
		for idx := range m.hiddenIndices {
			if idx >= trim {
				newHidden[idx-trim] = true
			}
		}
		m.hiddenIndices = newHidden
		for i := range m.lines {
			m.lines[i].Index = i
		}
		if m.selectedIndex >= 0 {
			m.selectedIndex -= trim
			if m.selectedIndex < 0 {
				m.selectedIndex = 0
			}
		}
	}
	visibleLines := m.getVisibleLines()
	if len(visibleLines) == 0 {
		m.selectedIndex = -1
	} else if m.follow || m.selectedIndex == -1 {
		m.selectedIndex = len(visibleLines) - 1
	}
	m.counts[evt.Severity]++
	if evt.RuleName != "" {
		m.lastRule = evt.RuleName
		m.notification = fmt.Sprintf("%s · %s", evt.Severity, evt.RuleName)
		m.notificationT = time.Now()
	}
	if !m.paused {
		m.viewport.SetContent(m.renderLogContent())
		if m.follow {
			m.viewport.GotoBottom()
		} else {
			m.ensureSelectionVisible()
		}
	}
	return m, m.listen()
}

func (m *Model) moveSelection(delta int) {
	visibleLines := m.getVisibleLines()
	if len(visibleLines) == 0 {
		m.selectedIndex = -1
		return
	}
	if m.selectedIndex < 0 {
		m.selectedIndex = len(visibleLines) - 1
	}
	target := m.selectedIndex + delta
	if target < 0 {
		target = 0
	}
	if target >= len(visibleLines) {
		target = len(visibleLines) - 1
	}
	if target == m.selectedIndex {
		return
	}
	m.selectedIndex = target
	m.follow = false
	m.ensureSelectionVisible()
	m.viewport.SetContent(m.renderLogContent())
}

func (m *Model) pageSelection(pages int) {
	if pages == 0 {
		return
	}
	height := m.viewport.Height
	if height <= 1 {
		height = 1
	}
	m.moveSelection(pages * height)
}

func (m *Model) ensureSelectionVisible() {
	if m.selectedIndex < 0 {
		m.viewport.SetYOffset(0)
		return
	}
	height := m.viewport.Height
	if height <= 0 {
		return
	}
	yOffset := m.viewport.YOffset
	if m.selectedIndex < yOffset {
		m.viewport.SetYOffset(m.selectedIndex)
		return
	}
	maxVisible := yOffset + height - 1
	if m.selectedIndex > maxVisible {
		m.viewport.SetYOffset(m.selectedIndex - height + 1)
	}
}

func (m Model) selectedLine() (displayLine, bool) {
	visibleLines := m.getVisibleLines()
	if m.selectedIndex < 0 || m.selectedIndex >= len(visibleLines) {
		return displayLine{}, false
	}
	return visibleLines[m.selectedIndex], true
}

func (m *Model) refreshVisibleState() {
	visibleLines := m.getVisibleLines()
	if len(visibleLines) == 0 {
		m.selectedIndex = -1
	} else if m.selectedIndex >= len(visibleLines) {
		m.selectedIndex = len(visibleLines) - 1
	}
	m.viewport.SetContent(m.renderLogContent())
	m.ensureSelectionVisible()
}

func (m *Model) hideCurrentLine() {
	line, ok := m.selectedLine()
	if !ok {
		return
	}
	m.hiddenIndices[line.Index] = true
	m.notification = "Hidden 1 line"
	m.notificationT = time.Now()
	m.refreshVisibleState()
}

func (m *Model) filterCurrentRule() {
	line, ok := m.selectedLine()
	if !ok || line.RuleName == "" {
		return
	}
	m.filteredRules[line.RuleName] = true
	count := 0
	for _, l := range m.lines {
		if l.RuleName == line.RuleName {
			count++
		}
	}
	m.notification = fmt.Sprintf("Filtered rule: %s (%d lines)", line.RuleName, count)
	m.notificationT = time.Now()
	m.refreshVisibleState()
}

func (m *Model) resetFilters() {
	hiddenCount := len(m.hiddenIndices)
	ruleCount := len(m.filteredRules)
	m.filteredRules = make(map[string]bool)
	m.hiddenIndices = make(map[int]bool)
	m.notification = fmt.Sprintf("Reset filters (%d lines, %d rules restored)", hiddenCount, ruleCount)
	m.notificationT = time.Now()
	m.refreshVisibleState()
}

func (m Model) getVisibleLines() []displayLine {
	visible := make([]displayLine, 0, len(m.lines))
	for _, line := range m.lines {
		if line.RuleName != "" && m.filteredRules[line.RuleName] {
			continue
		}
		if m.hiddenIndices[line.Index] {
			continue
		}
		visible = append(visible, line)
	}
	return visible
}

func (m *Model) openDetail() {
	if m.detailOpen {
		return
	}
	line, ok := m.selectedLine()
	if !ok {
		return
	}
	m.detailLine = line
	m.detailOpen = true
	m.updateDetailViewportSize()
	m.detailViewport.GotoTop()
	m.refreshDetailContent()
}

func (m *Model) closeDetail() {
	m.detailOpen = false
	m.detailLine = displayLine{}
}

func (m *Model) openHelp() {
	if m.helpOpen {
		return
	}
	m.helpOpen = true
	m.updateHelpViewportSize()
	m.helpViewport.GotoTop()
}

func (m *Model) refreshDetailContent() {
	if !m.detailOpen {
		m.detailContent = "no alert selected"
	} else {
		m.detailContent = m.buildDetailContent(m.detailLine)
	}
	width := m.detailViewport.Width
	if width <= 0 {
		width = 60
	}
	wrapped := wrapText(m.detailContent, width)
	m.detailViewport.SetContent(wrapped)
}

func (m Model) buildDetailContent(line displayLine) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Severity: %s\n", strings.ToUpper(string(line.Severity)))
	if line.RuleName != "" {
		fmt.Fprintf(&b, "Rule: %s\n", line.RuleName)
	} else {
		fmt.Fprintf(&b, "Rule: (unmatched)\n")
	}
	fmt.Fprintf(&b, "File: %s\n", line.Path)
	fmt.Fprintf(&b, "Timestamp: %s\n", line.Timestamp.Format(time.RFC3339))
	if len(line.Tags) > 0 {
		fmt.Fprintf(&b, "Tags: %s\n", strings.Join(line.Tags, ", "))
	}
	if text := strings.TrimSpace(line.Text); text != "" {
		fmt.Fprintf(&b, "\nLog Entry:\n%s\n", line.Text)
	}
	if combined := strings.TrimSpace(highlight.String(line.Fragments)); combined != "" && combined != strings.TrimSpace(line.Text) {
		fmt.Fprintf(&b, "\nHighlighted:\n%s\n", combined)
	}
	return b.String()
}

func (m Model) modalSize() (int, int) {
	width := m.windowWidth
	if width <= 0 {
		width = m.viewport.Width + m.sidebarWidth + 6
	}
	height := m.windowHeight
	if height <= 0 {
		height = m.viewport.Height + 6
	}
	modalWidth := width * 8 / 10
	if modalWidth < 40 {
		modalWidth = width - 4
	}
	if modalWidth < 20 {
		modalWidth = width
	}
	if modalWidth > width-2 {
		modalWidth = width - 2
	}
	if modalWidth < 20 {
		modalWidth = 20
	}
	modalHeight := height * 8 / 10
	if modalHeight < 12 {
		modalHeight = height - 2
	}
	if modalHeight > height-2 {
		modalHeight = height - 2
	}
	if modalHeight < 10 {
		modalHeight = 10
	}
	return modalWidth, modalHeight
}

func (m *Model) updateDetailViewportSize() {
	if !m.detailOpen {
		return
	}
	width, height := m.modalSize()
	innerWidth := width - (modalPaddingX * 2) - 2
	if innerWidth < 20 {
		innerWidth = 20
	}
	innerHeight := height - (modalPaddingY * 2) - 2 - modalChromeLines
	if innerHeight < 3 {
		innerHeight = 3
	}
	m.detailViewport.Width = innerWidth
	m.detailViewport.Height = innerHeight
	m.refreshDetailContent()
}

func (m *Model) updateHelpViewportSize() {
	if !m.helpOpen {
		return
	}
	width, height := m.modalSize()
	innerWidth := width - (modalPaddingX * 2) - 2
	if innerWidth < 40 {
		innerWidth = 40
	}
	innerHeight := height - (modalPaddingY * 2) - 4
	if innerHeight < 10 {
		innerHeight = 10
	}
	m.helpViewport.Width = innerWidth
	m.helpViewport.Height = innerHeight
	helpText := `
NAVIGATION
  ↑ / ↓         Move selection up/down
  PgUp / PgDn   Page up/down
  
ACTIONS
  Enter         Open alert details
  h             Hide current line
  x             Filter out all logs of this rule type
  r             Reset all filters (show everything)
  
DETAIL VIEW (when alert open)
  y / c         Copy alert details to clipboard
  ↑ / ↓         Scroll detail content
  Enter / Esc   Close detail view
  
PLAYBACK
  p             Pause/unpause log streaming
  f             Toggle auto-follow (scroll to bottom)
  
APPEARANCE
  t             Cycle themes (vapor → midnight → dusk)
  
OTHER
  ?             Show this help
  q / Ctrl+C    Quit application
  
TIPS
  • Pause (p) to stop scrolling while reviewing logs
  • Filter (x) noisy rules to focus on important events
  • Copy (y/c) alert details to share with your team
  • Fullscreen terminal shows severity counts in sidebar
`
	m.helpViewport.SetContent(strings.TrimSpace(helpText))
}

func (m *Model) copyDetailToClipboard() {
	if !m.detailOpen {
		m.notification = "No alert to copy"
		m.notificationT = time.Now()
		return
	}
	content := m.buildDetailContent(m.detailLine)
	var cmd *exec.Cmd
	if goruntime.GOOS == "darwin" {
		cmd = exec.Command("pbcopy")
	} else if goruntime.GOOS == "linux" {
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		}
	}
	if cmd == nil {
		m.notification = "Clipboard not supported on this system"
		m.notificationT = time.Now()
		return
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		m.notification = fmt.Sprintf("Clipboard error: %v", err)
		m.notificationT = time.Now()
		return
	}
	if err := cmd.Start(); err != nil {
		m.notification = fmt.Sprintf("Clipboard error: %v", err)
		m.notificationT = time.Now()
		return
	}
	if _, err := io.WriteString(stdin, content); err != nil {
		stdin.Close()
		m.notification = fmt.Sprintf("Clipboard error: %v", err)
		m.notificationT = time.Now()
		return
	}
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		m.notification = fmt.Sprintf("Clipboard error: %v", err)
		m.notificationT = time.Now()
		return
	}
	m.notification = "Copied alert details to clipboard"
	m.notificationT = time.Now()
}

func (m Model) renderDetailModal() string {
	width, height := m.modalSize()
	title := m.theme.Header.Render("alert details")
	instructions := m.theme.TagStyle.Render("y/c copy · enter/esc close · arrows scroll")
	body := m.detailViewport.View()
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.accentColor()).
		Width(width).
		Height(height).
		Padding(modalPaddingY, modalPaddingX).
		Background(lipgloss.Color("#1A0F1F")).
		Align(lipgloss.Left)
	content := lipgloss.JoinVertical(lipgloss.Left, title, instructions, body)
	return modalStyle.Render(content)
}

func (m Model) renderHelpModal() string {
	width, height := m.modalSize()
	title := m.theme.Header.Render("keyboard shortcuts")
	instructions := lipgloss.NewStyle().
		Foreground(m.accentColor()).
		Italic(true).
		Render("↑/↓ scroll · q/esc/enter/? close")
	body := m.helpViewport.View()
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.accentColor()).
		Width(width).
		Height(height).
		Padding(modalPaddingY, modalPaddingX).
		Background(lipgloss.Color("#1A0F1F")).
		Align(lipgloss.Left)
	content := lipgloss.JoinVertical(lipgloss.Left, title, instructions, body)
	return modalStyle.Render(content)
}

func (m Model) View() string {
	if m.windowWidth <= 0 || m.windowHeight <= 0 {
		return "Loading..."
	}

	sidebarWidth := clamp(m.sidebarWidth, 20, 40)
	if m.windowWidth < sidebarWidth+20 {
		sidebarWidth = clamp(m.windowWidth/3, 18, 40)
	}
	m.sidebarWidth = sidebarWidth

	header := ""
	headerHeight := 0
	if m.showHeader {
		header = m.renderHeader()
		headerHeight = lipgloss.Height(header)
	}

	status := ""
	statusHeight := 0
	if m.showStatus {
		status = m.renderStatus()
		statusHeight = lipgloss.Height(status)
	}

	availableBodyHeight := m.windowHeight - headerHeight - statusHeight
	if availableBodyHeight < 3 {
		availableBodyHeight = 3
	}

	paneView := m.theme.Pane.Render(m.viewport.View())
	sidebarContent := m.renderSidebar(availableBodyHeight)
	sidebarView := m.theme.Sidebar.Render(sidebarContent)

	paneHeight := lipgloss.Height(paneView)
	sidebarHeight := lipgloss.Height(sidebarView)
	maxHeight := paneHeight
	if sidebarHeight > maxHeight {
		maxHeight = sidebarHeight
	}

	if maxHeight > availableBodyHeight {
		_, paneFrameH := m.theme.Pane.GetFrameSize()
		desiredViewportHeight := availableBodyHeight - paneFrameH
		if desiredViewportHeight < 1 {
			desiredViewportHeight = 1
		}

		viewportContent := m.viewport.View()
		lines := strings.Split(viewportContent, "\n")
		if len(lines) > desiredViewportHeight {
			lines = lines[:desiredViewportHeight]
			viewportContent = strings.Join(lines, "\n")
		}

		paneView = m.theme.Pane.Render(viewportContent)

		_, sidebarFrameH := m.theme.Sidebar.GetFrameSize()
		desiredSidebarHeight := availableBodyHeight - sidebarFrameH
		if desiredSidebarHeight < 1 {
			desiredSidebarHeight = 1
		}
		sidebarContent = m.renderSidebar(desiredSidebarHeight)
		sidebarView = m.theme.Sidebar.Render(sidebarContent)

		paneHeight = lipgloss.Height(paneView)
		sidebarHeight = lipgloss.Height(sidebarView)
	}

	targetHeight := paneHeight
	if sidebarHeight > targetHeight {
		targetHeight = sidebarHeight
	}
	if paneHeight < targetHeight {
		paneView = lipgloss.NewStyle().Height(targetHeight).Render(paneView)
	}
	if sidebarHeight < targetHeight {
		sidebarView = lipgloss.NewStyle().Height(targetHeight).Render(sidebarView)
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top, paneView, sidebarView)
	segments := make([]string, 0, 3)
	if header != "" {
		segments = append(segments, header)
	}
	segments = append(segments, body)
	if status != "" {
		segments = append(segments, status)
	}
	result := lipgloss.JoinVertical(lipgloss.Left, segments...)

	if lipgloss.Height(result) > m.windowHeight {
		lines := strings.Split(result, "\n")
		if len(lines) > m.windowHeight {
			lines = lines[:m.windowHeight]
		}
		result = strings.Join(lines, "\n")
	}

	if m.helpOpen {
		modal := m.renderHelpModal()
		return lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, modal,
			lipgloss.WithWhitespaceChars(" "),
			lipgloss.WithWhitespaceBackground(lipgloss.Color("#05010A")))
	}
	if m.config.open {
		modal := m.renderConfigModal()
		return lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, modal,
			lipgloss.WithWhitespaceChars(" "),
			lipgloss.WithWhitespaceBackground(lipgloss.Color("#05010A")))
	}
	if m.detailOpen {
		modal := m.renderDetailModal()
		return lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, modal,
			lipgloss.WithWhitespaceChars(" "),
			lipgloss.WithWhitespaceBackground(lipgloss.Color("#05010A")))
	}

	return result
}

func (m Model) constrainToWindow(view string) string {
	if m.windowWidth <= 0 || m.windowHeight <= 0 {
		return view
	}
	viewHeight := lipgloss.Height(view)
	viewWidth := lipgloss.Width(view)
	if viewHeight <= m.windowHeight && viewWidth <= m.windowWidth {
		return view
	}
	if viewHeight > m.windowHeight {
		lines := strings.Split(view, "\n")
		if len(lines) > m.windowHeight {
			lines = lines[:m.windowHeight]
		}
		view = strings.Join(lines, "\n")
	}
	return view
}

func (m Model) renderHeader() string {
	if !m.showHeader {
		return ""
	}
	return m.theme.Header.Render(m.renderHeaderInfo())
}

func (m Model) renderSidebar(maxHeight int) string {
	sections := []string{}
	wideTerminal := m.windowWidth > 0 && m.windowWidth > 140
	mediumTerminal := m.windowWidth > 0 && m.windowWidth > 100
	appendSection := func(content string, essential bool) {
		if strings.TrimSpace(content) == "" {
			return
		}
		if maxHeight > 0 {
			candidate := append(append([]string{}, sections...), content)
			height := lipgloss.Height(strings.Join(candidate, "\n\n"))
			if height > maxHeight && !essential {
				return
			}
		}
		sections = append(sections, content)
	}

	if mediumTerminal {
		if eye := m.renderEyeball(); strings.TrimSpace(eye) != "" {
			appendSection(eye, false)
		}
	}

	var files strings.Builder
	files.WriteString(m.theme.Header.Render("files"))
	if len(m.activeFiles) == 0 {
		files.WriteString("\n" + m.theme.TagStyle.Render("no files selected"))
	} else {
		for _, file := range m.activeFiles {
			files.WriteString("\n" + m.theme.PillStyle.Render(file))
		}
	}
	appendSection(files.String(), true)

	if wideTerminal {
		var pulse strings.Builder
		pulse.WriteString(m.theme.Header.Render("pulse"))
		order := []rules.Severity{
			rules.SeverityCritical,
			rules.SeverityHigh,
			rules.SeverityMedium,
			rules.SeverityLow,
			rules.SeverityNormal,
		}
		for _, sev := range order {
			count := m.counts[sev]
			pill := m.theme.PillStyle.Copy().Inherit(m.severityStyle(sev)).Render(fmt.Sprintf("%s %d", strings.ToUpper(string(sev)), count))
			pulse.WriteString("\n" + pill)
		}
		appendSection(pulse.String(), false)
	}

	lastSection := fmt.Sprintf("%s\n%s", m.theme.Header.Render("last"), m.theme.TagStyle.Render(coalesce(m.lastRule, "—")))
	appendSection(lastSection, true)

	if m.notification != "" {
		alertStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF61D8")).Padding(0, 1)
		note := fmt.Sprintf("%s\n%s", m.theme.Header.Render("signal"), alertStyle.Render(m.notification))
		appendSection(note, true)
	}

	content := strings.Join(sections, "\n\n")
	if maxHeight > 0 {
		currentHeight := lipgloss.Height(content)
		if currentHeight < maxHeight {
			padding := maxHeight - currentHeight
			content = content + strings.Repeat("\n", padding)
		}
	}
	return content
}

func (m Model) renderStatus() string {
	if !m.showStatus {
		return ""
	}
	state := "streaming"
	if m.paused {
		state = "paused"
	}
	glow := "✧"
	if m.shimmer {
		glow = "✦"
	}
	paneFrameW, _ := m.theme.Pane.GetFrameSize()
	sidebarFrameW, _ := m.theme.Sidebar.GetFrameSize()
	totalWidth := m.viewport.Width + paneFrameW + m.sidebarWidth + sidebarFrameW
	var content string
	if totalWidth < 80 {
		content = fmt.Sprintf("%s %s  ·  ? help  ·  h/x/r  ·  p/f/t/q", glow, state)
	} else if totalWidth < 120 {
		content = fmt.Sprintf("%s %s  ·  ? help  ·  h hide  ·  x filter  ·  r reset  ·  p/f/t/q", glow, state)
	} else {
		content = fmt.Sprintf("%s %s  ·  ? help  ·  h hide  ·  x filter  ·  r reset  ·  p pause  ·  f follow  ·  t theme  ·  q quit", glow, state)
	}
	if totalWidth < 10 {
		totalWidth = 10
	}
	return m.theme.StatusBar.Width(totalWidth).Render(content)
}

func (m Model) renderLogContent() string {
	visibleLines := m.getVisibleLines()
	if len(visibleLines) == 0 {
		if len(m.filteredRules) > 0 || len(m.hiddenIndices) > 0 {
			return "all lines filtered (press 'r' to reset)"
		}
		return "awaiting signals…"
	}
	rows := make([]string, 0, len(visibleLines))
	for idx, line := range visibleLines {
		rows = append(rows, m.renderLine(line, idx == m.selectedIndex))
	}
	return strings.Join(rows, "\n")
}

func (m Model) renderLine(line displayLine, selected bool) string {
	style := m.severityStyle(line.Severity)
	timestamp := m.theme.TagStyle.Copy().Render(line.Timestamp.Format("15:04:05"))
	fragments := renderFragments(line.Fragments, style, m.theme.HighlightStyle)
	meta := style.Copy().Faint(true).Render(line.Path)
	rule := ""
	if line.RuleName != "" {
		rule = m.theme.PillStyle.Copy().Inherit(style).Render(line.RuleName)
	}
	content := fmt.Sprintf("%s %s %s %s", timestamp, fragments, meta, rule)
	if selected {
		indicator := m.theme.HighlightStyle.Copy().Bold(true).Render("➤")
		return lipgloss.JoinHorizontal(lipgloss.Top, indicator, " ", content)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, " ", " ", content)
}

func renderFragments(frags []highlight.Fragment, base, emphasis lipgloss.Style) string {
	if len(frags) == 0 {
		return base.Render("—")
	}
	var b strings.Builder
	for _, frag := range frags {
		sty := base
		if frag.Emphasized {
			sty = emphasis.Inherit(base)
		}
		b.WriteString(sty.Render(frag.Text))
	}
	return b.String()
}

func (m Model) severityStyle(sev rules.Severity) lipgloss.Style {
	if style, ok := m.theme.LevelStyles[sev]; ok {
		return style
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
}

func (m Model) renderEyeball() string {
	if len(eyeFrames) == 0 {
		return ""
	}
	frame := strings.TrimSpace(eyeFrames[m.eyeFrame%len(eyeFrames)])
	sidebarStyle := m.theme.Sidebar
	actualSidebarWidth := m.sidebarWidth
	if w := sidebarStyle.GetWidth(); w > 0 {
		actualSidebarWidth = w
	}
	sidebarFrameW, _ := sidebarStyle.GetFrameSize()
	availableWidth := actualSidebarWidth - sidebarFrameW
	if availableWidth < 6 {
		availableWidth = 6
	}
	eyeBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.accentColor()).
		Foreground(m.accentColor()).
		Padding(0, 1)
	if m.shimmer {
		eyeBoxStyle = eyeBoxStyle.Bold(true)
	}
	eyeBoxFrameW, _ := eyeBoxStyle.GetFrameSize()
	contentWidth := availableWidth - eyeBoxFrameW
	if contentWidth < 4 {
		contentWidth = 4
	}
	lines := strings.Split(frame, "\n")
	for i, line := range lines {
		lines[i] = centerText(line, contentWidth)
	}
	block := strings.Join(lines, "\n")
	return eyeBoxStyle.Render(block)
}

func (m Model) renderHeaderInfo() string {
	parts := []string{
		"Spectra Watch",
		fmt.Sprintf("theme:%s", strings.ToUpper(m.theme.Name)),
		fmt.Sprintf("min:%s", strings.ToUpper(string(m.cfg.MinSeverity))),
		fmt.Sprintf("show:%v", m.cfg.ShowAll),
	}
	return strings.Join(parts, "  ·  ")
}

func (m Model) accentColor() lipgloss.TerminalColor {
	if fg := m.theme.Header.GetForeground(); fg != nil {
		return fg
	}
	return lipgloss.Color("#FF61D8")
}

func (m Model) sidebarContentWidth() int {
	frameW, _ := m.theme.Sidebar.GetFrameSize()
	width := m.sidebarWidth - frameW
	if width < 6 {
		width = 6
	}
	return width
}

func centerText(line string, width int) string {
	if width <= 0 {
		return line
	}
	lw := lipgloss.Width(line)
	if lw >= width {
		return line
	}
	pad := width - lw
	left := pad / 2
	right := pad - left
	return strings.Repeat(" ", left) + line + strings.Repeat(" ", right)
}

func clamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func wrapText(value string, width int) string {
	if width <= 0 {
		return value
	}
	segments := strings.Split(value, "\n")
	wrapped := make([]string, 0, len(segments))
	for _, segment := range segments {
		for _, line := range wrapLine(segment, width) {
			wrapped = append(wrapped, line)
		}
	}
	return strings.Join(wrapped, "\n")
}

func wrapLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}
	runes := []rune(line)
	if len(runes) == 0 {
		return []string{""}
	}
	var lines []string
	for len(runes) > width {
		split := width
		for i := width; i > 0; i-- {
			if unicode.IsSpace(runes[i-1]) {
				split = i
				break
			}
		}
		segment := strings.TrimRightFunc(string(runes[:split]), unicode.IsSpace)
		if segment == "" {
			segment = string(runes[:split])
		}
		lines = append(lines, segment)
		runes = trimLeadingSpaces(runes[split:])
		if len(runes) == 0 {
			return append(lines, "")
		}
	}
	lines = append(lines, string(runes))
	return lines
}

func trimLeadingSpaces(runes []rune) []rune {
	idx := 0
	for idx < len(runes) {
		if runes[idx] != ' ' {
			break
		}
		idx++
	}
	return runes[idx:]
}

var eyeFrames = []string{
	`╭──────╮
│ ╲  ╱ │
│  ◉◉  │
│ ╱  ╲ │
╰──────╯`,
	`╭──────╮
│  ╲╱  │
│ ◉◉◉◉ │
│  ╱╲  │
╰──────╯`,
	`╭──────╮
│ ╲  ╱ │
│ ◉  ◉ │
│ ╱  ╲ │
╰──────╯`,
}

func coalesce(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

func nextTheme(current string) string {
	order := []string{"vapor", "midnight", "dusk"}
	for i, theme := range order {
		if theme == strings.ToLower(current) {
			return order[(i+1)%len(order)]
		}
	}
	return order[0]
}
