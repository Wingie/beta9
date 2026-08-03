package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestControlServer wires a minimal Agent with state + OllamaManager so the
// /inference/pull handler's state touches are safe without starting a TUI or
// keepalive loop.
func newTestControlServer(t *testing.T, ollamaBaseURL string) *ControlServer {
	t.Helper()
	state := NewAgentState("test-machine", "test-pool", "http://gateway")
	agent := &Agent{
		state:  state,
		ollama: NewOllamaManager("127.0.0.1", 0),
	}
	cs := NewControlServer(agent, 0)
	cs.ollamaBaseURL = ollamaBaseURL
	return cs
}

// TestInferencePullStatus verifies that the handler only reports success when
// Ollama returns HTTP 200 AND the NDJSON stream decodes cleanly with at least
// one progress frame. Any non-200 upstream must become HTTP 502 at the
// control-plane boundary (Cubic finding on pkg/agent/control.go:227).
func TestInferencePullStatus(t *testing.T) {
	// Valid NDJSON progress-stream frames.
	okFrames := "{\"status\":\"pulling manifest\"}\n{\"status\":\"success\"}\n"

	cases := []struct {
		name             string
		upstreamStatus   int
		upstreamBody     string
		wantHandlerCode  int
	}{
		{"ollama 200 returns 201", http.StatusOK, okFrames, http.StatusCreated},
		{"ollama 404 surfaces 502", http.StatusNotFound, "{\"error\":\"model not found\"}", http.StatusBadGateway},
		{"ollama 500 surfaces 502", http.StatusInternalServerError, "{\"error\":\"boom\"}", http.StatusBadGateway},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.upstreamStatus)
				_, _ = w.Write([]byte(tc.upstreamBody))
			}))
			defer ollama.Close()

			cs := newTestControlServer(t, ollama.URL)

			body, _ := json.Marshal(map[string]string{"model": "llama3:8b"})
			req := httptest.NewRequest(http.MethodPost, "/inference/pull", bytes.NewReader(body))
			rr := httptest.NewRecorder()
			cs.handleInferencePull(rr, req)

			if rr.Code != tc.wantHandlerCode {
				t.Fatalf("handler code = %d, want %d; body=%s", rr.Code, tc.wantHandlerCode, rr.Body.String())
			}
		})
	}
}

// TestInferencePullDecodeFailureReturnsBadGateway covers the case where
// Ollama returns 200 but the body is not NDJSON. The handler must not
// pretend success — it must surface a 502.
func TestInferencePullDecodeFailureReturnsBadGateway(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("this is not valid json at all"))
	}))
	defer ollama.Close()

	cs := newTestControlServer(t, ollama.URL)

	body, _ := json.Marshal(map[string]string{"model": "llama3:8b"})
	req := httptest.NewRequest(http.MethodPost, "/inference/pull", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	cs.handleInferencePull(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("decode failure should produce 502, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

// TestInferencePullRejectsInvalidModelName asserts the P0-A allowlist blocks
// shell-metacharacter payloads before they reach Ollama.
func TestInferencePullRejectsInvalidModelName(t *testing.T) {
	cs := newTestControlServer(t, "http://127.0.0.1:1") // unreachable on purpose

	payloads := []string{
		"llama3; rm -rf /",
		"../../etc/passwd",
		"model`whoami`",
		"",
	}
	for _, p := range payloads {
		body, _ := json.Marshal(map[string]string{"model": p})
		req := httptest.NewRequest(http.MethodPost, "/inference/pull", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		cs.handleInferencePull(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("payload %q should be rejected with 400, got %d", p, rr.Code)
		}
	}
}

// TestInferencePullMethodNotAllowed checks GET is refused.
func TestInferencePullMethodNotAllowed(t *testing.T) {
	cs := newTestControlServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/inference/pull", nil)
	rr := httptest.NewRecorder()
	cs.handleInferencePull(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET should be 405, got %d", rr.Code)
	}
}
