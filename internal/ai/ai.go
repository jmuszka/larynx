package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	APIKey     string
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

type Service struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config) (*Service, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("ai: BaseURL is required")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("ai: Model is required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Service{
		cfg:    cfg,
		client: client,
	}, nil
}

// maxTokens is sized to fit a concise answer plus the model's hidden reasoning
// tokens (deepseek-v4-flash emits `reasoning_content` that counts against the
// output budget). Keeping it comfortably large lets a single request finish with
// finish_reason "stop", avoiding the continuation loop and its extra round trips.
const maxTokens = 1024

type chatRequest struct {
	Model       string        `json:"model"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
	Messages    []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (s *Service) Prompt(ctx context.Context, system, prompt string) (string, error) {
	url := strings.TrimRight(s.cfg.BaseURL, "/")
	if !strings.HasSuffix(url, "/v1") {
		url += "/v1"
	}
	url += "/chat/completions"

	messages := make([]chatMessage, 0, 2)
	if system != "" {
		messages = append(messages, chatMessage{Role: "system", Content: system})
	}
	messages = append(messages, chatMessage{Role: "user", Content: prompt})

	const maxContinuations = 10

	var result strings.Builder
	for i := 0; i <= maxContinuations; i++ {
		reqBody := chatRequest{
			Model:       s.cfg.Model,
			MaxTokens:   maxTokens,
			Temperature: 0.0,
			Messages:    messages,
		}

		b, err := json.Marshal(reqBody)
		if err != nil {
			return "", fmt.Errorf("ai: marshal request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
		if err != nil {
			return "", fmt.Errorf("ai: create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if s.cfg.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
		}

		resp, err := s.client.Do(req)
		if err != nil {
			return "", fmt.Errorf("ai: send request: %w", err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("ai: read response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("ai: HTTP %d: %s", resp.StatusCode, string(body))
		}

		var cr chatResponse
		if err := json.Unmarshal(body, &cr); err != nil {
			return "", fmt.Errorf("ai: unmarshal response: %w", err)
		}

		if cr.Error != nil {
			return "", fmt.Errorf("ai: API error: %s", cr.Error.Message)
		}

		if len(cr.Choices) == 0 {
			return "", fmt.Errorf("ai: no choices in response")
		}

		choice := cr.Choices[0]
		result.WriteString(choice.Message.Content)

		if choice.FinishReason != "length" {
			return result.String(), nil
		}

		// Truncated by max_tokens; continue from where the model left off.
		messages = append(messages, choice.Message)
	}

	return "", fmt.Errorf("ai: exceeded %d continuations without finishing", maxContinuations)
}
