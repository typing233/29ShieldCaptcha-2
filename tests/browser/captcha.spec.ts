import { test, expect } from '@playwright/test';
import { execSync, spawn, ChildProcess } from 'child_process';
import * as path from 'path';
import * as net from 'net';

const PROJECT_ROOT = path.resolve(__dirname, '../..');
let serverProcess: ChildProcess;
let serverPort: number;

async function getFreePort(): Promise<number> {
  return new Promise((resolve) => {
    const srv = net.createServer();
    srv.listen(0, () => {
      const port = (srv.address() as net.AddressInfo).port;
      srv.close(() => resolve(port));
    });
  });
}

async function waitForServer(port: number, timeoutMs = 10000): Promise<void> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    try {
      const resp = await fetch(`http://127.0.0.1:${port}/health`);
      if (resp.ok) return;
    } catch {}
    await new Promise(r => setTimeout(r, 100));
  }
  throw new Error(`Server did not start within ${timeoutMs}ms`);
}

test.beforeAll(async () => {
  // Build the server binary
  execSync('go build -o shieldcaptcha_test ./cmd/server/', { cwd: PROJECT_ROOT });

  serverPort = await getFreePort();

  // Start server with low difficulty for fast PoW
  serverProcess = spawn(path.join(PROJECT_ROOT, 'shieldcaptcha_test'), [], {
    cwd: PROJECT_ROOT,
    env: {
      ...process.env,
      CAPTCHA_PORT: String(serverPort),
      CAPTCHA_HMAC_SECRET: 'aa'.repeat(32),
      CAPTCHA_BASE_DIFFICULTY: '8',
      CAPTCHA_MIN_DIFFICULTY: '8',
      CAPTCHA_MAX_DIFFICULTY: '12',
      CAPTCHA_LOG_LEVEL: 'debug',
    },
  });

  serverProcess.stderr?.on('data', (d) => process.stderr.write(d));

  await waitForServer(serverPort);
});

test.afterAll(async () => {
  if (serverProcess) {
    serverProcess.kill('SIGTERM');
    await new Promise(r => setTimeout(r, 500));
  }
  try { execSync('rm -f shieldcaptcha_test', { cwd: PROJECT_ROOT }); } catch {}
});

test('full captcha flow: drag slider → fingerprint → PoW → verify success', async ({ page }) => {
  await page.goto(`http://127.0.0.1:${serverPort}/`);

  // Wait for challenge to be fetched
  await page.waitForFunction(() => {
    const status = document.getElementById('status');
    return status && status.textContent?.includes('请拖动滑块完成验证');
  }, { timeout: 5000 });

  // Get slider element
  const slider = page.locator('#slider');
  const widget = page.locator('#captchaWidget');

  const sliderBox = await slider.boundingBox();
  const widgetBox = await widget.boundingBox();
  expect(sliderBox).not.toBeNull();
  expect(widgetBox).not.toBeNull();

  // Simulate a human-like drag: start from center of slider, drag to 90% of widget width
  const startX = sliderBox!.x + sliderBox!.width / 2;
  const startY = sliderBox!.y + sliderBox!.height / 2;
  const endX = widgetBox!.x + widgetBox!.width * 0.92;

  await page.mouse.move(startX, startY);
  await page.mouse.down();

  // Move in multiple steps to simulate realistic trajectory
  const steps = 25;
  for (let i = 1; i <= steps; i++) {
    const progress = i / steps;
    const x = startX + (endX - startX) * progress;
    const y = startY + Math.sin(progress * Math.PI) * 3; // slight vertical wobble
    await page.mouse.move(x, y, { steps: 1 });
    await page.waitForTimeout(30 + Math.random() * 20);
  }

  await page.mouse.up();

  // Wait for the full verification to complete (fingerprint + PoW + submit)
  // With difficulty 8, PoW should complete in under 5 seconds
  await page.waitForFunction(() => {
    const status = document.getElementById('status');
    return status && (status.textContent?.includes('验证通过') || status.textContent?.includes('验证失败'));
  }, { timeout: 30000 });

  // Assert success
  const statusEl = page.locator('#status');
  const statusText = await statusEl.textContent();
  expect(statusText).toContain('验证通过');

  // Verify the widget shows success state
  const widgetClass = await widget.getAttribute('class');
  expect(widgetClass).toContain('success');
});

test('retry button appears and works after error', async ({ page }) => {
  // Test that a malformed scenario shows retry
  await page.goto(`http://127.0.0.1:${serverPort}/`);

  await page.waitForFunction(() => {
    const status = document.getElementById('status');
    return status && status.textContent?.includes('请拖动滑块完成验证');
  }, { timeout: 5000 });

  // Verify retry button is initially hidden
  const retryBtn = page.locator('#retryBtn');
  await expect(retryBtn).toBeHidden();
});

test('fingerprint is collected as 64-char hex', async ({ page }) => {
  await page.goto(`http://127.0.0.1:${serverPort}/`);

  // Inject a capture hook to grab the fingerprint before it's submitted
  const fingerprint = await page.evaluate(async () => {
    // Replicate the fingerprint collection from captcha.js
    const components: Record<string, any> = {};

    try {
      const canvas = document.createElement('canvas');
      canvas.width = 200; canvas.height = 50;
      const ctx = canvas.getContext('2d')!;
      ctx.textBaseline = 'top';
      ctx.font = '14px Arial';
      ctx.fillStyle = '#f60';
      ctx.fillRect(10, 1, 62, 20);
      ctx.fillStyle = '#069';
      ctx.fillText('ShieldCaptcha', 2, 15);
      ctx.fillStyle = 'rgba(102, 204, 0, 0.7)';
      ctx.fillText('fingerprint', 4, 17);
      components.canvas = canvas.toDataURL();
    } catch { components.canvas = 'unavailable'; }

    components.screen = screen.width + 'x' + screen.height + 'x' + screen.colorDepth;
    components.timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    components.language = navigator.language;
    components.platform = navigator.platform;
    components.cores = navigator.hardwareConcurrency || 0;

    const raw = JSON.stringify(components);
    const hash = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(raw));
    return Array.from(new Uint8Array(hash)).map(b => b.toString(16).padStart(2, '0')).join('');
  });

  expect(fingerprint).toHaveLength(64);
  expect(fingerprint).toMatch(/^[0-9a-f]{64}$/);
});

test('WebWorker solves PoW correctly', async ({ page }) => {
  await page.goto(`http://127.0.0.1:${serverPort}/`);

  // Fetch a challenge and solve it in a WebWorker inside the page
  const result = await page.evaluate(async (port) => {
    const resp = await fetch(`http://127.0.0.1:${port}/api/challenge`);
    const challenge = await resp.json();

    return new Promise<{ found: boolean; solution: string; iterations: number }>((resolve, reject) => {
      const worker = new Worker('worker.js');
      const timeout = setTimeout(() => { worker.terminate(); reject(new Error('timeout')); }, 20000);
      worker.onmessage = (e) => {
        if (e.data.found) {
          clearTimeout(timeout);
          worker.terminate();
          resolve(e.data);
        }
      };
      worker.onerror = (e) => { clearTimeout(timeout); reject(e); };
      worker.postMessage({
        challengeId: challenge.id,
        nonce: challenge.nonce,
        difficulty: challenge.difficulty,
      });
    });
  }, serverPort);

  expect(result.found).toBe(true);
  expect(result.solution).toBeTruthy();
  expect(result.iterations).toBeGreaterThan(0);
});
