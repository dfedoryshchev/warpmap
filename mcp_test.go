package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type toolReply struct {
	ID     int `json:"id"`
	Result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"result"`
}

func TestMCPServerAnswersTheNextCallAfterAFailedOne(t *testing.T) {
	broken := chainProject(t)
	if err := os.WriteFile(filepath.Join(broken, "warpmap.json"), []byte(`{"thresholds":`), 0o644); err != nil {
		t.Fatal(err)
	}
	good := chainProject(t)

	call := func(id int, dir string) string {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call",
			"params": map[string]any{"name": "trace", "arguments": map[string]any{"dir": dir, "file": "src/c.ts"}}})
		return string(b) + "\n"
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "WARPMAP_SERVE_MCP=1")
	cmd.Stdin = strings.NewReader(call(1, broken) + call(2, good))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("server exited with %v after answering %q", err, out)
	}

	var replies []toolReply
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var r toolReply
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("reply %q: %v", line, err)
		}
		replies = append(replies, r)
	}
	if len(replies) != 2 {
		t.Fatalf("got %d replies, want 2: %q", len(replies), out)
	}
	first, second := replies[0], replies[1]
	if first.ID != 1 || !first.Result.IsError || len(first.Result.Content) != 1 ||
		!strings.Contains(first.Result.Content[0].Text, "warpmap.json") {
		t.Errorf("failed call reply = %+v, want an isError result naming warpmap.json", first)
	}
	want := "2 files depend on src/c.ts\nsrc/a.ts\nsrc/b.ts\n"
	if second.ID != 2 || second.Result.IsError || len(second.Result.Content) != 1 || second.Result.Content[0].Text != want {
		t.Errorf("next call reply = %+v, want the dependents %q", second, want)
	}
}
