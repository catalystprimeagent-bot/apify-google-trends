package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// getInput reads the Actor's input. On the platform it comes from the default key-value store
// over the REST API; locally (e.g. `apify run`, or a bare `go run` for development) it comes
// from the local storage emulation Apify's CLI sets up, matching the "Actors in any language"
// pattern from Apify's docs.
func getInput(env actorEnv, client *http.Client) (Input, error) {
	var raw []byte
	var err error

	if env.isAtHome {
		raw, err = getInputFromAPI(env, client)
	} else {
		raw, err = getInputFromDisk(env)
	}
	if err != nil {
		return Input{}, err
	}

	var in Input
	if len(raw) == 0 {
		return in, nil
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return Input{}, fmt.Errorf("input is not valid JSON: %w", err)
	}
	return in, nil
}

func getInputFromAPI(env actorEnv, client *http.Client) ([]byte, error) {
	url := fmt.Sprintf("%s/v2/key-value-stores/%s/records/%s", env.apiBase, env.kvStoreID, env.inputKey)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+env.token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // no input provided
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetching input: HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func getInputFromDisk(env actorEnv) ([]byte, error) {
	path := filepath.Join(env.localStorageDir, "key_value_stores", "default", env.inputKey+".json")
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return body, err
}

// resultWriter appends dataset items either to the platform's dataset API or to the local
// storage folder, mirroring Apify's own numbered-file convention.
type resultWriter struct {
	env       actorEnv
	client    *http.Client
	localNext int
}

func newResultWriter(env actorEnv, client *http.Client) *resultWriter {
	w := &resultWriter{env: env, client: client}
	if !env.isAtHome {
		w.localNext = countExistingLocalItems(env) + 1
	}
	return w
}

func countExistingLocalItems(env actorEnv) int {
	dir := filepath.Join(env.localStorageDir, "datasets", "default")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			count++
		}
	}
	return count
}

func (w *resultWriter) Push(results []ResultRow) error {
	if len(results) == 0 {
		return nil
	}
	if w.env.isAtHome {
		return w.pushAPI(results)
	}
	return w.pushLocal(results)
}

func (w *resultWriter) pushAPI(results []ResultRow) error {
	url := fmt.Sprintf("%s/v2/datasets/%s/items", w.env.apiBase, w.env.datasetID)
	return apifyRequest(w.client, http.MethodPost, url, w.env.token, nil, results, nil)
}

func (w *resultWriter) pushLocal(results []ResultRow) error {
	dir := filepath.Join(w.env.localStorageDir, "datasets", "default")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, r := range results {
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		name := fmt.Sprintf("%09d.json", w.localNext)
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return err
		}
		w.localNext++
	}
	return nil
}

// sortedKeys is a small helper used by tests to get deterministic map iteration order.
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
