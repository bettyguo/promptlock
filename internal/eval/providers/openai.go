package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OpenAI implements Provider against the chat completions API. Compatible
// endpoints (vLLM, Together, etc.) are reachable via OPENAI_BASE_URL.
type OpenAI struct {
	APIKey  string
	BaseURL string
	Client  *http.Client
}

// NewOpenAI constructs a default OpenAI provider.
func NewOpenAI() *OpenAI {
	return &OpenAI{
		BaseURL: getEnvOr("OPENAI_BASE_URL", "https://api.openai.com"),
		Client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

// Name returns "openai".
func (*OpenAI) Name() string { return "openai" }

func (o *OpenAI) apiKey() (string, error) {
	if o.APIKey != "" {
		return o.APIKey, nil
	}
	k := os.Getenv("OPENAI_API_KEY")
	if k == "" {
		return "", &ErrMissingAPIKey{Provider: "openai", EnvVar: "OPENAI_API_KEY"}
	}
	return k, nil
}

// Call implements Provider.
func (o *OpenAI) Call(ctx context.Context, req Request) (Response, error) {
	key, err := o.apiKey()
	if err != nil {
		return Response{}, err
	}
	msgs := make([]map[string]string, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	body := map[string]any{
		"model":    req.Model,
		"messages": msgs,
	}
	// Reasoning models (o1, o3, o4) don't support temperature — strip silently.
	isReasoning := strings.HasPrefix(req.Model, "o1-") ||
		strings.HasPrefix(req.Model, "o3-") ||
		strings.HasPrefix(req.Model, "o4-")
	if req.Temperature != nil && !isReasoning {
		body["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil {
		// reasoning models use max_completion_tokens; chat models use max_tokens.
		if isReasoning {
			body["max_completion_tokens"] = *req.MaxTokens
		} else {
			body["max_tokens"] = *req.MaxTokens
		}
	}
	if req.TopP != nil && !isReasoning {
		body["top_p"] = *req.TopP
	}
	if len(req.Stop) > 0 && !isReasoning {
		body["stop"] = req.Stop
	}
	if req.Seed != nil {
		body["seed"] = *req.Seed
	}
	buf, _ := json.Marshal(body)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+key)

	httpResp, err := o.Client.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("openai call: %w", err)
	}
	defer httpResp.Body.Close()
	respBody, _ := io.ReadAll(httpResp.Body)

	if httpResp.StatusCode >= 400 {
		return Response{}, fmt.Errorf("openai %d: %s", httpResp.StatusCode, snippet(respBody))
	}

	var parsed struct {
		Choices []struct {
			Message      struct{ Content string `json:"content"` } `json:"message"`
			FinishReason string                                    `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Response{}, fmt.Errorf("openai decode: %w", err)
	}
	out := ""
	stop := ""
	if len(parsed.Choices) > 0 {
		out = parsed.Choices[0].Message.Content
		stop = parsed.Choices[0].FinishReason
	}
	return Response{
		Output:       out,
		InputTokens:  parsed.Usage.PromptTokens,
		OutputTokens: parsed.Usage.CompletionTokens,
		StopReason:   stop,
		Raw:          respBody,
	}, nil
}
