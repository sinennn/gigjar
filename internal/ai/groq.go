package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://api.groq.com/openai/v1"
	DefaultModel   = "openai/gpt-oss-20b"
)

var (
	ErrNotConfigured = errors.New("AI features are not configured")
	ErrBusy          = errors.New("the AI provider is busy; try again shortly")
	ErrBadOutput     = errors.New("the AI returned an unusable answer")
)

type BusyError struct {
	RetryAfter time.Duration
}

func (e *BusyError) Error() string { return ErrBusy.Error() }
func (e *BusyError) Unwrap() error { return ErrBusy }

type GroqConfig struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
}

type Groq struct {
	cfg GroqConfig
}

func NewGroq(cfg GroqConfig) *Groq {
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Groq{cfg: cfg}
}

func (g *Groq) Configured() bool {
	return g != nil && g.cfg.APIKey != ""
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (g *Groq) Generate(ctx context.Context, system, user, schemaName string, schema map[string]any, maxTokens int, out any) error {
	if !g.Configured() {
		return ErrNotConfigured
	}
	body, err := json.Marshal(map[string]any{
		"model":                 g.cfg.Model,
		"messages":              []message{{Role: "system", Content: system}, {Role: "user", Content: user}},
		"temperature":           0.4,
		"max_completion_tokens": maxTokens,
		"reasoning_effort":      "low",
		"include_reasoning":     false,
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   schemaName,
				"strict": true,
				"schema": schema,
			},
		},
	})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+g.cfg.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := g.cfg.HTTPClient.Do(request)
	if err != nil {
		return &BusyError{}
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return &BusyError{}
	}
	switch {
	case response.StatusCode == http.StatusTooManyRequests:
		seconds, _ := strconv.Atoi(response.Header.Get("retry-after"))
		return &BusyError{RetryAfter: time.Duration(seconds) * time.Second}
	case response.StatusCode >= 500:
		return &BusyError{}
	case response.StatusCode != http.StatusOK:
		return fmt.Errorf("groq returned %d: %s", response.StatusCode, truncate(payload, 300))
	}
	var completion struct {
		Choices []struct {
			Message      message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(payload, &completion); err != nil || len(completion.Choices) == 0 {
		return fmt.Errorf("%w: unreadable completion", ErrBadOutput)
	}
	content := completion.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(content), out); err != nil {
		return fmt.Errorf("%w: %v (finish reason %s)", ErrBadOutput, err, completion.Choices[0].FinishReason)
	}
	return nil
}

func truncate(payload []byte, limit int) string {
	if len(payload) > limit {
		return string(payload[:limit]) + "..."
	}
	return string(payload)
}
