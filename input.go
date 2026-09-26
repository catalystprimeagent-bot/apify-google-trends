package main

import (
	"encoding/json"
	"strings"
)

// timeRangeParams maps the input schema's enum values to Google Trends' own "time" request
// parameter values. Verified recipe: runtime/reports/trends-feasibility.md used "today 12-m"
// as its default and it worked; the rest follow the same documented convention.
var timeRangeParams = map[string]string{
	"pastHour":     "now 1-H",
	"past4Hours":   "now 4-H",
	"pastDay":      "now 1-d",
	"past7Days":    "now 7-d",
	"past30Days":   "today 1-m",
	"past90Days":   "today 3-m",
	"past12Months": "today 12-m",
	"past5Years":   "today 5-y",
}

const defaultTimeRange = "past12Months"

// termsFromInput gathers every search term the buyer supplied across the primary field and its
// four aliases, normalizes and dedupes (case-insensitive) while preserving first-seen casing
// and order.
func termsFromInput(in Input) (terms []string, rejected []string) {
	seen := make(map[string]bool)

	add := func(raw string) {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			rejected = append(rejected, raw)
			return
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			return
		}
		seen[key] = true
		terms = append(terms, trimmed)
	}

	for _, field := range [][]string{
		coerceToList(in.SearchTerms),
		coerceToList(in.Terms),
		coerceToList(in.Keywords),
		coerceToList(in.Queries),
		coerceToList(in.Term),
	} {
		for _, raw := range field {
			add(raw)
		}
	}

	return terms, rejected
}

// coerceToList accepts a JSON array of strings, a JSON array of {"query"/"term": "..."}
// objects, a single string, or a comma/newline-separated string, for every alias field.
func coerceToList(v interface{}) []string {
	switch val := v.(type) {
	case nil:
		return nil
	case string:
		return splitStringList(val)
	case []interface{}:
		var out []string
		for _, item := range val {
			switch it := item.(type) {
			case string:
				out = append(out, it)
			case map[string]interface{}:
				if q, ok := it["query"].(string); ok {
					out = append(out, q)
				} else if t, ok := it["term"].(string); ok {
					out = append(out, t)
				}
			}
		}
		return out
	case []string:
		return val
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return nil
		}
		var arr []interface{}
		if err := json.Unmarshal(b, &arr); err == nil {
			return coerceToList(arr)
		}
		var s string
		if err := json.Unmarshal(b, &s); err == nil {
			return splitStringList(s)
		}
		return nil
	}
}

func splitStringList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	var out []string
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// resolveTimeRange turns the input's timeRange enum value into Google's "time" param.
// An unrecognised value falls back to the default rather than failing the run.
func resolveTimeRange(in *string) (enumValue, param string) {
	if in == nil || strings.TrimSpace(*in) == "" {
		return defaultTimeRange, timeRangeParams[defaultTimeRange]
	}
	if param, ok := timeRangeParams[*in]; ok {
		return *in, param
	}
	return defaultTimeRange, timeRangeParams[defaultTimeRange]
}

func resolveCategory(in *int) int {
	if in == nil {
		return 0
	}
	return *in
}

// resolveOutputs validates the buyer's requested output types against the known set and
// returns them in the canonical order, defaulting to all three when none (or none valid)
// were supplied.
func resolveOutputs(in []string) []string {
	if len(in) == 0 {
		return allOutputs
	}
	requested := make(map[string]bool, len(in))
	for _, o := range in {
		requested[o] = true
	}
	var out []string
	for _, o := range allOutputs {
		if requested[o] {
			out = append(out, o)
		}
	}
	if len(out) == 0 {
		return allOutputs
	}
	return out
}

func resolveTimeoutSeconds(in *int) int {
	const def, min, max = 20, 5, 60
	if in == nil {
		return def
	}
	v := *in
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
