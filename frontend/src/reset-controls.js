let undoButton;
let busy = false;
let latestStats;

export function updateResetControls(stats) {
  latestStats = stats;
  if (!undoButton) return;
  undoButton.disabled = busy || !stats.canUndoReset;
  undoButton.textContent = stats.undoResetKind === "cumulative" ? "撤销清空累计学习" : "撤销清空今日";
}

export function wireResetControls({ App, refresh }) {
  const todayButton = document.getElementById("btnReset");
  const container = todayButton.parentElement;
  container.style.flexWrap = "wrap";
  const cumulativeButton = document.createElement("button");
  cumulativeButton.id = "btnResetCumulative";
  cumulativeButton.className = "textButton mutedButton";
  cumulativeButton.textContent = "清空累计学习";
  undoButton = document.createElement("button");
  undoButton.id = "btnUndoReset";
  undoButton.className = "textButton";
  undoButton.textContent = "撤销清空";
  undoButton.disabled = true;
  const hint = document.createElement("span");
  hint.className = "note";
  hint.setAttribute("role", "status");
  hint.style.width = "100%";
  container.append(cumulativeButton, undoButton, hint);

  const overlay = document.createElement("div");
  overlay.className = "overlay hidden";
  overlay.style.zIndex = "100";
  overlay.setAttribute("role", "dialog");
  overlay.setAttribute("aria-modal", "true");
  overlay.setAttribute("aria-labelledby", "resetTitle");
  overlay.innerHTML = `<div class="modal"><div id="resetTitle" class="modalTitle"></div><p class="modalBody" id="resetDescription"></p><div class="modalActions"><button id="btnCancelReset" class="btn">取消清空</button><button id="btnConfirmReset" class="btn btn--primary">确认清空</button></div></div>`;
  document.body.append(overlay);
  const cancel = overlay.querySelector("#btnCancelReset");
  const confirm = overlay.querySelector("#btnConfirmReset");
  let kind = "today";
  let returnFocus;
  function close() { overlay.classList.add("hidden"); returnFocus?.focus(); }
  function open(nextKind, trigger) {
    kind = nextKind;
    returnFocus = trigger;
    overlay.querySelector("#resetTitle").textContent = kind === "today" ? "清空今日累计？" : "清空累计学习时间？";
    overlay.querySelector("#resetDescription").textContent = kind === "today"
      ? "清空今日统计和今日图表记录，历史累计学习与休息倒计时保持不变。清空后可撤销，重启后也可以恢复。"
      : "仅清空累计学习时间，今日统计、历史图表与休息倒计时保持不变。清空后可撤销，恢复时会保留后来新增的计时。";
    overlay.classList.remove("hidden");
    cancel.focus();
  }
  async function run(action, message) {
    busy = true;
    todayButton.disabled = cumulativeButton.disabled = confirm.disabled = cancel.disabled = true;
    undoButton.disabled = true;
    try { await action(); hint.textContent = message; close(); await refresh(); }
    catch (error) { hint.textContent = `操作失败：${error?.message || error}`; close(); }
    finally {
      busy = false;
      todayButton.disabled = cumulativeButton.disabled = confirm.disabled = cancel.disabled = false;
      if (latestStats) updateResetControls(latestStats);
    }
  }
  todayButton.addEventListener("click", () => open("today", todayButton));
  cumulativeButton.addEventListener("click", () => open("cumulative", cumulativeButton));
  cancel.addEventListener("click", close);
  overlay.addEventListener("click", event => { if (event.target === overlay && !busy) close(); });
  overlay.addEventListener("keydown", event => {
    if (event.key === "Escape" && !busy) { event.preventDefault(); close(); }
    if (event.key === "Tab") { event.preventDefault(); (document.activeElement === cancel ? confirm : cancel).focus(); }
  });
  confirm.addEventListener("click", () => run(
    () => kind === "today" ? App.ResetToday() : App.ResetCumulative(), "已清空，可使用撤销按钮恢复。"));
  undoButton.addEventListener("click", () => { returnFocus = undoButton; void run(() => App.UndoReset(), "已恢复清空前的时间，并保留清空后新增的计时。"); });
  if (latestStats) updateResetControls(latestStats);
}
