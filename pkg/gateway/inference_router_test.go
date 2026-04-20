package gateway

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/beam-cloud/beta9/pkg/types"
)

// TestUnloadModelNonOK verifies that UnloadModel only clears registry state
// when the worker responds with HTTP 200. Non-OK responses must return an
// error and leave the model's LoadState unchanged (Cubic finding on
// pkg/gateway/inference_router.go:463 — cache-coherency bug).
func TestUnloadModelNonOK(t *testing.T) {
	cases := []struct {
		name             string
		status           int
		body             string
		wantErr          bool
		wantFinalState   types.LoadState
	}{
		{"ok clears state", http.StatusOK, "{}", false, types.LoadStateIdle},
		{"500 preserves state", http.StatusInternalServerError, "boom", true, types.LoadStateReady},
		{"404 preserves state", http.StatusNotFound, "not found", true, types.LoadStateReady},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			// Parse httptest URL into host + port. srv.URL looks like "http://127.0.0.1:54321".
			u := strings.TrimPrefix(srv.URL, "http://")
			host, portStr, err := net.SplitHostPort(u)
			if err != nil {
				t.Fatalf("split host:port: %v", err)
			}
			port, err := strconv.Atoi(portStr)
			if err != nil {
				t.Fatalf("port: %v", err)
			}

			reg := NewModelRegistry()
			reg.RegisterNode(&types.NodeInferenceInfo{
				NodeID:      "node-a",
				TailscaleIP: host,
				Port:        port,
				Models: map[string]*types.ModelInfo{
					"llama3": {
						Name:      "llama3",
						LoadState: types.LoadStateReady,
					},
				},
			})

			router := NewInferenceRouter(reg)

			err = router.UnloadModel(context.Background(), "node-a", "llama3")
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}

			got := reg.GetNode("node-a").Models["llama3"].LoadState
			if got != tc.wantFinalState {
				t.Fatalf("LoadState = %q, want %q (status=%d)", got, tc.wantFinalState, tc.status)
			}
		})
	}
}

// TestUnloadModelNodeNotFound verifies the early-return when the node is
// absent from the registry.
func TestUnloadModelNodeNotFound(t *testing.T) {
	reg := NewModelRegistry()
	router := NewInferenceRouter(reg)
	err := router.UnloadModel(context.Background(), "missing-node", "any-model")
	if err == nil {
		t.Fatalf("expected error for missing node")
	}
}
