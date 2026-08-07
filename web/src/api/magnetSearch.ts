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
}

export function searchMagnet(q: string, limit = 20) {
  return http.get<MagnetSearchResult[]>("/magnet-search", { q, limit });
}
