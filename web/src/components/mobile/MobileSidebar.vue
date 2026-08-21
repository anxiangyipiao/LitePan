<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, nextTick } from "vue";

const props = defineProps<{
  open: boolean;
}>();

const emit = defineEmits<{
  close: [];
}>();

const panelRef = ref<HTMLElement | null>(null);
let lastActive: HTMLElement | null = null;
let savedScrollY = 0;

function onBackdropClick() {
  emit("close");
}

function onKeyDown(e: KeyboardEvent) {
  if (e.key === "Escape" && props.open) {
    emit("close");
    return;
  }
  if (e.key !== "Tab" || !props.open || !panelRef.value) return;
  const focusable = Array.from(
    panelRef.value.querySelectorAll<HTMLElement>(
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
  document.body.style.position = "";
  document.body.style.top = "";
  document.body.style.left = "";
  document.body.style.right = "";
  document.body.style.width = "";
  document.body.style.overflow = "";
});

watch(
  () => props.open,
  async (v) => {
    if (v) {
      lastActive = document.activeElement as HTMLElement | null;
      savedScrollY = window.scrollY || 0;
      document.body.style.position = "fixed";
      document.body.style.top = `-${savedScrollY}px`;
      document.body.style.left = "0";
      document.body.style.right = "0";
      document.body.style.width = "100%";
      document.body.style.overflow = "hidden";
      await nextTick();
      panelRef.value?.querySelector<HTMLElement>("button, a, [tabindex]")?.focus();
    } else {
      document.body.style.position = "";
      document.body.style.top = "";
      document.body.style.left = "";
      document.body.style.right = "";
      document.body.style.width = "";
      document.body.style.overflow = "";
      window.scrollTo(0, savedScrollY);
      lastActive?.focus?.();
      lastActive = null;
    }
  },
);
</script>

<template>
  <Teleport to="body">
    <Transition name="mobile-sidebar">
      <div
        v-if="open"
        class="mobile-sidebar-backdrop"
        role="dialog"
        aria-modal="true"
        aria-label="导航抽屉"
        @click="onBackdropClick"
      >
        <nav ref="panelRef" class="mobile-sidebar" @click.stop>
          <div class="mobile-sidebar__header">
            <span class="mobile-sidebar__title">导航</span>
            <button class="mobile-sidebar__close" @click="emit('close')" aria-label="关闭">
              <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          </div>
          <div class="mobile-sidebar__body">
            <slot />
          </div>
        </nav>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.mobile-sidebar-backdrop {
  position: fixed;
  inset: 0;
  z-index: 900;
  background: rgba(0, 0, 0, 0.35);
}

.mobile-sidebar {
  position: fixed;
  top: 0;
  left: 0;
  bottom: 0;
  width: 280px;
  max-width: 80vw;
  background: var(--bg-primary, #fff);
  box-shadow: 4px 0 24px rgba(0, 0, 0, 0.12);
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding-top: env(safe-area-inset-top, 0);
}

.mobile-sidebar__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 16px 12px;
  border-bottom: 1px solid var(--border-soft, #e5e7eb);
}

.mobile-sidebar__title {
  font-size: 17px;
  font-weight: 600;
  color: var(--text-primary, #1f2937);
}

.mobile-sidebar__close {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: none;
  border-radius: 8px;
  color: var(--text-secondary, #6b7280);
  cursor: pointer;
}
.mobile-sidebar__close:active {
  background: var(--bg-secondary, #f3f4f6);
}

.mobile-sidebar__body {
  flex: 1;
  overflow-y: auto;
  padding: 8px 0;
}

/* Transition */
.mobile-sidebar-enter-active,
.mobile-sidebar-leave-active {
  transition: opacity 0.25s ease;
}
.mobile-sidebar-enter-active .mobile-sidebar,
.mobile-sidebar-leave-active .mobile-sidebar {
  transition: transform 0.3s cubic-bezier(0.32, 0.72, 0, 1);
}
.mobile-sidebar-enter-from,
.mobile-sidebar-leave-to {
  opacity: 0;
}
.mobile-sidebar-enter-from .mobile-sidebar,
.mobile-sidebar-leave-to .mobile-sidebar {
  transform: translateX(-100%);
}
</style>
