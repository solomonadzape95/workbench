package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/history"
	"github.com/solomonadzape95/workbench/internal/ops/file"
	"github.com/solomonadzape95/workbench/internal/pipeline"
	"github.com/solomonadzape95/workbench/internal/runner"
)

func testServer(t *testing.T, token string) (*Server, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.Output.Dir = filepath.Join(dir, "out")
	r := runner.New(runner.Options{
		Operations: core.NewRegistry(file.Copy{}),
		CacheDir:   filepath.Join(dir, "cache"),
		LogDir:     filepath.Join(dir, "logs"),
		MaxJobs:    2,
	})
	t.Cleanup(r.Close)
	s := New(Options{
		Config:     cfg,
		Operations: core.NewRegistry(file.Copy{}),
		Runner:     r,
		Pipelines:  pipeline.NewStore(filepath.Join(dir, "pipelines")),
		History:    history.New(filepath.Join(dir, "data")),
		Token:      token,
		Version:    "test",
	})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

func TestCreateJobAndWait(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(src, []byte("hello api"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	_, ts := testServer(t, "")

	body := fmt.Sprintf(`{"operation":"file.copy","inputs":[%q],"target":{"dir":%q},"name":"api copy"}`, src, out)
	resp, err := http.Post(ts.URL+"/v1/jobs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, readBody(t, resp))
	}
	var created struct {
		Job jobInfo `json:"job"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Job.ID == "" {
		t.Fatal("job has no id")
	}

	resp2, err := http.Get(ts.URL + "/v1/jobs/" + created.Job.ID + "?wait=5")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var done jobInfo
	if err := json.NewDecoder(resp2.Body).Decode(&done); err != nil {
		t.Fatal(err)
	}
	if done.Status != string(core.StatusDone) {
		t.Fatalf("status = %q, error = %+v", done.Status, done.Error)
	}
	if len(done.Items) != 1 || len(done.Items[0].Outputs) != 1 {
		t.Fatalf("items = %+v", done.Items)
	}
	data, err := os.ReadFile(done.Items[0].Outputs[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello api" {
		t.Errorf("output = %q", data)
	}
}

func TestAuthAndCatalog(t *testing.T) {
	_, ts := testServer(t, "secret")

	resp, err := http.Get(ts.URL + "/v1/operations")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/operations", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authorized status = %d", resp.StatusCode)
	}
	var catalog struct {
		Operations []operationInfo `json:"operations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Operations) != 1 || catalog.Operations[0].ID != "file.copy" {
		t.Errorf("catalog = %+v", catalog.Operations)
	}
}

func TestCreateJobRejectsBadRequests(t *testing.T) {
	_, ts := testServer(t, "")
	cases := []string{
		`{"inputs":["/tmp/x"]}`,
		`{"operation":"nope","inputs":["/tmp/x"]}`,
		`{"operation":"file.copy","inputs":[]}`,
		`{"operation":"file.copy","inputs":["/does/not/exist"]}`,
	}
	for _, body := range cases {
		resp, err := http.Post(ts.URL+"/v1/jobs", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode < 400 {
			t.Errorf("body %q got status %d, want 4xx", body, resp.StatusCode)
		}
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	return buf.String()
}

func TestSavedPipelineAndSteps(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "chain.txt")
	if err := os.WriteFile(src, []byte("chain"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	s, ts := testServer(t, "")
	if err := s.pipelines.Save(core.Pipeline{
		Name: "copy-twice", Version: core.PipelineVersion,
		Steps: []core.Step{{Op: "file.copy"}, {Op: "file.copy"}},
	}); err != nil {
		t.Fatal(err)
	}

	for _, body := range []string{
		fmt.Sprintf(`{"pipeline":"copy-twice","inputs":[%q],"target":{"dir":%q}}`, src, out),
		fmt.Sprintf(`{"inputs":[%q],"steps":[{"operation":"file.copy"}],"target":{"dir":%q}}`, src, out),
	} {
		resp, err := http.Post(ts.URL+"/v1/jobs", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		var created struct {
			Job jobInfo `json:"job"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("create status = %d", resp.StatusCode)
		}
		doneResp, err := http.Get(ts.URL + "/v1/jobs/" + created.Job.ID + "?wait=5")
		if err != nil {
			t.Fatal(err)
		}
		var done jobInfo
		if err := json.NewDecoder(doneResp.Body).Decode(&done); err != nil {
			t.Fatal(err)
		}
		doneResp.Body.Close()
		if done.Status != string(core.StatusDone) {
			t.Fatalf("status = %q, error = %+v", done.Status, done.Error)
		}
	}
}

func TestHistoryJobsAreListedAndFetched(t *testing.T) {
	s, ts := testServer(t, "")
	now := time.Now().UTC()
	rec := runner.Record{Version: 1, View: runner.JobView{
		ID: "job-history", Name: "old job", Status: core.StatusDone,
		QueuedAt: now.Add(-time.Hour), FinishedAt: now.Add(-59 * time.Minute),
		Pipeline: core.Pipeline{Name: "old job", Steps: []core.Step{{Op: "file.copy"}}},
		Items: []runner.ItemView{{ID: "item-1", Status: core.ItemStatusDone,
			Input: core.Artifact{Path: "/tmp/a.txt"}, Outputs: []core.Artifact{{Path: "/tmp/a.txt"}}}},
	}}
	if err := s.history.Append(rec); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(ts.URL + "/v1/jobs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list struct {
		Jobs []jobInfo `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, job := range list.Jobs {
		if job.ID == "job-history" {
			found = true
		}
	}
	if !found {
		t.Fatalf("history job missing: %+v", list.Jobs)
	}

	resp2, err := http.Get(ts.URL + "/v1/jobs/job-history")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp2.StatusCode)
	}
	var job jobInfo
	if err := json.NewDecoder(resp2.Body).Decode(&job); err != nil {
		t.Fatal(err)
	}
	if job.ID != "job-history" {
		t.Fatalf("job = %+v", job)
	}
}

func TestJobArtifactsAndSSE(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "artifact.txt")
	if err := os.WriteFile(src, []byte("artifact body"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	_, ts := testServer(t, "")
	body := fmt.Sprintf(`{"operation":"file.copy","inputs":[%q],"target":{"dir":%q}}`, src, out)
	resp, err := http.Post(ts.URL+"/v1/jobs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		Job jobInfo `json:"job"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	jobID := created.Job.ID

	resp, err = http.Get(ts.URL + "/v1/jobs/" + jobID + "?wait=5")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/v1/jobs/" + jobID + "/artifacts")
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Artifacts []artifactRef `json:"artifacts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(list.Artifacts) != 1 {
		t.Fatalf("artifacts = %+v", list.Artifacts)
	}
	resp, err = http.Get(ts.URL + "/v1/jobs/" + jobID + "/artifacts/1")
	if err != nil {
		t.Fatal(err)
	}
	data := readBody(t, resp)
	resp.Body.Close()
	if data != "artifact body" {
		t.Fatalf("artifact body = %q", data)
	}

	resp, err = http.Get(ts.URL + "/v1/jobs/" + jobID + "/events")
	if err != nil {
		t.Fatal(err)
	}
	events := readBody(t, resp)
	resp.Body.Close()
	if !strings.Contains(events, "event: job") || !strings.Contains(events, `"status":"done"`) {
		t.Fatalf("events = %s", events)
	}
}

func TestJobCallbackURL(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "callback.txt")
	if err := os.WriteFile(src, []byte("callback"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	notified := make(chan map[string]any, 1)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		notified <- payload
		w.WriteHeader(http.StatusNoContent)
	}))
	defer hook.Close()

	_, ts := testServer(t, "")
	body := fmt.Sprintf(`{"operation":"file.copy","inputs":[%q],"target":{"dir":%q},"callback_url":%q}`, src, out, hook.URL)
	resp, err := http.Post(ts.URL+"/v1/jobs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	select {
	case payload := <-notified:
		if payload["status"] != "done" {
			t.Fatalf("callback = %+v", payload)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("callback was not delivered")
	}
}
