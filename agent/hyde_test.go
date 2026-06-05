package main

import "testing"

func TestParseHydeParam(t *testing.T) {
	forced, auto := parseHydeParam("true")
	if !forced || auto {
		t.Fatal("expected forced hyde")
	}
	forced, auto = parseHydeParam("auto")
	if forced || !auto {
		t.Fatal("expected auto hyde")
	}
}

func TestShouldApplyHyde(t *testing.T) {
	if !shouldApplyHyde(true, false, 0.9) {
		t.Fatal("forced hyde should apply")
	}
	if !shouldApplyHyde(false, true, 0.5) {
		t.Fatal("auto hyde should apply below threshold")
	}
	if shouldApplyHyde(false, true, 0.9) {
		t.Fatal("auto hyde should not apply above threshold")
	}
}
