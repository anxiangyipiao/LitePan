<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from "vue";

const props = defineProps<{
  open: boolean;
  title?: string;
}>();

const emit = defineEmits<{
  close: [];
}>();

const sheetRef = ref<HTMLElement | null>(null);
let startY = 0;
let currentY = 0;
let dragging = false;

function onTouchStart(e: TouchEvent) {
  if (!props.open) return;
  startY = e.touches[0].clientY;
  dragging = true;
}

function onTouchMove(e: TouchEvent) {
  if (!dragging) return;
  currentY = e.touches[0].clientY;
  const delta = currentY - startY;
  if (delta > 0 && sheetRef.value) {
    sheetRef.value.style.transform = `translateY(${delta}px)`;
  }
}

function onTouchEnd() {
  if (!dragging) return;
  dragging = false;
  const delta = currentY - startY;
  if (sheetRef.value) {
    sheetRef.value.style.transform = "";
  }
  if (delta > 80) {
    emit("close");
  }
}

function onBackdropClick() {
  emit("close");
}

function onKeyDown(e: KeyboardEvent) {
  if (e.key === "Escape" && props.open) {
    emit("close");
  }
}

onMounted(() => {
  document.addEventListener("keydown", onKeyDown);
});

onUnmounted(() => {
  document.removeEventListener("keydown", onKeyDown);
});

watch(
  () => props.open,
  (v) => {
    document.body.style.overflow = v ? "hidden" : "";
  },
);
</script>

<template>
  <Teleport to="body">
    <Transition name="bottom-sheet">
      <div v-if="open" class="bottom-sheet-backdrop" @click="onBackdropClick">
        <div
          ref="sheetRef"
          class="bottom-sheet"
          @click.stop
          @touchstart="onTouchStart"
          @touchmove="onTouchMove"
          @touchend="onTouchEnd"
        >
          <div class="bottom-sheet__handle" />
          <div v-if="title" class="bottom-sheet__title">{{ title }}</div>
          <div class="bottom-sheet__content">
            <slot />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.bottom-sheet-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1000;
  background: rgba(0, 0, 0, 0.4);
  display: flex;
  align-items: flex-end;
  justify-content: center;
}

.bottom-sheet {
  width: 100%;
  max-width: 480px;
  max-height: 85vh;
  background: var(--bg-primary, #fff);
  border-radius: 16px 16px 0 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding-bottom: env(safe-area-inset-bottom, 0);
  touch-action: pan-y;
}

.bottom-sheet__handle {
  width: 36px;
  height: 4px;
  background: var(--border-strong, #d1d5db);
  border-radius: 2px;
  margin: 10px auto 6px;
}

.bottom-sheet__title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary, #1f2937);
  padding: 4px 20px 10px;
  border-bottom: 1px solid var(--border-soft, #e5e7eb);
}

.bottom-sheet__content {
  padding: 8px 0;
}

/* Transition */
.bottom-sheet-enter-active,
.bottom-sheet-leave-active {
  transition: opacity 0.25s ease;
}
.bottom-sheet-enter-active .bottom-sheet,
.bottom-sheet-leave-active .bottom-sheet {
  transition: transform 0.3s cubic-bezier(0.32, 0.72, 0, 1);
}
.bottom-sheet-enter-from,
.bottom-sheet-leave-to {
  opacity: 0;
}
.bottom-sheet-enter-from .bottom-sheet,
.bottom-sheet-leave-to .bottom-sheet {
  transform: translateY(100%);
}
</style>
