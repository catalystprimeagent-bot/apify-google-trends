package main

// Input mirrors the incumbent's field names so a buyer can swap tools without editing a
// saved input. See README.md "Compatibility" section.
type Input struct {
	SearchTerms    interface{} `json:"searchTerms"`
	Terms          interface{} `json:"terms"`
	Keywords       interface{} `json:"keywords"`
	Queries        interface{} `json:"queries"`
	Term           interface{} `json:"term"`
	TimeRange      *string     `json:"timeRange"`
	Category       *int        `json:"category"`
	Outputs        []string    `json:"outputs"`
	TimeoutSeconds *int        `json:"timeoutSeconds"`
}

// ResultRow is one dataset item: one row per (search term x output type).
type ResultRow struct {
	Term       string      `json:"term"`
	OutputType string      `json:"outputType"`
	TimeRange  string      `json:"timeRange"`
	Category   int         `json:"category"`
	Status     string      `json:"status"`
	FetchMs    int64       `json:"fetchMs"`
	Charged    bool        `json:"charged"`
	Data       interface{} `json:"data,omitempty"`
	Error      string      `json:"error,omitempty"`
}

const (
	statusOK     = "ok"
	statusNoData = "no_data"
	statusError  = "error"
)

const (
	outputInterestOverTime = "interestOverTime"
	outputRelatedQueries   = "relatedQueries"
)

// allOutputs is the canonical order dataset rows are written in, regardless of the order the
// buyer listed them in the "outputs" input field.
//
// relatedTopics was removed 2026-09-26 (CAT-48): Google's RELATED_TOPICS widget returned a
// genuinely empty `{"default":{"rankedList":[]}}` — confirmed via raw response body, not a
// parser bug — for every term tested (coffee, nike, bitcoin, tesla, chess set; worldwide and
// geo=US). See memory/030.
var allOutputs = []string{outputInterestOverTime, outputRelatedQueries}
