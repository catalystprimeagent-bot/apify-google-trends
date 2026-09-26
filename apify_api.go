package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// apiError mirrors the shape of Apify's JSON error responses: {"error": {"type", "message"}}.
type apiError struct {
	StatusCode int
	Type       string
	Message    string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("apify api error %d (%s): %s", e.StatusCode, e.Type, e.Message)
}

// apifyRequest performs one Apify REST API call and decodes a JSON response into out (if non-nil).
// It returns *apiError for non-2xx responses so callers can branch on the machine-readable type.
func apifyRequest(client *http.Client, method, url, token string, headers map[string]string, body interface{}, out interface{}) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var parsed struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(respBody, &parsed)
		return &apiError{StatusCode: resp.StatusCode, Type: parsed.Error.Type, Message: parsed.Error.Message}
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decoding response from %s: %w", url, err)
		}
	}
	return nil
}
