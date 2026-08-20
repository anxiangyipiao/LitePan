import { uploadApi } from "@/api/upload";
import { getUploadTaskStableKey, isLocalUploadTask } from "@/composables/upload/uploadTaskFormatters";
import type { UploadRuntimeHooks, UploadTaskDeps } from "@/composables/upload/uploadTaskTypes";
import type { UploadTaskStore } from "@/composables/upload/useUploadTaskStore";
import type { UploadTask } from "@/types/upload";

export function useUploadTaskStream(deps: UploadTaskDeps, store: UploadTaskStore, hooks: UploadRuntimeHooks) {
  let uploadTaskPollingTimer: ReturnType<typeof setInterval> | null = null;
  let uploadTaskEventSource: EventSource | null = null;
  let uploadTaskSseReconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let uploadTaskPollIntervalMs = 1000;
  const refreshedSuccessfulTaskKeys = new Set<string>();
  let pausedByHidden = false;

  function onVisibilityChange() {
    if (document.hidden) {
      pausedByHidden = uploadTaskEventSource !== null || uploadTaskPollingTimer !== null;
      disconnectUploadTaskStream();
      stopUploadTaskPolling();
    } else if (pausedByHidden) {
      pausedByHidden = false;
      if (store.uploadTaskPanelOpen.value) {
        connectUploadTaskStream();
      }
    }
  }

  document.addEventListener("visibilitychange", onVisibilityChange);

  function refreshCurrentDirectoryForNewSuccess(tasks: UploadTask[]) {
    const currentSuccessKeys = new Set<string>();
    let hasNewSuccess = false;
    for (const task of tasks) {
      if (!store.uploadAffectsCurrentDirectory(task, deps.currentPath.value)) continue;
      const key = getUploadTaskStableKey(task);
      if (!key) continue;
      currentSuccessKeys.add(key);
      if (!refreshedSuccessfulTaskKeys.has(key)) {
        refreshedSuccessfulTaskKeys.add(key);
        hasNewSuccess = true;
      }
    }
    for (const key of refreshedSuccessfulTaskKeys) {
      if (!currentSuccessKeys.has(key)) refreshedSuccessfulTaskKeys.delete(key);
    }
    if (hasNewSuccess || store.consumeFolderUploadRefreshPending()) {
      void deps.refreshFiles(true);
    }
  }

  async function refreshUploadTaskServerConcurrency() {
    try {
      const data = await uploadApi.getRuntime();
      const limit = Number(data.concurrency || 3);
      store.uploadTaskServerConcurrency.value = Number.isFinite(limit) && limit > 0 ? limit : 3;
    } catch {
      store.uploadTaskServerConcurrency.value = 3;
    }
  }

  async function fetchUploadTasks() {
    try {
      store.uploadTasks.value = await uploadApi.listTasks();
      store.uploadTasks.value.forEach(store.ensureUploadTaskDisplayOrder);
      refreshCurrentDirectoryForNewSuccess(store.uploadTasks.value);
      await hooks.startScheduler();
    } catch (e) {
      console.error("获取上传任务失败:", e);
    } finally {
      adaptUploadTaskPolling();
    }
  }

  // 轮询节奏自适应：有活跃任务 1s 快速刷新；面板开着但空闲 3s 低频保活；否则停止。
  function adaptUploadTaskPolling() {
    const active = store.activeUploadTasks.value.length > 0;
    const panelOpen = store.uploadTaskPanelOpen.value;
    const target = active ? 1000 : panelOpen ? 3000 : 0;
    if (target === 0) {
      stopUploadTaskPolling();
      return;
    }
    if (uploadTaskPollingTimer && target === uploadTaskPollIntervalMs) return;
    uploadTaskPollIntervalMs = target;
    stopUploadTaskPolling();
    startUploadTaskPolling();
  }

  function startUploadTaskPolling() {
    if (uploadTaskPollingTimer) return;
    uploadTaskPollingTimer = setInterval(() => void fetchUploadTasks(), uploadTaskPollIntervalMs);
  }

  function stopUploadTaskPolling() {
    if (uploadTaskPollingTimer) {
      clearInterval(uploadTaskPollingTimer);
      uploadTaskPollingTimer = null;
    }
  }

  function connectUploadTaskStream() {
    if (!store.uploadTaskPanelOpen.value || uploadTaskEventSource) return;
    if (typeof EventSource === "undefined") {
      startUploadTaskPolling();
      return;
    }
    const es = new EventSource("/api/files/upload/tasks/stream");
    uploadTaskEventSource = es;
    es.addEventListener("tasks", (ev) => {
      try {
        const payload = JSON.parse(ev.data || "{}") as { tasks?: UploadTask[] };
        store.uploadTasks.value = payload.tasks || [];
        store.uploadTasks.value.forEach(store.ensureUploadTaskDisplayOrder);
        refreshCurrentDirectoryForNewSuccess(store.uploadTasks.value);
      } catch (e) {
        console.error(e);
      }
    });
    es.onopen = () => stopUploadTaskPolling();
    es.onerror = () => {
      disconnectUploadTaskStream();
      startUploadTaskPolling();
      if (!uploadTaskSseReconnectTimer) {
        uploadTaskSseReconnectTimer = setTimeout(() => {
          uploadTaskSseReconnectTimer = null;
          connectUploadTaskStream();
        }, 3000);
      }
    };
  }

  function disconnectUploadTaskStream() {
    if (uploadTaskSseReconnectTimer) {
      clearTimeout(uploadTaskSseReconnectTimer);
      uploadTaskSseReconnectTimer = null;
    }
    uploadTaskEventSource?.close();
    uploadTaskEventSource = null;
  }

  function cleanupUploadTasks() {
    document.removeEventListener("visibilitychange", onVisibilityChange);
    store.localUploadTaskControllers.forEach((c) => c.abort());
    store.localUploadTaskControllers.clear();
    store.localUploadTaskPayloads.clear();
    disconnectUploadTaskStream();
    stopUploadTaskPolling();
  }

  return {
    fetchUploadTasks,
    refreshUploadTaskServerConcurrency,
    startUploadTaskPolling,
    stopUploadTaskPolling,
    connectUploadTaskStream,
    disconnectUploadTaskStream,
    cleanupUploadTasks,
  };
}

export type UploadTaskStream = ReturnType<typeof useUploadTaskStream>;

export function getActiveUploadSlotUsage(store: UploadTaskStore) {
  return (
    store.localDispatchingTaskIds.size +
    store.uploadTasks.value.filter((t) => {
      if (t.status === "running") return true;
      if (t.status === "pending") return !store.pendingRemoteResumeTaskIds.has(String(t.task_id));
      return false;
    }).length
  );
}

export function getNextUploadTaskCandidate(store: UploadTaskStore) {
  return (
    store.displayUploadTasks.value.find((task) => {
      if (store.hiddenUploadTaskKeys.has(getUploadTaskStableKey(task))) return false;
      if (isLocalUploadTask(task)) {
        return (
          task.status === "pending" &&
          !store.localDispatchingTaskIds.has(task.task_id) &&
          !store.canceledLocalUploadTaskIds.has(task.task_id) &&
          !store.pausedLocalUploadTaskIds.has(task.task_id) &&
          Boolean(store.localUploadTaskPayloads.get(task.task_id)?.file)
        );
      }
      return (
        store.pendingRemoteResumeTaskIds.has(String(task.task_id)) &&
        ["paused", "failed", "canceled"].includes(task.status)
      );
    }) || null
  );
}
