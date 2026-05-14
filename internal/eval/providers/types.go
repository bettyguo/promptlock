// Package providers wraps each LLM provider with a uniform interface.
package providers

import (
	"context"
	"encoding/json"
	"errors"
)

// Provider is implemented by every LLM provider promptlock supports.
type Provider interface {
	Name() string
	Call(ctx context.Context, req Request) (Response, error)
}

// Request is the cross-provider call shape. Providers translate to native API.
type Request struct {
	Model       string
	System      string    // optional; rendered before Messages for providers that take a separate field
	Messages    []Message // user/assistant turns
	Temperature *float64
	MaxTokens   *int
	TopP        *float64
	Stop        []string
	Seed        *int64
}

// Message is one user/assistant turn.
type Message struct {
	Role    string // "user" | "assistant"
	Content string
}

// Response is the cross-provider response shape.
type Response struct {
	Output       string
	InputTokens  int
	OutputTokens int
	StopReason   string
	Raw          json.RawMessage // provider-native body for debugging
}

// CacheKey returns a deterministic key for the request, suitable for hashing
// into the response cache. Excludes Stop ordering sensitivity by sorting.
func (r Request) CacheKey() string {
	b, _ := json.Marshal(struct {
		Model       string    `json:"model"`
		System      string    `json:"system"`
		Messages    []Message `json:"messages"`
		Temperature *float64  `json:"temperature,omitempty"`
		MaxTokens   *int      `json:"max_tokens,omitempty"`
		TopP        *float64  `json:"top_p,omitempty"`
		Stop        []string  `json:"stop,omitempty"`
		Seed        *int64    `json:"seed,omitempty"`
	}{r.Model, r.System, r.Messages, r.Temperature, r.MaxTokens, r.TopP, r.Stop, r.Seed})
	return string(b)
}

// ErrMissingAPIKey is returned when a provider's auth env var isn't set.
type ErrMissingAPIKey struct {
	Provider string
	EnvVar   string
}

func (e *ErrMissingAPIKey) Error() string {
	return "provider " + e.Provider + ": missing " + e.EnvVar +
		" (set it in your shell environment)"
}

// IsAuthErr reports whether err is an ErrMissingAPIKey.
func IsAuthErr(err error) bool {
	var x *ErrMissingAPIKey
	return errors.As(err, &x)
}
