import { computed, onUnmounted, ref } from "vue";
import {
  deleteCrossTransferRelayTasks,
  listCrossTransferRelayTasks,
  type CrossTransferRelayTask,
} from "@/api/crossTransfer";

function relayTaskActivityRank(task: CrossTransferRelayTask) {
  if (task.status === "running") return 0;
  if (task.status === "pending") return 1;
  return 2;
}

export function useRelayTasks() {
  const relayTasks = ref<CrossTransferRelayTask[]>([]);
  const relayTaskView = ref<"running" | "completed">("running");
  const relayTaskOrderMap = ref<Record<string, number>>({});
  let relayTaskOrderCounter = 0;
  let relayPollingTimer: ReturnType<typeof setInterval> | null = null;
  let relayEventSource: EventSource | null = null;
  let relaySseReconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let pausedByHidden = false;
  let panelOpen = false;

  function onVisibilityChange() {
    if (document.hidden) {
      pausedByHidden = relayEventSource !== null || relayPollingTimer !== null;
      disconnectRelayStream();
    } else if (pausedByHidden && panelOpen) {
      pausedByHidden = false;
      connectRelayStream(panelOpen);
    }
  }

  document.addEventListener("visibilitychange", onVisibilityChange);

  function ensureRelayTaskDisplayOrder(task: CrossTransferRelayTask) {
    const key = String(task.task_id || "");
    if (!key || relayTaskOrderMap.value[key]) return;
    const preferred = Number(task.queue_order || 0);
    const next = preferred > 0 ? preferred : relayTaskOrderCounter + 1;
    relayTaskOrderCounter = Math.max(relayTaskOrderCounter, next);
    relayTaskOrderMap.value = { ...relayTaskOrderMap.value, [key]: next };
  }

  function sortRelayTasksForDisplay(tasks: CrossTransferRelayTask[]) {
    for (const task of tasks) ensureRelayTaskDisplayOrder(task);
    return [...tasks].sort((a, b) => {
      const rankA = relayTaskActivityRank(a);
      const rankB = relayTaskActivityRank(b);
      if (rankA !== rankB) return rankA - rankB;

      const orderA = relayTaskOrderMap.value[a.task_id];
      const orderB = relayTaskOrderMap.value[b.task_id];
      if (orderA && orderB && orderA !== orderB) return orderA - orderB;

      const queueOrderA = Number(a.queue_order || 0);
      const queueOrderB = Number(b.queue_order || 0);
      if (queueOrderA > 0 && queueOrderB > 0 && queueOrderA !== queueOrderB) {
        return queueOrderA - queueOrderB;
      }

      const createdAtA = Number(a.created_at || 0);
      const createdAtB = Number(b.created_at || 0);
      if (createdAtA !== createdAtB) return createdAtA - createdAtB;

      return (
        Number(orderA || Number.MAX_SAFE_INTEGER) - Number(orderB || Number.MAX_SAFE_INTEGER)
      );
    });
  }

  const displayRelayTasks = computed(() => sortRelayTasksForDisplay(relayTasks.value));

  const runningRelayTasks = computed(() =>
    displayRelayTasks.value.filter((task) => ["pending", "running"].includes(task.status)),
  );

  const completedRelayTasks = computed(() =>
    displayRelayTasks.value.filter((task) => ["success", "failed", "canceled"].includes(task.status)),
  );

  const filteredRelayTasks = computed(() =>
    relayTaskView.value === "completed" ? completedRelayTasks.value : runningRelayTasks.value,
  );

  const activeRelayCount = computed(() => runningRelayTasks.value.length);

  function applyRelayTasks(tasks: CrossTransferRelayTask[]) {
    relayTasks.value = Array.isArray(tasks) ? tasks : [];
  }

  async function fetchRelayTasks() {
    try {
      applyRelayTasks(await listCrossTransferRelayTasks());
    } catch (error) {
      console.error("获取跨盘任务失败:", error);
    }
  }

  function stopRelayPolling() {
    if (relayPollingTimer) {
      clearInterval(relayPollingTimer);
      relayPollingTimer = null;
    }
  }

  function startRelayPolling() {
    if (relayPollingTimer) return;
    relayPollingTimer = setInterval(() => {
      void fetchRelayTasks();
    }, 4000);
  }

  function clearRelaySseReconnectTimer() {
    if (relaySseReconnectTimer) {
      clearTimeout(relaySseReconnectTimer);
      relaySseReconnectTimer = null;
    }
  }

  function disconnectRelayStream() {
    clearRelaySseReconnectTimer();
    if (relayEventSource) {
      relayEventSource.close();
      relayEventSource = null;
    }
    stopRelayPolling();
  }

  function scheduleRelayStreamReconnect(panelOpen?: boolean) {
    if (!panelOpen || relaySseReconnectTimer) return;
    relaySseReconnectTimer = setTimeout(() => {
      relaySseReconnectTimer = null;
      connectRelayStream(panelOpen);
    }, 3000);
  }

  function connectRelayStream(panelOpenArg?: boolean) {
    panelOpen = panelOpenArg || false;
    if (!panelOpen || relayEventSource) return;
    if (typeof window === "undefined" || !window.EventSource) {
      startRelayPolling();
      void fetchRelayTasks();
      return;
    }
    stopRelayPolling();
    relayEventSource = new EventSource("/api/cross-transfer/relay/tasks/stream");
    relayEventSource.addEventListener("tasks", (event) => {
      try {
        const payload = JSON.parse((event as MessageEvent).data || "{}");
        applyRelayTasks(payload.tasks || []);
      } catch {}
    });
    relayEventSource.onerror = () => {
      disconnectRelayStream();
      scheduleRelayStreamReconnect(panelOpen);
    };
  }

  async function batchDeleteRelayTasks(taskIds: string[]) {
    if (!taskIds.length) return;
    await deleteCrossTransferRelayTasks(taskIds);
    for (const id of taskIds) {
      if (!relayTaskOrderMap.value[id]) continue;
      const next = { ...relayTaskOrderMap.value };
      delete next[id];
      relayTaskOrderMap.value = next;
    }
    await fetchRelayTasks();
  }

  async function openRelayMonitoring() {
    await fetchRelayTasks();
    connectRelayStream(true);
    if (!relayEventSource) startRelayPolling();
  }

  onUnmounted(() => {
    document.removeEventListener("visibilitychange", onVisibilityChange);
    disconnectRelayStream();
  });

  return {
    relayTasks,
    relayTaskView,
    displayRelayTasks,
    runningRelayTasks,
    completedRelayTasks,
    filteredRelayTasks,
    activeRelayCount,
    fetchRelayTasks,
    connectRelayStream,
    disconnectRelayStream,
    openRelayMonitoring,
    batchDeleteRelayTasks,
  };
}
