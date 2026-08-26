import { computed, ref } from "vue";
import {
  addMagnetFavorite,
  fetchMagnetFavorites,
  removeMagnetFavorite,
  type MagnetFavorite,
  type MagnetFavoriteInput,
} from "@/api/magnetFavorites";
import { getApiErrorMessage } from "@/api/client";
import { toast } from "@/composables/useToast";

const items = ref<MagnetFavorite[]>([]);
const loading = ref(false);
const loaded = ref(false);
let inflight: Promise<void> | null = null;

async function refresh(): Promise<void> {
  if (inflight) return inflight;
  loading.value = true;
  inflight = (async () => {
    try {
      items.value = (await fetchMagnetFavorites()) ?? [];
      loaded.value = true;
    } catch (e) {
      toast.error(getApiErrorMessage(e, "读取磁力收藏失败"));
    } finally {
      loading.value = false;
      inflight = null;
    }
  })();
  return inflight;
}

function isFavorited(hash: string): boolean {
  if (!hash) return false;
  const target = hash.toLowerCase();
  return items.value.some((it) => it.hash.toLowerCase() === target);
}

async function toggle(input: MagnetFavoriteInput): Promise<boolean> {
  if (!input?.hash) return false;
  if (isFavorited(input.hash)) {
    await unfavorite(input.hash);
    return false;
  }
  await favorite(input);
  return true;
}

async function favorite(input: MagnetFavoriteInput): Promise<boolean> {
  try {
    items.value = (await addMagnetFavorite(input)) ?? [];
    toast.success("已加入收藏");
    return true;
  } catch (e) {
    toast.error(getApiErrorMessage(e, "收藏失败"));
    return false;
  }
}

async function unfavorite(hash: string): Promise<boolean> {
  try {
    items.value = (await removeMagnetFavorite(hash)) ?? [];
    toast.success("已取消收藏");
    return true;
  } catch (e) {
    toast.error(getApiErrorMessage(e, "取消收藏失败"));
    return false;
  }
}

export function useMagnetFavorites() {
  return {
    items: computed(() => items.value),
    loading: computed(() => loading.value),
    loaded: computed(() => loaded.value),
    refresh,
    isFavorited,
    favorite,
    unfavorite,
    toggle,
  };
}
