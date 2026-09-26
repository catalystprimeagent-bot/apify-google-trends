package main

import "testing"

func TestSentinelErrors_AreDistinctAndDescriptive(t *testing.T) {
	if errEmptyInput == nil || errEmptyInput.Error() == "" {
		t.Error("errEmptyInput should be a non-empty error")
	}
	if errNoTerms == nil || errNoTerms.Error() == "" {
		t.Error("errNoTerms should be a non-empty error")
	}
	if errEmptyInput.Error() == errNoTerms.Error() {
		t.Error("sentinel errors should have distinct messages")
	}
}
