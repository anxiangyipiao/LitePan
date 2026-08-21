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
let startTime = 0;
let dragging = false;

function onTouchStart(e: TouchEvent) {
  if (!props.open) return;
  startY = e.touches[0].clientY;
  currentY = startY;
  startTime = Date.now();
  dragging = true;
  if (sheetRef.value) sheetRef.value.style.transition = "none";
}

function onTouchMove(e: TouchEvent) {
  if (!dragging) return;
  currentY = e.touches[0].clientY;
  const delta = currentY - startY;
  if (delta > 0 && sheetRef.value) {
    // 轻微阻尼，避免过度跟手
    const damped = delta * 0.92;
    sheetRef.value.style.transform = `translateY(${damped}px)`;
    if (e.cancelable) e.preventDefault();
  }
}

function onTouchEnd() {
  if (!dragging) return;
  dragging = false;
  const delta = currentY - startY;
  const elapsed = Math.max(1, Date.now() - startTime);
  const velocity = delta / elapsed; // px/ms
  if (sheetRef.value) {
    sheetRef.value.style.transition = "transform 180ms ease";
    sheetRef.value.style.transform = "";
  }
  if (delta > 80 || velocity > 0.6) {
    emit("close");
  }
}

function onBackdropClick() {
  emit("close");
}

function onKeyDown(e: KeyboardEvent) {
  if (e.key === "Escape" && props.open) {
    emit("close");
    return;
  }
  if (e.key !== "Tab" || !props.open || !sheetRef.value) return;
  const focusable = Array.from(
    sheetRef.value.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ),
  );
  if (!focusable.length) return;
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  const active = document.activeElement as HTMLElement | null;
  if (e.shiftKey && active === first) {
    e.preventDefault();
    last.focus();
  } else if (!e.shiftKey && active === last) {
    e.preventDefault();
    first.focus();
  }
}

onMounted(() => {
  document.addEventListener("keydown", onKeyDown);
});

onUnmounted(() => {
  document.removeEventListener("keydown", onKeyDown);
});

let savedScrollY = 0;

watch(
  () => props.open,
  (v) => {
    if (v) {
      savedScrollY = window.scrollY || 0;
      document.body.style.position = "fixed";
      document.body.style.top = `-${savedScrollY}px`;
      document.body.style.left = "0";
      document.body.style.right = "0";
      document.body.style.width = "100%";
      document.body.style.overflow = "hidden";
      requestAnimationFrame(() => sheetRef.value?.querySelector<HTMLElement>("button, a, [tabindex]")?.focus());
    } else {
      document.body.style.position = "";
      document.body.style.top = "";
      document.body.style.left = "";
      document.body.style.right = "";
      document.body.style.width = "";
      document.body.style.overflow = "";
      window.scrollTo(0, savedScrollY);
    }
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
