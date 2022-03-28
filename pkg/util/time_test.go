package util

import (
	"testing"
	"time"
)

func TestGetDuration(t *testing.T) {
	want := 2 * time.Second
	start := time.Now().UTC()
	end := start.Add(want)
	duration := GetDuration(start, end)
	if duration != want {
		t.Fatalf(`GetDuration(%v, %v) = %v, want %v`, start, end, duration, want)
	}
}
