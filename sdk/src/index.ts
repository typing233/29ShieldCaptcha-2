import { BiometricsCollector, BehaviorPayload } from './core/biometrics';
import { collectFingerprint } from './core/fingerprint';
import { solvePoW } from './core/pow-worker';
import { Transport, Challenge, VerifyResult, WidgetConfig } from './transport';
import { CaptchaWidget, ThemeConfig } from './widget';

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

export function render(container: HTMLElement | string, options: CaptchaOptions = {}): CaptchaInstance {
  const el: HTMLElement = typeof container === 'string'
    ? (document.querySelector<HTMLElement>(container) ?? (() => { throw new Error('ShieldCaptcha: container not found'); })())
    : container;

  const apiBase = options.apiBase || (window as any).CAPTCHA_API_BASE || '';
  const transport = new Transport(apiBase);
  const biometrics = new BiometricsCollector();
  const enableBiometrics = options.enableBiometrics !== false;

  let token: string | null = null;
  let currentChallenge: Challenge | null = null;

  const widget = new CaptchaWidget(el, options.theme, {
    onReady: () => {
      if (enableBiometrics) biometrics.start(el);
      loadRemoteConfig();
      prefetchChallenge();
    },
    onDragEnd: async () => {
      await runVerification();
    },
  }, options.locale);

  async function loadRemoteConfig() {
    try {
      const config: WidgetConfig = await transport.fetchWidgetConfig();
      let themeToApply = config.theme;

      // Apply experiment: if active, assign user to group A or B based on traffic_pct
      if (config.experiment) {
        const hash = simpleHash(await collectFingerprint());
        const bucket = hash % 100;
        const inExperiment = bucket < config.experiment.traffic_pct;
        if (inExperiment) {
          // Group B gets experimental config, Group A gets config_a
          const expConfig = bucket < (config.experiment.traffic_pct / 2)
            ? config.experiment.config_a
            : config.experiment.config_b;
          if (expConfig && typeof expConfig === 'object') {
            themeToApply = { ...(themeToApply || {}), ...expConfig } as WidgetConfig['theme'];
          }
        }
      }

      if (themeToApply) {
        widget.applyTheme(themeToApply as Partial<ThemeConfig>);
      }
    } catch {
      // Non-critical: continue with local/default theme
    }
  }

  function simpleHash(str: string): number {
    let hash = 0;
    for (let i = 0; i < str.length; i++) {
      hash = ((hash << 5) - hash) + str.charCodeAt(i);
      hash |= 0;
    }
    return Math.abs(hash);
  }

  async function prefetchChallenge() {
    try {
      currentChallenge = await transport.fetchChallenge();
    } catch (err) {
      console.warn('[ShieldCaptcha] Failed to prefetch challenge:', err);
    }
  }

  async function runVerification() {
    try {
      widget.setStatus('solving');

      if (!currentChallenge) {
        currentChallenge = await transport.fetchChallenge();
      }

      const [fingerprint, powResult] = await Promise.all([
        collectFingerprint(),
        solvePoW(currentChallenge.id, currentChallenge.nonce, currentChallenge.difficulty, (iters) => {
          widget.setStatus('solving', `计算中... ${Math.floor(iters / 1000)}k`);
        }),
      ]);

      widget.setStatus('verifying');

      const interaction = widget.getInteractionData();
      let behavior: BehaviorPayload | undefined;
      if (enableBiometrics) {
        biometrics.stop();
        behavior = biometrics.getPayload();
      }

      const result: VerifyResult = await transport.submitVerification({
        challenge: currentChallenge,
        solution: powResult.solution,
        fingerprint,
        interaction,
        behavior,
      });

      if (result.success) {
        token = result.token || currentChallenge.id;
        widget.setStatus('success');
        options.onSuccess?.(token);
      } else {
        widget.setStatus('error', result.error || '验证失败');
        options.onError?.(result.error || 'verification_failed');
        currentChallenge = null;
        if (enableBiometrics) {
          biometrics.reset();
          biometrics.start(el);
        }
        prefetchChallenge();
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : '网络错误';
      widget.setStatus('error', message);
      options.onError?.(message);
      currentChallenge = null;
      prefetchChallenge();
    }
  }

  return {
    getToken: () => token,
    reset: () => {
      token = null;
      currentChallenge = null;
      widget.reset();
      if (enableBiometrics) {
        biometrics.reset();
        biometrics.start(el);
      }
      prefetchChallenge();
    },
    destroy: () => {
      biometrics.stop();
      widget.destroy();
    },
  };
}

export { BiometricsCollector } from './core/biometrics';
export { collectFingerprint } from './core/fingerprint';
export { solvePoW } from './core/pow-worker';
export { extractFeaturesFromRaw } from './core/features';
export { Transport } from './transport';
export { CaptchaWidget } from './widget';
export type { ThemeConfig, WidgetStatus } from './widget';
export type { Challenge, VerifyResult, VerifyPayload, WidgetConfig } from './transport';
export type { BehaviorPayload, BehaviorFeatures, Point } from './core/biometrics';
