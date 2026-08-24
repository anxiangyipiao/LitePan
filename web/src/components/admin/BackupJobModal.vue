<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { accountsApi } from "@/api/accounts";
import { getApiErrorMessage } from "@/api/client";
import {
  createBackupJob,
  updateBackupJob,
  type BackupJob,
  type BackupJobInput,
  type BackupScheduleMode,
} from "@/api/backup";
import { toast } from "@/composables/useToast";
import type { Account } from "@/api/types";
import AppModal from "@/components/base/AppModal.vue";
import AppButton from "@/components/base/AppButton.vue";
import AppInput from "@/components/base/AppInput.vue";
import AppSelect from "@/components/base/AppSelect.vue";
import SettingsBoolSegment from "@/components/admin/SettingsBoolSegment.vue";
import LocalDirBrowserModal from "@/components/common/LocalDirBrowserModal.vue";
import FolderPickerModal from "@/components/file/FolderPickerModal.vue";

const props = defineProps<{
  open: boolean;
  job?: BackupJob | null;
}>();

const emit = defineEmits<{
  close: [];
  saved: [job: BackupJob];
}>();

const form = reactive<BackupJobInput>({
  name: "",
  source_path: "",
  target_account_id: 0,
  target_parent_id: "",
  target_display_path: "/",
  method: "sha1",
  schedule_mode: "manual",
  time: "",
  start_time: "",
  interval_hours: 24,
  enabled: true,
});

const saving = ref(false);
const accounts = ref<Account[]>([]);
const accountsLoading = ref(false);
const localPickerOpen = ref(false);
const folderPickerOpen = ref(false);

const isEdit = computed(() => props.job != null);

const accountOptions = computed(() =>
  accounts.value.map((a) => ({
    value: String(a.id),
    label: `${a.name}（${a.driver_card_name?.trim() || a.driver_type}）`,
  })),
);

const accountValue = computed({
  get: () => (form.target_account_id ? String(form.target_account_id) : ""),
  set: (v: string) => {
    form.target_account_id = v ? Number(v) : 0;
    form.target_parent_id = "";
    form.target_display_path = "/";
  },
});

const saveDisabled = computed(() => {
  if (saving.value) return true;
  if (!form.name.trim()) return true;
  if (!form.source_path.trim()) return true;
  if (!form.target_account_id || !form.target_parent_id) return true;
  if (form.schedule_mode === "daily" && !/^\d{1,2}:\d{2}$/.test(form.time.trim())) return true;
  if (form.schedule_mode === "interval" && !/^\d{1,2}:\d{2}$/.test(form.start_time.trim())) return true;
  return false;
});

const scheduleModeOptions = [
  { value: "manual", label: "手动（仅在点击「立即备份」时执行）" },
  { value: "daily", label: "每天定时" },
  { value: "interval", label: "每隔 N 小时" },
];

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    resetForm();
    void loadAccounts();
  },
);

function resetForm() {
  const j = props.job;
  form.name = j?.name ?? "";
  form.source_path = j?.source_path ?? "";
  form.target_account_id = j?.target_account_id ?? 0;
  form.target_parent_id = j?.target_parent_id ?? "";
  form.target_display_path = j?.target_display_path ?? "/";
  form.method = (j?.method as BackupJobInput["method"]) ?? "sha1";
  form.schedule_mode = (j?.schedule_mode as BackupScheduleMode) ?? "manual";
  form.time = j?.time ?? "";
  form.start_time = j?.start_time ?? "";
  form.interval_hours = j?.interval_hours ?? 24;
  form.enabled = j?.enabled ?? true;
}

async function loadAccounts() {
  accountsLoading.value = true;
  try {
    accounts.value = (await accountsApi.list()) ?? [];
  } catch (e) {
    toast.error(getApiErrorMessage(e, "加载网盘账号失败"));
  } finally {
    accountsLoading.value = false;
  }
}

function pickSourcePath(path: string) {
  form.source_path = path;
  localPickerOpen.value = false;
}

function targetResolved(payload: { parentId: string; path: string }) {
  folderPickerOpen.value = false;
  form.target_parent_id = payload.parentId;
  form.target_display_path = payload.path || "/";
}

async function save() {
  if (saveDisabled.value) return;
  saving.value = true;
  try {
    const payload: BackupJobInput = {
      ...form,
      time: form.time.trim(),
      start_time: form.start_time.trim(),
      interval_hours: Number(form.interval_hours || 0),
    };
    const job = isEdit.value
      ? await updateBackupJob(props.job!.id, payload)
      : await createBackupJob(payload);
    toast.success(isEdit.value ? "备份任务已更新" : "备份任务已创建");
    emit("saved", job);
    emit("close");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存失败"));
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <AppModal :open="open" :title="isEdit ? '编辑备份任务' : '新增备份任务'" size="lg" @close="emit('close')">
    <div class="backup-modal">
      <div class="backup-modal__grid">
        <label class="backup-modal__field backup-modal__field--span">
          <span class="backup-modal__label">任务名称</span>
          <AppInput v-model="form.name" placeholder="如：照片备份到百度网盘" />
        </label>

        <label class="backup-modal__field backup-modal__field--span">
          <span class="backup-modal__label">本地源目录（要备份的文件夹）</span>
          <div class="backup-modal__row">
            <AppInput
              v-model="form.source_path"
              placeholder="如 /mnt/disk1/photos 或 D:\photos"
              class="backup-modal__grow"
            />
            <AppButton size="sm" variant="secondary" @click="localPickerOpen = true">浏览</AppButton>
          </div>
          <small class="backup-modal__hint">LitePan 所在机器上的绝对路径，支持容器挂载目录（/mnt、/media 等）与本地盘符。</small>
        </label>

        <label class="backup-modal__field backup-modal__field--span">
          <span class="backup-modal__label">目标网盘账号</span>
          <AppSelect
            v-if="!accountsLoading"
            v-model="accountValue"
            :options="accountOptions"
            placeholder="选择目标网盘"
          />
          <div v-else class="backup-modal__hint">正在加载网盘…</div>
          <small v-if="!accountsLoading && !accounts.length" class="backup-modal__hint">
            暂无网盘账号，请先到「存储管理」添加账号。
          </small>
        </label>

        <div v-if="form.target_account_id" class="backup-modal__field backup-modal__field--span">
          <span class="backup-modal__label">目标目录（备份存放位置）</span>
          <div class="backup-modal__row">
            <span class="backup-modal__path" :title="form.target_display_path">
              {{ form.target_display_path || "/" }}
            </span>
            <AppButton size="sm" variant="secondary" @click="folderPickerOpen = true">更改目录</AppButton>
          </div>
        </div>

        <label class="backup-modal__field">
          <span class="backup-modal__label">哈希方法（秒传）</span>
          <AppSelect
            v-model="form.method"
            :options="[
              { value: 'sha1', label: 'SHA1 秒传' },
              { value: 'md5', label: 'MD5 秒传' },
            ]"
          />
        </label>

        <label class="backup-modal__field backup-modal__field--span">
          <span class="backup-modal__label">调度方式</span>
          <AppSelect v-model="form.schedule_mode" :options="scheduleModeOptions" />
        </label>

        <template v-if="form.schedule_mode === 'daily'">
          <label class="backup-modal__field backup-modal__field--span">
            <span class="backup-modal__label">每天触发时间（HH:MM）</span>
            <AppInput v-model="form.time" placeholder="如 03:00" />
          </label>
        </template>

        <template v-if="form.schedule_mode === 'interval'">
          <label class="backup-modal__field">
            <span class="backup-modal__label">首次触发时间（HH:MM）</span>
            <AppInput v-model="form.start_time" placeholder="如 00:00" />
          </label>
          <label class="backup-modal__field">
            <span class="backup-modal__label">间隔（小时）</span>
            <AppInput v-model="form.interval_hours" type="number" min="1" placeholder="24" />
          </label>
        </template>

        <label class="backup-modal__field backup-modal__field--span">
          <span class="backup-modal__label">启用</span>
          <SettingsBoolSegment
            v-model="form.enabled"
            label="启用"
            on-label="启用"
            off-label="停用"
          />
          <small class="backup-modal__hint">
            启用后按调度自动备份；停用仅保留手动「立即备份」。
          </small>
        </label>

        <p class="backup-modal__note backup-modal__field--span">
          增量备份说明：重复运行时跳过内容未变化的文件（按大小+修改时间），只上传新增/修改的文件；目标网盘已存在同内容文件时走秒传免上传。
        </p>
      </div>
    </div>

    <template #footer>
      <AppButton variant="cancel" @click="emit('close')">取消</AppButton>
      <AppButton variant="primary" :disabled="saveDisabled" @click="save">
        {{ saving ? "保存中…" : isEdit ? "保存修改" : "创建任务" }}
      </AppButton>
    </template>
  </AppModal>

  <LocalDirBrowserModal
    :open="localPickerOpen"
    :initial-path="form.source_path"
    title="选择本地源目录"
    confirm-text="选择当前目录"
    @select="pickSourcePath"
    @close="localPickerOpen = false"
  />
  <FolderPickerModal
    :open="folderPickerOpen"
    title="选择备份目标目录"
    confirm-text="保存到当前目录"
    :account-id="form.target_account_id"
    :allow-create-folder="true"
    :show-refresh="false"
    @resolve="targetResolved"
    @close="folderPickerOpen = false"
  />
</template>

<style scoped>
.backup-modal {
  min-width: 0;
}
.backup-modal__grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px 14px;
}
.backup-modal__field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}
.backup-modal__field--span {
  grid-column: 1 / -1;
}
.backup-modal__label {
  color: var(--text);
  font-size: 13px;
  font-weight: 600;
}
.backup-modal__hint {
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.45;
}
.backup-modal__row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.backup-modal__grow {
  flex: 1;
}
.backup-modal__path {
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
.backup-modal__note {
  margin: 0;
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.5;
}
@media (max-width: 720px) {
  .backup-modal__grid {
    grid-template-columns: 1fr;
  }
}
</style>
