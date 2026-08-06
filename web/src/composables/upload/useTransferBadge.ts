// 传输任务角标的极薄中转：useUploadTaskStore 是带依赖注入的工厂（非全局 store），
// 顶栏 AppHeader 无法直接取数，故由 FileBrowser 将任务计数喂到这里，AppHeader 读取。
import { ref } from "vue";

export type TransferBadgeKind = "" | "active" | "failed" | "success";

const count = ref(0);
const kind = ref<TransferBadgeKind>("");
const open = ref(false);

export function useTransferBadge() {
  function setBadge(c: number, k: TransferBadgeKind = "") {
    count.value = c > 0 ? c : 0;
    kind.value = c > 0 ? k : "";
  }
  function setOpen(v: boolean) {
    open.value = v;
  }
  return { count, kind, open, setBadge, setOpen };
}
