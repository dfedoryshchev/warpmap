package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// markerScript writes a script, outside the repo, that leaves a file behind
// whenever anything runs it.
func markerScript(t *testing.T) (script, marker string) {
	t.Helper()
	bin := t.TempDir()
	marker = filepath.Join(bin, "ran")
	script = filepath.Join(bin, "hook.sh")
	body := "#!/bin/sh\necho \"$0 $*\" >> '" + filepath.ToSlash(marker) + "'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(script), marker
}

func gitOutput(t *testing.T, dir, stdin string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// configProject is a repo with one committed file and an ignored folder, so
// both the ls-files path and the check-ignore path get asked about it.
func configProject(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	writeFiles(t, repo, map[string]string{
		".gitignore":   "drop/\n",
		"src/a.ts":     "export const a = 1;\n",
		"drop/skip.ts": "export const s = 1;\n",
	})
	gitIn(t, repo, "init")
	gitIn(t, repo, "add", "--", ".gitignore", "src")
	gitIn(t, repo, "commit", "-m", "add the project")
	return repo
}

func runEveryGitPath(t *testing.T, repo string) {
	t.Helper()
	inGitRepo(repo)
	gitFiles(repo)
	gitFiles(filepath.Join(repo, "drop"))
	hotspotsBoth(t, repo)
	captureStderr(t, func() int {
		_, code := captureStdout(t, func() int { return ownershipCmd([]string{repo}) })
		return code
	})
}

func assertNotRun(t *testing.T, marker string) {
	t.Helper()
	if b, err := os.ReadFile(marker); err == nil {
		t.Fatalf("the repo's config made git run a program:\n%s", b)
	}
}

func TestGitDoesNotRunTheRepoFsmonitorHook(t *testing.T) {
	repo := configProject(t)
	script, marker := markerScript(t)
	gitIn(t, repo, "config", "core.fsmonitor", script)
	os.Remove(marker)

	// the hook has to fire through plain git, or the test proves nothing.
	exec.Command("git", "-C", repo, "ls-files").Run()
	if _, err := os.Stat(marker); err != nil {
		t.Skip("this git does not run a core.fsmonitor hook")
	}
	os.Remove(marker)

	runEveryGitPath(t, repo)
	assertNotRun(t, marker)
}

func TestGitDoesNotRunTheRepoGpgProgram(t *testing.T) {
	repo := configProject(t)
	commit := gitOutput(t, repo, "", "cat-file", "commit", "HEAD")
	head, msg, _ := strings.Cut(commit, "\n\n")
	sig := "gpgsig -----BEGIN PGP SIGNATURE-----\n \n iQEzBAABCAAdFiEE\n -----END PGP SIGNATURE-----"
	signed := gitOutput(t, repo, head+"\n"+sig+"\n\n"+msg+"\n", "hash-object", "-t", "commit", "-w", "--stdin")
	gitIn(t, repo, "update-ref", "HEAD", signed)

	script, marker := markerScript(t)
	gitIn(t, repo, "config", "log.showSignature", "true")
	gitIn(t, repo, "config", "gpg.program", script)
	os.Remove(marker)

	exec.Command("git", "-C", repo, "log", "--format=%H").Run()
	if _, err := os.Stat(marker); err != nil {
		t.Skip("this git does not start gpg.program for log.showSignature")
	}
	os.Remove(marker)

	runEveryGitPath(t, repo)
	assertNotRun(t, marker)
}
