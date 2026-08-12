<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, toRef, watch } from "vue";
import type { FileItem } from "@/api/types";
import type { FileRowOperation, SortKey, SortOrder } from "@/types/file-browser";
import { formatSize, formatTime } from "@/utils/format";
import type { DeleteFileHooks } from "@/composables/useFileActions";
import { useFileTableInline } from "@/composables/useFileTableInline";
import { getSvg } from "@/components/icons/svgRegistry";
import { getIconfontSymbolId } from "@/components/icons/iconfontSymbolMap";
import FileIcon from "./FileIcon.vue";
import FileTableHeader from "./FileTableHeader.vue";
import FileGridSortMenu from "./FileGridSortMenu.vue";
import FileContextMenu from "./FileContextMenu.vue";
import SvgIcon from "@/components/icons/SvgIcon.vue";
import type { ContextMenuItem } from "@/composables/useFileTableInline";
import { fileKind } from "@/utils/fileIcon";

const props = defineProps<{
  files: FileItem[];
  view: "list" | "grid";
  loading: boolean;
  isAdmin: boolean;
  selectedIds: string[];
  sortKey: SortKey;
  sortOrder: SortOrder;
  sortClass: (key: SortKey) => SortOrder | "";
  createFolderRequest: number;
  rowOperations?: Record<string, FileRowOperation>;
  renameFile: (file: FileItem, newName: string) => Promise<boolean>;
  createFolder: (name: string) => Promise<boolean>;
  deleteFile: (file: FileItem, hooks?: DeleteFileHooks) => Promise<boolean>;
  downloadFile: (file: FileItem) => void;
  moveFile: (file: FileItem) => void;
  copyFile: (file: FileItem) => void;
  nameAlignFile: (file: FileItem) => void;
  dragActive?: boolean;
  activeDropTargetId?: string;
  canDropOnFolder?: (file: FileItem) => boolean;
  accountId?: number | null;
}>();

const INITIAL_LIST_RENDER_COUNT = 200;
const LIST_RENDER_CHUNK_SIZE = 150;

const emit = defineEmits<{
  open: [file: FileItem];
  "update:selectedIds": [ids: string[]];
  "sort-by": [key: SortKey];
  "set-sort": [payload: { key: SortKey; order: SortOrder }];
  "generate-current-directory-strm": [];
  "drag-file-start": [file: FileItem];
  "drag-file-end": [];
  "drag-enter-folder": [file: FileItem];
  "drag-leave-folder": [file: FileItem];
  "drop-on-folder": [file: FileItem];
}>();

const inline = useFileTableInline({
  files: toRef(props, "files"),
  isAdmin: toRef(props, "isAdmin"),
  loading: toRef(props, "loading"),
  createFolderRequest: toRef(props, "createFolderRequest"),
  externalRowOps: toRef(props, "rowOperations"),
  renameFile: (file, name) => props.renameFile(file, name),
  createFolder: (name) => props.createFolder(name),
  deleteFile: (file, hooks) => props.deleteFile(file, hooks),
  downloadFile: (file) => props.downloadFile(file),
  moveFile: (file) => props.moveFile(file),
  copyFile: (file) => props.copyFile(file),
  nameAlignFile: (file) => props.nameAlignFile(file),
});

const {
  renameDraft,
  renameComposing,
  inlineCreatingFolder,
  createFolderDraft,
  createFolderSaving,
  createFolderComposing,
  createFolderPendingName,
  contextMenu,
  contextMenuItems,
  emptyColSpan,
  emptyStateText,
  showEmptyRow,
  isInlineRenaming,
  isInlineProcessing,
  getRowOperationText,
  openContextMenu,
  runRowAction,
  cancelInlineRename,
  submitInlineRename,
  cancelInlineCreateFolder,
  submitInlineCreateFolder,
  handleContextAction,
  closeContextMenu,
} = inline;

function bindRenameInput(el: unknown) {
  inline.renameInputRef.value = el as HTMLInputElement | null;
}

function bindCreateFolderInput(el: unknown) {
  inline.createFolderInputRef.value = el as HTMLInputElement | null;
}

const selectedSet = computed(() => new Set(props.selectedIds));
const selectAll = computed(
  () => props.files.length > 0 && props.selectedIds.length === props.files.length,
);
const selectedCount = computed(() => props.selectedIds.length);
const listVisibleCount = ref(INITIAL_LIST_RENDER_COUNT);
const listLoadMoreSentinel = ref<HTMLTableRowElement | null>(null);
let listLoadMoreObserver: IntersectionObserver | null = null;

const visibleListFiles = computed(() =>
  props.view === "list" ? props.files.slice(0, listVisibleCount.value) : props.files,
);
const hasMoreListFiles = computed(
  () => props.view === "list" && visibleListFiles.value.length < props.files.length,
);
const headerContextMenu = ref({
  open: false,
  x: 0,
  y: 0,
});
const headerContextMenuItems = computed<ContextMenuItem[]>(() =>
  props.isAdmin ? [{ action: "generate-current-directory-strm", label: "生成当前目录 STRM" }] : [],
);

function fileKey(f: FileItem) {
  return f.id || f.name;
}

function closeDirectoryContextMenu() {
  headerContextMenu.value.open = false;
}

function openDirectoryContextMenu(event: MouseEvent) {
  if (!props.isAdmin) return;
  const target = event.target as HTMLElement | null;
  if (target?.closest(".file-row, .file-card, .inline-create-row")) return;
  const items = headerContextMenuItems.value;
  if (!items.length) return;
  closeContextMenu();
  const menuWidth = 188;
  const menuHeight = items.length * 38 + 14;
  headerContextMenu.value = {
    open: true,
    x: Math.max(8, Math.min(event.clientX, window.innerWidth - menuWidth - 8)),
    y: Math.max(8, Math.min(event.clientY, window.innerHeight - menuHeight - 8)),
  };
}

function handleHeaderContextAction(action: string) {
  closeDirectoryContextMenu();
  if (action === "generate-current-directory-strm") {
    emit("generate-current-directory-strm");
  }
}

function expandVisibleListFiles() {
  listVisibleCount.value = Math.min(
    props.files.length,
    listVisibleCount.value + LIST_RENDER_CHUNK_SIZE,
  );
}

function resetVisibleListFiles() {
  listVisibleCount.value = INITIAL_LIST_RENDER_COUNT;
}

function disconnectListLoadMoreObserver() {
  listLoadMoreObserver?.disconnect();
  listLoadMoreObserver = null;
}

function bindListLoadMoreSentinel(el: unknown) {
  listLoadMoreSentinel.value = el instanceof HTMLTableRowElement ? el : null;
  void nextTick(updateListLoadMoreObserver);
}

async function updateListLoadMoreObserver() {
  disconnectListLoadMoreObserver();
  if (!hasMoreListFiles.value || !listLoadMoreSentinel.value) return;
  if (typeof window === "undefined" || typeof window.IntersectionObserver === "undefined") {
    listVisibleCount.value = props.files.length;
    return;
  }
  listLoadMoreObserver = new window.IntersectionObserver(
    (entries) => {
      if (!entries.some((entry) => entry.isIntersecting)) return;
      expandVisibleListFiles();
      void nextTick(updateListLoadMoreObserver);
    },
    {
      root: null,
      rootMargin: "240px 0px",
      threshold: 0,
    },
  );
  listLoadMoreObserver.observe(listLoadMoreSentinel.value);
}

function toggleSelection(id: string, checked: boolean) {
  const next = new Set(props.selectedIds);
  if (checked) next.add(id);
  else next.delete(id);
  emit("update:selectedIds", Array.from(next));
}

function toggleSelectAll(checked: boolean) {
  emit("update:selectedIds", checked ? props.files.map(fileKey) : []);
}

function isDroppableFolder(file: FileItem) {
  return Boolean(props.dragActive) && file.is_dir && !isInlineProcessing(file) && (props.canDropOnFolder?.(file) ?? false);
}

function isActiveDropTarget(file: FileItem) {
  return props.activeDropTargetId === file.id;
}

function truncateDragText(input: string, limit: number) {
  if (input.length <= limit) return input;
  return `${input.slice(0, Math.max(0, limit - 1))}…`;
}

function createDragPreview(file: FileItem) {
  if (typeof document === "undefined") return null;
  const draggingSelected = selectedSet.value.has(fileKey(file));
  const count = draggingSelected && selectedCount.value > 1 ? selectedCount.value : 1;
  const rootStyle = getComputedStyle(document.documentElement);
  const brand = rootStyle.getPropertyValue("--brand").trim() || "#3b82f6";
  const surface = rootStyle.getPropertyValue("--surface").trim() || "#ffffff";
  const text = rootStyle.getPropertyValue("--text").trim() || "#111827";
  const muted = rootStyle.getPropertyValue("--text-muted").trim() || "#6b7280";
  const border = rootStyle.getPropertyValue("--border-soft").trim() || "rgba(59,130,246,0.24)";
  const hotAreaWidth = 24;
  const width = Math.min(320, Math.max(196, 132 + Math.min(file.name.length, 18) * 8));
  const height = 54;
  const iconX = hotAreaWidth + 12;
  const iconY = 13;
  const textX = iconX + 28;
  const title = truncateDragText(file.name, count > 1 ? 20 : 22);
  const subtitle = count > 1 ? `等 ${count} 个项目` : file.is_dir ? "移动文件夹" : "移动文件";
  const NS = "http://www.w3.org/2000/svg";

  const svg = document.createElementNS(NS, "svg");
  svg.setAttribute("xmlns", NS);
  svg.setAttribute("width", String(width));
  svg.setAttribute("height", String(height));
  svg.setAttribute("viewBox", `0 0 ${width} ${height}`);
  svg.style.position = "fixed";
  svg.style.left = "-9999px";
  svg.style.top = "-9999px";
  svg.style.pointerEvents = "none";
  svg.style.zIndex = "99999";

  const rect = document.createElementNS(NS, "rect");
  rect.setAttribute("x", "0.5");
  rect.setAttribute("y", "0.5");
  rect.setAttribute("width", String(width - 1));
  rect.setAttribute("height", String(height - 1));
  rect.setAttribute("rx", "16");
  rect.setAttribute("ry", "16");
  rect.setAttribute("fill", surface);
  rect.setAttribute("stroke", border);
  svg.appendChild(rect);

  const iconBg = document.createElementNS(NS, "rect");
  iconBg.setAttribute("x", String(iconX - 5));
  iconBg.setAttribute("y", String(iconY - 4));
  iconBg.setAttribute("width", "28");
  iconBg.setAttribute("height", "28");
  iconBg.setAttribute("rx", "10");
  iconBg.setAttribute("ry", "10");
  iconBg.setAttribute("fill", surface);
  iconBg.setAttribute("stroke", border);
  svg.appendChild(iconBg);

  const iconName = fileKind(file);
  const symbolId = getIconfontSymbolId(iconName);
  if (symbolId) {
    const iconSvg = document.createElementNS(NS, "svg");
    iconSvg.setAttribute("x", String(iconX));
    iconSvg.setAttribute("y", String(iconY));
    iconSvg.setAttribute("width", "18");
    iconSvg.setAttribute("height", "18");
    iconSvg.setAttribute("viewBox", "0 0 1024 1024");
    iconSvg.setAttribute("fill", brand);
    iconSvg.setAttribute("color", brand);
    const use = document.createElementNS(NS, "use");
    use.setAttribute("href", `#${symbolId}`);
    use.setAttributeNS("http://www.w3.org/1999/xlink", "xlink:href", `#${symbolId}`);
    iconSvg.appendChild(use);
    svg.appendChild(iconSvg);
  } else {
    const iconMarkup = getSvg(iconName);
    const parsed = new DOMParser().parseFromString(iconMarkup, "image/svg+xml").documentElement;
    if (parsed?.tagName?.toLowerCase() === "svg") {
      parsed.setAttribute("x", String(iconX));
      parsed.setAttribute("y", String(iconY));
      parsed.setAttribute("width", "18");
      parsed.setAttribute("height", "18");
      parsed.setAttribute("color", brand);
      parsed.setAttribute("fill", brand);
      svg.appendChild(document.importNode(parsed, true));
    }
  }

  const titleText = document.createElementNS(NS, "text");
  titleText.setAttribute("x", String(textX));
  titleText.setAttribute("y", "24");
  titleText.setAttribute("fill", text);
  titleText.setAttribute("font-size", "13");
  titleText.setAttribute("font-weight", "600");
  titleText.setAttribute("font-family", "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif");
  titleText.textContent = title;
  svg.appendChild(titleText);

  const subtitleText = document.createElementNS(NS, "text");
  subtitleText.setAttribute("x", String(textX));
  subtitleText.setAttribute("y", "40");
  subtitleText.setAttribute("fill", muted);
  subtitleText.setAttribute("font-size", "12");
  subtitleText.setAttribute("font-weight", "500");
  subtitleText.setAttribute("font-family", "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif");
  subtitleText.textContent = subtitle;
  svg.appendChild(subtitleText);

  if (count > 1) {
    const badge = document.createElementNS(NS, "circle");
    badge.setAttribute("cx", "12");
    badge.setAttribute("cy", "12");
    badge.setAttribute("r", "10");
    badge.setAttribute("fill", brand);
    svg.appendChild(badge);

    const badgeText = document.createElementNS(NS, "text");
    badgeText.setAttribute("x", "12");
    badgeText.setAttribute("y", "16");
    badgeText.setAttribute("text-anchor", "middle");
    badgeText.setAttribute("fill", "#ffffff");
    badgeText.setAttribute("font-size", "11");
    badgeText.setAttribute("font-weight", "700");
    badgeText.setAttribute("font-family", "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif");
    badgeText.textContent = String(count);
    svg.appendChild(badgeText);
  }

  document.body.appendChild(svg);
  return { wrapper: svg, offsetX: 14, offsetY: 26 };
}

function handleDragStart(event: DragEvent, file: FileItem) {
  if (!props.isAdmin || isInlineProcessing(file) || isInlineRenaming(file)) {
    event.preventDefault();
    return;
  }
  event.dataTransfer?.setData("text/plain", file.id || file.name);
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = "move";
    const preview = createDragPreview(file);
    if (preview) {
      event.dataTransfer.setDragImage(preview.wrapper, preview.offsetX, preview.offsetY);
      window.setTimeout(() => {
        preview.wrapper.remove();
      }, 0);
    }
  }
  emit("drag-file-start", file);
}

function handleDragEnd() {
  emit("drag-file-end");
}

function handleFolderDragEnter(event: DragEvent, file: FileItem) {
  if (!isDroppableFolder(file)) return;
  event.preventDefault();
  emit("drag-enter-folder", file);
}

function handleFolderDragOver(event: DragEvent, file: FileItem) {
  if (!isDroppableFolder(file)) return;
  event.preventDefault();
  if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
  emit("drag-enter-folder", file);
}

function handleFolderDragLeave(event: DragEvent, file: FileItem) {
  if (!props.dragActive) return;
  const currentTarget = event.currentTarget as Node | null;
  const relatedTarget = event.relatedTarget as Node | null;
  if (currentTarget && relatedTarget && currentTarget.contains(relatedTarget)) return;
  emit("drag-leave-folder", file);
}

function handleFolderDrop(event: DragEvent, file: FileItem) {
  if (!props.dragActive) return;
  event.preventDefault();
  emit("drop-on-folder", file);
}

function onRowClick(event: MouseEvent, file: FileItem) {
  // 桌面：单击 = 选中；双击 = 进文件夹/预览（由 onRowDblClick 处理）
  // 触屏：单击直接打开，选择靠常显 checkbox
  const target = event.target as HTMLElement | null;
  if (target?.closest('input[type="checkbox"]')) return;
  if (target?.closest(".file-row-actions")) return;
  if (target?.closest(".inline-rename-wrap")) return;
  if (!props.isAdmin || isTouchDevice.value) {
    emit("open", file);
    return;
  }
  toggleSelection(fileKey(file), !selectedSet.value.has(fileKey(file)));
}

function onRowDblClick(_event: MouseEvent, file: FileItem) {
  if (isInlineRenaming(file)) return;
  if (!props.isAdmin) return; // 非 admin 单击即打开，双击不再重复触发
  toggleSelection(fileKey(file), true);
  emit("open", file);
}

function onCardClick(file: FileItem) {
  if (!props.isAdmin || isTouchDevice.value) {
    emit("open", file);
    return;
  }
  toggleSelection(fileKey(file), !selectedSet.value.has(fileKey(file)));
}

function onCardDblClick(file: FileItem) {
  if (!props.isAdmin) return;
  toggleSelection(fileKey(file), true);
  emit("open", file);
}

function clearSelection() {
  if (props.selectedIds.length) emit("update:selectedIds", []);
}

function handleGlobalKeydown(event: KeyboardEvent) {
  if (event.key !== "Escape") return;
  const target = event.target as HTMLElement | null;
  if (target?.closest("input, textarea")) return;
  clearSelection();
}

watch(
  [() => props.files, () => props.view, () => props.sortKey, () => props.sortOrder],
  async () => {
    resetVisibleListFiles();
    await nextTick();
    void updateListLoadMoreObserver();
  },
);

watch(hasMoreListFiles, async () => {
  await nextTick();
  void updateListLoadMoreObserver();
});

// 触屏设备（无 hover 主输入）：单击打开，选择靠常显 checkbox
const isTouchDevice = ref(false);
let touchMediaQuery: MediaQueryList | null = null;
function updateTouchDevice() {
  isTouchDevice.value = Boolean(touchMediaQuery?.matches);
}

onMounted(() => {
  touchMediaQuery = window.matchMedia("(hover: none)");
  updateTouchDevice();
  touchMediaQuery.addEventListener("change", updateTouchDevice);
  void nextTick(updateListLoadMoreObserver);
  document.addEventListener("keydown", handleHeaderMenuKeydown);
  document.addEventListener("keydown", handleGlobalKeydown);
  window.addEventListener("resize", closeDirectoryContextMenu);
  window.addEventListener("scroll", closeDirectoryContextMenu, true);
});

onUnmounted(() => {
  touchMediaQuery?.removeEventListener("change", updateTouchDevice);
  touchMediaQuery = null;
  disconnectListLoadMoreObserver();
  document.removeEventListener("keydown", handleHeaderMenuKeydown);
  document.removeEventListener("keydown", handleGlobalKeydown);
  window.removeEventListener("resize", closeDirectoryContextMenu);
  window.removeEventListener("scroll", closeDirectoryContextMenu, true);
});

function handleHeaderMenuKeydown(event: KeyboardEvent) {
  if (event.key === "Escape") closeDirectoryContextMenu();
}
</script>

<template>
  <div
    class="file-list"
    :class="`view-${view}`"
    @contextmenu.prevent="openDirectoryContextMenu($event)"
  >
    <table v-if="view === 'list'" class="file-table">
      <FileTableHeader
        :is-admin="isAdmin"
        :selected-count="selectedCount"
        :select-all="selectAll"
        :files-count="files.length"
        :sort-class="sortClass"
        @toggle-select-all="toggleSelectAll"
        @sort-by="emit('sort-by', $event)"
      />
      <tbody>
        <tr v-if="inlineCreatingFolder" class="inline-create-row">
          <td v-if="isAdmin" class="checkbox-col" />
          <td class="name-col">
            <div
              v-if="createFolderSaving"
              class="file-name inline-create-processing"
              @click.stop
              @contextmenu.stop
            >
              <span class="file-icon-wrap"><SvgIcon name="folder" :size="18" /></span>
              <span class="file-label" :title="createFolderPendingName">{{
                createFolderPendingName
              }}</span>
              <span class="inline-delete-status">
                <span class="inline-rename-spinner" aria-label="正在创建" />
                创建中
              </span>
            </div>
            <div v-else class="inline-rename-wrap inline-create-wrap" @click.stop @contextmenu.stop>
              <span class="file-icon-wrap"><SvgIcon name="folder" :size="18" /></span>
              <input
                :ref="bindCreateFolderInput"
                v-model="createFolderDraft"
                class="inline-rename-input"
                placeholder="输入文件夹名称"
                maxlength="100"
                @compositionstart="createFolderComposing = true"
                @compositionend="createFolderComposing = false"
                @keydown.enter="!createFolderComposing && submitInlineCreateFolder()"
                @keydown.esc.prevent="cancelInlineCreateFolder()"
                @blur="submitInlineCreateFolder()"
              />
              <button
                type="button"
                class="folder-inline-btn confirm"
                title="确认"
                @mousedown.prevent
                @click="submitInlineCreateFolder()"
              >
                ✓
              </button>
              <button
                type="button"
                class="folder-inline-btn cancel"
                title="取消"
                @mousedown.prevent
                @click="cancelInlineCreateFolder()"
              >
                ×
              </button>
            </div>
          </td>
          <td class="size-col">-</td>
          <td class="time-col">-</td>
        </tr>

        <tr v-if="showEmptyRow">
          <td :colspan="emptyColSpan" class="state state--empty-cell">
            <span v-if="loading" class="empty-spinner" aria-label="加载中" />
            <template v-else>
              <span class="empty-icon"><SvgIcon name="fa-cubes" :size="22" /></span>
              <span>{{ emptyStateText }}</span>
            </template>
          </td>
        </tr>

        <tr
          v-for="(f, index) in visibleListFiles"
          v-if="!showEmptyRow"
          :key="fileKey(f)"
          class="file-row"
          :class="{ processing: isInlineProcessing(f), 'drag-target': isActiveDropTarget(f) }"
          :draggable="isAdmin && !isInlineProcessing(f) && !isInlineRenaming(f)"
          @click="onRowClick($event, f)"
          @dblclick="onRowDblClick($event, f)"
          @contextmenu.prevent.stop="openContextMenu($event, f)"
          @dragstart="handleDragStart($event, f)"
          @dragend="handleDragEnd"
          @dragenter="handleFolderDragEnter($event, f)"
          @dragover="handleFolderDragOver($event, f)"
          @dragleave="handleFolderDragLeave($event, f)"
          @drop="handleFolderDrop($event, f)"
        >
          <td v-if="isAdmin" class="checkbox-col" @click.stop>
            <input
              :id="`file-checkbox-${index}`"
              type="checkbox"
              :checked="selectedSet.has(fileKey(f))"
              @change="toggleSelection(fileKey(f), ($event.target as HTMLInputElement).checked)"
            />
          </td>
          <td class="name-col">
            <div
              v-if="isInlineRenaming(f)"
              class="inline-rename-wrap"
              @click.stop
              @contextmenu.stop
            >
              <span class="file-icon-wrap"><FileIcon :file="f" :size="18" /></span>
              <input
                :ref="bindRenameInput"
                v-model="renameDraft"
                class="inline-rename-input"
                @compositionstart="renameComposing = true"
                @compositionend="renameComposing = false"
                @keydown.enter="!renameComposing && submitInlineRename()"
                @keydown.esc.prevent="cancelInlineRename()"
                @blur="submitInlineRename()"
              />
              <button
                type="button"
                class="folder-inline-btn confirm"
                title="确认"
                @mousedown.prevent
                @click="submitInlineRename()"
              >
                ✓
              </button>
              <button
                type="button"
                class="folder-inline-btn cancel"
                title="取消"
                @mousedown.prevent
                @click="cancelInlineRename()"
              >
                ×
              </button>
            </div>
            <div v-else class="file-name">
              <span class="file-icon-wrap"><FileIcon :file="f" :size="18" /></span>
              <span class="file-text">
                <span class="file-label" :title="f.name">{{ f.name }}</span>
                <span class="file-mobile-meta">
                  {{ formatTime(f.mod_time) }}<template v-if="!f.is_dir"> · {{ formatSize(f.size, f.is_dir) }}</template>
                </span>
              </span>
              <span v-if="isInlineProcessing(f)" class="inline-delete-status">
                <span class="inline-rename-spinner" :aria-label="getRowOperationText(f)" />
                {{ getRowOperationText(f) }}
              </span>
            </div>
          </td>
          <td class="size-col">{{ formatSize(f.size, f.is_dir) }}</td>
          <td class="time-col">{{ formatTime(f.mod_time) }}</td>
          <td v-if="isAdmin" class="actions-col">
            <div class="file-row-actions" @click.stop>
              <button
                v-if="!f.is_dir"
                type="button"
                class="file-row-action"
                title="下载"
                aria-label="下载"
                @click="runRowAction('download', f)"
              >
                <SvgIcon name="download" :size="15" />
              </button>
              <button
                type="button"
                class="file-row-action"
                title="重命名"
                aria-label="重命名"
                @click="runRowAction('rename', f)"
              >
                <SvgIcon name="pencil" :size="14" />
              </button>
              <button
                type="button"
                class="file-row-action"
                title="更多"
                aria-label="更多"
                @click="openContextMenu($event, f)"
              >
                <SvgIcon name="dots-h" :size="15" />
              </button>
            </div>
          </td>
        </tr>
        <tr
          v-if="hasMoreListFiles && !showEmptyRow"
          :ref="bindListLoadMoreSentinel"
          aria-hidden="true"
          class="file-list__load-more-row"
        >
          <td :colspan="emptyColSpan" class="file-list__load-more-sentinel" />
        </tr>
      </tbody>
    </table>

    <div v-else class="file-grid-wrapper">
      <div class="file-grid-toolbar">
        <div class="file-grid-toolbar-left">
          <label v-if="isAdmin" class="grid-select-all">
            <input
              type="checkbox"
              :checked="selectAll"
              :disabled="files.length === 0"
              @change="toggleSelectAll(($event.target as HTMLInputElement).checked)"
            />
            <span>{{ selectedCount > 0 ? `已选中 ${selectedCount} 项` : "全选" }}</span>
          </label>
          <span v-else class="grid-count">共 {{ files.length }} 项</span>
        </div>
        <FileGridSortMenu
          :sort-key="sortKey"
          :sort-order="sortOrder"
          @set-sort="emit('set-sort', $event)"
        />
      </div>

      <div v-if="showEmptyRow && !inlineCreatingFolder" class="grid-state">
        <span v-if="loading" class="empty-spinner" aria-label="加载中" />
        <template v-else>
          <span class="empty-icon"><SvgIcon name="fa-cubes" :size="40" /></span>
          <p>{{ emptyStateText }}</p>
        </template>
      </div>

      <div v-else class="file-grid">
        <article v-if="inlineCreatingFolder" class="file-card file-card-inline-create">
          <div
            v-if="createFolderSaving"
            class="file-card-main inline-create-processing"
            @click.stop
            @contextmenu.stop
          >
            <span class="file-card-icon"><SvgIcon name="folder" :size="40" /></span>
            <span class="file-card-name" :title="createFolderPendingName">{{
              createFolderPendingName
            }}</span>
            <span class="inline-delete-status">
              <span class="inline-rename-spinner" aria-label="正在创建" />
              创建中
            </span>
          </div>
          <div v-else class="file-card-main file-card-rename" @click.stop @contextmenu.stop>
            <span class="file-card-icon"><SvgIcon name="folder" :size="40" /></span>
            <div class="inline-rename-wrap inline-create-wrap">
              <input
                :ref="bindCreateFolderInput"
                v-model="createFolderDraft"
                class="inline-rename-input grid-rename-input"
                placeholder="输入文件夹名称"
                maxlength="100"
                @compositionstart="createFolderComposing = true"
                @compositionend="createFolderComposing = false"
                @keydown.enter="!createFolderComposing && submitInlineCreateFolder()"
                @keydown.esc.prevent="cancelInlineCreateFolder()"
                @blur="submitInlineCreateFolder()"
              />
              <button
                type="button"
                class="folder-inline-btn confirm"
                @mousedown.prevent
                @click="submitInlineCreateFolder()"
              >
                ✓
              </button>
              <button
                type="button"
                class="folder-inline-btn cancel"
                @mousedown.prevent
                @click="cancelInlineCreateFolder()"
              >
                ×
              </button>
            </div>
          </div>
        </article>

        <article
          v-for="(f, index) in files"
          :key="fileKey(f)"
          class="file-card"
          :class="{
            selected: selectedSet.has(fileKey(f)),
            processing: isInlineProcessing(f),
            'drag-target': isActiveDropTarget(f),
          }"
          :draggable="isAdmin && !isInlineProcessing(f) && !isInlineRenaming(f)"
          @contextmenu.prevent.stop="openContextMenu($event, f)"
          @dragstart="handleDragStart($event, f)"
          @dragend="handleDragEnd"
          @dragenter="handleFolderDragEnter($event, f)"
          @dragover="handleFolderDragOver($event, f)"
          @dragleave="handleFolderDragLeave($event, f)"
          @drop="handleFolderDrop($event, f)"
        >
          <label
            v-if="isAdmin"
            class="file-card-checkbox"
            :for="`grid-file-checkbox-${index}`"
            @click.stop
          >
            <input
              :id="`grid-file-checkbox-${index}`"
              type="checkbox"
              :checked="selectedSet.has(fileKey(f))"
              @change="toggleSelection(fileKey(f), ($event.target as HTMLInputElement).checked)"
            />
          </label>

          <div
            v-if="isInlineRenaming(f)"
            class="file-card-main file-card-rename"
            @click.stop
            @contextmenu.stop
          >
            <span class="file-card-icon"><FileIcon :file="f" :size="40" /></span>
            <input
              ref="renameInputRef"
              v-model="renameDraft"
              class="inline-rename-input grid-rename-input"
              @compositionstart="renameComposing = true"
              @compositionend="renameComposing = false"
              @keydown.enter="!renameComposing && submitInlineRename()"
              @keydown.esc.prevent="cancelInlineRename()"
              @blur="submitInlineRename()"
            />
            <div class="inline-rename-wrap">
              <button
                type="button"
                class="folder-inline-btn confirm"
                @mousedown.prevent
                @click="submitInlineRename()"
              >
                ✓
              </button>
              <button
                type="button"
                class="folder-inline-btn cancel"
                @mousedown.prevent
                @click="cancelInlineRename()"
              >
                ×
              </button>
            </div>
          </div>

          <div
            v-else
            role="button"
            tabindex="0"
            class="file-card-main"
            @click="onCardClick(f)"
            @dblclick="onCardDblClick(f)"
            @keydown.enter="onCardClick(f)"
          >
            <span class="file-card-icon">
              <FileIcon :file="f" :size="56" thumbnail :account-id="props.accountId ?? null" />
            </span>
            <span class="file-card-name" :title="f.name">{{ f.name }}</span>
            <span v-if="isInlineProcessing(f)" class="inline-delete-status">
              <span class="inline-rename-spinner" :aria-label="getRowOperationText(f)" />
              {{ getRowOperationText(f) }}
            </span>
            <span v-else class="file-card-time">{{ formatTime(f.mod_time) }}</span>
            <span v-if="isAdmin && !isInlineProcessing(f) && !isInlineRenaming(f)" class="file-card-actions" @click.stop>
              <button
                v-if="!f.is_dir"
                type="button"
                class="file-card-action"
                title="下载"
                aria-label="下载"
                @click="runRowAction('download', f)"
              >
                <SvgIcon name="download" :size="14" />
              </button>
              <button
                type="button"
                class="file-card-action"
                title="重命名"
                aria-label="重命名"
                @click="runRowAction('rename', f)"
              >
                <SvgIcon name="pencil" :size="13" />
              </button>
              <button
                type="button"
                class="file-card-action"
                title="更多"
                aria-label="更多"
                @click="openContextMenu($event, f)"
              >
                <SvgIcon name="dots-h" :size="14" />
              </button>
            </span>
          </div>
        </article>
      </div>
    </div>

    <FileContextMenu
      :open="contextMenu.open"
      :x="contextMenu.x"
      :y="contextMenu.y"
      :items="contextMenuItems"
      @action="handleContextAction"
      @close="closeContextMenu"
    />
    <FileContextMenu
      :open="headerContextMenu.open"
      :x="headerContextMenu.x"
      :y="headerContextMenu.y"
      :items="headerContextMenuItems"
      @action="handleHeaderContextAction"
      @close="closeDirectoryContextMenu"
    />
  </div>
</template>

<style scoped>
.file-list {
  overflow-x: auto;
  position: relative;
  border-radius: 0 0 12px 12px;
}

.file-list.view-grid {
  overflow: visible;
}

.state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 60px 20px;
  color: var(--text-muted);
}

.state--empty p,
.state--empty-cell {
  margin: 0;
  text-align: center;
  color: var(--text-muted);
}

.empty-spinner {
  width: 22px;
  height: 22px;
  flex: 0 0 auto;
  border: 2px solid var(--border);
  border-top-color: var(--brand);
  border-radius: 999px;
  animation: spin 0.72s linear infinite;
}

.empty-icon {
  display: inline-flex;
  color: var(--text-muted);
  opacity: 0.75;
}

.file-row {
  cursor: pointer;
  transition: background 0.15s ease;
}

.file-row:hover {
  background: var(--surface-sunken);
}

.file-row.drag-target {
  position: relative;
  background:
    linear-gradient(90deg, color-mix(in srgb, var(--brand) 14%, var(--surface)) 0%, transparent 100%),
    var(--info-soft);
  box-shadow: 0 8px 22px color-mix(in srgb, var(--brand) 14%, transparent);
  transform: none;
}

.file-row.drag-target::after {
  content: "";
  position: absolute;
  inset: 4px 8px;
  border-radius: 12px;
  border: 1px dashed color-mix(in srgb, var(--brand) 48%, transparent);
  pointer-events: none;
}

.file-list__load-more-row {
  pointer-events: none;
}

.file-list__load-more-sentinel {
  height: 1px;
  padding: 0;
  border-bottom: none;
  background: transparent;
}

.file-name {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  font-size: 14px;
  color: var(--text-regular);
  min-width: 0;
}

.file-icon-wrap {
  line-height: 0;
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
}

.file-label {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 1.4;
}

.file-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.file-mobile-meta {
  display: none;
  margin-top: 2px;
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.3;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.file-grid-wrapper {
  padding: 0 0 16px;
}

.file-grid-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 52px;
  padding: 0 16px;
  background: var(--surface-muted);
  border-bottom: 1px solid var(--border-soft);
}

.grid-select-all {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text-regular);
  cursor: pointer;
  user-select: none;
}

.grid-count {
  font-size: 13px;
  color: var(--text-muted);
}

.file-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(142px, 1fr));
  gap: 14px 12px;
  padding: 16px;
}

.file-card {
  position: relative;
  min-width: 0;
  border-radius: 14px;
}

.file-card-main {
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 18px 10px 14px;
  border: none;
  border-radius: 14px;
  background: transparent;
  cursor: pointer;
  text-align: center;
  transition: background-color 0.18s ease;
}

.file-card:hover .file-card-main,
.file-card.selected .file-card-main {
  background: var(--surface-sunken);
}

/* 键盘聚焦：卡片主体（role=button + tabindex）聚焦可见 */
.file-card-main:focus-visible {
  outline: 2px solid var(--brand);
  outline-offset: 2px;
}

.file-card.selected .file-card-main {
  background: var(--info-soft);
}

.file-card.drag-target .file-card-main {
  background:
    radial-gradient(circle at top right, color-mix(in srgb, var(--brand) 16%, transparent), transparent 55%),
    var(--info-soft);
  box-shadow: 0 14px 26px color-mix(in srgb, var(--brand) 16%, transparent);
  transform: translateY(-2px) scale(1.01);
}

.file-card-icon {
  width: 56px;
  height: 56px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  line-height: 0;
}

.file-card-name {
  width: 100%;
  color: var(--text);
  font-size: 15px;
  font-weight: 500;
  line-height: 1.35;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.file-card-time {
  width: 100%;
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.4;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.file-card-checkbox {
  position: absolute;
  top: 8px;
  left: 8px;
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.18s ease;
  z-index: 2;
  display: inline-flex;
  align-items: center;
}

.file-card:hover .file-card-checkbox,
.file-card.selected .file-card-checkbox {
  opacity: 1;
  pointer-events: auto;
}

/* 触屏设备：卡片勾选常显，避免 hover-only 不可达 */
@media (hover: none) {
  .file-card-checkbox {
    opacity: 1;
    pointer-events: auto;
  }
}

.file-card-inline-create .file-card-main {
  background: var(--surface-sunken);
}

@media (max-width: 768px) {
  .file-list {
    overflow-x: hidden;
  }

  .file-name {
    align-items: flex-start;
  }

  .file-mobile-meta {
    display: block;
  }
}
</style>
