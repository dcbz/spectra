package tui

import (
	"fmt"
	"strings"
	"time"

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
	config         configState
	windowWidth    int
	windowHeight   int
	showHeader     bool
	showStatus     bool
}

type displayLine struct {
	Severity  rules.Severity
	RuleName  string
	Path      string
	Timestamp time.Time
	Fragments []highlight.Fragment
	Tags      []string
	Text      string
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
		config:         newConfigState(),
		showHeader:     true,
		showStatus:     true,
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
		if msg.Height > 0 {
			if headerHeight+statusHeight+minBody > msg.Height {
				m.showHeader = false
				headerHeight = 0
				if statusHeight+minBody > msg.Height {
					m.showStatus = false
					statusHeight = 0
				}
			}
		}
		totalHeight := msg.Height - headerHeight - statusHeight
		if totalHeight < minBody {
			totalHeight = minBody
		}
		contentHeight := totalHeight - paneFrameH
		if contentHeight < 1 {
			contentHeight = 1
		}
		if contentHeight > totalHeight {
			contentHeight = totalHeight
		}
		if contentHeight > msg.Height {
			contentHeight = msg.Height
		}
		m.viewport.Height = contentHeight
		m.viewport.SetContent(m.renderLogContent())
		m.ensureSelectionVisible()
		if m.detailOpen {
			m.updateDetailViewportSize()
		}
	case tea.KeyMsg:
		if m.config.open {
			return m.handleConfigKey(msg)
		}
		if m.detailOpen {
			switch msg.String() {
			case "enter", "esc", "q":
				m.closeDetail()
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
	}
	m.lines = append(m.lines, dl)
	if len(m.lines) > m.scrollback {
		trim := len(m.lines) - m.scrollback
		m.lines = m.lines[trim:]
		if m.selectedIndex >= 0 {
			m.selectedIndex -= trim
			if m.selectedIndex < 0 {
				m.selectedIndex = 0
			}
		}
	}
	if len(m.lines) == 0 {
		m.selectedIndex = -1
	} else if m.follow || m.selectedIndex == -1 {
		m.selectedIndex = len(m.lines) - 1
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
	if m.detailOpen {
		m.refreshDetailContent()
	}
	return m, m.listen()
}

func (m *Model) moveSelection(delta int) {
	if len(m.lines) == 0 {
		m.selectedIndex = -1
		return
	}
	if m.selectedIndex < 0 {
		m.selectedIndex = len(m.lines) - 1
	}
	target := m.selectedIndex + delta
	if target < 0 {
		target = 0
	}
	if target >= len(m.lines) {
		target = len(m.lines) - 1
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
	if m.selectedIndex < 0 || m.selectedIndex >= len(m.lines) {
		return displayLine{}, false
	}
	return m.lines[m.selectedIndex], true
}

func (m *Model) openDetail() {
	if m.detailOpen {
		return
	}
	if _, ok := m.selectedLine(); !ok {
		return
	}
	m.detailOpen = true
	m.updateDetailViewportSize()
	m.refreshDetailContent()
	m.detailViewport.GotoTop()
}

func (m *Model) closeDetail() {
	m.detailOpen = false
}

func (m *Model) refreshDetailContent() {
	line, ok := m.selectedLine()
	if !ok {
		m.detailContent = "no alert selected"
	} else {
		m.detailContent = m.buildDetailContent(line)
	}
	m.detailViewport.SetContent(m.detailContent)
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
}

func (m Model) renderDetailModal() string {
	width, height := m.modalSize()
	title := m.theme.Header.Render("alert details")
	instructions := m.theme.TagStyle.Render("enter/esc close · arrows scroll")
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

func (m Model) View() string {
	// Clamp sidebar width for very small terminals so panes don't collapse.
	minSidebar := 20
	maxSidebar := 40
	sidebarWidth := m.sidebarWidth
	if m.windowWidth > 0 {
		avail := m.windowWidth - 10
		if avail < minSidebar {
			avail = minSidebar
		}
		if avail < sidebarWidth {
			sidebarWidth = avail
		}
	}
	if sidebarWidth < minSidebar {
		sidebarWidth = minSidebar
	}
	if sidebarWidth > maxSidebar {
		sidebarWidth = maxSidebar
	}
	m.sidebarWidth = sidebarWidth

	paneFrameW, paneFrameH := m.theme.Pane.GetFrameSize()
	bodyHeight := m.viewport.Height + paneFrameH
	if bodyHeight < 3 {
		bodyHeight = 3
	}
	if m.windowHeight > 0 && bodyHeight > m.windowHeight {
		bodyHeight = m.windowHeight
	}
	paneWidth := m.viewport.Width + paneFrameW
	if paneWidth < 10 {
		paneWidth = 10
	}
	paneView := m.theme.Pane.Width(paneWidth).Render(m.viewport.View())
	sidebarContent := m.renderSidebar(bodyHeight)
	sidebarPane := m.theme.Sidebar.Width(m.sidebarWidth)
	sidebarView := sidebarPane.Render(sidebarContent)

	paneView = lipgloss.NewStyle().Height(bodyHeight).Render(paneView)
	sidebarView = lipgloss.NewStyle().Height(bodyHeight).Render(sidebarView)
	body := lipgloss.JoinHorizontal(lipgloss.Top, paneView, sidebarView)
	header := m.renderHeader()
	status := m.renderStatus()
	segments := make([]string, 0, 3)
	if header != "" {
		segments = append(segments, header)
	}
	segments = append(segments, body)
	if status != "" {
		segments = append(segments, status)
	}
	base := lipgloss.JoinVertical(lipgloss.Left, segments...)
	if m.config.open {
		modal := m.renderConfigModal()
		width := m.windowWidth
		height := m.windowHeight
		if width <= 0 {
			width = lipgloss.Width(base)
		}
		if height <= 0 {
			height = lipgloss.Height(base)
		}
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal,
			lipgloss.WithWhitespaceChars(" "),
			lipgloss.WithWhitespaceBackground(lipgloss.Color("#05010A")))
	}
	if !m.detailOpen {
		return m.constrainToWindow(base)
	}
	modal := m.renderDetailModal()
	width := m.windowWidth
	height := m.windowHeight
	if width <= 0 {
		width = lipgloss.Width(base)
	}
	if height <= 0 {
		height = lipgloss.Height(base)
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceBackground(lipgloss.Color("#05010A")))
}

func (m Model) constrainToWindow(view string) string {
	if m.windowWidth <= 0 || m.windowHeight <= 0 {
		return view
	}
	return lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Left, lipgloss.Top, view,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceBackground(lipgloss.Color("#05010A")))
}

func (m Model) renderHeader() string {
	if !m.showHeader {
		return ""
	}
	return m.theme.Header.Render(m.renderHeaderInfo())
}

func (m Model) renderSidebar(maxHeight int) string {
	sections := []string{}
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

	if eye := m.renderEyeball(); strings.TrimSpace(eye) != "" {
		appendSection(eye, false)
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

	lastSection := fmt.Sprintf("%s\n%s", m.theme.Header.Render("last"), m.theme.TagStyle.Render(coalesce(m.lastRule, "—")))
	appendSection(lastSection, true)

	if m.notification != "" {
		alertStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF61D8")).Padding(0, 1)
		note := fmt.Sprintf("%s\n%s", m.theme.Header.Render("signal"), alertStyle.Render(m.notification))
		appendSection(note, true)
	}

	return strings.Join(sections, "\n\n")
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
	content := fmt.Sprintf("%s %s  ·  ↑/↓ select  ·  PgUp/PgDn page  ·  enter detail  ·  esc close  ·  p pause  ·  f follow  ·  t theme  ·  c configure  ·  q quit", glow, state)
	paneFrameW, _ := m.theme.Pane.GetFrameSize()
	sidebarFrameW, _ := m.theme.Sidebar.GetFrameSize()
	width := m.viewport.Width + paneFrameW + m.sidebarWidth + sidebarFrameW
	if width < 10 {
		width = 10
	}
	return m.theme.StatusBar.Width(width).Render(content)
}

func (m Model) renderLogContent() string {
	var rows []string
	for idx, line := range m.lines {
		rows = append(rows, m.renderLine(line, idx == m.selectedIndex))
	}
	if len(rows) == 0 {
		return "awaiting signals…"
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
	cw := m.sidebarContentWidth()
	lines := strings.Split(frame, "\n")
	for i, line := range lines {
		lines[i] = centerText(line, cw-4)
	}
	block := strings.Join(lines, "\n")
	if cw < 8 {
		cw = 8
	}
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(m.accentColor()).Foreground(m.accentColor()).Padding(0, 1).Width(cw)
	if m.shimmer {
		style = style.Bold(true)
	}
	return style.Render(block)
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
