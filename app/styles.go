package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

// Styles hosts lipgloss styles used across panes.
type Styles struct {
	StatusBar       lipgloss.Style
	StatusBadge     lipgloss.Style
	StatusBadgeWarn lipgloss.Style
	StatusMode      lipgloss.Style

	ChatPane    lipgloss.Style
	StatsPane   lipgloss.Style
	InputBar    lipgloss.Style
	FooterBar   lipgloss.Style
	CommandMode lipgloss.Style

	UserBubble      lipgloss.Style
	AssistantBubble lipgloss.Style
	SystemBubble    lipgloss.Style
	Timestamp       lipgloss.Style
	Label           lipgloss.Style
	Value           lipgloss.Style
	Cursor          lipgloss.Style
}

// NewStyles builds the reusable style sheet.
func NewStyles() Styles {
	base := lipgloss.Color("#1E1E2E")
	accent := lipgloss.Color("#89B4FA")
	local := lipgloss.Color("#A6E3A1")
	remote := lipgloss.Color("#F38BA8")

	status := lipgloss.NewStyle().Background(base).Foreground(lipgloss.Color("#CDD6F4")).Padding(0, 1)

	return Styles{
		StatusBar:       status.Copy().Bold(true),
		StatusBadge:     status.Copy().Background(local).Foreground(base).Bold(true).Padding(0, 1),
		StatusBadgeWarn: status.Copy().Background(remote).Foreground(base).Bold(true).Padding(0, 1),
		StatusMode:      status.Copy().Foreground(accent).Bold(true),

		ChatPane:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#313244")).Padding(0, 1).Margin(0, 1, 0, 0),
		StatsPane:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#45475A")).Padding(0, 1).Margin(0, 0, 0, 0),
		InputBar:    lipgloss.NewStyle().Foreground(lipgloss.Color("#F8F8F2")).Background(lipgloss.Color("#11111B")).Padding(0, 1),
		FooterBar:   lipgloss.NewStyle().Foreground(lipgloss.Color("#BAC2DE")).Background(lipgloss.Color("#11111B")).Padding(0, 1),
		CommandMode: lipgloss.NewStyle().Foreground(lipgloss.Color("#FAB387")).Background(lipgloss.Color("#1A1B26")).Padding(0, 1),

		UserBubble:      lipgloss.NewStyle().Foreground(lipgloss.Color("#94E2D5")).Padding(0, 1).MarginTop(1),
		AssistantBubble: lipgloss.NewStyle().Foreground(lipgloss.Color("#CBA6F7")).Padding(0, 1).MarginTop(1),
		SystemBubble:    lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADC8")).MarginTop(1).Italic(true),
		Timestamp:       lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086")).MarginLeft(1),
		Label:           lipgloss.NewStyle().Foreground(lipgloss.Color("#9399B2")),
		Value:           lipgloss.NewStyle().Foreground(lipgloss.Color("#CDD6F4")).Bold(true),
		Cursor:          lipgloss.NewStyle().Background(lipgloss.Color("#A6E3A1")).Foreground(lipgloss.Color("#1E1E2E")).Bold(true),
	}
}

// RenderMessage formats a chat message for the viewport.
func (s Styles) RenderMessage(msg *Message, alias string, width int) string {
	ts := s.Timestamp.Render(msg.Created.Format("15:04:05"))

	var bubble lipgloss.Style
	var header string
	switch msg.Role {
	case RoleUser:
		bubble = s.UserBubble
		header = "🧑 YOU"
	case RoleAssistant:
		bubble = s.AssistantBubble
		header = fmt.Sprintf("🤖 %s", alias)
	default:
		bubble = s.SystemBubble
		header = "⚙️ SYSTEM"
	}

	content := msg.Content()
	maxWidth := width
	if maxWidth <= 0 {
		maxWidth = 80
	}
	frameWidth, _ := bubble.GetFrameSize()
	innerWidth := maxWidth - frameWidth
	if innerWidth < 10 {
		innerWidth = maxWidth
	}
	content = wrapText(content, innerWidth)
	body := bubble.Render(content)
	return lipgloss.JoinHorizontal(lipgloss.Top, header, ts) + "\n" + body
}

func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		wrapped := wordwrap.String(line, width)
		wrapped = strings.TrimSuffix(wrapped, "\n")
		lines[i] = wrapped
	}
	return strings.Join(lines, "\n")
}

// RenderMetricsRow returns a formatted label/value row.
func (s Styles) RenderMetricsRow(label string, value string) string {
	left := s.Label.Render(label)
	right := s.Value.Render(value)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

// FooterLine builds the footer summary line.
func (s Styles) FooterLine(items ...string) string {
	return s.FooterBar.Render(strings.Join(items, " | "))
}

// StatusLine renders the top status row.
func (s Styles) StatusLine(mode, locality, model string, remote bool) string {
	badge := s.StatusBadge
	if remote {
		badge = s.StatusBadgeWarn
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		s.StatusMode.Render(mode),
		" ",
		badge.Render(locality),
		" ",
		s.StatusBar.Render(model),
	)
}

// FormatDuration renders durations with ms precision.
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "--"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Truncate(10 * time.Millisecond).String()
}
