import { http } from "./client";

export interface MagnetSearchResult {
  id: number;
  name: string;
  category?: string;
  size: number;
  date: number;
  seeders: number;
  leechers: number;
  downloads: number;
  hash: string;
  magnet: string;
  view_url: string;
  source?: string; // 单站模式时填当前选中的镜像 ID
}

export function searchMagnet(q: string, site: string, limit = 20) {
  return http.get<MagnetSearchResult[]>("/magnet-search", { q, site, limit });
}
