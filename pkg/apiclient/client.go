// Package apiclient is a small, typed client for the Workbench Agent API.
// The wire schema is documented in docs/openapi.yaml.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 60 * time.Second}}
}

type APIError struct {
	Status  int
	Code    string
	Message string
	Hint    string
}

func (e *APIError) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("%s (HTTP %d): %s — %s", e.Code, e.Status, e.Message, e.Hint)
	}
	return fmt.Sprintf("%s (HTTP %d): %s", e.Code, e.Status, e.Message)
}

type ErrorInfo struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Hint      string `json:"hint,omitempty"`
	Path      string `json:"path,omitempty"`
	Operation string `json:"operation,omitempty"`
}

type Operation struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Summary        string         `json:"summary,omitempty"`
	Domain         string         `json:"domain"`
	Source         string         `json:"source"`
	Batchable      bool           `json:"batchable"`
	Cost           string         `json:"cost,omitempty"`
	Mode           string         `json:"mode,omitempty"`
	Engines        []string       `json:"engines,omitempty"`
	RequireEngines bool           `json:"require_engines,omitempty"`
	Accepts        []any          `json:"accepts,omitempty"`
	Params         []Param        `json:"params,omitempty"`
	Output         map[string]any `json:"output"`
}

type Param struct {
	Key      string         `json:"key"`
	Label    string         `json:"label,omitempty"`
	Type     string         `json:"type"`
	Default  any            `json:"default,omitempty"`
	Options  []Option       `json:"options,omitempty"`
	Required bool           `json:"required,omitempty"`
	Advanced bool           `json:"advanced,omitempty"`
	Min      *float64       `json:"min,omitempty"`
	Max      *float64       `json:"max,omitempty"`
	Help     string         `json:"help,omitempty"`
	Extra    map[string]any `json:"-"`
}

type Option struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
}

type Engine struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	Available bool   `json:"available"`
	Detail    string `json:"detail,omitempty"`
	Hint      string `json:"hint,omitempty"`
	Error     string `json:"error,omitempty"`
}

type PipelineMeta struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Steps       int    `json:"steps"`
	Error       string `json:"error,omitempty"`
}

type Pipeline struct {
	Name        string `json:"name"`
	Version     int    `json:"version"`
	Description string `json:"description,omitempty"`
	Steps       []Step `json:"steps"`
}

type Step struct {
	ID     string         `json:"id,omitempty"`
	Op     string         `json:"operation"`
	Params map[string]any `json:"params,omitempty"`
}

type Target struct {
	Dir       string `json:"dir,omitempty"`
	Template  string `json:"template,omitempty"`
	Collision string `json:"collision,omitempty"`
}

type CreateJobRequest struct {
	Operation   string         `json:"operation,omitempty"`
	Pipeline    string         `json:"pipeline,omitempty"`
	Steps       []Step         `json:"steps,omitempty"`
	Inputs      []string       `json:"inputs"`
	Params      map[string]any `json:"params,omitempty"`
	Name        string         `json:"name,omitempty"`
	Output      string         `json:"output,omitempty"`
	Target      *Target        `json:"target,omitempty"`
	CallbackURL string         `json:"callback_url,omitempty"`
}

type Artifact struct {
	Name          string         `json:"name"`
	Path          string         `json:"path"`
	Kind          string         `json:"kind"`
	Format        string         `json:"format,omitempty"`
	Size          int64          `json:"size,omitempty"`
	SuggestedName string         `json:"suggested_name,omitempty"`
	Meta          map[string]any `json:"meta,omitempty"`
}

type ArtifactRef struct {
	Index  int    `json:"index"`
	ItemID string `json:"item_id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Format string `json:"format,omitempty"`
	Size   int64  `json:"size,omitempty"`
}

type Item struct {
	ID      string     `json:"id"`
	Input   Artifact   `json:"input"`
	Current Artifact   `json:"current"`
	Status  string     `json:"status"`
	Stage   string     `json:"stage,omitempty"`
	Percent int        `json:"percent"`
	Step    int        `json:"step"`
	Outputs []Artifact `json:"outputs,omitempty"`
	Error   *ErrorInfo `json:"error,omitempty"`
}

type Job struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Status     string         `json:"status"`
	Percent    int            `json:"percent"`
	QueuedAt   time.Time      `json:"queued_at"`
	StartedAt  *time.Time     `json:"started_at,omitempty"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	DurationMS int64          `json:"duration_ms"`
	Counts     map[string]int `json:"counts"`
	Steps      []any          `json:"steps,omitempty"`
	Items      []Item         `json:"items,omitempty"`
	Error      *ErrorInfo     `json:"error,omitempty"`
	Metrics    map[string]any `json:"metrics"`
	Target     Target         `json:"target"`
	Aggregate  []Artifact     `json:"aggregate,omitempty"`
	Logs       []string       `json:"logs,omitempty"`
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeError(resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func decodeError(resp *http.Response) error {
	var payload struct {
		Error ErrorInfo `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload)
	if payload.Error.Message == "" {
		payload.Error.Message = resp.Status
	}
	return &APIError{Status: resp.StatusCode, Code: payload.Error.Code, Message: payload.Error.Message, Hint: payload.Error.Hint}
}

func (c *Client) Health(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.do(ctx, http.MethodGet, "/healthz", nil, &out)
	return out, err
}

func (c *Client) ListOperations(ctx context.Context) ([]Operation, error) {
	var out struct {
		Operations []Operation `json:"operations"`
	}
	err := c.do(ctx, http.MethodGet, "/v1/operations", nil, &out)
	return out.Operations, err
}

func (c *Client) GetOperation(ctx context.Context, id string) (Operation, error) {
	var out Operation
	err := c.do(ctx, http.MethodGet, "/v1/operations/"+url.PathEscape(id), nil, &out)
	return out, err
}

func (c *Client) ListEngines(ctx context.Context) ([]Engine, error) {
	var out struct {
		Engines []Engine `json:"engines"`
	}
	err := c.do(ctx, http.MethodGet, "/v1/engines", nil, &out)
	return out.Engines, err
}

func (c *Client) ListPipelines(ctx context.Context) ([]PipelineMeta, error) {
	var out struct {
		Pipelines []PipelineMeta `json:"pipelines"`
	}
	err := c.do(ctx, http.MethodGet, "/v1/pipelines", nil, &out)
	return out.Pipelines, err
}

func (c *Client) GetPipeline(ctx context.Context, name string) (Pipeline, error) {
	var out Pipeline
	err := c.do(ctx, http.MethodGet, "/v1/pipelines/"+url.PathEscape(name), nil, &out)
	return out, err
}

func (c *Client) CreateJob(ctx context.Context, request CreateJobRequest) (Job, error) {
	var out struct {
		Job Job `json:"job"`
	}
	err := c.do(ctx, http.MethodPost, "/v1/jobs", request, &out)
	return out.Job, err
}

func (c *Client) ListJobs(ctx context.Context, limit int) ([]Job, error) {
	path := "/v1/jobs"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	var out struct {
		Jobs []Job `json:"jobs"`
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out.Jobs, err
}

func (c *Client) GetJob(ctx context.Context, id string, waitSeconds int) (Job, error) {
	path := "/v1/jobs/" + url.PathEscape(id)
	if waitSeconds > 0 {
		path += "?wait=" + strconv.Itoa(waitSeconds)
	}
	var out Job
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) ListArtifacts(ctx context.Context, id string) ([]ArtifactRef, error) {
	var out struct {
		Artifacts []ArtifactRef `json:"artifacts"`
	}
	err := c.do(ctx, http.MethodGet, "/v1/jobs/"+url.PathEscape(id)+"/artifacts", nil, &out)
	return out.Artifacts, err
}

func (c *Client) DownloadArtifact(ctx context.Context, id string, index int) ([]byte, error) {
	path := "/v1/jobs/" + url.PathEscape(id) + "/artifacts/" + strconv.Itoa(index)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, decodeError(resp)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<30))
}

func (c *Client) CancelJob(ctx context.Context, id string) (Job, error) {
	return c.postJob(ctx, "/v1/jobs/"+url.PathEscape(id)+"/cancel")
}

func (c *Client) RetryJob(ctx context.Context, id string) (Job, error) {
	return c.postJob(ctx, "/v1/jobs/"+url.PathEscape(id)+"/retry")
}

func (c *Client) RerunJob(ctx context.Context, id string) (Job, error) {
	return c.postJob(ctx, "/v1/jobs/"+url.PathEscape(id)+"/rerun")
}

func (c *Client) postJob(ctx context.Context, path string) (Job, error) {
	var out struct {
		Job Job `json:"job"`
	}
	err := c.do(ctx, http.MethodPost, path, nil, &out)
	return out.Job, err
}
