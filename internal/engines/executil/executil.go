// Package executil configures external commands for Workbench's cancellation
// model. On Unix, the child gets its own process group so cancellation kills
// helpers as well as the parent process.
package executil

import "os/exec"

func Configure(cmd *exec.Cmd) {
	configure(cmd)
}
