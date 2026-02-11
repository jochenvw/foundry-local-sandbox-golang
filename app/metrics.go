package app

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// MetricsState tracks runtime statistics for the model session.
type MetricsState struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	TokensPerSecond  float64
	Latency          time.Duration
	CtxUsed          int
	CtxLimit         int
	hasUsage         bool

	samples    []float64
	maxSamples int
}

// NewMetricsState constructs a metrics state.
func NewMetricsState(ctxLimit int) *MetricsState {
	return &MetricsState{CtxLimit: ctxLimit, maxSamples: 30}
}

// Reset clears token counters.
func (m *MetricsState) Reset() {
	m.PromptTokens = 0
	m.CompletionTokens = 0
	m.TotalTokens = 0
	m.TokensPerSecond = 0
	m.Latency = 0
	m.samples = nil
	m.CtxUsed = 0
	m.hasUsage = false
}

// BeginExchange prepares counters for a new assistant response cycle.
func (m *MetricsState) BeginExchange() {
	m.hasUsage = false
	m.PromptTokens = 0
	m.CompletionTokens = 0
	m.TotalTokens = 0
	m.TokensPerSecond = 0
	m.Latency = 0
}

// UpdateUsage updates token counts and context usage.
func (m *MetricsState) UpdateUsage(prompt, completion, total, ctxUsed int) {
	m.hasUsage = true
	if prompt >= 0 {
		m.PromptTokens = prompt
	}
	if completion >= 0 {
		m.CompletionTokens = completion
	}
	if total >= 0 {
		m.TotalTokens = total
	}
	if ctxUsed >= 0 {
		m.CtxUsed = ctxUsed
	}
	if m.CtxLimit > 0 && m.CtxUsed > m.CtxLimit {
		m.CtxLimit = m.CtxUsed
	}
}

// AddPromptEstimate increments prompt tokens using a rough heuristic.
func (m *MetricsState) AddPromptEstimate(text string) int {
	tokens := estimateTokens(text)
	if tokens <= 0 {
		return 0
	}
	if !m.hasUsage {
		m.PromptTokens += tokens
		m.TotalTokens += tokens
		m.CtxUsed += tokens
		if m.CtxLimit > 0 && m.CtxUsed > m.CtxLimit {
			m.CtxLimit = m.CtxUsed
		}
	}
	return tokens
}

// AddCompletionRunes increments completion tokens from a rune delta.
func (m *MetricsState) AddCompletionRunes(runes int) int {
	tokens := estimateTokensFromRunes(runes)
	if tokens <= 0 {
		return 0
	}
	if !m.hasUsage {
		m.CompletionTokens += tokens
		m.TotalTokens += tokens
		m.CtxUsed += tokens
		if m.CtxLimit > 0 && m.CtxUsed > m.CtxLimit {
			m.CtxLimit = m.CtxUsed
		}
	}
	return tokens
}

// RecordRate adds a tokens/sec sample and updates latency.
func (m *MetricsState) RecordRate(rate float64, latency time.Duration) {
	if latency > 0 {
		m.Latency = latency
	}
	if rate > 0 {
		m.TokensPerSecond = rate
		m.addSample(rate)
	}
}

// SparklineString returns an ASCII sparkline for rate samples.
func (m *MetricsState) SparklineString() string {
	if len(m.samples) == 0 {
		return ""
	}
	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	max := m.samples[0]
	for _, v := range m.samples {
		if v > max {
			max = v
		}
	}
	if max <= 0 {
		max = 1
	}
	r := make([]rune, len(m.samples))
	for i, v := range m.samples {
		idx := int((v / max) * float64(len(blocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		r[i] = blocks[idx]
	}
	return string(r)
}

func (m *MetricsState) addSample(rate float64) {
	m.samples = append(m.samples, rate)
	if len(m.samples) > m.maxSamples {
		m.samples = m.samples[len(m.samples)-m.maxSamples:]
	}
}

func estimateTokens(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	chars := utf8.RuneCountInString(trimmed)
	if chars == 0 {
		return 0
	}
	tokens := int(math.Ceil(float64(chars) / 4.0))
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

func estimateTokensFromRunes(runes int) int {
	if runes <= 0 {
		return 0
	}
	tokens := int(math.Ceil(float64(runes) / 4.0))
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}
