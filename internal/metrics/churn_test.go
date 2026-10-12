package metrics

import (
	"errors"
	"strings"
	"testing"
)

func TestWindowNote(t *testing.T) {
	churned := []Hotspot{{File: "a.ts", Churn: 2}, {File: "b.ts"}}
	flat := []Hotspot{{File: "a.ts"}, {File: "b.ts"}}
	for _, tc := range []struct {
		name   string
		w      Window
		ranked []Hotspot
		want   string
	}{
		{"no git", Window{Months: 6, Err: errors.New("not a repo")}, flat,
			"churn: no git history to read here - ranked by complexity alone"},
		{"empty", Window{Months: 6}, flat,
			"churn: 0 commits in the last 6 months - ranked by complexity alone; --months widens the window"},
		{"none to a ranked file", Window{Months: 1, Commits: 4}, flat,
			"churn: 4 commits in the last 1 month, none to a ranked file - ranked by complexity alone"},
		{"thin", Window{Months: 6, Commits: ThinWindow - 1}, churned,
			"churn: 19 commits in the last 6 months - under 20, so churn differences are mostly noise; --months widens the window"},
		{"enough", Window{Months: 6, Commits: ThinWindow}, churned,
			"churn: 20 commits in the last 6 months"},
	} {
		if got := tc.w.Note(tc.ranked); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
		if strings.ContainsAny(tc.w.Note(tc.ranked), "\n\t") {
			t.Errorf("%s: the note is not one line", tc.name)
		}
	}
}
