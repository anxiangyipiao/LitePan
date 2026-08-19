import { onActivated, onDeactivated, onUnmounted } from "vue";

export function useConditionalPolling(options: {
  intervalMs: number;
  shouldPoll: () => boolean;
  onTick: () => void | Promise<void>;
  tickWhen?: () => boolean;
}) {
  let timer: number | null = null;
  let scopeActive = true;
  let pausedByHidden = false;

  function start() {
    if (!scopeActive || timer !== null) return;
    timer = window.setInterval(() => {
      if (document.hidden) return;
      if (options.tickWhen && !options.tickWhen()) return;
      void options.onTick();
    }, options.intervalMs);
  }

  function stop() {
    if (timer === null) return;
    window.clearInterval(timer);
    timer = null;
  }

  function sync() {
    if (options.shouldPoll()) start();
    else stop();
  }

  function isActive() {
    return timer !== null;
  }

  function onVisibilityChange() {
    if (document.hidden) {
      pausedByHidden = timer !== null;
      stop();
    } else if (pausedByHidden && scopeActive) {
      pausedByHidden = false;
      sync();
    }
  }

  document.addEventListener("visibilitychange", onVisibilityChange);

  onActivated(() => {
    scopeActive = true;
    sync();
  });
  onDeactivated(() => {
    scopeActive = false;
    stop();
  });
  onUnmounted(() => {
    document.removeEventListener("visibilitychange", onVisibilityChange);
    stop();
  });

  return { start, stop, sync, isActive };
}
