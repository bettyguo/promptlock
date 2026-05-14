package providers

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

// Ollama implements Provider against the local /api/chat endpoint.
type Ollama struct {
	Host   string
	Client *http.Client
}

// NewOllama constructs a default Ollama provider.
func NewOllama() *Ollama {
	return &Ollama{
		Host:   getEnvOr("OLLAMA_HOST", "http://localhost:11434"),
		Client: &http.Client{Timeout: 10 * time.Minute},
	}
}

// Name returns "ollama".
func (*Ollama) Name() string { return "ollama" }

// Call implements Provider.
func (o *Ollama) Call(ctx context.Context, req Request) (Response, error) {
	model := strings.TrimPrefix(req.Model, "ollama/")
	msgs := make([]map[string]string, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	body := map[string]any{
		"model":    model,
		"messages": msgs,
		"stream":   false,
	}
	options := map[string]any{}
	if req.Temperature != nil {
		options["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil {
		options["num_predict"] = *req.MaxTokens
	}
	if req.TopP != nil {
		options["top_p"] = *req.TopP
	}
	if len(req.Stop) > 0 {
		options["stop"] = req.Stop
	}
	if req.Seed != nil {
		options["seed"] = *req.Seed
	}
	if len(options) > 0 {
		body["options"] = options
	}
	buf, _ := json.Marshal(body)

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		strings.TrimRight(o.Host, "/")+"/api/chat", bytes.NewReader(buf))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := o.Client.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("ollama call: %w", err)
	}
	defer httpResp.Body.Close()
	respBody, _ := io.ReadAll(httpResp.Body)

	if httpResp.StatusCode >= 400 {
		return Response{}, fmt.Errorf("ollama %d: %s", httpResp.StatusCode, snippet(respBody))
	}
	var parsed struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		PromptEvalCount int `json:"prompt_eval_count"`
		EvalCount       int `json:"eval_count"`
		Done            bool `json:"done"`
		DoneReason      string `json:"done_reason"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Response{}, fmt.Errorf("ollama decode: %w", err)
	}
	stop := parsed.DoneReason
	if stop == "" && parsed.Done {
		stop = "stop"
	}
	return Response{
		Output:       parsed.Message.Content,
		InputTokens:  parsed.PromptEvalCount,
		OutputTokens: parsed.EvalCount,
		StopReason:   stop,
		Raw:          respBody,
	}, nil
}
