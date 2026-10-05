package metrics

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// LizardComplexity runs the lizard executable at bin over files and returns each
// file's cyclomatic complexity, the sum over the functions lizard found in it. A
// file with no functions, or none lizard could read, scores 0.
func LizardComplexity(bin string, files []string) (map[string]int, error) {
	cx := make(map[string]int, len(files))
	if len(files) == 0 {
		return cx, nil
	}
	byName := make(map[string]string, len(files))
	for _, f := range files {
		cx[f] = 0
		byName[filepath.Clean(f)] = f
	}

	list, err := os.CreateTemp("", "warpmap-lizard-*.txt")
	if err != nil {
		return nil, err
	}
	defer os.Remove(list.Name())
	_, err = io.WriteString(list, strings.Join(files, "\n")+"\n")
	if cerr := list.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(bin, "--csv", "-f", list.Name())
	// lizard reads the list and prints file names in the platform's default text
	// encoding, which on windows is not utf-8: a non-ascii path would come back
	// spelled differently and match nothing.
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}

	r := csv.NewReader(bytes.NewReader(out))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rows, matched := 0, 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading lizard's csv: %w", err)
		}
		if rec[0] == "NLOC" {
			continue
		}
		if len(rec) < 7 {
			return nil, fmt.Errorf("lizard's csv has %d columns, want at least 7", len(rec))
		}
		ccn, err := strconv.Atoi(rec[1])
		if err != nil {
			return nil, fmt.Errorf("lizard's csv: CCN %q is not a number", rec[1])
		}
		rows++
		if f, ok := byName[filepath.Clean(rec[6])]; ok {
			cx[f] += ccn
			matched++
		}
	}
	if rows > 0 && matched == 0 {
		return nil, errors.New("lizard named none of the files it was given")
	}
	return cx, nil
}

// LizardHotspots is Hotspots with each file's complexity taken from the lizard
// executable at bin instead of the internal measure.
func LizardHotspots(bin, repoDir string, files []string, churn Churn) ([]Hotspot, error) {
	files = shipped(repoDir, files)
	cx, err := LizardComplexity(bin, files)
	if err != nil {
		return nil, err
	}
	return rank(repoDir, files, churn, func(f string) int { return cx[f] }), nil
}
