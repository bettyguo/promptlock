package providers

import (
	"fmt"
	"strings"
)

// Get returns a Provider by canonical name.
//
//	"anthropic", "openai", "gemini", "ollama", "mock"
func Get(name string) (Provider, error) {
	switch name {
	case "anthropic":
		return NewAnthropic(), nil
	case "openai":
		return NewOpenAI(), nil
	case "gemini":
		return NewGemini(), nil
	case "ollama":
		return NewOllama(), nil
	case "mock":
		return NewMock(), nil
	}
	return nil, fmt.Errorf("unknown provider %q (allowed: anthropic, openai, gemini, ollama, mock)", name)
}

// InferProvider returns the canonical provider name for a model ID. The
// second return is the model ID with any provider-prefix stripped.
func InferProvider(model string) (string, string) {
	switch {
	case strings.HasPrefix(model, "claude-"):
		return "anthropic", model
	case strings.HasPrefix(model, "gpt-"),
		strings.HasPrefix(model, "o1-"),
		strings.HasPrefix(model, "o3-"),
		strings.HasPrefix(model, "o4-"):
		return "openai", model
	case strings.HasPrefix(model, "gemini-"):
		return "gemini", model
	case strings.HasPrefix(model, "ollama/"):
		return "ollama", strings.TrimPrefix(model, "ollama/")
	case strings.HasPrefix(model, "mock/"):
		return "mock", strings.TrimPrefix(model, "mock/")
	}
	return "", model
}
