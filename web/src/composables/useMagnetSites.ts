// 共享的磁力镜像站点状态：拉取内置+自定义镜像清单，跨页签共享当前选中的 site。
// 三个入口（搜索页、顶栏弹窗、TMDB 详情）共用同一份本地状态。
import { computed, ref } from "vue";
import { http } from "@/api/client";

export interface MagnetSite {
  id: string;
  label: string;
  base_url: string;
  enabled: boolean;
}

const sites = ref<MagnetSite[]>([]);
const loading = ref(false);
let loadPromise: Promise<MagnetSite[]> | null = null;

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

  // 已启用的镜像（按内置 + 自定义顺序）
  const enabledSites = computed(() => sites.value.filter((s) => s.enabled));

  function labelOf(id: string): string {
    const s = sites.value.find((x) => x.id === id);
    return s ? s.label : id;
  }

  return { sites, enabledSites, loading, load, labelOf };
}
