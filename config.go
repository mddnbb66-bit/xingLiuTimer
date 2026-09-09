package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	AppName                         = "BiliBreakReminder"
	ConfigFolder                    = "BiliBreakReminder"
	ConfigFile                      = "config.json"
	StatsFile                       = "stats.json"
	InitialCumulativeWatchedSeconds = 96*3600 + 25*60 // 96h 25min
)

// Config contains user settings persisted on disk.
type Config struct {
	RecognitionVersion int `json:"recognitionVersion"`
	// IntervalMinutes: reminder interval in minutes. v3 allows going smaller than 30.
	IntervalMinutes int `json:"intervalMinutes"`

	// MonitorEnabled indicates whether the monitor loop counts time.
	MonitorEnabled bool `json:"monitorEnabled"`

	// Reminder channels
	NotifySystem bool `json:"notifySystem"` // native system toast via Wails notifications service
	NotifyPopup  bool `json:"notifyPopup"`  // in-app modal
	NotifySound  bool `json:"notifySound"`  // webaudio beep

	// SnoozeMinutes: the default snooze duration used by the Snooze button.
	SnoozeMinutes int `json:"snoozeMinutes"`

	// AutoStart: Windows autostart via HKCU\\...\\Run
	AutoStart bool `json:"autoStart"`

	// Matching rules
	Keywords  []string `json:"keywords"`  // if window title contains any keyword -> considered Bilibili
	Processes []string `json:"processes"` // if process name is in this list -> considered browser; leave empty to allow any

	// Clock overlay
	ClockAlwaysOn      bool `json:"clockAlwaysOn"`      // keep overlay clock visible at full opacity
	ClockFadeAfterSecs int  `json:"clockFadeAfterSecs"` // fade clock after inactivity seconds
	ClockAlertMinutes  int  `json:"clockAlertMinutes"`  // when remaining time <= this value, clock enters alert style
	ClockAutoShowAlert bool `json:"clockAutoShowAlert"` // if not always-on, auto reveal clock during alert window
}

type PersistedStats struct {
	CumulativeWatchedSeconds int            `json:"cumulativeWatchedSeconds"`
	DailySeconds             map[string]int `json:"dailySeconds,omitempty"` // "2006-01-02" -> seconds
	ResetHistory             []ResetRecord  `json:"resetHistory,omitempty"`
}

// Each reset stores only the removed amount, so undo retains newly counted time.
type ResetRecord struct {
	Kind    string `json:"kind"`
	Day     string `json:"day"`
	Seconds int    `json:"seconds"`
}

func defaultPersistedStats() PersistedStats {
	return PersistedStats{
		CumulativeWatchedSeconds: InitialCumulativeWatchedSeconds,
	}
}

func defaultConfig() Config {
	return Config{
		RecognitionVersion: 1,
		IntervalMinutes:    30,
		MonitorEnabled:     true,
		NotifySystem:       true,
		NotifyPopup:        true,
		NotifySound:        false,
		SnoozeMinutes:      10,
		AutoStart:          false,
		Keywords:           []string{"bilibili", "哔哩哔哩", "b站"},
		Processes:          []string{"chrome.exe", "msedge.exe", "firefox.exe", "brave.exe", "opera.exe", "chatgpt.exe"},
		ClockAlwaysOn:      false,
		ClockFadeAfterSecs: 20,
		ClockAlertMinutes:  5,
		ClockAutoShowAlert: true,
	}
}

func (c *Config) Normalize() {
	if c.IntervalMinutes < 1 {
		c.IntervalMinutes = 1
	}
	if c.IntervalMinutes > 240 {
		c.IntervalMinutes = 240
	}
	if c.SnoozeMinutes < 0 {
		c.SnoozeMinutes = 0
	}
	if c.SnoozeMinutes > 240 {
		c.SnoozeMinutes = 240
	}
	if c.ClockFadeAfterSecs < 3 {
		c.ClockFadeAfterSecs = 3
	}
	if c.ClockFadeAfterSecs > 600 {
		c.ClockFadeAfterSecs = 600
	}
	if c.ClockAlertMinutes < 1 {
		c.ClockAlertMinutes = 1
	}
	if c.ClockAlertMinutes > 60 {
		c.ClockAlertMinutes = 60
	}
	// Migrate existing ChatGPT webpage users to desktop support once.
	if c.RecognitionVersion < 1 {
		for _, keyword := range normalizeStringSlice(c.Keywords) {
			if keyword == "chatgpt" || keyword == "gpt" {
				c.Processes = append(c.Processes, "chatgpt.exe")
				break
			}
		}
		c.RecognitionVersion = 1
	}
	// Normalize strings
	c.Keywords = normalizeStringSlice(c.Keywords)
	c.Processes = normalizeStringSlice(c.Processes)
}

func normalizeStringSlice(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, s := range strings.FieldsFunc(strings.Join(in, ","), func(r rune) bool { return r == ',' || r == '，' || r == '\n' || r == '\r' }) {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		sLower := strings.ToLower(s)
		if _, ok := seen[sLower]; ok {
			continue
		}
		seen[sLower] = struct{}{}
		out = append(out, sLower)
	}
	return out
}

func configPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, ConfigFolder, ConfigFile), nil
}

func statsPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, ConfigFolder, StatsFile), nil
}

func LoadPersistedStats() (PersistedStats, error) {
	p, err := statsPath()
	if err != nil {
		return PersistedStats{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultPersistedStats(), nil
		}
		return PersistedStats{}, err
	}
	var stats PersistedStats
	if err := json.Unmarshal(b, &stats); err != nil {
		return defaultPersistedStats(), nil
	}
	if stats.CumulativeWatchedSeconds < 0 {
		stats.CumulativeWatchedSeconds = 0
	}
	return stats, nil
}

func SavePersistedStats(stats PersistedStats) error {
	if stats.CumulativeWatchedSeconds < 0 {
		stats.CumulativeWatchedSeconds = 0
	}
	p, err := statsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func LoadConfig() (Config, error) {
	p, err := configPath()
	if err != nil {
		return Config{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := defaultConfig()
			cfg.Normalize()
			return cfg, nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		// If file is corrupt, return defaults instead of crashing.
		cfg = defaultConfig()
		cfg.Normalize()
		return cfg, nil
	}
	cfg.Normalize()
	return cfg, nil
}

func SaveConfig(cfg Config) error {
	cfg.Normalize()
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}
