package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Gemini implements Provider against the v1beta generateContent API.
type Gemini struct {
	APIKey  string
	BaseURL string
	Client  *http.Client
}

// NewGemini constructs a default Gemini provider.
func NewGemini() *Gemini {
	return &Gemini{
		BaseURL: getEnvOr("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com"),
		Client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

// Name returns "gemini".
func (*Gemini) Name() string { return "gemini" }

func (g *Gemini) apiKey() (string, error) {
	if g.APIKey != "" {
		return g.APIKey, nil
	}
	k := os.Getenv("GEMINI_API_KEY")
	if k == "" {
		return "", &ErrMissingAPIKey{Provider: "gemini", EnvVar: "GEMINI_API_KEY"}
	}
	return k, nil
}

// Call implements Provider.
func (g *Gemini) Call(ctx context.Context, req Request) (Response, error) {
	key, err := g.apiKey()
	if err != nil {
		return Response{}, err
	}
	contents := []map[string]any{}
	for _, m := range req.Messages {
		role := m.Role
		if role == "assistant" {
			role = "model" // Gemini calls assistant turns "model"
		}
		contents = append(contents, map[string]any{
			"role":  role,
			"parts": []map[string]any{{"text": m.Content}},
		})
	}
	body := map[string]any{"contents": contents}
	if req.System != "" {
		body["systemInstruction"] = map[string]any{
			"parts": []map[string]any{{"text": req.System}},
		}
	}
	gen := map[string]any{}
	if req.Temperature != nil {
		gen["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil {
		gen["maxOutputTokens"] = *req.MaxTokens
	}
	if req.TopP != nil {
		gen["topP"] = *req.TopP
	}
	if len(req.Stop) > 0 {
		gen["stopSequences"] = req.Stop
	}
	if len(gen) > 0 {
		body["generationConfig"] = gen
	}
	buf, _ := json.Marshal(body)

	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s",
		g.BaseURL, url.PathEscape(req.Model), url.QueryEscape(key))
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(buf))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := g.Client.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("gemini call: %w", err)
	}
	defer httpResp.Body.Close()
	respBody, _ := io.ReadAll(httpResp.Body)

	if httpResp.StatusCode >= 400 {
		return Response{}, fmt.Errorf("gemini %d: %s", httpResp.StatusCode, snippet(respBody))
	}
	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct{ Text string `json:"text"` } `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Response{}, fmt.Errorf("gemini decode: %w", err)
	}
	var out string
	stop := ""
	if len(parsed.Candidates) > 0 {
		stop = parsed.Candidates[0].FinishReason
		for _, p := range parsed.Candidates[0].Content.Parts {
			out += p.Text
		}
	}
	return Response{
		Output:       out,
		InputTokens:  parsed.UsageMetadata.PromptTokenCount,
		OutputTokens: parsed.UsageMetadata.CandidatesTokenCount,
		StopReason:   stop,
		Raw:          respBody,
	}, nil
}
