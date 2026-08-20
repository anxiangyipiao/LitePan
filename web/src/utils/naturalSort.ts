// 预编译 zh-CN 排序 collator：避免每次比较都做 locale 解析。
// sensitivity "accent" 与调用方原语义一致（toLowerCase().localeCompare(...,"zh-CN")）：
// 忽略大小写差异，保留基础字母与音调差异，按拼音排序。
const zhCollator = new Intl.Collator("zh-CN", { sensitivity: "accent" });

// 字符串 → token 拆分结果缓存：一次排序中每个名字只拆一次，
// 避免 O(N log N) 次比较里反复跑正则。设上限防无界增长。
const tokenCache = new Map<string, string[]>();
const tokenCacheLimit = 100_000;

function splitTokens(s: string): string[] {
  let tokens = tokenCache.get(s);
  if (tokens) return tokens;
  tokens = String(s).toLowerCase().match(/(\d+|\D+)/g) || [];
  if (tokenCache.size >= tokenCacheLimit) tokenCache.clear();
  tokenCache.set(s, tokens);
  return tokens;
}

export function naturalSort(a: string, b: string): number {
  const splitA = splitTokens(a);
  const splitB = splitTokens(b);
  const maxLength = Math.max(splitA.length, splitB.length);

  for (let i = 0; i < maxLength; i++) {
    const partA = splitA[i] || "";
    const partB = splitB[i] || "";

    if (/^\d+$/.test(partA) && /^\d+$/.test(partB)) {
      const numA = parseInt(partA, 10);
      const numB = parseInt(partB, 10);
      if (numA !== numB) return numA - numB;
    } else {
      const comparison = zhCollator.compare(partA, partB);
      if (comparison !== 0) return comparison;
    }
  }

  return 0;
}
