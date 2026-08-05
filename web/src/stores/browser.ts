import { defineStore } from "pinia";
import { computed, ref } from "vue";
import { filesApi } from "@/api/files";
import { publicApi } from "@/api/public";
import { getApiErrorMessage } from "@/api/client";
import type { BrowserFavoriteItem, BrowserFavoritesState, FileItem } from "@/api/types";
import { toast } from "@/composables/useToast";
import { useAccountsStore } from "./accounts";

export interface Crumb {
  id: string;
  name: string;
}

const ROOT: Crumb = { id: "", name: "根目录" };

function cloneCrumbs(crumbs: Crumb[]) {
  return crumbs.map((item) => ({ id: item.id, name: item.name }));
}

export interface LoadFilesOptions {
  silent?: boolean;
  forceRefresh?: boolean;
}

export const useBrowserStore = defineStore("browser", () => {
  const accountsStore = useAccountsStore();
  const currentAccountId = ref<number | null>(null);
  const breadcrumb = ref<Crumb[]>([ROOT]);
  const files = ref<FileItem[]>([]);
  const filesResortTick = ref(0);
  const loading = ref(false);
  const refreshing = ref(false);
  const error = ref("");
  const responseTime = ref("-");
  const cacheRate = ref("-");
  const favorites = ref<BrowserFavoriteItem[]>([]);
  const favoritesOpen = ref(false);
  // 会话内是否已加载过一次全局收藏夹：首次加载恢复历史开合状态
  let favoritesLoadedOnce = false;
  let loadFilesSeq = 0;
  let refreshFilesSeq = 0;
  let favoritesSaveChain = Promise.resolve();

  const accounts = computed(() => accountsStore.accounts.filter((a) => a.is_active));
  const currentAccount = computed(
    () => accounts.value.find((a) => a.id === currentAccountId.value) ?? null,
  );
  const currentParentId = computed(() => breadcrumb.value[breadcrumb.value.length - 1]?.id ?? "");

  function setCurrentAccount(accountId: number | null) {
    if (currentAccountId.value === accountId) return;
    currentAccountId.value = accountId;
  }

  function enqueueFavoritesSave(payload: Parameters<typeof filesApi.saveFavorites>[0]) {
    const request = favoritesSaveChain.then(() => filesApi.saveFavorites(payload));
    favoritesSaveChain = request.then(() => undefined, () => undefined);
    return request;
  }

  // 全局收藏夹：启动时加载一次，切换存储盘不再重新加载（各账号共享同一份列表）
  async function loadFavorites(opts?: { silent?: boolean }) {
    try {
      const data = await filesApi.getFavorites();
      favorites.value = data.items;
      // 首次加载恢复历史开合状态；切换存储盘时保留当前状态
      if (!favoritesLoadedOnce) {
        favoritesOpen.value = data.open;
        favoritesLoadedOnce = true;
      }
    } catch (e) {
      if (!opts?.silent) {
        toast.error(getApiErrorMessage(e, "收藏夹加载失败"));
      }
    }
  }

  async function persistFavoritesState(nextState: BrowserFavoritesState, opts?: { successMessage?: string; silent?: boolean }) {
    const prevItems = favorites.value;
    const prevOpen = favoritesOpen.value;
    favorites.value = nextState.items;
    favoritesOpen.value = nextState.open;
    try {
      const saved = await enqueueFavoritesSave({
        open: nextState.open,
        items: nextState.items,
      });
      favorites.value = saved.items;
      favoritesOpen.value = saved.open;
      if (opts?.successMessage) {
        toast.success(opts.successMessage);
      }
      return true;
    } catch (e) {
      favorites.value = prevItems;
      favoritesOpen.value = prevOpen;
      if (!opts?.silent) {
        toast.error(getApiErrorMessage(e, "收藏夹保存失败"));
      }
      return false;
    }
  }

  async function setFavoritesOpen(open: boolean) {
    await persistFavoritesState({
      open,
      items: favorites.value,
    }, { silent: true });
  }

  async function toggleFavoritesOpen() {
    await setFavoritesOpen(!favoritesOpen.value);
  }

  // 全局收藏：收藏项跨账号，用 账号+文件夹ID 作为唯一键
  function favoriteKey(item: Pick<BrowserFavoriteItem, "id" | "account_id">) {
    return `${item.account_id ?? 0}:${item.id}`;
  }

  async function addCurrentDirectoryFavorite(customName?: string) {
    if (currentParentId.value === "") {
      toast.info("根目录无需加入收藏夹");
      return;
    }
    const accountId = currentAccountId.value;
    if (accountId == null) return;
    if (
      favorites.value.some(
        (item) => item.account_id === accountId && item.id === currentParentId.value,
      )
    ) {
      toast.info("当前文件夹已在收藏夹中");
      return;
    }
    const lastCrumb = breadcrumb.value[breadcrumb.value.length - 1];
    const nextName = (customName ?? lastCrumb?.name ?? "当前目录").trim();
    if (!nextName) {
      toast.info("收藏名不能为空");
      return;
    }
    await persistFavoritesState({
      open: true,
      items: [
        ...favorites.value,
        {
          id: currentParentId.value,
          name: nextName,
          account_id: accountId,
          crumbs: breadcrumb.value.map((item) => ({ id: item.id, name: item.name })),
        },
      ],
    }, { successMessage: "已加入收藏夹" });
  }

  async function removeFavorite(item: BrowserFavoriteItem) {
    const key = favoriteKey(item);
    await persistFavoritesState({
      open: favoritesOpen.value,
      items: favorites.value.filter((it) => favoriteKey(it) !== key),
    });
  }

  async function moveFavorite(item: BrowserFavoriteItem, direction: -1 | 1) {
    const key = favoriteKey(item);
    const idx = favorites.value.findIndex((it) => favoriteKey(it) === key);
    if (idx < 0) return;
    const next = idx + direction;
    if (next < 0 || next >= favorites.value.length) return;
    const items = [...favorites.value];
    [items[idx], items[next]] = [items[next], items[idx]];
    await persistFavoritesState({
      open: favoritesOpen.value,
      items,
    }, { silent: true });
  }

  async function renameFavorite(item: BrowserFavoriteItem, name: string) {
    const nextName = name.trim();
    if (!nextName) {
      toast.info("收藏名不能为空");
      return;
    }
    const key = favoriteKey(item);
    await persistFavoritesState({
      open: favoritesOpen.value,
      items: favorites.value.map((it) =>
        favoriteKey(it) === key ? { ...it, name: nextName } : it,
      ),
    }, { successMessage: "收藏名已更新" });
  }

  async function openFavorite(favorite: BrowserFavoriteItem) {
    if (favorite.account_id == null) return;
    await openDirectory(favorite.account_id, favorite.crumbs, { silent: true });
  }

  async function ensureValidCurrentAccount() {
    if (!accounts.value.length) {
      setCurrentAccount(null);
      breadcrumb.value = [ROOT];
      files.value = [];
      return false;
    }
    const current = accounts.value.find((a) => a.id === currentAccountId.value);
    if (currentAccountId.value !== null && current) {
      return false;
    }
    const next = accounts.value.find((a) => a.is_default) ?? accounts.value[0];
    setCurrentAccount(next.id);
    breadcrumb.value = [ROOT];
    return true;
  }

  function primeLocation(accountId: number, crumbs: Crumb[]) {
    setCurrentAccount(accountId);
    breadcrumb.value = crumbs.length ? cloneCrumbs(crumbs) : [ROOT];
  }

  async function loadAccounts(opts?: { reconcile?: boolean }) {
    await accountsStore.loadAccounts();
    if (!accounts.value.length) {
      setCurrentAccount(null);
      breadcrumb.value = [ROOT];
      files.value = [];
      return;
    }
    if (opts?.reconcile) {
      const switched = await ensureValidCurrentAccount();
      if (switched) {
        await loadFiles();
      }
    }
  }

  async function resetToDefaultAccount() {
    const active = accounts.value;
    if (!active.length) {
      setCurrentAccount(null);
      breadcrumb.value = [ROOT];
      files.value = [];
      return;
    }
    const def = active.find((a) => a.is_default) ?? active[0];
    await selectAccount(def.id);
  }

  async function selectAccount(id: number) {
    if (!accounts.value.some((a) => a.id === id)) {
      await ensureValidCurrentAccount();
      if (currentAccountId.value !== null) {
        await loadFiles();
      }
      return;
    }
    setCurrentAccount(id);
    breadcrumb.value = [ROOT];
    await loadFiles();
  }

  async function openDirectory(
    accountId: number,
    crumbs: Crumb[],
    opts?: LoadFilesOptions,
  ) {
    if (!accounts.value.some((a) => a.id === accountId)) {
      await ensureValidCurrentAccount();
      if (currentAccountId.value === null) return;
    } else {
      setCurrentAccount(accountId);
    }
    breadcrumb.value = crumbs.length ? cloneCrumbs(crumbs) : [ROOT];
    await loadFiles(opts);
  }

  async function fetchCacheHitRate() {
    try {
      const data = await publicApi.cacheHitRate();
      cacheRate.value = `${data.hit_rate}%`;
    } catch {
      cacheRate.value = "-";
    }
  }

  async function loadFiles(opts?: LoadFilesOptions) {
    const accountId = currentAccountId.value;
    if (accountId === null) return;
    const parentId = currentParentId.value;
    const requestSeq = ++loadFilesSeq;
    const silent = opts?.silent ?? false;
    if (!silent) loading.value = true;
    error.value = "";
    const started = performance.now();
    try {
      const res = await filesApi.list(accountId, parentId, {
        forceRefresh: opts?.forceRefresh,
      });
      if (isStaleFileListRequest(requestSeq, accountId, parentId)) return;
      files.value = res.items;
      filesResortTick.value += 1;
      responseTime.value = `${Math.round(performance.now() - started)}ms`;
      void fetchCacheHitRate();
    } catch (e) {
      if (isStaleFileListRequest(requestSeq, accountId, parentId)) return;
      if (!silent) files.value = [];
      error.value = getApiErrorMessage(e, "加载失败");
      responseTime.value = "-";
      if (!silent) toast.error(error.value);
    } finally {
      if (!silent && requestSeq === loadFilesSeq) loading.value = false;
    }
  }

  function removeFilesLocally(ids: string[]) {
    const drop = new Set(ids.map(String));
    files.value = files.value.filter((f) => !drop.has(String(f.id || f.name)));
  }

  function renameFileLocally(fileId: string, newName: string) {
    const id = String(fileId);
    files.value = files.value.map((f) =>
      String(f.id || f.name) === id ? { ...f, name: newName } : f,
    );
  }

  function addFolderLocally(folder: FileItem) {
    if (files.value.some((f) => f.name.toLowerCase() === folder.name.toLowerCase())) return;
    files.value = [folder, ...files.value];
  }

  async function refreshFiles() {
    const accountId = currentAccountId.value;
    if (accountId === null) {
      toast.info("请先选择一个账号");
      return;
    }
    const parentId = currentParentId.value;
    const requestSeq = ++refreshFilesSeq;
    refreshing.value = true;
    error.value = "";
    const started = performance.now();
    try {
      const res = await filesApi.list(accountId, parentId, {
        forceRefresh: true,
      });
      if (isStaleRefreshRequest(requestSeq, accountId, parentId)) return;
      files.value = res.items;
      filesResortTick.value += 1;
      responseTime.value = `${Math.round(performance.now() - started)}ms`;
      void fetchCacheHitRate();
      toast.success(`强制刷新成功，获取到 ${res.items.length} 个项目`);
    } catch (e) {
      if (isStaleRefreshRequest(requestSeq, accountId, parentId)) return;
      error.value = getApiErrorMessage(e, "刷新失败");
      responseTime.value = "-";
      toast.error(error.value);
    } finally {
      if (requestSeq === refreshFilesSeq) refreshing.value = false;
    }
  }

  function isCurrentLocation(accountId: number, parentId: string) {
    return currentAccountId.value === accountId && currentParentId.value === parentId;
  }

  function isStaleFileListRequest(requestSeq: number, accountId: number, parentId: string) {
    return requestSeq !== loadFilesSeq || !isCurrentLocation(accountId, parentId);
  }

  function isStaleRefreshRequest(requestSeq: number, accountId: number, parentId: string) {
    return requestSeq !== refreshFilesSeq || !isCurrentLocation(accountId, parentId);
  }

  async function enterFolder(folder: FileItem) {
    breadcrumb.value.push({ id: folder.id, name: folder.name });
    await loadFiles();
  }

  async function goTo(index: number) {
    if (index >= breadcrumb.value.length - 1) return;
    breadcrumb.value = breadcrumb.value.slice(0, index + 1);
    await loadFiles();
  }

  function downloadURL(file: FileItem): string {
    return filesApi.downloadURL(currentAccountId.value as number, file.id);
  }

  return {
    accounts,
    currentAccountId,
    currentAccount,
    breadcrumb,
    favorites,
    favoritesOpen,
    files,
    filesResortTick,
    loading,
    refreshing,
    error,
    responseTime,
    cacheRate,
    currentParentId,
    loadAccounts,
    loadFavorites,
    primeLocation,
    resetToDefaultAccount,
    selectAccount,
    openDirectory,
    loadFiles,
    refreshFiles,
    removeFilesLocally,
    renameFileLocally,
    addFolderLocally,
    setFavoritesOpen,
    toggleFavoritesOpen,
    addCurrentDirectoryFavorite,
    removeFavorite,
    moveFavorite,
    renameFavorite,
    openFavorite,
    enterFolder,
    goTo,
    downloadURL,
  };
});
