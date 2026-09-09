// Serialize saves so an older response can never overwrite a newer selection.
export function createIntervalController({ save, applied, status, delay = 400 }) {
  let pending = null;
  let active = null;
  let timer = null;
  let revision = 0;
  let failure = null;
  async function drain() {
    if (active) return active;
    active = (async () => {
      while (pending) {
        const request = pending;
        pending = null;
        try {
          const stats = await save(request.value);
          failure = null;
          if (request.revision === revision) {
            applied(request.value, stats);
            status(`间隔已生效：${request.value} 分钟。更换间隔会重新开始计时。`);
          }
        } catch (error) {
          failure = error;
          if (request.revision === revision) status(`间隔保存失败：${error?.message || error}。请重新选择后重试。`);
        }
      }
    })();
    try { await active; } finally { active = null; }
  }
  return {
    get busy() { return pending !== null || active !== null; },
    schedule(value, immediate = false) {
      if (!Number.isInteger(value) || value < 1 || value > 240) return false;
      pending = { value, revision: ++revision };
      status(`正在保存 ${value} 分钟…`);
      clearTimeout(timer);
      if (immediate) void drain();
      else timer = setTimeout(() => { void drain(); }, delay);
      return true;
    },
    async flush() {
      clearTimeout(timer);
      await drain();
      if (failure) throw failure;
    },
  };
}
