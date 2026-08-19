/**
 * 360°/180° VR 视频识别与 WebXR 能力探测。
 *
 * 文件名 token 匹配（大小写不敏感、要求词边界），避免普通文件名误判：
 * - 视野：180（半球）或 360（整球），`180p` 分辨率不误判；
 * - 立体：sbs（左右分屏，左半左眼/右半右眼）或 mono（单目）。
 *
 * 用例：
 *   true {180,sbs}: My_180_sbs.mp4 / VR180 left-right.mkv / 180sbs 全景.mp4
 *   true {360,sbs}: tour_360_sbs.mkv / 360VR 3dv.mp4 / pano 左右.mp4
 *   true {180,mono}: halfsphere_tour.mp4 / 180 panorama.mp4
 *   true {360,mono}: My_VR_Tour_360.mp4 / equirect_sample.webm / 全景视频.mp4
 *   null: Ep 360p 1080p.mp4 / 1080 HDR.mp4 / Breakfast_club.mkv / My360Video.mp4 / S01E01.mkv
 */

export type Video360Config = {
  fieldOfView: 180 | 360;
  stereo: "sbs" | "mono";
};

// 180° 标记：兼容 180 与 vr/360/sbs 拼接（VR180、180sbs…），排除 180p 分辨率
const FOV_180_TOKENS = String.raw`(?:vr180|180vr|180sbs|sbs180|halfsphere|half|180(?!p))`;
// 360°/VR 标记
const VR_360_TOKENS = String.raw`(?:vr360|360vr|panorama|pano|equirect|eq|vr|360(?!p)|全景)`;
// 左右立体标记
const SBS_TOKENS = String.raw`(?:180sbs|sbs180|sbs|左右|stereo|3dv|lr|立体)`;

const VR_RE = new RegExp(`(?<![a-z0-9])(?:${FOV_180_TOKENS}|${VR_360_TOKENS})(?![a-z0-9])`, "i");
const FOV_180_RE = new RegExp(`(?<![a-z0-9])${FOV_180_TOKENS}(?![a-z0-9])`, "i");
const SBS_RE = new RegExp(`(?<![a-z0-9])${SBS_TOKENS}(?![a-z0-9])`, "i");

export function detectVideo360(name: string): Video360Config | null {
  if (!name) return null;
  if (!VR_RE.test(name)) return null;
  return {
    fieldOfView: FOV_180_RE.test(name) ? 180 : 360,
    stereo: SBS_RE.test(name) ? "sbs" : "mono",
  };
}

export function is360File(name: string): boolean {
  return detectVideo360(name) !== null;
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
