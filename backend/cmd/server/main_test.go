package main

import "testing"

func TestRun(t *testing.T) {
	code := run()
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}
