package main

import "testing"

func TestTermsFromInput_AllAliases(t *testing.T) {
	in := Input{
		SearchTerms: []interface{}{"coffee"},
		Terms:       []interface{}{map[string]interface{}{"term": "yoga mats"}},
		Keywords:    "electric cars",
		Queries:     "sourdough bread, running shoes",
		Term:        "air fryer",
	}
	terms, rejected := termsFromInput(in)

	if len(rejected) != 0 {
		t.Fatalf("unexpected rejects: %v", rejected)
	}
	want := []string{"coffee", "yoga mats", "electric cars", "sourdough bread", "running shoes", "air fryer"}
	if len(terms) != len(want) {
		t.Fatalf("got %d terms, want %d: %+v", len(terms), len(want), terms)
	}
	for i, w := range want {
		if terms[i] != w {
			t.Errorf("term %d: got %q, want %q", i, terms[i], w)
		}
	}
}

func TestTermsFromInput_Dedup(t *testing.T) {
	in := Input{SearchTerms: []interface{}{"Coffee", "coffee", " COFFEE "}}
	terms, _ := termsFromInput(in)
	if len(terms) != 1 || terms[0] != "Coffee" {
		t.Fatalf("expected dedup to 1 entry keeping first casing, got %+v", terms)
	}
}

func TestTermsFromInput_RejectsEmptyEntries(t *testing.T) {
	in := Input{SearchTerms: []interface{}{"coffee", "", "  ", "electric cars"}}
	terms, rejected := termsFromInput(in)
	if len(terms) != 2 {
		t.Fatalf("expected 2 valid terms, got %+v", terms)
	}
	if len(rejected) != 2 {
		t.Fatalf("expected 2 rejected empty entries, got %v", rejected)
	}
}

func TestTermsFromInput_Empty(t *testing.T) {
	terms, rejected := termsFromInput(Input{})
	if len(terms) != 0 || len(rejected) != 0 {
		t.Fatalf("expected nothing from empty input, got terms=%+v rejected=%v", terms, rejected)
	}
}

func TestResolveTimeRange(t *testing.T) {
	cases := []struct {
		in        *string
		wantEnum  string
		wantParam string
	}{
		{nil, "past12Months", "today 12-m"},
		{strPtr("past7Days"), "past7Days", "now 7-d"},
		{strPtr("pastHour"), "pastHour", "now 1-H"},
		{strPtr("past4Hours"), "past4Hours", "now 4-H"},
		{strPtr("pastDay"), "pastDay", "now 1-d"},
		{strPtr("past30Days"), "past30Days", "today 1-m"},
		{strPtr("past90Days"), "past90Days", "today 3-m"},
		{strPtr("past5Years"), "past5Years", "today 5-y"},
		{strPtr("nonsense"), "past12Months", "today 12-m"},
	}
	for _, c := range cases {
		gotEnum, gotParam := resolveTimeRange(c.in)
		if gotEnum != c.wantEnum || gotParam != c.wantParam {
			t.Errorf("resolveTimeRange(%v): got (%q, %q), want (%q, %q)", c.in, gotEnum, gotParam, c.wantEnum, c.wantParam)
		}
	}
}

func TestResolveCategory(t *testing.T) {
	if got := resolveCategory(nil); got != 0 {
		t.Errorf("nil category: got %d, want 0", got)
	}
	v := 71
	if got := resolveCategory(&v); got != 71 {
		t.Errorf("explicit category: got %d, want 71", got)
	}
}

func TestResolveOutputs(t *testing.T) {
	if got := resolveOutputs(nil); len(got) != 2 {
		t.Errorf("nil outputs: got %v, want both", got)
	}
	if got := resolveOutputs([]string{"relatedQueries", "interestOverTime"}); len(got) != 2 || got[0] != "interestOverTime" || got[1] != "relatedQueries" {
		t.Errorf("subset outputs: got %v, want canonical order [interestOverTime relatedQueries]", got)
	}
	if got := resolveOutputs([]string{"bogus"}); len(got) != 2 {
		t.Errorf("all-invalid outputs: got %v, want fallback to both", got)
	}
	// relatedTopics was removed (CAT-48, memory/030): a saved input still requesting it must
	// not error, just fall back like any other unrecognized value.
	if got := resolveOutputs([]string{"relatedTopics"}); len(got) != 2 {
		t.Errorf("relatedTopics-only outputs: got %v, want fallback to both", got)
	}
}

func TestResolveTimeoutSeconds(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 5},
		{3, 5},
		{20, 20},
		{60, 60},
		{999, 60},
	}
	for _, c := range cases {
		v := c.in
		if got := resolveTimeoutSeconds(&v); got != c.want {
			t.Errorf("resolveTimeoutSeconds(%d): got %d, want %d", c.in, got, c.want)
		}
	}
	if got := resolveTimeoutSeconds(nil); got != 20 {
		t.Errorf("nil timeoutSeconds: got %d, want default 20", got)
	}
}

func strPtr(s string) *string { return &s }
