package signal

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fake struct {
	tool string
	fs   []Finding
	err  error
}

func (f fake) Tool() string                       { return f.tool }
func (f fake) Findings(string) ([]Finding, error) { return f.fs, f.err }

func TestCollectKeepsAdapterOrderAndStampsTheTool(t *testing.T) {
	own := fake{tool: Warpmap, fs: []Finding{{ID: "wm-cycle-00000000", RuleID: "cycle", Severity: Med}}}
	other := fake{tool: "gitleaks", fs: []Finding{
		{RuleID: "generic-api-key", Severity: High, Locations: []Location{{File: "src/a.ts", Line: 3}}},
		{SourceTool: "warpmap", RuleID: "aws-key", Severity: High},
	}}
	got, unread := Collect("dir", own, other)
	if len(unread) != 0 {
		t.Fatalf("nothing failed and unread is %+v", unread)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 findings in adapter order, got %+v", got)
	}
	for i, want := range []string{"cycle", "generic-api-key", "aws-key"} {
		if got[i].RuleID != want {
			t.Fatalf("finding %d is %s, want %s: %+v", i, got[i].RuleID, want, got)
		}
	}
	for i, want := range []string{Warpmap, "gitleaks", "gitleaks"} {
		if got[i].SourceTool != want {
			t.Fatalf("finding %d says it came from %q, its adapter is %q", i, got[i].SourceTool, want)
		}
	}
	if got[1].Locations[0] != (Location{File: "src/a.ts", Line: 3}) {
		t.Fatalf("the location did not survive: %+v", got[1])
	}
}

// one id scheme: an ingested finding is known by its tool and its tool's rule,
// never by an id that reads as warpmap's own.
func TestCollectGivesNoWarpmapIDToAForeignFinding(t *testing.T) {
	got, _ := Collect("dir",
		fake{tool: Warpmap, fs: []Finding{{ID: "wm-dead-code-12345678", RuleID: "dead-code"}}},
		fake{tool: "semgrep", fs: []Finding{{ID: "wm-cycle-deadbeef", RuleID: "x"}}},
	)
	if len(got) != 2 {
		t.Fatalf("want 2 findings, got %+v", got)
	}
	if got[0].ID != "wm-dead-code-12345678" {
		t.Fatalf("warpmap's own id was dropped: %+v", got[0])
	}
	if got[1].ID != "" {
		t.Fatalf("a semgrep finding kept the id %q", got[1].ID)
	}
}

func TestCollectNamesAnAdapterThatFailedAndKeepsTheRest(t *testing.T) {
	got, unread := Collect("dir",
		fake{tool: "lcov", err: errors.New("coverage/lcov.info: line 4: no SF record")},
		fake{tool: "absent"},
		fake{tool: Warpmap, fs: []Finding{{RuleID: "cycle"}}},
	)
	if len(got) != 1 || got[0].RuleID != "cycle" {
		t.Fatalf("one failed adapter cost the others their findings: %+v", got)
	}
	if len(unread) != 1 || unread[0].Tool != "lcov" || !strings.Contains(unread[0].Error, "no SF record") {
		t.Fatalf("want the lcov failure named, got %+v", unread)
	}
}

func TestAFindingEncodesItsLocationsAsAListAndKeepsTheRawPayload(t *testing.T) {
	got, _ := Collect("dir", fake{tool: "gitleaks", fs: []Finding{
		{RuleID: "generic-api-key", Raw: json.RawMessage(`{"RuleID":"generic-api-key"}`)},
	}})
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %+v", got)
	}
	b, err := json.Marshal(got[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"locations":[]`, `"raw":{"RuleID":"generic-api-key"}`, `"source_tool":"gitleaks"`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("want %s in %s", want, b)
		}
	}
}
