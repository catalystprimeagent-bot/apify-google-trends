package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestApifyRequest_SuccessDecodesJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" {
			t.Errorf("missing/wrong Authorization header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	var out struct {
		OK bool `json:"ok"`
	}
	err := apifyRequest(&http.Client{}, http.MethodGet, srv.URL, "tok123", nil, nil, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.OK {
		t.Error("expected decoded ok=true")
	}
}

func TestApifyRequest_NonJSONErrorReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate-limited","message":"slow down"}}`))
	}))
	defer srv.Close()

	err := apifyRequest(&http.Client{}, http.MethodGet, srv.URL, "tok", nil, nil, nil)
	if err == nil {
		t.Fatal("expected an error for HTTP 429")
	}
	apiErr, ok := err.(*apiError)
	if !ok {
		t.Fatalf("expected *apiError, got %T", err)
	}
	if apiErr.StatusCode != 429 || apiErr.Type != "rate-limited" {
		t.Errorf("got %+v", apiErr)
	}
}

func TestApifyRequest_SendsJSONBodyAndHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "abc" {
			t.Errorf("missing custom header")
		}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["count"] != float64(1) {
			t.Errorf("unexpected body: %v", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := apifyRequest(&http.Client{}, http.MethodPost, srv.URL, "tok", map[string]string{"Idempotency-Key": "abc"}, map[string]interface{}{"count": 1}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
