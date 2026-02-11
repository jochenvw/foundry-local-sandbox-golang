package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/microsoft/foundry-local/sdk/go/foundrylocal"
)

// Mode represents the UI mode.
type Mode int

const (
	ModeNormal Mode = iota
	ModeCommand
	ModeSending
)

// UIState keeps track of mode and layout toggles.
type UIState struct {
	Mode        Mode
	Locality    string
	Device      string
	Remote      bool
	ShowStats   bool
	FooterMsg   string
	FooterStamp time.Time
}

// ModelConfig bootstraps the application model.
type ModelConfig struct {
	Alias        string
	DeviceChoice string
	SystemPrompt string
	Temperature  float64
	CtxLimit     int
	Locality     string
	Remote       bool
}

// tea message types

type (
	modelLoadedMsg struct {
		info foundrylocal.ModelInfo
	}
	streamStartedMsg struct {
		events  <-chan StreamingEvent
		started time.Time
		cancel  context.CancelFunc
	}
	streamEventMsg struct {
		event  StreamingEvent
		events <-chan StreamingEvent
	}
	streamFinishedMsg struct{}
	errMsg            struct{ error }
	tickMsg           struct{}
	footerClearMsg    struct{}
)

// Model orchestrates the Bubble Tea update loop.
type Model struct {
	cfg     ModelConfig
	client  *Client
	session *SessionState
	metrics *MetricsState
	ui      *UIState
	styles  Styles

	viewport viewport.Model
	input    textinput.Model

	commandRouter *CommandRouter

	chatCache string
	chatDirty bool

	streamChan   <-chan StreamingEvent
	streamStart  time.Time
	streamCancel context.CancelFunc

	statsWidth int
	winWidth   int
	winHeight  int

	pendingAssistant int
	cursorBlink      bool
	inputHistory     []string
	historyIndex     int
}

// NewModel constructs a Bubble Tea model.
func NewModel(cfg ModelConfig) *Model {
	session := NewSessionState(cfg.Alias, cfg.SystemPrompt, cfg.Temperature, cfg.CtxLimit)
	metrics := NewMetricsState(cfg.CtxLimit)
	styles := NewStyles()

	vp := viewport.New(60, 20)
	vp.SetContent("")
	vp.KeyMap = viewport.KeyMap{}

	input := textinput.New()
	input.Placeholder = "Type a message"
	input.Focus()
	input.CharLimit = 0
	input.Prompt = "> "

	deviceLabel := strings.ToUpper(cfg.DeviceChoice)
	if deviceLabel == "" {
		deviceLabel = "AUTO"
	}
	ui := &UIState{Mode: ModeNormal, Locality: strings.ToUpper(cfg.Locality), Device: deviceLabel, Remote: cfg.Remote, ShowStats: true}

	m := &Model{
		cfg:       cfg,
		client:    NewClient(cfg.Alias, cfg.DeviceChoice, cfg.SystemPrompt, cfg.Temperature),
		session:   session,
		metrics:   metrics,
		ui:        ui,
		styles:    styles,
		viewport:  vp,
		input:     input,
		chatDirty: true,
		pendingAssistant: -1,
		cursorBlink:      false,
		inputHistory:     []string{},
		historyIndex:     0,
	}

	router := &CommandRouter{
		ClearHistory: func() error {
			m.session.Clear()
			m.metrics.Reset()
			m.chatDirty = true
			return nil
		},
		SetTemp: func(v float64) error {
			if v < 0 || v > 2 {
				return fmt.Errorf("temperature out of range (0-2)")
			}
			m.session.UpdateTemperature(v)
			m.client.UpdateTemperature(v)
			return nil
		},
		SetModel: func(alias string) error {
			m.session.UpdateModelAlias(alias)
			m.client.UpdateAlias(alias)
			m.chatDirty = true
			return m.reloadModel()
		},
		ToggleStats: func() error {
			m.ui.ShowStats = !m.ui.ShowStats
			if m.winWidth > 0 && m.winHeight > 0 {
				m.handleWindowSize(m.winWidth, m.winHeight)
			}
			return nil
		},
		Export: func(path string) (string, error) {
			return exportTranscript(path, m.session)
		},
	}
	m.commandRouter = router

	return m
}

// Init starts model loading and UI tick.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.loadModelCmd(), m.tickCmd())
}

// Update processes incoming messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.handleWindowSize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case modelLoadedMsg:
		m.session.UpdateModelAlias(msg.info.Alias)
		m.metrics.CtxLimit = m.session.CtxLimit
		m.ui.FooterMsg = fmt.Sprintf("Loaded %s", msg.info.ID)
		m.ui.FooterStamp = time.Now()
		return m, tea.Batch(tea.ClearScreen, m.tickCmd(), m.footerTimeout())
	case streamStartedMsg:
		m.streamChan = msg.events
		m.streamStart = msg.started
		m.streamCancel = msg.cancel
		m.pendingAssistant = -1
		m.cursorBlink = true
		m.ui.Mode = ModeSending
		m.chatDirty = true
		return m, listenStream(msg.events)
	case streamEventMsg:
		return m.handleStreamEvent(msg)
	case streamFinishedMsg:
		if m.streamCancel != nil {
			m.streamCancel()
			m.streamCancel = nil
		}
		m.streamChan = nil
		latency := time.Since(m.streamStart)
		if latency > 0 {
			rate := 0.0
			if m.metrics.CompletionTokens > 0 {
				rate = float64(m.metrics.CompletionTokens) / latency.Seconds()
			}
			m.metrics.RecordRate(rate, latency)
		}
		m.cursorBlink = false
		m.chatDirty = true
		m.ui.Mode = ModeNormal
		m.input.Focus()
		return m, nil
	case errMsg:
		m.ui.FooterMsg = "error: " + msg.error.Error()
		m.ui.FooterStamp = time.Now()
		m.ui.Mode = ModeNormal
		m.input.Focus()
		if m.pendingAssistant >= 0 {
			m.session.PopLast()
			m.pendingAssistant = -1
			m.chatDirty = true
		}
		if m.cursorBlink {
			m.cursorBlink = false
			m.chatDirty = true
		}
		return m, m.footerTimeout()
	case tickMsg:
		if m.ui.Mode == ModeSending {
			m.cursorBlink = !m.cursorBlink
			m.chatDirty = true
		} else if m.cursorBlink {
			m.cursorBlink = false
			m.chatDirty = true
		}
		return m, m.tickCmd()
	case footerClearMsg:
		m.ui.FooterMsg = ""
		return m, nil
	}

	return m, nil
}

// View renders the UI.
func (m *Model) View() string {
	if m.client.ModelInfo().ID == "" {
		return "Loading model..."
	}

	if m.chatDirty {
		m.refreshChat()
	}

	status := m.styles.StatusLine(m.modeLabel(), m.ui.Locality, m.ui.Device, m.session.ModelAlias, m.ui.Remote)

	chatPane := m.styles.ChatPane.Width(max(0, m.viewport.Width))
	chat := chatPane.Render(m.viewport.View())
	stats := ""
	if m.ui.ShowStats && m.statsWidth > 0 {
		statsPane := m.styles.StatsPane.Width(m.statsWidth)
		stats = statsPane.Render(m.renderStats())
	}
	body := chat
	if stats != "" {
		body = lipgloss.JoinHorizontal(lipgloss.Top, chat, stats)
	}

	promptStyle := m.styles.InputBar
	if m.ui.Mode == ModeCommand {
		promptStyle = m.styles.CommandMode
		m.input.Prompt = ":"
	} else {
		m.input.Prompt = "> "
	}

	inputField := promptStyle.Render(m.input.View())

	footerItems := []string{m.modeLabel(), strings.ToUpper(m.ui.Locality), m.ui.Device}
	footerItems = append(footerItems,
		fmt.Sprintf("temp=%.2f", m.session.Temperature),
		fmt.Sprintf("ctx=%d/%d", m.session.CtxUsed, max(1, m.session.CtxLimit)),
		fmt.Sprintf("%.0f tok/s", m.metrics.TokensPerSecond),
		FormatDuration(m.metrics.Latency),
	)
	footerItems = append(footerItems,
		"quit: Ctrl+C/Ctrl+Q",
		"history: ↑",
		"reset: clear",
	)
	footer := m.styles.FooterLine(footerItems...)

	if m.ui.FooterMsg != "" {
		footer += "\n" + m.styles.FooterBar.Render(m.ui.FooterMsg)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		status,
		body,
		footer,
		inputField,
	)
}

func (m *Model) modeLabel() string {
	switch m.ui.Mode {
	case ModeCommand:
		return "COMMAND ✨"
	case ModeSending:
		return "SENDING 🚀"
	default:
		return "NORMAL 😊"
	}
}


func (m *Model) handleWindowSize(width, height int) {
	chatFrameW, _ := m.styles.ChatPane.GetFrameSize()
	chatMarginTop, chatMarginRight, chatMarginBottom, chatMarginLeft := m.styles.ChatPane.GetMargin()
	statsFrameW, _ := m.styles.StatsPane.GetFrameSize()
	_, statsMarginRight, _, statsMarginLeft := m.styles.StatsPane.GetMargin()

	const statsMinInner = 28
	const minChatInner = 40

	statsInner := 0
	statsOuter := 0
	if m.ui.ShowStats {
		statsInner = statsMinInner
		statsOuter = statsInner + statsFrameW + statsMarginLeft + statsMarginRight
	}

	chatMargins := chatMarginLeft + chatMarginRight
	available := width - statsOuter - chatMargins
	chatInner := available - chatFrameW
	if chatInner < minChatInner {
		chatInner = minChatInner
		if m.ui.ShowStats {
			remaining := width - (chatInner + chatFrameW + chatMargins)
			statsInner = remaining - (statsFrameW + statsMarginLeft + statsMarginRight)
			if statsInner < 0 {
				statsInner = 0
			}
		}
	}
	if chatInner < 1 {
		chatInner = 1
	}

	m.viewport.Width = chatInner
	m.viewport.Height = max(10, height-8-chatMarginTop-chatMarginBottom)
	m.statsWidth = statsInner
	m.winWidth = width
	m.winHeight = height
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "ctrl+q":
		return m, tea.Quit
	}
	switch m.ui.Mode {
	case ModeSending:
		return m, nil
	case ModeCommand:
		switch msg.Type {
		case tea.KeyEsc:
			m.ui.Mode = ModeNormal
			m.input.Reset()
			m.input.Prompt = "> "
			return m, nil
		case tea.KeyEnter:
			cmd := m.input.Value()
			m.input.Reset()
			m.ui.Mode = ModeNormal
			status, err := m.commandRouter.Execute(cmd)
			if err != nil {
				return m, errorCmd(err)
			}
			if status != "" {
				m.ui.FooterMsg = status
				m.ui.FooterStamp = time.Now()
				return m, m.footerTimeout()
			}
			return m, nil
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	default:
		if msg.String() == ":" {
			m.ui.Mode = ModeCommand
			m.input.Reset()
			m.input.Prompt = ":"
			m.input.Placeholder = "command"
			return m, nil
		}
		switch msg.Type {
		case tea.KeyUp:
			if len(m.inputHistory) == 0 {
				return m, nil
			}
			if m.historyIndex > 0 {
				m.historyIndex--
			} else {
				m.historyIndex = 0
			}
			value := m.inputHistory[m.historyIndex]
			m.input.SetValue(value)
			m.input.CursorEnd()
			return m, nil
		case tea.KeyDown:
			if len(m.inputHistory) == 0 {
				return m, nil
			}
			if m.historyIndex < len(m.inputHistory)-1 {
				m.historyIndex++
				value := m.inputHistory[m.historyIndex]
				m.input.SetValue(value)
				m.input.CursorEnd()
			} else {
				m.historyIndex = len(m.inputHistory)
				m.input.SetValue("")
				m.input.CursorStart()
			}
			return m, nil
		case tea.KeyEnter:
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				m.input.Reset()
				return m, nil
			}
			norm := strings.ToLower(text)
			switch norm {
			case "q", "quit", "quit()", "exit", "exit()":
				return m, tea.Quit
			case "clear":
				m.input.Reset()
				if m.commandRouter != nil && m.commandRouter.ClearHistory != nil {
					if err := m.commandRouter.ClearHistory(); err != nil {
						return m, errorCmd(err)
					}
				}
				m.inputHistory = nil
				m.historyIndex = 0
				m.ui.FooterMsg = "history cleared"
				m.ui.FooterStamp = time.Now()
				return m, m.footerTimeout()
			}
			m.pushHistory(text)
			m.input.Reset()
			return m, m.sendChat(text)
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			m.historyIndex = len(m.inputHistory)
			return m, cmd
		}
	}
}

func (m *Model) sendChat(text string) tea.Cmd {
	if m.streamCancel != nil {
		m.streamCancel()
		m.streamCancel = nil
	}
	m.metrics.BeginExchange()
	promptTokens := m.metrics.AddPromptEstimate(text)
	if promptTokens > 0 {
		m.session.AddCtxEstimate(promptTokens)
	}
	idx := m.session.AddMessage(RoleUser, text)
	_ = idx
	history := buildPromptHistory(m.session)
	m.pendingAssistant = m.session.AddMessage(RoleAssistant, "")
	m.chatDirty = true

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		ch, err := m.client.StreamChat(ctx, history)
		if err != nil {
			cancel()
			return errMsg{err}
		}
		return streamStartedMsg{events: ch, started: time.Now(), cancel: cancel}
	}
}

func (m *Model) handleStreamEvent(msg streamEventMsg) (tea.Model, tea.Cmd) {
	ev := msg.event
	if ev.Err != nil {
		if m.streamCancel != nil {
			m.streamCancel()
			m.streamCancel = nil
		}
		m.streamChan = nil
		return m, errorCmd(ev.Err)
	}
	if ev.Usage != nil {
		m.metrics.UpdateUsage(ev.Usage.PromptTokens, ev.Usage.CompletionTokens, ev.Usage.TotalTokens, ev.Usage.ContextTokens)
		m.session.SetCtxUsage(ev.Usage.ContextTokens, m.session.CtxLimit)
		m.metrics.CtxLimit = m.session.CtxLimit
	}
	if ev.Chunk != "" {
		deltaRunes := m.session.AppendToLast(ev.Chunk)
		if deltaRunes > 0 {
			if tokens := m.metrics.AddCompletionRunes(deltaRunes); tokens > 0 {
				m.session.AddCtxEstimate(tokens)
				if !m.streamStart.IsZero() {
					elapsed := time.Since(m.streamStart).Seconds()
					if elapsed > 0 {
						m.metrics.TokensPerSecond = float64(m.metrics.CompletionTokens) / elapsed
					}
				}
			}
		}
		m.chatDirty = true
	}
	if ev.Done {
		if m.streamCancel != nil {
			m.streamCancel()
			m.streamCancel = nil
		}
		m.streamChan = nil
		latency := time.Since(m.streamStart)
		if latency > 0 {
			rate := 0.0
			if m.metrics.CompletionTokens > 0 {
				rate = float64(m.metrics.CompletionTokens) / latency.Seconds()
			}
			m.metrics.RecordRate(rate, latency)
		}
		m.cursorBlink = false
		m.chatDirty = true
		m.ui.Mode = ModeNormal
		m.input.Focus()
		return m, nil
	}
	return m, listenStream(msg.events)
}

func (m *Model) refreshChat() {
	var b strings.Builder
	alias := m.session.ModelAlias
	msgs := m.session.Messages()
	for i, msg := range msgs {
		rendered := m.styles.RenderMessage(msg, alias, m.viewport.Width)
		if i == len(msgs)-1 && msg.Role == RoleAssistant && m.ui.Mode == ModeSending {
			rendered += m.styles.Cursor.Render(" ")
		}
		b.WriteString(rendered)
		b.WriteString("\n")
	}
	m.chatCache = b.String()
	m.viewport.SetContent(m.chatCache)
	m.viewport.GotoBottom()
	m.chatDirty = false
}

func (m *Model) pushHistory(entry string) {
	trimmed := strings.TrimSpace(entry)
	if trimmed == "" {
		return
	}
	if len(m.inputHistory) == 0 || m.inputHistory[len(m.inputHistory)-1] != trimmed {
		m.inputHistory = append(m.inputHistory, trimmed)
	}
	m.historyIndex = len(m.inputHistory)
}

func (m *Model) renderStats() string {
	rows := []string{
		m.styles.RenderMetricsRow("⚡ Tokens/sec", fmt.Sprintf("%.1f", m.metrics.TokensPerSecond)),
		m.styles.RenderMetricsRow("🧠 Prompt", fmt.Sprintf("%d", m.metrics.PromptTokens)),
		m.styles.RenderMetricsRow("🤖 Completion", fmt.Sprintf("%d", m.metrics.CompletionTokens)),
		m.styles.RenderMetricsRow("📊 Total", fmt.Sprintf("%d", m.metrics.TotalTokens)),
		m.styles.RenderMetricsRow("⏱ Latency", FormatDuration(m.metrics.Latency)),
		m.styles.RenderMetricsRow("🧵 Context", fmt.Sprintf("%d/%d", m.session.CtxUsed, max(1, m.metrics.CtxLimit))),
		"",
		m.styles.Value.Render(m.metrics.SparklineString()),
	}
	return strings.Join(rows, "\n")
}

func (m *Model) loadModelCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		info, err := m.client.EnsureLoaded(ctx)
		if err != nil {
			return errMsg{err}
		}
		return modelLoadedMsg{info: info}
	}
}

func (m *Model) reloadModel() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*3)
	defer cancel()
	_, err := m.client.EnsureLoaded(ctx)
	return err
}

func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m *Model) footerTimeout() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return footerClearMsg{} })
}

func errorCmd(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	return func() tea.Msg { return errMsg{err} }
}

func listenStream(ch <-chan StreamingEvent) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return streamFinishedMsg{}
		}
		return streamEventMsg{event: ev, events: ch}
	}
}

func buildPromptHistory(session *SessionState) []PromptMessage {
	msgs := session.Messages()
	history := make([]PromptMessage, 0, len(msgs)+1)
	if session.SystemPrompt != "" {
		history = append(history, PromptMessage{Role: string(RoleSystem), Content: session.SystemPrompt})
	}
	for i, msg := range msgs {
		// Skip trailing assistant placeholder (empty content)
		if msg.Role == RoleAssistant && i == len(msgs)-1 && msg.Content() == "" {
			continue
		}
		history = append(history, PromptMessage{Role: string(msg.Role), Content: msg.Content()})
	}
	return history
}

func exportTranscript(path string, session *SessionState) (string, error) {
	var b strings.Builder
	for _, msg := range session.Messages() {
		b.WriteString(fmt.Sprintf("[%s] %s\n", msg.Created.Format(time.RFC3339), strings.ToUpper(string(msg.Role))))
		b.WriteString(msg.Content())
		b.WriteString("\n\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
