package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func isolateTimerFiles(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("APPDATA", base)
	t.Setenv("XDG_CONFIG_HOME", base)
}

func TestIntervalImmediatelyRestartsAndPersists(t *testing.T) {
	isolateTimerFiles(t)
	svc := NewBiliBreakService(nil)
	svc.cfg.IntervalMinutes = 45
	svc.stats.SinceLastBreakSeconds = 600
	svc.stats.TotalWatchedSeconds = 600
	svc.stats.CumulativeWatchedSeconds = 10000
	svc.snoozedUntil = time.Now().Add(time.Hour)
	stats, err := svc.SetReminderInterval(1)
	if err != nil {
		t.Fatal(err)
	}
	if stats.NextBreakInSeconds != 60 || stats.SinceLastBreakSeconds != 0 {
		t.Fatalf("countdown not applied: %+v", stats)
	}
	if stats.TotalWatchedSeconds != 600 || stats.CumulativeWatchedSeconds != 10000 {
		t.Fatal("interval changed totals")
	}
	if !svc.snoozedUntil.IsZero() {
		t.Fatal("old snooze kept blocking new interval")
	}
	cfg, err := LoadConfig()
	if err != nil || cfg.IntervalMinutes != 1 {
		t.Fatalf("config: %+v %v", cfg, err)
	}
	svc.cfg.Processes = []string{"chatgpt.exe"}
	svc.cfg.NotifySystem = false
	svc.cfg.NotifyPopup = false
	svc.cfg.NotifySound = false
	now := time.Now()
	for i := 0; i < 59; i++ {
		svc.tickWindow(now, ActiveWindowInfo{OK: true, Process: "chatgpt.exe"})
	}
	if svc.stats.NextBreakInSeconds != 1 {
		t.Fatalf("expected one second, got %d", svc.stats.NextBreakInSeconds)
	}
	svc.tickWindow(now, ActiveWindowInfo{OK: true, Process: "chatgpt.exe"})
	if svc.stats.NextBreakInSeconds != 60 {
		t.Fatal("new interval did not fire/reset")
	}
}

func TestIntervalSaveFailureDoesNotChangeRuntime(t *testing.T) {
	isolateTimerFiles(t)
	base := t.TempDir()
	blocked := filepath.Join(base, "not-a-directory")
	if err := os.WriteFile(blocked, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPDATA", blocked)
	t.Setenv("XDG_CONFIG_HOME", blocked)
	svc := NewBiliBreakService(nil)
	svc.stats.NextBreakInSeconds = 100
	if _, err := svc.SetReminderInterval(5); err == nil {
		t.Fatal("expected save failure")
	}
	if svc.cfg.IntervalMinutes != 30 || svc.stats.NextBreakInSeconds != 100 {
		t.Fatal("failed save changed runtime")
	}
	cfg := svc.cfg
	cfg.IntervalMinutes = 10
	if err := svc.SetConfig(cfg); err == nil {
		t.Fatal("expected full config save failure")
	}
	if svc.cfg.IntervalMinutes != 30 {
		t.Fatal("failed full save changed config")
	}
}

func TestResetUndoSurvivesRestartAndRetainsNewTime(t *testing.T) {
	isolateTimerFiles(t)
	now := time.Now()
	day := now.Format("2006-01-02")
	svc := NewBiliBreakService(nil)
	svc.stats.TotalWatchedSeconds = 600
	svc.stats.CumulativeWatchedSeconds = 10000
	svc.stats.SinceLastBreakSeconds = 120
	svc.stats.NextBreakInSeconds = 1680
	svc.persistedStats.DailySeconds = map[string]int{day: 600, "2020-01-01": 300}
	if err := svc.ResetToday(); err != nil {
		t.Fatal(err)
	}
	if svc.stats.TotalWatchedSeconds != 0 || svc.persistedStats.DailySeconds[day] != 0 {
		t.Fatal("today not cleared")
	}
	if svc.stats.SinceLastBreakSeconds != 120 || svc.stats.NextBreakInSeconds != 1680 {
		t.Fatal("reset changed countdown")
	}
	if err := svc.ResetCumulative(); err != nil {
		t.Fatal(err)
	}
	svc.stats.TotalWatchedSeconds = 7
	svc.stats.CumulativeWatchedSeconds = 7
	svc.persistedStats.DailySeconds[day] = 7
	svc.persistStats(true)
	loaded, err := LoadPersistedStats()
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewBiliBreakService(nil)
	restarted.persistedStats = loaded
	restarted.stats.CumulativeWatchedSeconds = loaded.CumulativeWatchedSeconds
	restarted.stats.TotalWatchedSeconds = loaded.DailySeconds[day]
	restarted.updateResetStatusLocked()
	if !restarted.stats.CanUndoReset {
		t.Fatal("restart lost undo")
	}
	if err := restarted.UndoReset(); err != nil {
		t.Fatal(err)
	}
	if restarted.stats.CumulativeWatchedSeconds != 10007 {
		t.Fatal("cumulative undo lost post-reset seconds")
	}
	if err := restarted.UndoReset(); err != nil {
		t.Fatal(err)
	}
	if restarted.stats.TotalWatchedSeconds != 607 || restarted.persistedStats.DailySeconds[day] != 607 {
		t.Fatal("today undo lost post-reset seconds")
	}
	if restarted.persistedStats.DailySeconds["2020-01-01"] != 300 {
		t.Fatal("past history changed")
	}
	if restarted.stats.CanUndoReset {
		t.Fatal("undo not consumed")
	}
	if err := restarted.UndoReset(); err == nil {
		t.Fatal("repeated undo duplicated time")
	}
}

func TestUndoTodayAfterMidnightRestoresOriginalDay(t *testing.T) {
	isolateTimerFiles(t)
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	today := now.Format("2006-01-02")
	svc := NewBiliBreakService(nil)
	svc.persistedStats.DailySeconds = map[string]int{today: 20, yesterday: 5}
	svc.persistedStats.ResetHistory = []ResetRecord{{Kind: "today", Day: yesterday, Seconds: 100}}
	if err := svc.undoResetAt(now); err != nil {
		t.Fatal(err)
	}
	if svc.stats.TotalWatchedSeconds != 20 || svc.persistedStats.DailySeconds[yesterday] != 105 {
		t.Fatal("undo moved previous-day seconds into today")
	}
}

func TestResetSaveFailurePreservesData(t *testing.T) {
	isolateTimerFiles(t)
	svc := NewBiliBreakService(nil)
	svc.stats.CumulativeWatchedSeconds = 100
	base := t.TempDir()
	blocked := filepath.Join(base, "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPDATA", blocked)
	t.Setenv("XDG_CONFIG_HOME", blocked)
	if err := svc.ResetCumulative(); err == nil {
		t.Fatal("expected save failure")
	}
	if svc.stats.CumulativeWatchedSeconds != 100 || len(svc.persistedStats.ResetHistory) != 0 {
		t.Fatal("failed reset destroyed data")
	}
}
