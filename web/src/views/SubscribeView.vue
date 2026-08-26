<script setup lang="ts">
import { computed, onActivated, onDeactivated, ref, watch } from "vue";

const props = withDefaults(defineProps<{ active?: boolean }>(), { active: true });
import AppButton from "@/components/base/AppButton.vue";
import AppBadge from "@/components/base/AppBadge.vue";
import BusySpinner from "@/components/base/BusySpinner.vue";
import AdminEnableToggle from "@/components/admin/AdminEnableToggle.vue";
import AdminTableActionBtn from "@/components/admin/AdminTableActionBtn.vue";
import AdminStatusPill from "@/components/admin/AdminStatusPill.vue";
import RSSSubscriptionModal from "@/components/rss/RSSSubscriptionModal.vue";
import {
  fetchRSSSubscriptions,
  deleteRSSSubscription,
  toggleRSSSubscription,
  fetchRSSSubscriptionNow,
  fetchRSSHistory,
  retryRSSHistory,
  clearRSSHistory,
  type RSSSubscription,
  type RSSDownloadHistory,
  type RSSHistoryStatus,
} from "@/api/rss";
import { accountsApi } from "@/api/accounts";
import { getApiErrorMessage } from "@/api/client";
import { toast } from "@/composables/useToast";
import { confirm } from "@/composables/useConfirm";
import { formatTimeShort, formatTime } from "@/utils/format";
import "@/styles/admin-table.css";

const subscriptions = ref<RSSSubscription[]>([]);
const loading = ref(false);
const error = ref("");
const modalOpen = ref(false);
const editing = ref<RSSSubscription | null>(null);
const accountNames = ref<Record<number, string>>({});

const historyOpen = ref(false);
const history = ref<RSSDownloadHistory[]>([]);
const historyLoading = ref(false);

const activeCount = computed(() => subscriptions.value.filter((s) => s.enabled).length);

const statusTone: Record<string, "success" | "warning" | "danger" | "muted"> = {
  ok: "success",
  error: "danger",
  "": "muted",
};

const historyStatusTone: Record<RSSHistoryStatus, "success" | "warning" | "brand" | "danger" | "muted"> = {
  pushed: "success",
  queued: "brand",
  completed: "success",
  matched: "muted",
  failed: "danger",
  skipped: "warning",
};

const historyStatusLabel: Record<RSSHistoryStatus, string> = {
  pushed: "已推送",
  queued: "已提交",
  completed: "已完成",
  matched: "已匹配",
  failed: "失败",
  skipped: "跳过",
};

async function load() {
  loading.value = true;
  error.value = "";
  try {
    subscriptions.value = (await fetchRSSSubscriptions()) ?? [];
  } catch (e) {
    error.value = getApiErrorMessage(e, "加载订阅失败");
  } finally {
    loading.value = false;
  }
}

async function loadAccounts() {
  try {
    const list = (await accountsApi.list()) ?? [];
    accountNames.value = Object.fromEntries(list.map((a) => [a.id, a.name]));
  } catch {
    // 账号名仅用于展示，失败可忽略
  }
}

function targetLabel(sub: RSSSubscription): string {
  if (sub.target_type === "qb") return "qBittorrent";
  const name = accountNames.value[sub.account_id];
  const path = sub.target_display_path || "/";
  return name ? `${name} · ${path}` : `网盘 #${sub.account_id} · ${path}`;
}

function ruleLabel(sub: RSSSubscription): string {
  const parts: string[] = [];
  if (sub.title_keyword) parts.push(sub.title_keyword);
  if (sub.episode_min > 0) parts.push(`EP${sub.episode_min}+`);
  if (sub.quality_keyword) parts.push(sub.quality_keyword);
  return parts.join(" · ") || "不限";
}

function intervalLabel(sub: RSSSubscription): string {
  return sub.fetch_interval_minutes > 0 ? `${sub.fetch_interval_minutes}m` : "默认";
}

function openCreate() {
  editing.value = null;
  modalOpen.value = true;
}

function openEdit(sub: RSSSubscription) {
  editing.value = sub;
  modalOpen.value = true;
}

async function toggle(sub: RSSSubscription, enabled: boolean) {
  try {
    const updated = await toggleRSSSubscription(sub.id);
    Object.assign(sub, updated);
    toast.success(enabled ? "订阅已启用" : "订阅已停用");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "切换失败"));
  }
}

async function fetchNow(sub: RSSSubscription) {
  try {
    const result = await fetchRSSSubscriptionNow(sub.id);
    toast.success(`抓取完成：${result.message}`);
    void load();
  } catch (e) {
    toast.error(getApiErrorMessage(e, "抓取失败"));
  }
}

async function remove(sub: RSSSubscription) {
  try {
    await confirm({
      title: "删除订阅",
      message: `确认删除订阅「${sub.name}」？其下载记录也会一并删除。`,
      confirmText: "删除",
      danger: true,
      icon: "trash",
    });
    await deleteRSSSubscription(sub.id);
    subscriptions.value = subscriptions.value.filter((s) => s.id !== sub.id);
    toast.success("订阅已删除");
  } catch (e) {
    if ((e as { message?: string })?.message === "Modal closed") return;
    toast.error(getApiErrorMessage(e, "删除失败"));
  }
}

function onSaved(sub: RSSSubscription) {
  const idx = subscriptions.value.findIndex((s) => s.id === sub.id);
  if (idx >= 0) subscriptions.value[idx] = sub;
  else subscriptions.value.unshift(sub);
}

async function openHistory() {
  historyOpen.value = true;
  await loadHistory();
}

async function loadHistory() {
  historyLoading.value = true;
  try {
    history.value = (await fetchRSSHistory({ limit: 100 })) ?? [];
  } catch (e) {
    toast.error(getApiErrorMessage(e, "加载下载记录失败"));
  } finally {
    historyLoading.value = false;
  }
}

async function retry(rec: RSSDownloadHistory) {
  try {
    const updated = await retryRSSHistory(rec.id);
    const idx = history.value.findIndex((h) => h.id === rec.id);
    if (idx >= 0) history.value[idx] = updated;
    toast.success(updated.status === "pushed" || updated.status === "queued" ? "已重新推送" : "重推未成功");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "重推失败"));
  }
}

async function clearHistory() {
  if (!history.value.length) return;
  try {
    await confirm({
      title: "清空下载记录",
      message: "确认清空全部 RSS 下载记录？不会影响已下载的资源。",
      confirmText: "清空",
      danger: true,
      icon: "trash",
    });
    await clearRSSHistory();
    history.value = [];
    toast.success("记录已清空");
  } catch (e) {
    if ((e as { message?: string })?.message === "Modal closed") return;
    toast.error(getApiErrorMessage(e, "清空失败"));
  }
}

let pollTimer: ReturnType<typeof setInterval> | null = null;

function startPolling() {
  stopPolling();
  if (props.active === false) return;
  pollTimer = setInterval(() => {
    if (document.hidden) return;
    void load();
    if (historyOpen.value) void loadHistory();
  }, 5000);
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

// 作为磁力页标签时，仅在标签激活时轮询
watch(
  () => props.active,
  (active) => {
    if (active) {
      void load();
      startPolling();
    } else {
      stopPolling();
    }
  },
);

onActivated(() => {
  void load();
  void loadAccounts();
  startPolling();
});

onDeactivated(stopPolling);
</script>

<template>
  <div class="subscribe-page">
    <header class="subscribe-page__head">
      <div class="subscribe-page__hero">
        <div class="subscribe-page__icon">
          <i class="fa-solid fa-rss" aria-hidden="true"></i>
        </div>
        <div class="subscribe-page__hero-text">
          <h1 class="subscribe-page__title">订阅追番</h1>
          <p class="subscribe-page__desc">自动抓取动漫 RSS，按关键词 / 集数 / 画质匹配并推送到 qB 或网盘离线下载</p>
        </div>
      </div>
      <div class="subscribe-page__stats">
        <div class="subscribe-page__stat">
          <span class="subscribe-page__stat-value subscribe-page__stat-value--active">{{ activeCount }}</span>
          <span class="subscribe-page__stat-label">运行中</span>
        </div>
        <div class="subscribe-page__stat">
          <span class="subscribe-page__stat-value">{{ subscriptions.length }}</span>
          <span class="subscribe-page__stat-label">总订阅</span>
        </div>
      </div>
    </header>

    <div class="subscribe-page__bar">
      <AppButton variant="primary" @click="openCreate">
        <i class="fa-solid fa-plus" aria-hidden="true"></i>
        新增订阅
      </AppButton>
      <AppButton variant="secondary" @click="openHistory">
        抓取记录
        <AppBadge v-if="subscriptions.length" tone="info">{{ subscriptions.length }}</AppBadge>
      </AppButton>
      <span class="subscribe-page__count">
        运行中 {{ activeCount }} / 共 {{ subscriptions.length }} 条
      </span>
    </div>

    <p v-if="error" class="subscribe-page__error">{{ error }}</p>

    <div v-if="loading && !subscriptions.length" class="subscribe-page__loading">
      <BusySpinner :size="22" />
      <span>正在加载订阅…</span>
    </div>

    <div v-else-if="!subscriptions.length" class="subscribe-page__empty">
      还没有订阅。点击「新增订阅」添加一个动漫 RSS 源。
    </div>

    <div v-else class="admin-panel-table-wrap">
      <div class="table-wrap">
        <table class="admin-table">
          <thead>
            <tr>
              <th>名称</th>
              <th>规则</th>
              <th>目标</th>
              <th>间隔</th>
              <th>上次抓取</th>
              <th class="subscribe-page__th-actions">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="sub in subscriptions" :key="sub.id">
              <td class="subscribe-page__td-name">
                <div class="subscribe-page__name" :title="sub.name">{{ sub.name }}</div>
                <div class="subscribe-page__feed" :title="sub.feed_url">{{ sub.feed_url }}</div>
              </td>
              <td class="subscribe-page__td-trunc" :title="ruleLabel(sub)">
                <span class="subscribe-page__cell">{{ ruleLabel(sub) }}</span>
              </td>
              <td class="subscribe-page__td-trunc" :title="targetLabel(sub)">
                <span class="subscribe-page__cell">{{ targetLabel(sub) }}</span>
              </td>
              <td class="subscribe-page__td-nowrap">
                <span class="subscribe-page__cell">{{ intervalLabel(sub) }}</span>
              </td>
              <td class="subscribe-page__td-nowrap">
                <div class="subscribe-page__fetch">
                  <AdminStatusPill :tone="statusTone[sub.last_fetch_status] ?? 'muted'">
                    {{ sub.last_fetch_status === "ok" ? "正常" : sub.last_fetch_status === "error" ? "异常" : "未抓取" }}
                  </AdminStatusPill>
                  <span
                    v-if="sub.last_fetch_at"
                    class="subscribe-page__time"
                    :title="sub.last_fetch_message || formatTime(sub.last_fetch_at)"
                  >
                    {{ formatTimeShort(sub.last_fetch_at) }}
                  </span>
                </div>
              </td>
              <td class="subscribe-page__td-nowrap subscribe-page__td-actions">
                <div class="subscribe-page__actions">
                  <AdminEnableToggle :enabled="sub.enabled" :aria-label="sub.name" @enable="(v) => toggle(sub, v)" />
                  <AdminTableActionBtn icon="play" title="立即抓取" @click="fetchNow(sub)" />
                  <AdminTableActionBtn icon="edit" title="编辑" @click="openEdit(sub)" />
                  <AdminTableActionBtn icon="delete" title="删除" danger @click="remove(sub)" />
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <RSSSubscriptionModal :open="modalOpen" :subscription="editing" @close="modalOpen = false" @saved="onSaved" />

    <Teleport to="body">
      <div v-if="historyOpen" class="subscribe-drawer">
        <div class="subscribe-drawer__backdrop" @click="historyOpen = false" />
        <div class="subscribe-drawer__panel">
          <div class="subscribe-drawer__head">
            <h2 class="subscribe-drawer__title">抓取记录</h2>
            <div class="subscribe-drawer__actions">
              <AppButton v-if="history.length" size="sm" variant="ghost" @click="clearHistory">清空</AppButton>
              <AppButton size="sm" variant="cancel" @click="historyOpen = false">关闭</AppButton>
            </div>
          </div>
          <div v-if="historyLoading && !history.length" class="subscribe-drawer__loading">
            <BusySpinner :size="20" />
            <span>加载中…</span>
          </div>
          <div v-else-if="!history.length" class="subscribe-drawer__empty">暂无下载记录</div>
          <div v-else class="subscribe-drawer__list">
            <div v-for="rec in history" :key="rec.id" class="subscribe-drawer__row">
              <div class="subscribe-drawer__main">
                <div class="subscribe-drawer__row-top">
                  <AdminStatusPill :tone="historyStatusTone[rec.status] ?? 'muted'">
                    {{ historyStatusLabel[rec.status] ?? rec.status }}
                  </AdminStatusPill>
                  <span v-if="rec.episode" class="subscribe-drawer__ep">EP{{ rec.episode }}</span>
                  <span class="subscribe-drawer__time">{{ formatTimeShort(rec.pushed_at || rec.created_at) }}</span>
                </div>
                <div class="subscribe-drawer__title" :title="rec.title">{{ rec.title }}</div>
                <div v-if="rec.message || rec.error" class="subscribe-drawer__msg">
                  {{ rec.error || rec.message }}
                </div>
                <div v-if="rec.torrent_url" class="subscribe-drawer__magnet" :title="rec.torrent_url">
                  {{ rec.torrent_url }}
                </div>
              </div>
              <div class="subscribe-drawer__ops">
                <AppButton
                  v-if="rec.status === 'failed' || rec.status === 'skipped'"
                  size="sm"
                  variant="secondary"
                  @click="retry(rec)"
                >
                  重试
                </AppButton>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.subscribe-page {
  max-width: 960px;
  margin: 0 auto;
  padding: 28px 20px calc(20px + env(safe-area-inset-bottom, 0px));
  box-sizing: border-box;
  width: 100%;
  overflow-x: hidden;
}

.subscribe-page__head {
  margin-bottom: 20px;
}

.subscribe-page__hero {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px 24px;
  border-radius: var(--radius-md);
  background: linear-gradient(135deg, #7c3aed, #6d28d9);
  box-shadow: var(--shadow-card);
  margin-bottom: 14px;
}

.subscribe-page__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: var(--radius-sm);
  background: rgba(255, 255, 255, 0.2);
  color: #fff;
  font-size: 20px;
  flex-shrink: 0;
}

.subscribe-page__hero-text {
  flex: 1;
  min-width: 0;
}

.subscribe-page__title {
  margin: 0;
  font-size: 22px;
  font-weight: 700;
  color: #fff;
}

.subscribe-page__desc {
  margin: 2px 0 0;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.85);
}

.subscribe-page__stats {
  display: flex;
  gap: 12px;
}

.subscribe-page__stat {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 14px 18px;
  border-radius: var(--radius-sm);
  background: var(--surface);
  border: 1px solid var(--border-soft);
  box-shadow: var(--shadow-soft);
}

.subscribe-page__stat-value {
  font-size: 24px;
  font-weight: 800;
  color: var(--text);
  line-height: 1;
}

.subscribe-page__stat-value--active {
  color: var(--success);
}

.subscribe-page__stat-label {
  font-size: 12px;
  color: var(--text-muted);
}

.subscribe-page__bar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.subscribe-page__count {
  margin-left: auto;
  color: var(--text-muted);
  font-size: 13px;
}

.subscribe-page__error {
  color: var(--danger, #ef4444);
  font-size: 13px;
  margin: 8px 0;
}

.subscribe-page__loading,
.subscribe-page__empty {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: var(--text-muted);
  font-size: 14px;
  padding: 48px 0;
}

/* 订阅表格：固定布局，操作列固定 160px，其他列按比例分剩余空间，确保不溢出容器 */
.admin-panel-table-wrap .admin-table {
  table-layout: fixed;
  width: 100%;
}

/* 表格容器不再保留横向滚动，列宽由 calc 严格控制 */
.admin-panel-table-wrap .table-wrap {
  overflow-x: visible;
}

/* 固定列宽：操作列 160px，其他列基于「容器宽 - 160px」按百分比分配
   这样总宽度永远 = 容器宽，不会溢出 */
.subscribe-page__th:nth-child(1),
.subscribe-page__td-name {
  width: calc((100% - 160px) * 0.32);
}

.subscribe-page__th:nth-child(2),
.subscribe-page__td-trunc:nth-child(2) {
  width: calc((100% - 160px) * 0.22);
}

.subscribe-page__th:nth-child(3),
.subscribe-page__td-trunc:nth-child(3) {
  width: calc((100% - 160px) * 0.22);
}

.subscribe-page__th:nth-child(4),
.subscribe-page__td-nowrap:nth-child(4) {
  width: calc((100% - 160px) * 0.10);
}

.subscribe-page__th:nth-child(5),
.subscribe-page__td-nowrap:nth-child(5) {
  width: calc((100% - 160px) * 0.14);
}

.subscribe-page__td-name {
  white-space: nowrap;
}

.subscribe-page__name {
  color: var(--text);
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}

.subscribe-page__feed {
  color: var(--text-muted);
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}

.subscribe-page__cell {
  color: var(--text);
  font-size: 13px;
}

.subscribe-page__td-trunc {
  white-space: normal;
  word-break: break-word;
}

.subscribe-page__td-trunc .subscribe-page__cell {
  display: block;
  word-break: break-word;
  white-space: normal;
}

.subscribe-page__td-nowrap {
  white-space: nowrap;
}

.subscribe-page__td-actions {
  text-align: right;
}

.subscribe-page__th-actions {
  text-align: right;
}

.subscribe-page__fetch {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
}

.subscribe-page__time {
  color: var(--text-muted);
  font-size: 12px;
}

.subscribe-page__actions {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: nowrap;
  gap: 4px;
}

/* 操作列：固定 160px，容纳 4 个按钮 */
.subscribe-page__th-actions,
.subscribe-page__td-actions {
  width: 160px;
  white-space: nowrap;
}

/* 抓取记录抽屉 */
.subscribe-drawer {
  position: fixed;
  inset: 0;
  z-index: 120;
}

.subscribe-drawer__backdrop {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
}

.subscribe-drawer__panel {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  width: min(460px, 92vw);
  background: var(--surface);
  border-left: 1px solid var(--border-soft);
  box-shadow: var(--shadow-card);
  display: flex;
  flex-direction: column;
}

.subscribe-drawer__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 18px;
  border-bottom: 1px solid var(--border-soft);
}

.subscribe-drawer__title {
  font-size: 17px;
  font-weight: 700;
  color: var(--text);
  margin: 0;
}

.subscribe-drawer__actions {
  display: flex;
  gap: 8px;
}

.subscribe-drawer__loading,
.subscribe-drawer__empty {
  color: var(--text-muted);
  font-size: 14px;
  padding: 40px 0;
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}

.subscribe-drawer__list {
  flex: 1;
  overflow-y: auto;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.subscribe-drawer__row {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 10px 12px;
  border: 1px solid var(--border-soft);
  border-radius: 10px;
  background: var(--surface-sunken);
}

.subscribe-drawer__main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.subscribe-drawer__row-top {
  display: flex;
  align-items: center;
  gap: 8px;
}

.subscribe-drawer__ep {
  color: var(--brand-strong, var(--brand));
  font-size: 12px;
  font-weight: 600;
}

.subscribe-drawer__time {
  color: var(--text-muted);
  font-size: 12px;
  margin-left: auto;
}

.subscribe-drawer__title {
  color: var(--text);
  font-size: 13px;
  word-break: break-word;
}

.subscribe-drawer__msg {
  color: var(--text-muted);
  font-size: 12px;
}

.subscribe-drawer__magnet {
  color: var(--text-muted);
  font-size: 11px;
  word-break: break-all;
  max-height: 40px;
  overflow: hidden;
  padding: 4px 6px;
  border-radius: 6px;
  background: var(--surface);
}

.subscribe-drawer__ops {
  flex: 0 0 auto;
}

/* 平板及以下（≤768px）：hero 收窄内边距，统计卡片 2 列，操作栏允许换行 */
@media (max-width: 768px) {
  .subscribe-page {
    padding: 22px 16px calc(18px + env(safe-area-inset-bottom, 0px));
  }

  .subscribe-page__hero {
    padding: 18px 18px;
  }

  .subscribe-page__title {
    font-size: 19px;
  }

  .subscribe-page__desc {
    font-size: 12px;
  }

  .subscribe-page__stats {
    flex-wrap: wrap;
  }

  .subscribe-page__stat {
    flex: 1 1 calc(50% - 6px);
    min-width: 0;
  }

  .subscribe-page__count {
    width: 100%;
    margin-left: 0;
  }
}

/* 手机（≤480px）：hero 紧凑，按钮 / 计数垂直堆叠 */
@media (max-width: 480px) {
  .subscribe-page__hero {
    gap: 12px;
    flex-wrap: wrap;
    padding: 16px 16px;
  }

  .subscribe-page__icon {
    width: 40px;
    height: 40px;
    font-size: 16px;
  }

  .subscribe-page__title {
    font-size: 17px;
  }

  .subscribe-page__bar {
    gap: 8px;
  }

  .subscribe-page__bar .btn {
    flex: 1 1 auto;
    justify-content: center;
  }
}
</style>
