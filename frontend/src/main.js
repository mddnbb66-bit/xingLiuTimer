import { Events } from "@wailsio/runtime";

// ---------------------------------------------------------
// ✅ 核心修复：路径必须匹配你截图里的 "changeme"
// ---------------------------------------------------------
import * as App from "../bindings/changeme/bilibreakservice.js";
import { createIntervalController } from "./interval-controller.js";
import { wireResetControls, updateResetControls } from "./reset-controls.js";

const $ = (id) => document.getElementById(id);

const els = {
  statusPill: $("statusPill"),
  btnStartStop: $("btnStartStop"),
  statTotal: $("statTotal"),
  statCumulative: $("statCumulative"),
  statNext: $("statNext"),
  statDailyAvg: $("statDailyAvg"),
  statWeeklyAvg: $("statWeeklyAvg"),
  statWindow: $("statWindow"),
  statProcess: $("statProcess"),
  btnManual: $("btnManual"),
  btnReset: $("btnReset"),
  btnClearNext: $("btnClearNext"),
  btnCycleCumulativeFormat: $("btnCycleCumulativeFormat"),
  cumulativeFormatHint: $("cumulativeFormatHint"),
  intervalRange: $("intervalRange"),
  intervalNumber: $("intervalNumber"),
  intervalNote: $("intervalNote"),
  alertLeadRange: $("alertLeadRange"),
  alertLeadNumber: $("alertLeadNumber"),
  toggleAutoShowNearBreak: $("toggleAutoShowNearBreak"),
  toggleSystem: $("toggleSystem"),
  togglePopup: $("togglePopup"),
  toggleSound: $("toggleSound"),
  snoozeNumber: $("snoozeNumber"),
  btnSnooze: $("btnSnooze"),
  snoozeHint: $("snoozeHint"),
  keywords: $("keywords"),
  processes: $("processes"),
  toggleAutoStart: $("toggleAutoStart"),
  toggleClockAlwaysOn: $("toggleClockAlwaysOn"),
  clockFadeAfter: $("clockFadeAfter"),
  btnSave: $("btnSave"),
  saveHint: $("saveHint"),
  modalOverlay: $("modalOverlay"),
  modalBody: $("modalBody"),
  modalOk: $("modalOk"),
  modalSnooze: $("modalSnooze"),
  floatingClock: $("floatingClock"),
  studyChart: $("studyChart"),
};

const cumulativeDisplayModes = ["hms", "day", "month", "year", "smart"];

let state = {
  cfg: null,
  stats: null,
  dirty: false,
  cumulativeDisplayMode: "hms",
};

const intervalController = createIntervalController({
  save: (minutes) => App.SetReminderInterval(minutes),
  applied: (minutes, stats) => {
    state.cfg = { ...(state.cfg || {}), intervalMinutes: minutes };
    state.stats = stats;
    renderStats(stats);
  },
  status: (message) => { els.intervalNote.textContent = message; },
});

const clockState = {
  hideTimer: null,
  leaveTimer: null,
  dragging: false,
  offsetX: 0,
  offsetY: 0,
  x: null,
  y: null,
};

function updatePresetSelection() {
  document.querySelectorAll("[data-preset]").forEach(button => {
    const selected = button.dataset.preset === els.intervalNumber.value;
    button.classList.toggle("chip--selected", selected);
    button.setAttribute("aria-pressed", String(selected));
  });
}

function markDirty(on = true) {
  updatePresetSelection();
  state.dirty = on;
  els.saveHint.textContent = on ? "未保存" : "";
}

function clampInt(v, min, max) {
  const n = Number.parseInt(String(v), 10);
  if (Number.isNaN(n)) return min;
  return Math.max(min, Math.min(max, n));
}

function splitList(text) {
  return String(text || "")
    .split(/[,，\n]/g)
    .map((s) => s.trim())
    .filter(Boolean);
}

function fmtHMS(totalSeconds) {
  const s = Math.max(0, Number(totalSeconds || 0));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = s % 60;
  if (h > 0) return `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}:${String(ss).padStart(2, "0")}`;
  return `${String(m).padStart(2, "0")}:${String(ss).padStart(2, "0")}`;
}

function fmtMMSS(totalSeconds) {
  const s = Math.max(0, Number(totalSeconds || 0));
  const m = Math.floor(s / 60);
  const ss = s % 60;
  return `${String(m).padStart(2, "0")}:${String(ss).padStart(2, "0")}`;
}

function fmtDecimal(value) {
  if (value >= 100) return Math.round(value).toString();
  if (value >= 10) return value.toFixed(1).replace(/\.0$/, "");
  return value.toFixed(2).replace(/0+$/, "").replace(/\.$/, "");
}

function getCumulativeModeLabel(mode) {
  switch (mode) {
    case "day":
      return "累计格式：天";
    case "month":
      return "累计格式：月";
    case "year":
      return "累计格式：年";
    case "smart":
      return "累计格式：智能";
    default:
      return "累计格式：时分秒";
  }
}

function formatCumulativeSeconds(totalSeconds, mode = state.cumulativeDisplayMode) {
  const s = Math.max(0, Number(totalSeconds || 0));
  const daySeconds = 24 * 3600;
  const monthSeconds = 30 * daySeconds;
  const yearSeconds = 365 * daySeconds;

  switch (mode) {
    case "day":
      return `${fmtDecimal(s / daySeconds)} 天`;
    case "month":
      return `${fmtDecimal(s / monthSeconds)} 月`;
    case "year":
      return `${fmtDecimal(s / yearSeconds)} 年`;
    case "smart":
      if (s >= yearSeconds) return `${fmtDecimal(s / yearSeconds)} 年`;
      if (s >= monthSeconds) return `${fmtDecimal(s / monthSeconds)} 月`;
      if (s >= daySeconds) return `${fmtDecimal(s / daySeconds)} 天`;
      return fmtHMS(s);
    default:
      return fmtHMS(s);
  }
}

function cycleCumulativeDisplayMode() {
  const currentIndex = cumulativeDisplayModes.indexOf(state.cumulativeDisplayMode);
  const nextIndex = currentIndex >= 0 ? (currentIndex + 1) % cumulativeDisplayModes.length : 0;
  state.cumulativeDisplayMode = cumulativeDisplayModes[nextIndex];
  els.btnCycleCumulativeFormat.textContent = getCumulativeModeLabel(state.cumulativeDisplayMode);
  els.cumulativeFormatHint.textContent = state.cumulativeDisplayMode === "smart"
    ? "智能模式会自动切到天 / 月 / 年。"
    : "可在时分秒 / 天 / 月 / 年 / 智能之间切换。";
  if (state.stats) renderStats(state.stats);
}

function applyCfgToUI(cfg) {
  if (!cfg) return;
  els.intervalRange.value = String(cfg.intervalMinutes ?? 30);
  els.intervalNumber.value = String(cfg.intervalMinutes ?? 30);
  els.alertLeadRange.value = String(cfg.clockAlertMinutes ?? 5);
  els.alertLeadNumber.value = String(cfg.clockAlertMinutes ?? 5);
  els.toggleAutoShowNearBreak.checked = cfg.clockAutoShowAlert ?? true;
  els.toggleSystem.checked = !!cfg.notifySystem;
  els.togglePopup.checked = !!cfg.notifyPopup;
  els.toggleSound.checked = !!cfg.notifySound;
  els.snoozeNumber.value = String(cfg.snoozeMinutes ?? 10);
  els.keywords.value = (cfg.keywords || []).join(", ");
  els.processes.value = (cfg.processes || []).join(", ");
  els.toggleAutoStart.checked = !!cfg.autoStart;
  els.toggleClockAlwaysOn.checked = !!cfg.clockAlwaysOn;
  els.clockFadeAfter.value = String(cfg.clockFadeAfterSecs ?? 20);
  updatePresetSelection();
  updateClockVisibility(true);
}

function collectCfgFromUI() {
  const intervalMinutes = clampInt(els.intervalNumber.value, 1, 240);
  const snoozeMinutes = clampInt(els.snoozeNumber.value, 0, 240);
  const clockFadeAfterSecs = clampInt(els.clockFadeAfter.value, 3, 600);
  const clockAlertMinutes = clampInt(els.alertLeadNumber.value, 1, 60);
  return {
    recognitionVersion: state.cfg?.recognitionVersion ?? 1,
    intervalMinutes,
    monitorEnabled: state.cfg?.monitorEnabled ?? true,
    notifySystem: !!els.toggleSystem.checked,
    notifyPopup: !!els.togglePopup.checked,
    notifySound: !!els.toggleSound.checked,
    snoozeMinutes,
    autoStart: !!els.toggleAutoStart.checked,
    keywords: splitList(els.keywords.value),
    processes: splitList(els.processes.value),
    clockAlwaysOn: !!els.toggleClockAlwaysOn.checked,
    clockFadeAfterSecs,
    clockAlertMinutes,
    clockAutoShowAlert: !!els.toggleAutoShowNearBreak.checked,
  };
}

function getRemainingSeconds() {
  const n = state.stats?.nextBreakInSeconds;
  if (typeof n !== "number") return null;
  return Math.max(0, n);
}

function isInAlertWindow() {
  const remaining = getRemainingSeconds();
  if (remaining === null) return false;
  const alertSecs = clampInt(state.cfg?.clockAlertMinutes ?? 5, 1, 60) * 60;
  return !!state.stats?.running && remaining <= alertSecs;
}

function shouldForceShowClock() {
  if (state.cfg?.clockAlwaysOn) return true;
  return !!state.cfg?.clockAutoShowAlert && isInAlertWindow();
}

function updateFloatingClockFromStats() {
  const remaining = getRemainingSeconds();
  els.floatingClock.textContent = remaining === null ? "--:--" : fmtMMSS(remaining);
  els.floatingClock.classList.toggle("floatingClock--alert", isInAlertWindow());

  if (shouldForceShowClock()) {
    clearClockTimers();
    showClock();
  }
}

function showClock() {
  els.floatingClock.classList.add("floatingClock--visible");
  els.floatingClock.classList.remove("floatingClock--hidden");
}

function hideClock() {
  if (shouldForceShowClock()) {
    showClock();
    return;
  }
  els.floatingClock.classList.remove("floatingClock--visible");
  els.floatingClock.classList.add("floatingClock--hidden");
}

function clearClockTimers() {
  if (clockState.hideTimer) {
    clearTimeout(clockState.hideTimer);
    clockState.hideTimer = null;
  }
  if (clockState.leaveTimer) {
    clearTimeout(clockState.leaveTimer);
    clockState.leaveTimer = null;
  }
}

function scheduleClockHide() {
  if (shouldForceShowClock()) {
    clearClockTimers();
    showClock();
    return;
  }
  if (clockState.hideTimer) clearTimeout(clockState.hideTimer);
  const secs = clampInt(state.cfg?.clockFadeAfterSecs ?? 20, 3, 600);
  clockState.hideTimer = setTimeout(() => hideClock(), secs * 1000);
}

function updateClockVisibility(forceShow = false) {
  clearClockTimers();
  if (forceShow) showClock();
  scheduleClockHide();
}

function setClockPosition(x, y) {
  x = Math.max(8, Math.min(x, window.innerWidth - els.floatingClock.offsetWidth - 8));
  y = Math.max(8, Math.min(y, window.innerHeight - els.floatingClock.offsetHeight - 8));
  clockState.x = x;
  clockState.y = y;
  els.floatingClock.style.right = "auto";
  els.floatingClock.style.bottom = "auto";
  els.floatingClock.style.left = `${x}px`;
  els.floatingClock.style.top = `${y}px`;
  els.floatingClock.style.transform = "translate(0, 0)";
}

function wireClock() {
  if (!els.floatingClock) return;
  els.floatingClock.textContent = "--:--";
  els.floatingClock.classList.add("floatingClock--visible");

  const onPointerMove = (event) => {
    if (!clockState.dragging) return;
    const x = event.clientX - clockState.offsetX;
    const y = event.clientY - clockState.offsetY;
    setClockPosition(x, y);
  };

  const onPointerUp = () => {
    clockState.dragging = false;
    window.removeEventListener("pointermove", onPointerMove);
    window.removeEventListener("pointerup", onPointerUp);
    scheduleClockHide();
  };

  els.floatingClock.addEventListener("pointerdown", (event) => {
    const rect = els.floatingClock.getBoundingClientRect();
    clockState.dragging = true;
    clockState.offsetX = event.clientX - rect.left;
    clockState.offsetY = event.clientY - rect.top;
    clearClockTimers();
    showClock();
    window.addEventListener("pointermove", onPointerMove);
    window.addEventListener("pointerup", onPointerUp);
  });

  els.floatingClock.addEventListener("mouseenter", () => {
    clearClockTimers();
    showClock();
  });

  els.floatingClock.addEventListener("mouseleave", () => {
    if (shouldForceShowClock()) return;
    if (clockState.leaveTimer) clearTimeout(clockState.leaveTimer);
    clockState.leaveTimer = setTimeout(() => {
      hideClock();
    }, 5000);
  });

}

function renderStats(stats) {
  if (!stats) return;
  updateResetControls(stats);
  updateFloatingClockFromStats();
  els.statTotal.textContent = fmtHMS(stats.totalWatchedSeconds);
  els.statCumulative.textContent = formatCumulativeSeconds(stats.cumulativeWatchedSeconds);
  els.btnCycleCumulativeFormat.textContent = getCumulativeModeLabel(state.cumulativeDisplayMode);
  if (typeof stats.nextBreakInSeconds === "number") {
    els.statNext.textContent = fmtMMSS(stats.nextBreakInSeconds);
  } else {
    els.statNext.textContent = "--:--";
  }
  els.statDailyAvg.textContent = typeof stats.dailyAvgSeconds === "number" && stats.dailyAvgSeconds > 0
    ? fmtHMS(stats.dailyAvgSeconds)
    : "--:--";
  els.statWeeklyAvg.textContent = typeof stats.weeklyAvgSeconds === "number" && stats.weeklyAvgSeconds > 0
    ? fmtHMS(stats.weeklyAvgSeconds)
    : "--:--";
  els.statWindow.textContent = stats.activeTitle || "--";
  els.statProcess.textContent = stats.activeProcess || "--";

  if (stats.running) {
    els.statusPill.classList.remove("pill--stopped");
    els.statusPill.classList.add("pill--running");
    els.statusPill.textContent = stats.watching ? "监测中 · 计时中" : "监测中 · 等待匹配窗口";
    els.btnStartStop.textContent = "停止监控";
  } else {
    els.statusPill.classList.remove("pill--running");
    els.statusPill.classList.add("pill--stopped");
    els.statusPill.textContent = "未运行";
    els.btnStartStop.textContent = "启动监控";
  }

  if (stats.snoozedUntil) {
    els.snoozeHint.textContent = `已延后至：${stats.snoozedUntil.replace("T", " ").replace("Z", "")}`;
  } else {
    els.snoozeHint.textContent = "";
  }
}

function showModal(message) {
  els.modalBody.textContent = message || "";
  els.modalOverlay.classList.remove("hidden");
}

function hideModal() {
  els.modalOverlay.classList.add("hidden");
}

function beep() {
  try {
    const AudioContext = window.AudioContext || window.webkitAudioContext;
    if (!AudioContext) return;
    const ctx = new AudioContext();
    const o = ctx.createOscillator();
    const g = ctx.createGain();
    o.type = "sine";
    o.frequency.value = 880;
    g.gain.value = 0.05;
    o.connect(g);
    g.connect(ctx.destination);
    o.start();
    setTimeout(() => {
      o.stop();
      ctx.close();
    }, 220);
  } catch (e) {
    console.warn("beep failed:", e);
  }
}

async function refreshOnce() {
  try {
    // ✅ 使用 App (从 changeme 文件夹导入的)
    const cfg = await App.GetConfig();
    state.cfg = cfg;
    applyCfgToUI(cfg);
    const stats = await App.GetStats();
    state.stats = stats;
    renderStats(stats);
    markDirty(false);
  } catch (e) {
    console.error("refreshOnce failed:", e);
    els.saveHint.textContent = "连接后端失败";
  }
}

function wireUI() {
  wireResetControls({ App, refresh: async () => {
    state.stats = await App.GetStats();
    renderStats(state.stats);
    await refreshChart();
  } });
  const captureButton = $("btnCaptureTarget");
  const captureHint = $("captureHint");
  captureButton.addEventListener("click", async () => {
    captureButton.disabled = true;
    els.btnSave.disabled = true;
    captureButton.textContent = "正在等待切换窗口…";
    captureHint.textContent = "请在 5 秒内切换到目标软件或浏览器标签页，并保持该窗口聚焦。";
    try {
      await intervalController.flush();
      const target = await App.CaptureFocusedTarget();
      const appendUnique = (field, value) => {
        const values = splitList(field.value);
        if (value && !values.some(item => item.toLowerCase() === value.toLowerCase())) values.push(value);
        field.value = values.join(", ");
      };
      appendUnique(els.processes, target.process);
      if (target.browser) appendUnique(els.keywords, target.keyword);
      markDirty(true);
      const cfg = collectCfgFromUI();
      await App.SetConfig(cfg);
      state.cfg = cfg;
      markDirty(false);
      captureHint.textContent = target.browser
        ? `已保存：${target.process} · 网页关键词「${target.keyword}」。可在识别规则中缩短关键词，以匹配同类页面。`
        : `已保存：${target.process}。聚焦此软件时开始计时。`;
    } catch (error) {
      captureHint.textContent = `添加未完成：${String(error?.message || error)}。如已出现新规则，可点击保存设置重试。`;
    } finally {
      captureButton.disabled = false;
      els.btnSave.disabled = false;
      captureButton.textContent = "添加聚焦的软件 / 网页";
    }
  });

  const syncAlertLead = (from) => {
    const v = clampInt(from.value, 1, 60);
    els.alertLeadRange.value = String(v);
    els.alertLeadNumber.value = String(v);
    markDirty(true);
    state.cfg = {
      ...(state.cfg || {}),
      clockAlertMinutes: v,
      clockAutoShowAlert: !!els.toggleAutoShowNearBreak.checked,
    };
    updateFloatingClockFromStats();
    updateClockVisibility(true);
  };

  const syncInterval = (from) => {
    const v = Number(from.value);
    if (from.value === "" || !Number.isInteger(v) || v < 1 || v > 240) {
      els.intervalNote.textContent = "请输入 1～240 之间的整数分钟。";
      return;
    }
    els.intervalRange.value = String(v);
    els.intervalNumber.value = String(v);
    updatePresetSelection();
    intervalController.schedule(v);
  };

  els.intervalRange.addEventListener("input", () => syncInterval(els.intervalRange));
  els.intervalNumber.addEventListener("input", () => syncInterval(els.intervalNumber));
  els.alertLeadRange.addEventListener("input", () => syncAlertLead(els.alertLeadRange));
  els.alertLeadNumber.addEventListener("input", () => syncAlertLead(els.alertLeadNumber));

  document.querySelectorAll(".chip[data-preset]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const v = clampInt(btn.getAttribute("data-preset"), 1, 240);
      els.intervalRange.value = String(v);
      els.intervalNumber.value = String(v);
      updatePresetSelection();
      intervalController.schedule(v, true);
    });
  });

  [
    els.toggleSystem,
    els.togglePopup,
    els.toggleSound,
    els.toggleAutoStart,
    els.toggleClockAlwaysOn,
    els.toggleAutoShowNearBreak,
    els.snoozeNumber,
    els.clockFadeAfter,
    els.keywords,
    els.processes,
  ].forEach((el) =>
    el.addEventListener("input", () => {
      markDirty(true);
      if (el === els.toggleClockAlwaysOn || el === els.clockFadeAfter || el === els.toggleAutoShowNearBreak) {
        state.cfg = {
          ...(state.cfg || {}),
          clockAlwaysOn: !!els.toggleClockAlwaysOn.checked,
          clockFadeAfterSecs: clampInt(els.clockFadeAfter.value, 3, 600),
          clockAlertMinutes: clampInt(els.alertLeadNumber.value, 1, 60),
          clockAutoShowAlert: !!els.toggleAutoShowNearBreak.checked,
        };
        updateFloatingClockFromStats();
        updateClockVisibility(true);
      }
    }),
  );

  els.btnSave.addEventListener("click", async () => {
    els.saveHint.textContent = "保存中...";
    try {
      await intervalController.flush();
      if (!els.intervalNumber.checkValidity() || els.intervalNumber.value === "") {
        els.intervalNumber.reportValidity();
        throw new Error("提醒间隔必须是 1～240 的整数");
      }
      const cfg = collectCfgFromUI();
      await App.SetConfig(cfg); // ✅ Direct call
      state.cfg = cfg;
      markDirty(false);
      els.saveHint.textContent = "已保存";
      setTimeout(() => (els.saveHint.textContent = ""), 1200);
    } catch (e) {
      console.error(e);
      els.saveHint.textContent = "保存失败";
    }
  });

  els.btnStartStop.addEventListener("click", async () => {
    try {
      if (state.stats?.running) {
        await App.Stop(); // ✅ Direct call
      } else {
        await App.Start(); // ✅ Direct call
      }
    } catch (e) {
      console.error(e);
    }
  });

  els.btnManual.addEventListener("click", async () => {
    try {
      await App.ManualRemind(); // ✅ Direct call
    } catch (e) {
      console.error(e);
    }
  });

  els.btnClearNext.addEventListener("click", async () => {
    try {
      await App.Snooze(0);
    } catch (e) {
      console.error(e);
    }
  });

  els.btnCycleCumulativeFormat.addEventListener("click", () => {
    cycleCumulativeDisplayMode();
  });

  els.btnSnooze.addEventListener("click", async () => {
    const mins = clampInt(els.snoozeNumber.value, 0, 240);
    try {
      await App.Snooze(mins); // ✅ Direct call
    } catch (e) {
      console.error(e);
    }
  });

  els.modalOk.addEventListener("click", hideModal);
  els.modalOverlay.addEventListener("click", (e) => {
    if (e.target === els.modalOverlay) hideModal();
  });
  els.modalSnooze.addEventListener("click", async () => {
    const mins = clampInt(els.snoozeNumber.value, 0, 240);
    try {
      await App.Snooze(mins); // ✅ Direct call
    } catch (e) {
      console.error(e);
    } finally {
      hideModal();
    }
  });
}

function wireEvents() {
  Events.On("bili:stats", (event) => {
    state.stats = event.data;
    renderStats(event.data);
    // Refresh chart at most every 30s to avoid excessive redraws
    const now = Date.now();
    if (!wireEvents._lastChartRefresh || now - wireEvents._lastChartRefresh > 30000) {
      wireEvents._lastChartRefresh = now;
      refreshChart();
    }
  });

  Events.On("bili:config", (event) => {
    state.cfg = event.data;
    if (!state.dirty && !intervalController.busy) applyCfgToUI(event.data);
  });

  Events.On("bili:remind", (event) => {
    const payload = event.data || {};
    if (state.cfg?.notifySound) beep();
    if (state.cfg?.notifyPopup) showModal(payload.message || "该休息啦～");
  });
}

// ===== Study history chart =====

const chartState = {
  days: 7,
};

function fmtMinutes(totalSeconds) {
  return Math.round(Math.max(0, Number(totalSeconds || 0)) / 60);
}

function isoDateLabel(dateStr, compact) {
  // dateStr: "2006-01-02"
  const parts = dateStr.split("-");
  if (parts.length < 3) return dateStr;
  const m = parseInt(parts[1], 10);
  const d = parseInt(parts[2], 10);
  if (compact) return `${m}/${d}`;
  return `${m}月${d}日`;
}

function drawChart(points) {
  const svg = els.studyChart;
  if (!svg) return;

  // Clear previous content
  while (svg.firstChild) svg.removeChild(svg.firstChild);

  const W = svg.clientWidth || 700;
  const H = 210;
  const padL = 48;
  const padR = 16;
  const padT = 18;
  const padB = 36;
  const innerW = W - padL - padR;
  const innerH = H - padT - padB;

  const ns = "http://www.w3.org/2000/svg";
  const mk = (tag, attrs) => {
    const el = document.createElementNS(ns, tag);
    for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
    return el;
  };

  // Gradient definition
  const defs = mk("defs", {});
  const gradId = "chartGrad";
  const grad = mk("linearGradient", { id: gradId, x1: "0", y1: "0", x2: "0", y2: "1" });
  grad.appendChild(mk("stop", { offset: "0%", "stop-color": "rgba(107,184,129,0.20)" }));
  grad.appendChild(mk("stop", { offset: "100%", "stop-color": "rgba(107,184,129,0.02)" }));
  defs.appendChild(grad);
  svg.appendChild(defs);

  if (!points.length) return;
  const maxSecs = Math.max(...points.map((p) => p.seconds), 60);
  const n = points.length;

  const xOf = (i) => n > 1 ? padL + (i / (n - 1)) * innerW : padL + innerW / 2;
  const yOf = (secs) => padT + innerH - (secs / maxSecs) * innerH;

  // Grid lines + Y labels (4 lines)
  const yTicks = 4;
  for (let t = 0; t <= yTicks; t++) {
    const secs = (maxSecs / yTicks) * t;
    const y = yOf(secs);
    svg.appendChild(mk("line", {
      x1: padL, y1: y, x2: padL + innerW, y2: y,
      stroke: "#e8e8ed", "stroke-width": "1",
      "stroke-dasharray": t === 0 ? "" : "4 4",
    }));
    const mins = Math.round(secs / 60);
    const label = mins >= 60 ? `${(mins / 60).toFixed(1)}h` : `${mins}m`;
    const txt = mk("text", {
      x: padL - 6, y: y + 4,
      "text-anchor": "end",
      fill: "#6e6e73",
      "font-size": "11",
      "font-family": "ui-monospace,monospace",
    });
    txt.textContent = label;
    svg.appendChild(txt);
  }

  // Area fill path
  const compact = n > 14;
  const linePoints = points.map((p, i) => `${xOf(i)},${yOf(p.seconds)}`).join(" ");
  const areaD = `M${xOf(0)},${yOf(points[0].seconds)} `
    + points.slice(1).map((p, i) => `L${xOf(i + 1)},${yOf(p.seconds)}`).join(" ")
    + ` L${xOf(n - 1)},${padT + innerH} L${padL},${padT + innerH} Z`;
  svg.appendChild(mk("path", {
    d: areaD,
    fill: `url(#${gradId})`,
  }));

  // Line
  const lineD = `M${xOf(0)},${yOf(points[0].seconds)} `
    + points.slice(1).map((p, i) => `L${xOf(i + 1)},${yOf(p.seconds)}`).join(" ");
  svg.appendChild(mk("path", {
    d: lineD,
    fill: "none",
    stroke: "#326b46",
    "stroke-width": "2",
    "stroke-linejoin": "round",
    "stroke-linecap": "round",
  }));

  // Dots + X labels
  const today = new Date().toISOString().slice(0, 10);
  points.forEach((p, i) => {
    const x = xOf(i);
    const y = yOf(p.seconds);
    const isToday = p.date === today;

    // Only draw dot if it has data or is today
    if (p.seconds > 0 || isToday) {
      if (isToday) {
        svg.appendChild(mk("circle", {
          cx: x, cy: y, r: "6",
          fill: "rgba(107,184,129,0.20)",
          stroke: "#326b46",
          "stroke-width": "2",
        }));
      }
      svg.appendChild(mk("circle", {
        cx: x, cy: y, r: isToday ? "4" : "3",
        fill: isToday ? "#326b46" : "#326b46",
      }));
    }

    // X-axis label — show every label when ≤14 days, every other when >14
    if (!compact || i % 2 === 0 || i === n - 1) {
      const txt = mk("text", {
        x: x, y: H - 6,
        "text-anchor": "middle",
        fill: isToday ? "#255b3b" : "#6e6e73",
        "font-size": "10",
        "font-family": "ui-monospace,monospace",
        "font-weight": isToday ? "700" : "400",
      });
      txt.textContent = isoDateLabel(p.date, compact);
      svg.appendChild(txt);
    }

    // Tooltip-style value above dot for non-zero days
    if (p.seconds > 0) {
      const mins = fmtMinutes(p.seconds);
      const label = mins >= 60 ? `${(mins / 60).toFixed(1)}h` : `${mins}m`;
      const txt = mk("text", {
        x: x, y: y - 9,
        "text-anchor": "middle",
        fill: "#255b3b",
        "font-size": "10",
        "font-family": "ui-monospace,monospace",
        "font-weight": "600",
      });
      txt.textContent = label;
      svg.appendChild(txt);
    }
  });
}

async function refreshChart() {
  try {
    const points = await App.GetDailyHistory(chartState.days);
    drawChart(points);
  } catch {
    // If backend is unavailable (e.g. dev preview), draw with empty data
    const today = new Date();
    const points = Array.from({ length: chartState.days }, (_, i) => {
      const d = new Date(today);
      d.setDate(d.getDate() - (chartState.days - 1 - i));
      return { date: d.toISOString().slice(0, 10), seconds: 0 };
    });
    drawChart(points);
  }
}

function wireChart() {
  document.querySelectorAll(".chartRangeBtn").forEach((btn) => {
    btn.addEventListener("click", () => {
      document.querySelectorAll(".chartRangeBtn").forEach((b) =>
        b.classList.remove("chartRangeBtn--active"),
      );
      btn.classList.add("chartRangeBtn--active");
      chartState.days = parseInt(btn.getAttribute("data-days"), 10);
      refreshChart();
    });
  });

  // Redraw on resize
  let resizeTimer = null;
  window.addEventListener("resize", () => {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(() => refreshChart(), 80);
  });
}

async function main() {
  wireClock();
  wireUI();
  wireChart();
  wireEvents();
  els.btnCycleCumulativeFormat.textContent = getCumulativeModeLabel(state.cumulativeDisplayMode);
  els.cumulativeFormatHint.textContent = "可在时分秒 / 天 / 月 / 年 / 智能之间切换。";
  setTimeout(refreshOnce, 100);
  setTimeout(refreshChart, 200);
}

main();
