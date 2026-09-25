package main

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// explain is the one command that talks to a server. every other package only
// reads the tree and git, and linkage is decidable in go: a package that does
// not link the net tree has no way to dial.
var mayLinkNetwork = []string{"github.com/dfedoryshchev/warpmap/internal/explain"}

func TestAnalysisPackagesLinkNoNetworkStack(t *testing.T) {
	if len(mayLinkNetwork) != 1 {
		t.Fatalf("%d packages may link the network stack, want exactly 1: %v", len(mayLinkNetwork), mayLinkNetwork)
	}
	// go test puts its own toolchain first on PATH, so a missing go here means the
	// binary was run by hand; skipping would let the guard pass without looking.
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go is not on PATH: %v", err)
	}
	out, err := exec.Command(goBin, "list", "-f", "{{.ImportPath}} {{join .Deps \" \"}}", "./internal/...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}

	listed := map[string]bool{}
	checked := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pkg := fields[0]
		listed[pkg] = true
		if slices.Contains(mayLinkNetwork, pkg) {
			continue
		}
		checked++
		if bad := networkDeps(fields[1:]); len(bad) > 0 {
			t.Errorf("%s links %v", pkg, bad)
		}
	}
	if checked == 0 {
		t.Fatalf("no packages checked:\n%s", out)
	}
	for _, pkg := range mayLinkNetwork {
		if !listed[pkg] {
			t.Errorf("%s is allowed the network stack but no longer exists", pkg)
		}
	}
}

func TestNetworkDepsFindsTheNetTree(t *testing.T) {
	deps := []string{"fmt", "net", "net/http", "net/url", "netlify.example/net", "os", "vendor/golang.org/x/net/idna"}
	want := []string{"net", "net/http", "net/url"}
	if got := networkDeps(deps); !slices.Equal(got, want) {
		t.Fatalf("networkDeps = %v, want %v", got, want)
	}
	if got := networkDeps([]string{"fmt", "os/exec", "encoding/json"}); len(got) != 0 {
		t.Fatalf("networkDeps flagged %v in a list with no network package", got)
	}
}

func networkDeps(deps []string) []string {
	var out []string
	for _, d := range deps {
		if d == "net" || strings.HasPrefix(d, "net/") {
			out = append(out, d)
		}
	}
	return out
}
