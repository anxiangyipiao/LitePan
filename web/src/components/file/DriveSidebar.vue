<script setup lang="ts">
import type { Account } from "@/api/types";

const props = defineProps<{
  accounts: Account[];
  modelValue: number | null;
}>();
const emit = defineEmits<{ "update:modelValue": [number] }>();

function driverColor(a: Account): string {
  return a.driver_card_color || "#6366f1";
}

function driverText(a: Account): string {
  const card = String(a.driver_card_name || "").trim();
  if (card) return card;
  const driverType = String(a.driver_type || "").trim();
  return driverType ? driverType.slice(0, 2).toUpperCase() : "盘";
}
</script>

<template>
  <div class="drive-sidebar">
    <div class="drive-sidebar__head">
      <span class="drive-sidebar__title">存储盘</span>
    </div>
    <div class="drive-sidebar__list">
      <button
        v-for="a in props.accounts"
        :key="a.id"
        type="button"
        class="drive-sidebar__row"
        :class="{ 'drive-sidebar__row--active': modelValue === a.id }"
        :style="{ '--driver-color': driverColor(a) }"
        :title="a.name"
        @click="emit('update:modelValue', a.id)"
      >
        <span class="drive-sidebar__badge" :class="{ 'drive-sidebar__badge--logo': a.driver_card_logo }">
          <img
            v-if="a.driver_card_logo"
            :src="a.driver_card_logo"
            :alt="a.name"
            class="drive-sidebar__logo"
          />
          <span v-else class="drive-sidebar__badge-text">{{ driverText(a) }}</span>
        </span>
        <span class="drive-sidebar__name">{{ a.name }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.drive-sidebar {
  display: flex;
  flex-direction: column;
  background: var(--surface);
  min-width: 0;
  height: 100%;
}

.drive-sidebar__head {
  height: 52px;
  display: flex;
  align-items: center;
  padding: 0 12px 0 16px;
  background: var(--surface-muted);
  border-bottom: 1px solid var(--border-soft);
  flex: 0 0 auto;
}

.drive-sidebar__title {
  color: var(--text-muted);
  font-size: 13px;
  font-weight: 500;
  line-height: 1;
}

.drive-sidebar__list {
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
}

.drive-sidebar__row {
  appearance: none;
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 10px 12px 10px 16px;
  border: 0;
  border-left: 2px solid transparent;
  background: transparent;
  text-align: left;
  cursor: pointer;
  transition: background 0.18s ease, border-left-color 0.18s ease;
  min-width: 0;
}

.drive-sidebar__row:hover {
  background: var(--surface-hover);
  border-left-color: var(--brand);
}

.drive-sidebar__row--active {
  background: color-mix(in srgb, var(--brand) 10%, var(--surface));
  border-left-color: var(--brand);
}

.drive-sidebar__badge {
  flex: 0 0 auto;
  width: 28px;
  height: 28px;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: var(--driver-color, #6366f1);
  color: #fff;
  font-size: 12px;
  font-weight: 700;
  line-height: 1;
  overflow: hidden;
}

.drive-sidebar__badge--logo {
  background: #fff;
}

.drive-sidebar__logo {
  width: 100%;
  height: 100%;
  object-fit: contain;
  display: block;
}

.drive-sidebar__badge-text {
  max-width: 24px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.drive-sidebar__name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
  line-height: 1.2;
}

:root[data-theme="dark"] .drive-sidebar__row--active {
  background: color-mix(in srgb, var(--brand) 12%, var(--surface));
}

@media (max-width: 768px) {
  .drive-sidebar__list {
    flex-direction: row;
    overflow-x: auto;
    gap: 8px;
    padding: 8px 12px;
  }

  .drive-sidebar__row {
    flex: 0 0 auto;
    width: auto;
    padding: 8px 10px;
    border-left: none;
    border-radius: 8px;
  }

  .drive-sidebar__row--active {
    background: color-mix(in srgb, var(--brand) 12%, var(--surface));
  }
}
</style>
