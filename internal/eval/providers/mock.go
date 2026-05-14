package providers

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
)

// Mock is a deterministic provider for tests and dogfooding without network.
//
// Behavior:
//   - If the System message contains the marker "MOCK_RETURN: <text>" on its
//     own line, that text is returned verbatim.
//   - Else, if the last user message starts with "echo:", returns the rest.
//   - Else, returns "mock-output" as a fallback.
//
// Token counts are character-length / 4 estimates — fine for testing.
type Mock struct {
	mu    sync.Mutex
	calls int // counter exposed via CallsMade for tests
}

// NewMock builds a fresh Mock.
func NewMock() *Mock { return &Mock{} }

// Name returns "mock".
func (*Mock) Name() string { return "mock" }

// Call implements Provider.
func (m *Mock) Call(_ context.Context, req Request) (Response, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()

	out := "mock-output"
	for _, line := range strings.Split(req.System, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "MOCK_RETURN:") {
			out = strings.TrimSpace(strings.TrimPrefix(t, "MOCK_RETURN:"))
			break
		}
	}
	if out == "mock-output" && len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1].Content
		if strings.HasPrefix(strings.TrimSpace(last), "echo:") {
			out = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(last), "echo:"))
		}
	}

	in := len(req.System)
	for _, msg := range req.Messages {
		in += len(msg.Content)
	}
	raw, _ := json.Marshal(map[string]any{"mock": true, "model": req.Model})
	return Response{
		Output:       out,
		InputTokens:  in / 4,
		OutputTokens: len(out) / 4,
		StopReason:   "stop",
		Raw:          raw,
	}, nil
}

// CallsMade returns the number of times Call has been invoked.
func (m *Mock) CallsMade() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}
