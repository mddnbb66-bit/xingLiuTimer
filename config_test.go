package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPersistedStatsStartsAtInitialBaseline(t *testing.T) {
	stats := defaultPersistedStats()
	if stats.CumulativeWatchedSeconds != InitialCumulativeWatchedSeconds {
		t.Fatalf("expected default cumulative seconds %d, got %d", InitialCumulativeWatchedSeconds, stats.CumulativeWatchedSeconds)
	}
}

func TestLoadPersistedStatsMissingFileUsesDefaultBaseline(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("APPDATA", base)
	t.Setenv("APPDATA", base)
	stats, err := LoadPersistedStats()
	if err != nil {
		t.Fatalf("LoadPersistedStats returned error: %v", err)
	}
	if stats.CumulativeWatchedSeconds != InitialCumulativeWatchedSeconds {
		t.Fatalf("expected missing stats file to default to %d, got %d", InitialCumulativeWatchedSeconds, stats.CumulativeWatchedSeconds)
	}
}

func TestLoadPersistedStatsPreservesLargeDurations(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("APPDATA", base)
	statsDir := filepath.Join(base, ConfigFolder)
	if err := os.MkdirAll(statsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	data := []byte(`{"cumulativeWatchedSeconds": 172801}`)
	if err := os.WriteFile(filepath.Join(statsDir, StatsFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	stats, err := LoadPersistedStats()
	if err != nil {
		t.Fatalf("LoadPersistedStats returned error: %v", err)
	}
	if stats.CumulativeWatchedSeconds != 172801 {
		t.Fatalf("expected large cumulative seconds to survive load, got %d", stats.CumulativeWatchedSeconds)
	}
}
