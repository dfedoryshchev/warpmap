// Package signal is the one shape a finding takes, whichever tool produced it,
// and the interface a source of findings implements to reach the report.
package signal

import "encoding/json"

type Severity string

const (
	High Severity = "high"
	Med  Severity = "medium"
	Low  Severity = "low"
)

// Warpmap is the source tool of the findings warpmap computes itself, the only
// ones that carry an id of warpmap's scheme.
const Warpmap = "warpmap"

// Location is a project-relative, slashed path, and a 1-based line when the
// finding points inside the file; 0 means the whole file.
type Location struct {
	File string `json:"file"`
	Line int    `json:"line,omitempty"`
}

// Finding is one finding from any tool. RuleID is the tool's own name for the
// check and Raw its record as the tool wrote it, so nothing the tool said is
// lost in the mapping.
type Finding struct {
	ID             string          `json:"id,omitempty"`
	SourceTool     string          `json:"source_tool"`
	RuleID         string          `json:"rule_id"`
	Severity       Severity        `json:"severity"`
	Message        string          `json:"message"`
	Recommendation string          `json:"recommendation,omitempty"`
	Locations      []Location      `json:"locations"`
	Raw            json.RawMessage `json:"raw,omitempty"`
}

// Adapter turns one tool's output for a project into findings. A tool whose
// output is not there is not an error: it returns no findings.
type Adapter interface {
	Tool() string
	Findings(dir string) ([]Finding, error)
}

// Unread is a source whose output was there but could not be read.
type Unread struct {
	Tool  string `json:"tool"`
	Error string `json:"error"`
}

// Collect runs the adapters in order and returns their findings in that order,
// each stamped with the tool its adapter names. Only warpmap's own findings
// keep an id: an ingested one is known by its tool and rule. A failing adapter
// is named in the second list and costs the others nothing.
func Collect(dir string, adapters ...Adapter) ([]Finding, []Unread) {
	out := []Finding{}
	unread := []Unread{}
	for _, a := range adapters {
		fs, err := a.Findings(dir)
		if err != nil {
			unread = append(unread, Unread{Tool: a.Tool(), Error: err.Error()})
			continue
		}
		for _, f := range fs {
			f.SourceTool = a.Tool()
			if f.SourceTool != Warpmap {
				f.ID = ""
			}
			if f.Locations == nil {
				f.Locations = []Location{}
			}
			out = append(out, f)
		}
	}
	return out, unread
}
