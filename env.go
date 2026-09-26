package main

import (
	"os"
	"strconv"
)

// actorEnv is a snapshot of the Actor-specification and Apify-platform environment variables
// this actor needs. Read once at startup. See:
// https://docs.apify.com/platform/actors/development/programming-interface/environment-variables
type actorEnv struct {
	isAtHome          bool
	apiBase           string
	token             string
	actorID           string
	runID             string
	datasetID         string
	kvStoreID         string
	inputKey          string
	localStorageDir   string
	maxTotalChargeUSD *float64
}

func loadEnv() actorEnv {
	e := actorEnv{
		isAtHome:        os.Getenv("APIFY_IS_AT_HOME") != "",
		apiBase:         getenvDefault("APIFY_API_PUBLIC_BASE_URL", "https://api.apify.com"),
		token:           os.Getenv("APIFY_TOKEN"),
		actorID:         os.Getenv("ACTOR_ID"),
		runID:           os.Getenv("ACTOR_RUN_ID"),
		datasetID:       os.Getenv("ACTOR_DEFAULT_DATASET_ID"),
		kvStoreID:       os.Getenv("ACTOR_DEFAULT_KEY_VALUE_STORE_ID"),
		inputKey:        getenvDefault("ACTOR_INPUT_KEY", "INPUT"),
		localStorageDir: getenvDefault("APIFY_LOCAL_STORAGE_DIR", "./storage"),
	}
	if raw := os.Getenv("ACTOR_MAX_TOTAL_CHARGE_USD"); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			e.maxTotalChargeUSD = &v
		}
	}
	return e
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
