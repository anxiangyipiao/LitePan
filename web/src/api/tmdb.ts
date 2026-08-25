import { http } from "./client";

export interface TmdbMedia {
  id: number;
  media_type?: "movie" | "tv";
  // Movie fields
  title?: string;
  original_title?: string;
  release_date?: string;
  // TV fields
  name?: string;
  original_name?: string;
  first_air_date?: string;
  // Common fields
  overview?: string;
  poster_path?: string;
  backdrop_path?: string;
  vote_average?: number;
  vote_count?: number;
  genre_ids?: number[];
  popularity?: number;
}

export interface TmdbSearchResult {
  page: number;
  results: TmdbMedia[];
  total_pages: number;
  total_results: number;
}

export interface TmdbGenre {
  id: number;
  name: string;
}

export interface TmdbProductionCompany {
  id: number;
  name: string;
  logo_path?: string;
  origin_country?: string;
}

export interface TmdbMovieDetails {
  id: number;
  title: string;
  original_title: string;
  overview: string;
  poster_path?: string;
  backdrop_path?: string;
  release_date: string;
  runtime?: number;
  budget?: number;
  revenue?: number;
  vote_average: number;
  vote_count: number;
  popularity: number;
  genres: TmdbGenre[];
  production_companies: TmdbProductionCompany[];
  imdb_id?: string;
  status: string;
  tagline?: string;
  homepage?: string;
  original_language: string;
  spoken_languages: { english_name: string; iso_639_1: string; name: string }[];
  production_countries: { iso_3166_1: string; name: string }[];
}

export interface TmdbTvDetails {
  id: number;
  name: string;
  original_name: string;
  overview: string;
  poster_path?: string;
  backdrop_path?: string;
  first_air_date: string;
  last_air_date?: string;
  episode_run_time: number[];
  number_of_episodes: number;
  number_of_seasons: number;
  vote_average: number;
  vote_count: number;
  popularity: number;
  genres: TmdbGenre[];
  production_companies: TmdbProductionCompany[];
  status: string;
  tagline?: string;
  homepage?: string;
  original_language: string;
  spoken_languages: { english_name: string; iso_639_1: string; name: string }[];
  production_countries: { iso_3166_1: string; name: string }[];
  networks: { id: number; name: string; logo_path?: string }[];
  created_by: { id: number; name: string; profile_path?: string }[];
  seasons: {
    id: number;
    name: string;
    season_number: number;
    episode_count: number;
    poster_path?: string;
    air_date?: string;
    overview?: string;
  }[];
}

export interface TmdbCredits {
  id: number;
  cast: {
    id: number;
    name: string;
    character: string;
    profile_path?: string;
    order: number;
  }[];
  crew: {
    id: number;
    name: string;
    job: string;
    department: string;
    profile_path?: string;
  }[];
}

export interface TmdbImage {
  file_path: string;
  width: number;
  height: number;
  vote_average: number;
  vote_count: number;
}

export interface TmdbImages {
  id: number;
  backdrops: TmdbImage[];
  posters: TmdbImage[];
  logos: TmdbImage[];
}

// 搜索影视
export function searchTmdb(query: string, page = 1) {
  return http.get<TmdbSearchResult>("/tmdb/search", { q: query, page });
}

// 获取热门电影
export function getTmdbPopular(type: "movie" | "tv" = "movie", page = 1) {
  return http.get<TmdbSearchResult>("/tmdb/popular", { type, page });
}

// 获取高分电影
export function getTmdbTopRated(type: "movie" | "tv" = "movie", page = 1) {
  return http.get<TmdbSearchResult>("/tmdb/top-rated", { type, page });
}

// 获取正在热映
export function getTmdbNowPlaying(page = 1) {
  return http.get<TmdbSearchResult>("/tmdb/now-playing", { page });
}

// 获取即将上映
export function getTmdbUpcoming(page = 1) {
  return http.get<TmdbSearchResult>("/tmdb/upcoming", { page });
}

// 获取电影详情
export function getTmdbMovieDetails(id: number) {
  return http.get<TmdbMovieDetails>("/tmdb/movie", { id });
}

// 获取剧集详情
export function getTmdbTvDetails(id: number) {
  return http.get<TmdbTvDetails>("/tmdb/tv", { id });
}

// 获取影视详情（自动判断类型）
export function getTmdbDetails(id: number, mediaType: "movie" | "tv" = "movie") {
  if (mediaType === "tv") {
    return getTmdbTvDetails(id);
  }
  return getTmdbMovieDetails(id);
}

// 获取演员列表
export function getTmdbCredits(id: number, mediaType: "movie" | "tv" = "movie") {
  return http.get<TmdbCredits>(`/tmdb/credits`, { id, type: mediaType });
}

// 获取图片列表
export function getTmdbImages(id: number, mediaType: "movie" | "tv" = "movie") {
  return http.get<TmdbImages>(`/tmdb/images`, { id, type: mediaType });
}

// 获取分类列表
export function getTmdbGenres(mediaType: "movie" | "tv" = "movie") {
  return http.get<{ genres: TmdbGenre[] }>("/tmdb/genres", { type: mediaType });
}

// 发现/推荐
export function discoverTmdb(params: {
  mediaType?: "movie" | "tv";
  sort_by?: string;
  with_genres?: string;
  page?: number;
}) {
  return http.get<TmdbSearchResult>("/tmdb/discover", {
    type: params.mediaType || "movie",
    sort_by: params.sort_by || "popularity.desc",
    with_genres: params.with_genres || "",
    page: params.page || 1,
  });
}

// 图片 URL 工具 - 使用后端代理（支持代理环境）
export const tmdbImage = {
  poster: (path?: string, size = "w500") =>
    path ? `/api/tmdb/image?s=${encodeURIComponent(size)}&p=${encodeURIComponent(path)}` : "",
  backdrop: (path?: string, size = "w780") =>
    path ? `/api/tmdb/image?s=${encodeURIComponent(size)}&p=${encodeURIComponent(path)}` : "",
  profile: (path?: string, size = "w185") =>
    path ? `/api/tmdb/image?s=${encodeURIComponent(size)}&p=${encodeURIComponent(path)}` : "",
  original: (path?: string) =>
    path ? `/api/tmdb/image?s=original&p=${encodeURIComponent(path)}` : "",
};
