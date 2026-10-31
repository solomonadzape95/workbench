package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientCallsAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/v1/operations":
			_ = json.NewEncoder(w).Encode(map[string]any{"operations": []map[string]any{{
				"id": "image.convert", "title": "Convert image", "domain": "image", "output": map[string]any{},
			}}})
		case "/v1/jobs":
			if r.Method != http.MethodPost {
				t.Errorf("method = %s", r.Method)
			}
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{"job": map[string]any{
				"id": "job-1", "name": "convert", "status": "queued", "percent": 0,
			}})
		case "/v1/jobs/job-1/artifacts/1":
			_, _ = w.Write([]byte("artifact bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c := New(server.URL)
	c.Token = "secret"
	ops, err := c.ListOperations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].ID != "image.convert" {
		t.Fatalf("operations = %+v", ops)
	}
	job, err := c.CreateJob(context.Background(), CreateJobRequest{Operation: "image.convert", Inputs: []string{"a.png"}})
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != "job-1" || job.Status != "queued" {
		t.Fatalf("job = %+v", job)
	}
	data, err := c.DownloadArtifact(context.Background(), "job-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "artifact bytes" {
		t.Fatalf("artifact = %q", data)
	}
}

func TestClientDecodesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
			"code": "usage", "message": "job not found", "hint": "list jobs",
		}})
	}))
	defer server.Close()
	_, err := New(server.URL).GetJob(context.Background(), "missing", 0)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error = %T %v", err, err)
	}
	if apiErr.Status != http.StatusNotFound || apiErr.Message != "job not found" {
		t.Fatalf("error = %+v", apiErr)
	}
}
