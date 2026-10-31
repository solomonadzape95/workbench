package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"text/tabwriter"
	"time"
)

// PrintDoctor writes a human-readable engine report and reports whether every
// registered engine is ready.
func (a *App) PrintDoctor(w io.Writer) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	infos := a.Registry.ProbeAll(ctx)

	configState := ""
	if _, err := os.Stat(a.Paths.ConfigFile); errors.Is(err, fs.ErrNotExist) {
		configState = " (not found — using defaults)"
	}
	fmt.Fprintf(w, "Config  %s%s\n", a.Paths.ConfigFile, configState)
	for _, warning := range a.Warnings {
		fmt.Fprintf(w, "Warning %s\n", warning)
	}

	if len(infos) == 0 {
		fmt.Fprintln(w, "\nNo engines are registered.")
		return false
	}

	fmt.Fprintln(w)
	allReady := true
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ENGINE\tSTATUS\tVERSION\tPATH\tDETAIL")
	for _, info := range infos {
		status := "ready"
		if !info.Available {
			status = "missing"
			allReady = false
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			orDash(info.Name),
			status,
			orDash(info.Version),
			orDash(info.Path),
			orDash(info.Detail),
		)
	}
	if err := tw.Flush(); err != nil {
		fmt.Fprintf(w, "cannot write report: %v\n", err)
		return false
	}

	firstHint := true
	for _, info := range infos {
		if info.Available || info.Hint == "" {
			continue
		}
		if firstHint {
			fmt.Fprintln(w)
			firstHint = false
		}
		fmt.Fprintf(w, "%s: %s\n", info.Name, info.Hint)
	}
	return allReady
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
