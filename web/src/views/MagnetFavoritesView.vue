<script setup lang="ts">
import { computed, onActivated, onDeactivated, onMounted, ref } from "vue";
import AppButton from "@/components/base/AppButton.vue";
import BusySpinner from "@/components/base/BusySpinner.vue";
import MagnetOfflineModal from "@/components/common/MagnetOfflineModal.vue";
import { addToQB } from "@/api/qb";
import { getApiErrorMessage } from "@/api/client";
import { formatSize } from "@/utils/format";
import { toast, copyTextToClipboard } from "@/composables/useToast";
import { confirm } from "@/composables/useConfirm";
import { useMagnetFavorites } from "@/composables/useMagnetFavorites";
import type { MagnetFavorite } from "@/api/magnetFavorites";
import "@/styles/magnet.css";

const { items, loading, refresh, unfavorite } = useMagnetFavorites();

const qbPushing = ref<Record<string, boolean>>({});
const offlineOpen = ref(false);
const offlineMagnet = ref("");
const offlineName = ref("");

const sortedItems = computed(() => [...items.value]);

onMounted(() => {
  void refresh();
});

onActivated(() => {
  void refresh();
});

onDeactivated(() => {});

function relativeTime(unix: number): string {
  if (!unix) return "-";
  const d = new Date(unix * 1000);
  if (Number.isNaN(d.getTime())) return "-";
  const diff = Date.now() - d.getTime();
  if (diff < 60_000) return "刚刚";
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`;
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

async function copyMagnet(r: MagnetFavorite) {
  if (!r.magnet) {
    toast.warning("该结果没有磁力链");
    return;
  }
  await copyTextToClipboard(r.magnet, { successMessage: "磁力链已复制" });
}

async function pushToQB(r: MagnetFavorite) {
  if (!r.magnet) {
    toast.warning("该结果没有磁力链");
    return;
  }
  qbPushing.value[r.hash] = true;
  try {
    await addToQB(r.magnet);
    toast.success("已推送到 qBittorrent");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "推送到 qB 失败，请检查 qB 地址与账号配置"));
  } finally {
    qbPushing.value[r.hash] = false;
  }
}

function openOffline(r: MagnetFavorite) {
  if (!r.magnet) {
    toast.warning("该结果没有磁力链");
    return;
  }
  offlineMagnet.value = r.magnet;
  offlineName.value = r.name;
  offlineOpen.value = true;
}

async function remove(r: MagnetFavorite) {
  try {
    await confirm({
      title: "取消收藏",
      message: `确认取消收藏「${r.name}」？`,
      confirmText: "取消收藏",
      danger: true,
      icon: "warning",
    });
    await unfavorite(r.hash);
  } catch (e) {
    if ((e as { message?: string })?.message === "Modal closed") return;
  }
}
</script>

<template>
  <div class="magnet-page">
    <header class="magnet-page__head">
      <div class="magnet-page__hero magnet-page__hero--favorites">
        <div class="magnet-page__hero-info">
          <div class="magnet-page__icon">
            <i class="fa-solid fa-star" aria-hidden="true"></i>
          </div>
          <div class="magnet-page__hero-text">
            <h1 class="magnet-page__title">我的收藏</h1>
            <p class="magnet-page__desc">保存感兴趣的磁力结果，方便后续推送到 qB 或离线下载</p>
          </div>
        </div>
      </div>
    </header>

    <div v-if="loading && !sortedItems.length" class="magnet-page__loading">
      <BusySpinner :size="22" />
      <span>正在加载收藏…</span>
    </div>

    <div v-else-if="!sortedItems.length" class="magnet-page__empty">
      <i class="fa-regular fa-star" aria-hidden="true"></i>
      <p>还没有收藏的磁力。回到「磁力搜索」点 ⭐ 把感兴趣的结果加入这里。</p>
    </div>

    <div v-if="sortedItems.length" class="magnet-page__stats">
      <span class="magnet-page__stats-item">
        <i class="fa-solid fa-list" aria-hidden="true"></i>
        共 {{ sortedItems.length }} 条收藏
      </span>
    </div>

    <ul v-if="sortedItems.length" class="magnet-page__list">
      <li v-for="r in sortedItems" :key="r.hash" class="magnet-page__row">
        <div class="magnet-page__main">
          <div class="magnet-page__name" :title="r.name">{{ r.name }}</div>
          <div class="magnet-page__meta">
            <span v-if="r.category" class="magnet-page__tag magnet-page__tag--cat">
              <i class="fa-solid fa-folder-open" aria-hidden="true"></i>{{ r.category }}
            </span>
            <span class="magnet-page__tag magnet-page__tag--size">
              <i class="fa-solid fa-database" aria-hidden="true"></i>{{ formatSize(r.size) }}
            </span>
            <span class="magnet-page__tag magnet-page__tag--seed">
              <i class="fa-solid fa-arrow-up" aria-hidden="true"></i>{{ r.seeders }}
            </span>
            <span class="magnet-page__tag magnet-page__tag--leech">
              <i class="fa-solid fa-arrow-down" aria-hidden="true"></i>{{ r.leechers }}
            </span>
            <span class="magnet-page__tag magnet-page__tag--date">
              <i class="fa-regular fa-clock" aria-hidden="true"></i>
              收藏于 {{ relativeTime(r.created_at) }}
            </span>
          </div>
        </div>
        <div class="magnet-page__actions">
          <AppButton type="button" variant="secondary" size="sm" @click="copyMagnet(r)">
            <i class="fa-solid fa-copy" aria-hidden="true"></i>复制
          </AppButton>
          <AppButton
            type="button"
            variant="primary"
            size="sm"
            :disabled="!!qbPushing[r.hash]"
            @click="pushToQB(r)"
          >
            <i class="fa-solid fa-download" aria-hidden="true"></i>
            {{ qbPushing[r.hash] ? "推送中…" : "下载到 qB" }}
          </AppButton>
          <AppButton type="button" variant="secondary" size="sm" @click="openOffline(r)">
            <i class="fa-solid fa-cloud-arrow-down" aria-hidden="true"></i>离线
          </AppButton>
          <a
            v-if="r.view_url"
            :href="r.view_url"
            target="_blank"
            rel="noopener"
            class="magnet-page__link"
          >
            <i class="fa-solid fa-external-link-alt" aria-hidden="true"></i>详情
          </a>
          <AppButton type="button" variant="ghost" size="sm" @click="remove(r)" aria-label="取消收藏">
            <i class="fa-solid fa-star" aria-hidden="true"></i>已收藏
          </AppButton>
        </div>
      </li>
    </ul>
    <MagnetOfflineModal :open="offlineOpen" :magnet="offlineMagnet" :magnet-name="offlineName" @close="offlineOpen = false" />
  </div>
</template>

<style scoped>
/* favorites 页容器：跟随 hub panel 全宽 */
.magnet-page {
  max-width: 960px;
  margin: 0 auto;
  padding: 28px 20px calc(20px + env(safe-area-inset-bottom, 0px));
  box-sizing: border-box;
}
</style>
