package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookPostsJob(t *testing.T) {
	var got Job
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request = %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	want := Job{ID: "job-1", Name: "resize", Status: "done", DurationMS: 1200, Outputs: []string{"/tmp/out.png"}}
	if err := Webhook(context.Background(), server.URL, want); err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Name != want.Name || len(got.Outputs) != 1 {
		t.Fatalf("payload = %+v", got)
	}
}

func TestWebhookRejectsNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusBadRequest)
	}))
	defer server.Close()
	if err := Webhook(context.Background(), server.URL, Job{ID: "x"}); err == nil {
		t.Fatal("non-2xx webhook accepted")
	}
}

func TestWebhookEmptyURLIsNoop(t *testing.T) {
	if err := Webhook(context.Background(), "  ", Job{ID: "x"}); err != nil {
		t.Fatal(err)
	}
}
