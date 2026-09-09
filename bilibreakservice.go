package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// Stats is pushed to the frontend via events and is also queryable via GetStats().
type Stats struct {
	CanUndoReset             bool   `json:"canUndoReset"`
	UndoResetKind            string `json:"undoResetKind"`
	Running                  bool   `json:"running"`
	Watching                 bool   `json:"watching"`
	TotalWatchedSeconds      int    `json:"totalWatchedSeconds"`
	CumulativeWatchedSeconds int    `json:"cumulativeWatchedSeconds"`
	SinceLastBreakSeconds    int    `json:"sinceLastBreakSeconds"`
	NextBreakInSeconds       int    `json:"nextBreakInSeconds"`
	ActiveTitle              string `json:"activeTitle"`
	ActiveProcess            string `json:"activeProcess"`
	Day                      string `json:"day"`
	SnoozedUntil             string `json:"snoozedUntil,omitempty"` // RFC3339 if snoozed
	DailyAvgSeconds          int    `json:"dailyAvgSeconds"`        // average seconds per active day
	WeeklyAvgSeconds         int    `json:"weeklyAvgSeconds"`       // average seconds per day over last 7 days
}

type RemindPayload struct {
	Message   string `json:"message"`
	IntervalM int    `json:"intervalMinutes"`
	WatchedS  int    `json:"watchedSeconds"`
}

type BiliBreakService struct {
	mu                 sync.RWMutex
	cfg                Config
	stats              Stats
	day                string
	snoozedUntil       time.Time
	ctx                context.Context
	cancel             context.CancelFunc
	persistedStats     PersistedStats
	lastStatsPersistAt time.Time
	notifier           *notifications.NotificationService
	mainWindow         *application.WebviewWindow
}

func NewBiliBreakService(notifier *notifications.NotificationService) *BiliBreakService {
	cfg := defaultConfig()
	cfg.Normalize()
	return &BiliBreakService{
		cfg:      cfg,
		notifier: notifier,
		day:      time.Now().Format("2006-01-02"),
	}
}

func (s *BiliBreakService) SetMainWindow(win *application.WebviewWindow) {
	s.mainWindow = win
}

func (s *BiliBreakService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	cfg, err := LoadConfig()
	if err == nil {
		s.mu.Lock()
		s.cfg = cfg
		s.mu.Unlock()
	}
	_ = SetAutoStart(s.cfg.AutoStart)
	if persisted, err := LoadPersistedStats(); err == nil {
		s.mu.Lock()
		s.persistedStats = persisted
		s.stats.CumulativeWatchedSeconds = persisted.CumulativeWatchedSeconds
		s.stats.TotalWatchedSeconds = persisted.DailySeconds[s.day]
		s.updateResetStatusLocked()
		s.mu.Unlock()
	}
	go s.loop()
	s.emitConfig()
	s.emitStats()
	return nil
}

func (s *BiliBreakService) ServiceShutdown() error {
	s.persistStats(true)
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

func (s *BiliBreakService) setWindowTop(top bool) {
	win := s.mainWindow
	if win == nil {
		return
	}
	if top {
		if win.IsMinimised() {
			win.Restore()
		}
		win.Show()
		win.SetAlwaysOnTop(true)
		win.Focus()
	} else {
		win.SetAlwaysOnTop(false)
	}
}

// DailyPoint represents the study seconds for a single calendar day.
type DailyPoint struct {
	Date    string `json:"date"`
	Seconds int    `json:"seconds"`
}

// FocusTarget describes the foreground window captured after the switch delay.
type FocusTarget struct {
	Process string `json:"process"`
	Keyword string `json:"keyword"`
	Browser bool   `json:"browser"`
}

func isBrowserProcess(process string) bool {
	switch strings.ToLower(strings.TrimSpace(process)) {
	case "chrome.exe", "msedge.exe", "firefox.exe", "brave.exe", "opera.exe", "vivaldi.exe", "iexplore.exe", "browser.exe", "sunbrowser.exe", "arc.exe", "zen.exe", "librewolf.exe", "waterfox.exe":
		return true
	}
	return false
}

func containsTitleKeyword(title string, keywords []string) bool {
	for _, keyword := range normalizeStringSlice(keywords) {
		if strings.Contains(strings.ToLower(title), keyword) {
			return true
		}
	}
	return false
}

func focusTargetForWindow(info ActiveWindowInfo, ownPID uint32) (FocusTarget, error) {
	if !info.OK || strings.TrimSpace(info.Process) == "" {
		return FocusTarget{}, fmt.Errorf("无法读取当前窗口的进程，请切换到目标软件后重试")
	}
	if info.PID == ownPID {
		return FocusTarget{}, fmt.Errorf("仍停留在提醒软件，请在 5 秒内切换到目标软件或网页")
	}
	target := FocusTarget{Process: strings.ToLower(strings.TrimSpace(info.Process)), Browser: isBrowserProcess(info.Process)}
	if target.Browser {
		target.Keyword = strings.TrimSpace(info.Title)
		// Remove browser branding while retaining the page's distinguishing title.
		for _, suffix := range []string{" - Google Chrome", " - Microsoft Edge", " — Mozilla Firefox", " - Brave", " - Opera", " - Vivaldi", " - SunBrowser", " - Arc", " — Zen Browser"} {
			if strings.HasSuffix(strings.ToLower(target.Keyword), strings.ToLower(suffix)) {
				target.Keyword = strings.TrimSpace(target.Keyword[:len(target.Keyword)-len(suffix)])
				break
			}
		}
		if target.Keyword == "" {
			return FocusTarget{}, fmt.Errorf("网页标题为空，请打开目标网页后重试")
		}
		// Known services keep matching when their conversation/document title changes.
		for _, keyword := range []string{"chatgpt", "bilibili", "哔哩哔哩", "claude", "gemini", "deepseek"} {
			if strings.Contains(strings.ToLower(target.Keyword), keyword) {
				target.Keyword = keyword
				break
			}
		}
		parts := strings.FieldsFunc(target.Keyword, func(r rune) bool { return r == ',' || r == '，' || r == '\n' || r == '\r' })
		target.Keyword = ""
		for _, part := range parts {
			if len(strings.TrimSpace(part)) > len(target.Keyword) {
				target.Keyword = strings.TrimSpace(part)
			}
		}
		if target.Keyword == "" {
			return FocusTarget{}, fmt.Errorf("网页标题不含可用关键词，请换一个网页后重试")
		}
	}
	return target, nil
}

// CaptureFocusedTarget runs in Go so switching away cannot throttle the delay.
func (s *BiliBreakService) CaptureFocusedTarget() (FocusTarget, error) {
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return FocusTarget{}, ctx.Err()
	case <-timer.C:
	}
	return focusTargetForWindow(GetActiveWindowInfo(), uint32(os.Getpid()))
}

// ===== Public methods =====

func (s *BiliBreakService) GetConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *BiliBreakService) SetConfig(cfg Config) error {
	cfg.Normalize()
	s.mu.Lock()
	if err := SaveConfig(cfg); err != nil {
		s.mu.Unlock()
		return err
	}
	s.applyConfigLocked(cfg)
	s.mu.Unlock()
	_ = SetAutoStart(cfg.AutoStart)
	s.emitConfig()
	s.emitStats()
	return nil
}

func (s *BiliBreakService) applyConfigLocked(cfg Config) {
	if cfg.IntervalMinutes != s.cfg.IntervalMinutes {
		s.stats.SinceLastBreakSeconds = 0
		s.snoozedUntil = time.Time{}
		s.stats.SnoozedUntil = ""
	}
	s.cfg = cfg
	s.stats.NextBreakInSeconds = max(0, cfg.IntervalMinutes*60-s.stats.SinceLastBreakSeconds)
}

// Save only the interval; unrelated unsaved frontend settings remain untouched.
func (s *BiliBreakService) SetReminderInterval(minutes int) (Stats, error) {
	s.mu.Lock()
	cfg := s.cfg
	cfg.IntervalMinutes = minutes
	cfg.Normalize()
	if err := SaveConfig(cfg); err != nil {
		s.mu.Unlock()
		return Stats{}, err
	}
	s.applyConfigLocked(cfg)
	stats := s.stats
	s.mu.Unlock()
	s.emitStats()
	return stats, nil
}

func (s *BiliBreakService) GetStats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// GetDailyHistory returns the study seconds for the last `days` calendar days (7–30),
// ordered from oldest to newest. Days with no tracked activity have Seconds = 0.
func (s *BiliBreakService) GetDailyHistory(days int) []DailyPoint {
	if days < 7 {
		days = 7
	}
	if days > 30 {
		days = 30
	}
	now := time.Now()
	s.mu.RLock()
	defer s.mu.RUnlock()
	dailySeconds := s.persistedStats.DailySeconds
	result := make([]DailyPoint, days)
	for i := 0; i < days; i++ {
		day := now.AddDate(0, 0, -(days - 1 - i)).Format("2006-01-02")
		secs := 0
		if dailySeconds != nil {
			secs = dailySeconds[day]
		}
		result[i] = DailyPoint{Date: day, Seconds: secs}
	}
	return result
}

func (s *BiliBreakService) Start() error {
	s.mu.Lock()
	s.cfg.MonitorEnabled = true
	cfg := s.cfg
	s.mu.Unlock()
	s.setWindowTop(false)
	if err := SaveConfig(cfg); err != nil {
		return err
	}
	s.emitConfig()
	s.emitStats()
	return nil
}

func (s *BiliBreakService) Stop() error {
	s.mu.Lock()
	s.cfg.MonitorEnabled = false
	cfg := s.cfg
	s.mu.Unlock()
	s.setWindowTop(false)
	if err := SaveConfig(cfg); err != nil {
		return err
	}
	s.emitConfig()
	s.emitStats()
	return nil
}

func (s *BiliBreakService) ResetToday() error {
	return s.resetTime("today", time.Now())
}

func (s *BiliBreakService) ResetCumulative() error {
	return s.resetTime("cumulative", time.Now())
}

func clonePersistedStats(data PersistedStats) PersistedStats {
	copy := data
	copy.DailySeconds = make(map[string]int, len(data.DailySeconds))
	for day, seconds := range data.DailySeconds {
		copy.DailySeconds[day] = seconds
	}
	copy.ResetHistory = append([]ResetRecord(nil), data.ResetHistory...)
	return copy
}

func (s *BiliBreakService) updateResetStatusLocked() {
	s.stats.CanUndoReset = len(s.persistedStats.ResetHistory) > 0
	s.stats.UndoResetKind = ""
	if s.stats.CanUndoReset {
		s.stats.UndoResetKind = s.persistedStats.ResetHistory[len(s.persistedStats.ResetHistory)-1].Kind
	}
}

func (s *BiliBreakService) resetTime(kind string, now time.Time) error {
	s.mu.Lock()
	data := clonePersistedStats(s.persistedStats)
	data.CumulativeWatchedSeconds = s.stats.CumulativeWatchedSeconds
	day := now.Format("2006-01-02")
	record := ResetRecord{Kind: kind, Day: day}
	if kind == "today" {
		record.Seconds = data.DailySeconds[day]
		data.DailySeconds[day] = 0
	} else {
		record.Seconds = data.CumulativeWatchedSeconds
		data.CumulativeWatchedSeconds = 0
	}
	if record.Seconds == 0 {
		s.mu.Unlock()
		return fmt.Errorf("这项累计时间已为零，无需清空")
	}
	data.ResetHistory = append(data.ResetHistory, record)
	if err := SavePersistedStats(data); err != nil {
		s.mu.Unlock()
		return err
	}
	s.persistedStats = data
	s.stats.CumulativeWatchedSeconds = data.CumulativeWatchedSeconds
	s.stats.TotalWatchedSeconds = data.DailySeconds[day]
	s.stats.DailyAvgSeconds = computeDailyAvg(data.DailySeconds)
	s.stats.WeeklyAvgSeconds = computeWeeklyAvg(data.DailySeconds, now)
	s.updateResetStatusLocked()
	s.mu.Unlock()
	s.emitStats()
	return nil
}

func (s *BiliBreakService) UndoReset() error {
	return s.undoResetAt(time.Now())
}

func (s *BiliBreakService) undoResetAt(now time.Time) error {
	s.mu.Lock()
	data := clonePersistedStats(s.persistedStats)
	data.CumulativeWatchedSeconds = s.stats.CumulativeWatchedSeconds
	if len(data.ResetHistory) == 0 {
		s.mu.Unlock()
		return fmt.Errorf("没有可撤销的清空操作")
	}
	record := data.ResetHistory[len(data.ResetHistory)-1]
	data.ResetHistory = data.ResetHistory[:len(data.ResetHistory)-1]
	if record.Kind == "today" {
		data.DailySeconds[record.Day] += record.Seconds
	} else {
		data.CumulativeWatchedSeconds += record.Seconds
	}
	if err := SavePersistedStats(data); err != nil {
		s.mu.Unlock()
		return err
	}
	s.persistedStats = data
	s.stats.CumulativeWatchedSeconds = data.CumulativeWatchedSeconds
	s.stats.TotalWatchedSeconds = data.DailySeconds[now.Format("2006-01-02")]
	s.stats.DailyAvgSeconds = computeDailyAvg(data.DailySeconds)
	s.stats.WeeklyAvgSeconds = computeWeeklyAvg(data.DailySeconds, now)
	s.updateResetStatusLocked()
	s.mu.Unlock()
	s.emitStats()
	return nil
}

func (s *BiliBreakService) Snooze(minutes int) {
	if minutes < 0 {
		minutes = 0
	}
	if minutes > 240 {
		minutes = 240
	}
	s.setWindowTop(false)
	s.mu.Lock()
	if minutes > 0 {
		s.snoozedUntil = time.Now().Add(time.Duration(minutes) * time.Minute)
	} else {
		s.snoozedUntil = time.Time{}
	}
	s.stats.SinceLastBreakSeconds = 0
	intervalSec := s.cfg.IntervalMinutes * 60
	if intervalSec < 60 {
		intervalSec = 60
	}
	s.stats.NextBreakInSeconds = intervalSec
	s.mu.Unlock()
	s.emitStats()
}

func (s *BiliBreakService) ManualRemind() {
	s.fireReminder("手动提醒")
}

// ===== Internal loop =====

func (s *BiliBreakService) loop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *BiliBreakService) tick() {
	now := time.Now()
	aw := GetActiveWindowInfo()
	s.tickWindow(now, aw)
}

func (s *BiliBreakService) tickWindow(now time.Time, aw ActiveWindowInfo) {
	day := now.Format("2006-01-02")

	// 获取配置
	s.mu.Lock()
	cfg := s.cfg
	snoozedUntil := s.snoozedUntil
	currentDay := s.day

	// 正式版：不再修改标题栏，只计算匹配结果
	match, _ := s.matchesDebug(cfg, strings.ToLower(aw.Title), strings.ToLower(aw.Process))

	// 如果你想让标题栏恢复成软件名，可以加这一句（或者干脆什么都不做，它就是静态的）
	// if s.mainWindow != nil { s.mainWindow.SetTitle("Bili Break Reminder") }

	if day != currentDay {
		s.day = day
		s.stats.TotalWatchedSeconds = s.persistedStats.DailySeconds[day]
		s.stats.SinceLastBreakSeconds = 0
	}

	running := cfg.MonitorEnabled
	watching := false
	if running && aw.OK {
		watching = match
	}

	intervalSec := cfg.IntervalMinutes * 60
	if intervalSec < 60 {
		intervalSec = 60
	}

	s.stats.Running = running
	s.stats.Watching = watching
	s.stats.ActiveTitle = aw.Title
	s.stats.ActiveProcess = aw.Process
	s.stats.Day = day
	if !s.snoozedUntil.IsZero() && now.Before(s.snoozedUntil) {
		s.stats.SnoozedUntil = s.snoozedUntil.Format(time.RFC3339)
	} else {
		s.stats.SnoozedUntil = ""
	}
	if watching {
		s.stats.TotalWatchedSeconds++
		s.stats.CumulativeWatchedSeconds++
		s.stats.SinceLastBreakSeconds++
		if s.persistedStats.DailySeconds == nil {
			s.persistedStats.DailySeconds = make(map[string]int)
		}
		s.persistedStats.DailySeconds[day]++
	}
	s.stats.DailyAvgSeconds = computeDailyAvg(s.persistedStats.DailySeconds)
	s.stats.WeeklyAvgSeconds = computeWeeklyAvg(s.persistedStats.DailySeconds, now)
	next := intervalSec - s.stats.SinceLastBreakSeconds
	if next < 0 {
		next = 0
	}
	s.stats.NextBreakInSeconds = next

	canRemind := now.After(snoozedUntil) || snoozedUntil.IsZero()
	if watching && canRemind && s.stats.SinceLastBreakSeconds >= intervalSec {
		s.stats.SinceLastBreakSeconds = 0
		s.stats.NextBreakInSeconds = intervalSec
		s.mu.Unlock()
		s.persistStats(false)
		s.fireReminder("时间到了")
		s.emitStats()
		return
	}
	s.mu.Unlock()
	s.persistStats(false)
	s.emitStats()
}

func (s *BiliBreakService) persistStats(force bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.stats.CumulativeWatchedSeconds
	stored := s.persistedStats.CumulativeWatchedSeconds
	now := time.Now()
	if !force {
		if current == stored {
			return
		}
		if !s.lastStatsPersistAt.IsZero() && now.Sub(s.lastStatsPersistAt) < 5*time.Second {
			return
		}
	}
	data := s.persistedStats
	data.CumulativeWatchedSeconds = current

	if err := SavePersistedStats(data); err != nil {
		fmt.Println("save persisted stats failed:", err)
		return
	}
	s.persistedStats.CumulativeWatchedSeconds = current
	s.lastStatsPersistAt = now
}
func (s *BiliBreakService) matchesDebug(cfg Config, titleLower, processLower string) (bool, string) {
	titleLower = strings.ToLower(titleLower)
	processLower = strings.ToLower(processLower)

	// 1. 检查白名单 (强力容错分割)
	isTargetProcess := false

	if len(cfg.Processes) > 0 {
		realList := make([]string, 0)
		for _, raw := range cfg.Processes {
			clean := strings.ReplaceAll(raw, "，", ",")
			clean = strings.ReplaceAll(clean, "\n", ",")
			parts := strings.Split(clean, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					realList = append(realList, p)
				}
			}
		}

		for _, p := range realList {
			pLower := strings.ToLower(p)
			if strings.EqualFold(processLower, pLower) {
				isTargetProcess = true
				break
			}
			// .exe 容错
			if !strings.HasSuffix(pLower, ".exe") && strings.EqualFold(processLower, pLower+".exe") {
				isTargetProcess = true
				break
			}
		}

		if !isTargetProcess {
			return false, "不在白名单"
		}
	} else {
		// Empty process list allows any process, but still requires a title keyword.
		return containsTitleKeyword(titleLower, cfg.Keywords), "按标题关键词匹配"
	}

	// 2. 检查是否为浏览器
	isBrowser := isBrowserProcess(processLower)

	// 3. 决策
	if isBrowser {
		if len(cfg.Keywords) == 0 {
			return false, "浏览器无关键词"
		}
		for _, k := range cfg.Keywords {
			if k != "" && strings.Contains(titleLower, strings.ToLower(k)) {
				return true, "浏览器关键词匹配"
			}
		}
		return false, "标题未含关键词"
	} else {
		return true, "应用匹配成功"
	}
}

func (s *BiliBreakService) fireReminder(reason string) {
	s.mu.RLock()
	cfg := s.cfg
	stats := s.stats
	s.mu.RUnlock()
	msg := fmt.Sprintf("休息一下吧～（%s）\n本日累计：%s", reason, formatDuration(stats.TotalWatchedSeconds))

	if cfg.NotifySystem && s.notifier != nil {
		_ = s.notifier.SendNotification(notifications.NotificationOptions{
			ID:    fmt.Sprintf("bili-break-%d", time.Now().UnixNano()),
			Title: "长时间使用提醒",
			Body:  msg,
		})
	}
	if cfg.NotifyPopup || cfg.NotifySound {
		s.setWindowTop(true)
		app := application.Get()
		if app != nil {
			app.Event.Emit("bili:remind", RemindPayload{
				Message:   msg,
				IntervalM: cfg.IntervalMinutes,
				WatchedS:  stats.TotalWatchedSeconds,
			})
		}
	}
}

func (s *BiliBreakService) emitStats() {
	app := application.Get()
	if app == nil {
		return
	}
	s.mu.RLock()
	stats := s.stats
	s.mu.RUnlock()
	app.Event.Emit("bili:stats", stats)
}

func (s *BiliBreakService) emitConfig() {
	app := application.Get()
	if app == nil {
		return
	}
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	app.Event.Emit("bili:config", cfg)
}

func formatDuration(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// computeDailyAvg returns the average seconds per active day across all recorded days.
func computeDailyAvg(dailySeconds map[string]int) int {
	if len(dailySeconds) == 0 {
		return 0
	}
	total := 0
	for _, v := range dailySeconds {
		total += v
	}
	return total / len(dailySeconds)
}

// computeWeeklyAvg returns the average seconds per day over the last 7 calendar days.
func computeWeeklyAvg(dailySeconds map[string]int, now time.Time) int {
	total := 0
	for i := 0; i < 7; i++ {
		day := now.AddDate(0, 0, -i).Format("2006-01-02")
		total += dailySeconds[day]
	}
	return total / 7
}
