package app

import (
	"strings"
	"time"
	"unicode/utf8"
)
// Role represents the source of a message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single chat entry with incremental rendering support.
type Message struct {
	Role     Role
	Created  time.Time
	parts    []string
	cached   string
	rendered bool
}

// Append adds new text to the message and returns the rune count appended.
func (m *Message) Append(text string) int {
	if text == "" {
		return 0
	}
	m.parts = append(m.parts, text)
	m.rendered = false
	return utf8.RuneCountInString(text)
}

// Content returns the full text for the message.
func (m *Message) Content() string {
	if m.rendered {
		return m.cached
	}
	m.cached = strings.Join(m.parts, "")
	m.rendered = true
	return m.cached
}

// SessionState stores chat history and conversational parameters.
type SessionState struct {
	messages     []*Message
	SystemPrompt string
	Temperature  float64
	ModelAlias   string
	CtxUsed      int
	CtxLimit     int
}

// NewSessionState creates a default session state.
func NewSessionState(alias string, systemPrompt string, temp float64, ctxLimit int) *SessionState {
	return &SessionState{
		SystemPrompt: systemPrompt,
		Temperature:  temp,
		ModelAlias:   alias,
		CtxLimit:     ctxLimit,
	}
}

// Messages returns the chat history.
func (s *SessionState) Messages() []*Message {
	return s.messages
}

// Clear removes all chat messages.
func (s *SessionState) Clear() {
	s.messages = nil
	s.CtxUsed = 0
}

// AddMessage appends a new message and returns its index.

func (s *SessionState) AddMessage(role Role, content string) int {
	msg := &Message{Role: role, Created: time.Now()}
	if content != "" {
		msg.Append(content)
	}
	s.messages = append(s.messages, msg)
	return len(s.messages) - 1
}

// PopLast removes the most recent message, if any.
func (s *SessionState) PopLast() {
	if len(s.messages) == 0 {
		return
	}
	s.messages = s.messages[:len(s.messages)-1]
}

// AppendToLast appends text to the most recent message.
func (s *SessionState) AppendToLast(text string) int {
 	if len(s.messages) == 0 {
		return 0
	}
	return s.messages[len(s.messages)-1].Append(text)
}

// AddCtxEstimate increases the tracked context usage heuristically.
func (s *SessionState) AddCtxEstimate(delta int) {
	if delta <= 0 {
		return
	}
	s.CtxUsed += delta
	if s.CtxLimit > 0 && s.CtxUsed > s.CtxLimit {
		s.CtxUsed = s.CtxLimit
	}
}

// SetCtxUsage updates context usage metrics.
func (s *SessionState) SetCtxUsage(used, limit int) {
	s.CtxUsed = used
	if limit > 0 {
		s.CtxLimit = limit
	}
}

// UpdateTemperature sets chat temperature.
func (s *SessionState) UpdateTemperature(t float64) {
	s.Temperature = t
}

// UpdateSystemPrompt sets the system prompt.
func (s *SessionState) UpdateSystemPrompt(prompt string) {
	s.SystemPrompt = prompt
}

// UpdateModelAlias sets the current model alias or ID.
func (s *SessionState) UpdateModelAlias(alias string) {
	s.ModelAlias = alias
}
