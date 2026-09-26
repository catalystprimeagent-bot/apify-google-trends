package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResultWriter_PushLocalRoundTrip(t *testing.T) {
	dir := t.TempDir()
	env := actorEnv{isAtHome: false, localStorageDir: dir}
	w := newResultWriter(env, nil)

	rows := []ResultRow{
		{Term: "coffee", OutputType: outputInterestOverTime, Status: statusOK},
		{Term: "tea", OutputType: outputRelatedQueries, Status: statusNoData},
	}
	if err := w.Push(rows); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items := filepath.Join(dir, "datasets", "default")
	entries, err := os.ReadDir(items)
	if err != nil {
		t.Fatalf("reading dataset dir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 written items, got %d", len(entries))
	}

	b, err := os.ReadFile(filepath.Join(items, entries[0].Name()))
	if err != nil {
		t.Fatalf("reading item file: %v", err)
	}
	var got ResultRow
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("decoding item file: %v", err)
	}
	if got.Term != "coffee" {
		t.Errorf("got term %q, want coffee", got.Term)
	}
}

func TestResultWriter_PushEmptyIsNoop(t *testing.T) {
	dir := t.TempDir()
	w := newResultWriter(actorEnv{isAtHome: false, localStorageDir: dir}, nil)
	if err := w.Push(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "datasets")); !os.IsNotExist(err) {
		t.Error("expected no dataset dir to be created for an empty push")
	}
}

func TestCountExistingLocalItems(t *testing.T) {
	dir := t.TempDir()
	env := actorEnv{localStorageDir: dir}
	if got := countExistingLocalItems(env); got != 0 {
		t.Errorf("expected 0 for missing dir, got %d", got)
	}
	sub := filepath.Join(dir, "datasets", "default")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"000000001.json", "000000002.json", "not-json.txt"} {
		if err := os.WriteFile(filepath.Join(sub, name), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := countExistingLocalItems(env); got != 2 {
		t.Errorf("got %d, want 2 (only .json files counted)", got)
	}
}
