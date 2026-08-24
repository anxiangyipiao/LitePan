<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { accountsApi } from "@/api/accounts";
import { offlineDownloadApi } from "@/api/offlineDownload";
import { getApiErrorMessage } from "@/api/client";
import {
  createRSSSubscription,
  updateRSSSubscription,
  previewRSSFeed,
  type RSSSubscription,
  type RSSSubscriptionInput,
  type RSSPreviewItem,
  type RSSPreviewResult,
  type RSSTargetType,
} from "@/api/rss";
import { toast } from "@/composables/useToast";
import type { Account } from "@/api/types";
import type { OfflineDownloadCapabilities } from "@/types/offline-download";
import AppModal from "@/components/base/AppModal.vue";
import AppButton from "@/components/base/AppButton.vue";
import AppInput from "@/components/base/AppInput.vue";
import AppSelect from "@/components/base/AppSelect.vue";
import BusySpinner from "@/components/base/BusySpinner.vue";
import SettingsBoolSegment from "@/components/admin/SettingsBoolSegment.vue";
import FolderPickerModal from "@/components/file/FolderPickerModal.vue";

const props = defineProps<{
  open: boolean;
  subscription?: RSSSubscription | null;
}>();

const emit = defineEmits<{
  close: [];
  saved: [sub: RSSSubscription];
}>();

const form = reactive<RSSSubscriptionInput>({
  name: "",
  feed_url: "",
  enabled: true,
  title_keyword: "",
  exclude_keywords: "",
  episode_min: 0,
  episode_max: 0,
  quality_keyword: "",
  target_type: "qb",
  qb_save_path: "",
  qb_category: "",
  account_id: 0,
  target_parent_id: "",
  target_display_path: "/",
  convert_torrent_to_magnet: false,
  fetch_interval_minutes: 0,
});

const saving = ref(false);
const accounts = ref<Account[]>([]);
const accountsLoading = ref(false);
const capsByAccount = ref<Record<number, OfflineDownloadCapabilities>>({});
const folderPickerOpen = ref(false);

const previewResult = ref<RSSPreviewResult | null>(null);
const previewLoading = ref(false);
const previewError = ref("");

const supportsMagnet = (cap: OfflineDownloadCapabilities | undefined): boolean => {
  if (!cap?.supported) return false;
  return (cap.url_schemes ?? []).some((s) => s.toLowerCase() === "magnet");
};

const availableAccounts = computed(() =>
  accounts.value.filter((a) => supportsMagnet(capsByAccount.value[a.id])),
);

const accountOptions = computed(() =>
  availableAccounts.value.map((a) => ({
    value: String(a.id),
    label: `${a.name}（${a.driver_card_name?.trim() || a.driver_type}）`,
  })),
);

const accountValue = computed({
  get: () => (form.account_id ? String(form.account_id) : ""),
  set: (v: string) => {
    form.account_id = v ? Number(v) : 0;
    form.target_parent_id = "";
    form.target_display_path = "/";
  },
});

const isEdit = computed(() => props.subscription != null);

const saveDisabled = computed(() => {
  if (saving.value) return true;
  if (!form.feed_url.trim()) return true;
  if (form.target_type === "offline" && !form.account_id) return true;
  return false;
});

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    resetForm();
    void loadAccountsAndCaps();
  },
);

function resetForm() {
  const s = props.subscription;
  form.name = s?.name ?? "";
  form.feed_url = s?.feed_url ?? "";
  form.enabled = s?.enabled ?? true;
  form.title_keyword = s?.title_keyword ?? "";
  form.exclude_keywords = s?.exclude_keywords ?? "";
  form.episode_min = s?.episode_min ?? 0;
  form.episode_max = s?.episode_max ?? 0;
  form.quality_keyword = s?.quality_keyword ?? "";
  form.target_type = (s?.target_type as RSSTargetType) ?? "qb";
  form.qb_save_path = s?.qb_save_path ?? "";
  form.qb_category = s?.qb_category ?? "";
  form.account_id = s?.account_id ?? 0;
  form.target_parent_id = s?.target_parent_id ?? "";
  form.target_display_path = s?.target_display_path ?? "/";
  form.convert_torrent_to_magnet = s?.convert_torrent_to_magnet ?? false;
  form.fetch_interval_minutes = s?.fetch_interval_minutes ?? 0;
  previewResult.value = null;
  previewError.value = "";
}

async function loadAccountsAndCaps() {
  accountsLoading.value = true;
  try {
    const list = (await accountsApi.list()) ?? [];
    accounts.value = list;
    const caps: Record<number, OfflineDownloadCapabilities> = {};
    await Promise.all(
      list.map(async (acc) => {
        try {
          caps[acc.id] = await offlineDownloadApi.capabilities(acc.id);
        } catch {
          // 不支持离线的账号忽略
        }
      }),
    );
    capsByAccount.value = caps;
  } catch (e) {
    toast.error(getApiErrorMessage(e, "加载网盘账号失败"));
  } finally {
    accountsLoading.value = false;
  }
}

function targetResolved(payload: { parentId: string; path: string }) {
  folderPickerOpen.value = false;
  form.target_parent_id = payload.parentId;
  form.target_display_path = payload.path || "/";
}

async function runPreview() {
  const url = form.feed_url.trim();
  if (!url) {
    toast.warning("请先填写订阅地址");
    return;
  }
  previewLoading.value = true;
  previewError.value = "";
  previewResult.value = null;
  try {
    previewResult.value = await previewRSSFeed({
      feed_url: url,
      title_keyword: form.title_keyword,
      exclude_keywords: form.exclude_keywords,
      episode_min: Number(form.episode_min || 0),
      episode_max: Number(form.episode_max || 0),
      quality_keyword: form.quality_keyword,
      limit: 50,
    });
  } catch (e) {
    previewError.value = getApiErrorMessage(e, "抓取订阅源失败");
  } finally {
    previewLoading.value = false;
  }
}

async function save() {
  if (saveDisabled.value) return;
  saving.value = true;
  try {
    const payload: RSSSubscriptionInput = {
      ...form,
      episode_min: Number(form.episode_min || 0),
      episode_max: Number(form.episode_max || 0),
      fetch_interval_minutes: Number(form.fetch_interval_minutes || 0),
    };
    const sub = isEdit.value
      ? await updateRSSSubscription(props.subscription!.id, payload)
      : await createRSSSubscription(payload);
    toast.success(isEdit.value ? "订阅已更新" : "订阅已创建");
    emit("saved", sub);
    emit("close");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存失败"));
  } finally {
    saving.value = false;
  }
}

const matchedCount = computed(() => previewResult.value?.items.filter((i) => i.matched).length ?? 0);

function torrentType(it: RSSPreviewItem): string {
  if (!it.torrent_url) return "无链接";
  return it.torrent_url.toLowerCase().startsWith("magnet:") ? "磁力" : "HTTP 种子";
}
</script>

<template>
  <AppModal :open="open" :title="isEdit ? '编辑订阅' : '新增订阅'" size="lg" @close="emit('close')">
    <div class="rss-modal">
      <div class="rss-modal__grid">
        <label class="rss-modal__field rss-modal__field--span">
          <span class="rss-modal__label">名称</span>
          <AppInput v-model="form.name" placeholder="如：孤独摇滚 第2季" />
        </label>
        <label class="rss-modal__field rss-modal__field--span">
          <span class="rss-modal__label">订阅地址（RSS / Atom）</span>
          <AppInput
            v-model="form.feed_url"
            placeholder="如 https://mikanani.me/RSS/Bangumi?bangumiId=1234"
          />
        </label>

        <label class="rss-modal__field rss-modal__field--span">
          <span class="rss-modal__label">标题关键词（必须包含）</span>
          <AppInput v-model="form.title_keyword" placeholder="如：孤独摇滚" />
        </label>
        <label class="rss-modal__field rss-modal__field--span">
          <span class="rss-modal__label">排除词（空格/逗号分隔，命中即跳过）</span>
          <AppInput v-model="form.exclude_keywords" placeholder="如：生肉 合集 特典" />
        </label>

        <label class="rss-modal__field">
          <span class="rss-modal__label">起始集数（0=不限）</span>
          <AppInput v-model="form.episode_min" type="number" placeholder="0" />
        </label>
        <label class="rss-modal__field">
          <span class="rss-modal__label">结束集数（0=不限）</span>
          <AppInput v-model="form.episode_max" type="number" placeholder="0" />
        </label>
        <label class="rss-modal__field">
          <span class="rss-modal__label">画质关键词</span>
          <AppInput v-model="form.quality_keyword" placeholder="如：1080p" />
        </label>
        <label class="rss-modal__field">
          <span class="rss-modal__label">抓取间隔（分钟，0=用默认）</span>
          <AppInput v-model="form.fetch_interval_minutes" type="number" placeholder="30" />
        </label>

        <label class="rss-modal__field rss-modal__field--span">
          <span class="rss-modal__label">推送目标</span>
          <AppSelect
            v-model="form.target_type"
            :options="[
              { value: 'qb', label: '推送到本地 qBittorrent' },
              { value: 'offline', label: '离线到网盘（自动触发整理/STRM 联动）' },
            ]"
          />
        </label>

        <template v-if="form.target_type === 'qb'">
          <label class="rss-modal__field">
            <span class="rss-modal__label">qB 保存路径（留空用系统设置）</span>
            <AppInput v-model="form.qb_save_path" placeholder="按系统设置" />
          </label>
          <label class="rss-modal__field">
            <span class="rss-modal__label">qB 分类（留空用系统设置）</span>
            <AppInput v-model="form.qb_category" placeholder="按系统设置" />
          </label>
        </template>

        <template v-else>
          <label class="rss-modal__field rss-modal__field--span">
            <span class="rss-modal__label">目标网盘账号（仅显示支持离线下载的网盘）</span>
            <AppSelect
              v-if="!accountsLoading"
              v-model="accountValue"
              :options="accountOptions"
              placeholder="选择网盘账号"
            />
            <div v-else class="rss-modal__hint">正在加载网盘…</div>
            <small v-if="!accountsLoading && !availableAccounts.length" class="rss-modal__hint">
              暂无支持离线下载的账号，请先到「存储管理」添加 115 / 光鸭等网盘。
            </small>
          </label>
          <div v-if="form.account_id" class="rss-modal__target rss-modal__field--span">
            <span class="rss-modal__label">保存位置</span>
            <div class="rss-modal__target-row">
              <span class="rss-modal__target-path" :title="form.target_display_path">
                {{ form.target_display_path || "/" }}
              </span>
              <AppButton size="sm" variant="secondary" @click="folderPickerOpen = true">
                更改目录
              </AppButton>
            </div>
          </div>
          <label class="rss-modal__field rss-modal__field--span">
            <span class="rss-modal__label">HTTP 种子转磁力</span>
            <SettingsBoolSegment
              v-model="form.convert_torrent_to_magnet"
              label="HTTP 种子转磁力"
              on-label="转换"
              off-label="跳过"
            />
            <small class="rss-modal__hint">
              仅提供 http .torrent 链接的条目：开启会下载种子解析 infohash 转成磁力链再离线，关闭则直接跳过。
            </small>
          </label>
        </template>
      </div>

      <div class="rss-modal__preview">
        <div class="rss-modal__preview-head">
          <span class="rss-modal__label">匹配预览（保存前先验证）</span>
          <AppButton size="sm" :disabled="previewLoading" @click="runPreview">
            {{ previewLoading ? "抓取中…" : "预览匹配" }}
          </AppButton>
        </div>
        <p v-if="previewError" class="rss-modal__error">{{ previewError }}</p>
        <div v-if="previewLoading" class="rss-modal__loading">
          <BusySpinner :size="18" />
          <span>正在抓取订阅源…</span>
        </div>
        <template v-else-if="previewResult">
          <p class="rss-modal__hint">
            {{ previewResult.feed_title }} · 共 {{ previewResult.total }} 条 · 命中
            {{ matchedCount }} 条
          </p>
          <div class="rss-modal__preview-list">
            <div
              v-for="(it, idx) in previewResult.items"
              :key="idx"
              class="rss-modal__preview-row"
              :class="{ 'is-hit': it.matched }"
            >
              <span
                class="rss-modal__badge"
                :class="it.matched ? 'is-hit' : 'is-miss'"
                :title="it.reason"
              >
                {{ it.matched ? "命中" : "未命中" }}
              </span>
              <span class="rss-modal__ep" :title="it.episode ? '第' + it.episode + '集' : '未解析集数'">
                {{ it.episode ? "EP" + it.episode : "—" }}
              </span>
              <span class="rss-modal__preview-title" :title="it.title">{{ it.title }}</span>
              <span class="rss-modal__type">{{ torrentType(it) }}</span>
            </div>
          </div>
        </template>
        <p v-else class="rss-modal__hint">填入订阅地址后可点「预览匹配」查看命中结果。</p>
      </div>
    </div>

    <template #footer>
      <AppButton variant="cancel" @click="emit('close')">取消</AppButton>
      <AppButton variant="primary" :disabled="saveDisabled" @click="save">
        {{ saving ? "保存中…" : isEdit ? "保存修改" : "创建订阅" }}
      </AppButton>
    </template>
  </AppModal>

  <FolderPickerModal
    :open="folderPickerOpen"
    title="选择离线下载目录"
    confirm-text="保存到当前目录"
    :account-id="form.account_id"
    :allow-create-folder="true"
    :show-refresh="false"
    @resolve="targetResolved"
    @close="folderPickerOpen = false"
  />
</template>

<style scoped>
.rss-modal {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}
.rss-modal__grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px 14px;
}
.rss-modal__field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}
.rss-modal__field--span {
  grid-column: 1 / -1;
}
.rss-modal__label {
  color: var(--text);
  font-size: 13px;
  font-weight: 600;
}
.rss-modal__hint {
  color: var(--text-muted);
  font-size: 12px;
}
.rss-modal__target {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.rss-modal__target-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.rss-modal__target-path {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text);
  font-size: 13px;
  padding: 8px 10px;
  border: 1px solid var(--border-soft);
  border-radius: 8px;
  background: var(--surface-sunken);
}
.rss-modal__preview {
  border: 1px solid var(--border-soft);
  border-radius: 10px;
  padding: 12px;
  background: var(--surface-sunken);
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.rss-modal__preview-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.rss-modal__preview-list {
  max-height: 260px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.rss-modal__preview-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 8px;
  border-radius: 6px;
  background: var(--surface);
}
.rss-modal__preview-row.is-hit {
  background: color-mix(in srgb, var(--success, #22c55e) 8%, var(--surface));
}
.rss-modal__badge {
  flex: 0 0 auto;
  font-size: 11px;
  font-weight: 700;
  padding: 2px 6px;
  border-radius: 5px;
}
.rss-modal__badge.is-hit {
  color: var(--success, #22c55e);
  background: color-mix(in srgb, var(--success, #22c55e) 14%, transparent);
}
.rss-modal__badge.is-miss {
  color: var(--text-muted);
  background: color-mix(in srgb, var(--text-muted) 12%, transparent);
}
.rss-modal__ep {
  flex: 0 0 auto;
  font-size: 12px;
  color: var(--brand-strong, var(--brand));
  font-weight: 600;
}
.rss-modal__preview-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text);
  font-size: 13px;
}
.rss-modal__type {
  flex: 0 0 auto;
  font-size: 11px;
  color: var(--text-muted);
}
.rss-modal__error {
  color: var(--danger, #ef4444);
  font-size: 13px;
}
.rss-modal__loading {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-muted);
  font-size: 13px;
  padding: 8px 0;
}

@media (max-width: 720px) {
  .rss-modal__grid {
    grid-template-columns: 1fr;
  }
}
</style>
