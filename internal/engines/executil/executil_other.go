//go:build !unix

package executil

import "os/exec"

func configure(cmd *exec.Cmd) {}
