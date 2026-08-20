import { computed, onUnmounted, ref, type Ref } from "vue";
import { offlineDownloadApi } from "@/api/offlineDownload";
import { getApiErrorMessage } from "@/api/client";
import { showConfirm } from "@/composables/useConfirm";
import { toast } from "@/composables/useToast";
import type { OfflineDownloadCapabilities, OfflineDownloadTask } from "@/types/offline-download";

type Deps = {
  selectedAccountId: Ref<number | null>;
  currentParentId: Ref<string>;
  refreshFiles: () => Promise<void>;
};

const activeStatuses = new Set(["pending", "running", "retrying"]);

function sameParent(left: string, right: string) {
  const normalize = (value: string) => (!value || value === "0" ? "" : value);
  return normalize(left) === normalize(right);
}

export function useOfflineDownloads(deps: Deps) {
  const capability = ref<OfflineDownloadCapabilities | null>(null);
  const capabilityLoading = ref(false);
  const modalOpen = ref(false);
  const tasks = ref<OfflineDownloadTask[]>([]);
  const taskView = ref<"running" | "completed">("running");
  const loading = ref(false);
  const refreshing = ref(false);
  let pollTimer: number | undefined;
  let capabilityRequest = 0;

  const activeTasks = computed(() => tasks.value.filter((task) => activeStatuses.has(task.status)));
  const runningTasks = computed(() =>
    tasks.value.filter((task) => activeStatuses.has(task.status) || task.status === "failed"),
  );
  const completedTasks = computed(() => tasks.value.filter((task) => task.status === "success"));
  const filteredTasks = computed(() =>
    taskView.value === "completed" ? completedTasks.value : runningTasks.value,
  );
  const failedTasks = computed(() => tasks.value.filter((task) => task.status === "failed"));
  const successfulTasks = computed(() => tasks.value.filter((task) => task.status === "success"));
  const deletableTasks = computed(() =>
    filteredTasks.value.filter((task) => !activeStatuses.has(task.status) || task.remote_delete),
  );

  async function loadCapability(accountId = deps.selectedAccountId.value) {
    const request = ++capabilityRequest;
    capability.value = null;
    if (!accountId) {
      capabilityLoading.value = false;
      return;
    }
    capabilityLoading.value = true;
    try {
      const next = await offlineDownloadApi.capabilities(accountId);
      if (request === capabilityRequest) capability.value = next;
    } catch {
      if (request === capabilityRequest) capability.value = null;
    } finally {
      if (request === capabilityRequest) capabilityLoading.value = false;
    }
  }

  function openModal() {
    if (!deps.selectedAccountId.value) {
      toast.info("请先选择一个账号");
      return;
    }
    if (!capability.value?.supported) {
      toast.info("当前网盘不支持离线下载");
      return;
    }
    modalOpen.value = true;
  }

  function closeModal() {
    modalOpen.value = false;
  }

  function registerTasks(created: OfflineDownloadTask[]) {
    const byId = new Map(tasks.value.map((task) => [task.task_id, task]));
    for (const task of created) byId.set(task.task_id, task);
    tasks.value = [...byId.values()].sort((a, b) => b.created_at - a.created_at);
    taskView.value = "running";
    ensurePolling();
  }

  function registerTask(task: OfflineDownloadTask) {
    registerTasks([task]);
  }

  async function replaceTasks(next: OfflineDownloadTask[]) {
    const before = new Map(tasks.value.map((task) => [task.task_id, task.status]));
    tasks.value = next;
    const affectsCurrent = next.some(
      (task) =>
        before.get(task.task_id) !== "success" &&
        task.status === "success" &&
        task.account_id === deps.selectedAccountId.value &&
        sameParent(task.target_parent_id, deps.currentParentId.value),
    );
    if (affectsCurrent) await deps.refreshFiles();
  }

  async function fetchTasks(refresh = true, quiet = false) {
    if (!quiet) loading.value = true;
    try {
      const next = await offlineDownloadApi.listTasks(refresh);
      await replaceTasks(next);
      ensurePolling();
    } catch (error) {
      if (!quiet) toast.error(getApiErrorMessage(error, "离线下载任务加载失败"));
    } finally {
      if (!quiet) loading.value = false;
    }
  }

  async function refreshTasks() {
    if (refreshing.value) return;
    refreshing.value = true;
    try {
      await replaceTasks(await offlineDownloadApi.refreshTasks());
    } catch (error) {
      toast.error(getApiErrorMessage(error, "离线下载任务刷新失败"));
    } finally {
      refreshing.value = false;
      ensurePolling();
    }
  }

  async function deleteTask(task: OfflineDownloadTask) {
    const active = activeStatuses.has(task.status);
    if (active && !task.remote_delete) {
      toast.info("当前网盘官方接口不能取消进行中的离线任务");
      return;
    }
    const result = await showConfirm({
      title: active ? "取消离线下载任务" : "删除任务记录",
      message: task.name,
      hint: active
        ? "这会同时删除网盘端正在执行的离线任务，但不会删除已经下载的文件。"
        : task.remote_delete
          ? "这会同步删除网盘端的任务历史，但不会删除已经下载的文件。"
          : "这里只删除 LitePan 中的任务记录，不会删除网盘文件。",
      icon: "trash",
      confirmText: active ? "取消任务" : "删除记录",
      cancelText: "保留",
      danger: true,
    }).catch(() => null);
    if (result?.action !== "confirm") return;
    try {
      await offlineDownloadApi.deleteTask(task.task_id);
      tasks.value = tasks.value.filter((item) => item.task_id !== task.task_id);
      toast.success(active ? "离线下载任务已取消" : "任务记录已删除");
    } catch (error) {
      toast.error(getApiErrorMessage(error, "离线下载任务删除失败"));
    }
  }

  async function batchDelete() {
    const target = deletableTasks.value;
    if (!target.length) return;
    const result = await showConfirm({
      title: taskView.value === "completed" ? "清空已完成任务" : "删除当前任务",
      message: `将处理 ${target.length} 条离线下载任务。`,
      hint: "115 任务会同步删除网盘端任务历史；已下载文件不会被删除。",
      icon: "trash",
      confirmText: "确认删除",
      cancelText: "取消",
      danger: true,
    }).catch(() => null);
    if (result?.action !== "confirm") return;
    try {
      const deleted = await offlineDownloadApi.batchDelete(target.map((task) => task.task_id));
      const ids = new Set(deleted.deleted_task_ids);
      tasks.value = tasks.value.filter((task) => !ids.has(task.task_id));
      if (deleted.failed_task_ids.length) toast.warning(`${deleted.failed_task_ids.length} 个任务未能删除`);
      else toast.success("离线下载任务已清理");
    } catch (error) {
      toast.error(getApiErrorMessage(error, "批量删除离线下载任务失败"));
    }
  }

  // 轮询节奏：10s 低频 + 后台标签页暂停；refresh=false 只读任务状态，避免每 10s 强制驱动刷新。
  function ensurePolling() {
    if (pollTimer !== undefined) window.clearTimeout(pollTimer);
    pollTimer = undefined;
    if (document.hidden) return;
    if (activeTasks.value.length === 0) return;
    pollTimer = window.setTimeout(async () => {
      pollTimer = undefined;
      await fetchTasks(false, true);
    }, 10000);
  }

  function onOfflineVisibilityChange() {
    if (document.hidden) {
      if (pollTimer !== undefined) window.clearTimeout(pollTimer);
      pollTimer = undefined;
    } else if (activeTasks.value.length > 0) {
      void fetchTasks(false, true);
    }
  }
  document.addEventListener("visibilitychange", onOfflineVisibilityChange);

  function statusText(task: OfflineDownloadTask) {
    switch (task.status) {
      case "pending": return "等待中";
      case "running": return "下载中";
      case "retrying": return "重试中";
      case "success": return "已完成";
      case "failed": return "失败";
      default: return task.status;
    }
  }

  function sourceLabel(task: OfflineDownloadTask) {
    if (task.source_kind === "bt") return "BT";
    try {
      return new URL(task.source).protocol.replace(":", "").toUpperCase();
    } catch {
      return "链接";
    }
  }

  onUnmounted(() => {
    if (pollTimer !== undefined) window.clearTimeout(pollTimer);
    document.removeEventListener("visibilitychange", onOfflineVisibilityChange);
  });

  return {
    capability,
    capabilityLoading,
    modalOpen,
    tasks,
    taskView,
    loading,
    refreshing,
    activeTasks,
    runningTasks,
    completedTasks,
    filteredTasks,
    failedTasks,
    successfulTasks,
    deletableTasks,
    loadCapability,
    openModal,
    closeModal,
    registerTasks,
    registerTask,
    fetchTasks,
    refreshTasks,
    deleteTask,
    batchDelete,
    statusText,
    sourceLabel,
  };
}

export type OfflineDownloads = ReturnType<typeof useOfflineDownloads>;
