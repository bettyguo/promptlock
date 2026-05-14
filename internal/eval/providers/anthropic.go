package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Anthropic implements Provider against the Messages API.
type Anthropic struct {
	APIKey  string
	BaseURL string
	Client  *http.Client
}

// NewAnthropic constructs a default Anthropic provider reading auth from env.
// API key resolution is deferred to Call so missing-env errors propagate
// cleanly through the eval pipeline (rather than panicking at construction).
func NewAnthropic() *Anthropic {
	return &Anthropic{
		BaseURL: getEnvOr("ANTHROPIC_BASE_URL", "https://api.anthropic.com"),
		Client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

// Name returns "anthropic".
func (*Anthropic) Name() string { return "anthropic" }

func (a *Anthropic) apiKey() (string, error) {
	if a.APIKey != "" {
		return a.APIKey, nil
	}
	k := os.Getenv("ANTHROPIC_API_KEY")
	if k == "" {
		return "", &ErrMissingAPIKey{Provider: "anthropic", EnvVar: "ANTHROPIC_API_KEY"}
	}
	return k, nil
}

// Call implements Provider.
func (a *Anthropic) Call(ctx context.Context, req Request) (Response, error) {
	key, err := a.apiKey()
	if err != nil {
		return Response{}, err
	}
	body := map[string]any{
		"model":      req.Model,
		"max_tokens": derefIntOr(req.MaxTokens, 1024),
	}
	if req.System != "" {
		body["system"] = req.System
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}
	if len(req.Stop) > 0 {
		body["stop_sequences"] = req.Stop
	}
	msgs := make([]map[string]string, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	body["messages"] = msgs
	buf, _ := json.Marshal(body)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.BaseURL+"/v1/messages", bytes.NewReader(buf))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", key)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	httpResp, err := a.Client.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("anthropic call: %w", err)
	}
	defer httpResp.Body.Close()
	respBody, _ := io.ReadAll(httpResp.Body)

	if httpResp.StatusCode >= 400 {
		return Response{}, fmt.Errorf("anthropic %d: %s", httpResp.StatusCode, snippet(respBody))
	}

	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		StopReason string `json:"stop_reason"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Response{}, fmt.Errorf("anthropic decode: %w", err)
	}
	var out string
	for _, c := range parsed.Content {
		if c.Type == "text" {
			out += c.Text
		}
	}
	return Response{
		Output:       out,
		InputTokens:  parsed.Usage.InputTokens,
		OutputTokens: parsed.Usage.OutputTokens,
		StopReason:   parsed.StopReason,
		Raw:          respBody,
	}, nil
}

func getEnvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func derefIntOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func snippet(b []byte) string {
	if len(b) > 500 {
		return string(b[:500]) + "..."
	}
	return string(b)
}
