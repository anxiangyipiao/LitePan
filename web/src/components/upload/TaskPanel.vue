<template>
    <div class="upload-task-panel">
      <div class="upload-task-panel-header">
        <span>任务面板</span>
        <div class="upload-task-panel-actions">
          <AppIconButton label="关闭" variant="ghost" size="sm" @click="closeUploadTaskPanel">×</AppIconButton>
        </div>
      </div>
      <div class="upload-task-panel-body">
        <div class="upload-task-layout">
          <div class="upload-task-sidebar">
            <div class="upload-task-sidebar__nav">
            <button
              type="button"
              class="upload-task-nav-item"
              :class="{ active: taskPanelCategory === 'upload' }"
              @click="taskPanelCategory = 'upload'"
            >
              <span class="upload-task-nav-icon"><SvgIcon name="upload" :size="16" /></span>
              <span class="upload-task-nav-label">上传任务</span>
              <span
                class="upload-task-nav-badge"
                :class="{ 'is-empty': activeUploadTasks.length === 0 }"
              >{{ activeUploadTasks.length || 0 }}</span>
            </button>
            <button
              type="button"
              class="upload-task-nav-item"
              :class="{ active: taskPanelCategory === 'relay' }"
              @click="taskPanelCategory = 'relay'"
            >
              <span class="upload-task-nav-icon"><SvgIcon name="relay" :size="16" /></span>
              <span class="upload-task-nav-label">跨盘任务</span>
              <span
                class="upload-task-nav-badge"
                :class="{ 'is-empty': activeRelayCount === 0 }"
              >{{ activeRelayCount || 0 }}</span>
            </button>
            <button
              type="button"
              class="upload-task-nav-item"
              :class="{ active: taskPanelCategory === 'offline' }"
              @click="taskPanelCategory = 'offline'"
            >
              <span class="upload-task-nav-icon"><SvgIcon name="cloud" :size="16" /></span>
              <span class="upload-task-nav-label">离线任务</span>
              <span
                class="upload-task-nav-badge"
                :class="{ 'is-empty': offline.activeTasks.value.length === 0 }"
              >{{ offline.activeTasks.value.length || 0 }}</span>
            </button>
            <button
              type="button"
              class="upload-task-nav-item"
              :class="{ active: taskPanelCategory === 'qb' }"
              @click="taskPanelCategory = 'qb'"
            >
              <span class="upload-task-nav-icon"><SvgIcon name="download" :size="16" /></span>
              <span class="upload-task-nav-label">qb 下载</span>
              <span
                class="upload-task-nav-badge"
                :class="{ 'is-empty': qb.activeTasks.value.length === 0 }"
              >{{ qb.activeTasks.value.length || 0 }}</span>
            </button>
            </div>

            <div v-if="taskPanelCategory === 'offline'" class="upload-task-sidebar__footer upload-task-sidebar__footer--note">
              <SvgIcon name="help-circle" :size="15" />
              <span>任务由网盘执行，关闭页面也不会中断。</span>
            </div>
            <div v-else class="upload-task-sidebar__footer">
              <AppIconButton
                icon="help-circle"
                label="查看上传说明"
                title="查看上传说明"
                variant="ghost"
                size="sm"
                @click="openUploadHelp"
              />
              <div class="upload-task-settings-wrap">
                <AppIconButton
                  icon="settings"
                  label="上传设置"
                  title="上传设置"
                  variant="ghost"
                  size="sm"
                  :class="{ 'upload-task-footer-icon--active': settingsOpen }"
                  @click.stop="settingsOpen = !settingsOpen"
                />
                <UploadTaskSettingsPanel
                  v-if="settingsOpen"
                  :open="settingsOpen"
                  :server-concurrency="uploadTaskServerConcurrency"
                  @update:server-concurrency="onConcurrencyUpdated"
                  @close="settingsOpen = false"
                />
              </div>
            </div>
          </div>

          <div class="upload-task-content">
            <template v-if="taskPanelCategory === 'upload'">
            <div class="upload-task-toolbar">
              <div class="upload-task-batch-actions">
                <button
                  v-if="uploadTaskView === 'running'"
                  type="button"
                  class="task-toolbar-btn"
                  :class="{ primary: batchToggleMode === 'resume' }"
                  :disabled="!canBatchToggle"
                  :title="batchToggleTitle"
                  @click="handleBatchToggle"
                >
                  <span class="task-btn-icon">
                    <SvgIcon :name="batchToggleMode === 'pause' ? 'pause' : 'play'" :size="14" />
                  </span>
                  {{ batchToggleLabel }}
                </button>
                <button
                  type="button"
                  class="task-toolbar-btn danger"
                  :disabled="!canBatchDelete"
                  :title="canBatchDelete ? '删除当前页签中的任务' : '当前没有可删除的任务'"
                  @click="handleBatchDelete"
                >
                  <span class="task-btn-icon"><SvgIcon name="trash-button" :size="14" /></span>
                  {{ batchDeleteLabel }}
                </button>
              </div>
              <div class="upload-task-tabs">
                <button
                  type="button"
                  class="upload-task-tab"
                  :class="{ active: uploadTaskView === 'running' }"
                  @click="uploadTaskView = 'running'"
                >
                  进行中
                  <span class="upload-task-tab-count">{{ runningUploadTasks.length }}</span>
                </button>
                <button
                  type="button"
                  class="upload-task-tab"
                  :class="{ active: uploadTaskView === 'completed' }"
                  @click="uploadTaskView = 'completed'"
                >
                  已完成
                  <span class="upload-task-tab-count">{{ completedUploadTasks.length }}</span>
                </button>
              </div>
            </div>

            <AppStateBlock
              v-if="uploadTaskPanelLoading"
              :message="uploadTaskPanelLoadingText"
              loading
              min-height="220px"
            />

            <div v-else-if="filteredUploadTasks.length > 0" class="upload-task-list">
              <div
                v-for="task in filteredUploadTasks"
                :key="task.task_id"
                class="upload-task-item"
                :class="{ completed: ['success', 'skipped'].includes(task.status) }"
              >
                <div class="upload-task-item-main">
                  <DriverIcon
                    class="upload-task-file-icon"
                    :logo="getUploadTaskDriverBadge(task).logo"
                    :color="getUploadTaskDriverBadge(task).color"
                    :name="getUploadTaskDriverBadge(task).name"
                    :title="getUploadTaskDriverBadge(task).title"
                    :size="40"
                  />
                  <div class="upload-task-file-info">
                    <div class="upload-task-title-row">
                      <span class="upload-task-name" :title="task.file_name">{{ task.file_name }}</span>
                      <span
                        v-if="['success', 'skipped', 'failed', 'canceled'].includes(task.status)"
                        class="upload-task-status"
                        :class="`status-${task.status}`"
                      >
                        {{ getUploadTaskStatusText(task.status) }}
                      </span>
                    </div>
                    <div v-if="isUploadTaskActive(task)" class="upload-task-meta">
                      <span class="task-phase-pill is-upload">
                        <span class="phase-dot"></span>{{ getUploadTaskPhaseLabel(task) }}
                      </span>
                      <span v-if="getUploadTaskSpeedText(task)" class="task-chip is-speed">{{ getUploadTaskSpeedText(task) }}</span>
                      <span v-if="formatUploadPart(task)" class="task-chip">{{ formatUploadPart(task) }}</span>
                      <span v-if="shouldShowUploadTaskMetaPercent(task)" class="task-chip is-percent">{{ task.progress || 0 }}%</span>
                    </div>
                    <div v-if="task.error" class="upload-task-error">{{ task.error }}</div>
                  </div>
                </div>
                <div
                  v-if="shouldShowUploadTaskHairline(task)"
                  class="upload-task-hairline"
                >
                  <UploadProgressInner :task="task" />
                </div>
                <div class="upload-task-item-actions">
                  <AppIconButton
                    :icon="getUploadTaskPrimaryActionIcon(task)"
                    label="执行任务操作"
                    variant="secondary"
                    size="sm"
                    :disabled="!canHandleUploadTaskPrimaryAction(task)"
                    :title="getUploadTaskPrimaryActionTitle(task)"
                    @click="handleUploadTaskPrimaryAction(task)"
                  />
                  <AppIconButton
                    icon="trash-button"
                    label="删除任务"
                    variant="danger"
                    size="sm"
                    :disabled="!canDeleteUploadTask(task)"
                    :title="canDeleteUploadTask(task) ? '删除任务' : '当前不可删除'"
                    @click="handleDeleteUploadTask(task)"
                  />
                </div>
              </div>
            </div>

            <AppStateBlock v-else :message="uploadTaskEmptyText" min-height="220px" />
            </template>

            <template v-else-if="taskPanelCategory === 'relay'">
              <div class="upload-task-toolbar">
                <div class="upload-task-batch-actions">
                  <button
                    type="button"
                    class="task-toolbar-btn danger"
                    :disabled="filteredRelayTasks.length === 0"
                    @click="handleBatchDeleteRelayTasks"
                  >
                    <span class="task-btn-icon"><SvgIcon name="trash-button" :size="14" /></span>
                    {{ relayTaskView === 'completed' ? '全部清空' : '全部删除' }}
                  </button>
                </div>
                <div class="upload-task-tabs">
                  <button
                    type="button"
                    class="upload-task-tab"
                    :class="{ active: relayTaskView === 'running' }"
                    @click="relayTaskView = 'running'"
                  >
                    进行中
                    <span class="upload-task-tab-count">{{ runningRelayTasks.length }}</span>
                  </button>
                  <button
                    type="button"
                    class="upload-task-tab"
                    :class="{ active: relayTaskView === 'completed' }"
                    @click="relayTaskView = 'completed'"
                  >
                    已完成
                    <span class="upload-task-tab-count">{{ completedRelayTasks.length }}</span>
                  </button>
                </div>
              </div>

              <div v-if="filteredRelayTasks.length > 0" class="upload-task-list">
                <div
                  v-for="task in filteredRelayTasks"
                  :key="task.task_id"
                  class="upload-task-item"
                  :class="{ completed: ['success', 'failed', 'canceled'].includes(task.status) }"
                >
                  <div class="upload-task-item-main">
                    <DriverIcon
                      class="upload-task-file-icon"
                      :logo="getRelayTaskDriverBadge(task).logo"
                      :color="getRelayTaskDriverBadge(task).color"
                      :name="getRelayTaskDriverBadge(task).name"
                      :title="getRelayTaskDriverBadge(task).title"
                      :size="40"
                    />
                    <div
                      class="upload-task-file-info"
                      :title="`${task.source_account_name || '源盘'} → ${task.target_account_name || '目标盘'}${task.target_display_path ? ' · ' + task.target_display_path : ''}`"
                    >
                      <div class="upload-task-title-row">
                        <span class="upload-task-name" :title="task.file_name">{{ task.file_name }}</span>
                        <span
                          v-if="['success', 'failed', 'canceled'].includes(task.status)"
                          class="upload-task-status"
                          :class="`status-${task.status}`"
                        >{{ getRelayStatusText(task.status) }}</span>
                      </div>
                      <div v-if="isRelayTaskActive(task)" class="upload-task-meta">
                        <span class="task-phase-pill" :class="task.phase === 'downloading' ? 'is-download' : 'is-upload'">
                          <span class="phase-dot"></span>{{ getRelayPhaseLabel(task) }}
                        </span>
                        <span v-if="formatRelaySpeed(task)" class="task-chip is-speed">{{ formatRelaySpeed(task) }}</span>
                        <span v-if="formatRelayPart(task)" class="task-chip">{{ formatRelayPart(task) }}</span>
                        <span v-if="shouldShowRelayTaskMetaPercent(task)" class="task-chip is-percent">{{ task.progress || 0 }}%</span>
                      </div>
                      <div v-if="task.error" class="upload-task-error">{{ task.error }}</div>
                    </div>
                  </div>
                  <div
                    v-if="shouldShowRelayTaskHairline(task)"
                    class="upload-task-hairline"
                  >
                    <UploadProgressInner :task="task" />
                  </div>
                  <div class="upload-task-item-actions">
                    <AppIconButton
                      icon="trash-button"
                      label="删除任务"
                      variant="danger"
                      size="sm"
                      title="删除任务"
                      @click="handleDeleteRelayTask(task)"
                    />
                  </div>
                </div>
              </div>
              <AppStateBlock
                v-else
                :message="relayTaskView === 'completed' ? '暂无已完成跨盘任务' : '暂无进行中的跨盘任务'"
                min-height="220px"
              />
            </template>

            <template v-else-if="taskPanelCategory === 'offline'">
              <div class="upload-task-toolbar">
                <div class="upload-task-batch-actions">
                  <button
                    type="button"
                    class="task-toolbar-btn"
                    :disabled="offline.refreshing.value"
                    @click="offline.refreshTasks"
                  >
                    <span class="task-btn-icon" :class="{ spin: offline.refreshing.value }"><SvgIcon name="refresh" :size="14" /></span>
                    刷新
                  </button>
                  <button
                    type="button"
                    class="task-toolbar-btn danger"
                    :disabled="offline.deletableTasks.value.length === 0"
                    @click="offline.batchDelete"
                  >
                    <span class="task-btn-icon"><SvgIcon name="trash-button" :size="14" /></span>
                    {{ offline.taskView.value === 'completed' ? '全部清空' : '全部删除' }}
                  </button>
                </div>
                <div class="upload-task-tabs">
                  <button
                    type="button"
                    class="upload-task-tab"
                    :class="{ active: offline.taskView.value === 'running' }"
                    @click="offline.taskView.value = 'running'"
                  >
                    进行中
                    <span class="upload-task-tab-count">{{ offline.runningTasks.value.length }}</span>
                  </button>
                  <button
                    type="button"
                    class="upload-task-tab"
                    :class="{ active: offline.taskView.value === 'completed' }"
                    @click="offline.taskView.value = 'completed'"
                  >
                    已完成
                    <span class="upload-task-tab-count">{{ offline.completedTasks.value.length }}</span>
                  </button>
                </div>
              </div>

              <AppStateBlock
                v-if="offline.loading.value"
                message="正在加载离线下载任务…"
                loading
                min-height="220px"
              />
              <div v-else-if="offline.filteredTasks.value.length > 0" class="upload-task-list">
                <div
                  v-for="task in offline.filteredTasks.value"
                  :key="task.task_id"
                  class="upload-task-item"
                  :class="{ completed: task.status === 'success' }"
                >
                  <div class="upload-task-item-main">
                    <DriverIcon
                      class="upload-task-file-icon"
                      :logo="getUploadTaskDriverBadge(task).logo"
                      :color="getUploadTaskDriverBadge(task).color"
                      :name="getUploadTaskDriverBadge(task).name"
                      :title="getUploadTaskDriverBadge(task).title"
                      :size="40"
                    />
                    <div class="upload-task-file-info">
                      <div class="upload-task-title-row">
                        <span class="upload-task-name" :title="task.name">{{ task.name }}</span>
                        <span
                          v-if="['success', 'failed'].includes(task.status)"
                          class="upload-task-status"
                          :class="`status-${task.status}`"
                        >{{ offline.statusText(task) }}</span>
                      </div>
                      <div v-if="task.status !== 'success' && task.status !== 'failed'" class="upload-task-meta">
                        <span class="task-phase-pill is-download"><span class="phase-dot"></span>{{ offline.statusText(task) }}</span>
                        <span class="task-chip">{{ offline.sourceLabel(task) }} · {{ task.progress || 0 }}%</span>
                        <span v-if="task.size > 0" class="task-chip">{{ formatSize(task.size) }}</span>
                        <span class="task-chip" :title="task.target_display_path">{{ task.target_display_path }}</span>
                      </div>
                      <div v-else class="upload-task-meta">
                        <span class="task-chip">{{ offline.sourceLabel(task) }}</span>
                        <span v-if="task.size > 0" class="task-chip">{{ formatSize(task.size) }}</span>
                        <span class="task-chip" :title="task.target_display_path">{{ task.target_display_path }}</span>
                      </div>
                      <div v-if="task.error" class="upload-task-error">{{ task.error }}</div>
                    </div>
                  </div>
                  <div v-if="task.status !== 'success' && task.status !== 'failed'" class="upload-task-hairline">
                    <span class="upload-task-progress-inner" :style="{ width: `${task.progress || 0}%` }" />
                  </div>
                  <div class="upload-task-item-actions">
                    <AppIconButton
                      v-if="task.status === 'success' || task.status === 'failed' || task.remote_delete"
                      icon="trash-button"
                      :label="task.status === 'success' || task.status === 'failed' ? '删除任务记录' : '取消离线任务'"
                      variant="danger"
                      size="sm"
                      :title="task.status === 'success' || task.status === 'failed' ? '删除任务记录' : '取消网盘端任务'"
                      @click="offline.deleteTask(task)"
                    />
                    <AppIconButton
                      v-else
                      icon="help-circle"
                      label="当前网盘不能取消进行中的任务"
                      variant="secondary"
                      size="sm"
                      title="当前网盘官方接口不能取消进行中的任务"
                      disabled
                    />
                  </div>
                </div>
              </div>
              <AppStateBlock
                v-else
                :message="offline.taskView.value === 'completed' ? '暂无已完成离线任务' : '暂无进行中的离线任务'"
                min-height="220px"
              />
            </template>

            <template v-else>
              <div class="upload-task-toolbar">
                <div class="upload-task-batch-actions">
                  <button
                    type="button"
                    class="task-toolbar-btn"
                    :disabled="qb.refreshing.value"
                    @click="qb.refreshTasks"
                  >
                    <span class="task-btn-icon" :class="{ spin: qb.refreshing.value }"><SvgIcon name="refresh" :size="14" /></span>
                    刷新
                  </button>
                  <button
                    type="button"
                    class="task-toolbar-btn danger"
                    :disabled="qb.filteredTasks.value.length === 0"
                    @click="qb.batchDelete"
                  >
                    <span class="task-btn-icon"><SvgIcon name="trash-button" :size="14" /></span>
                    {{ qb.taskView.value === 'completed' ? '全部清空' : '全部删除' }}
                  </button>
                </div>
                <div class="upload-task-tabs">
                  <button
                    type="button"
                    class="upload-task-tab"
                    :class="{ active: qb.taskView.value === 'running' }"
                    @click="qb.taskView.value = 'running'"
                  >
                    进行中
                    <span class="upload-task-tab-count">{{ qb.runningTasks.value.length }}</span>
                  </button>
                  <button
                    type="button"
                    class="upload-task-tab"
                    :class="{ active: qb.taskView.value === 'completed' }"
                    @click="qb.taskView.value = 'completed'"
                  >
                    已完成
                    <span class="upload-task-tab-count">{{ qb.completedTasks.value.length }}</span>
                  </button>
                </div>
              </div>

              <div v-if="qb.filteredTasks.value.length > 0" class="upload-task-list">
                <div
                  v-for="task in qb.filteredTasks.value"
                  :key="task.hash"
                  class="upload-task-item"
                  :class="{ completed: ['finished', 'seeding', 'paused'].includes(task.state) }"
                >
                  <div class="upload-task-item-main">
                    <span class="upload-task-file-icon qb-task-file-icon">
                      <SvgIcon name="download" :size="18" />
                    </span>
                    <div class="upload-task-file-info">
                      <div class="upload-task-title-row">
                        <span class="upload-task-name" :title="task.name">{{ task.name }}</span>
                        <span
                          v-if="['finished', 'seeding', 'paused', 'error'].includes(task.state)"
                          class="upload-task-status"
                          :class="`status-${task.state === 'error' ? 'failed' : task.state === 'finished' ? 'success' : 'paused'}`"
                        >{{ qb.statusText(task) }}</span>
                      </div>
                      <div v-if="['pending', 'running'].includes(task.state)" class="upload-task-meta">
                        <span class="task-phase-pill is-download"><span class="phase-dot"></span>{{ qb.statusText(task) }}</span>
                        <span class="task-chip is-percent">{{ task.progress || 0 }}%</span>
                        <span v-if="task.size > 0" class="task-chip">{{ formatSize(task.size) }}</span>
                        <span v-if="task.save_path" class="task-chip" :title="task.save_path">{{ task.save_path }}</span>
                      </div>
                      <div v-else class="upload-task-meta">
                        <span v-if="task.size > 0" class="task-chip">{{ formatSize(task.size) }}</span>
                        <span v-if="task.save_path" class="task-chip" :title="task.save_path">{{ task.save_path }}</span>
                      </div>
                      <div v-if="task.error" class="upload-task-error">{{ task.error }}</div>
                    </div>
                  </div>
                  <div v-if="['pending', 'running'].includes(task.state)" class="upload-task-hairline">
                    <span class="upload-task-progress-inner" :style="{ width: `${task.progress || 0}%` }" />
                  </div>
                  <div class="upload-task-item-actions">
                    <AppIconButton
                      icon="trash-button"
                      label="删除任务"
                      variant="danger"
                      size="sm"
                      title="删除任务"
                      @click="qb.deleteTask(task)"
                    />
                  </div>
                </div>
              </div>
              <AppStateBlock
                v-else
                :message="qb.taskView.value === 'completed' ? '暂无已完成 qb 下载任务' : '暂无进行中的 qb 下载任务'"
                min-height="220px"
              />
            </template>
          </div>
        </div>
      </div>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import SvgIcon from "@/components/icons/SvgIcon.vue";
import DriverIcon from "@/components/driver/DriverIcon.vue";
import AppIconButton from "@/components/base/AppIconButton.vue";
import AppStateBlock from "@/components/base/AppStateBlock.vue";
import UploadProgressInner from "@/components/upload/UploadProgressInner.vue";
import UploadTaskSettingsPanel from "@/components/upload/UploadTaskSettingsPanel.vue";
import { formatSize } from "@/utils/format";
import "@/styles/upload-task-panel.css";

const props = defineProps<{
  uploadApi: Record<string, unknown>;
  relay: Record<string, unknown>;
  offline: Record<string, unknown>;
  qb: Record<string, unknown>;
}>();

const api = props.uploadApi as Record<string, any>;
const relay = props.relay as Record<string, any>;
const offline = props.offline as Record<string, any>;
const qb = props.qb as Record<string, any>;

const {
  activeUploadTasks,
  batchToggleMode,
  canBatchToggle,
  batchToggleTitle,
  handleBatchToggle,
  batchToggleLabel,
  canBatchDelete,
  handleBatchDelete,
  batchDeleteLabel,
  runningUploadTasks,
  completedUploadTasks,
  uploadTaskPanelLoading,
  uploadTaskPanelLoadingText,
  filteredUploadTasks,
  getUploadTaskDriverBadge,
  isUploadTaskActive,
  getUploadTaskPhaseLabel,
  getUploadTaskSpeedText,
  formatUploadPart,
  shouldShowUploadTaskMetaPercent,
  shouldShowUploadTaskHairline,
  canHandleUploadTaskPrimaryAction,
  getUploadTaskPrimaryActionTitle,
  handleUploadTaskPrimaryAction,
  getUploadTaskPrimaryActionIcon,
  canDeleteUploadTask,
  handleDeleteUploadTask,
  getUploadTaskStatusText,
  uploadTaskEmptyText,
  openUploadNoticeFromPanel,
  closeUploadTaskPanel,
  refreshUploadTaskServerConcurrency,
  getRelayTaskDriverBadge,
  handleDeleteRelayTask,
  handleBatchDeleteRelayTasks,
  getRelayPhaseLabel,
  shouldShowRelayTaskMetaPercent,
  shouldShowRelayTaskHairline,
  isRelayTaskActive,
  getRelayStatusText,
  formatRelaySpeed,
  formatRelayPart,
} = api;

const {
  filteredRelayTasks,
  runningRelayTasks,
  completedRelayTasks,
  activeRelayCount,
} = relay;

const taskPanelCategory = computed({
  get: () => api.taskPanelCategory.value as "upload" | "relay" | "offline" | "qb",
  set: (v: "upload" | "relay" | "offline" | "qb") => {
    api.taskPanelCategory.value = v;
  },
});
const uploadTaskView = computed({
  get: () => api.uploadTaskView.value as "running" | "completed",
  set: (v: "running" | "completed") => {
    api.uploadTaskView.value = v;
  },
});
const relayTaskView = computed({
  get: () => relay.relayTaskView.value as "running" | "completed",
  set: (v: "running" | "completed") => {
    relay.relayTaskView.value = v;
  },
});

const uploadTaskServerConcurrency = computed({
  get: () => api.uploadTaskServerConcurrency.value as number,
  set: (v: number) => {
    api.uploadTaskServerConcurrency.value = v;
  },
});

const settingsOpen = ref(false);

function openUploadHelp() {
  settingsOpen.value = false;
  openUploadNoticeFromPanel();
}

function onConcurrencyUpdated(value: number) {
  uploadTaskServerConcurrency.value = value;
}

function onDocumentClick(event: MouseEvent) {
  if (!settingsOpen.value) return;
  const target = event.target as HTMLElement | null;
  if (target?.closest(".upload-task-settings-wrap")) return;
  settingsOpen.value = false;
}

onMounted(() => {
  document.addEventListener("click", onDocumentClick);
  void refreshUploadTaskServerConcurrency?.();
});

onUnmounted(() => {
  document.removeEventListener("click", onDocumentClick);
});
</script>
