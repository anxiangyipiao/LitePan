/**
 * 360°/VR 视频识别与 WebXR 能力探测。
 *
 * 文件名 token 匹配（大小写不敏感、要求词边界），避免普通文件名误判：
 * - 真命中：My_VR_Tour_360.mp4 / VR360 panorama.mkv / 360VR scene.mp4
 *           / equirect_sample.webm / pano test.mp4 / 全景视频.mp4
 * - 不命中：Ep 360p 1080p.mp4（分辨率）/ Breakfast_club.mkv / S01E01.mkv
 *           / My360Video.mp4（token 未用分隔符）
 */

const TOKENS = String.raw`(?:vr360|360vr|panorama|pano|equirect|eq|vr|360(?!p)|全景)`;
const RE = new RegExp(`(?<![a-z0-9])${TOKENS}(?![a-z0-9])`, "i");

export function is360File(name: string): boolean {
  if (!name) return false;
  return RE.test(name);
}

export function isVrCapable(): boolean {
  return (
    typeof navigator !== "undefined" &&
    !!navigator.xr &&
    typeof navigator.xr.isSessionSupported === "function"
  );
}

export async function isVrSupported(): Promise<boolean> {
  if (!isVrCapable()) return false;
  try {
    return await navigator.xr!.isSessionSupported("immersive-vr");
  } catch {
    return false;
  }
}
