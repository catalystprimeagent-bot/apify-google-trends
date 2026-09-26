package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeTrendsAPI lets processTerm be tested without making real requests to Google.
type fakeTrendsAPI struct {
	exploreWidgets []exploreWidget
	exploreErr     error
	widgetData     map[string][]map[string]interface{} // keyed by widget ID
	widgetNoData   map[string]bool
	widgetErr      map[string]error
}

func (f *fakeTrendsAPI) doExplore(term, timeParam string, category int) ([]exploreWidget, int64, error) {
	if f.exploreErr != nil {
		return nil, 5, f.exploreErr
	}
	return f.exploreWidgets, 5, nil
}

func (f *fakeTrendsAPI) fetchWidgetData(w exploreWidget) ([]map[string]interface{}, bool, int64, error) {
	if err, ok := f.widgetErr[w.ID]; ok {
		return nil, false, 3, err
	}
	if f.widgetNoData[w.ID] {
		return nil, true, 3, nil
	}
	return f.widgetData[w.ID], false, 3, nil
}

func disabledPricing() *pricingManager {
	return newPricingManager(actorEnv{isAtHome: false}, &http.Client{}, primaryChargeEvent)
}

func TestProcessTerm_ExploreFailure_AllRowsError(t *testing.T) {
	fake := &fakeTrendsAPI{exploreErr: errors.New("boom")}
	rows := processTerm(fake, disabledPricing(), "coffee", "past12Months", "today 12-m", 0, allOutputs)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	for _, r := range rows {
		if r.Status != statusError || r.Error == "" || r.Charged {
			t.Errorf("expected error row with no charge, got %+v", r)
		}
	}
}

func TestProcessTerm_MissingWidgetInExploreResponse(t *testing.T) {
	fake := &fakeTrendsAPI{exploreWidgets: []exploreWidget{{ID: "TIMESERIES"}}}
	rows := processTerm(fake, disabledPricing(), "coffee", "past12Months", "today 12-m", 0, []string{outputRelatedQueries})
	if len(rows) != 1 || rows[0].Status != statusError {
		t.Fatalf("expected 1 error row for missing widget, got %+v", rows)
	}
}

func TestProcessTerm_NoData(t *testing.T) {
	fake := &fakeTrendsAPI{
		exploreWidgets: []exploreWidget{{ID: "RELATED_QUERIES"}},
		widgetNoData:   map[string]bool{"RELATED_QUERIES": true},
	}
	rows := processTerm(fake, disabledPricing(), "zzz obscure term", "past12Months", "today 12-m", 0, []string{outputRelatedQueries})
	if len(rows) != 1 || rows[0].Status != statusNoData || rows[0].Error == "" {
		t.Fatalf("expected clean no_data row, got %+v", rows)
	}
	if rows[0].Charged {
		t.Error("a no_data row must never be charged (CAT-48, memory/030)")
	}
}

// TestProcessTerm_OnlyOKRowsAreCharged is the direct test for the CAT-48 billing decision
// (memory/030): a buyer pays only for rows that actually deliver data. Uses a real, enabled
// pricingManager (not disabledPricing()) against a fake charge endpoint so the charge path
// itself — not just the row's Status field — is exercised.
func TestProcessTerm_OnlyOKRowsAreCharged(t *testing.T) {
	var chargeCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chargeCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pm := &pricingManager{
		env:           actorEnv{apiBase: srv.URL, runID: "run1", token: "tok"},
		client:        &http.Client{},
		eventName:     primaryChargeEvent,
		eventPriceUSD: 0.01,
		enabled:       true,
	}

	fake := &fakeTrendsAPI{
		exploreWidgets: []exploreWidget{{ID: "TIMESERIES"}, {ID: "RELATED_QUERIES"}},
		widgetData: map[string][]map[string]interface{}{
			"TIMESERIES": {{"time": "1"}},
		},
		widgetNoData: map[string]bool{"RELATED_QUERIES": true},
	}
	rows := processTerm(fake, pm, "coffee", "past12Months", "today 12-m", 0, allOutputs)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	for _, r := range rows {
		switch r.Status {
		case statusOK:
			if !r.Charged {
				t.Errorf("expected OK row to be charged: %+v", r)
			}
		case statusNoData:
			if r.Charged {
				t.Errorf("expected no_data row NOT to be charged: %+v", r)
			}
		default:
			t.Errorf("unexpected status: %+v", r)
		}
	}
	if chargeCalls != 1 {
		t.Errorf("expected exactly 1 charge call (for the OK row only), got %d", chargeCalls)
	}
}

func TestProcessTerm_OK(t *testing.T) {
	fake := &fakeTrendsAPI{
		exploreWidgets: []exploreWidget{{ID: "TIMESERIES"}},
		widgetData: map[string][]map[string]interface{}{
			"TIMESERIES": {{"time": "123", "value": []interface{}{42.0}}},
		},
	}
	rows := processTerm(fake, disabledPricing(), "coffee", "past12Months", "today 12-m", 0, []string{outputInterestOverTime})
	if len(rows) != 1 || rows[0].Status != statusOK || rows[0].Data == nil || rows[0].Error != "" {
		t.Fatalf("expected clean ok row with data, got %+v", rows)
	}
}

func TestProcessTerm_WidgetFetchError(t *testing.T) {
	fake := &fakeTrendsAPI{
		exploreWidgets: []exploreWidget{{ID: "RELATED_QUERIES"}},
		widgetErr:      map[string]error{"RELATED_QUERIES": errors.New("HTTP 429")},
	}
	rows := processTerm(fake, disabledPricing(), "coffee", "past12Months", "today 12-m", 0, []string{outputRelatedQueries})
	if len(rows) != 1 || rows[0].Status != statusError {
		t.Fatalf("expected error row, got %+v", rows)
	}
}

func TestProcessAllTerms_PreservesOrderUnderConcurrency(t *testing.T) {
	fake := &fakeTrendsAPI{
		exploreWidgets: []exploreWidget{{ID: "TIMESERIES"}},
		widgetData: map[string][]map[string]interface{}{
			"TIMESERIES": {{"time": "1"}},
		},
	}
	terms := []string{"a", "b", "c", "d", "e", "f", "g"}
	results := processAllTerms(fake, disabledPricing(), terms, "past12Months", "today 12-m", 0, []string{outputInterestOverTime}, 3)
	if len(results) != len(terms) {
		t.Fatalf("got %d result groups, want %d", len(results), len(terms))
	}
	for i, term := range terms {
		if len(results[i]) != 1 || results[i][0].Term != term {
			t.Errorf("index %d: expected term %q, got %+v", i, term, results[i])
		}
	}
}

func TestChargedCount(t *testing.T) {
	rows := []ResultRow{{Charged: true}, {Charged: false}, {Charged: true}}
	if got := chargedCount(rows); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}
