<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from "vue";
import * as THREE from "three";
import { isVrSupported } from "@/utils/video360";

const props = defineProps<{
  /** VideoPreview 持有的同一个 <video>，three.js 以 VideoTexture 直接采样其解码帧 */
  videoEl: HTMLVideoElement | null;
}>();

const emit = defineEmits<{
  notice: [message: string];
}>();

const canvasRef = ref<HTMLCanvasElement | null>(null);
const vrCapable = ref(false);
const presenting = ref(false);

let renderer: THREE.WebGLRenderer | null = null;
let scene: THREE.Scene | null = null;
let camera: THREE.PerspectiveCamera | null = null;
let geometry: THREE.SphereGeometry | null = null;
let material: THREE.MeshBasicMaterial | null = null;
let texture: THREE.VideoTexture | null = null;
let session: XRSession | null = null;
let ro: ResizeObserver | null = null;

// 视角状态
let yaw = 0;
let pitch = 0;
const SENS = 0.0035;
const MIN_FOV = 30;
const MAX_FOV = 110;
let fov = 80;

// 拖拽 / 捏合
let dragId = -1;
let dragX = 0;
let dragY = 0;
let yaw0 = 0;
let pitch0 = 0;
let dragMoved = 0;
const pointers = new Map<number, { x: number; y: number }>();
let pinchDist = 0;

function clamp(v: number, lo: number, hi: number) {
  return Math.min(hi, Math.max(lo, v));
}

function resize() {
  const canvas = canvasRef.value;
  if (!renderer || !camera || !canvas) return;
  const w = canvas.clientWidth || 1;
  const h = canvas.clientHeight || 1;
  camera.aspect = w / h;
  camera.updateProjectionMatrix();
  renderer.setSize(w, h, false);
}

function render() {
  if (!renderer || !camera || !scene || !texture) return;
  if (!renderer.xr.isPresenting) {
    camera.rotation.set(pitch, yaw, 0, "YXZ");
    camera.fov = fov;
    camera.updateProjectionMatrix();
  }
  texture.needsUpdate = true;
  renderer.render(scene, camera);
}

function teardown() {
  if (session) {
    session.removeEventListener?.("end", onSessionEnd);
    session.end?.().catch(() => undefined);
    session = null;
  }
  ro?.disconnect();
  ro = null;
  window.removeEventListener("fullscreenchange", resize);
  if (renderer) {
    renderer.setAnimationLoop(null);
    renderer.xr.enabled = false;
    renderer.dispose();
    // 避免连续切集时耗尽浏览器 WebGL 上下文配额
    renderer.forceContextLoss?.();
    renderer = null;
  }
  texture?.dispose();
  texture = null;
  geometry?.dispose();
  geometry = null;
  material?.dispose();
  material = null;
  scene = null;
  camera = null;
}

function init() {
  const video = props.videoEl;
  const canvas = canvasRef.value;
  if (!video || !canvas) return;
  teardown();

  renderer = new THREE.WebGLRenderer({ canvas, antialias: false, powerPreference: "high-performance" });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
  scene = new THREE.Scene();
  camera = new THREE.PerspectiveCamera(fov, 1, 0.1, 100);
  camera.rotation.order = "YXZ";
  geometry = new THREE.SphereGeometry(32, 128, 64);
  texture = new THREE.VideoTexture(video);
  texture.colorSpace = THREE.SRGBColorSpace;
  material = new THREE.MeshBasicMaterial({ map: texture, side: THREE.BackSide });
  scene.add(new THREE.Mesh(geometry, material));
  renderer.xr.enabled = true;
  resize();
  ro = new ResizeObserver(resize);
  ro.observe(canvas);
  window.addEventListener("fullscreenchange", resize);
  renderer.setAnimationLoop(render);
}

function onPointerDown(e: PointerEvent) {
  const canvas = canvasRef.value;
  if (!canvas) return;
  canvas.setPointerCapture(e.pointerId);
  pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
  if (pointers.size === 2) {
    const [a, b] = [...pointers.values()];
    pinchDist = Math.hypot(a.x - b.x, a.y - b.y);
    dragId = -1;
    return;
  }
  dragId = e.pointerId;
  dragX = e.clientX;
  dragY = e.clientY;
  yaw0 = yaw;
  pitch0 = pitch;
  dragMoved = 0;
}

function onPointerMove(e: PointerEvent) {
  if (presenting.value) return;
  const canvas = canvasRef.value;
  if (!canvas) return;
  const prev = pointers.get(e.pointerId);
  pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
  if (pointers.size === 2) {
    const [a, b] = [...pointers.values()];
    const dist = Math.hypot(a.x - b.x, a.y - b.y);
    if (pinchDist > 0) {
      fov = clamp(fov * (pinchDist / dist), MIN_FOV, MAX_FOV);
    }
    pinchDist = dist;
    dragId = -1;
    return;
  }
  if (e.pointerId !== dragId) return;
  dragMoved += Math.abs(e.clientX - (prev?.x ?? e.clientX)) + Math.abs(e.clientY - (prev?.y ?? e.clientY));
  const factor = fov / 80;
  yaw = yaw0 + (e.clientX - dragX) * SENS * factor;
  pitch = clamp(pitch0 + (e.clientY - dragY) * SENS * factor, -1.45, 1.45);
}

function onPointerUp(e: PointerEvent) {
  pointers.delete(e.pointerId);
  if (e.pointerId === dragId) {
    dragId = -1;
    // 位移很小视为点击：切换播放/暂停（media-chrome 手势层不响应 canvas 目标）
    if (dragMoved < 10) {
      const video = props.videoEl;
      if (video) {
        if (video.paused) void video.play().catch(() => undefined);
        else video.pause();
      }
    }
  }
}

function onWheel(e: WheelEvent) {
  e.preventDefault();
  fov = clamp(fov + e.deltaY * 0.05, MIN_FOV, MAX_FOV);
}

function onSessionEnd() {
  presenting.value = false;
  session = null;
  resize();
}

async function checkVr() {
  vrCapable.value = await isVrSupported();
}

async function enterVr() {
  if (!navigator.xr || !renderer) return;
  try {
    session = await navigator.xr.requestSession("immersive-vr", { optionalFeatures: ["local-floor"] });
    await renderer.xr.setSession(session);
    session.addEventListener("end", onSessionEnd);
    presenting.value = true;
  } catch (error) {
    emit("notice", error instanceof Error ? `进入 VR 失败：${error.message}` : "进入 VR 失败");
  }
}

function exitVr() {
  void session?.end();
}

watch(
  () => props.videoEl,
  () => init(),
);

onMounted(() => {
  void checkVr();
  init();
});

onUnmounted(teardown);
</script>

<template>
  <div class="video-player-360" slot="centered-chrome" noautohide>
    <canvas
      ref="canvasRef"
      class="video-player-360__canvas"
      @pointerdown="onPointerDown"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp"
      @pointercancel="onPointerUp"
      @wheel.prevent="onWheel"
    ></canvas>
    <button
      v-if="vrCapable && !presenting"
      type="button"
      class="video-player-360__vr"
      @click="enterVr"
    >
      <i class="fa-solid fa-vr-cardboard" aria-hidden="true"></i>
      <span>进入 VR</span>
    </button>
    <button
      v-else-if="presenting"
      type="button"
      class="video-player-360__vr"
      @click="exitVr"
    >
      <i class="fa-solid fa-vr-cardboard" aria-hidden="true"></i>
      <span>退出 VR</span>
    </button>
  </div>
</template>

<style scoped>
.video-player-360 {
  position: absolute;
  inset: 0;
  z-index: 5;
  pointer-events: none;
}

.video-player-360__canvas {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  display: block;
  touch-action: none;
  cursor: grab;
  pointer-events: auto;
}

.video-player-360__canvas:active {
  cursor: grabbing;
}

.video-player-360__vr {
  position: absolute;
  top: 88px;
  right: 16px;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 8px 14px;
  border: 1px solid rgba(255, 255, 255, 0.18);
  border-radius: 999px;
  background: rgba(2, 7, 17, 0.55);
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  line-height: 1;
  cursor: pointer;
  pointer-events: auto;
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

.video-player-360__vr:hover {
  background: rgba(22, 135, 255, 0.75);
  border-color: transparent;
}

@media (max-width: 768px) {
  .video-player-360__vr {
    top: 74px;
    right: 12px;
    padding: 7px 12px;
    font-size: 12px;
  }
}
</style>
