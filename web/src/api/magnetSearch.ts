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
  source?: string; // 多站聚合时标注来源镜像 ID（sukebei / nyaa / sukebei_cn / custom:*）
}

export function searchMagnet(q: string, limit = 20) {
  return http.get<MagnetSearchResult[]>("/magnet-search", { q, limit });
}
