export interface FingerprintComponents {
  canvas: string;
  webgl: string;
  audio: string;
  fonts: string[];
  screen: string;
  timezone: string;
  language: string;
  platform: string;
  hardwareConcurrency: number;
  deviceMemory: number;
  touchPoints: number;
  colorDepth: number;
  pixelRatio: number;
}

export async function collectFingerprint(): Promise<string> {
  const components = await gatherComponents();
  const data = JSON.stringify(components);
  return await sha256(data);
}

async function gatherComponents(): Promise<FingerprintComponents> {
  return {
    canvas: getCanvasFingerprint(),
    webgl: getWebGLFingerprint(),
    audio: await getAudioFingerprint(),
    fonts: detectFonts(),
    screen: `${screen.width}x${screen.height}`,
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    language: navigator.language,
    platform: navigator.platform,
    hardwareConcurrency: navigator.hardwareConcurrency || 0,
    deviceMemory: (navigator as any).deviceMemory || 0,
    touchPoints: navigator.maxTouchPoints || 0,
    colorDepth: screen.colorDepth,
    pixelRatio: window.devicePixelRatio || 1,
  };
}

function getCanvasFingerprint(): string {
  try {
    const canvas = document.createElement('canvas');
    canvas.width = 200;
    canvas.height = 50;
    const ctx = canvas.getContext('2d');
    if (!ctx) return '';
    ctx.textBaseline = 'top';
    ctx.font = '14px Arial';
    ctx.fillStyle = '#f60';
    ctx.fillRect(125, 1, 62, 20);
    ctx.fillStyle = '#069';
    ctx.fillText('ShieldCaptcha,😃', 2, 15);
    ctx.fillStyle = 'rgba(102, 204, 0, 0.7)';
    ctx.fillText('biometrics', 4, 35);
    return canvas.toDataURL();
  } catch {
    return '';
  }
}

function getWebGLFingerprint(): string {
  try {
    const canvas = document.createElement('canvas');
    const gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl');
    if (!gl) return '';
    const glCtx = gl as WebGLRenderingContext;
    const debugInfo = glCtx.getExtension('WEBGL_debug_renderer_info');
    const renderer = debugInfo ? glCtx.getParameter(debugInfo.UNMASKED_RENDERER_WEBGL) : '';
    const vendor = debugInfo ? glCtx.getParameter(debugInfo.UNMASKED_VENDOR_WEBGL) : '';
    return `${vendor}~${renderer}`;
  } catch {
    return '';
  }
}

async function getAudioFingerprint(): Promise<string> {
  try {
    const AudioCtx = (window as any).AudioContext || (window as any).webkitAudioContext;
    if (!AudioCtx) return '';
    const ctx = new AudioCtx();
    const oscillator = ctx.createOscillator();
    const analyser = ctx.createAnalyser();
    const gain = ctx.createGain();
    const scriptProcessor = ctx.createScriptProcessor(4096, 1, 1);

    gain.gain.value = 0;
    oscillator.type = 'triangle';
    oscillator.frequency.value = 10000;

    oscillator.connect(analyser);
    analyser.connect(scriptProcessor);
    scriptProcessor.connect(gain);
    gain.connect(ctx.destination);

    oscillator.start(0);

    const result = await new Promise<string>((resolve) => {
      scriptProcessor.onaudioprocess = (event: AudioProcessingEvent) => {
        const data = event.inputBuffer.getChannelData(0);
        let sum = 0;
        for (let i = 0; i < data.length; i++) {
          sum += Math.abs(data[i]);
        }
        resolve(sum.toString().slice(0, 20));
        oscillator.stop();
        ctx.close();
      };
      setTimeout(() => resolve('timeout'), 500);
    });

    return result;
  } catch {
    return '';
  }
}

function detectFonts(): string[] {
  const baseFonts = ['monospace', 'sans-serif', 'serif'];
  const testFonts = [
    'Arial', 'Courier New', 'Georgia', 'Helvetica', 'Times New Roman',
    'Verdana', 'Comic Sans MS', 'Impact', 'Lucida Console',
    'Palatino Linotype', 'Tahoma', 'Trebuchet MS',
  ];

  const testStr = 'mmmmmmmmmmlli';
  const testSize = '72px';
  const body = document.body;

  const span = document.createElement('span');
  span.style.fontSize = testSize;
  span.style.position = 'absolute';
  span.style.left = '-9999px';
  span.innerHTML = testStr;
  body.appendChild(span);

  const baseWidths: Record<string, number> = {};
  for (const base of baseFonts) {
    span.style.fontFamily = base;
    baseWidths[base] = span.offsetWidth;
  }

  const detected: string[] = [];
  for (const font of testFonts) {
    for (const base of baseFonts) {
      span.style.fontFamily = `'${font}', ${base}`;
      if (span.offsetWidth !== baseWidths[base]) {
        detected.push(font);
        break;
      }
    }
  }

  body.removeChild(span);
  return detected;
}

async function sha256(message: string): Promise<string> {
  if (window.crypto && window.crypto.subtle) {
    const encoder = new TextEncoder();
    const data = encoder.encode(message);
    const hashBuffer = await window.crypto.subtle.digest('SHA-256', data);
    const hashArray = Array.from(new Uint8Array(hashBuffer));
    return hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
  }
  return fallbackSHA256(message);
}

function fallbackSHA256(str: string): string {
  // Simplified fallback for environments without crypto.subtle
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    const chr = str.charCodeAt(i);
    hash = ((hash << 5) - hash) + chr;
    hash |= 0;
  }
  return Math.abs(hash).toString(16).padStart(64, '0');
}
