<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import type { Account, BrowserFavoriteItem } from "@/api/types";
import SvgIcon from "@/components/icons/SvgIcon.vue";

// Edge 收藏式下拉面板：点工具栏按钮弹出，点外部/Esc 关闭。
const props = defineProps<{
  items: BrowserFavoriteItem[];
  accounts: Account[];
  currentCrumbIds: string[];
  currentAccountId: number | null;
  currentFolderFavorited: boolean;
}>();

const emit = defineEmits<{
  "add-current": [];
  collapse: [];
  open: [item: BrowserFavoriteItem];
  rename: [item: BrowserFavoriteItem];
  remove: [item: BrowserFavoriteItem];
  move: [item: BrowserFavoriteItem, direction: -1 | 1];
}>();

const rootEl = ref<HTMLElement | null>(null);
const editing = ref(false);

function favoriteKey(item: BrowserFavoriteItem) {
  return `${item.account_id ?? 0}:${item.id}`;
}

function accountName(item: BrowserFavoriteItem) {
  if (item.account_id == null) return "";
  return props.accounts.find((a) => a.id === item.account_id)?.name || "";
}

function formatFavoritePath(item: BrowserFavoriteItem) {
  const names = item.crumbs.map((c) => c.name).filter((n) => n && n !== "根目录");
  return names.length ? `/${names.join("/")}` : "/";
}

function isFavoriteActive(item: BrowserFavoriteItem) {
  if (item.account_id != null && item.account_id !== props.currentAccountId) return false;
  const ids = item.crumbs.map((c) => c.id);
  return (
    ids.length === props.currentCrumbIds.length &&
    ids.every((id, i) => id === props.currentCrumbIds[i])
  );
}

function onDocPointerDown(e: MouseEvent) {
  if (rootEl.value && !rootEl.value.contains(e.target as Node)) emit("collapse");
}

function onDocKeydown(e: KeyboardEvent) {
  if (e.key === "Escape") emit("collapse");
}

onMounted(() => {
  document.addEventListener("mousedown", onDocPointerDown);
  document.addEventListener("keydown", onDocKeydown);
});

onUnmounted(() => {
  document.removeEventListener("mousedown", onDocPointerDown);
  document.removeEventListener("keydown", onDocKeydown);
});
</script>

<template>
  <div ref="rootEl" class="fav-dropdown">
    <header class="fav-dropdown__head">
      <span class="fav-dropdown__title">收藏夹</span>
      <span class="fav-dropdown__head-actions">
        <button
          type="button"
          class="fav-dropdown__head-btn"
          :class="{ active: currentFolderFavorited }"
          :title="currentFolderFavorited ? '当前文件夹已收藏' : '收藏当前文件夹'"
          :aria-label="currentFolderFavorited ? '当前文件夹已收藏' : '收藏当前文件夹'"
          @click="emit('add-current')"
        >
          <SvgIcon name="package" :size="14" />
          <span>{{ currentFolderFavorited ? "已收藏" : "收藏当前" }}</span>
        </button>
        <button
          v-if="items.length"
          type="button"
          class="fav-dropdown__head-btn"
          :class="{ active: editing }"
          :title="editing ? '退出编辑' : '编辑收藏夹'"
          @click="editing = !editing"
        >
          <SvgIcon name="pencil" :size="13" />
          <span>{{ editing ? "完成" : "编辑" }}</span>
        </button>
        <button
          type="button"
          class="fav-dropdown__close"
          title="关闭收藏夹"
          aria-label="关闭收藏夹"
          @click="emit('collapse')"
        >
          <SvgIcon name="sign-out" :size="13" />
        </button>
      </span>
    </header>

    <div class="fav-dropdown__list">
      <div v-if="items.length">
        <div
          v-for="item in items"
          :key="favoriteKey(item)"
          class="fav-dropdown__row"
          :class="{ active: isFavoriteActive(item), editing }"
          :title="formatFavoritePath(item)"
        >
          <component
            :is="editing ? 'div' : 'button'"
            :type="editing ? undefined : 'button'"
            class="fav-dropdown__main"
            @click="!editing && emit('open', item)"
          >
            <span class="fav-dropdown__label">{{ item.name }}</span>
            <span class="fav-dropdown__path">
              <span v-if="accountName(item)" class="fav-dropdown__account">{{ accountName(item) }}</span>
              {{ formatFavoritePath(item) }}
            </span>
          </component>

          <div v-if="editing" class="fav-dropdown__actions">
            <button
              type="button"
              class="fav-dropdown__act"
              title="重命名收藏"
              @click.stop="emit('rename', item)"
            >
              <SvgIcon name="pencil" :size="12" />
            </button>
            <button
              type="button"
              class="fav-dropdown__act"
              title="上移"
              :disabled="items[0] ? favoriteKey(items[0]) === favoriteKey(item) : true"
              @click.stop="emit('move', item, -1)"
            >
              ↑
            </button>
            <button
              type="button"
              class="fav-dropdown__act"
              title="下移"
              :disabled="items[items.length - 1] ? favoriteKey(items[items.length - 1]) === favoriteKey(item) : true"
              @click.stop="emit('move', item, 1)"
            >
              ↓
            </button>
            <button
              type="button"
              class="fav-dropdown__act fav-dropdown__act--danger"
              title="移除收藏"
              @click.stop="emit('remove', item)"
            >
              <SvgIcon name="trash" :size="12" />
            </button>
          </div>
        </div>
      </div>
      <div v-else class="fav-dropdown__empty">暂无收藏</div>
    </div>
  </div>
</template>

<style scoped>
.fav-dropdown {
  width: 320px;
  max-height: 70vh;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border-soft);
  border-radius: 12px;
  background: var(--surface);
  box-shadow: 0 16px 44px rgba(15, 23, 42, 0.18);
  overflow: hidden;
}

.fav-dropdown__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--border-soft);
  background: var(--surface-muted);
}

.fav-dropdown__title {
  color: var(--text-regular);
  font-size: 13px;
  font-weight: 600;
}

.fav-dropdown__head-actions {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.fav-dropdown__head-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 4px 8px;
  border: 1px solid var(--border-soft);
  border-radius: 7px;
  background: var(--surface);
  color: var(--text-muted);
  font-size: 12px;
  cursor: pointer;
  transition: all 0.18s ease;
}

.fav-dropdown__head-btn:hover,
.fav-dropdown__head-btn.active {
  color: var(--brand);
  border-color: color-mix(in srgb, var(--brand) 34%, var(--border));
}

.fav-dropdown__close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: none;
  border-radius: 7px;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
}

.fav-dropdown__close:hover {
  color: var(--brand);
  background: var(--surface-sunken);
}

.fav-dropdown__list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 4px;
}

.fav-dropdown__row {
  display: flex;
  align-items: center;
  gap: 8px;
  border-radius: 8px;
}

.fav-dropdown__row:not(.editing):hover,
.fav-dropdown__row.active {
  background: var(--surface-sunken);
}

.fav-dropdown__row.editing {
  padding-right: 4px;
}

.fav-dropdown__main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
  border: none;
  background: transparent;
  text-align: left;
  color: inherit;
  cursor: pointer;
}

.fav-dropdown__label {
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.fav-dropdown__path {
  font-size: 12px;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.fav-dropdown__account {
  color: var(--brand);
  font-weight: 600;
  margin-right: 6px;
}

.fav-dropdown__actions {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.fav-dropdown__act {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: 1px solid var(--border-soft);
  border-radius: 6px;
  background: var(--surface);
  color: var(--text-regular);
  font-size: 12px;
  cursor: pointer;
}

.fav-dropdown__act:hover:not(:disabled) {
  color: var(--brand);
  border-color: color-mix(in srgb, var(--brand) 34%, var(--border));
}

.fav-dropdown__act:disabled {
  opacity: 0.35;
  cursor: default;
}

.fav-dropdown__act--danger:hover:not(:disabled) {
  color: var(--danger);
  border-color: color-mix(in srgb, var(--danger) 34%, var(--border));
}

.fav-dropdown__empty {
  padding: 28px 12px;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
}
</style>
