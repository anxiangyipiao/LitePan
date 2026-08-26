import { http } from "./client";

export interface MagnetFavorite {
  hash: string;
  name: string;
  size: number;
  seeders: number;
  leechers: number;
  date: number;
  category?: string;
  magnet: string;
  view_url?: string;
  created_at: number;
}

export type MagnetFavoriteInput = Omit<MagnetFavorite, "created_at">;

export function fetchMagnetFavorites() {
  return http.get<MagnetFavorite[]>("/magnet-favorites");
}

export function addMagnetFavorite(item: MagnetFavoriteInput) {
  return http.post<MagnetFavorite[]>("/magnet-favorites", item);
}

export function removeMagnetFavorite(hash: string) {
  return http.del<MagnetFavorite[]>(`/magnet-favorites/${encodeURIComponent(hash)}`);
}
