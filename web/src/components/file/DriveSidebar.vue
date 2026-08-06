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
    <div class="drive-sidebar__list">
      <button
        v-for="a in props.accounts"
        :key="a.id"
        type="button"
        class="drive-sidebar__card"
        :class="{ 'drive-sidebar__card--active': modelValue === a.id }"
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
  min-width: 0;
}

.drive-sidebar__list {
  display: flex;
  flex-direction: row;
  align-items: stretch;
  gap: 10px;
  padding: 12px 4px 4px;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;
  -webkit-overflow-scrolling: touch;
}

.drive-sidebar__card {
  appearance: none;
  flex: 0 0 auto;
  width: 96px;
  min-height: 76px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 7px;
  padding: 10px 6px;
  border: 1px solid var(--border-soft);
  border-radius: 12px;
  background: var(--surface);
  cursor: pointer;
  transition: transform var(--transition), box-shadow var(--transition);
}

.drive-sidebar__card:hover {
  transform: translateY(-1px);
  box-shadow: var(--shadow-card);
}

.drive-sidebar__card:active {
  transform: translateY(0);
}

/* 选中态：浅蓝底 + 渐变描边（background-origin 双层实现渐变边框） */
.drive-sidebar__card--active {
  background-color: color-mix(in srgb, var(--brand) 9%, var(--surface));
  background-image: linear-gradient(var(--surface), var(--surface)),
    linear-gradient(90deg, var(--brand-start), var(--brand-end));
  background-origin: border-box;
  background-clip: padding-box, border-box;
  border-color: transparent;
  box-shadow: 0 0 0 1px rgba(79, 142, 247, 0.18);
}

.drive-sidebar__badge {
  flex: 0 0 auto;
  width: 34px;
  height: 34px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: var(--driver-color, #6366f1);
  color: #fff;
  font-size: 13px;
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
  max-width: 30px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.drive-sidebar__name {
  max-width: 84px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-regular);
  line-height: 1.2;
}

.drive-sidebar__card--active .drive-sidebar__name {
  color: var(--brand-strong);
}

/* 移动端压缩盘条卡片：更小更紧凑，让出竖向空间给文件区 */
@media (max-width: 768px) {
  .drive-sidebar__list {
    gap: 8px;
    padding: 8px 2px 2px;
  }

  .drive-sidebar__card {
    width: 72px;
    min-height: 54px;
    gap: 4px;
    padding: 6px 4px;
    border-radius: 10px;
  }

  .drive-sidebar__badge {
    width: 26px;
    height: 26px;
    border-radius: 8px;
    font-size: 11px;
  }

  .drive-sidebar__badge-text {
    max-width: 24px;
  }

  .drive-sidebar__name {
    max-width: 64px;
    font-size: 11px;
  }
}
</style>
