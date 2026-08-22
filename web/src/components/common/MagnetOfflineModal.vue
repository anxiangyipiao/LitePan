<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { accountsApi } from "@/api/accounts";
import { offlineDownloadApi } from "@/api/offlineDownload";
import { getApiErrorMessage } from "@/api/client";
import { toast } from "@/composables/useToast";
import type { Account } from "@/api/types";
import type { OfflineDownloadCapabilities, OfflineDownloadTask } from "@/types/offline-download";
import AppModal from "@/components/base/AppModal.vue";
import AppButton from "@/components/base/AppButton.vue";
import AppSelect from "@/components/base/AppSelect.vue";
import SvgIcon from "@/components/icons/SvgIcon.vue";
import FolderPickerModal from "@/components/file/FolderPickerModal.vue";

const props = defineProps<{
  open: boolean;
  magnet: string;
  magnetName?: string;
}>();

const emit = defineEmits<{
  close: [];
  created: [tasks: OfflineDownloadTask[]];
}>();

const accounts = ref<Account[]>([]);
const accountsLoading = ref(false);
const capsByAccount = ref<Record<number, OfflineDownloadCapabilities>>({});
const selectedAccountId = ref<number | null>(null);
const targetParentId = ref("");
const targetDisplayPath = ref("/");
const folderPickerOpen = ref(false);
const submitting = ref(false);

const supportsMagnet = (cap: OfflineDownloadCapabilities | undefined): boolean => {
  if (!cap?.supported) return false;
  const schemes = (cap.url_schemes ?? []).map((s) => s.toLowerCase());
  return schemes.includes("magnet");
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

const selectedCapability = computed<OfflineDownloadCapabilities | null>(() => {
  if (selectedAccountId.value == null) return null;
  return capsByAccount.value[selectedAccountId.value] ?? null;
});

const selectedOptionValue = computed({
  get: () => (selectedAccountId.value == null ? "" : String(selectedAccountId.value)),
  set: (v: string) => {
    selectedAccountId.value = v ? Number(v) : null;
    initTarget();
  },
});

const submitDisabled = computed(() => {
  if (submitting.value) return true;
  if (!props.magnet.trim()) return true;
  if (selectedAccountId.value == null) return true;
  return false;
});

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    void loadAccountsAndCaps();
    initTarget();
  },
);

watch(selectedAccountId, () => initTarget());

function initTarget() {
  const cap = selectedCapability.value;
  const currentIsRoot = !targetParentId.value || targetParentId.value === "0";
  // 首次打开时未选择过目录，保持根；若目标盘不支持根则显示提示
  if (currentIsRoot && cap && !cap.root_target_allowed) {
    targetDisplayPath.value = "来自:离线下载（网盘默认目录）";
    targetParentId.value = "";
    return;
  }
  if (!targetDisplayPath.value) targetDisplayPath.value = "/";
}

function targetResolved(payload: { parentId: string; path: string }) {
  folderPickerOpen.value = false;
  const cap = selectedCapability.value;
  if ((payload.parentId === "" || payload.parentId === "0") && cap && !cap.root_target_allowed) {
    targetParentId.value = "";
    targetDisplayPath.value = "来自:离线下载（网盘默认目录）";
    return;
  }
  targetParentId.value = payload.parentId;
  targetDisplayPath.value = payload.path || "/";
}

async function loadAccountsAndCaps() {
  accountsLoading.value = true;
  try {
    const list = await accountsApi.list();
    accounts.value = list ?? [];
    const caps: Record<number, OfflineDownloadCapabilities> = {};
    await Promise.all(
      list.map(async (acc) => {
        try {
          const cap = await offlineDownloadApi.capabilities(acc.id);
          caps[acc.id] = cap;
        } catch {
          // 不支持离线的账号忽略
        }
      }),
    );
    capsByAccount.value = caps;
    const avail = list.filter((a) => supportsMagnet(caps[a.id]));
    if (avail.length) {
      // 保持已选可用账号，否则默认首个可用
      const stillValid = avail.some((a) => a.id === selectedAccountId.value);
      if (!stillValid) {
        selectedAccountId.value = avail[0].id;
        targetParentId.value = "";
        targetDisplayPath.value = "/";
      }
    } else {
      selectedAccountId.value = null;
    }
  } catch (e) {
    toast.error(getApiErrorMessage(e, "加载账号失败"));
  } finally {
    accountsLoading.value = false;
  }
}

async function submit() {
  if (submitDisabled.value || selectedAccountId.value == null) return;
  const magnet = props.magnet.trim();
  if (!magnet) {
    toast.warning("磁力链为空");
    return;
  }
  submitting.value = true;
  try {
    const tasks = await offlineDownloadApi.addURLs({
      account_id: selectedAccountId.value,
      urls: [magnet],
      target_parent_id: targetParentId.value,
      target_display_path: targetDisplayPath.value,
    });
    emit("created", tasks);
    const failed = tasks.filter((t) => t.status === "failed").length;
    if (failed) toast.warning(`${tasks.length - failed} 个任务提交成功，${failed} 个失败`);
    else toast.success("已提交到网盘离线下载");
    emit("close");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "离线下载提交失败"));
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <AppModal :open="open" title="离线到网盘" size="lg" @close="emit('close')">
    <div class="magnet-offline">
      <div v-if="magnetName" class="magnet-offline__name" :title="magnetName">{{ magnetName }}</div>
      <div class="magnet-offline__magnet" :title="magnet">{{ magnet }}</div>

      <div v-if="accountsLoading" class="magnet-offline__loading">正在加载可用网盘…</div>

      <template v-else>
        <label v-if="availableAccounts.length" class="magnet-offline__field">
          <span class="magnet-offline__label">目标网盘账号（仅显示支持离线下载的网盘）</span>
          <AppSelect v-model="selectedOptionValue" :options="accountOptions" />
          <small v-if="selectedCapability" class="magnet-offline__hint">
            支持 {{ selectedCapability.url_schemes.map((s) => s.toUpperCase()).join(" / ") }}
            <template v-if="selectedCapability.supports_batch_urls"> · 可批量</template>
          </small>
        </label>

        <div v-else class="magnet-offline__empty">
          当前无支持离线下载的网盘账号。请先到「存储管理」添加 115 / 光鸭等支持离线的网盘。
        </div>

        <div v-if="availableAccounts.length" class="magnet-offline__target">
          <span class="magnet-offline__target-icon"><SvgIcon name="folder" :size="22" /></span>
          <span class="magnet-offline__target-body">
            <small>保存位置</small>
            <strong :title="targetDisplayPath">{{ targetDisplayPath }}</strong>
          </span>
          <AppButton size="sm" @click="folderPickerOpen = true">更改目录</AppButton>
        </div>
        <div v-if="availableAccounts.length" class="magnet-offline__help">任务完成后会自动刷新该目录的文件缓存。</div>
      </template>
    </div>

    <template #footer>
      <AppButton variant="cancel" @click="emit('close')">取消</AppButton>
      <AppButton variant="primary" :disabled="submitDisabled" @click="submit">
        <SvgIcon name="cloud" :size="17" />
        {{ submitting ? "正在提交…" : "开始离线下载" }}
      </AppButton>
    </template>
  </AppModal>

  <FolderPickerModal
    :open="folderPickerOpen"
    title="选择离线下载目录"
    confirm-text="保存到当前目录"
    :account-id="selectedAccountId"
    :allow-create-folder="true"
    :show-refresh="false"
    @resolve="targetResolved"
    @close="folderPickerOpen = false"
  />
</template>

<style scoped>
.magnet-offline { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
.magnet-offline__name { color: var(--text); font-size: 14px; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.magnet-offline__magnet { color: var(--text-muted); font-size: 12px; word-break: break-all; padding: 8px 10px; border: 1px solid var(--border-soft); border-radius: 8px; background: var(--surface-sunken); max-height: 84px; overflow: auto; }
.magnet-offline__loading, .magnet-offline__empty { color: var(--text-muted); font-size: 13px; padding: 12px 0; text-align: center; }
.magnet-offline__field { display: flex; flex-direction: column; gap: 8px; }
.magnet-offline__label { color: var(--text); font-size: 13px; font-weight: 600; }
.magnet-offline__hint { color: var(--text-muted); font-size: 12px; }
.magnet-offline__target { display: flex; align-items: center; gap: 10px; padding: 11px 12px; border: 1px solid var(--border); border-radius: 10px; background: var(--surface-sunken); }
.magnet-offline__target-icon { color: var(--brand); }
.magnet-offline__target-body { min-width: 0; flex: 1; display: flex; flex-direction: column; gap: 2px; }
.magnet-offline__target-body small { color: var(--text-muted); font-size: 12px; }
.magnet-offline__target-body strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text); font-size: 13px; }
.magnet-offline__help { color: var(--text-muted); font-size: 12px; margin-top: -6px; padding-left: 4px; }
</style>
