package explain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Options carries the model + key. Both empty -> deterministic offline fallback.
type Options struct {
	Model  string
	APIKey string
}

func offline(file string, churn, complexity, blast int) string {
	return fmt.Sprintf(
		"%s is a hotspot: churn %d, complexity %d, %d files depend on it. A change here has a "+
			"wide blast radius - read its public surface and its heaviest dependents, and add a "+
			"test before you touch it.",
		file, churn, complexity, blast)
}

// Hotspot asks the model to explain the risk of a hotspot in plain language, grounded
// in its graph context. Falls back to a deterministic summary offline.
func Hotspot(file string, churn, complexity, blast int, opts Options) string {
	if opts.APIKey == "" || opts.Model == "" {
		return offline(file, churn, complexity, blast)
	}
	prompt := fmt.Sprintf(
		"A codebase audit flagged %s as a hotspot (churn %d, complexity %d, %d files depend on "+
			"it). In 3-4 sentences, explain the risk of changing it and what to check first.",
		file, churn, complexity, blast)

	body, _ := json.Marshal(map[string]any{
		"model":      opts.Model,
		"max_tokens": 400,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
	})
	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return offline(file, churn, complexity, blast)
	}
	req.Header.Set("x-api-key", opts.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return offline(file, churn, complexity, blast)
	}
	defer resp.Body.Close()

	var out struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) == nil && len(out.Content) > 0 && out.Content[0].Text != "" {
		return out.Content[0].Text
	}
	return offline(file, churn, complexity, blast)
}
