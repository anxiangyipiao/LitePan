<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { getApiErrorMessage } from "@/api/client";
import {
  addQBDownload,
  deleteQBDownloads,
  fetchQBDownloads,
  fetchQBSettings,
  saveQBSettings,
  testQB,
  type QBDownloadTask,
  type QBSettings,
} from "@/api/qb";
import AppButton from "@/components/base/AppButton.vue";
import AppInput from "@/components/base/AppInput.vue";
import SettingsBoolSegment from "@/components/admin/SettingsBoolSegment.vue";
import SettingsCard from "@/components/admin/SettingsCard.vue";
import SettingsHelpTooltip from "@/components/admin/SettingsHelpTooltip.vue";
import SettingsRow from "@/components/admin/SettingsRow.vue";
import { toast } from "@/composables/useToast";
import { formatSize } from "@/utils/format";
import "@/styles/admin-shared.css";

const QB_ACCENT = "#2eaadc";

const settings = ref<QBSettings>({
  enabled: false,
  url: "",
  username: "admin",
  password: "",
  save_path: "",
});
const settingsLoaded = ref(false);
const saving = ref(false);
const testing = ref(false);

const magnetText = ref("");
const addSavePath = ref("");
const adding = ref(false);

const tasks = ref<QBDownloadTask[]>([]);
const tasksLoading = ref(false);
const taskError = ref("");
const deleting = ref(false);

const urlLines = computed(() =>
  magnetText.value
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean),
);
const addDisabled = computed(() => adding.value || urlLines.value.length === 0);

const stateLabel: Record<string, string> = {
  pending: "排队",
  running: "下载中",
  seeding: "做种",
  paused: "已暂停",
  error: "错误",
  finished: "完成",
};

async function loadSettings() {
  try {
    settings.value = await fetchQBSettings();
    addSavePath.value = settings.value.save_path || "";
    settingsLoaded.value = true;
  } catch (e) {
    toast.error(getApiErrorMessage(e, "加载 qB 设置失败"));
  }
}

async function save() {
  if (saving.value) return;
  saving.value = true;
  try {
    settings.value = await saveQBSettings({ ...settings.value });
    toast.success("qB 设置已保存");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存 qB 设置失败"));
  } finally {
    saving.value = false;
  }
}

async function testConnection() {
  if (testing.value) return;
  testing.value = true;
  try {
    const result = await testQB();
    if (result.ok) {
      toast.success(result.version ? `qBittorrent 连通正常（${result.version}）` : "qBittorrent 连通正常");
    } else {
      toast.error("qBittorrent 测试未通过");
    }
  } catch (e) {
    toast.error(getApiErrorMessage(e, "qBittorrent 测试失败"));
  } finally {
    testing.value = false;
  }
}

async function addDownloads() {
  if (addDisabled.value) return;
  adding.value = true;
  try {
    await addQBDownload({
      urls: urlLines.value,
      save_path: addSavePath.value.trim(),
    });
    magnetText.value = "";
    toast.success(`已添加 ${urlLines.value.length} 个任务到 qBittorrent`);
    await loadTasks();
  } catch (e) {
    toast.error(getApiErrorMessage(e, "添加下载失败"));
  } finally {
    adding.value = false;
  }
}

async function loadTasks() {
  tasksLoading.value = true;
  taskError.value = "";
  try {
    tasks.value = await fetchQBDownloads();
  } catch (e) {
    taskError.value = getApiErrorMessage(e, "加载任务失败");
  } finally {
    tasksLoading.value = false;
  }
}

async function removeTask(task: QBDownloadTask) {
  if (deleting.value) return;
  deleting.value = true;
  try {
    await deleteQBDownloads({ hashes: [task.hash] });
    toast.success("已删除任务");
    await loadTasks();
  } catch (e) {
    toast.error(getApiErrorMessage(e, "删除任务失败"));
  } finally {
    deleting.value = false;
  }
}

let pollTimer: number | undefined;
onMounted(() => {
  void loadSettings();
  void loadTasks();
  pollTimer = window.setInterval(() => {
    if (!settings.value.enabled) return;
    void loadTasks();
  }, 5000);
});
onUnmounted(() => {
  if (pollTimer) window.clearInterval(pollTimer);
});
</script>

<template>
  <div class="qb-panel" :style="{ '--settings-accent': QB_ACCENT }">
    <SettingsCard title="连接设置" :accent="QB_ACCENT">
      <template #head-actions>
        <AppButton type="button" variant="secondary" size="sm" :disabled="testing" @click="testConnection">
          {{ testing ? "测试中…" : "测试连接" }}
        </AppButton>
      </template>
      <SettingsRow>
        <template #info>
          <div class="settings-row__label">启用</div>
        </template>
        <template #control>
          <SettingsBoolSegment v-model="settings.enabled" label="启用 qBittorrent 下载" />
        </template>
      </SettingsRow>
      <SettingsRow>
        <template #info>
          <div class="settings-row__label">
            <span>WebUI 地址</span>
            <SettingsHelpTooltip title="qBittorrent WebUI 地址">
              <p>qBittorrent Web 管理界面的地址，例如 http://192.168.1.10:8080。</p>
            </SettingsHelpTooltip>
          </div>
        </template>
        <template #control>
          <AppInput v-model="settings.url" placeholder="http://192.168.1.10:8080" autocomplete="off" />
        </template>
      </SettingsRow>
      <SettingsRow>
        <template #info>
          <div class="settings-row__label">用户名</div>
        </template>
        <template #control>
          <AppInput v-model="settings.username" placeholder="admin" autocomplete="off" />
        </template>
      </SettingsRow>
      <SettingsRow>
        <template #info>
          <div class="settings-row__label">密码</div>
        </template>
        <template #control>
          <AppInput v-model="settings.password" type="password" placeholder="不修改请留空" autocomplete="off" />
        </template>
      </SettingsRow>
      <SettingsRow>
        <template #info>
          <div class="settings-row__label">
            <span>默认保存目录</span>
            <SettingsHelpTooltip title="默认保存目录">
              <p>留空则使用 qBittorrent 自身的默认下载目录；填写后新任务默认保存到该目录。</p>
            </SettingsHelpTooltip>
          </div>
        </template>
        <template #control>
          <AppInput v-model="settings.save_path" placeholder="留空使用 qB 默认目录，如 /downloads" autocomplete="off" />
        </template>
      </SettingsRow>
      <div class="qb-panel__save">
        <AppButton type="button" variant="primary" :disabled="saving" @click="save">
          {{ saving ? "保存中…" : "保存设置" }}
        </AppButton>
      </div>
    </SettingsCard>

    <SettingsCard title="添加下载" :accent="QB_ACCENT">
      <SettingsRow>
        <template #info>
          <div class="settings-row__label">
            <span>磁力链接 / HTTP 链接</span>
            <SettingsHelpTooltip title="下载链接">
              <p>每行一个磁力链接（magnet:?xt=…）或 HTTP 下载地址，可批量添加。</p>
            </SettingsHelpTooltip>
          </div>
        </template>
        <template #control>
          <textarea
            v-model="magnetText"
            class="qb-panel__textarea"
            rows="5"
            placeholder="每行一个 magnet: 或 http(s):// 链接"
          />
        </template>
      </SettingsRow>
      <SettingsRow>
        <template #info>
          <div class="settings-row__label">本次保存目录（可选）</div>
        </template>
        <template #control>
          <AppInput v-model="addSavePath" placeholder="留空使用上方默认目录" autocomplete="off" />
        </template>
      </SettingsRow>
      <div class="qb-panel__save">
        <AppButton type="button" variant="primary" :disabled="addDisabled" @click="addDownloads">
          {{ adding ? "添加中…" : `添加到 qBittorrent（${urlLines.length}）` }}
        </AppButton>
      </div>
    </SettingsCard>

    <SettingsCard title="任务列表" :accent="QB_ACCENT">
      <template #head-actions>
        <AppButton type="button" variant="secondary" size="sm" @click="loadTasks">刷新</AppButton>
      </template>
      <p v-if="taskError" class="qb-panel__error">{{ taskError }}</p>
      <div v-else-if="tasksLoading && !tasks.length" class="qb-panel__hint">加载中…</div>
      <div v-else-if="!tasks.length" class="qb-panel__hint">暂无任务</div>
      <table v-else class="qb-panel__table">
        <thead>
          <tr>
            <th>任务</th>
            <th>状态</th>
            <th>进度</th>
            <th>大小</th>
            <th>保存目录</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="task in tasks" :key="task.hash">
            <td class="qb-panel__name" :title="task.name">{{ task.name }}</td>
            <td><span class="qb-panel__state" :class="`qb-panel__state--${task.state}`">{{ stateLabel[task.state] || task.state }}</span></td>
            <td class="qb-panel__progress-cell">
              <div class="qb-panel__progress">
                <div class="qb-panel__progress-bar" :style="{ width: `${task.progress}%` }" />
              </div>
              <span class="qb-panel__progress-text">{{ task.progress }}%</span>
            </td>
            <td>{{ formatSize(task.size) }}</td>
            <td class="qb-panel__path" :title="task.save_path">{{ task.save_path }}</td>
            <td>
              <AppButton type="button" variant="secondary" size="sm" :disabled="deleting" @click="removeTask(task)">
                删除
              </AppButton>
            </td>
          </tr>
        </tbody>
      </table>
    </SettingsCard>
  </div>
</template>

<style scoped>
.qb-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.settings-row__label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
}
.qb-panel__save {
  padding: 4px 0 14px 16px;
}
.qb-panel__textarea {
  width: 100%;
  min-height: 104px;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  line-height: 1.55;
  resize: vertical;
  box-sizing: border-box;
}
.qb-panel__textarea:focus {
  outline: none;
  border-color: var(--brand);
}
.qb-panel__hint {
  padding: 18px 16px;
  color: var(--text-muted);
  font-size: 13px;
}
.qb-panel__error {
  padding: 12px 16px;
  color: var(--danger, #ef4444);
  font-size: 13px;
}
.qb-panel__table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.qb-panel__table th,
.qb-panel__table td {
  padding: 10px 12px;
  text-align: left;
  border-bottom: 1px solid var(--border-soft);
  vertical-align: middle;
}
.qb-panel__table th {
  color: var(--text-muted);
  font-weight: 500;
  white-space: nowrap;
  background: var(--surface-muted);
}
.qb-panel__name {
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
}
.qb-panel__path {
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-muted);
}
.qb-panel__state {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 12px;
  white-space: nowrap;
}
.qb-panel__state--running {
  background: color-mix(in srgb, var(--brand) 16%, transparent);
  color: var(--brand);
}
.qb-panel__state--finished,
.qb-panel__state--seeding {
  background: color-mix(in srgb, var(--success, #22c55e) 16%, transparent);
  color: var(--success, #22c55e);
}
.qb-panel__state--paused {
  background: color-mix(in srgb, var(--text-muted) 18%, transparent);
  color: var(--text-muted);
}
.qb-panel__state--error {
  background: color-mix(in srgb, var(--danger, #ef4444) 16%, transparent);
  color: var(--danger, #ef4444);
}
.qb-panel__progress-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}
.qb-panel__progress {
  flex: 1 1 auto;
  min-width: 80px;
  height: 6px;
  border-radius: 999px;
  background: var(--surface-muted);
  overflow: hidden;
}
.qb-panel__progress-bar {
  height: 100%;
  border-radius: 999px;
  background: var(--brand);
  transition: width 0.3s ease;
}
.qb-panel__progress-text {
  flex: 0 0 auto;
  color: var(--text-muted);
  font-size: 12px;
}
</style>
