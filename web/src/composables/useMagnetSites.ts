import { ref } from "vue";
import { http } from "@/api/client";

export interface MagnetSite {
  url: string;
  label: string;
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
  return { sites, loading, load };
}
