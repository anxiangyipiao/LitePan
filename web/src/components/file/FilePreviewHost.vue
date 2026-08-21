<script setup lang="ts">
import { computed, defineAsyncComponent, h } from "vue";
import BusySpinner from "@/components/base/BusySpinner.vue";

const previewFallback = () => h(BusySpinner, { variant: "notch", size: 28, color: "var(--brand)" });

function lazyPreview(loader: () => Promise<unknown>) {
  return defineAsyncComponent({
    loader: loader as never,
    loadingComponent: previewFallback as never,
    delay: 200,
    timeout: 15000,
  });
}
import type { FileItem } from "@/api/types";
import type { ActiveFilePreview, FilePreviewKind } from "./filePreview";

const props = defineProps<{
  accountId: number;
  files: FileItem[];
  active: ActiveFilePreview;
}>();

const emit = defineEmits<{
  close: [];
  download: [file: FileItem];
}>();

const previewComponents = {
  video: lazyPreview(() => import("./VideoPreview.vue")),
  audio: lazyPreview(() => import("./AudioPreview.vue")),
  image: lazyPreview(() => import("./ImagePreview.vue")),
  text: lazyPreview(() => import("./TextPreview.vue")),
  pdf: lazyPreview(() => import("./PdfPreview.vue")),
  docx: lazyPreview(() => import("./DocxPreview.vue")),
  spreadsheet: lazyPreview(() => import("./SpreadsheetPreview.vue")),
  archive: lazyPreview(() => import("./ArchivePreview.vue")),
  pptx: lazyPreview(() => import("./PptxPreview.vue")),
} satisfies Record<FilePreviewKind, ReturnType<typeof defineAsyncComponent>>;

const mediaPreviewKinds = new Set<FilePreviewKind>(["video", "audio", "image"]);
const activeComponent = computed(() => previewComponents[props.active.kind]);
const activeProps = computed(() =>
  mediaPreviewKinds.has(props.active.kind)
    ? { files: props.files, initialFileId: props.active.file.id }
    : { file: props.active.file },
);

function forwardDownload(file: FileItem) {
  emit("download", file);
}
</script>

<template>
  <component
    :is="activeComponent"
    :account-id="accountId"
    v-bind="activeProps"
    @close="emit('close')"
    @download="forwardDownload"
  />
</template>
