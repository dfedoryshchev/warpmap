// Package gitcmd runs git in a repository warpmap was pointed at.
package gitcmd

import (
	"os"
	"os/exec"
)

// Command is `git -C dir args...`. The repository's .git/config travels with
// a checkout someone else handed over, and two of its settings name a program
// git starts on a plain read: core.fsmonitor on every index read, gpg.program
// for each signed commit log prints under log.showSignature. Both are switched
// off on the command line, which outranks the repository's own config, and a
// partial clone is kept from fetching the objects it lacks.
func Command(dir string, args ...string) *exec.Cmd {
	full := append([]string{
		"--no-optional-locks", "-C", dir,
		"-c", "core.fsmonitor=false",
		"-c", "log.showSignature=false",
	}, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = append(os.Environ(), "GIT_NO_LAZY_FETCH=1")
	return cmd
}
