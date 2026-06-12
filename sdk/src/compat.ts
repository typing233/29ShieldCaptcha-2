/**
 * Backward-compatibility shim for the legacy captcha.js API.
 * Drop this file in place of the old captcha.js for a seamless upgrade.
 */
import { render, CaptchaInstance } from './index';

let instance: CaptchaInstance | null = null;

function autoInit(): void {
  const container = document.getElementById('captchaWidget') || document.getElementById('captcha-widget');
  if (!container) return;

  const apiBase = (window as any).CAPTCHA_API_BASE || '';

  instance = render(container, {
    apiBase,
    enableBiometrics: true,
    onSuccess: (token) => {
      const event = new CustomEvent('captcha:success', { detail: { token } });
      document.dispatchEvent(event);
      const input = document.querySelector<HTMLInputElement>('input[name="captcha_token"]');
      if (input) input.value = token;
    },
    onError: (error) => {
      const event = new CustomEvent('captcha:error', { detail: { error } });
      document.dispatchEvent(event);
    },
  });
}

// Auto-init on DOM ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', autoInit);
} else {
  autoInit();
}

// Expose global API for legacy code
(window as any).ShieldCaptcha = {
  render,
  getInstance: () => instance,
  reset: () => instance?.reset(),
  getToken: () => instance?.getToken() ?? null,
};
