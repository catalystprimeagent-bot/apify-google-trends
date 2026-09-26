package main

import "testing"

func TestAllOutputs_CanonicalOrder(t *testing.T) {
	want := []string{outputInterestOverTime, outputRelatedQueries}
	if len(allOutputs) != len(want) {
		t.Fatalf("got %v, want %v", allOutputs, want)
	}
	for i, w := range want {
		if allOutputs[i] != w {
			t.Errorf("index %d: got %q, want %q", i, allOutputs[i], w)
		}
	}
}

func TestResultRow_JSONOmitsEmptyOptionalFields(t *testing.T) {
	row := ResultRow{Term: "coffee", OutputType: outputInterestOverTime, Status: statusOK}
	if row.Data != nil {
		t.Error("expected nil Data by default")
	}
	if row.Error != "" {
		t.Error("expected empty Error by default")
	}
}
