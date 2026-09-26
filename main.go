package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	pushBatchSize = 10
	// termConcurrency bounds how many terms are processed at once. *trendsClient's http.Client
	// and cookie jar are safe for concurrent use. Gate 1 (runtime/reports/trends-feasibility.md)
	// found 90 sequential requests succeed from a cold start in 3.5 seconds, so a handful of
	// terms processed concurrently (a few times that request rate, briefly) is well inside what
	// has already been shown safe; it is the many-runs-in-a-row pattern that isn't, not a wider
	// fan-out within one run.
	termConcurrency = 5
)

// trendsAPI is the subset of *trendsClient that processTerm depends on, extracted so tests can
// exercise the row-construction logic (explore failure, missing widget, no-data, ok) without
// making real requests to Google.
type trendsAPI interface {
	doExplore(term, timeParam string, category int) ([]exploreWidget, int64, error)
	fetchWidgetData(w exploreWidget) ([]map[string]interface{}, bool, int64, error)
}

func main() {
	if err := run(); err != nil {
		log.Printf("FATAL: %v", err)
		os.Exit(1)
	}
}

func run() error {
	env := loadEnv()
	httpClient := &http.Client{}

	input, err := getInput(env, httpClient)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	terms, rejected := termsFromInput(input)
	for _, r := range rejected {
		log.Printf("skipping empty search term entry: %q", r)
	}
	if len(terms) == 0 {
		return fmt.Errorf("no valid search terms in input — provide at least one term via " +
			"\"searchTerms\" (or its aliases terms, keywords, queries, term)")
	}
	log.Printf("processing %d search term(s)", len(terms))

	timeRangeEnum, timeParam := resolveTimeRange(input.TimeRange)
	category := resolveCategory(input.Category)
	outputs := resolveOutputs(input.Outputs)
	timeoutSeconds := resolveTimeoutSeconds(input.TimeoutSeconds)

	trends, err := newTrendsClient(time.Duration(timeoutSeconds) * time.Second)
	if err != nil {
		return fmt.Errorf("setting up Trends client: %w", err)
	}
	if err := trends.warmUp(); err != nil {
		return fmt.Errorf("warm-up request to trends.google.com failed: %w", err)
	}

	pricing := newPricingManager(env, httpClient, primaryChargeEvent)
	writer := newResultWriter(env, httpClient)

	perTerm := processAllTerms(trends, pricing, terms, timeRangeEnum, timeParam, category, outputs, termConcurrency)

	var allRows []ResultRow
	var pending []ResultRow
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		if err := writer.Push(pending); err != nil {
			return err
		}
		pending = nil
		return nil
	}
	for _, rows := range perTerm {
		allRows = append(allRows, rows...)
		pending = append(pending, rows...)
		if len(pending) >= pushBatchSize {
			if err := flush(); err != nil {
				return fmt.Errorf("pushing results to dataset: %w", err)
			}
		}
	}
	if err := flush(); err != nil {
		return fmt.Errorf("pushing results to dataset: %w", err)
	}

	log.Printf("done: %d term(s) processed, %d dataset row(s) written, %d charged", len(terms), len(allRows), chargedCount(allRows))
	return nil
}

// processAllTerms runs processTerm over every term with bounded concurrency, preserving input
// order in the returned slice (one []ResultRow per term).
func processAllTerms(trends trendsAPI, pricing *pricingManager, terms []string, timeRangeEnum, timeParam string, category int, outputs []string, workers int) [][]ResultRow {
	results := make([][]ResultRow, len(terms))
	if workers > len(terms) {
		workers = len(terms)
	}
	if workers < 1 {
		workers = 1
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = processTerm(trends, pricing, terms[i], timeRangeEnum, timeParam, category, outputs)
			}
		}()
	}
	for i := range terms {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
}

// chargedCount counts how many rows were actually billed.
func chargedCount(rows []ResultRow) int {
	n := 0
	for _, r := range rows {
		if r.Charged {
			n++
		}
	}
	return n
}

// processTerm runs one term through explore, then one widget follow-up call per requested
// output type, and returns one ResultRow per output type.
func processTerm(trends trendsAPI, pricing *pricingManager, term, timeRangeEnum, timeParam string, category int, outputs []string) []ResultRow {
	rows := make([]ResultRow, 0, len(outputs))

	widgets, exploreMs, err := trends.doExplore(term, timeParam, category)
	if err != nil {
		log.Printf("term %q: explore failed: %v", term, err)
		for _, output := range outputs {
			rows = append(rows, ResultRow{
				Term: term, OutputType: output, TimeRange: timeRangeEnum, Category: category,
				Status: statusError, FetchMs: exploreMs, Charged: false,
				Error: fmt.Sprintf("could not reach Google Trends for this term: %v", err),
			})
		}
		return rows
	}

	byID := make(map[string]exploreWidget, len(widgets))
	for _, w := range widgets {
		byID[w.ID] = w
	}

	for _, output := range outputs {
		row := ResultRow{Term: term, OutputType: output, TimeRange: timeRangeEnum, Category: category}
		id := widgetIDForOutput(output)
		w, ok := byID[id]
		if !ok {
			row.Status = statusError
			row.Error = fmt.Sprintf("Google Trends did not return the %q widget for this query", id)
			rows = append(rows, row)
			continue
		}

		data, noData, ms, err := trends.fetchWidgetData(w)
		row.FetchMs = ms
		if err != nil {
			row.Status = statusError
			row.Error = fmt.Sprintf("fetching %s data failed: %v", output, err)
			rows = append(rows, row)
			continue
		}
		if noData {
			// Not charged (CAT-48, memory/030): a buyer should never pay for a row that
			// cannot contain data. Billing stays tied to rows that actually deliver data.
			row.Status = statusNoData
			row.Error = "no data available from Google Trends for this term, time range and category"
			rows = append(rows, row)
			continue
		}
		row.Status = statusOK
		row.Data = data
		row.Charged = pricing.Charge(term + ":" + output)
		rows = append(rows, row)
	}
	return rows
}
