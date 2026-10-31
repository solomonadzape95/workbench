// Package opencode talks to the local OpenCode background service.
//
// Workbench does not store provider keys of its own. It asks the OpenCode
// service that is already running on the machine to run a prompt on one of the
// user's models, using the service password that OpenCode writes to its config
// directory. That keeps model access in one place and means a free model added
// in OpenCode shows up here without extra configuration.
package opencode

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/executil"
)

const (
	maxResponseBytes = 64 << 20
	serviceStartWait = 20 * time.Second
)

// ModelRef identifies one model in the OpenCode catalog. Variant is optional;
// an empty variant lets OpenCode pick the model default.
type ModelRef struct {
	ProviderID string `json:"providerID"`
	ID         string `json:"id"`
	Variant    string `json:"variant,omitempty"`
}

func (m ModelRef) String() string {
	if m.ProviderID == "" {
		return m.ID
	}
	return m.ProviderID + "/" + m.ID
}

// ParseRef turns "provider/model" into a ModelRef. Model IDs may contain
// slashes, so only the first slash separates the provider.
func ParseRef(value string) (ModelRef, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ModelRef{}, errors.New("empty model")
	}
	provider, model, ok := strings.Cut(value, "/")
	if !ok || provider == "" || model == "" {
		return ModelRef{}, fmt.Errorf("model %q must be provider/model", value)
	}
	return ModelRef{ProviderID: provider, ID: model}, nil
}

// APIError is a non-2xx response from the OpenCode API.
type APIError struct {
	Status     int
	Tag        string
	Message    string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if e == nil {
		return "opencode request failed"
	}
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	if e.Status > 0 {
		return fmt.Sprintf("opencode: %s (HTTP %d)", msg, e.Status)
	}
	return "opencode: " + msg
}

// IsModelUnavailable reports whether trying a different model is more useful
// than retrying this one. A free-tier limit, a region block, a privacy policy,
// a missing model, or a rejected request are all model-specific.
func IsModelUnavailable(err error) bool {
	var e *APIError
	if !errors.As(err, &e) {
		return false
	}
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusTooManyRequests:
		return true
	case http.StatusBadRequest:
		return true
	default:
		return false
	}
}

// IsAuthError reports a bad service password rather than a bad model.
func IsAuthError(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status == http.StatusUnauthorized
}

// Client is a thin HTTP client for the OpenCode service. It also owns service
// discovery: ServerURL/Password override auto-discovery for tests and unusual
// setups.
type Client struct {
	Path      string // path or name of the opencode binary
	ServerURL string
	Password  string
	HTTP      *http.Client
}

func NewClient(path string) *Client {
	return &Client{Path: strings.TrimSpace(path)}
}

func (c *Client) ID() engines.ID { return engines.OpenCode }

// Resolve finds the opencode binary.
func (c *Client) Resolve() (string, error) {
	name := strings.TrimSpace(c.Path)
	if name == "" {
		name = "opencode"
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", core.NewError(core.CodeEngineMissing, "", "",
			"OpenCode is not installed", InstallHint())
	}
	return resolved, nil
}

func (c *Client) Check() error {
	_, err := c.Resolve()
	return err
}

// Probe reports whether OpenCode can be used. A stopped background service is
// not a failure: the first humanize run starts it. A missing binary is.
func (c *Client) Probe(parent context.Context) engines.Info {
	info := engines.Info{ID: engines.OpenCode, Name: "opencode", Detail: "free text models via the local OpenCode service"}
	path, err := c.Resolve()
	if err != nil {
		info.Hint = InstallHint()
		info.Err = err
		return info
	}
	info.Path = path
	info.Available = true

	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, path, "--version").Output(); err == nil {
		info.Version = firstLine(out)
	}

	models, err := c.freeModels(ctx, false)
	switch {
	case err == nil:
		info.Detail = fmt.Sprintf("%d free text models ready", len(models))
	case IsAuthError(err):
		info.Available = false
		info.Detail = "server is running, but the service password was rejected"
		info.Hint = "restart OpenCode with `opencode service restart`"
		info.Err = err
	default:
		info.Detail = "installed; the service starts on first use"
	}
	return info
}

// FreeModels returns the free text models available through OpenCode, ordered
// so the models that have been most reliable for rewriting come first.
func (c *Client) FreeModels(ctx context.Context) ([]ModelRef, error) {
	return c.freeModels(ctx, true)
}

func (c *Client) freeModels(ctx context.Context, start bool) ([]ModelRef, error) {
	base, pass, err := c.ensureServer(ctx, start)
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []modelInfo `json:"data"`
	}
	if err := c.doJSON(ctx, base, pass, http.MethodGet, "/api/model", nil, &response); err != nil {
		return nil, err
	}

	out := make([]ModelRef, 0, len(response.Data))
	for _, m := range response.Data {
		if !m.usable() {
			continue
		}
		ref := ModelRef{ProviderID: m.ProviderID, ID: m.ID}
		if m.hasVariant("none") {
			ref.Variant = "none"
		}
		out = append(out, ref)
	}
	if len(out) == 0 {
		return nil, core.NewError(core.CodeEngineMissing, "", "",
			"OpenCode has no free text models available",
			"enable or connect a free provider in OpenCode, then run `opencode models`")
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := curatedRank(out[i].String()), curatedRank(out[j].String())
		if ri != rj {
			return ri < rj
		}
		if out[i].ProviderID != out[j].ProviderID {
			return out[i].ProviderID < out[j].ProviderID
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Generate runs one prompt on one model. It creates a throwaway session, so
// calls are independent and safe to run concurrently. OpenCode's "generate"
// endpoint returns transient text without mutating session history, which keeps
// a hundred-page job from dragging an ever-growing conversation behind it.
func (c *Client) Generate(ctx context.Context, model ModelRef, prompt string) (string, error) {
	if strings.TrimSpace(model.ID) == "" {
		return "", errors.New("opencode: empty model id")
	}
	base, pass, err := c.ensureServer(ctx, true)
	if err != nil {
		return "", err
	}

	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.doJSON(ctx, base, pass, http.MethodPost, "/api/session", map[string]any{}, &created); err != nil {
		return "", err
	}
	sessionID := strings.TrimSpace(created.Data.ID)
	if sessionID == "" {
		return "", errors.New("opencode: session creation returned no id")
	}
	defer c.deleteSession(sessionID, base, pass)

	ref := map[string]string{"providerID": model.ProviderID, "id": model.ID}
	if model.Variant != "" {
		ref["variant"] = model.Variant
	}
	if err := c.doJSON(ctx, base, pass, http.MethodPost,
		"/api/session/"+urlPath(sessionID)+"/model", map[string]any{"model": ref}, nil); err != nil {
		return "", err
	}

	var generated struct {
		Data struct {
			Text string `json:"text"`
		} `json:"data"`
	}
	if err := c.doJSON(ctx, base, pass, http.MethodPost,
		"/api/session/"+urlPath(sessionID)+"/generate", map[string]string{"prompt": prompt}, &generated); err != nil {
		return "", err
	}
	text := strings.TrimSpace(generated.Data.Text)
	if text == "" {
		return "", &APIError{Status: http.StatusBadGateway, Message: "the model returned an empty response"}
	}
	return text, nil
}

func (c *Client) deleteSession(sessionID, base, pass string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// A timed-out or cancelled request may still be generating on the server.
	// Stop it before deleting the session so it does not burn free quota.
	_ = c.doJSON(ctx, base, pass, http.MethodPost, "/api/session/"+urlPath(sessionID)+"/interrupt", nil, nil)
	_ = c.doJSON(ctx, base, pass, http.MethodDelete, "/api/session/"+urlPath(sessionID), nil, nil)
}

// ensureServer discovers the running service, starting it if needed. OpenCode
// normally keeps a service running; Workbench starts it only when a humanize
// job actually needs a model.
func (c *Client) ensureServer(ctx context.Context, start bool) (string, string, error) {
	base := strings.TrimSpace(c.ServerURL)
	if base == "" {
		base = strings.TrimSpace(os.Getenv("OPENCODE_SERVER_URL"))
	}
	pass := strings.TrimSpace(c.Password)
	if pass == "" {
		pass = strings.TrimSpace(os.Getenv("OPENCODE_SERVICE_PASSWORD"))
	}

	if base == "" {
		base, _ = c.serviceURL(ctx)
	}
	if base == "" && start {
		if err := c.startService(ctx); err != nil {
			return "", "", err
		}
		deadline := time.Now().Add(serviceStartWait)
		for time.Now().Before(deadline) {
			base, _ = c.serviceURL(ctx)
			if base != "" {
				break
			}
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
	if base == "" {
		return "", "", core.NewError(core.CodeEngineMissing, "", "",
			"the OpenCode background service is not running",
			"run `opencode service start`, then retry")
	}
	if pass == "" {
		pass = readServicePassword()
	}
	if pass == "" {
		return "", "", core.NewError(core.CodeEngineMissing, "", "",
			"cannot read the OpenCode service password",
			"set OPENCODE_SERVICE_PASSWORD, or restart OpenCode with `opencode service restart`")
	}
	base = strings.TrimRight(base, "/")
	return base, pass, nil
}

func (c *Client) serviceURL(ctx context.Context) (string, error) {
	binary, err := c.Resolve()
	if err != nil {
		return "", err
	}
	runCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, binary, "service", "status")
	executil.Configure(cmd)
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return parseServiceURL(string(out)), nil
}

func (c *Client) startService(ctx context.Context) error {
	binary, err := c.Resolve()
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithTimeout(ctx, serviceStartWait)
	defer cancel()
	cmd := exec.CommandContext(runCtx, binary, "service", "start")
	executil.Configure(cmd)
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		if runCtx.Err() != nil {
			return core.NewError(core.CodeEngineMissing, "", "",
				"timed out starting the OpenCode background service",
				"run `opencode service start` manually, then retry")
		}
		return core.Wrap(core.CodeEngineMissing, "", "", err,
			"run `opencode service start` manually; "+firstLine(out))
	}
	return nil
}

var serviceURLPattern = regexp.MustCompile(`https?://[^\s]+`)

func parseServiceURL(output string) string {
	matches := serviceURLPattern.FindAllString(output, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		value := strings.TrimRight(matches[i], ".,;)'\"")
		if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
			return value
		}
	}
	return ""
}

func readServicePassword() string {
	home, _ := os.UserHomeDir()
	var candidates []string
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		candidates = append(candidates, filepath.Join(xdg, "opencode", "service.json"))
	}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".config", "opencode", "service.json"),
			filepath.Join(home, ".local", "share", "opencode", "service.json"),
		)
	}
	if dir, err := os.UserConfigDir(); err == nil {
		candidates = append(candidates, filepath.Join(dir, "opencode", "service.json"))
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var payload struct {
			Password string `json:"password"`
		}
		if json.Unmarshal(data, &payload) == nil && strings.TrimSpace(payload.Password) != "" {
			return strings.TrimSpace(payload.Password)
		}
	}
	return ""
}

func (c *Client) doJSON(ctx context.Context, base, pass, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("opencode:"+pass)))

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp.StatusCode, data)
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("opencode: cannot decode response: %w", err)
	}
	return nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{}
}

func decodeAPIError(status int, body []byte) *APIError {
	e := &APIError{Status: status}
	var payload struct {
		Tag          string `json:"_tag"`
		Message      string `json:"message"`
		RetryAfterMS int64  `json:"retryAfterMs"`
		Error        struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
		Data struct {
			Type         string `json:"type"`
			Message      string `json:"message"`
			RetryAfterMS int64  `json:"retryAfterMs"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &payload) == nil {
		e.Tag = payload.Tag
		switch {
		case payload.Message != "":
			e.Message = payload.Message
		case payload.Error.Message != "":
			e.Message = payload.Error.Message
		case payload.Data.Message != "":
			e.Message = payload.Data.Message
		case payload.Error.Type != "":
			e.Message = payload.Error.Type
		case payload.Data.Type != "":
			e.Message = payload.Data.Type
		}
		if payload.RetryAfterMS == 0 {
			payload.RetryAfterMS = payload.Data.RetryAfterMS
		}
		if payload.RetryAfterMS > 0 {
			e.RetryAfter = time.Duration(payload.RetryAfterMS) * time.Millisecond
		}
	}
	if e.Message == "" {
		e.Message = strings.TrimSpace(string(body))
	}
	if len(e.Message) > 500 {
		e.Message = e.Message[:500] + "…"
	}
	return e
}

func urlPath(value string) string {
	return strings.ReplaceAll(value, "/", "%2F")
}

func firstLine(out []byte) string {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return ""
	}
	line, _, _ := strings.Cut(text, "\n")
	return strings.TrimSpace(line)
}

// InstallHint is shown when the opencode binary is missing.
func InstallHint() string {
	return "install OpenCode from https://opencode.ai, then run `opencode service start`"
}

type modelInfo struct {
	ID           string         `json:"id"`
	ProviderID   string         `json:"providerID"`
	Name         string         `json:"name"`
	Cost         []modelCost    `json:"cost"`
	Capabilities modelCaps      `json:"capabilities"`
	Variants     []modelVariant `json:"variants"`
	Status       string         `json:"status"`
	Enabled      bool           `json:"enabled"`
	Limit        modelLimit     `json:"limit"`
}

type modelCost struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
	Cache  struct {
		Read  float64 `json:"read"`
		Write float64 `json:"write"`
	} `json:"cache"`
}

type modelCaps struct {
	Output []string `json:"output"`
}

type modelVariant struct {
	ID string `json:"id"`
}

type modelLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

func (m modelInfo) usable() bool {
	if m.ProviderID == "" || m.ID == "" {
		return false
	}
	if !m.Enabled || (m.Status != "" && m.Status != "active") {
		return false
	}
	if m.Limit.Context > 0 && m.Limit.Context < 16_000 {
		return false
	}
	if !m.hasTextOutput() {
		return false
	}
	if !m.isFree() {
		return false
	}
	name := strings.ToLower(m.ID + " " + m.Name)
	for _, blocked := range []string{"embed", "rerank", "guard", "safety", "voicechat", "paligemma"} {
		if strings.Contains(name, blocked) {
			return false
		}
	}
	switch m.ProviderID {
	case "opencode", "opencode-go", "nvidia":
		return true
	default:
		return false
	}
}

func (m modelInfo) hasTextOutput() bool {
	if len(m.Capabilities.Output) == 0 {
		return true
	}
	for _, output := range m.Capabilities.Output {
		if output == "text" {
			return true
		}
	}
	return false
}

func (m modelInfo) isFree() bool {
	if len(m.Cost) == 0 {
		return false
	}
	for _, cost := range m.Cost {
		if cost.Input != 0 || cost.Output != 0 || cost.Cache.Read != 0 || cost.Cache.Write != 0 {
			return false
		}
	}
	return true
}

func (m modelInfo) hasVariant(id string) bool {
	for _, variant := range m.Variants {
		if variant.ID == id {
			return true
		}
	}
	return false
}

// curated models are tried first. The order puts reliable, fast, free text
// models ahead of the OpenCode free tier, which is often rate-limited.
var curatedModels = []string{
	"nvidia/nvidia/nemotron-3-ultra-550b-a55b",
	"nvidia/nvidia/nemotron-3.5-super-vl-120b-a12b",
	"nvidia/nvidia/nemotron-3.5-lightning-30b-a3b",
	"nvidia/minimaxai/minimax-m3.1-flash",
	"nvidia/poolside/laguna-xs-2.1",
	"nvidia/openai/gpt-oss-20b",
	"nvidia/stepfun-ai/step-5-preview",
	"opencode/deepseek-v4.1-flash-free",
	"opencode/glm-5.3-flash-free",
	"opencode/mimo-v2.6-flash-free",
	"opencode/big-pickle",
	"opencode-go/iceman-contributor-free",
	"opencode-go/beam-free",
}

var curatedIndex = func() map[string]int {
	out := make(map[string]int, len(curatedModels))
	for i, id := range curatedModels {
		out[id] = i
	}
	return out
}()

func curatedRank(id string) int {
	if rank, ok := curatedIndex[id]; ok {
		return rank
	}
	return len(curatedModels) + 100
}
