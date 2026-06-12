import { ThemeConfig } from './widget';
export interface CaptchaOptions {
    apiBase?: string;
    theme?: Partial<ThemeConfig>;
    locale?: Record<string, string>;
    enableBiometrics?: boolean;
    onSuccess?: (token: string) => void;
    onError?: (error: string) => void;
    onExpire?: () => void;
}
export interface CaptchaInstance {
    getToken(): string | null;
    reset(): void;
    destroy(): void;
}
export declare function render(container: HTMLElement | string, options?: CaptchaOptions): CaptchaInstance;
export { BiometricsCollector } from './core/biometrics';
export { collectFingerprint } from './core/fingerprint';
export { solvePoW } from './core/pow-worker';
export { extractFeaturesFromRaw } from './core/features';
export { Transport } from './transport';
export { CaptchaWidget } from './widget';
export type { ThemeConfig, WidgetStatus } from './widget';
export type { Challenge, VerifyResult, VerifyPayload, WidgetConfig } from './transport';
export type { BehaviorPayload, BehaviorFeatures, Point } from './core/biometrics';
