package main

import (
	"net/http"
	"testing"
)

func TestPricingManager_DisabledWhenNotAtHome(t *testing.T) {
	env := actorEnv{isAtHome: false}
	pm := newPricingManager(env, &http.Client{}, primaryChargeEvent)
	if pm.enabled {
		t.Fatal("pricing manager should be disabled for local (non-platform) runs")
	}
	if charged := pm.Charge("coffee:interestOverTime"); charged {
		t.Error("Charge should return false when disabled")
	}
}
