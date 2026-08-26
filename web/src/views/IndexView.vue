<script setup lang="ts">
import FileBrowser from "@/components/file/FileBrowser.vue";
</script>

<template>
  <div class="index">
    <FileBrowser />
  </div>
</template>

<style scoped>
.index {
  min-height: 100%;
  padding: 0 16px;
  box-sizing: border-box;
  overflow-x: hidden;
}
</style>
