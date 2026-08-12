<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";

interface Option {
  value: string | number | boolean;
  label: string;
}

const props = withDefaults(
  defineProps<{
    modelValue: string | number | boolean | null;
    options: Option[];
    placeholder?: string;
    disabled?: boolean;
  }>(),
  { placeholder: "请选择", disabled: false },
);
const emit = defineEmits<{ "update:modelValue": [string | number | boolean] }>();

const open = ref(false);
const triggerRef = ref<HTMLButtonElement | null>(null);
const menuRef = ref<HTMLElement | null>(null);
const menuStyle = ref<Record<string, string>>({});

const selectedLabel = computed(
  () => props.options.find((o) => o.value === props.modelValue)?.label ?? "",
);

async function positionMenu() {
  await nextTick();
  await nextTick();
  const trigger = triggerRef.value;
  const menu = menuRef.value;
  if (!trigger || !menu) return;
  const rect = trigger.getBoundingClientRect();
  const menuH = menu.offsetHeight;
  const gap = 4;
  let top = rect.bottom + gap;
  if (top + menuH > window.innerHeight - 8) {
    top = Math.max(8, rect.top - menuH - gap);
  }
  menuStyle.value = {
    top: `${top}px`,
    left: `${rect.left}px`,
    width: `${rect.width}px`,
  };
}

function close() {
  open.value = false;
}

function toggle() {
  if (props.disabled) return;
  open.value = !open.value;
}

function choose(opt: Option) {
  emit("update:modelValue", opt.value);
  close();
}

function onScrollOrResize() {
  if (open.value) void positionMenu();
}

watch(open, (v) => {
  if (v) void positionMenu();
});

onMounted(() => {
  window.addEventListener("scroll", onScrollOrResize, true);
  window.addEventListener("resize", onScrollOrResize);
});
onUnmounted(() => {
  window.removeEventListener("scroll", onScrollOrResize, true);
  window.removeEventListener("resize", onScrollOrResize);
});
</script>

<template>
  <div class="select" :class="{ 'select--disabled': disabled, 'select--open': open }">
    <button
      ref="triggerRef"
      type="button"
      class="select__trigger"
      :disabled="disabled"
      @click="toggle"
    >
      <span :class="{ select__placeholder: !selectedLabel }">
        {{ selectedLabel || placeholder }}
      </span>
      <span class="select__arrow" :class="{ 'select__arrow--open': open }">▾</span>
    </button>

    <Teleport to="body">
      <template v-if="open">
        <div class="select__backdrop" @click="close" />
        <ul ref="menuRef" class="select__menu" :style="menuStyle">
          <li
            v-for="opt in options"
            :key="String(opt.value)"
            class="select__option"
            :class="{ 'select__option--active': opt.value === modelValue }"
            @click="choose(opt)"
          >
            {{ opt.label }}
          </li>
          <li v-if="!options.length" class="select__empty">无可选项</li>
        </ul>
      </template>
    </Teleport>
  </div>
</template>

<style scoped>
.select {
  position: relative;
  width: 100%;
  min-width: 0;
}
.select__trigger {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 12px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text);
  transition: var(--transition);
}
.select__trigger:hover:not(:disabled) {
  border-color: var(--brand);
}
.select--open .select__trigger {
  border-color: var(--brand);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--brand) 14%, transparent);
}
.select--disabled .select__trigger {
  opacity: 0.55;
  cursor: not-allowed;
}
.select__placeholder {
  color: var(--text-muted);
}
.select__arrow {
  color: var(--text-muted);
  transition: transform var(--transition);
}
.select__arrow--open {
  transform: rotate(180deg);
}
</style>

<style>
.select__backdrop {
  position: fixed;
  inset: 0;
  z-index: var(--z-popover);
}
.select__menu {
  position: fixed;
  z-index: calc(var(--z-popover) + 1);
  margin: 0;
  list-style: none;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow-pop);
  max-height: 260px;
  overflow-y: auto;
  padding: 4px;
  box-sizing: border-box;
}
.select__option {
  padding: 8px 12px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  color: var(--text);
}
.select__option:hover {
  background: var(--border-soft);
}
.select__option--active {
  color: var(--brand);
  font-weight: 600;
}
.select__empty {
  padding: 8px 12px;
  color: var(--text-muted);
}
</style>
