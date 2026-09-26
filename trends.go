package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// Two operator constraints (CAT-16), enforced here rather than left to judgement:
//  1. Every request goes to the Apify platform's own egress, never the operator's connection.
//     (Enforced by where this Actor runs, not by this file — but recorded here as the reason
//     these are the only two path shapes this client is allowed to construct.)
//  2. /trends/explore? and /explore? are never requested — robots.txt disallows them for *,
//     and they are exactly the paths a headless browser would load. See
//     runtime/reports/trends-gate0.md. If a future change seems to need one of those two
//     paths, that is kill condition 1 firing late, not a detail to work around.
const (
	trendsOrigin = "https://trends.google.com/"
	exploreURL   = "https://trends.google.com/trends/api/explore"
	trendsUA     = "Mozilla/5.0 (compatible; CatalystTrendsLookup/1.0; +https://apify.com)"
)

// widgetURL is a var, not a const, so tests can point fetchWidgetData at an httptest.Server
// instead of Google.
var widgetURL = "https://trends.google.com/trends/api/widgetdata/"

// stripJunkPrefix removes Google's ")]}'\n" anti-hijacking prefix before the JSON body.
// Verified: runtime/reports/trends-gate0.md.
func stripJunkPrefix(body []byte) []byte {
	if strings.HasPrefix(string(body), ")]}'") {
		if i := strings.IndexByte(string(body), '\n'); i >= 0 {
			return body[i+1:]
		}
	}
	return body
}

type trendsClient struct {
	http *http.Client
}

func newTrendsClient(timeout time.Duration) (*trendsClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &trendsClient{http: &http.Client{Timeout: timeout, Jar: jar}}, nil
}

// warmUp fetches the bare origin to obtain the NID cookie the explore endpoint requires.
// Verified recipe: runtime/reports/trends-gate0.md.
func (c *trendsClient) warmUp() error {
	req, err := http.NewRequest(http.MethodGet, trendsOrigin, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", trendsUA)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (c *trendsClient) get(u string) (status int, body []byte, err error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", trendsUA)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", trendsOrigin)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, b, nil
}

type comparisonItem struct {
	Keyword string `json:"keyword"`
	Geo     string `json:"geo"`
	Time    string `json:"time"`
}

type exploreRequest struct {
	ComparisonItem []comparisonItem `json:"comparisonItem"`
	Category       int              `json:"category"`
	Property       string           `json:"property"`
}

type exploreWidget struct {
	Token   string          `json:"token"`
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Request json.RawMessage `json:"request"`
}

type exploreBody struct {
	Widgets []exploreWidget `json:"widgets"`
}

// buildExploreURL builds a request to /trends/api/explore. term is worldwide-only (no geo) in
// v1 — deliberately, per SPEC-005 §3.2, to keep this Actor off residential proxies.
func buildExploreURL(term, timeParam string, category int) (string, error) {
	req := exploreRequest{
		ComparisonItem: []comparisonItem{{Keyword: term, Geo: "", Time: timeParam}},
		Category:       category,
		Property:       "",
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("hl", "en-US")
	q.Set("tz", "0")
	q.Set("req", string(reqJSON))
	return exploreURL + "?" + q.Encode(), nil
}

// doExplore calls /trends/api/explore for one term, retrying once with a fresh cookie if the
// first attempt doesn't come back with usable widgets. A 429 that survives the fresh cookie is
// a real block, not a missing-credential signal (memory/023) — the caller treats a second
// failure as a clean per-row error, not something to keep retrying.
func (c *trendsClient) doExplore(term, timeParam string, category int) (widgets []exploreWidget, ms int64, err error) {
	u, err := buildExploreURL(term, timeParam, category)
	if err != nil {
		return nil, 0, err
	}

	start := time.Now()
	status, body, reqErr := c.get(u)
	ms = time.Since(start).Milliseconds()
	widgets, ok := parseExplore(status, body, reqErr)
	if ok {
		return widgets, ms, nil
	}

	firstErr := exploreErr(status, body, reqErr)
	_ = c.warmUp()
	start = time.Now()
	status, body, reqErr = c.get(u)
	ms = time.Since(start).Milliseconds()
	widgets, ok = parseExplore(status, body, reqErr)
	if ok {
		return widgets, ms, nil
	}
	return nil, ms, fmt.Errorf("explore failed after cookie refresh: %v (first attempt: %v)", exploreErr(status, body, reqErr), firstErr)
}

func parseExplore(status int, body []byte, reqErr error) ([]exploreWidget, bool) {
	if reqErr != nil || status != http.StatusOK {
		return nil, false
	}
	var eb exploreBody
	if err := json.Unmarshal(stripJunkPrefix(body), &eb); err != nil || len(eb.Widgets) == 0 {
		return nil, false
	}
	return eb.Widgets, true
}

func exploreErr(status int, body []byte, reqErr error) error {
	if reqErr != nil {
		return reqErr
	}
	n := 200
	if len(body) < n {
		n = len(body)
	}
	return fmt.Errorf("HTTP %d: %s", status, string(body[:n]))
}

// widgetEndpoint maps a widget's id to its known undocumented follow-up path. Verified:
// runtime/reports/trends-feasibility.md §2.1. RELATED_TOPICS is deliberately absent — see
// memory/030 and the allOutputs comment in types.go.
func widgetEndpoint(id string) string {
	switch id {
	case "TIMESERIES":
		return "multiline"
	case "RELATED_QUERIES":
		return "relatedsearches"
	}
	return ""
}

// widgetIDForOutput maps this Actor's output-type names to the explore response's widget ids.
func widgetIDForOutput(output string) string {
	switch output {
	case outputInterestOverTime:
		return "TIMESERIES"
	case outputRelatedQueries:
		return "RELATED_QUERIES"
	}
	return ""
}

type widgetDataResponse struct {
	Default struct {
		TimelineData []map[string]interface{} `json:"timelineData"`
		RankedList   []struct {
			RankedKeyword []map[string]interface{} `json:"rankedKeyword"`
		} `json:"rankedList"`
	} `json:"default"`
}

// fetchWidgetData calls the widget's own follow-up endpoint with its own request payload and
// token, verbatim, per the explore response — the token-then-data handshake confirmed in
// runtime/reports/trends-feasibility.md §2.2. One retry (same cookie, no refresh) on failure;
// the explore call already paid for one cookie-refresh retry, and a widget failure right after
// a successful explore is more likely transient than a fresh lockout.
//
// RELATED_QUERIES' rankedList holds up to two entries — Google's own TOP and RISING lists, in
// that order (confirmed via raw response body, CAT-48, memory/030). Earlier code read only
// rankedList[0], silently dropping RISING, the more valuable signal, from every row. Both are
// merged into one flat list here, each item tagged with a "ranking" field.
func (c *trendsClient) fetchWidgetData(w exploreWidget) (data []map[string]interface{}, noData bool, ms int64, err error) {
	endpoint := widgetEndpoint(w.ID)
	if endpoint == "" {
		return nil, false, 0, fmt.Errorf("no known follow-up endpoint for widget id %q", w.ID)
	}
	q := url.Values{}
	q.Set("hl", "en-US")
	q.Set("tz", "0")
	q.Set("req", string(w.Request))
	q.Set("token", w.Token)
	full := widgetURL + endpoint + "?" + q.Encode()

	var status int
	var body []byte
	var reqErr error
	start := time.Now()
	for attempt := 0; attempt < 2; attempt++ {
		start = time.Now()
		status, body, reqErr = c.get(full)
		ms = time.Since(start).Milliseconds()
		if reqErr == nil && status == http.StatusOK {
			break
		}
	}
	if reqErr != nil {
		return nil, false, ms, reqErr
	}
	if status != http.StatusOK {
		return nil, false, ms, exploreErr(status, body, nil)
	}

	var wd widgetDataResponse
	if err := json.Unmarshal(stripJunkPrefix(body), &wd); err != nil {
		return nil, false, ms, fmt.Errorf("decoding widgetdata response: %w", err)
	}

	switch w.ID {
	case "TIMESERIES":
		if len(wd.Default.TimelineData) == 0 {
			return nil, true, ms, nil
		}
		return wd.Default.TimelineData, false, ms, nil
	default: // RELATED_QUERIES
		var combined []map[string]interface{}
		for i, rl := range wd.Default.RankedList {
			ranking := "top"
			if i == 1 {
				ranking = "rising"
			}
			for _, kw := range rl.RankedKeyword {
				item := make(map[string]interface{}, len(kw)+1)
				for k, v := range kw {
					item[k] = v
				}
				item["ranking"] = ranking
				combined = append(combined, item)
			}
		}
		if len(combined) == 0 {
			return nil, true, ms, nil
		}
		return combined, false, ms, nil
	}
}
