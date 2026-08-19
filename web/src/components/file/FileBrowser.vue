<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch, watchEffect } from "vue";
import { useRoute, useRouter } from "vue-router";
import { storeToRefs } from "pinia";
import { useBrowserStore, type Crumb } from "@/stores/browser";
import { useAuthStore } from "@/stores/auth";
import { useFileSort } from "@/composables/useFileSort";
import { fileKey, useFileSelection } from "@/composables/useFileSelection";
import { useFileActions, type DeleteMode } from "@/composables/useFileActions";
import { showConfirm } from "@/composables/useConfirm";
import { useRelayTasks } from "@/composables/useRelayTasks";
import { useUploadTasks } from "@/composables/useUploadTasks";
import { useOfflineDownloads } from "@/composables/useOfflineDownloads";
import { useTransferBadge, type TransferBadgeKind } from "@/composables/upload/useTransferBadge";
import { toast } from "@/composables/useToast";
import { filesApi } from "@/api/files";
import type { Account, BrowserFavoriteItem, FileItem, FileNameAlignPreviewResult } from "@/api/types";
import type { OfflineDownloadTask } from "@/types/offline-download";
import { getApiErrorMessage } from "@/api/client";
import { generateCurrentDirectoryStrm } from "@/api/strm";
import { useStrmDirectoryPrompt } from "@/composables/useStrmDirectoryPrompt";
import { fileKind } from "@/utils/fileIcon";
import { publicApi } from "@/api/public";
import DriveSidebar from "./DriveSidebar.vue";
import BreadcrumbNav from "./BreadcrumbNav.vue";
import FavoritesSidebar from "./FavoritesSidebar.vue";
import FileToolbar from "./FileToolbar.vue";
import FileTable from "./FileTable.vue";
import FilePreviewHost from "./FilePreviewHost.vue";
import BusySpinner from "@/components/base/BusySpinner.vue";
import type { ActiveFilePreview, FilePreviewKind } from "./filePreview";
import FolderPickerModal from "./FolderPickerModal.vue";
import NameAlignModal from "./NameAlignModal.vue";
import AppModal from "@/components/base/AppModal.vue";
import AppInput from "@/components/base/AppInput.vue";
import TaskPanel from "@/components/upload/TaskPanel.vue";
import OfflineDownloadModal from "./OfflineDownloadModal.vue";
import MobileSidebar from "@/components/mobile/MobileSidebar.vue";
import BottomSheet from "@/components/mobile/BottomSheet.vue";

type FocusableInput = {
  focus: () => void;
  select: () => void;
};

const BROWSER_LOCATION_STORAGE_KEY = "litepan:index:browser-location";
const BROWSER_LOCATION_RESET_ONCE_KEY = "litepan:index:reset-once";

interface BrowserLocationSnapshot {
  accountId: number;
  crumbs: Crumb[];
}

const store = useBrowserStore();
const auth = useAuthStore();
const route = useRoute();
const router = useRouter();
const { accounts, currentAccountId, breadcrumb, favorites, favoritesOpen, files, filesResortTick, loading, refreshing, error, responseTime, cacheRate, currentParentId } =
  storeToRefs(store);
const { isAdmin } = storeToRefs(auth);

const view = ref<"list" | "grid">(
  (localStorage.getItem("litepan_view") as "list" | "grid") || "list",
);
const selectedIds = ref<string[]>([]);
const createFolderRequest = ref(0);
const uploadFileInput = ref<HTMLInputElement | null>(null);
const uploadFolderInput = ref<HTMLInputElement | null>(null);
const favoriteNameModalOpen = ref(false);
const favoriteNameInput = ref("");
const favoriteNameInputRef = ref<FocusableInput | null>(null);
const favoriteNameMode = ref<"create" | "rename">("create");
const favoriteRenameTarget = ref<BrowserFavoriteItem | null>(null);
const browserBootstrapping = ref(true);
const favoritesTransitionReady = ref(false);
const nameAlignOpen = ref(false);
const nameAlignLoading = ref(false);
const nameAlignApplying = ref(false);
const nameAlignError = ref("");
const nameAlignTargetFile = ref<FileItem | null>(null);
const nameAlignPreview = ref<FileNameAlignPreviewResult | null>(null);
const nameAlignSelectedSampleId = ref("");
const nameAlignSuspectIds = ref<string[]>([]);
const nameAlignIncludeSuspects = ref(true);
const nameAlignApplyTotal = ref(0);
const nameAlignApplyProgress = ref(0);
const activePreview = ref<ActiveFilePreview | null>(null);
let nameAlignApplyTimer: number | undefined;

// Mobile UI state
const mobileSidebarOpen = ref(false);
const bottomSheetOpen = ref(false);
const bottomSheetTarget = ref<FileItem | null>(null);
const pullRefreshing = ref(false);
let pullStartY = 0;
let pulling = false;

const selectedAccountName = computed(
  () => accounts.value.find((a) => a.id === currentAccountId.value)?.name || "",
);

const sortAccountKey = computed(() =>
  currentAccountId.value != null ? String(currentAccountId.value) : "",
);
const { sortKey, sortOrder, sortBy, sortClass } = useFileSort(files, sortAccountKey, filesResortTick);
const { selectedCount, selectedFiles } = useFileSelection(files, selectedIds);

function getDeleteMode(): DeleteMode {
  const account = accounts.value.find((a) => a.id === currentAccountId.value);
  if (!account?.config) return "recycle";
  try {
    const cfg = JSON.parse(account.config) as { delete_mode?: string };
    return cfg.delete_mode === "delete" || cfg.delete_mode === "permanent" ? "permanent" : "recycle";
  } catch {
    return "recycle";
  }
}

function getRootId(config: Record<string, unknown>) {
  return String(config.root_folder_id || "0");
}

function getCurrentBreadcrumbNameParts() {
  return breadcrumb.value.map((item) => item.name).filter((name) => name && name !== "根目录");
}

const fileActions = useFileActions({
  getAccountId: () => currentAccountId.value,
  getParentId: () => currentParentId.value,
  files,
  selectedIds,
  selectedFiles,
  getDeleteMode,
  removeFilesLocally: (ids) => store.removeFilesLocally(ids),
  renameFileLocally: (fileId, newName) => store.renameFileLocally(fileId, newName),
  addFolderLocally: (folder) => store.addFolderLocally(folder),
  reloadFiles: (opts) => store.loadFiles({ ...opts, silent: true }),
});

const relay = useRelayTasks();
const uploadApi = useUploadTasks({
  selectedAccountId: currentAccountId,
  selectedAccountName,
  accounts,
  currentPath: currentParentId,
  breadcrumbItems: breadcrumb,
  selectedFilesList: selectedFiles,
  files,
  uploadFileInput,
  uploadFolderInput,
  refreshFiles: (force?: boolean) =>
    store.loadFiles({ forceRefresh: Boolean(force), silent: true }),
  loadFiles: (opts) => store.loadFiles(opts),
  openDirectory: (accountId, crumbs, opts) => store.openDirectory(accountId, crumbs, opts),
  selectAccount: (account: Account) => store.selectAccount(account.id),
  getRootId,
  getCurrentBreadcrumbNameParts,
  relay,
});
const { uploadTaskPanelOpen } = uploadApi;
const offline = useOfflineDownloads({
  selectedAccountId: currentAccountId,
  currentParentId,
  refreshFiles: () => store.loadFiles({ forceRefresh: true, silent: true }),
});
const uploadTaskFailed = computed(
  () =>
    uploadApi.displayUploadTasks.value.some((task) => task.status === "failed") ||
    uploadApi.runningRelayTasks.value.some((task) => task.status === "failed") ||
    uploadApi.completedRelayTasks.value.some((task) => task.status === "failed") ||
    offline.failedTasks.value.length > 0,
);
const uploadTaskSuccess = computed(() =>
  uploadApi.displayUploadTasks.value.some((task) => task.status === "success") ||
  offline.successfulTasks.value.length > 0,
);

// 顶栏传输角标：任务计数喂给 useTransferBadge（AppHeader 读取）；点击顶栏角标时打开任务面板。
const transferBadge = useTransferBadge();
watchEffect(() => {
  const active =
    uploadApi.activeUploadTasks.value.length +
    uploadApi.activeRelayCount.value +
    offline.activeTasks.value.length;
  let kind: TransferBadgeKind = "";
  if (active > 0) kind = "active";
  else if (uploadTaskFailed.value) kind = "failed";
  else if (uploadTaskSuccess.value) kind = "success";
  transferBadge.setBadge(active, kind);
});
watch(
  () => transferBadge.open.value,
  (v) => {
    if (v) {
      transferBadge.setOpen(false);
      void openTaskPanel();
    }
  },
);
const showFavorites = computed(() => isAdmin.value && favoritesOpen.value);
const currentCrumbIds = computed(() => breadcrumb.value.map((item) => item.id));
const currentFolderFavorited = computed(
  () =>
    favorites.value.some(
      (item) =>
        item.account_id === currentAccountId.value && item.id === currentParentId.value,
    ) && currentParentId.value !== "",
);
const currentFolderName = computed(() => breadcrumb.value[breadcrumb.value.length - 1]?.name || "");

const dragMove = reactive({
  active: false,
  files: [] as FileItem[],
  targetId: "",
});

const transferTitle = computed(() =>
  fileActions.transfer.action === "move" ? "移动到" : "复制到",
);
const transferConfirmText = computed(() =>
  fileActions.transfer.action === "move" ? "移动到此目录" : "复制到此目录",
);

const strmGenerating = ref(false);
const strmAutoDetectEnabled = ref(true);

const strmPrompt = useStrmDirectoryPrompt({
  isAdmin,
  accountId: currentAccountId,
  files,
  loading,
  refreshing,
  enabled: strmAutoDetectEnabled,
  getDisplayPath: getCurrentDisplayPath,
  getParentId: () => currentParentId.value,
});

function getCurrentDisplayPath(): string {
  const parts = getCurrentBreadcrumbNameParts();
  return parts.length ? `/${parts.join("/")}` : "/";
}

function resetDragMove() {
  dragMove.active = false;
  dragMove.files = [];
  dragMove.targetId = "";
}

function resolveDraggedFiles(startFile: FileItem) {
  const startKey = fileKey(startFile);
  const startIsSelected = selectedIds.value.includes(startKey);
  if (startIsSelected && selectedFiles.value.length > 0) {
    return [...selectedFiles.value];
  }
  return [startFile];
}

function canDropToParent(targetParentId: string, ancestorIds: string[] = []) {
  if (!dragMove.active || !targetParentId) return false;
  if (targetParentId === currentParentId.value) return false;
  const draggedIds = new Set(dragMove.files.map((file) => file.id));
  if (draggedIds.has(targetParentId)) return false;
  const ancestorSet = new Set(ancestorIds.filter(Boolean));
  return !dragMove.files.some((file) => file.is_dir && ancestorSet.has(file.id));
}

function canDropOnFolder(file: FileItem) {
  return file.is_dir && canDropToParent(file.id);
}

// 全局收藏跨账号，用 账号+文件夹ID 作为拖放目标键
function favoriteDropKey(item: BrowserFavoriteItem) {
  return `${item.account_id ?? 0}:${item.id}`;
}

function canDropOnFavorite(item: BrowserFavoriteItem) {
  // 仅当前账号的收藏可作为拖放目标，跨盘收藏不接受文件拖入
  if (item.account_id != null && item.account_id !== currentAccountId.value) return false;
  return canDropToParent(item.id, item.crumbs.map((crumb) => crumb.id));
}

function startDragMove(file: FileItem) {
  if (!isAdmin.value) return;
  dragMove.active = true;
  dragMove.files = resolveDraggedFiles(file);
  dragMove.targetId = "";
}

function finishDragMove() {
  resetDragMove();
}

function handleFolderDragEnter(file: FileItem) {
  if (!canDropOnFolder(file)) {
    if (dragMove.targetId === file.id) dragMove.targetId = "";
    return;
  }
  dragMove.targetId = file.id;
}

function handleFolderDragLeave(file: FileItem) {
  if (dragMove.targetId === file.id) {
    dragMove.targetId = "";
  }
}

async function handleFolderDrop(file: FileItem) {
  if (!canDropOnFolder(file)) {
    resetDragMove();
    return;
  }
  const targets = [...dragMove.files];
  resetDragMove();
  await fileActions.moveTargetsToParent(targets, file.id);
}

function handleFavoriteDragEnter(item: BrowserFavoriteItem) {
  if (!canDropOnFavorite(item)) {
    if (dragMove.targetId === favoriteDropKey(item)) dragMove.targetId = "";
    return;
  }
  dragMove.targetId = favoriteDropKey(item);
}

function handleFavoriteDragLeave(item: BrowserFavoriteItem) {
  if (dragMove.targetId === favoriteDropKey(item)) {
    dragMove.targetId = "";
  }
}

async function handleFavoriteDrop(item: BrowserFavoriteItem) {
  if (!canDropOnFavorite(item)) {
    resetDragMove();
    return;
  }
  const targets = [...dragMove.files];
  resetDragMove();
  await fileActions.moveTargetsToParent(targets, item.id);
}

async function handleGenerateCurrentDirectoryStrm() {
  if (!currentAccountId.value) {
    toast.info("请先选择一个账号");
    return;
  }
  if (strmGenerating.value) return;
  strmGenerating.value = true;
  try {
    const result = await generateCurrentDirectoryStrm({
      account_id: currentAccountId.value,
      parent_id: currentParentId.value,
      path: getCurrentDisplayPath(),
      items: files.value.map((file) => ({
        id: file.id,
        name: file.name,
        size: file.size,
        is_dir: file.is_dir,
      })),
    });
    if (
      (result.media_count || 0) <= 0 &&
      (result.deleted || 0) <= 0 &&
      (result.metadata_created || 0) <= 0 &&
      (result.metadata_uploaded || 0) <= 0 &&
      (result.metadata_deleted || 0) <= 0
    ) {
      toast.info("当前目录没有需要同步的 STRM");
      return;
    }
    const parts = [
      `新增 ${result.created || 0}`,
      `更新 ${result.updated || 0}`,
      `删除 ${result.deleted || 0}`,
      `已存在 ${result.skipped_existing || 0}`,
    ];
    if ((result.metadata_created || 0) > 0) {
      parts.push(`元数据下载 ${result.metadata_created}`);
    }
    if ((result.metadata_uploaded || 0) > 0) {
      parts.push(`元数据上传 ${result.metadata_uploaded}`);
    }
    if ((result.metadata_deleted || 0) > 0) {
      parts.push(`元数据清理 ${result.metadata_deleted}`);
    }
    if ((result.skipped_conflict || 0) > 0) {
      parts.push(`冲突跳过 ${result.skipped_conflict}`);
    }
    toast.success(`STRM 同步完成：${parts.join("，")}`);
  } catch (error) {
    toast.error(getApiErrorMessage(error, "当前目录 STRM 生成失败"));
  } finally {
    strmGenerating.value = false;
    await strmPrompt.refreshStatus();
  }
}

function handleConfirmStrmPrompt() {
  void handleGenerateCurrentDirectoryStrm();
}

function handleDismissStrmPrompt() {
  strmPrompt.dismissPrompt();
}

function startCreateFolder() {
  if (!currentAccountId.value) {
    toast.info("请先选择一个账号");
    return;
  }
  createFolderRequest.value += 1;
}

function setView(v: "list" | "grid") {
  view.value = v;
  localStorage.setItem("litepan_view", v);
}

// Mobile: open bottom sheet for file operations
function openFileBottomSheet(file: FileItem) {
  bottomSheetTarget.value = file;
  bottomSheetOpen.value = true;
}
function closeBottomSheet() {
  bottomSheetOpen.value = false;
  bottomSheetTarget.value = null;
}
function bottomSheetAction(action: string) {
  const file = bottomSheetTarget.value;
  if (!file) return;
  closeBottomSheet();
  switch (action) {
    case "rename": fileActions.renameFile(file, file.name); break;
    case "download": fileActions.downloadFile(file); break;
    case "move": fileActions.requestSingleMove(file); break;
    case "copy": fileActions.requestSingleCopy(file); break;
    case "delete": fileActions.deleteFile(file); break;
    case "strm": openNameAlign(file); break;
  }
}

// Mobile: pull-to-refresh
function onPullStart(e: TouchEvent) {
  pullStartY = e.touches[0].clientY;
  pulling = true;
}
function onPullMove(e: TouchEvent) {
  if (!pulling) return;
  const delta = e.touches[0].clientY - pullStartY;
  if (delta > 60 && !refreshing.value && !loading.value) {
    pullRefreshing.value = true;
  }
}
function onPullEnd() {
  if (pullRefreshing.value) {
    store.refreshFiles();
    setTimeout(() => { pullRefreshing.value = false; }, 1500);
  }
  pulling = false;
}

function normalizeCrumbs(raw: unknown): Crumb[] | null {
  if (!Array.isArray(raw)) return null;
  const crumbs = raw
    .map((item) => {
      if (!item || typeof item !== "object") return null;
      const { id, name } = item as Record<string, unknown>;
      if (typeof id !== "string" || typeof name !== "string") return null;
      return { id, name };
    })
    .filter((item): item is Crumb => item !== null);
  return crumbs.length ? crumbs : null;
}

function loadSavedBrowserLocation(): BrowserLocationSnapshot | null {
  const raw = localStorage.getItem(BROWSER_LOCATION_STORAGE_KEY);
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw) as {
      accountId?: unknown;
      crumbs?: unknown;
    };
    if (typeof parsed.accountId !== "number" || !Number.isFinite(parsed.accountId)) {
      return null;
    }
    const crumbs = normalizeCrumbs(parsed.crumbs);
    if (!crumbs) return null;
    return {
      accountId: parsed.accountId,
      crumbs,
    };
  } catch {
    return null;
  }
}

function persistBrowserLocation() {
  if (currentAccountId.value == null) {
    localStorage.removeItem(BROWSER_LOCATION_STORAGE_KEY);
    return;
  }
  const snapshot: BrowserLocationSnapshot = {
    accountId: currentAccountId.value,
    crumbs: breadcrumb.value.map((item) => ({ id: item.id, name: item.name })),
  };
  localStorage.setItem(BROWSER_LOCATION_STORAGE_KEY, JSON.stringify(snapshot));
}

function consumeResetBrowserLocationOnce() {
  if (sessionStorage.getItem(BROWSER_LOCATION_RESET_ONCE_KEY) !== "1") {
    return false;
  }
  sessionStorage.removeItem(BROWSER_LOCATION_RESET_ONCE_KEY);
  localStorage.removeItem(BROWSER_LOCATION_STORAGE_KEY);
  return true;
}

function hasPendingBrowserLocationReset() {
  return sessionStorage.getItem(BROWSER_LOCATION_RESET_ONCE_KEY) === "1";
}

async function restoreBrowserLocation() {
  const saved = loadSavedBrowserLocation();
  if (!saved) return false;
  if (!accounts.value.some((account) => account.id === saved.accountId)) {
    return false;
  }
  await store.openDirectory(saved.accountId, saved.crumbs, { silent: true });
  if (error.value) {
    await store.selectAccount(saved.accountId);
  }
  return true;
}

async function loadPublicSystemConfig() {
  try {
    const cfg = await publicApi.systemConfig();
    strmAutoDetectEnabled.value = cfg.index_strm_auto_detect_enabled ?? true;
  } catch {
    strmAutoDetectEnabled.value = true;
  }
}

async function loadInitialTaskState() {
  if (!isAdmin.value) return;
  try {
    await Promise.allSettled([
      uploadApi.fetchUploadTasks(),
      relay.fetchRelayTasks(),
      offline.fetchTasks(true, true),
    ]);
    if (relay.activeRelayCount.value > 0) {
      await relay.openRelayMonitoring();
    }
    if (route.query.taskPanel === "relay") {
      await uploadApi.openUploadTaskPanel("relay");
      const nextQuery = { ...route.query };
      delete nextQuery.taskPanel;
      router.replace({ path: route.path, query: nextQuery });
    }
  } catch {
    // 传输任务状态不阻塞首页首屏，失败时保持静默，轮询或用户打开面板后会再次刷新。
  }
}

async function openTaskPanel() {
  const preferOffline =
    offline.tasks.value.length > 0 &&
    uploadApi.displayUploadTasks.value.length === 0 &&
    uploadApi.runningRelayTasks.value.length === 0 &&
    uploadApi.completedRelayTasks.value.length === 0;
  await uploadApi.openUploadTaskPanel(preferOffline ? "offline" : "");
  if (preferOffline) await offline.fetchTasks(true, true);
}

function handleOfflineTasksCreated(tasks: OfflineDownloadTask[]) {
  offline.registerTasks(tasks);
  uploadApi.taskPanelCategory.value = "offline";
  void uploadApi.openUploadTaskPanel("offline");
}

const initialLocation = !hasPendingBrowserLocationReset() ? loadSavedBrowserLocation() : null;
if (initialLocation) {
  store.primeLocation(initialLocation.accountId, initialLocation.crumbs);
}

function openFavoriteNameModal() {
  if (currentParentId.value === "") {
    toast.info("根目录无需加入收藏夹");
    return;
  }
  if (currentFolderFavorited.value) {
    toast.info("当前文件夹已在收藏夹中");
    return;
  }
  favoriteNameMode.value = "create";
  favoriteRenameTarget.value = null;
  favoriteNameInput.value = currentFolderName.value || "当前目录";
  favoriteNameModalOpen.value = true;
  focusFavoriteNameInput();
}

function openFavoriteRenameModal(item: BrowserFavoriteItem) {
  favoriteNameMode.value = "rename";
  favoriteRenameTarget.value = item;
  favoriteNameInput.value = item.name;
  favoriteNameModalOpen.value = true;
  focusFavoriteNameInput();
}

function closeFavoriteNameModal() {
  favoriteNameModalOpen.value = false;
  favoriteNameMode.value = "create";
  favoriteRenameTarget.value = null;
}

async function focusFavoriteNameInput() {
  await nextTick();
  window.requestAnimationFrame(() => {
    favoriteNameInputRef.value?.focus();
    favoriteNameInputRef.value?.select();
  });
}

async function confirmFavoriteName() {
  const next = favoriteNameInput.value.trim();
  if (!next) {
    toast.info("收藏名不能为空");
    return;
  }
  if (favoriteNameMode.value === "rename") {
    if (!favoriteRenameTarget.value) return;
    await store.renameFavorite(favoriteRenameTarget.value, next);
  } else {
    await store.addCurrentDirectoryFavorite(next);
  }
  favoriteNameModalOpen.value = false;
  favoriteNameMode.value = "create";
  favoriteRenameTarget.value = null;
}

function resetNameAlignState() {
  clearNameAlignApplyProgress();
  nameAlignLoading.value = false;
  nameAlignApplying.value = false;
  nameAlignError.value = "";
  nameAlignTargetFile.value = null;
  nameAlignPreview.value = null;
  nameAlignSelectedSampleId.value = "";
  nameAlignSuspectIds.value = [];
  nameAlignIncludeSuspects.value = true;
  nameAlignApplyTotal.value = 0;
  nameAlignApplyProgress.value = 0;
}

function closeNameAlignModal() {
  nameAlignOpen.value = false;
  resetNameAlignState();
}

function applyNameAlignPreview(preview: FileNameAlignPreviewResult) {
  nameAlignPreview.value = preview;
  nameAlignSelectedSampleId.value = preview.sample.file_id;
  nameAlignSuspectIds.value = preview.suspects.map((item) => item.file_id);
  nameAlignIncludeSuspects.value = preview.suspects.length > 0;
}

async function loadNameAlignPreview(sampleFileId = "") {
  if (!currentAccountId.value || !nameAlignTargetFile.value) return;
  nameAlignLoading.value = true;
  nameAlignError.value = "";
  try {
    const preview = await filesApi.previewNameAlign({
      account_id: currentAccountId.value,
      parent_id: currentParentId.value,
      target_file_id: nameAlignTargetFile.value.id,
      sample_file_id: sampleFileId || undefined,
    });
    applyNameAlignPreview(preview);
  } catch (error) {
    nameAlignPreview.value = null;
    nameAlignError.value = getApiErrorMessage(error, "命名对齐预览失败");
  } finally {
    nameAlignLoading.value = false;
  }
}

async function openNameAlign(file: FileItem) {
  if (!currentAccountId.value) {
    toast.info("请先选择一个账号");
    return;
  }
  if (file.is_dir) return;
  nameAlignOpen.value = true;
  resetNameAlignState();
  nameAlignTargetFile.value = file;
  await loadNameAlignPreview();
}

async function handleNameAlignSampleChange(sampleFileId: string) {
  if (!sampleFileId || sampleFileId === nameAlignSelectedSampleId.value || nameAlignLoading.value) return;
  nameAlignSelectedSampleId.value = sampleFileId;
  await loadNameAlignPreview(sampleFileId);
}

function handleNameAlignIncludeSuspects(checked: boolean) {
  nameAlignIncludeSuspects.value = checked;
}

function handleNameAlignRemove(fileId: string) {
  nameAlignSuspectIds.value = nameAlignSuspectIds.value.filter((id) => id !== fileId);
  if (!nameAlignPreview.value) return;
  nameAlignPreview.value = {
    ...nameAlignPreview.value,
    suspects: nameAlignPreview.value.suspects.filter((item) => item.file_id !== fileId),
  };
  if (nameAlignPreview.value.suspects.length === 0) {
    nameAlignIncludeSuspects.value = false;
  }
}

function clearNameAlignApplyProgress() {
  if (nameAlignApplyTimer !== undefined) {
    window.clearInterval(nameAlignApplyTimer);
    nameAlignApplyTimer = undefined;
  }
}

function startNameAlignApplyProgress(total: number) {
  clearNameAlignApplyProgress();
  nameAlignApplyTotal.value = total;
  nameAlignApplyProgress.value = 0;
  if (total <= 1) return;
  nameAlignApplyTimer = window.setInterval(() => {
    if (nameAlignApplyProgress.value < total - 1) {
      nameAlignApplyProgress.value += 1;
    }
  }, 700);
}

function finishNameAlignApplyProgress() {
  clearNameAlignApplyProgress();
  nameAlignApplyProgress.value = nameAlignApplyTotal.value;
}

async function handleNameAlignApply() {
  if (!currentAccountId.value || !nameAlignPreview.value || !nameAlignTargetFile.value) return;
  const selectedFileIds = [
    nameAlignPreview.value.target.file_id,
    ...(nameAlignIncludeSuspects.value ? nameAlignSuspectIds.value : []),
  ];
  nameAlignApplying.value = true;
  startNameAlignApplyProgress(selectedFileIds.length);
  let succeeded = false;
  try {
    const result = await filesApi.applyNameAlign({
      account_id: currentAccountId.value,
      parent_id: currentParentId.value,
      target_file_id: nameAlignTargetFile.value.id,
      sample_file_id: nameAlignSelectedSampleId.value || undefined,
      selected_file_ids: selectedFileIds,
    });
    succeeded = true;
    finishNameAlignApplyProgress();
    closeNameAlignModal();
    await store.loadFiles({ forceRefresh: true, silent: true });
    toast.success(`命名对齐完成，已重命名 ${result.renamed.length} 个文件`);
  } catch (error) {
    clearNameAlignApplyProgress();
    nameAlignApplyTotal.value = 0;
    nameAlignApplyProgress.value = 0;
    toast.error(getApiErrorMessage(error, "命名对齐执行失败"));
  } finally {
    if (!succeeded) {
      nameAlignApplying.value = false;
    }
  }
}

function resolvePreviewKind(file: FileItem, kind: ReturnType<typeof fileKind>): FilePreviewKind | null {
  if (kind === "video" || kind === "audio" || kind === "image" || kind === "pdf") return kind;
  if (kind === "text" || kind === "code") return "text";
  if (kind === "doc" && file.name.toLowerCase().endsWith(".docx")) return "docx";
  if (kind === "sheet") return "spreadsheet";
  if (kind === "archive" && /\.(zip|cbz)$/i.test(file.name)) return "archive";
  if (kind === "slide" && file.name.toLowerCase().endsWith(".pptx")) return "pptx";
  return null;
}

async function onOpen(file: FileItem) {
  if (file.is_dir) {
    store.enterFolder(file);
    return;
  }
  const kind = fileKind(file);
  const previewKind = resolvePreviewKind(file, kind);
  if (previewKind) {
    activePreview.value = { kind: previewKind, file };
    return;
  }
  if (kind === "file" && currentAccountId.value) {
    try {
      const head = await filesApi.previewHeadBytes(currentAccountId.value, file.id, file.name);
      const { isProbablyText } = await import("@/utils/textEncoding");
      if (isProbablyText(head)) {
        activePreview.value = { kind: "text", file };
        return;
      }
    } catch {
      // 无法读取文件头时，仍按不支持预览处理并给出明确反馈。
    }
  }
  const result = await showConfirm({
    title: "暂不支持在线预览",
    message: `当前文件：${file.name}`,
    hint: file.name.toLowerCase().endsWith(".doc")
      ? "这是旧版 Word 文档，请下载后使用本地应用打开。"
      : file.name.toLowerCase().endsWith(".ppt")
        ? "这是旧版 PowerPoint 演示文稿，请下载后使用本地应用打开。"
        : "当前文件格式暂不支持在线预览，请下载后使用本地应用打开。",
    icon: "info",
    confirmText: "下载文件",
    cancelText: "取消",
    danger: false,
  }).catch(() => null);
  if (result?.action === "confirm") fileActions.downloadFile(file);
}

watch([currentAccountId, breadcrumb], () => {
  selectedIds.value = [];
  activePreview.value = null;
  if (nameAlignOpen.value) closeNameAlignModal();
  persistBrowserLocation();
}, { deep: true });

watch(currentAccountId, () => {
  void offline.loadCapability();
}, { immediate: true });

onMounted(async () => {
  // 守卫进入首页时已拉取过认证状态，有缓存则跳过，避免重复的 /auth/status 往返。
  if (!auth.loaded) await auth.load();
  // 公共系统配置只影响账号切换 UI 与 STRM 提示，不在文件首屏关键路径上，后台并行拉取。
  void loadPublicSystemConfig();
  await store.loadAccounts();
  if (accounts.value.length) {
    const shouldResetToHome = consumeResetBrowserLocationOnce();
    if (shouldResetToHome) {
      await store.resetToDefaultAccount();
    } else {
      const restored = await restoreBrowserLocation();
      if (!restored) {
        await store.resetToDefaultAccount();
      }
    }
  }
  // 全局收藏夹与账号无关，启动时加载一次即可
  await store.loadFavorites({ silent: true });
  browserBootstrapping.value = false;
  await nextTick();
  window.requestAnimationFrame(() => {
    favoritesTransitionReady.value = true;
  });
  void loadInitialTaskState();
});

onUnmounted(() => {
  clearNameAlignApplyProgress();
  uploadApi.cleanupUploadTasks();
});
</script>

<template>
  <div class="browser">
    <!-- Mobile sidebar drawer -->
    <MobileSidebar :open="mobileSidebarOpen" @close="mobileSidebarOpen = false">
      <div class="mobile-sidebar__drives">
        <DriveSidebar
          v-if="accounts.length > 0"
          :accounts="accounts"
          :model-value="currentAccountId"
          @update:model-value="(id) => { store.selectAccount(id); mobileSidebarOpen = false; }"
        />
      </div>
      <div v-if="isAdmin" class="mobile-sidebar__favorites">
        <FavoritesSidebar
          :items="favorites"
          :accounts="accounts"
          :current-crumb-ids="currentCrumbIds"
          :current-account-id="currentAccountId"
          :current-folder-favorited="currentFolderFavorited"
          :drag-active="false"
          @add-current="openFavoriteNameModal"
          @collapse="mobileSidebarOpen = false"
          @open="(f) => { store.openFavorite(f); mobileSidebarOpen = false; }"
          @rename="openFavoriteRenameModal"
          @remove="store.removeFavorite"
          @move="store.moveFavorite"
        />
      </div>
    </MobileSidebar>

    <!-- File operation bottom sheet -->
    <BottomSheet :open="bottomSheetOpen" :title="bottomSheetTarget?.name" @close="closeBottomSheet">
      <div class="mobile-sheet-actions">
        <button class="mobile-sheet-action" @click="bottomSheetAction('download')">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
          <span>下载</span>
        </button>
        <button class="mobile-sheet-action" @click="bottomSheetAction('move')">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 12h14"/><path d="m12 5 7 7-7 7"/></svg>
          <span>移动</span>
        </button>
        <button class="mobile-sheet-action" @click="bottomSheetAction('copy')">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
          <span>复制</span>
        </button>
        <button class="mobile-sheet-action" @click="bottomSheetAction('rename')">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2"><path d="M17 3a2.85 2.85 0 0 1 4 4L7.5 20.5 2 22l1.5-5.5Z"/></svg>
          <span>重命名</span>
        </button>
        <button class="mobile-sheet-action mobile-sheet-action--danger" @click="bottomSheetAction('delete')">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
          <span>删除</span>
        </button>
      </div>
    </BottomSheet>
    <div v-if="accounts.length > 0" class="browser__drives-strip">
      <DriveSidebar
        :accounts="accounts"
        :model-value="currentAccountId"
        @update:model-value="store.selectAccount"
      />
    </div>

    <div v-if="!browserBootstrapping && !accounts.length && !loading" class="browser__empty">
      还没有可用账号，请到
      <RouterLink to="/admin" class="browser__link">管理后台</RouterLink>
      添加。
    </div>

    <div v-else-if="!browserBootstrapping" class="browser__frame" :class="{ 'browser__frame--grid': view === 'grid' }">
      <div v-if="refreshing" class="browser__refresh-overlay">
        <BusySpinner variant="notch" :size="28" color="var(--brand)" />
        <span class="browser__refresh-text">正在强制刷新…</span>
      </div>
      <div class="browser__panel-top">
        <button
          type="button"
          class="browser__hamburger"
          aria-label="打开导航"
          @click="mobileSidebarOpen = true"
        >
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <line x1="3" y1="6" x2="21" y2="6" />
            <line x1="3" y1="12" x2="21" y2="12" />
            <line x1="3" y1="18" x2="21" y2="18" />
          </svg>
        </button>
        <BreadcrumbNav class="browser__breadcrumb" :items="breadcrumb" @navigate="store.goTo" />
        <FileToolbar
          :is-admin="isAdmin"
          :selected-count="selectedCount"
          :view="view"
          :refreshing="refreshing"
          :response-time="responseTime"
          :cache-rate="cacheRate"
          :favorites-open="favoritesOpen"
          :offline-download-supported="offline.capability.value?.supported"
          @refresh="store.refreshFiles"
          @update:view="setView"
          @create-folder="startCreateFolder"
          @upload-file="uploadApi.handleUploadFile"
          @upload-folder="uploadApi.handleUploadFolder"
          @offline-download="offline.openModal"
          @batch-delete="fileActions.requestBatchDelete"
          @batch-move="fileActions.requestBatchMove"
          @batch-copy="fileActions.requestBatchCopy"
          @toggle-favorites="store.toggleFavoritesOpen"
        />
      </div>
      <div v-if="strmPrompt.showPrompt.value && !strmGenerating" class="strm-prompt-bar">
        <div class="strm-prompt-bar__main">
          <span class="strm-prompt-bar__dot" aria-hidden="true" />
          <span class="strm-prompt-bar__text">{{ strmPrompt.promptText.value }}</span>
        </div>
        <span class="strm-prompt-bar__actions">
          <button
            type="button"
            class="strm-prompt-bar__action"
            :disabled="strmGenerating"
            @click="handleConfirmStrmPrompt"
          >
            生成
          </button>
          <button
            type="button"
            class="strm-prompt-bar__action strm-prompt-bar__action--muted"
            @click="handleDismissStrmPrompt"
          >
            忽略
          </button>
        </span>
      </div>
      <div
        class="browser__content"
        :class="{
          'browser__content--with-favorites': showFavorites,
          'browser__content--favorites-transition-ready': favoritesTransitionReady,
        }"
      >
        <div class="browser__favorites-slot">
          <FavoritesSidebar
            v-if="isAdmin"
            :items="favorites"
            :accounts="accounts"
            :current-crumb-ids="currentCrumbIds"
            :current-account-id="currentAccountId"
            :current-folder-favorited="currentFolderFavorited"
            :drag-active="dragMove.active"
            :active-drop-target-id="dragMove.targetId"
            :can-drop-on-favorite="canDropOnFavorite"
            @add-current="openFavoriteNameModal"
            @collapse="store.toggleFavoritesOpen"
            @open="store.openFavorite"
            @rename="openFavoriteRenameModal"
            @remove="store.removeFavorite"
            @move="store.moveFavorite"
            @drag-enter="handleFavoriteDragEnter"
            @drag-leave="handleFavoriteDragLeave"
            @drop="handleFavoriteDrop"
          />
        </div>

        <div
          class="browser__main"
          @touchstart.passive="onPullStart"
          @touchmove.passive="onPullMove"
          @touchend="onPullEnd"
        >
          <div v-if="pullRefreshing" class="browser__pull-indicator">
            <BusySpinner variant="notch" :size="22" color="var(--brand)" />
            <span>下拉刷新</span>
          </div>
          <FileTable
            :files="files"
            :view="view"
            :loading="loading"
            :is-admin="isAdmin"
            :account-id="currentAccountId"
            :sort-key="sortKey"
            :sort-order="sortOrder"
            :sort-class="sortClass"
            :create-folder-request="createFolderRequest"
            :row-operations="fileActions.rowOps"
            v-model:selected-ids="selectedIds"
            :rename-file="fileActions.renameFile"
            :create-folder="fileActions.createFolder"
            :delete-file="fileActions.deleteFile"
            :download-file="fileActions.downloadFile"
            :move-file="fileActions.requestSingleMove"
            :copy-file="fileActions.requestSingleCopy"
            :name-align-file="openNameAlign"
            :drag-active="dragMove.active"
            :active-drop-target-id="dragMove.targetId"
            :can-drop-on-folder="canDropOnFolder"
            @open="onOpen"
            @more-actions="openFileBottomSheet"
            @sort-by="sortBy"
            @set-sort="({ key, order }) => sortBy(key, order)"
            @generate-current-directory-strm="handleGenerateCurrentDirectoryStrm"
            @drag-file-start="startDragMove"
            @drag-file-end="finishDragMove"
            @drag-enter-folder="handleFolderDragEnter"
            @drag-leave-folder="handleFolderDragLeave"
            @drop-on-folder="handleFolderDrop"
          />
        </div>
      </div>
    </div>

    <FolderPickerModal
      :open="fileActions.transfer.open"
      :title="transferTitle"
      :confirm-text="transferConfirmText"
      :account-id="currentAccountId"
      :excluded-folder-ids="fileActions.transfer.excluded"
      :allow-create-folder="true"
      :show-refresh="false"
      :initial-breadcrumb="breadcrumb"
      @resolve="fileActions.confirmTransfer"
      @close="fileActions.cancelTransfer"
    />

    <NameAlignModal
      :open="nameAlignOpen"
      :loading="nameAlignLoading"
      :applying="nameAlignApplying"
      :error="nameAlignError"
      :preview="nameAlignPreview"
      :selected-sample-id="nameAlignSelectedSampleId"
      :suspect-ids="nameAlignSuspectIds"
      :include-suspects="nameAlignIncludeSuspects"
      :apply-total="nameAlignApplyTotal"
      :apply-progress="nameAlignApplyProgress"
      @close="closeNameAlignModal"
      @update:sample-id="handleNameAlignSampleChange"
      @update:include-suspects="handleNameAlignIncludeSuspects"
      @remove-suspect="handleNameAlignRemove"
      @apply="handleNameAlignApply"
    />

    <AppModal
      :open="favoriteNameModalOpen"
      size="sm"
      :title="favoriteNameMode === 'rename' ? '重命名收藏' : '收藏当前文件夹'"
      @close="closeFavoriteNameModal"
    >
      <div class="favorite-name-modal">
        <div class="favorite-name-modal__label">收藏名称</div>
        <AppInput
          ref="favoriteNameInputRef"
          v-model="favoriteNameInput"
          :placeholder="favoriteNameMode === 'rename' ? '请输入新的收藏名称' : '请输入收藏名称'"
          @keydown.enter.prevent="confirmFavoriteName"
        />
      </div>
      <template #footer>
        <button type="button" class="favorite-name-modal__btn favorite-name-modal__btn--ghost" @click="closeFavoriteNameModal">
          取消
        </button>
        <button type="button" class="favorite-name-modal__btn favorite-name-modal__btn--primary" @click="confirmFavoriteName">
          {{ favoriteNameMode === "rename" ? "保存名称" : "保存" }}
        </button>
      </template>
    </AppModal>

    <input
      ref="uploadFileInput"
      type="file"
      multiple
      hidden
      @change="uploadApi.handleUploadFileChange"
    />
    <input
      ref="uploadFolderInput"
      type="file"
      multiple
      webkitdirectory
      hidden
      @change="uploadApi.handleUploadFolderChange"
    />

    <OfflineDownloadModal
      :open="offline.modalOpen.value"
      :account-id="currentAccountId"
      :account-name="selectedAccountName"
      :capability="offline.capability.value"
      :current-parent-id="currentParentId"
      :current-display-path="getCurrentDisplayPath()"
      :breadcrumb="breadcrumb"
      @close="offline.closeModal"
      @created="handleOfflineTasksCreated"
    />

    <TaskPanel
      v-if="uploadTaskPanelOpen"
      :upload-api="uploadApi"
      :relay="relay"
      :offline="offline"
    />

    <FilePreviewHost
      v-if="activePreview && currentAccountId != null"
      :account-id="currentAccountId"
      :files="files"
      :active="activePreview"
      @close="activePreview = null"
      @download="fileActions.downloadFile"
    />
  </div>
</template>

<style scoped>
.browser {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 20px 0;
}
.browser__drives-strip {
  display: flex;
  min-width: 0;
}
.browser__frame {
  position: relative;
  background: var(--surface);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-card);
  overflow: hidden;
}
.browser__panel-top {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 10px 20px;
  border-bottom: 1px solid var(--border-soft);
  background: var(--surface-muted);
  border-radius: var(--radius-md) var(--radius-md) 0 0;
}
.browser__breadcrumb {
  flex: 1 1 auto;
  min-width: 0;
}
.browser__panel-top :deep(.file-toolbar) {
  flex: 0 1 auto;
  padding: 0;
  border-bottom: none;
  border-radius: 0;
  background: transparent;
}
.browser__content {
  display: grid;
  grid-template-columns: 0 minmax(0, 1fr);
  gap: 0;
}
.browser__content--with-favorites {
  grid-template-columns: 168px minmax(0, 1fr);
}
.browser__content--favorites-transition-ready {
  transition: grid-template-columns 0.22s ease;
}
.browser__favorites-slot {
  min-width: 0;
  overflow: hidden;
}
.browser__content--with-favorites .browser__favorites-slot {
  border-right: 1px solid var(--border-soft);
}
.browser__favorites-slot :deep(.favorites-sidebar) {
  height: 100%;
  border-right: none;
}
.browser__main {
  min-width: 0;
}
.favorite-name-modal {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.favorite-name-modal__label {
  color: var(--text-muted);
  font-size: 13px;
}
.favorite-name-modal__btn {
  height: 36px;
  padding: 0 14px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: var(--surface);
  color: var(--text-regular);
  transition: var(--transition);
}
.favorite-name-modal__btn--ghost:hover {
  border-color: var(--brand);
  color: var(--brand);
}
.favorite-name-modal__btn--primary {
  border-color: transparent;
  background: var(--brand);
  color: var(--text-on-brand);
}
.favorite-name-modal__btn--primary:hover {
  filter: brightness(0.98);
}
.browser__refresh-overlay {
  position: absolute;
  inset: 0;
  z-index: 20;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  background: var(--overlay-scrim);
}
.browser__refresh-text {
  color: var(--text-muted);
  font-size: 14px;
}
.browser__frame--grid {
  overflow: visible;
}
.browser__empty {
  padding: 60px 20px;
  text-align: center;
  color: var(--text-muted);
  background: var(--surface);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-card);
}
.browser__link {
  color: var(--brand);
  font-weight: 600;
}

@media (max-width: 768px) {
  .browser {
    gap: 12px;
    padding: 12px 0;
  }

  .browser__panel-top {
    flex-wrap: wrap;
    gap: 10px;
    padding: 10px 12px;
  }

  .browser__breadcrumb {
    width: 100%;
    flex-basis: 100%;
  }

  .browser__content--with-favorites {
    grid-template-columns: 1fr;
  }

  .browser__favorites-slot {
    max-height: 0;
    opacity: 0;
    overflow: hidden;
    border-right: none;
  }

  .browser__content--with-favorites .browser__favorites-slot {
    max-height: 360px;
    opacity: 1;
    overflow-y: auto;
    border-bottom: 1px solid var(--border-soft);
  }

  /* 移动端收藏夹是内容自适应高度（由槽位滚动），不再按列高撑满 */
  .browser__favorites-slot :deep(.favorites-sidebar) {
    height: auto;
  }

  .browser__content--favorites-transition-ready .browser__favorites-slot {
    transition:
      max-height 0.22s ease,
      opacity 0.18s ease;
  }
}

.strm-prompt-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 11px 20px;
  border-bottom: 1px solid color-mix(in srgb, var(--brand) 22%, var(--border-soft));
  background: color-mix(in srgb, var(--brand) 14%, var(--surface));
}

.strm-prompt-bar__main {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  flex: 1;
}

.strm-prompt-bar__dot {
  flex-shrink: 0;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--brand);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--brand) 20%, transparent);
}

.strm-prompt-bar__text {
  min-width: 0;
  font-size: 13px;
  font-weight: 600;
  line-height: 1.5;
  color: color-mix(in srgb, var(--brand) 55%, var(--text));
}

.strm-prompt-bar__actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.strm-prompt-bar__action {
  border: none;
  background: transparent;
  padding: 4px 10px;
  border-radius: var(--radius-sm);
  font-size: 13px;
  font-weight: 600;
  color: var(--brand);
  cursor: pointer;
}

.strm-prompt-bar__action:hover:not(:disabled) {
  background: color-mix(in srgb, var(--brand) 12%, transparent);
}

.strm-prompt-bar__action--muted {
  color: var(--text-muted);
  font-weight: 500;
}

.strm-prompt-bar__action:disabled {
  opacity: 0.6;
  cursor: wait;
}

/* ---- Mobile: hamburger button ---- */
.browser__hamburger {
  display: none;
  width: 38px;
  height: 38px;
  align-items: center;
  justify-content: center;
  border: none;
  background: none;
  border-radius: 8px;
  color: var(--text-primary, #1f2937);
  cursor: pointer;
  flex-shrink: 0;
}
.browser__hamburger:active {
  background: var(--bg-secondary, #f3f4f6);
}
@media (max-width: 768px) {
  .browser__hamburger {
    display: inline-flex;
  }
}

/* ---- Mobile: sidebar content ---- */
.mobile-sidebar__drives {
  padding: 8px 0;
}
.mobile-sidebar__favorites {
  border-top: 1px solid var(--border-soft, #e5e7eb);
  padding: 8px 0;
}

/* ---- Mobile: bottom sheet actions grid ---- */
.mobile-sheet-actions {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 4px;
  padding: 8px 12px;
}
.mobile-sheet-action {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 14px 8px;
  border: none;
  background: none;
  border-radius: 12px;
  color: var(--text-primary, #1f2937);
  font-size: 12px;
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
}
.mobile-sheet-action:active {
  background: var(--bg-secondary, #f3f4f6);
}
.mobile-sheet-action--danger {
  color: #ef4444;
}
.mobile-sheet-action--danger:active {
  background: #fef2f2;
}

/* ---- Mobile: pull-to-refresh indicator ---- */
.browser__pull-indicator {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 10px 0;
  color: var(--text-muted, #9ca3af);
  font-size: 13px;
}
</style>
