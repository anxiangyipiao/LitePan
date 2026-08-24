<script setup lang="ts">
import {
  computed,
  nextTick,
  onActivated,
  onDeactivated,
  onMounted,
  onUnmounted,
  reactive,
  ref,
  watch,
} from "vue";
import { storeToRefs } from "pinia";
import { getApiErrorMessage } from "@/api/client";
import {
  applyMediaOrganizeTask,
  batchDeleteMediaOrganizePlanActions,
  createMediaOrganizeTask,
  deleteMediaOrganizePlanAction,
  deleteMediaOrganizeTask,
  fetchMediaOrganizeLogs,
  fetchMediaOrganizePlan,
  fetchMediaOrganizeProgress,
  fetchMediaOrganizeTasks,
  planMediaOrganizeTask,
  stopMediaOrganizeTask,
  testMediaOrganizeTmdb,
  updateMediaOrganizePlanAction,
  updateMediaOrganizeTask,
  type MediaOrganizeLogEntry,
  type MediaOrganizePlan,
  type MediaOrganizePlanAction,
  type MediaOrganizeProgress,
  type MediaOrganizeTask,
  type MediaOrganizeTaskInput,
} from "@/api/mediaOrganize";
import AccountFolderField from "@/components/admin/AccountFolderField.vue";
import AdminEmptyState from "@/components/admin/AdminEmptyState.vue";
import AdminRunStatusCell from "@/components/admin/AdminRunStatusCell.vue";
import AdminTableActionBtn from "@/components/admin/AdminTableActionBtn.vue";
import AdminRowActions from "@/components/admin/AdminRowActions.vue";
import type { AdminRunStatusVariant } from "@/components/admin/adminRunStatus";
import AdminStatsGrid from "@/components/admin/AdminStatsGrid.vue";
import FormField from "@/components/base/FormField.vue";
import AppButton from "@/components/base/AppButton.vue";
import AppIconButton from "@/components/base/AppIconButton.vue";
import AppInput from "@/components/base/AppInput.vue";
import AppModal from "@/components/base/AppModal.vue";
import BusySpinner from "@/components/base/BusySpinner.vue";
import AppSelect from "@/components/base/AppSelect.vue";
import StatCard from "@/components/base/StatCard.vue";
import SettingsHelpTooltip from "@/components/admin/SettingsHelpTooltip.vue";
import FolderPickerModal from "@/components/file/FolderPickerModal.vue";
import { useAccountPathLabel } from "@/composables/useAccountPathLabel";
import { useAdminPageLoading } from "@/composables/useAdminLoadingBar";
import { useConditionalPolling } from "@/composables/useConditionalPolling";
import { useOrganizePlanPreview, planActionMeta, type PlanGroup } from "@/composables/useOrganizePlanPreview";
import { confirm } from "@/composables/useConfirm";
import { toast } from "@/composables/useToast";
import { useAccountsStore } from "@/stores/accounts";
import "@/styles/admin-shared.css";
import "@/styles/admin-table.css";

const accountsStore = useAccountsStore();
const { accounts } = storeToRefs(accountsStore);

withDefaults(
  defineProps<{
    hideStats?: boolean;
  }>(),
  { hideStats: false },
);

const boolOptions = [
  { value: "true", label: "开启" },
  { value: "false", label: "关闭" },
];

const mediaTypeOptions = [
  { value: "auto", label: "自动检测" },
  { value: "movie", label: "电影" },
  { value: "tv", label: "剧集" },
];


type TaskForm = {
  task_name: string;
  account_id: number;
  target_directory: string;
  target_directory_id: string;
  action_type: string;
  target_root: string;
  target_root_id: string;
  media_type: string;
  rename_marker: string;
  use_tmdb: string;
  recursive: boolean;
  scatter_movie_per_file: string;
};

const emptyForm = (): TaskForm => ({
  task_name: "",
  account_id: 0,
  target_directory: "",
  target_directory_id: "",
  action_type: "move",
  target_root: "",
  target_root_id: "",
  media_type: "auto",
  rename_marker: "",
  use_tmdb: "true",
  recursive: true,
  scatter_movie_per_file: "true",
});

const tasks = ref<MediaOrganizeTask[]>([]);
const refreshing = ref(false);
const listReady = ref(false);
useAdminPageLoading(
  "tasks",
  computed(() => (!listReady.value || refreshing.value) && !tasks.value.length),
);
const dialogOpen = ref(false);
const editingId = ref<string | null>(null);
const submitting = ref(false);
const form = reactive(emptyForm());
const sourcePickerOpen = ref(false);
const rootPickerOpen = ref(false);

const logTaskId = ref<string | null>(null);
const logTaskName = ref("");
const logs = ref<MediaOrganizeLogEntry[]>([]);
const logBodyRef = ref<HTMLElement | null>(null);
const logAutoCloseOnFinish = ref(false);
let logPollTimer: number | null = null;
let logAutoCloseTimer: number | null = null;

const planOpen = ref(false);
const planTaskId = ref<string | null>(null);
const planTaskName = ref("");
const planLoading = ref(false);
const planApplying = ref(false);
const planProgress = ref<MediaOrganizeProgress>({});
const planEditingId = ref<string | null>(null);
const planEditingName = ref("");
const planEditingSaving = ref(false);
const tmdbTesting = ref(false);
let planProgressTimer: number | null = null;

const preview = useOrganizePlanPreview();

const activeAccounts = computed(() => accounts.value.filter((a) => a.is_active));

const runningCount = computed(() => tasks.value.filter((t) => isTaskActive(t)).length);
const errorTaskCount = computed(
  () => tasks.value.filter((t) => (t.last_run_result?.failed || 0) > 0).length,
);
const taskCount = computed(() => tasks.value.length);

function accountName(id: number): string {
  return accounts.value.find((a) => a.id === id)?.name ?? `#${id}`;
}

const { display: sourceDirDisplay, title: sourceDirTitle } = useAccountPathLabel({
  accountId: computed(() => form.account_id),
  path: computed(() => form.target_directory),
  accounts,
});

const rootDirDisplay = computed(() => (form.target_root || "").trim());

const rootDirTitle = computed(() => rootDirDisplay.value || "点击浏览选择目录");

function isTaskActive(task: MediaOrganizeTask): boolean {
  return ["running", "stopping", "planning"].includes(task.status);
}

function organizeStatusVariant(task: MediaOrganizeTask): AdminRunStatusVariant {
  if (isTaskActive(task)) return "running";
  if (task.last_run_result && (task.last_run_result.failed || 0) > 0) return "error";
  if (task.last_run_result) return "success";
  return "pending";
}

function statusText(task: MediaOrganizeTask): string {
  if (task.status === "stopping") return "停止中";
  if (task.status === "planning") return "生成计划中";
  if (task.status === "running") return "执行中";
  if (task.last_run_result?.stopped) return "已停止";
  if (task.last_run_result && (task.last_run_result.failed || 0) > 0) return "有失败";
  if (task.last_run_result) return "已完成";
  return "未执行";
}

function statusTitle(task: MediaOrganizeTask): string {
  const result = task.last_run_result;
  if (!result) {
    if (task.status === "stopping") return "任务正在停止，当前操作完成后退出";
    return isTaskActive(task) ? "任务正在执行" : "任务尚未执行";
  }
  if (result.stopped) {
    return `已停止：总数 ${result.total || 0}，改名 ${result.renamed || 0}，移动 ${result.moved || 0}，跳过 ${result.skipped || 0}，失败 ${result.failed || 0}`;
  }
  return `总数 ${result.total || 0}，改名 ${result.renamed || 0}，移动 ${result.moved || 0}，跳过 ${result.skipped || 0}，失败 ${result.failed || 0}`;
}

function resultSummary(task: MediaOrganizeTask): string {
  const r = task.last_run_result;
  if (!r) return "";
  return `${r.total || 0} 项 · 改${r.renamed || 0} · 移${r.moved || 0} · 失${r.failed || 0}`;
}

function hasActiveTasks(): boolean {
  return tasks.value.some((t) => isTaskActive(t));
}

const taskListPolling = useConditionalPolling({
  intervalMs: 4000,
  onTick: () => loadTasks(true),
  shouldPoll: hasActiveTasks,
});

async function loadTasks(quiet = false) {
  if (!quiet) refreshing.value = true;
  try {
    tasks.value = await fetchMediaOrganizeTasks();
    syncTaskListPolling();
  } catch (e) {
    if (!quiet) toast.error(getApiErrorMessage(e, "加载整理任务失败"));
  } finally {
    if (!quiet) refreshing.value = false;
    listReady.value = true;
  }
}

function syncTaskListPolling() {
  taskListPolling.sync();
}

function resetForm() {
  Object.assign(form, emptyForm());
}

function openCreate() {
  editingId.value = null;
  resetForm();
  dialogOpen.value = true;
}

function openEdit(task: MediaOrganizeTask) {
  editingId.value = task.id;
  const cfg = task.config || {};
  Object.assign(form, {
    task_name: task.task_name,
    account_id: task.account_id,
    target_directory: cfg.target_directory || "",
    target_directory_id: cfg.target_directory_id || "",
    action_type: cfg.action_type || "move",
    target_root: cfg.target_root || "",
    target_root_id: cfg.target_root_id || "",
    media_type: cfg.media_type || "auto",
    rename_marker: cfg.rename_marker || "",
    use_tmdb: cfg.use_tmdb !== false ? "true" : "false",
    recursive: cfg.recursive !== false,
    scatter_movie_per_file: cfg.scatter_movie_per_file !== false ? "true" : "false",
  });
  dialogOpen.value = true;
}

function switchActionType(type: string) {
  form.action_type = type;
  if (type === "rename" && !form.rename_marker.trim()) form.rename_marker = "tmdb";
}

function openRootPicker() {
  if (!form.account_id) {
    toast.error("请先选择整理目录");
    return;
  }
  rootPickerOpen.value = true;
}

function onSourceFolderPicked(payload: { accountId: number; parentId: string; path: string }) {
  form.account_id = payload.accountId;
  form.target_directory = payload.path || "/";
  form.target_directory_id = payload.parentId;
  sourcePickerOpen.value = false;
}

function onRootFolderPicked(payload: { accountId: number; parentId: string; path: string }) {
  form.account_id = payload.accountId;
  form.target_root = payload.path || "/";
  form.target_root_id = payload.parentId;
  rootPickerOpen.value = false;
}

function buildPayload(): MediaOrganizeTaskInput {
  return {
    task_name: form.task_name.trim(),
    account_id: form.account_id,
    target_directory: form.target_directory,
    target_directory_id: form.target_directory_id,
    action_type: form.action_type,
    target_root: form.target_root,
    target_root_id: form.target_root_id,
    media_type: form.media_type,
    rename_marker: form.rename_marker,
    use_tmdb: form.use_tmdb === "true",
    recursive: form.recursive,
    scatter_movie_per_file: form.scatter_movie_per_file === "true",
  };
}

async function submitTask() {
  if (!form.task_name.trim()) {
    toast.error("请输入任务名称");
    return;
  }
  if (!form.account_id || !form.target_directory.trim()) {
    toast.error("请选择整理目录");
    return;
  }
  if (form.action_type === "move" && !form.target_root.trim()) {
    toast.error("移动模式下目标根目录不能为空");
    return;
  }
  if (form.action_type === "rename" && !form.rename_marker.trim()) {
    toast.error("原地重命名必须设置标识：tmdb / 自定义 / off");
    return;
  }
  submitting.value = true;
  try {
    const body = buildPayload();
    if (editingId.value) {
      await updateMediaOrganizeTask(editingId.value, body);
      toast.success("任务已更新");
    } else {
      await createMediaOrganizeTask(body);
      toast.success("任务已创建");
    }
    dialogOpen.value = false;
    await loadTasks();
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存任务失败"));
  } finally {
    submitting.value = false;
  }
}

async function handleDelete(task: MediaOrganizeTask) {
  try {
    await confirm({
      title: "删除整理任务",
      message: `确定删除任务「${task.task_name}」吗？`,
      icon: "trash",
      confirmText: "删除",
      danger: true,
    });
  } catch {
    return;
  }
  try {
    const result = await deleteMediaOrganizeTask(task.id);
    if (result.stopping) {
      toast.info("任务正在执行，已请求停止");
      if (logTaskId.value === task.id) startLogPolling(task.id);
      await loadTasks();
      return;
    }
    toast.success("任务已删除");
    if (logTaskId.value === task.id) closeLogPanel();
    await loadTasks();
  } catch (e) {
    toast.error(getApiErrorMessage(e, "删除失败"));
  }
}

async function handleStop(task: MediaOrganizeTask) {
  const idx = tasks.value.findIndex((t) => t.id === task.id);
  if (idx >= 0) tasks.value[idx] = { ...tasks.value[idx], status: "stopping" };
  if (logTaskId.value === task.id) {
    void loadLogs(task.id);
    startLogPolling(task.id);
  }
  try {
    await stopMediaOrganizeTask(task.id);
    toast.info("已请求停止");
    await loadTasks();
  } catch (e) {
    toast.error(getApiErrorMessage(e, "停止失败"));
    await loadTasks();
  }
}

function patchTaskFromLogs(taskId: string, status?: string, lastRunResult?: MediaOrganizeTask["last_run_result"]) {
  const idx = tasks.value.findIndex((t) => t.id === taskId);
  if (idx < 0) return;
  const next = { ...tasks.value[idx] };
  let dirty = false;
  if (status && next.status !== status) {
    next.status = status;
    dirty = true;
  }
  if (lastRunResult !== undefined) {
    next.last_run_result = lastRunResult;
    dirty = true;
  }
  if (dirty) {
    const arr = [...tasks.value];
    arr.splice(idx, 1, next);
    tasks.value = arr;
  }
}

async function loadLogs(taskId: string) {
  try {
    const data = await fetchMediaOrganizeLogs(taskId);
    logs.value = data.logs || [];
    patchTaskFromLogs(taskId, data.status, data.last_run_result);
    await nextTick();
    if (logBodyRef.value) logBodyRef.value.scrollTop = logBodyRef.value.scrollHeight;
    if (!["running", "planning", "stopping"].includes(data.status)) {
      stopLogPolling();
      if (logAutoCloseOnFinish.value && logTaskId.value === taskId) {
        scheduleLogPanelAutoClose();
      }
    }
  } catch {}
}

function openLogPanel(task: MediaOrganizeTask, options?: { autoCloseOnFinish?: boolean }) {
  clearLogAutoCloseTimer();
  logTaskId.value = task.id;
  logTaskName.value = task.task_name;
  logs.value = [];
  logAutoCloseOnFinish.value = options?.autoCloseOnFinish ?? isTaskActive(task);
  void loadLogs(task.id);
  startLogPolling(task.id);
}

function closeLogPanel() {
  clearLogAutoCloseTimer();
  stopLogPolling();
  logTaskId.value = null;
  logTaskName.value = "";
  logs.value = [];
  logAutoCloseOnFinish.value = false;
}

function scheduleLogPanelAutoClose() {
  clearLogAutoCloseTimer();
  logAutoCloseTimer = window.setTimeout(() => {
    if (logTaskId.value) closeLogPanel();
  }, 1500);
}

function clearLogAutoCloseTimer() {
  if (logAutoCloseTimer) {
    window.clearTimeout(logAutoCloseTimer);
    logAutoCloseTimer = null;
  }
}

function startLogPolling(taskId: string) {
  stopLogPolling();
  logPollTimer = window.setInterval(() => {
    void loadLogs(taskId);
  }, 1000);
}

function stopLogPolling() {
  if (logPollTimer) {
    window.clearInterval(logPollTimer);
    logPollTimer = null;
  }
}

function startPlanProgressPolling(taskId: string, successMessage?: string) {
  stopPlanProgressPolling();
  planProgress.value = {};
  let seenActivity = false;
  const tick = async () => {
    try {
      planProgress.value = await fetchMediaOrganizeProgress(taskId);
    } catch {}
    const stage = planProgress.value.stage;
    const status = planProgress.value.status;
    if (stage === "done" || stage === "error") {
      stopPlanProgressPolling();
      planLoading.value = false;
      if (stage === "done") {
        void loadPlanAfterGenerate(taskId, successMessage);
      } else {
        toast.error("计划生成失败，请查看任务日志");
      }
      return;
    }
    if (stage === "planning" || (status && ["planning", "running", "stopping"].includes(status))) {
      seenActivity = true;
      return;
    }
    // 任务已结束但未产出 done/error（如后台执行任务结束、或生成被中止）：停止轮询，避免无限等待。
    if (seenActivity && status && !["planning", "running", "stopping"].includes(status)) {
      stopPlanProgressPolling();
      planLoading.value = false;
    }
  };
  void tick();
  planProgressTimer = window.setInterval(tick, 1200);
}

async function loadPlanAfterGenerate(taskId: string, successMessage?: string) {
  try {
    const plan = await fetchMediaOrganizePlan(taskId);
    preview.loadPlan(plan);
    if (successMessage) toast.success(successMessage);
  } catch (e) {
    toast.error(getApiErrorMessage(e, "读取计划失败"));
  }
}

async function requestPlanAndPoll(taskId: string, successMessage?: string) {
  try {
    await planMediaOrganizeTask(taskId);
  } catch (e) {
    const msg = getApiErrorMessage(e, "");
    // 已在后台生成/执行中：改为等待其结束，避免误报失败
    if (!/正在执行/.test(msg)) throw e;
  }
  startPlanProgressPolling(taskId, successMessage);
}

function stopPlanProgressPolling() {
  if (planProgressTimer) {
    window.clearInterval(planProgressTimer);
    planProgressTimer = null;
  }
}

async function previewPlan(task: MediaOrganizeTask) {
  planTaskId.value = task.id;
  planTaskName.value = task.task_name;
  preview.loadPlan(null);
  planOpen.value = true;
  planLoading.value = true;
  try {
    let existing: MediaOrganizePlan | null = null;
    try {
      existing = await fetchMediaOrganizePlan(task.id);
    } catch {
      existing = null;
    }
    if (existing && ((existing.actions?.length ?? 0) > 0 || (existing.skipped?.length ?? 0) > 0)) {
      preview.loadPlan(existing);
      planLoading.value = false;
      return;
    }
    await requestPlanAndPoll(task.id);
  } catch (e) {
    stopPlanProgressPolling();
    planLoading.value = false;
    toast.error(getApiErrorMessage(e, "计划生成失败"));
  }
}

async function refreshPlan() {
  if (!planTaskId.value) return;
  planLoading.value = true;
  try {
    await requestPlanAndPoll(planTaskId.value, "计划已重新生成");
  } catch (e) {
    stopPlanProgressPolling();
    planLoading.value = false;
    toast.error(getApiErrorMessage(e, "生成失败"));
  }
}

function closePlanDialog() {
  planOpen.value = false;
  planTaskId.value = null;
  planTaskName.value = "";
  preview.loadPlan(null);
  planEditingId.value = null;
  planEditingName.value = "";
  planLoading.value = false;
  stopPlanProgressPolling();
}

async function applyPlan() {
  if (!planTaskId.value) return;
  planApplying.value = true;
  try {
    await applyMediaOrganizeTask(planTaskId.value);
    toast.success("计划已开始执行，可在日志中查看进度");
    const taskId = planTaskId.value;
    closePlanDialog();
    const task = tasks.value.find((t) => t.id === taskId);
    if (task) openLogPanel(task, { autoCloseOnFinish: true });
    await loadTasks();
  } catch (e) {
    toast.error(getApiErrorMessage(e, "执行失败"));
  } finally {
    planApplying.value = false;
  }
}

async function testTmdb() {
  tmdbTesting.value = true;
  try {
    const result = await testMediaOrganizeTmdb();
    if (result.ok) toast.success("TMDB 连通正常");
    else toast.error("TMDB 不可达");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "TMDB 测试失败"));
  } finally {
    tmdbTesting.value = false;
  }
}

function startPlanActionEdit(action: MediaOrganizePlanAction) {
  planEditingId.value = action.id;
  planEditingName.value = action.target_name || "";
}

function cancelPlanActionEdit() {
  planEditingId.value = null;
  planEditingName.value = "";
}

async function commitPlanActionEdit(action: MediaOrganizePlanAction) {
  const taskId = planTaskId.value;
  const newName = planEditingName.value.trim();
  if (!taskId || !newName) {
    toast.error("目标名不能为空");
    return;
  }
  if (newName === action.target_name) {
    cancelPlanActionEdit();
    return;
  }
  planEditingSaving.value = true;
  try {
    const result = await updateMediaOrganizePlanAction(taskId, action.id, newName);
    action.target_name = result.action?.target_name || newName;
    if (!action.metadata) action.metadata = {};
    action.metadata.edited = true;
    cancelPlanActionEdit();
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存失败"));
  } finally {
    planEditingSaving.value = false;
  }
}

async function removePlanAction(action: MediaOrganizePlanAction) {
  const taskId = planTaskId.value;
  if (!taskId) return;
  try {
    await confirm({
      title: "从计划中移除",
      message: `确定从计划中移除「${action.source_name}」吗？该文件不会被整理。`,
      icon: "trash",
      confirmText: "移除",
      danger: true,
    });
  } catch {
    return;
  }
  try {
    await deleteMediaOrganizePlanAction(taskId, action.id);
    preview.relocates.value = preview.relocates.value.filter((a) => a.id !== action.id);
  } catch (e) {
    toast.error(getApiErrorMessage(e, "删除失败"));
  }
}

async function removePlanGroup(group: PlanGroup) {
  const taskId = planTaskId.value;
  if (!taskId) return;
  const total = group.actionCount;
  try {
    await confirm({
      title: "移除整个作品",
      message: `确定从计划中移除整个作品「${group.title}」（共 ${total} 项）吗？`,
      icon: "trash",
      confirmText: "移除整组",
      danger: true,
    });
  } catch {
    return;
  }
  const toRemove = [...group.actions];
  if (group.dirAction) toRemove.push(group.dirAction);
  try {
    const result = await batchDeleteMediaOrganizePlanActions(
      taskId,
      toRemove.map((a) => a.id),
    );
    const removedSet = new Set(result.removed ?? []);
    preview.relocates.value = preview.relocates.value.filter((a) => !removedSet.has(a.id));
    toast.success(`已移除 ${removedSet.size} 项`);
  } catch (e) {
    toast.error(getApiErrorMessage(e, "移除失败"));
  }
}

watch(logs, async () => {
  await nextTick();
  if (logBodyRef.value) logBodyRef.value.scrollTop = logBodyRef.value.scrollHeight;
});

onMounted(async () => {
  await Promise.all([loadTasks(), accountsStore.loadAccounts()]);
});

let activatedOnce = false;

onActivated(() => {
  if (activatedOnce) {
    void loadTasks(true);
    if (logTaskId.value) {
      void loadLogs(logTaskId.value);
      startLogPolling(logTaskId.value);
    }
    if (planOpen.value && planLoading.value && planTaskId.value) {
      startPlanProgressPolling(planTaskId.value);
    }
  }
  activatedOnce = true;
  syncTaskListPolling();
});

function stopPageActivity() {
  clearLogAutoCloseTimer();
  stopLogPolling();
  taskListPolling.stop();
  stopPlanProgressPolling();
}

onDeactivated(stopPageActivity);
onUnmounted(stopPageActivity);

defineExpose({
  openCreate,
  taskCount,
  runningCount,
  errorTaskCount,
  refreshing,
  loadTasks,
});
</script>

<template>
  <div class="organize-panel">
    <AdminStatsGrid v-if="!hideStats">
      <StatCard icon="📋" :value="tasks.length" label="任务数量" tone="blue" />
      <StatCard icon="▶️" :value="runningCount" label="执行中" tone="purple">
        <template #actions>
          <AppIconButton
            label="刷新"
            variant="secondary"
            size="xs"
            :disabled="refreshing"
            title="刷新任务列表"
            @click="() => loadTasks()"
          />
        </template>
      </StatCard>
    </AdminStatsGrid>

    <AdminEmptyState
      v-if="listReady && !refreshing && !tasks.length"
      icon="📁"
      title="还没有整理任务"
      description="添加整理任务后，可以预览整理目标，并在确认无误后手动执行。"
    >
      <AppButton type="button" variant="primary" @click="openCreate">添加第一个任务</AppButton>
    </AdminEmptyState>

    <div v-else-if="tasks.length" class="admin-panel-table-wrap">
      <table class="admin-table organize-table">
        <thead>
          <tr>
            <th>任务</th>
            <th class="organize-table__target-col">目标目录</th>
            <th class="organize-table__mode-col">操作方式</th>
            <th>状态</th>
            <th class="admin-table__actions">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="task in tasks" :key="task.id" class="organize-task-row">
            <td>
              <div class="organize-task-main" :title="`${task.task_name} · ${accountName(task.account_id)}`">
                <div class="organize-task-name">{{ task.task_name }}</div>
                <div class="organize-account-sub">{{ accountName(task.account_id) }}</div>
              </div>
            </td>
            <td class="organize-path organize-table__target-col" :title="task.config?.target_directory">{{ task.config?.target_directory || "—" }}</td>
            <td class="organize-table__mode-col">
              <span class="organize-mode-badge">{{ task.config?.action_type === "rename" ? "原地" : "移动" }}</span>
            </td>
            <td>
              <AdminRunStatusCell
                :title="statusTitle(task)"
                :primary="statusText(task)"
                :summary="task.last_run_result ? resultSummary(task) : undefined"
                :variant="organizeStatusVariant(task)"
                text-layout="column"
                primary-tone="strong"
              />
            </td>
            <td class="admin-table__actions">
              <AdminRowActions>
                <AdminTableActionBtn
                  v-if="isTaskActive(task)"
                  icon="stop"
                  :title="task.status === 'stopping' ? '正在停止' : '停止整理'"
                  danger
                  :disabled="task.status === 'stopping'"
                  @click="handleStop(task)"
                />
                <AdminTableActionBtn
                  v-else
                  icon="play"
                  title="预览并执行"
                  @click="previewPlan(task)"
                />
                <AdminTableActionBtn icon="log" title="查看日志" @click="openLogPanel(task)" />
                <AdminTableActionBtn icon="edit" title="编辑" @click="openEdit(task)" />
                <AdminTableActionBtn icon="delete" title="删除" danger @click="handleDelete(task)" />
                <template #menu>
                  <button
                    v-if="isTaskActive(task)"
                    type="button"
                    class="admin-row-actions__item admin-row-actions__item--danger"
                    :disabled="task.status === 'stopping'"
                    @click="handleStop(task)"
                  >
                    {{ task.status === "stopping" ? "正在停止" : "停止整理" }}
                  </button>
                  <button v-else type="button" class="admin-row-actions__item" @click="previewPlan(task)">
                    预览执行
                  </button>
                  <button type="button" class="admin-row-actions__item" @click="openLogPanel(task)">
                    查看日志
                  </button>
                  <button type="button" class="admin-row-actions__item" @click="openEdit(task)">编辑</button>
                  <button
                    type="button"
                    class="admin-row-actions__item admin-row-actions__item--danger"
                    @click="handleDelete(task)"
                  >
                    删除
                  </button>
                </template>
              </AdminRowActions>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="logTaskId" class="organize-log-panel">
      <header class="organize-log-panel__head">
        <div class="organize-log-panel__title">
          <span>整理日志</span>
          <small v-if="logTaskName">（{{ logTaskName }}）</small>
        </div>
        <button type="button" class="organize-log-panel__close" title="关闭日志" @click="closeLogPanel">×</button>
      </header>
      <div ref="logBodyRef" class="organize-log-panel__body">
        <div v-if="!logs.length" class="organize-log-panel__empty">等待任务输出…</div>
        <div v-for="(line, index) in logs" :key="index" class="organize-log-line">
          <span class="organize-log-time">[{{ line.time }}]</span>
          <span>{{ line.message }}</span>
        </div>
      </div>
    </div>

    <AppModal :open="dialogOpen" :title="editingId ? '修改任务' : '新建整理任务'" size="md" @close="dialogOpen = false">
      <div class="modal-form">
        <div class="modal-form__row">
          <FormField label="任务名称">
            <AppInput v-model="form.task_name" maxlength="10" placeholder="例如：电影整理" />
          </FormField>
          <FormField label="操作方式">
            <div class="mo-toggle-group">
              <button
                type="button"
                class="mo-toggle-btn"
                :class="{ 'mo-toggle-btn--active': form.action_type === 'rename' }"
                @click="switchActionType('rename')"
              >
                原地重命名
              </button>
              <button
                type="button"
                class="mo-toggle-btn"
                :class="{ 'mo-toggle-btn--active': form.action_type === 'move' }"
                @click="switchActionType('move')"
              >
                移动到新目录
              </button>
            </div>
          </FormField>
        </div>

        <FormField label="整理目录">
          <AccountFolderField
            :display="sourceDirDisplay"
            :title="sourceDirTitle"
            @browse="sourcePickerOpen = true"
          />
        </FormField>

        <div v-if="form.action_type === 'move'" class="modal-form__row">
          <FormField label="目标根目录">
            <AccountFolderField
              :display="rootDirDisplay"
              :title="rootDirTitle"
              placeholder="点击浏览选择目录"
              @browse="openRootPicker"
            />
          </FormField>
          <FormField label="媒体类型">
            <AppSelect v-model="form.media_type" :options="mediaTypeOptions" />
          </FormField>
        </div>

        <div v-else class="modal-form__row">
          <div class="form-field">
            <div class="form-field__label-row">
              <label class="form-field__label">整理标识</label>
              <SettingsHelpTooltip title="整理标识说明">
                <p>原地重命名靠它判断哪些文件已整理过，避免重复处理。</p>
                <p><b>tmdb</b>（推荐）：文件名写入 {"{tmdb-xxxx}"} 作为标识。</p>
                <p><b>off</b>：文件名不写入任何标识，靠规范命名结构判断。</p>
                <p><b>自定义</b>（如 v2）：文件名写入 [v2] 作为标识。</p>
              </SettingsHelpTooltip>
            </div>
            <AppInput v-model="form.rename_marker" placeholder="tmdb（推荐）/ off / 自定义如 v2" />
          </div>
          <FormField label="媒体类型">
            <AppSelect v-model="form.media_type" :options="mediaTypeOptions" />
          </FormField>
        </div>

        <div class="modal-form__row">
          <FormField label="使用 TMDB 匹配">
            <AppSelect v-model="form.use_tmdb" :options="boolOptions" />
          </FormField>
          <FormField label="散落电影按文件建夹">
            <AppSelect v-model="form.scatter_movie_per_file" :options="boolOptions" />
          </FormField>
        </div>
        <p class="form-hint">开启后：同一目录下多部无关电影各自新建同名文件夹（STARS-182/STARS-182.mp4），避免互相干扰。</p>

        <div class="modal-form__footer">
          <AppButton type="button" variant="primary" :disabled="submitting" @click="submitTask">
            {{ submitting ? "保存中…" : editingId ? "更新任务" : "保存任务" }}
          </AppButton>
        </div>
      </div>
    </AppModal>

    <AppModal :open="planOpen" size="lg" @close="closePlanDialog">
      <template #header>
        <h3 class="organize-plan-modal-title">
          整理计划预览
          <small v-if="planTaskName" class="organize-plan-subtitle">· {{ planTaskName }}</small>
        </h3>
      </template>

      <div class="organize-plan-content">
          <div v-if="planLoading" class="organize-plan-loading">
        <BusySpinner variant="notch" :size="42" color="var(--brand)" />
        <div class="organize-plan-loading__title">正在扫描并生成计划…</div>
        <div class="organize-plan-loading__metrics">
          <span class="organize-plan-metric">扫描目录 {{ planProgress.scanned_dirs || 0 }}</span>
          <span class="organize-plan-metric">媒体文件 {{ planProgress.scanned_files || 0 }}</span>
          <span class="organize-plan-metric">已分组 {{ planProgress.groups || 0 }}</span>
          <span class="organize-plan-metric">已生成 {{ planProgress.actions || 0 }} 个动作</span>
        </div>
        <div v-if="planProgress.current_dir" class="organize-plan-loading__current">
          当前批次: {{ planProgress.current_dir }}
        </div>
      </div>

      <template v-else>
        <div
          v-if="preview.tmdbStatus.value === 'disabled_task'"
          class="organize-plan-tmdb-banner organize-plan-tmdb-banner--info"
        >
          <span>任务未开启 TMDB 匹配，本次仅使用文件名识别。请编辑任务，将「使用 TMDB 匹配」设为开启后重新生成。</span>
        </div>
        <div
          v-else-if="preview.tmdbStatus.value && preview.tmdbStatus.value !== 'available'"
          class="organize-plan-tmdb-banner"
        >
          <span v-if="preview.tmdbStatus.value === 'no_api_key'">未配置 TMDB API Key，本次计划仅使用文件名识别；请先在「整理设置」保存 Key 后重新生成。</span>
          <span v-else-if="preview.tmdbStatus.value === 'unreachable'">TMDB 不可达，本次计划跳过了 TMDB 匹配；请检查网络或代理后再点击「重新生成」。</span>
          <span v-else>TMDB 状态异常: {{ preview.tmdbStatus.value }}</span>
          <AppButton type="button" variant="secondary" size="sm" :disabled="tmdbTesting" @click="testTmdb">
            {{ tmdbTesting ? "测试中…" : "立即测试" }}
          </AppButton>
        </div>

        <div class="organize-plan-tabs">
          <button
            type="button"
            class="organize-plan-tab"
            :class="{ 'organize-plan-tab--active': preview.activeTab.value === 'plan' }"
            @click="preview.activeTab.value = 'plan'"
          >
            待整理 <span class="organize-plan-tab-count">{{ preview.relocates.value.length }}</span>
          </button>
          <button
            v-if="preview.skipped.value.length"
            type="button"
            class="organize-plan-tab organize-plan-tab--skip"
            :class="{ 'organize-plan-tab--active': preview.activeTab.value === 'skip' }"
            @click="preview.activeTab.value = 'skip'"
          >
            已跳过 <span class="organize-plan-tab-count">{{ preview.skipped.value.length }}</span>
          </button>
          <span v-if="preview.noTmdbCount.value > 0" class="organize-plan-tabs-warning">
            ⚠ {{ preview.noTmdbCount.value }} 项未匹配 TMDB（无 ID）
          </span>
        </div>

        <template v-if="preview.activeTab.value === 'plan'">
          <AdminEmptyState
            v-if="!preview.groups.value.length"
            icon="📋"
            title="当前没有可执行的计划"
            description="点击「重新生成」让程序扫描目录并生成新的计划"
          />
          <div v-else class="organize-plan-list">
            <div
              v-for="group in preview.groups.value"
              :key="group.key"
              class="organize-plan-group"
              :class="{ 'organize-plan-group--expanded': group.expanded, 'organize-plan-group--editing': group.dirAction && planEditingId === group.dirAction.id }"
            >
              <div class="organize-plan-group-header" @click="preview.toggleGroup(group.key)">
                <span class="organize-plan-group-chevron" :class="{ 'organize-plan-group-chevron--open': group.expanded }">
                  <svg viewBox="0 0 8 12" fill="none"><path d="M1.5 1.5L6 6l-4.5 4.5" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
                </span>
                <div v-if="group.dirAction && planEditingId === group.dirAction.id" class="organize-plan-edit" @click.stop>
                  <div class="organize-plan-edit-source">{{ group.titleOld || group.title }}</div>
                  <span class="organize-plan-arrow">→</span>
                  <input
                    v-model="planEditingName"
                    class="organize-plan-edit-input"
                    @keydown.enter="commitPlanActionEdit(group.dirAction!)"
                    @keydown.esc="cancelPlanActionEdit"
                  />
                  <button type="button" class="plan-row-btn plan-row-btn--ok" :disabled="planEditingSaving" @click.stop="commitPlanActionEdit(group.dirAction!)"><i class="fas fa-check" aria-hidden="true" /></button>
                  <button type="button" class="plan-row-btn plan-row-btn--cancel" @click.stop="cancelPlanActionEdit"><i class="fas fa-xmark" aria-hidden="true" /></button>
                </div>
                <div v-else class="organize-plan-group-title-wrap">
                  <span v-if="group.hasDirInfo" class="organize-plan-group-title" :title="`${group.titleOld} → ${group.titleNew}`">
                    <span class="organize-plan-old">{{ group.titleOld }}</span>
                    <span class="organize-plan-arrow">→</span>
                    <span class="organize-plan-new">{{ group.titleNew }}</span>
                  </span>
                  <span v-else class="organize-plan-group-title" :title="group.title">{{ group.title }}</span>
                </div>
                <span class="organize-plan-group-right">
                  <span class="organize-plan-group-badges">
                    <span v-if="group.tmdbId" class="organize-plan-group-tmdb">tmdb-{{ group.tmdbId }}</span>
                    <span v-else class="organize-plan-group-notmdb">无 TMDB</span>
                    <span class="organize-plan-group-count">{{ group.actionCount }} 项</span>
                  </span>
                  <span class="organize-plan-group-controls">
                    <a
                      v-if="group.tmdbUrl"
                      :href="group.tmdbUrl"
                      class="plan-row-btn"
                      target="_blank"
                      rel="noopener noreferrer"
                      title="在 TMDB 核对作品"
                      @click.stop
                    ><i class="fas fa-arrow-up-right-from-square" aria-hidden="true" /></a>
                    <button v-if="group.dirAction" type="button" class="plan-row-btn" title="编辑作品目录名" @click.stop="startPlanActionEdit(group.dirAction)"><i class="fas fa-pen" aria-hidden="true" /></button>
                    <button type="button" class="plan-row-btn plan-row-btn--danger" title="从计划中移除整组" @click.stop="removePlanGroup(group)"><i class="fas fa-trash" aria-hidden="true" /></button>
                  </span>
                </span>
              </div>
              <div v-if="group.expanded" class="organize-plan-group-body">
                <template v-for="row in group.rows" :key="row.type === 'range' ? `r-${row.range!.key}` : `a-${row.action!.id}`">
                  <div v-if="row.type === 'range'" class="organize-plan-row organize-plan-row--range">
                    <div class="organize-plan-kind-col"><span class="organize-plan-kind organize-plan-kind--tv">剧集</span></div>
                    <div class="organize-plan-divider" />
                    <div class="organize-plan-body">
                      <div class="organize-plan-l1">
                        S{{ String(row.range!.season).padStart(2, "0") }} ·
                        <template v-if="row.range!.consecutive">E{{ String(row.range!.startEpisode).padStart(2, "0") }}–E{{ String(row.range!.endEpisode).padStart(2, "0") }}</template>
                        <template v-else>E{{ String(row.range!.startEpisode).padStart(2, "0") }}–E{{ String(row.range!.endEpisode).padStart(2, "0") }} 中</template>
                        共 {{ row.range!.count }} 集（命名一致）
                      </div>
                      <div class="organize-plan-l2">
                        <span class="organize-plan-oldchip">原名<span class="organize-plan-oldtip">{{ row.range!.samplePattern.oldPattern }}</span></span>
                        <span class="organize-plan-l2-dot">·</span>
                        <span class="organize-plan-l2-mode">{{ row.range!.samplePattern.newPattern }}</span>
                      </div>
                    </div>
                    <button type="button" class="plan-row-toggle" @click.stop="preview.toggleRange(row.range!.key)">
                      {{ row.range!.expanded ? "收起" : `展开 ${row.range!.count} 条` }}
                    </button>
                  </div>
                  <div
                    v-else
                    class="organize-plan-row"
                    :class="{ 'organize-plan-row--editing': planEditingId === row.action!.id, 'organize-plan-row--edited': row.action!.metadata?.edited }"
                  >
                    <template v-if="planEditingId === row.action!.id">
                      <div class="organize-plan-kind-col">
                        <span class="organize-plan-kind" :class="{ 'organize-plan-kind--tv': planActionMeta(row.action).typeLabel !== '电影' }">{{ planActionMeta(row.action).typeLabel }}</span>
                      </div>
                      <div class="organize-plan-divider" />
                      <div class="organize-plan-body">
                        <div class="organize-plan-edit">
                          <div class="organize-plan-edit-source">{{ row.action!.source_name }}</div>
                          <span class="organize-plan-arrow">→</span>
                          <input
                            v-model="planEditingName"
                            class="organize-plan-edit-input"
                            @keydown.enter="commitPlanActionEdit(row.action!)"
                            @keydown.esc="cancelPlanActionEdit"
                          />
                          <button type="button" class="plan-row-btn plan-row-btn--ok" :disabled="planEditingSaving" @click="commitPlanActionEdit(row.action!)"><i class="fas fa-check" aria-hidden="true" /></button>
                          <button type="button" class="plan-row-btn plan-row-btn--cancel" @click="cancelPlanActionEdit"><i class="fas fa-xmark" aria-hidden="true" /></button>
                        </div>
                      </div>
                    </template>
                    <template v-else>
                      <div class="organize-plan-kind-col">
                        <span class="organize-plan-kind" :class="{ 'organize-plan-kind--tv': planActionMeta(row.action).typeLabel !== '电影' }">{{ planActionMeta(row.action).typeLabel }}</span>
                      </div>
                      <div class="organize-plan-divider" />
                      <div class="organize-plan-body">
                        <div class="organize-plan-l1">
                          <span class="organize-plan-oldchip">原名<span class="organize-plan-oldtip">{{ row.action!.source_name || "?" }}</span></span>
                          <span class="organize-plan-l1-arrow">→</span>
                          <span class="organize-plan-l1-new">{{ row.action!.target_name || "?" }}</span>
                        </div>
                        <div class="organize-plan-l2">
                          <template v-if="planActionMeta(row.action).isDir">
                            <span class="organize-plan-l2-mode">{{ planActionMeta(row.action).dirLabel }}</span>
                          </template>
                          <template v-else>
                            <span v-if="planActionMeta(row.action).title || planActionMeta(row.action).se" class="organize-plan-l2-title">{{ planActionMeta(row.action).title || planActionMeta(row.action).se }}</span>
                            <span v-if="planActionMeta(row.action).title || planActionMeta(row.action).se" class="organize-plan-l2-dot">·</span>
                            <span class="organize-plan-l2-mode">{{ planActionMeta(row.action).mode }}</span>
                            <span class="organize-plan-l2-dot">·</span>
                            <span class="organize-plan-l2-conf" :class="{ 'organize-plan-l2-conf--low': planActionMeta(row.action).confLow }">识别可信度 {{ planActionMeta(row.action).conf }}%</span>
                          </template>
                        </div>
                      </div>
                      <span class="organize-plan-row-controls">
                        <button type="button" class="plan-row-btn" title="编辑目标名" @click="startPlanActionEdit(row.action!)"><i class="fas fa-pen" aria-hidden="true" /></button>
                        <button type="button" class="plan-row-btn plan-row-btn--danger" title="从计划中移除" @click="removePlanAction(row.action!)"><i class="fas fa-trash" aria-hidden="true" /></button>
                      </span>
                    </template>
                  </div>
                </template>
              </div>
            </div>
          </div>
        </template>

        <div v-if="preview.activeTab.value === 'skip'" class="organize-plan-skipped">
          <div v-for="group in preview.skipGroups.value" :key="group.reason" class="organize-plan-skipped-group">
            <div class="organize-plan-skipped-header" @click="preview.toggleSkipReason(group.reason)">
              <span class="organize-plan-skipped-chevron">{{ preview.skipExpandedReasons.value[group.reason] ? "▾" : "▸" }}</span>
              <span class="organize-plan-skipped-label">{{ group.reason || "其它" }}</span>
              <span class="organize-plan-skipped-count">{{ group.items.length }}</span>
            </div>
            <div v-if="preview.skipExpandedReasons.value[group.reason]" class="organize-plan-skipped-files">
              <div
                v-for="(item, idx) in (preview.skipShowAll.value[group.reason] ? group.items : group.items.slice(0, preview.skipPreviewLimit))"
                :key="idx"
                class="organize-plan-skipped-row"
              >
                {{ item.file_name }}
              </div>
              <div
                v-if="!preview.skipShowAll.value[group.reason] && group.items.length > preview.skipPreviewLimit"
                class="organize-plan-skipped-more"
                @click="preview.showAllSkip(group.reason)"
              >
                还有 {{ group.items.length - preview.skipPreviewLimit }} 项，点击展开
              </div>
            </div>
          </div>
        </div>
      </template>
      </div>

      <template #footer>
          <AppButton type="button" variant="secondary" :disabled="planLoading || planApplying" @click="refreshPlan">重新生成</AppButton>
          <AppButton
            type="button"
            variant="primary"
            :disabled="planLoading || planApplying || !preview.relocates.value.length"
            @click="applyPlan"
          >
            {{ planApplying ? "执行中…" : "确认执行" }}
          </AppButton>
      </template>
    </AppModal>

    <FolderPickerModal
      :open="sourcePickerOpen"
      selectable-account
      :accounts="activeAccounts"
      :account-id="form.account_id || null"
      :initial-path="form.target_directory"
      @close="sourcePickerOpen = false"
      @resolve="onSourceFolderPicked"
    />

    <FolderPickerModal
      :open="rootPickerOpen"
      :account-id="form.account_id || null"
      :accounts="activeAccounts"
      :initial-path="form.target_root"
      title="选择目标根目录"
      @close="rootPickerOpen = false"
      @resolve="onRootFolderPicked"
    />
  </div>
</template>

<style scoped>
.organize-panel {
  padding-bottom: 24px;
}

.organize-log-panel {
  margin-top: 14px;
  border: 1px solid var(--border);
  border-radius: 10px;
  overflow: hidden;
  background: #0f172a;
}

.organize-log-panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  height: 40px;
  padding: 0 12px;
  background: #111827;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.organize-log-panel__title {
  color: #e5e7eb;
  font-size: 13px;
  font-weight: 600;
}

.organize-log-panel__title small {
  margin-left: 6px;
  color: #94a3b8;
  font-weight: 400;
}

.organize-log-panel__close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: #94a3b8;
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}

.organize-log-panel__close:hover {
  background: rgba(255, 255, 255, 0.08);
  color: #e5e7eb;
}

.organize-log-panel__body {
  height: 240px;
  overflow-y: auto;
  padding: 10px 12px;
  font-family: Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  line-height: 1.65;
  color: #d1d5db;
  scrollbar-width: thin;
  scrollbar-color: rgba(148, 163, 184, 0.35) transparent;
}

.organize-log-panel__body::-webkit-scrollbar {
  width: 6px;
}

.organize-log-panel__body::-webkit-scrollbar-track {
  background: transparent;
}

.organize-log-panel__body::-webkit-scrollbar-thumb {
  background: rgba(148, 163, 184, 0.35);
  border-radius: 999px;
}

.organize-log-panel__empty {
  color: #94a3b8;
  text-align: center;
  padding: 24px 0;
}

.organize-log-line {
  white-space: pre-wrap;
  word-break: break-word;
}

.organize-log-time {
  color: #93c5fd;
  margin-right: 8px;
}

.organize-table {
  table-layout: fixed;
}

.organize-table th:nth-child(1),
.organize-table td:nth-child(1) {
  width: 15%;
}

.organize-table th:nth-child(2),
.organize-table td:nth-child(2) {
  width: 41%;
}

.organize-table th:nth-child(3),
.organize-table td:nth-child(3) {
  width: 9%;
}

.organize-table th:nth-child(4),
.organize-table td:nth-child(4) {
  width: 18%;
}

.organize-table th:last-child,
.organize-table td:last-child {
  text-align: center;
}

.organize-task-row:hover {
  background: color-mix(in srgb, var(--brand) 3%, var(--surface));
}

.organize-task-main {
  min-width: 0;
}

.organize-task-name {
  font-weight: 700;
  color: var(--text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.organize-account-sub {
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.organize-path {
  color: var(--text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.organize-mode-badge {
  display: inline-flex;
  padding: 4px 8px;
  border-radius: 6px;
  font-size: 11px;
  font-weight: 600;
  background: var(--surface-sunken);
  color: var(--text-muted);
}

@media (max-width: 720px) {
  .organize-table__target-col,
  .organize-table__mode-col {
    display: none;
  }

  .organize-table th,
  .organize-table td {
    padding: 10px 8px;
  }

  .organize-table th:nth-child(1),
  .organize-table td:nth-child(1) {
    width: 44%;
  }

  .organize-table th:nth-child(4),
  .organize-table td:nth-child(4) {
    width: auto;
    min-width: 150px;
  }

  .organize-table th:last-child,
  .organize-table td:last-child {
    width: 48px;
    text-align: right;
  }
}

.organize-plan-subtitle {
  margin-left: 6px;
  font-size: 14px;
  font-weight: 400;
  color: var(--text-muted);
}

.organize-plan-modal-title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text);
}

:deep(.modal--lg .modal__foot) {
  border-top: 1px solid var(--border);
  padding: 16px 24px 20px;
}

.modal-form {
  --mo-control-h: 40px;
}

.modal-form :deep(.app-input) {
  height: var(--mo-control-h);
  padding: 0 12px;
  box-sizing: border-box;
  font-size: 14px;
  line-height: 1.4;
}

.modal-form :deep(.select__trigger) {
  height: var(--mo-control-h);
  padding: 0 12px;
  box-sizing: border-box;
  font-size: 14px;
}

.mo-toggle-group {
  display: flex;
  height: var(--mo-control-h);
  box-sizing: border-box;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.mo-toggle-btn {
  flex: 1;
  margin: 0;
  padding: 0 12px;
  border: none;
  background: var(--surface-sunken);
  color: var(--text-muted);
  font-size: 13px;
  line-height: 1.2;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s ease, color 0.15s ease;
}

.mo-toggle-btn + .mo-toggle-btn {
  border-left: 1px solid var(--border);
}

.mo-toggle-btn:hover:not(.mo-toggle-btn--active) {
  background: color-mix(in srgb, var(--surface-sunken) 70%, var(--border));
  color: var(--text);
}

.mo-toggle-btn--active {
  background: linear-gradient(135deg, #4c74df, #02a6f0);
  color: #fff;
}

.organize-plan-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 14px;
  min-height: 280px;
  padding: 48px 16px;
}

.organize-plan-loading__title {
  font-size: 15px;
  color: var(--text);
}

.organize-plan-loading__metrics {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: center;
}

.organize-plan-metric {
  padding: 4px 12px;
  border-radius: 999px;
  background: var(--surface-sunken);
  color: var(--text-muted);
  font-size: 12px;
}

.organize-plan-loading__current {
  font-size: 12px;
  color: var(--text-muted);
  text-align: center;
  word-break: break-all;
}

.organize-plan-tmdb-banner {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  margin-bottom: 12px;
  background: color-mix(in srgb, var(--warning) 12%, var(--surface));
  border: 1px solid color-mix(in srgb, var(--warning) 35%, var(--border));
  border-radius: var(--radius-sm);
  font-size: 13px;
  color: var(--text);
}

.organize-plan-tmdb-banner > span {
  flex: 1;
}

.organize-plan-tmdb-banner--info {
  background: color-mix(in srgb, var(--primary) 8%, var(--surface));
  border-color: color-mix(in srgb, var(--primary) 25%, var(--border));
}

.organize-plan-tabs {
  display: flex;
  align-items: center;
  gap: 4px;
  border-bottom: 1px solid var(--border-soft);
  margin-bottom: 14px;
}

.organize-plan-tab {
  border: 0;
  background: transparent;
  cursor: pointer;
  padding: 9px 16px;
  font-size: 14px;
  color: var(--text-muted);
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.organize-plan-tab--active {
  color: var(--brand);
  border-bottom-color: var(--brand);
  font-weight: 600;
}

.organize-plan-tab-count {
  font-size: 11px;
  padding: 1px 7px;
  border-radius: 999px;
  background: var(--surface-sunken);
  color: var(--text-muted);
}

.organize-plan-tab--active .organize-plan-tab-count {
  background: color-mix(in srgb, var(--brand) 12%, var(--surface));
  color: var(--brand);
}

.organize-plan-tab--skip .organize-plan-tab-count {
  background: color-mix(in srgb, var(--warning) 15%, var(--surface));
  color: var(--warning);
}

.organize-plan-tabs-warning {
  margin-left: auto;
  font-size: 12px;
  color: var(--warning);
}

.organize-plan-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.organize-plan-group {
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-sm);
  background: var(--surface);
}

.organize-plan-group-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  background: var(--surface-sunken);
  border-radius: var(--radius-sm);
  cursor: pointer;
  font-size: 13px;
}

.organize-plan-group--expanded .organize-plan-group-header {
  border-radius: var(--radius-sm) var(--radius-sm) 0 0;
}

.organize-plan-group-chevron {
  color: var(--text-muted);
  width: 10px;
  transition: transform 0.15s ease;
}

.organize-plan-group-chevron--open {
  transform: rotate(90deg);
}

.organize-plan-group-title-wrap {
  flex: 1;
  min-width: 0;
  overflow: hidden;
}

.organize-plan-group-title {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  white-space: nowrap;
}

.organize-plan-old {
  color: var(--text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
}

.organize-plan-new {
  color: var(--text);
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
}

.organize-plan-arrow {
  color: var(--text-muted);
  flex-shrink: 0;
}

.organize-plan-group-right {
  position: relative;
  flex-shrink: 0;
  min-width: 80px;
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
}

.organize-plan-group-badges {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.organize-plan-group-controls {
  position: absolute;
  right: 0;
  top: 50%;
  transform: translateY(-50%);
  display: inline-flex;
  gap: 4px;
  opacity: 0;
  pointer-events: none;
}

.organize-plan-group-header:hover .organize-plan-group-badges,
.organize-plan-group--editing .organize-plan-group-badges {
  opacity: 0;
}

.organize-plan-group-header:hover .organize-plan-group-controls,
.organize-plan-group--editing .organize-plan-group-controls {
  opacity: 1;
  pointer-events: auto;
}

.organize-plan-group-tmdb {
  font-size: 12px;
  color: var(--brand);
  background: color-mix(in srgb, var(--brand) 10%, var(--surface));
  padding: 2px 8px;
  border-radius: 999px;
}

.organize-plan-group-notmdb {
  font-size: 12px;
  color: var(--warning);
  background: color-mix(in srgb, var(--warning) 12%, var(--surface));
  padding: 2px 8px;
  border-radius: 999px;
}

.organize-plan-group-count {
  font-size: 12px;
  color: var(--text-muted);
}

.organize-plan-group-body {
  padding: 2px 12px;
}

.organize-plan-row {
  position: relative;
  display: flex;
  align-items: center;
  padding: 9px 6px;
  border-radius: 8px;
}

.organize-plan-row:hover {
  background: #f8fafc;
  z-index: 20;
}

.organize-plan-row + .organize-plan-row {
  border-top: 1px solid var(--border-soft);
}

.organize-plan-kind-col {
  flex-shrink: 0;
  width: 46px;
  display: flex;
  justify-content: center;
}

.organize-plan-kind {
  font-size: 11px;
  font-weight: 600;
  padding: 2px 7px;
  border-radius: 5px;
  background: color-mix(in srgb, var(--brand) 10%, var(--surface));
  color: var(--brand);
}

.organize-plan-kind--tv {
  background: color-mix(in srgb, var(--info, #0ea5e9) 12%, var(--surface));
  color: #0e7490;
}

.organize-plan-divider {
  flex-shrink: 0;
  align-self: stretch;
  border-left: 1px dashed var(--border-soft);
  margin: 3px 13px;
}

.organize-plan-body {
  flex: 1;
  min-width: 0;
}

.organize-plan-l1 {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 6px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
  word-break: break-all;
}

.organize-plan-l1-arrow {
  color: var(--text-muted);
  font-size: 11px;
}

.organize-plan-l2 {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
  margin-top: 3px;
  font-size: 12px;
  color: var(--text-muted);
}

.organize-plan-l2-title {
  color: var(--text);
}

.organize-plan-l2-dot {
  color: var(--border);
}

.organize-plan-l2-conf--low {
  color: var(--warning);
  font-weight: 600;
}

.organize-plan-oldchip {
  position: relative;
  font-size: 12px;
  color: var(--brand);
  border-bottom: 1px dashed var(--brand);
  cursor: help;
}

.organize-plan-oldtip {
  position: absolute;
  left: 0;
  top: 130%;
  z-index: 30;
  visibility: hidden;
  opacity: 0;
  width: max-content;
  max-width: 520px;
  background: #1f2937;
  color: #fff;
  font-size: 12px;
  font-weight: 400;
  padding: 7px 10px;
  border-radius: 8px;
  white-space: normal;
  word-break: break-all;
}

.organize-plan-oldchip:hover .organize-plan-oldtip {
  visibility: visible;
  opacity: 1;
  transition-delay: 0.35s;
}

.organize-plan-row-controls {
  position: absolute;
  right: 6px;
  top: 50%;
  transform: translateY(-50%);
  display: none;
  gap: 4px;
}

.organize-plan-row:hover .organize-plan-row-controls,
.organize-plan-row--editing .organize-plan-row-controls {
  display: inline-flex;
}

.organize-plan-row--edited .organize-plan-l1-new {
  color: var(--brand);
}

.plan-row-toggle {
  margin-left: auto;
  font-size: 12px;
  color: var(--brand);
  background: none;
  border: none;
  cursor: pointer;
}

.plan-row-btn {
  width: 22px;
  height: 22px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: var(--surface);
  border: 1px solid var(--border-soft);
  border-radius: 6px;
  color: var(--text-muted);
  cursor: pointer;
  font-size: 11px;
  padding: 0;
  text-decoration: none;
}

.plan-row-btn:hover {
  background: var(--surface-sunken);
  color: var(--text);
}

.plan-row-btn--ok {
  color: #0f766e;
  border-color: #99f6e4;
  background: #ccfbf1;
}

.plan-row-btn--cancel {
  color: #b91c1c;
}

.plan-row-btn--danger {
  color: #b91c1c;
}

.organize-plan-edit {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  flex: 1;
}

.organize-plan-edit-source {
  font-size: 12px;
  color: var(--text-muted);
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.organize-plan-edit-input {
  flex: 1;
  min-width: 200px;
  padding: 5px 8px;
  border: 1px solid var(--border);
  border-radius: 6px;
  font-size: 13px;
  background: var(--surface);
  color: var(--text);
}

.organize-plan-skipped-group {
  border-bottom: 1px dashed var(--border-soft);
}

.organize-plan-skipped-header {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 2px;
  cursor: pointer;
}

.organize-plan-skipped-label {
  flex: 1;
  font-size: 12px;
  color: var(--text-muted);
}

.organize-plan-skipped-count {
  font-size: 11px;
  font-weight: 600;
  color: #fff;
  background: var(--warning);
  border-radius: 999px;
  padding: 1px 9px;
}

.organize-plan-skipped-files {
  max-height: 280px;
  overflow-y: auto;
  padding: 2px 0 6px 18px;
}

.organize-plan-skipped-row {
  font-size: 12px;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.organize-plan-skipped-more {
  margin-top: 4px;
  font-size: 11px;
  color: var(--warning);
  cursor: pointer;
}

.form-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-field__label-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.form-field__label {
  font-size: 13px;
  color: var(--text-regular);
  font-weight: 500;
}
</style>
