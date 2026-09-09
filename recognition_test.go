package main

import "testing"

func TestDesktopMigrationAndBrowserRules(t *testing.T) {
	cfg := Config{Keywords: []string{"chatgpt"}, Processes: []string{"chrome.exe"}}
	cfg.Normalize()
	svc := NewBiliBreakService(nil)
	cases := []struct {
		title, process string
		want           bool
	}{
		{"Conversation", "chatgpt.exe", true},
		{"ChatGPT - Google Chrome", "chrome.exe", true},
		{"Unrelated video", "chrome.exe", false},
		{"ChatGPT notes", "notepad.exe", false},
	}
	for _, tt := range cases {
		got, _ := svc.matchesDebug(cfg, tt.title, tt.process)
		if got != tt.want {
			t.Errorf("%s: got %v want %v", tt.process+tt.title, got, tt.want)
		}
	}
	cfg.Processes = []string{"chrome.exe"}
	cfg.Normalize()
	if got, _ := svc.matchesDebug(cfg, "ChatGPT", "chatgpt.exe"); got {
		t.Fatal("migration restored a rule explicitly removed after upgrade")
	}
	cfg.Processes = nil
	if got, _ := svc.matchesDebug(cfg, "ChatGPT", "anything.exe"); !got {
		t.Fatal("empty list must honor title keywords")
	}
	if got, _ := svc.matchesDebug(cfg, "Unrelated", "anything.exe"); got {
		t.Fatal("empty list must not count every app")
	}
}

func TestFocusedTargetCapture(t *testing.T) {
	cases := []struct {
		info             ActiveWindowInfo
		process, keyword string
		fail             bool
	}{
		{ActiveWindowInfo{OK: true, PID: 2, Process: "ChatGPT.exe", Title: "Conversation"}, "chatgpt.exe", "", false},
		{ActiveWindowInfo{OK: true, PID: 2, Process: "chrome.exe", Title: "A conversation - ChatGPT - Google Chrome"}, "chrome.exe", "chatgpt", false},
		{ActiveWindowInfo{OK: true, PID: 2, Process: "sunbrowser.exe", Title: "My course - SunBrowser"}, "sunbrowser.exe", "My course", false},
		{ActiveWindowInfo{OK: true, PID: 1, Process: "reminder.exe"}, "", "", true},
		{ActiveWindowInfo{OK: true, PID: 2}, "", "", true},
		{ActiveWindowInfo{OK: true, PID: 2, Process: "chrome.exe"}, "", "", true},
	}
	for _, tt := range cases {
		got, err := focusTargetForWindow(tt.info, 1)
		if (err != nil) != tt.fail {
			t.Fatalf("%+v: %v", tt.info, err)
		}
		if err == nil && (got.Process != tt.process || got.Keyword != tt.keyword) {
			t.Errorf("got %+v", got)
		}
	}
}
