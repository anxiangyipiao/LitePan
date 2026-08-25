<script setup lang="ts">
import { onActivated, onBeforeUnmount, onDeactivated, onMounted, reactive, ref } from "vue";
import { accountsApi } from "@/api/accounts";
import { getApiErrorMessage } from "@/api/client";
import {
  deleteBackupJob,
  fetchBackupJobs,
  fetchBackupRuns,
  runBackupJob,
  streamBackupRun,
  toggleBackupJob,
  type BackupJob,
  type BackupRun,
  type BackupStreamEvent,
} from "@/api/backup";
import { toast } from "@/composables/useToast";
import { confirm } from "@/composables/useConfirm";
import { formatTime } from "@/utils/format";
import type { Account } from "@/api/types";
import type { AdminRunStatusVariant } from "@/components/admin/adminRunStatus";
import AppButton from "@/components/base/AppButton.vue";
import AppBadge from "@/components/base/AppBadge.vue";
import AdminEnableToggle from "@/components/admin/AdminEnableToggle.vue";
import AdminRowActions from "@/components/admin/AdminRowActions.vue";
import AdminRunStatusCell from "@/components/admin/AdminRunStatusCell.vue";
import AdminTableActionBtn from "@/components/admin/AdminTableActionBtn.vue";
import BackupJobModal from "@/components/admin/BackupJobModal.vue";
import "@/styles/admin-table.css";

const jobs = ref<BackupJob[]>([]);
const accounts = ref<Account[]>([]);
const loading = ref(false);
const modalOpen = ref(false);
const editingJob = ref<BackupJob | null>(null);

const drawerOpen = ref(false);
const drawerJob = ref<BackupJob | null>(null);
const runs = ref<BackupRun[]>([]);
const runsLoading = ref(false);
const expandedRunIds = ref<Set<number>>(new Set());

function toggleRunFailures(runId: number) {
  const next = new Set(expandedRunIds.value);
  if (next.has(runId)) {
    next.delete(runId);
  } else {
    next.add(runId);
  }
  expandedRunIds.value = next;
}

function isRunFailuresExpanded(runId: number): boolean {
  return expandedRunIds.value.has(runId);
}

const livePhase = ref<"idle" | "streaming" | "done">("idle");
const liveCounters = reactive({ total: 0, skipped: 0, uploaded: 0, rapid: 0, failed: 0 });
const liveFiles = ref<Array<{ rel_path: string; mode: string; error?: string }>>([]);
const liveStatus = ref("");
const liveMessage = ref("");
let liveAborter: AbortController | null = null;
let refreshTimer: number | null = null;

const accountName = (id: number): string => {
  const hit = accounts.value.find((a) => a.id === id);
  if (hit) return hit.name;
  return id ? `账号 #${id}` : "-";
};

const statusLabel = (job: BackupJob): string => {
  if (job.last_run_status === "running") return "备份中";
  if (job.last_run_status === "success") return "成功";
  if (job.last_run_status === "partial") return "部分完成";
  if (job.last_run_status === "failed") return "失败";
  return "未运行";
};

const statusVariant = (job: BackupJob): AdminRunStatusVariant => {
  if (job.last_run_status === "running") return "running";
  if (job.last_run_status === "success") return "success";
  if (job.last_run_status === "failed") return "error";
  return "pending";
};

const scheduleLabel = (job: BackupJob): string => {
  if (job.schedule_mode === "daily") return `每天 ${job.time || "00:00"}`;
  if (job.schedule_mode === "interval")
    return `${job.start_time || "00:00"} 起，每 ${job.interval_hours || 0} 小时`;
  return "手动";
};

const runStatusText = (status: string): string => {
  if (status === "running") return "执行中";
  if (status === "success") return "成功";
  if (status === "partial") return "部分完成";
  if (status === "failed") return "失败";
  return status;
};

const runStatusClass = (status: string): string => {
  if (status === "running") return "running";
  if (status === "success") return "success";
  if (status === "partial") return "partial";
  return "failed";
};

const modeText = (mode: string): string => {
  if (mode === "skip") return "跳过";
  if (mode === "rapid") return "秒传";
  if (mode === "upload") return "上传";
  if (mode === "error") return "失败";
  return mode;
};

const formatDate = (value?: string): string => {
  if (!value) return "-";
  const f = formatTime(value);
  return f === "-" ? "-" : f.slice(0, 16);
};

async function loadJobs() {
  try {
    jobs.value = (await fetchBackupJobs()) ?? [];
  } catch (e) {
    toast.error(getApiErrorMessage(e, "加载备份任务失败"));
  }
}

async function loadAll() {
  loading.value = true;
  try {
    const [jobData, accData] = await Promise.all([fetchBackupJobs(), accountsApi.list()]);
    jobs.value = jobData ?? [];
    accounts.value = accData ?? [];
  } catch (e) {
    toast.error(getApiErrorMessage(e, "加载备份任务失败"));
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editingJob.value = null;
  modalOpen.value = true;
}

function openEdit(job: BackupJob) {
  editingJob.value = job;
  modalOpen.value = true;
}

function onSaved() {
  void loadJobs();
}

async function setEnabled(job: BackupJob, enabled: boolean) {
  if (job.enabled === enabled) return;
  try {
    await toggleBackupJob(job.id);
    toast.success(enabled ? "备份任务已启用" : "备份任务已停用");
    void loadJobs();
  } catch (e) {
    toast.error(getApiErrorMessage(e, "状态更新失败"));
  }
}

async function deleteJob(job: BackupJob) {
  try {
    await confirm({
      title: "删除备份任务",
      message: `确认删除「${job.name}」？文件指纹与运行记录也会一并清理，目标网盘上的文件不会被删除。`,
      confirmText: "删除",
      danger: true,
      icon: "trash",
    });
    await deleteBackupJob(job.id);
    toast.success("备份任务已删除");
    void loadJobs();
  } catch (e) {
    if (e && typeof e === "object" && "message" in e && e.message === "Modal closed") return;
    toast.error(getApiErrorMessage(e, "删除失败"));
  }
}

async function runNow(job: BackupJob) {
  try {
    await runBackupJob(job.id);
    openDrawer(job);
    startLive(job.id);
    void refreshRuns(job.id);
  } catch (e) {
    toast.error(getApiErrorMessage(e, "启动备份失败"));
  }
}

async function openRuns(job: BackupJob) {
  openDrawer(job);
  void refreshRuns(job.id);
  if (job.last_run_status === "running") {
    startLive(job.id);
  }
}

function openDrawer(job: BackupJob) {
  drawerJob.value = job;
  runs.value = [];
  livePhase.value = "idle";
  liveStatus.value = "";
  liveMessage.value = "";
  liveFiles.value = [];
  resetCounters();
  drawerOpen.value = true;
}

function closeDrawer() {
  stopLive();
  drawerOpen.value = false;
  drawerJob.value = null;
  runs.value = [];
}

function resetCounters() {
  liveCounters.total = 0;
  liveCounters.skipped = 0;
  liveCounters.uploaded = 0;
  liveCounters.rapid = 0;
  liveCounters.failed = 0;
}

function applyCounters(ev: BackupStreamEvent) {
  liveCounters.total = Number(ev.total ?? liveCounters.total);
  liveCounters.skipped = Number(ev.skipped ?? liveCounters.skipped);
  liveCounters.uploaded = Number(ev.uploaded ?? liveCounters.uploaded);
  liveCounters.rapid = Number(ev.rapid ?? liveCounters.rapid);
  liveCounters.failed = Number(ev.failed ?? liveCounters.failed);
}

async function refreshRuns(jobId: number) {
  if (!drawerOpen.value) return;
  runsLoading.value = true;
  try {
    runs.value = (await fetchBackupRuns(jobId, 20)) ?? [];
  } catch {
    /* 静默失败，下一轮刷新重试 */
  } finally {
    runsLoading.value = false;
  }
}

async function startLive(jobId: number) {
  stopLive();
  const ac = new AbortController();
  liveAborter = ac;
  livePhase.value = "streaming";
  liveFiles.value = [];
  resetCounters();
  try {
    for await (const ev of streamBackupRun(jobId, ac.signal)) {
      if (ac.signal.aborted) break;
      if (ev.event === "file") {
        applyCounters(ev);
        liveFiles.value.unshift({
          rel_path: String(ev.rel_path ?? ""),
          mode: String(ev.mode ?? ""),
          error: ev.error ? String(ev.error) : undefined,
        });
        if (liveFiles.value.length > 60) liveFiles.value.pop();
      } else if (ev.event === "end" || ev.event === "error") {
        applyCounters(ev);
        livePhase.value = "done";
        liveStatus.value = ev.event === "error" ? "failed" : String(ev.status ?? "success");
        liveMessage.value = String(ev.message ?? "");
        break;
      }
    }
  } catch (e) {
    // 用户关闭抽屉中止属正常流程，不弹错
  } finally {
    if (liveAborter === ac) liveAborter = null;
    if (livePhase.value !== "done") livePhase.value = "idle";
    if (drawerJob.value) void refreshRuns(drawerJob.value.id);
    void loadJobs();
  }
}

function stopLive() {
  if (liveAborter) {
    liveAborter.abort();
    liveAborter = null;
  }
}

function startPageActivity() {
  if (refreshTimer) return;
  refreshTimer = window.setInterval(() => {
    if (drawerOpen.value) return; // 抽屉打开时以流为准，避免列表刷新打扰
    void loadJobs();
  }, 5000);
}

function stopPageActivity() {
  if (refreshTimer) {
    clearInterval(refreshTimer);
    refreshTimer = null;
  }
}

onMounted(() => {
  void loadAll();
  startPageActivity();
});

onActivated(() => {
  void loadAll();
  startPageActivity();
});

onDeactivated(stopPageActivity);

onBeforeUnmount(() => {
  stopPageActivity();
  stopLive();
});
</script>

<template>
  <div class="backup-page">
    <section class="admin-panel-table-wrap backup-list-panel">
      <div class="panel-head">
        <div>
          <div class="panel-title">定时备份</div>
          <div class="panel-sub">把本地磁盘文件夹增量备份到目标网盘：跳过未变化文件，秒传已存在内容，支持手动与定时执行。</div>
        </div>
        <div class="panel-head-actions">
          <AppBadge tone="info">{{ jobs.length }} 个任务</AppBadge>
          <AppButton type="button" size="sm" variant="primary" @click="openCreate">
            <i class="fas fa-plus"></i>
            新增备份任务
          </AppButton>
        </div>
      </div>
      <div class="table-wrap">
        <table class="admin-table backup-table">
          <thead>
            <tr>
              <th class="col-name">任务</th>
              <th class="col-source">源（账号·目录）</th>
              <th class="col-target">目标</th>
              <th class="col-schedule">调度</th>
              <th class="col-last">上次运行</th>
              <th class="col-op">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading">
              <td colspan="6" class="empty-cell">加载中...</td>
            </tr>
            <tr v-else-if="jobs.length === 0">
              <td colspan="6" class="empty-cell">还没有备份任务，点右上角「新增备份任务」创建第一个</td>
            </tr>
            <tr v-for="job in jobs" v-else :key="job.id">
              <td>
                <div class="job-name">{{ job.name }}</div>
                <div class="job-meta">
                  <span class="job-status" :class="job.last_run_status">{{ statusLabel(job) }}</span>
                  <span v-if="job.last_run_status === 'running'" class="job-running-dot"></span>
                </div>
              </td>
              <td
                class="job-path-cell"
                :title="`${accountName(job.source_account_id)} · ${job.source_display_path || '/'}`"
              >
                {{ accountName(job.source_account_id) }} · {{ job.source_display_path || "/" }}
              </td>
              <td class="job-path-cell" :title="`${accountName(job.target_account_id)} · ${job.target_display_path || '/'}`">
                {{ accountName(job.target_account_id) }} · {{ job.target_display_path || "/" }}
              </td>
              <td>
                <span class="job-schedule">{{ scheduleLabel(job) }}</span>
              </td>
              <td>
                <AdminRunStatusCell
                  :title="job.last_run_message || statusLabel(job)"
                  :primary="statusLabel(job)"
                  :summary="job.last_run_message || formatDate(job.last_run_at)"
                  :variant="statusVariant(job)"
                  :live="job.last_run_status === 'running'"
                  primary-tone="strong"
                />
              </td>
              <td class="admin-table__actions">
                <AdminRowActions>
                  <AdminEnableToggle
                    :enabled="job.enabled"
                    aria-label="备份任务启用切换"
                    @enable="enabled => setEnabled(job, enabled)"
                  />
                  <AdminTableActionBtn
                    icon="play"
                    title="立即备份"
                    :disabled="job.last_run_status === 'running'"
                    @click="runNow(job)"
                  />
                  <AdminTableActionBtn icon="log" title="运行记录 / 进度" @click="openRuns(job)" />
                  <template #menu>
                    <button class="admin-row-actions__item" type="button" @click="openEdit(job)">编辑</button>
                    <button
                      class="admin-row-actions__item admin-row-actions__item--danger"
                      type="button"
                      @click="deleteJob(job)"
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
    </section>

    <!-- 运行记录抽屉 -->
    <Teleport to="body">
      <div v-if="drawerOpen" class="backup-drawer-overlay" @click.self="closeDrawer">
        <div class="backup-drawer">
          <div class="backup-drawer-head">
            <div>
              <div class="backup-drawer-title">运行记录</div>
              <div class="backup-drawer-sub">{{ drawerJob?.name }} · 最近 {{ runs.length || 0 }} 次</div>
            </div>
            <div class="backup-drawer-actions">
              <button
                type="button"
                class="backup-drawer-close"
                title="关闭"
                @click="closeDrawer"
              >
                <i class="fas fa-times"></i>
              </button>
            </div>
          </div>

          <div class="backup-drawer-body">
            <!-- 实时进度 -->
            <div v-if="livePhase === 'streaming' || livePhase === 'done'" class="live-progress">
              <div class="live-progress__head">
                <span class="live-progress__title">
                  <span v-if="livePhase === 'streaming'" class="live-spinner"></span>
                  {{ livePhase === "streaming" ? "备份进行中…" : "备份结束" }}
                </span>
                <span v-if="liveStatus" class="live-status" :class="runStatusClass(liveStatus)">
                  {{ runStatusText(liveStatus) }}
                </span>
              </div>
              <div class="live-progress__counters">
                <span class="live-counter">总文件 <b>{{ liveCounters.total }}</b></span>
                <span class="live-counter">跳过 <b>{{ liveCounters.skipped }}</b></span>
                <span class="live-counter">上传 <b>{{ liveCounters.uploaded }}</b></span>
                <span class="live-counter is-rapid">秒传 <b>{{ liveCounters.rapid }}</b></span>
                <span class="live-counter is-fail">失败 <b>{{ liveCounters.failed }}</b></span>
              </div>
              <p v-if="liveMessage" class="live-progress__message">{{ liveMessage }}</p>
              <div v-if="liveFiles.length" class="live-progress__files">
                <div
                  v-for="(f, idx) in liveFiles"
                  :key="idx"
                  class="live-file"
                  :class="{ 'is-error': f.mode === 'error' }"
                  :title="f.error"
                >
                  <span class="live-file__mode" :class="f.mode">{{ modeText(f.mode) }}</span>
                  <span class="live-file__path">{{ f.rel_path }}</span>
                </div>
              </div>
            </div>

            <div class="runs-section-title">历史运行</div>
            <div v-if="runsLoading && runs.length === 0" class="backup-drawer-empty">加载中...</div>
            <div v-else-if="runs.length === 0" class="backup-drawer-empty">暂无运行记录</div>
            <ul v-else class="runs-list">
              <li v-for="run in runs" :key="run.id" class="runs-item" :class="run.status">
                <div class="runs-item-head">
                  <span class="runs-status-mini" :class="runStatusClass(run.status)">
                    {{ runStatusText(run.status) }}
                  </span>
                  <span class="runs-item-meta">{{ formatDate(run.started_at) }}</span>
                </div>
                <div class="runs-item-msg">{{ run.message }}</div>
                <div class="runs-item-counters">
                  <span>共 {{ run.total }}</span>
                  <span>跳过 {{ run.skipped }}</span>
                  <span>上传 {{ run.uploaded }}</span>
                  <span>秒传 {{ run.rapid }}</span>
                  <span>失败 {{ run.failed }}</span>
                </div>
                <div
                  v-if="run.failed_files && run.failed_files.length"
                  class="runs-item-failures"
                >
                  <button
                    type="button"
                    class="runs-failures-toggle"
                    @click="toggleRunFailures(run.id)"
                  >
                    <span>失败原因（{{ run.failed_files.length }} 条）</span>
                    <i
                      class="fas fa-chevron-down runs-failures-ico"
                      :class="{ open: isRunFailuresExpanded(run.id) }"
                    ></i>
                  </button>
                  <ul v-if="isRunFailuresExpanded(run.id)" class="runs-failures-list">
                    <li
                      v-for="(fl, idx) in run.failed_files"
                      :key="idx"
                      class="runs-failure"
                    >
                      <span class="runs-failure__path" :title="fl.rel_path">{{ fl.rel_path }}</span>
                      <span class="runs-failure__error" :title="fl.error">{{ fl.error }}</span>
                    </li>
                  </ul>
                </div>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </Teleport>

    <BackupJobModal
      :open="modalOpen"
      :job="editingJob"
      @saved="onSaved"
      @close="modalOpen = false"
    />
  </div>
</template>

<style scoped>
.backup-page {
  --panel: var(--surface);
  --soft: var(--surface-sunken);
  --line: var(--border);
  --muted: var(--text-muted);
  --muted2: color-mix(in srgb, var(--text-muted) 72%, transparent);
  --ink: var(--text);
  --blue: var(--brand);
  --ok: var(--success);
  --warn: var(--warning);
  --bad: var(--danger);
  color: var(--ink);
}

.backup-table {
  min-width: 900px;
  table-layout: fixed;
}
.col-name { width: 14%; }
.col-source { width: 18%; }
.col-target { width: 20%; }
.col-schedule { width: 12%; }
.col-last { width: 16%; }
.col-op { width: 20%; }

.backup-table th.col-op {
  text-align: center;
}

.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--line);
  background: var(--soft);
}
.panel-title {
  font-size: 15.5px;
  font-weight: 800;
}
.panel-sub {
  margin-top: 3px;
  color: var(--muted);
  font-size: 12.5px;
  line-height: 1.5;
}
.panel-head-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.job-name {
  color: var(--ink);
  font-weight: 800;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.job-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 4px;
}
.job-status {
  font-size: 12px;
  color: var(--muted);
}
.job-status.running {
  color: var(--blue);
}
.job-status.success {
  color: var(--ok);
}
.job-status.failed {
  color: var(--bad);
}
.job-status.partial {
  color: var(--warn);
}
.job-running-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--blue);
  animation: backupPulse 1.1s ease-in-out infinite;
}
@keyframes backupPulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.25; }
}
.job-path-cell {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--muted);
  font-size: 12.5px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}
.job-schedule {
  color: var(--muted);
  font-size: 12.5px;
  white-space: nowrap;
}

/* 抽屉（Teleport 到 body，脱离 .backup-page 作用域，需在此重新声明局部变量） */
.backup-drawer-overlay {
  --panel: var(--surface);
  --soft: var(--surface-sunken);
  --line: var(--border);
  --line2: color-mix(in srgb, var(--border) 82%, var(--text-muted));
  --ink: var(--text);
  --muted: var(--text-muted);
  --muted2: color-mix(in srgb, var(--text-muted) 72%, transparent);
  --blue: var(--brand);
  --ok: var(--success);
  --warn: var(--warning);
  --bad: var(--danger);
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.42);
  backdrop-filter: blur(3px);
  display: flex;
  justify-content: flex-end;
  z-index: 2800;
}
.backup-drawer {
  width: 440px;
  max-width: 94vw;
  height: 100%;
  background: var(--panel);
  display: flex;
  flex-direction: column;
  box-shadow: var(--shadow-pop);
  animation: backupDrawerIn 0.22s ease-out;
}
@keyframes backupDrawerIn {
  from { transform: translateX(24px); opacity: 0.6; }
  to { transform: translateX(0); opacity: 1; }
}
.backup-drawer-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 18px 20px;
  border-bottom: 1px solid var(--line);
  background: var(--soft);
}
.backup-drawer-title {
  font-size: 16px;
  font-weight: 700;
}
.backup-drawer-sub {
  margin-top: 4px;
  font-size: 13px;
  color: var(--muted);
}
.backup-drawer-actions {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.backup-drawer-close {
  width: 32px;
  height: 32px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel);
  color: var(--muted);
  cursor: pointer;
}
.backup-drawer-close:hover {
  color: var(--ink);
  border-color: color-mix(in srgb, var(--border) 82%, var(--text-muted));
}
.backup-drawer-body {
  flex: 1;
  overflow-y: auto;
  padding: 16px 18px 22px;
}
.backup-drawer-empty {
  padding: 28px 0;
  text-align: center;
  color: var(--muted2);
  font-size: 13px;
}

/* 实时进度 */
.live-progress {
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 12px;
  background: var(--soft);
  margin-bottom: 18px;
}
.live-progress__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.live-progress__title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 800;
}
.live-spinner {
  width: 12px;
  height: 12px;
  border: 2px solid color-mix(in srgb, var(--blue) 30%, transparent);
  border-top-color: var(--blue);
  border-radius: 50%;
  animation: backupSpin 0.8s linear infinite;
}
@keyframes backupSpin {
  to { transform: rotate(360deg); }
}
.live-status {
  font-size: 11.5px;
  font-weight: 800;
  padding: 2px 8px;
  border-radius: 999px;
}
.live-status.running {
  color: var(--blue);
  background: color-mix(in srgb, var(--blue) 12%, transparent);
}
.live-status.success {
  color: var(--ok);
  background: color-mix(in srgb, var(--ok) 12%, transparent);
}
.live-status.partial {
  color: var(--warn);
  background: color-mix(in srgb, var(--warn) 12%, transparent);
}
.live-status.failed {
  color: var(--bad);
  background: color-mix(in srgb, var(--bad) 12%, transparent);
}
.live-progress__counters {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 14px;
  margin-top: 10px;
}
.live-counter {
  font-size: 12px;
  color: var(--muted);
}
.live-counter b {
  color: var(--ink);
  font-size: 13px;
}
.live-counter.is-rapid b {
  color: var(--blue);
}
.live-counter.is-fail b {
  color: var(--bad);
}
.live-progress__message {
  margin: 10px 0 0;
  font-size: 12px;
  color: var(--muted);
  line-height: 1.5;
}
.live-progress__files {
  display: flex;
  flex-direction: column;
  gap: 3px;
  max-height: 220px;
  overflow-y: auto;
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px solid var(--line);
}
.live-file {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  min-width: 0;
}
.live-file.is-error .live-file__path {
  color: var(--bad);
}
.live-file__mode {
  flex: 0 0 auto;
  font-size: 10.5px;
  font-weight: 800;
  padding: 1px 6px;
  border-radius: 5px;
  color: var(--muted);
  background: color-mix(in srgb, var(--text-muted) 12%, transparent);
}
.live-file__mode.skip {
  color: var(--muted);
}
.live-file__mode.upload {
  color: var(--ok);
  background: color-mix(in srgb, var(--ok) 14%, transparent);
}
.live-file__mode.rapid {
  color: var(--blue);
  background: color-mix(in srgb, var(--blue) 14%, transparent);
}
.live-file__mode.error {
  color: var(--bad);
  background: color-mix(in srgb, var(--bad) 14%, transparent);
}
.live-file__path {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--muted);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.runs-section-title {
  font-size: 12.5px;
  font-weight: 800;
  color: var(--muted);
  margin: 0 0 10px;
}
.runs-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.runs-item {
  border: 1px solid var(--line);
  border-radius: 10px;
  padding: 12px;
  background: var(--panel);
}
.runs-item-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.runs-status-mini {
  display: inline-flex;
  align-items: center;
  height: 22px;
  padding: 0 8px;
  border-radius: 999px;
  font-size: 11.5px;
  font-weight: 850;
  color: var(--muted);
  background: color-mix(in srgb, var(--text-muted) 12%, transparent);
}
.runs-status-mini.running {
  color: var(--blue);
  background: color-mix(in srgb, var(--blue) 12%, transparent);
}
.runs-status-mini.success {
  color: var(--ok);
  background: color-mix(in srgb, var(--ok) 12%, transparent);
}
.runs-status-mini.partial {
  color: var(--warn);
  background: color-mix(in srgb, var(--warn) 12%, transparent);
}
.runs-status-mini.failed {
  color: var(--bad);
  background: color-mix(in srgb, var(--bad) 12%, transparent);
}
.runs-item-meta {
  font-size: 11.5px;
  color: var(--muted2);
}
.runs-item-msg {
  margin-top: 8px;
  font-size: 12px;
  color: var(--muted);
  line-height: 1.5;
  word-break: break-all;
}
.runs-item-counters {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin-top: 8px;
  font-size: 11.5px;
  color: var(--muted2);
}
.runs-item-failures {
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px solid var(--line);
}
.runs-failures-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  border: 0;
  border-radius: 6px;
  background: color-mix(in srgb, var(--bad) 10%, transparent);
  color: var(--bad);
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
}
.runs-failures-ico {
  font-size: 10px;
  transition: transform 0.18s ease;
}
.runs-failures-ico.open {
  transform: rotate(180deg);
}
.runs-failures-list {
  list-style: none;
  margin: 8px 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.runs-failure {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
  border-radius: 8px;
  background: var(--soft);
}
.runs-failure__path {
  font-size: 12px;
  color: var(--ink);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  word-break: break-all;
}
.runs-failure__error {
  font-size: 11.5px;
  color: var(--bad);
  line-height: 1.45;
  word-break: break-all;
}
</style>
