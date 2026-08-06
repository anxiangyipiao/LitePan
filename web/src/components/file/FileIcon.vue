<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { FileItem } from "@/api/types";
import { fileKind } from "@/utils/fileIcon";
import { filesApi } from "@/api/files";
import SvgIcon from "@/components/icons/SvgIcon.vue";

const props = withDefaults(
  defineProps<{
    file: FileItem;
    size?: number;
    /** 卡片视图：图片文件渲染真实缩略图，失败回退到彩色图标 */
    thumbnail?: boolean;
    accountId?: number | null;
  }>(),
  { size: 18, thumbnail: false, accountId: null },
);

const kind = computed(() => fileKind(props.file));
const isImage = computed(() => kind.value === "image" && !props.file.is_dir);
const thumbFailed = ref(false);

const thumbUrl = computed(() => {
  if (!props.thumbnail || !isImage.value || props.accountId == null) return "";
  return filesApi.previewURL(props.accountId, props.file.id, props.file.name);
});

watch(thumbUrl, () => {
  thumbFailed.value = false;
});

function handleThumbError() {
  thumbFailed.value = true;
}
</script>

<template>
  <span
    class="file-icon"
    :class="[
      `file-icon--${kind}`,
      { 'file-icon--thumb': thumbnail && isImage && !thumbFailed },
    ]"
    :style="{ width: `${size}px`, height: `${size}px` }"
  >
    <img
      v-if="thumbnail && isImage && !thumbFailed"
      :src="thumbUrl"
      :alt="file.name"
      class="file-icon__thumb"
      loading="lazy"
      @error="handleThumbError"
    />
    <template v-else>
      <SvgIcon :name="kind" :size="size" class="file-icon__svg" />
      <span v-if="kind === 'video' && !file.is_dir" class="file-icon__play" aria-hidden="true">
        <SvgIcon name="play" :size="Math.max(7, Math.round(size * 0.38))" />
      </span>
    </template>
  </span>
</template>

<style scoped>
.file-icon {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: var(--type-file);
  line-height: 0;
}
.file-icon__svg {
  color: inherit;
}

/* 文件类型差异化颜色 */
.file-icon--folder { color: var(--type-folder); }
.file-icon--video { color: var(--type-video); }
.file-icon--audio { color: var(--type-audio); }
.file-icon--image { color: var(--type-image); }
.file-icon--pdf { color: var(--type-pdf); }
.file-icon--doc { color: var(--type-doc); }
.file-icon--sheet { color: var(--type-sheet); }
.file-icon--slide { color: var(--type-slide); }
.file-icon--archive { color: var(--type-archive); }
.file-icon--code { color: var(--type-code); }
.file-icon--text { color: var(--type-text); }

/* 视频播放角标（紫红圆底 + 白色播放三角） */
.file-icon__play {
  position: absolute;
  right: -1px;
  bottom: -1px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 46%;
  height: 46%;
  min-width: 10px;
  min-height: 10px;
  border-radius: 50%;
  background: var(--type-video);
  color: #fff;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.28);
}

/* 图片缩略图：铺满图标盒，圆角 */
.file-icon--thumb {
  overflow: hidden;
  border-radius: 8px;
  background: var(--surface-sunken);
}
.file-icon__thumb {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
</style>
