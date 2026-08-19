/**
 * 最小 WebXR 全局类型声明。
 * 项目的 tsconfig lib 未含 WebXR DOM 类型，而 three 的 WebXRManager 以全局 XRSession 为参数，
 * 这里补齐 360/VR 播放用到的部分即可（剩余 XR 类型由 @types/three 在 skipLibCheck 下容忍）。
 */

interface XRSessionInit {
  optionalFeatures?: string[];
  requiredFeatures?: string[];
}

interface XRSession {
  addEventListener(type: string, listener: (event: Event) => void): void;
  removeEventListener(type: string, listener: (event: Event) => void): void;
  end(): Promise<undefined>;
}

interface XRSystem {
  isSessionSupported(mode: string): Promise<boolean>;
  requestSession(mode: string, options?: XRSessionInit): Promise<XRSession>;
}

interface Navigator {
  xr?: XRSystem;
}
