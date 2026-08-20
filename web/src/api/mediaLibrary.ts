import { http } from "./client";

export interface MediaLibraryRoot {
  id: string;
  name: string;
  path: string;
}

export interface MediaLibraryItem {
  id: string;
  title: string;
  year?: number;
  media_type: "movie" | "tv";
  status: "ok" | "miss" | "doubt";
  tmdb_id?: string;
  poster_url?: string;
  folder_name?: string;
  file_count: number;
  ep_local?: number;
  ep_tmdb?: number;
  ep_scraped?: number;
  tv_state?: string;
  lib_id: string;
  play_url?: string;
}

export interface MediaLibraryItemsResult {
  items: MediaLibraryItem[];
  total: number;
  has_more: boolean;
}

export interface MediaLibraryEpisode {
  season?: number;
  episode: number;
  title?: string;
  play_url: string;
}

export interface MediaLibraryDisc {
  label: string;
  play_url: string;
}

export interface MediaLibraryDetail {
  id: string;
  title: string;
  year?: number;
  media_type: "movie" | "tv";
  tmdb_id?: string;
  folder_name?: string;
  file_count: number;
  ep_local?: number;
  ep_tmdb?: number;
  ep_scraped?: number;
  tv_state?: string;
  status: "ok" | "miss" | "doubt";
  poster_url?: string;
  backdrop_url?: string;
  overview?: string;
  genres?: string[];
  runtime?: string;
  studio?: string;
  director?: string;
  actors?: string[];
  extra_fanart_urls?: string[];
  play_url?: string;
  episodes?: MediaLibraryEpisode[];
  discs?: MediaLibraryDisc[];
}

export type MediaLibrarySort = "title_asc" | "year_desc" | "year_asc" | "added_desc" | "added_asc";

export interface MediaLibraryQuery {
  lib?: string;
  type?: "movie" | "tv" | "";
  sort?: MediaLibrarySort;
  keyword?: string;
  limit?: number;
  offset?: number;
  genre?: string;
  actor?: string;
}

export interface MediaLibraryFacets {
  genres: string[];
  actors: string[];
}

export const mediaLibraryApi = {
  roots: () => http.get<MediaLibraryRoot[]>("/media-library/roots"),

  facets: (lib?: string) =>
    http.get<MediaLibraryFacets>("/media-library/facets", {
      ...(lib ? { lib } : {}),
    }),

  detail: (lib: string, id: string) =>
    http.get<MediaLibraryDetail>("/media-library/detail", { lib, id }),

  items: (q: MediaLibraryQuery = {}) =>
    http.get<MediaLibraryItemsResult>("/media-library/items", {
      ...(q.lib ? { lib: q.lib } : {}),
      ...(q.type ? { media_type: q.type } : {}),
      ...(q.sort ? { sort: q.sort } : {}),
      ...(q.keyword ? { keyword: q.keyword } : {}),
      ...(q.genre ? { genre: q.genre } : {}),
      ...(q.actor ? { actor: q.actor } : {}),
      ...(q.limit != null ? { limit: String(q.limit) } : {}),
      ...(q.offset != null ? { offset: String(q.offset) } : {}),
    }),

  refresh: (q: MediaLibraryQuery = {}) =>
    http.post<MediaLibraryItemsResult>("/media-library/refresh", null, {
      ...(q.lib ? { lib: q.lib } : {}),
      ...(q.type ? { media_type: q.type } : {}),
      ...(q.sort ? { sort: q.sort } : {}),
      ...(q.keyword ? { keyword: q.keyword } : {}),
      ...(q.genre ? { genre: q.genre } : {}),
      ...(q.actor ? { actor: q.actor } : {}),
    }),

  saveRoots: (roots: MediaLibraryRoot[]) => http.put<MediaLibraryRoot[]>("/admin/media-library/roots", roots),
};
