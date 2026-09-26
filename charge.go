package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// primaryChargeEvent is the only chargeable event this actor defines. The event's price is set
// later, on the Apify Console, by whoever publishes the actor — not in this code. See README.md.
const primaryChargeEvent = "trends-lookup"

// pricingManager wires pay-per-event billing without hardcoding a price. It reads the actor's
// own current pricing info from the platform at startup and stops charging cleanly if the actor
// isn't monetized yet, or once the run's operator-set spending cap would be exceeded.
type pricingManager struct {
	mu              sync.Mutex
	env             actorEnv
	client          *http.Client
	eventName       string
	eventPriceUSD   float64
	enabled         bool
	disabledReason  string
	runningTotalUSD float64
}

func newPricingManager(env actorEnv, client *http.Client, eventName string) *pricingManager {
	pm := &pricingManager{env: env, client: client, eventName: eventName}

	if !env.isAtHome {
		pm.disabledReason = "not running on the Apify platform (local run)"
		return pm
	}

	price, ok, err := fetchActiveEventPrice(env, client, eventName)
	if err != nil {
		pm.disabledReason = fmt.Sprintf("could not read this actor's pricing info: %v", err)
		log.Printf("pay-per-event: %s", pm.disabledReason)
		return pm
	}
	if !ok {
		pm.disabledReason = fmt.Sprintf("actor is not yet published as pay-per-event with a price for %q — running unmetered", eventName)
		log.Printf("pay-per-event: %s", pm.disabledReason)
		return pm
	}

	pm.eventPriceUSD = price
	pm.enabled = true
	log.Printf("pay-per-event: charging $%.4f per %q event, max total $%v", price, eventName, env.maxTotalChargeUSD)
	return pm
}

// Charge charges one unit of the primary event, keyed by a caller-supplied idempotency
// fragment (typically term+outputType). Returns whether the charge actually happened; failures
// (including hitting the run's spending cap) disable further charging for the rest of the run
// but never fail the run itself.
func (pm *pricingManager) Charge(key string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !pm.enabled {
		return false
	}
	if pm.env.maxTotalChargeUSD != nil && pm.runningTotalUSD+pm.eventPriceUSD > *pm.env.maxTotalChargeUSD+1e-9 {
		pm.enabled = false
		pm.disabledReason = "reached the run's max total charge"
		log.Printf("pay-per-event: %s, disabling further charges", pm.disabledReason)
		return false
	}

	idempotencyKey := pm.env.runID + ":" + pm.eventName + ":" + key
	if err := postCharge(pm.env, pm.client, pm.eventName, 1, idempotencyKey); err != nil {
		pm.enabled = false
		pm.disabledReason = err.Error()
		log.Printf("pay-per-event: charge failed for %s (%v), disabling further charges this run", key, err)
		return false
	}
	pm.runningTotalUSD += pm.eventPriceUSD
	return true
}

func postCharge(env actorEnv, client *http.Client, eventName string, count int, idempotencyKey string) error {
	url := fmt.Sprintf("%s/v2/actor-runs/%s/charge", env.apiBase, env.runID)
	body := map[string]interface{}{"eventName": eventName, "count": count}
	headers := map[string]string{"Idempotency-Key": idempotencyKey}
	return apifyRequest(client, http.MethodPost, url, env.token, headers, body, nil)
}

// fetchActiveEventPrice reads this actor's own pricingInfos and returns the per-event price
// currently in effect for eventName. ok is false when the actor has no active pay-per-event
// pricing (e.g. it hasn't been published/monetized yet).
func fetchActiveEventPrice(env actorEnv, client *http.Client, eventName string) (price float64, ok bool, err error) {
	url := fmt.Sprintf("%s/v2/acts/%s", env.apiBase, env.actorID)

	var parsed struct {
		Data struct {
			PricingInfos []struct {
				PricingModel    string    `json:"pricingModel"`
				StartedAt       time.Time `json:"startedAt"`
				PricingPerEvent struct {
					ActorChargeEvents map[string]struct {
						EventPriceUsd float64 `json:"eventPriceUsd"`
					} `json:"actorChargeEvents"`
				} `json:"pricingPerEvent"`
			} `json:"pricingInfos"`
		} `json:"data"`
	}

	if err := apifyRequest(client, http.MethodGet, url, env.token, nil, nil, &parsed); err != nil {
		return 0, false, err
	}

	now := time.Now()
	var bestStart time.Time
	found := false
	for _, pi := range parsed.Data.PricingInfos {
		if pi.PricingModel != "PAY_PER_EVENT" {
			continue
		}
		if pi.StartedAt.After(now) {
			continue // scheduled for the future, not active yet
		}
		event, exists := pi.PricingPerEvent.ActorChargeEvents[eventName]
		if !exists {
			continue
		}
		if !found || pi.StartedAt.After(bestStart) {
			bestStart = pi.StartedAt
			price = event.EventPriceUsd
			found = true
		}
	}
	return price, found, nil
}
