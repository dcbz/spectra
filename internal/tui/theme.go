package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"watcher/internal/rules"
)

// Theme describes the colors and styles for the UI.
type Theme struct {
	Name           string
	Background     lipgloss.Style
	Pane           lipgloss.Style
	Sidebar        lipgloss.Style
	StatusBar      lipgloss.Style
	Header         lipgloss.Style
	LevelStyles    map[rules.Severity]lipgloss.Style
	HighlightStyle lipgloss.Style
	TagStyle       lipgloss.Style
	PillStyle      lipgloss.Style
}

func themeByName(name string) Theme {
	switch strings.ToLower(name) {
	case "midnight":
		return midnightTheme()
	case "dusk":
		return duskTheme()
	default:
		return vaporTheme()
	}
}

func vaporTheme() Theme {
	gradient := lipgloss.NewStyle().Background(lipgloss.Color("#1B1C30")).Foreground(lipgloss.Color("#E7E7FF"))
	pane := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#9F7AEA")).Padding(1, 2).Background(lipgloss.Color("#1B1C30"))
	sidebar := pane.Copy().BorderForeground(lipgloss.Color("#FF61D8")).Width(28)
	status := lipgloss.NewStyle().Foreground(lipgloss.Color("#1B1C30")).Background(lipgloss.Color("#FF61D8")).Padding(0, 2)
	header := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF61D8")).Bold(true).Underline(true)
	highlight := lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("#FFE066"))
	tag := lipgloss.NewStyle().Foreground(lipgloss.Color("#1B1C30")).Background(lipgloss.Color("#7AF7FF")).Padding(0, 1).Bold(true)
	pill := lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#FF61D8")).Foreground(lipgloss.Color("#FF61D8"))

	levelStyles := map[rules.Severity]lipgloss.Style{
		rules.SeverityCritical: lipgloss.NewStyle().Foreground(lipgloss.Color("#FF61D8")).Bold(true),
		rules.SeverityHigh:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8B5D")).Bold(true),
		rules.SeverityMedium:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FFC857")),
		rules.SeverityLow:      lipgloss.NewStyle().Foreground(lipgloss.Color("#7AF7FF")),
		rules.SeverityNormal:   lipgloss.NewStyle().Foreground(lipgloss.Color("#A4A9FF")),
	}

	return Theme{
		Name:           "vapor",
		Background:     gradient,
		Pane:           pane,
		Sidebar:        sidebar,
		StatusBar:      status,
		Header:         header,
		LevelStyles:    levelStyles,
		HighlightStyle: highlight,
		TagStyle:       tag,
		PillStyle:      pill,
	}
}

func midnightTheme() Theme {
	pane := lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("#00C9A7")).Background(lipgloss.Color("#02070D")).Padding(1, 2)
	sidebar := pane.Copy().BorderForeground(lipgloss.Color("#00E6D2")).Width(26)
	status := lipgloss.NewStyle().Foreground(lipgloss.Color("#02070D")).Background(lipgloss.Color("#00E6D2")).Padding(0, 2)
	header := lipgloss.NewStyle().Foreground(lipgloss.Color("#00E6D2")).Bold(true)
	highlight := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F4F269"))
	tag := lipgloss.NewStyle().Foreground(lipgloss.Color("#02070D")).Background(lipgloss.Color("#00E6D2")).Padding(0, 1)
	pill := lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("#009688")).Foreground(lipgloss.Color("#00E6D2"))

	levelStyles := map[rules.Severity]lipgloss.Style{
		rules.SeverityCritical: lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F5F")).Bold(true),
		rules.SeverityHigh:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA552")).Bold(true),
		rules.SeverityMedium:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FFE066")),
		rules.SeverityLow:      lipgloss.NewStyle().Foreground(lipgloss.Color("#78FECF")),
		rules.SeverityNormal:   lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7A89")),
	}

	return Theme{
		Name:           "midnight",
		Background:     lipgloss.NewStyle().Background(lipgloss.Color("#02070D")).Foreground(lipgloss.Color("#E3FDFD")),
		Pane:           pane,
		Sidebar:        sidebar,
		StatusBar:      status,
		Header:         header,
		LevelStyles:    levelStyles,
		HighlightStyle: highlight,
		TagStyle:       tag,
		PillStyle:      pill,
	}
}

func duskTheme() Theme {
	pane := lipgloss.NewStyle().Border(lipgloss.HiddenBorder()).Background(lipgloss.Color("#211830")).Padding(1, 1)
	sidebar := pane.Copy().Width(25)
	status := lipgloss.NewStyle().Foreground(lipgloss.Color("#211830")).Background(lipgloss.Color("#FFB4A2")).Padding(0, 2)
	header := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB4A2")).Bold(true).Italic(true)
	highlight := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFE066")).Underline(true)
	tag := lipgloss.NewStyle().Foreground(lipgloss.Color("#211830")).Background(lipgloss.Color("#FFD6BA")).Padding(0, 1)
	pill := lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#FFCAD4")).Foreground(lipgloss.Color("#FFCAD4"))

	levelStyles := map[rules.Severity]lipgloss.Style{
		rules.SeverityCritical: lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E5B")).Bold(true),
		rules.SeverityHigh:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA552")).Bold(true),
		rules.SeverityMedium:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FFEAA7")),
		rules.SeverityLow:      lipgloss.NewStyle().Foreground(lipgloss.Color("#A0E8AF")),
		rules.SeverityNormal:   lipgloss.NewStyle().Foreground(lipgloss.Color("#C7CEEA")),
	}

	return Theme{
		Name:           "dusk",
		Background:     lipgloss.NewStyle().Background(lipgloss.Color("#120F16")).Foreground(lipgloss.Color("#F1F2F8")),
		Pane:           pane,
		Sidebar:        sidebar,
		StatusBar:      status,
		Header:         header,
		LevelStyles:    levelStyles,
		HighlightStyle: highlight,
		TagStyle:       tag,
		PillStyle:      pill,
	}
}
