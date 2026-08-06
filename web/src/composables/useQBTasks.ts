import { computed, onUnmounted, ref } from "vue";
import { fetchQBDownloads, deleteQBDownloads, type QBDownloadTask } from "@/api/qb";
import { getApiErrorMessage } from "@/api/client";
import { showConfirm } from "@/composables/useConfirm";
import { toast } from "@/composables/useToast";

const activeStatuses = new Set(["pending", "running"]);

export function useQBTasks() {
  const tasks = ref<QBDownloadTask[]>([]);
  const taskView = ref<"running" | "completed">("running");
  const loading = ref(false);
  const refreshing = ref(false);
  const enabled = ref(false);
  let pollTimer: number | undefined;
  let pollEnabled = false;

  const activeTasks = computed(() => tasks.value.filter((task) => activeStatuses.has(task.state)));
  const runningTasks = computed(() =>
    tasks.value.filter((task) => ["pending", "running"].includes(task.state)),
  );
  const completedTasks = computed(() =>
    tasks.value.filter((task) => ["finished", "seeding", "paused"].includes(task.state)),
  );
  const filteredTasks = computed(() =>
    taskView.value === "completed" ? completedTasks.value : runningTasks.value,
  );
  const failedTasks = computed(() => tasks.value.filter((task) => task.state === "error"));
  const successfulTasks = computed(() => tasks.value.filter((task) => task.state === "finished"));

  function resetView() {
    taskView.value = "running";
  }

  async function loadTasks(quiet = false) {
    try {
      const list = await fetchQBDownloads();
      tasks.value = list;
    } catch (e) {
      if (!quiet) toast.error(getApiErrorMessage(e, "加载 qB 下载任务失败"));
    }
  }

  async function refreshTasks() {
    if (refreshing.value) return;
    refreshing.value = true;
    try {
      await loadTasks(true);
    } finally {
      refreshing.value = false;
    }
  }

  async function deleteTask(task: QBDownloadTask) {
    const ok = await showConfirm({
      title: "删除 qB 下载任务",
      message: `确定删除「${task.name}」${task.state === "error" ? "（仅移除任务，不删文件）" : "？"}`,
      confirmText: "删除",
      danger: true,
    });
    if (!ok) return;
    try {
      await deleteQBDownloads({ hashes: [task.hash] });
      await loadTasks(true);
      toast.success("已删除 qB 下载任务");
    } catch (e) {
      toast.error(getApiErrorMessage(e, "删除 qB 下载任务失败"));
    }
  }

  async function batchDelete() {
    const target = filteredTasks.value;
    if (target.length === 0) return;
    const ok = await showConfirm({
      title: taskView.value === "completed" ? "清空已完成任务" : "删除当前任务",
      message: `确定删除 ${target.length} 个 qB 下载任务？`,
      confirmText: "删除",
      danger: true,
    });
    if (!ok) return;
    try {
      await deleteQBDownloads({ hashes: target.map((task) => task.hash) });
      await loadTasks(true);
      toast.success(`已删除 ${target.length} 个 qB 下载任务`);
    } catch (e) {
      toast.error(getApiErrorMessage(e, "批量删除 qB 下载任务失败"));
    }
  }

  function statusText(task: QBDownloadTask): string {
    const map: Record<string, string> = {
      pending: "排队中",
      running: "下载中",
      seeding: "做种中",
      paused: "已暂停",
      error: "错误",
      finished: "已完成",
    };
    return map[task.state] || task.state;
  }

  function startPolling() {
    if (pollEnabled) return;
    pollEnabled = true;
    pollTimer = window.setInterval(() => {
      if (pollEnabled) void loadTasks(true);
    }, 5000);
  }

  function stopPolling() {
    pollEnabled = false;
    if (pollTimer) {
      window.clearInterval(pollTimer);
      pollTimer = undefined;
    }
  }

  onUnmounted(stopPolling);

  return {
    tasks,
    taskView,
    loading,
    refreshing,
    enabled,
    activeTasks,
    runningTasks,
    completedTasks,
    filteredTasks,
    failedTasks,
    successfulTasks,
    resetView,
    loadTasks,
    refreshTasks,
    deleteTask,
    batchDelete,
    statusText,
    startPolling,
    stopPolling,
  };
}
