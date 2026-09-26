package main

import (
	"os"
	"testing"
)

func TestLoadEnv_Defaults(t *testing.T) {
	for _, k := range []string{"APIFY_IS_AT_HOME", "APIFY_API_PUBLIC_BASE_URL", "ACTOR_INPUT_KEY", "APIFY_LOCAL_STORAGE_DIR", "ACTOR_MAX_TOTAL_CHARGE_USD"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	env := loadEnv()
	if env.isAtHome {
		t.Error("expected isAtHome false when APIFY_IS_AT_HOME unset")
	}
	if env.apiBase != "https://api.apify.com" {
		t.Errorf("got apiBase %q, want default", env.apiBase)
	}
	if env.inputKey != "INPUT" {
		t.Errorf("got inputKey %q, want INPUT", env.inputKey)
	}
	if env.localStorageDir != "./storage" {
		t.Errorf("got localStorageDir %q, want ./storage", env.localStorageDir)
	}
	if env.maxTotalChargeUSD != nil {
		t.Errorf("expected nil maxTotalChargeUSD, got %v", *env.maxTotalChargeUSD)
	}
}

func TestLoadEnv_Overrides(t *testing.T) {
	t.Setenv("APIFY_IS_AT_HOME", "1")
	t.Setenv("ACTOR_MAX_TOTAL_CHARGE_USD", "2.50")
	env := loadEnv()
	if !env.isAtHome {
		t.Error("expected isAtHome true")
	}
	if env.maxTotalChargeUSD == nil || *env.maxTotalChargeUSD != 2.50 {
		t.Errorf("got maxTotalChargeUSD %v, want 2.50", env.maxTotalChargeUSD)
	}
}

func TestGetenvDefault(t *testing.T) {
	t.Setenv("GT_TEST_VAR", "")
	os.Unsetenv("GT_TEST_VAR")
	if got := getenvDefault("GT_TEST_VAR", "fallback"); got != "fallback" {
		t.Errorf("got %q, want fallback", got)
	}
	t.Setenv("GT_TEST_VAR", "set")
	if got := getenvDefault("GT_TEST_VAR", "fallback"); got != "set" {
		t.Errorf("got %q, want set", got)
	}
}
