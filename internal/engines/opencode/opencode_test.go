package opencode

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestParseRefSplitsOnlyFirstSlash(t *testing.T) {
	ref, err := ParseRef("nvidia/nvidia/nemotron-3-ultra-550b-a55b")
	if err != nil {
		t.Fatal(err)
	}
	if ref.ProviderID != "nvidia" || ref.ID != "nvidia/nemotron-3-ultra-550b-a55b" {
		t.Fatalf("ref = %+v", ref)
	}
	if _, err := ParseRef("model-without-provider"); err == nil {
		t.Fatal("ParseRef accepted a value without a provider")
	}
}

func TestParseServiceURL(t *testing.T) {
	cases := map[string]string{
		"http://127.0.0.1:49374\n":                 "http://127.0.0.1:49374",
		"starting...\nhttp://127.0.0.1:4096.\n":    "http://127.0.0.1:4096",
		"no server here":                           "",
		"server: https://example.test:1234/path\n": "https://example.test:1234/path",
	}
	for input, want := range cases {
		if got := parseServiceURL(input); got != want {
			t.Errorf("parseServiceURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFreeModelsFiltersAndOrders(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/api/model" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{
				"id": "paid", "providerID": "opencode", "name": "Paid",
				"cost":         []map[string]any{{"input": 1, "output": 2, "cache": map[string]any{"read": 0, "write": 0}}},
				"capabilities": map[string]any{"output": []string{"text"}},
				"variants":     []map[string]string{{"id": "none"}},
				"status":       "active", "enabled": true,
				"limit": map[string]any{"context": 200000, "output": 8000},
			},
			{
				"id": "nvidia/nemotron", "providerID": "nvidia", "name": "Nemotron",
				"cost":         []map[string]any{{"input": 0, "output": 0, "cache": map[string]any{"read": 0, "write": 0}}},
				"capabilities": map[string]any{"output": []string{"text"}},
				"variants":     []map[string]string{{"id": "none"}, {"id": "high"}},
				"status":       "active", "enabled": true,
				"limit": map[string]any{"context": 1000000, "output": 8000},
			},
			{
				"id": "embed", "providerID": "nvidia", "name": "Embedding",
				"cost":         []map[string]any{{"input": 0, "output": 0, "cache": map[string]any{"read": 0, "write": 0}}},
				"capabilities": map[string]any{"output": []string{"text"}},
				"variants":     []map[string]string{},
				"status":       "active", "enabled": true,
				"limit": map[string]any{"context": 8192, "output": 0},
			},
			{
				"id": "disabled", "providerID": "nvidia", "name": "Disabled",
				"cost":         []map[string]any{{"input": 0, "output": 0, "cache": map[string]any{"read": 0, "write": 0}}},
				"capabilities": map[string]any{"output": []string{"text"}},
				"variants":     []map[string]string{},
				"status":       "active", "enabled": false,
				"limit": map[string]any{"context": 200000, "output": 8000},
			},
		}})
	}))
	defer srv.Close()

	client := &Client{ServerURL: srv.URL, Password: "secret", HTTP: srv.Client()}
	models, err := client.FreeModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].String() != "nvidia/nvidia/nemotron" {
		t.Fatalf("models = %+v", models)
	}
	if models[0].Variant != "none" {
		t.Errorf("variant = %q, want none", models[0].Variant)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("opencode:secret"))
	if auth != wantAuth {
		t.Errorf("auth = %q, want %q", auth, wantAuth)
	}
}

func TestGenerateCreatesSwitchesAndDeletesSession(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var gotPrompt string
	var gotModel map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/api/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"id": "ses_test"}})
		case "/api/session/ses_test/model":
			var body struct {
				Model map[string]string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotModel = body.Model
			w.WriteHeader(http.StatusNoContent)
		case "/api/session/ses_test/generate":
			var body struct {
				Prompt string `json:"prompt"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotPrompt = body.Prompt
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"text": "  rewritten text  "}})
		case "/api/session/ses_test/interrupt":
			w.WriteHeader(http.StatusNoContent)
		case "/api/session/ses_test":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := &Client{ServerURL: srv.URL, Password: "secret", HTTP: srv.Client()}
	text, err := client.Generate(context.Background(), ModelRef{
		ProviderID: "nvidia", ID: "nvidia/nemotron", Variant: "none",
	}, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if text != "rewritten text" {
		t.Errorf("text = %q", text)
	}
	if gotPrompt != "hello" {
		t.Errorf("prompt = %q", gotPrompt)
	}
	if gotModel["providerID"] != "nvidia" || gotModel["id"] != "nvidia/nemotron" || gotModel["variant"] != "none" {
		t.Errorf("model = %+v", gotModel)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{
		"POST /api/session",
		"POST /api/session/ses_test/model",
		"POST /api/session/ses_test/generate",
		"POST /api/session/ses_test/interrupt",
		"DELETE /api/session/ses_test",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls =\n%s\nwant\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestGenerateReturnsStructuredError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"id": "ses_test"}})
		case "/api/session/ses_test/model":
			w.WriteHeader(http.StatusNoContent)
		case "/api/session/ses_test/generate":
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"type":         "error",
				"message":      "Free usage limit reached",
				"retryAfterMs": 1500,
			})
		case "/api/session/ses_test/interrupt":
			w.WriteHeader(http.StatusNoContent)
		case "/api/session/ses_test":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := &Client{ServerURL: srv.URL, Password: "secret", HTTP: srv.Client()}
	_, err := client.Generate(context.Background(), ModelRef{ProviderID: "opencode", ID: "free"}, "hello")
	if err == nil {
		t.Fatal("Generate succeeded on a 429")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v, want *APIError", err, err)
	}
	if apiErr.Status != http.StatusTooManyRequests || !strings.Contains(apiErr.Message, "Free usage limit") {
		t.Errorf("apiErr = %+v", apiErr)
	}
	if apiErr.RetryAfter.String() != "1.5s" {
		t.Errorf("retry after = %s, want 1.5s", apiErr.RetryAfter)
	}
	if !IsModelUnavailable(err) {
		t.Error("429 should be model-unavailable")
	}
}

func TestIsAuthError(t *testing.T) {
	if !IsAuthError(&APIError{Status: http.StatusUnauthorized}) {
		t.Error("401 should be an auth error")
	}
	if IsAuthError(&APIError{Status: http.StatusTooManyRequests}) {
		t.Error("429 should not be an auth error")
	}
}
