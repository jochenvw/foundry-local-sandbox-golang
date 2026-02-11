package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/microsoft/foundry-local/sdk/go/foundrylocal"
)

// StreamingEvent represents a unit of streamed model output.
type StreamingEvent struct {
	Chunk string
	Usage *UsageSummary
	Done  bool
	Err   error
	Stamp time.Time
}

// UsageSummary contains token accounting from the backend.
type UsageSummary struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ContextTokens    int
}

// PromptMessage is sent to the model.
type PromptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Client wraps the Foundry Local manager lifecycle.
type Client struct {
	mu           sync.Mutex
	alias        string
	deviceChoice string

	systemPrompt string
	temperature  float64

	manager   *foundrylocal.Manager
	modelInfo foundrylocal.ModelInfo
}

// NewClient constructs a client; call EnsureLoaded before use.
func NewClient(alias, deviceChoice, systemPrompt string, temperature float64) *Client {
	return &Client{
		alias:        alias,
		deviceChoice: deviceChoice,
		systemPrompt: systemPrompt,
		temperature:  temperature,
	}
}

// Alias returns the current alias/id.
func (c *Client) Alias() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.alias
}

// Temperature returns the active temperature.
func (c *Client) Temperature() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.temperature
}

// SystemPrompt returns the system prompt string.
func (c *Client) SystemPrompt() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.systemPrompt
}

// UpdateTemperature updates the sampling temperature.
func (c *Client) UpdateTemperature(t float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.temperature = t
}

// UpdateSystemPrompt updates the system instruction.
func (c *Client) UpdateSystemPrompt(prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.systemPrompt = prompt
}

// UpdateAlias configures a new model alias or ID.
func (c *Client) UpdateAlias(alias string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.alias = alias
}

// UpdateDevice stores the preferred device filter.
func (c *Client) UpdateDevice(choice string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deviceChoice = choice
}

// ModelInfo returns the cached model info.
func (c *Client) ModelInfo() foundrylocal.ModelInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.modelInfo
}

// EnsureLoaded initializes the Foundry manager and loads the model.
func (c *Client) EnsureLoaded(ctx context.Context) (foundrylocal.ModelInfo, error) {
	c.mu.Lock()
	alias := c.alias
	deviceChoice := c.deviceChoice
	c.mu.Unlock()

	opts := []foundrylocal.Option{foundrylocal.WithModel(alias)}
	if device, ok, _ := ParseDeviceChoice(deviceChoice); ok {
		opts = append(opts, foundrylocal.WithDevice(device))
	}

	mgr, err := foundrylocal.New(ctx, opts...)
	if err != nil {
		return foundrylocal.ModelInfo{}, err
	}

	lookupOpts := []foundrylocal.LookupOption{}
	if device, ok, _ := ParseDeviceChoice(deviceChoice); ok {
		lookupOpts = append(lookupOpts, foundrylocal.ForDevice(device))
	}

	info, err := mgr.LookupModel(ctx, alias, lookupOpts...)
	if err != nil {
		return foundrylocal.ModelInfo{}, err
	}

	c.mu.Lock()
	c.manager = mgr
	c.modelInfo = info
	c.mu.Unlock()

	return info, nil
}

// StreamChat streams a completion for the provided history.
func (c *Client) StreamChat(ctx context.Context, history []PromptMessage) (<-chan StreamingEvent, error) {
	c.mu.Lock()
	mgr := c.manager
	info := c.modelInfo
	temp := c.temperature
	c.mu.Unlock()

	if mgr == nil {
		return nil, fmt.Errorf("model not initialized")
	}

	payload := map[string]any{
		"model":    info.ID,
		"stream":   true,
		"messages": history,
	}
	if temp > 0 {
		payload["temperature"] = temp
	}

	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mgr.Endpoint()+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+mgr.APIKey())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamingEvent)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			ch <- StreamingEvent{Err: fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))}
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" || line == "data: [DONE]" {
				continue
			}
			const prefix = "data: "
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			data := line[len(prefix):]

			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
				Usage *struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				} `json:"usage"`
			}

			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if chunk.Usage != nil {
				ch <- StreamingEvent{
					Usage: &UsageSummary{
						PromptTokens:     chunk.Usage.PromptTokens,
						CompletionTokens: chunk.Usage.CompletionTokens,
						TotalTokens:      chunk.Usage.TotalTokens,
						ContextTokens:    chunk.Usage.TotalTokens,
					},
					Stamp: time.Now(),
				}
			}

			if len(chunk.Choices) > 0 {
				if chunk := chunk.Choices[0].Delta.Content; chunk != "" {
					ch <- StreamingEvent{Chunk: chunk, Stamp: time.Now()}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- StreamingEvent{Err: err}
			return
		}

		ch <- StreamingEvent{Done: true, Stamp: time.Now()}
	}()

	return ch, nil
}
