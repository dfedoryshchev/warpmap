package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dfedoryshchev/warpmap/internal/metrics"
	"github.com/dfedoryshchev/warpmap/internal/signal"
)

type tool struct {
	name string
	fs   []signal.Finding
	err  error
}

func (a tool) Tool() string                              { return a.name }
func (a tool) Findings(string) ([]signal.Finding, error) { return a.fs, a.err }

type signalPack struct {
	Findings []struct {
		ID         string `json:"id"`
		SourceTool string `json:"source_tool"`
		RuleID     string `json:"rule_id"`
		Severity   string `json:"severity"`
		Message    string `json:"message"`
		Kind       string `json:"kind"`
		Detail     string `json:"detail"`
		Locations  []struct {
			File string `json:"file"`
			Line int    `json:"line"`
		} `json:"locations"`
		Files []string        `json:"files"`
		Raw   json.RawMessage `json:"raw"`
	} `json:"findings"`
	Limitations struct {
		Unread []struct {
			Tool  string `json:"tool"`
			Error string `json:"error"`
		} `json:"unread_sources"`
	} `json:"limitations"`
}

func packOf(t *testing.T, r Report) signalPack {
	t.Helper()
	raw, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var p signalPack
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("%v\n%s", err, raw)
	}
	return p
}

// warpmap's own findings take the shape any other tool's will, so the report
// reads one list and not one per tool.
func TestOwnFindingsComeThroughTheSeam(t *testing.T) {
	dir := auditProject(t)
	p := packOf(t, Build(dir, sources(t, dir), metrics.Churn{}, nil))
	if len(p.Findings) < 3 {
		t.Fatalf("the fixture stopped producing findings: %+v", p.Findings)
	}
	for i, f := range p.Findings {
		if f.SourceTool != signal.Warpmap || f.RuleID == "" || f.RuleID != f.Kind || f.Message != f.Detail {
			t.Fatalf("finding %d is not in the shared shape: %+v", i, f)
		}
		if len(f.Locations) != len(f.Files) || len(f.Locations) == 0 {
			t.Fatalf("finding %d has %d locations for %d files", i, len(f.Locations), len(f.Files))
		}
		for j, l := range f.Locations {
			if l.File != f.Files[j] || l.Line != 0 {
				t.Fatalf("finding %d location %d is %+v, its file %s", i, j, l, f.Files[j])
			}
		}
	}
}

func TestAnIngestedFindingJoinsTheReport(t *testing.T) {
	dir := auditProject(t)
	leak := tool{name: "gitleaks", fs: []signal.Finding{{
		ID:        "wm-cycle-00000000",
		RuleID:    "generic-api-key",
		Severity:  signal.High,
		Message:   "Generic API Key",
		Locations: []signal.Location{{File: "src/hub.ts", Line: 2}},
		Raw:       json.RawMessage(`{"RuleID":"generic-api-key"}`),
	}}}
	own := Build(dir, sources(t, dir), metrics.Churn{}, nil)
	r := Build(dir, sources(t, dir), metrics.Churn{}, nil, leak)

	if len(r.Findings) != len(own.Findings)+1 {
		t.Fatalf("want warpmap's %d findings and the ingested one, got %d", len(own.Findings), len(r.Findings))
	}
	p := packOf(t, r)
	last := p.Findings[len(p.Findings)-1]
	if last.SourceTool != "gitleaks" || last.RuleID != "generic-api-key" || last.ID != "" {
		t.Fatalf("the ingested finding is not last, or carries a warpmap id: %+v", last)
	}
	if len(last.Locations) != 1 || last.Locations[0].File != "src/hub.ts" || last.Locations[0].Line != 2 {
		t.Fatalf("its location did not survive: %+v", last)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, last.Raw); err != nil || compact.String() != `{"RuleID":"generic-api-key"}` {
		t.Fatalf("its raw payload is %s", last.Raw)
	}

	md := r.Markdown()
	if !strings.Contains(md, "- **[high] gitleaks generic-api-key** - Generic API Key - `src/hub.ts:2`\n") {
		t.Fatalf("the markdown does not carry the ingested finding:\n%s", md)
	}
	if !strings.Contains(md, "- findings: "+itoa(len(r.Findings))+"\n") {
		t.Fatalf("the summary count leaves the ingested finding out:\n%s", md)
	}
}

func TestASourceThatCouldNotBeReadIsALimitation(t *testing.T) {
	dir := auditProject(t)
	broken := tool{name: "lcov", err: errors.New("coverage/lcov.info: line 4: no SF record")}
	r := Build(dir, sources(t, dir), metrics.Churn{}, nil, broken)

	if !strings.Contains(r.Markdown(), "- not read: lcov - coverage/lcov.info: line 4: no SF record\n") {
		t.Fatalf("the markdown limitations do not name the unread source:\n%s", r.Markdown())
	}
	if status, _ := r.Coverage(); status != "partial" {
		t.Fatalf("a source that could not be read leaves coverage %q", status)
	}
	p := packOf(t, r)
	if len(p.Limitations.Unread) != 1 || p.Limitations.Unread[0].Tool != "lcov" {
		t.Fatalf("the json limitations do not name the unread source: %+v", p.Limitations)
	}
	if clean := packOf(t, Build(dir, sources(t, dir), metrics.Churn{}, nil)); clean.Limitations.Unread == nil {
		t.Fatalf("with every source read, unread_sources should be an empty list, not missing")
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
