import { http } from "./client";

export interface DoubanMovie {
  id: string;
  title: string;
  original_title?: string;
  year?: string;
  rating?: string;
  ratings_count?: number;
  directors?: string[];
  casts?: string[];
  genres?: string[];
  durations?: string[];
  countries?: string[];
  languages?: string[];
  summary?: string;
  poster?: string;
  thumb?: string;
  imdb?: string;
  release_date?: string;
}

export interface DoubanSearchResult {
  movies: DoubanMovie[];
  total: number;
  start: number;
  count: number;
}

export interface DoubanHotItem {
  id: string;
  title: string;
  rating?: string;
  poster?: string;
  year?: string;
  regions?: string[];
  genres?: string[];
  directors?: string[];
  actors?: string[];
}

export interface DoubanHotResult {
  movies: DoubanHotItem[];
  title: string;
}

// 搜索豆瓣影视
export function searchDouban(query: string, start = 0, count = 20) {
  return http.get<DoubanSearchResult>("/douban/search", { q: query, start, count });
}

// 获取豆瓣热门影视
export function getDoubanHot(type: "movie" | "tv" = "movie", start = 0, count = 20) {
  return http.get<DoubanHotResult>("/douban/hot", { type, start, count });
}

// 获取豆瓣影视详情
export function getDoubanDetail(id: string) {
  return http.get<DoubanMovie>("/douban/detail", { id });
}

// 获取豆瓣正在热映
export function getDoubanNowPlaying(city = "北京") {
  return http.get<DoubanHotItem[]>("/douban/now-playing", { city });
}

// 获取豆瓣即将上映
export function getDoubanComingSoon(start = 0, count = 20) {
  return http.get<DoubanHotItem[]>("/douban/coming-soon", { start, count });
}

// 获取豆瓣 Top 250
export function getDoubanTop250(start = 0, count = 20) {
  return http.get<DoubanHotItem[]>("/douban/top250", { start, count });
}

// 获取豆瓣分类推荐
export function getDoubanRecommend(genre = "", start = 0, count = 20) {
  return http.get<DoubanHotItem[]>("/douban/recommend", { genre, start, count });
}
