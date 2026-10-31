// Package notify sends best-effort desktop notifications and webhook callbacks
// for finished jobs. It never blocks a job and never treats a missing notifier
// as a failure.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type Job struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	DurationMS int64    `json:"duration_ms"`
	BytesIn    int64    `json:"bytes_in,omitempty"`
	BytesOut   int64    `json:"bytes_out,omitempty"`
	Outputs    []string `json:"outputs,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// Desktop sends a native notification. Missing tools return an error so callers
// can log it, but callers normally ignore it.
func Desktop(ctx context.Context, title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf("display notification %s with title %s", appleString(body), appleString(title))
		return exec.CommandContext(ctx, "osascript", "-e", script).Run()
	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			return fmt.Errorf("notify-send is not installed")
		}
		return exec.CommandContext(ctx, "notify-send", title, body).Run()
	case "windows":
		return fmt.Errorf("desktop notifications are not wired on Windows")
	default:
		return fmt.Errorf("desktop notifications are not supported on %s", runtime.GOOS)
	}
}

func appleString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\n", " ")
	return `"` + value + `"`
}

// Webhook posts a JSON job record to a user-supplied endpoint.
func Webhook(ctx context.Context, url string, job Job) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil
	}
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %s", resp.Status)
	}
	return nil
}
