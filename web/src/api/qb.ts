import { http } from "./client";

export function addToQB(magnet: string, savepath?: string, category?: string) {
  return http.post<{ ok: boolean }>("/qb/add", { magnet, savepath, category });
}

export function testQB(url?: string, username?: string, password?: string) {
  return http.post<{ ok: boolean }>("/qb/test", { url, username, password });
}
