import { filesApi } from "@/api/files";

// 缩略图 blob 缓存：同一 (账号, 文件, 修改时间) 只请求一次，跨目录往返复用，
// 避免每次进入目录都重新走下载网关解析直链。mod_time 参与 key，文件更新后自动失效。
// 并发去重：同 key 多个组件同时挂载只发一个请求。

const MAX_ENTRIES = 200;

interface ThumbEntry {
  url?: string;
  promise?: Promise<string>;
}

const cache = new Map<string, ThumbEntry>();

export interface ThumbKey {
  accountId: number;
  fileId: string;
  modTime?: string | null;
  /** 仅用于请求构造，不参与 key（同一文件内容不变） */
  fileName?: string;
}

function thumbKey(k: ThumbKey): string {
  return `${k.accountId}:${k.fileId}:${k.modTime ?? ""}`;
}

/** 同步命中返回已缓存的 blob URL；未命中返回 null。 */
export function getThumbURL(k: ThumbKey): string | null {
  const entry = cache.get(thumbKey(k));
  return entry && entry.url ? entry.url : null;
}

/** 取缩略图 blob URL：命中直接返回，未命中发起请求并缓存（并发去重）。 */
export function loadThumbURL(k: ThumbKey): Promise<string> {
  const key = thumbKey(k);
  const existing = cache.get(key);
  if (existing?.promise) return existing.promise;
  if (existing?.url) return Promise.resolve(existing.url);

  const promise = fetch(filesApi.previewURL(k.accountId, k.fileId, k.fileName), {
    credentials: "include",
  })
    .then(async (res) => {
      if (!res.ok) throw new Error(`preview ${res.status}`);
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      cache.set(key, { url });
      trim();
      return url;
    })
    .finally(() => {
      // 清除 promise 引用，后续命中直接返回已缓存 url。
      const entry = cache.get(key);
      if (entry && entry.promise) entry.promise = undefined;
    });

  cache.set(key, { promise });
  trim();
  return promise;
}

// 淘汰最旧条目；不主动 revoke 以免打断仍在渲染中的 <img>，内存上限 = MAX_ENTRIES。
function trim() {
  while (cache.size > MAX_ENTRIES) {
    const oldest = cache.keys().next().value;
    if (oldest === undefined) break;
    cache.delete(oldest);
  }
}
