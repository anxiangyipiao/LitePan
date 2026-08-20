import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import { router } from "./router";
import { initTheme } from "./utils/theme";

// 懒加载 chunk 加载失败自动刷新一次。
// 前端重建后哈希资源更名、旧哈希被清理，浏览器里仍开着旧页面（或缓存了旧 bundle）时，
// 懒加载 import() 会 404，导致页面部分区域空白（列表不渲染等）。
// 捕获此类错误后强制整页刷新，拉取最新 index.html 与资源自愈。
let chunkReloaded = false;
function handleChunkLoadError(reason: unknown) {
  if (chunkReloaded) return;
  const msg = reason instanceof Error ? reason.message : String(reason ?? "");
  const hit =
    msg.includes("Failed to fetch dynamically imported module") ||
    msg.includes("ChunkLoadError") ||
    msg.includes("Importing a module script failed") ||
    msg.includes("Loading chunk") ||
    msg.includes("dynamically imported");
  if (!hit) return;
  chunkReloaded = true;
  const url = new URL(location.href);
  url.searchParams.set("__reload", String(Date.now()));
  location.replace(url.toString());
}
window.addEventListener("unhandledrejection", (e) => handleChunkLoadError(e.reason));
window.addEventListener("error", (e) => handleChunkLoadError(e.message));

import "./assets/iconfont/iconfont.js";
import "./styles/tokens.css";
import "./styles/base.css";
import "./styles/buttons.css";
import "./styles/file-list.css";
import "./styles/file-toolbar.css";
import "./styles/dropdown-menu.css";
import "./styles/confirm-modal.css";
import "./styles/skins/brutal.css";

initTheme();

createApp(App).use(createPinia()).use(router).mount("#app");
