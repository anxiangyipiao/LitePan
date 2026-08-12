<script setup lang="ts">
import { onUnmounted, watch } from "vue";
import { lockPageScroll, unlockPageScroll } from "@/utils/scrollLock";

const props = withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    size?: "sm" | "md" | "lg" | "account" | "branch";
    // bare：仅渲染弹窗外壳与默认插槽，去掉默认头部/内边距，由内容自带头部时使用。
    bare?: boolean;
    // nested：叠在另一层弹窗之上（目录选择等）。
    nested?: boolean;
  }>(),
  { title: "", size: "md", bare: false, nested: false },
);
const emit = defineEmits<{ close: [] }>();

function onKey(e: KeyboardEvent) {
  if (e.key === "Escape") emit("close");
}

function lockPageScrollState(lock: boolean) {
  if (lock) lockPageScroll();
  else unlockPageScroll();
}

watch(
  () => props.open,
  (open) => {
    if (open) {
      window.addEventListener("keydown", onKey);
      lockPageScrollState(true);
    } else {
      window.removeEventListener("keydown", onKey);
      lockPageScrollState(false);
    }
  },
);
onUnmounted(() => {
  window.removeEventListener("keydown", onKey);
  lockPageScrollState(false);
});
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <!-- overlay 滚动：溢出时滚动条在视口右侧，而非弹窗内部 -->
      <div v-if="open" class="overlay" :class="{ 'overlay--nested': nested }">
        <div class="overlay__center">
          <div
            class="modal"
            :class="bare ? 'modal--bare' : `modal--${size}`"
            role="dialog"
          >
            <template v-if="bare">
              <slot />
            </template>
            <template v-else>
              <header class="modal__head">
                <slot name="header">
                  <h3 v-if="title" class="modal__title">{{ title }}</h3>
                </slot>
                <button class="modal__close" aria-label="关闭" @click="emit('close')">×</button>
              </header>
              <div class="modal__body">
                <slot />
              </div>
              <footer v-if="$slots.footer" class="modal__foot">
                <slot name="footer" />
              </footer>
            </template>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  z-index: var(--z-modal);
  overflow-y: auto;
  background: var(--modal-scrim);
}
.overlay--nested {
  z-index: calc(var(--z-modal) + 40);
}

.overlay__center {
  min-height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  box-sizing: border-box;
}

.modal {
  width: 100%;
  background: var(--surface);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-pop);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

.modal--sm {
  max-width: 420px;
}
.modal--md {
  max-width: 620px;
}
.modal--lg {
  max-width: 860px;
}
.modal--account {
  max-width: 700px;
  width: 90%;
}
.modal--account .modal__head {
  padding: 14px 24px;
}
.modal--account .modal__body {
  padding: 16px 20px 18px;
}
.modal--branch {
  max-width: 900px;
  width: 90%;
}
.modal--branch .modal__body {
  min-height: 540px;
  max-height: min(86vh, 620px);
  display: flex;
  flex-direction: column;
}
.modal--bare {
  width: auto;
  max-width: min(94vw, 960px);
  overflow: visible;
}

.modal__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 18px 24px;
  background: var(--panel-head-bg);
  border-bottom: 1px solid var(--border);
  border-radius: var(--radius-md) var(--radius-md) 0 0;
}
.modal__title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text);
}
.modal__close {
  border: none;
  background: transparent;
  font-size: 22px;
  line-height: 1;
  color: var(--text-muted);
}
.modal__close:hover {
  color: var(--text);
}
.modal__body {
  padding: 20px;
}
.modal__foot {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 4px 20px 18px;
}

.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}
.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}
</style>
