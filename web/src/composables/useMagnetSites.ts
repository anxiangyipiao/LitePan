// 共享的磁力镜像站点状态：拉取内置+自定义镜像清单、当前用户启用哪些。
// 三个入口（搜索页、顶栏 modal、TMDB 详情）共用同一份本地状态，避免重复请求。
import { computed, ref } from "vue";
import { http } from "@/api/client";
import { saveSettings } from "@/api/settings";

export interface MagnetSite {
  id: string;
  label: string;
  base_url: string;
  enabled: boolean;
}

const sites = ref<MagnetSite[]>([]);
const loading = ref(false);
let loadPromise: Promise<MagnetSite[]> | null = null;

const ENABLED_KEY = "magnet_search_enabled_sites";
// 同步一份到 localStorage，三个入口跨页签共用。settings 仍以 settings API 为准。

async function fetchSites(): Promise<MagnetSite[]> {
  loading.value = true;
  try {
    const res = (await http.get<MagnetSite[]>("/magnet-search/sites")) ?? [];
    sites.value = res;
    return res;
  } finally {
    loading.value = false;
  }
}

export function useMagnetSites() {
  function load(force = false): Promise<MagnetSite[]> {
    if (force) loadPromise = null;
    if (!loadPromise) loadPromise = fetchSites();
    return loadPromise;
  }

  const enabledIds = computed(() => sites.value.filter((s) => s.enabled).map((s) => s.id));

  // 把当前 enabled 状态同步到 settings（以 JSON 数组字符串形式存）。
  // 全启用时存空串（=兼容"全启用"语义），否则存数组 JSON。
  async function persistEnabled(): Promise<void> {
    const allEnabled = sites.value.length > 0 && sites.value.every((s) => s.enabled);
    const value = allEnabled ? "" : JSON.stringify(enabledIds.value);
    await saveSettings({ [ENABLED_KEY]: value });
  }

  function setEnabled(id: string, on: boolean) {
    const s = sites.value.find((x) => x.id === id);
    if (s) s.enabled = on;
  }

  function toggle(id: string) {
    const s = sites.value.find((x) => x.id === id);
    if (s) s.enabled = !s.enabled;
  }

  return { sites, loading, load, enabledIds, setEnabled, toggle, persistEnabled };
}
