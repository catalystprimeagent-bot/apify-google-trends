package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestStripJunkPrefix(t *testing.T) {
	if got := string(stripJunkPrefix([]byte(")]}'\n{\"a\":1}"))); got != `{"a":1}` {
		t.Errorf("got %q", got)
	}
	if got := string(stripJunkPrefix([]byte(`{"a":1}`))); got != `{"a":1}` {
		t.Errorf("body without prefix should pass through unchanged, got %q", got)
	}
}

func TestBuildExploreURL(t *testing.T) {
	u, err := buildExploreURL("coffee", "today 12-m", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(u, exploreURL+"?") {
		t.Fatalf("got %q, want prefix %q", u, exploreURL+"?")
	}
	// Never construct the disallowed /explore? or /trends/explore? paths (robots.txt,
	// runtime/reports/trends-gate0.md). /trends/api/explore is the permitted, different path.
	path := strings.Split(u, "?")[0]
	if path == "https://trends.google.com/explore" || path == "https://trends.google.com/trends/explore" {
		t.Fatalf("URL touches a disallowed explore path: %q", u)
	}
	if path != exploreURL {
		t.Fatalf("unexpected path %q, want %q", path, exploreURL)
	}

	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("built URL doesn't parse: %v", err)
	}
	q := parsed.Query()
	if q.Get("hl") != "en-US" || q.Get("tz") != "0" {
		t.Errorf("unexpected hl/tz: %v", q)
	}
	req := q.Get("req")
	if !strings.Contains(req, `"keyword":"coffee"`) || !strings.Contains(req, `"time":"today 12-m"`) || !strings.Contains(req, `"category":0`) {
		t.Errorf("req payload missing expected fields: %s", req)
	}
}

func TestBuildExploreURL_EscapesSpecialCharacters(t *testing.T) {
	u, err := buildExploreURL(`weird "term" \with/ chars`, "today 12-m", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("built URL doesn't parse: %v", err)
	}
	req := parsed.Query().Get("req")
	if !strings.Contains(req, `weird \"term\"`) {
		t.Errorf("expected properly JSON-escaped quotes in req, got %s", req)
	}
}

func TestWidgetEndpoint(t *testing.T) {
	cases := map[string]string{
		"TIMESERIES":      "multiline",
		"RELATED_QUERIES": "relatedsearches",
		"RELATED_TOPICS":  "", // removed 2026-09-26, CAT-48: Google's widget returned empty for every term tested
		"GEO_MAP":         "", // not used in v1
		"UNKNOWN":         "",
	}
	for id, want := range cases {
		if got := widgetEndpoint(id); got != want {
			t.Errorf("widgetEndpoint(%q): got %q, want %q", id, got, want)
		}
	}
}

func TestWidgetIDForOutput(t *testing.T) {
	cases := map[string]string{
		outputInterestOverTime: "TIMESERIES",
		outputRelatedQueries:   "RELATED_QUERIES",
		"relatedTopics":        "", // removed 2026-09-26, CAT-48
		"bogus":                "",
	}
	for output, want := range cases {
		if got := widgetIDForOutput(output); got != want {
			t.Errorf("widgetIDForOutput(%q): got %q, want %q", output, got, want)
		}
	}
}

// withFakeWidgetServer points widgetURL at an httptest.Server for the duration of fn, then
// restores it. Real trends.google.com is never touched by this test (SPEC-005 Rule 0).
func withFakeWidgetServer(t *testing.T, body string) *trendsClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(")]}'\n" + body))
	}))
	t.Cleanup(srv.Close)
	orig := widgetURL
	widgetURL = srv.URL + "/"
	t.Cleanup(func() { widgetURL = orig })

	c, err := newTrendsClient(5 * time.Second)
	if err != nil {
		t.Fatalf("newTrendsClient: %v", err)
	}
	return c
}

func TestFetchWidgetData_RelatedQueries_MergesTopAndRising(t *testing.T) {
	// Shape confirmed against Google's real response, CAT-48 (memory/030): rankedList[0] is
	// TOP, rankedList[1] is RISING. Earlier code read only index 0, silently dropping RISING.
	body := `{"default":{"rankedList":[
		{"rankedKeyword":[{"query":"coffee shop","value":100,"hasData":true}]},
		{"rankedKeyword":[{"query":"coffee break","value":250,"hasData":true}]}
	]}}`
	c := withFakeWidgetServer(t, body)

	data, noData, _, err := c.fetchWidgetData(exploreWidget{ID: "RELATED_QUERIES", Request: []byte(`{}`)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if noData {
		t.Fatal("expected noData=false")
	}
	if len(data) != 2 {
		t.Fatalf("expected 2 merged rows (1 top + 1 rising), got %d: %+v", len(data), data)
	}
	if data[0]["query"] != "coffee shop" || data[0]["ranking"] != "top" {
		t.Errorf("row 0: expected top/coffee shop, got %+v", data[0])
	}
	if data[1]["query"] != "coffee break" || data[1]["ranking"] != "rising" {
		t.Errorf("row 1: expected rising/coffee break, got %+v", data[1])
	}
}

func TestFetchWidgetData_RelatedQueries_EmptyRankedListIsNoData(t *testing.T) {
	// The exact shape Google returned for RELATED_TOPICS on every term tested in CAT-48 —
	// HTTP 200, genuinely empty. Confirms the empty-list path is still clean, not an error.
	c := withFakeWidgetServer(t, `{"default":{"rankedList":[]}}`)

	data, noData, _, err := c.fetchWidgetData(exploreWidget{ID: "RELATED_QUERIES", Request: []byte(`{}`)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !noData || data != nil {
		t.Errorf("expected clean no-data result, got noData=%v data=%+v", noData, data)
	}
}

func TestParseExplore(t *testing.T) {
	body := []byte(")]}'\n" + `{"widgets":[{"id":"TIMESERIES","type":"fe_line_chart","token":"tok","request":{}}]}`)
	widgets, ok := parseExplore(200, body, nil)
	if !ok || len(widgets) != 1 || widgets[0].ID != "TIMESERIES" {
		t.Fatalf("expected 1 usable widget, got ok=%v widgets=%+v", ok, widgets)
	}

	if _, ok := parseExplore(429, []byte("rate limited"), nil); ok {
		t.Error("429 should not be usable")
	}
	if _, ok := parseExplore(200, []byte(")]}'\n{\"widgets\":[]}"), nil); ok {
		t.Error("empty widgets array should not be usable")
	}
	if _, ok := parseExplore(200, []byte("not json"), nil); ok {
		t.Error("unparseable body should not be usable")
	}
}
