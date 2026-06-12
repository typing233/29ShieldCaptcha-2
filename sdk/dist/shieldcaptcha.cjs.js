'use strict';

const MAX_BUFFER_SIZE = 2000;
const PAUSE_THRESHOLD_MS = 100;
class BiometricsCollector {
    constructor() {
        this.points = [];
        this.collecting = false;
        this.element = null;
        this.onMouseMove = (e) => {
            this.addPoint(e.clientX, e.clientY, 'move');
        };
        this.onMouseDown = (e) => {
            this.addPoint(e.clientX, e.clientY, 'down');
        };
        this.onMouseUp = (e) => {
            this.addPoint(e.clientX, e.clientY, 'up');
        };
        this.onTouchMove = (e) => {
            var _a;
            const touch = e.touches[0];
            if (touch) {
                this.addPoint(touch.clientX, touch.clientY, 'touch', (_a = touch.force) !== null && _a !== void 0 ? _a : 0);
            }
        };
        this.onTouchStart = (e) => {
            var _a;
            const touch = e.touches[0];
            if (touch) {
                this.addPoint(touch.clientX, touch.clientY, 'down', (_a = touch.force) !== null && _a !== void 0 ? _a : 0);
            }
        };
        this.onTouchEnd = (e) => {
            const touch = e.changedTouches[0];
            if (touch) {
                this.addPoint(touch.clientX, touch.clientY, 'up');
            }
        };
    }
    start(element) {
        this.element = element;
        this.collecting = true;
        this.points = [];
        this.bindEvents();
    }
    stop() {
        this.collecting = false;
        this.unbindEvents();
    }
    reset() {
        this.points = [];
    }
    getPayload() {
        const trajectory = this.points.map(p => [p.x, p.y]);
        const timestamps = this.points.map(p => p.t);
        const pressures = this.points.map(p => { var _a; return (_a = p.pressure) !== null && _a !== void 0 ? _a : 0; });
        const features = this.extractFeatures();
        return { trajectory, timestamps, pressures, features };
    }
    extractFeatures() {
        const pts = this.points;
        if (pts.length < 2) {
            return this.emptyFeatures();
        }
        const velocities = [];
        const accelerations = [];
        const angles = [];
        let totalPath = 0;
        let directionChanges = 0;
        const pauseDurations = [];
        for (let i = 1; i < pts.length; i++) {
            const dx = pts[i].x - pts[i - 1].x;
            const dy = pts[i].y - pts[i - 1].y;
            const dt = (pts[i].t - pts[i - 1].t) / 1000;
            const dist = Math.sqrt(dx * dx + dy * dy);
            totalPath += dist;
            if (dt > 0) {
                velocities.push(dist / dt);
            }
            if (dt * 1000 > PAUSE_THRESHOLD_MS && dist < 2) {
                pauseDurations.push(dt * 1000);
            }
            if (i >= 2) {
                const prevDx = pts[i - 1].x - pts[i - 2].x;
                const prevDy = pts[i - 1].y - pts[i - 2].y;
                const angle = Math.atan2(dy, dx) - Math.atan2(prevDy, prevDx);
                angles.push(angle);
                const prevAngle = angles.length > 1 ? angles[angles.length - 2] : 0;
                if (Math.sign(angle) !== Math.sign(prevAngle) && Math.abs(angle) > 0.1) {
                    directionChanges++;
                }
            }
        }
        for (let i = 1; i < velocities.length; i++) {
            const dt = (pts[i + 1].t - pts[i].t) / 1000;
            if (dt > 0) {
                accelerations.push((velocities[i] - velocities[i - 1]) / dt);
            }
        }
        const avgVelocity = velocities.length > 0
            ? velocities.reduce((a, b) => a + b, 0) / velocities.length : 0;
        const maxVelocity = velocities.length > 0 ? Math.max(...velocities) : 0;
        const velocityVariance = this.variance(velocities);
        const avgAcceleration = accelerations.length > 0
            ? accelerations.reduce((a, b) => a + Math.abs(b), 0) / accelerations.length : 0;
        // Jerk smoothness (lower = more robotic)
        let jerkSum = 0;
        for (let i = 1; i < accelerations.length; i++) {
            jerkSum += Math.abs(accelerations[i] - accelerations[i - 1]);
        }
        const jerkSmoothness = accelerations.length > 1 ? jerkSum / (accelerations.length - 1) : 0;
        // Curvature (average angular change)
        const curvature = angles.length > 0
            ? angles.reduce((a, b) => a + Math.abs(b), 0) / angles.length : 0;
        // Straightness ratio
        const startPt = pts[0];
        const endPt = pts[pts.length - 1];
        const displacement = Math.sqrt(Math.pow(endPt.x - startPt.x, 2) + Math.pow(endPt.y - startPt.y, 2));
        const straightness = totalPath > 0 ? displacement / totalPath : 1;
        return {
            avgVelocity,
            maxVelocity,
            velocityVariance,
            avgAcceleration,
            jerkSmoothness,
            curvature,
            pauseCount: pauseDurations.length,
            pauseDurations,
            straightness,
            directionChanges,
            totalPathLength: totalPath,
            displacement,
        };
    }
    addPoint(x, y, eventType, pressure) {
        if (!this.collecting)
            return;
        if (this.points.length >= MAX_BUFFER_SIZE) {
            this.points.shift();
        }
        this.points.push({ x, y, t: Date.now(), pressure, eventType });
    }
    bindEvents() {
        var _a;
        const el = (_a = this.element) !== null && _a !== void 0 ? _a : document;
        el.addEventListener('mousemove', this.onMouseMove);
        el.addEventListener('mousedown', this.onMouseDown);
        el.addEventListener('mouseup', this.onMouseUp);
        el.addEventListener('touchmove', this.onTouchMove, { passive: true });
        el.addEventListener('touchstart', this.onTouchStart, { passive: true });
        el.addEventListener('touchend', this.onTouchEnd);
    }
    unbindEvents() {
        var _a;
        const el = (_a = this.element) !== null && _a !== void 0 ? _a : document;
        el.removeEventListener('mousemove', this.onMouseMove);
        el.removeEventListener('mousedown', this.onMouseDown);
        el.removeEventListener('mouseup', this.onMouseUp);
        el.removeEventListener('touchmove', this.onTouchMove);
        el.removeEventListener('touchstart', this.onTouchStart);
        el.removeEventListener('touchend', this.onTouchEnd);
    }
    variance(arr) {
        if (arr.length === 0)
            return 0;
        const mean = arr.reduce((a, b) => a + b, 0) / arr.length;
        return arr.reduce((sum, v) => sum + Math.pow(v - mean, 2), 0) / arr.length;
    }
    emptyFeatures() {
        return {
            avgVelocity: 0, maxVelocity: 0, velocityVariance: 0,
            avgAcceleration: 0, jerkSmoothness: 0, curvature: 0,
            pauseCount: 0, pauseDurations: [], straightness: 1,
            directionChanges: 0, totalPathLength: 0, displacement: 0,
        };
    }
}

async function collectFingerprint() {
    const components = await gatherComponents();
    const data = JSON.stringify(components);
    return await sha256(data);
}
async function gatherComponents() {
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
        deviceMemory: navigator.deviceMemory || 0,
        touchPoints: navigator.maxTouchPoints || 0,
        colorDepth: screen.colorDepth,
        pixelRatio: window.devicePixelRatio || 1,
    };
}
function getCanvasFingerprint() {
    try {
        const canvas = document.createElement('canvas');
        canvas.width = 200;
        canvas.height = 50;
        const ctx = canvas.getContext('2d');
        if (!ctx)
            return '';
        ctx.textBaseline = 'top';
        ctx.font = '14px Arial';
        ctx.fillStyle = '#f60';
        ctx.fillRect(125, 1, 62, 20);
        ctx.fillStyle = '#069';
        ctx.fillText('ShieldCaptcha,😃', 2, 15);
        ctx.fillStyle = 'rgba(102, 204, 0, 0.7)';
        ctx.fillText('biometrics', 4, 35);
        return canvas.toDataURL();
    }
    catch (_a) {
        return '';
    }
}
function getWebGLFingerprint() {
    try {
        const canvas = document.createElement('canvas');
        const gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl');
        if (!gl)
            return '';
        const glCtx = gl;
        const debugInfo = glCtx.getExtension('WEBGL_debug_renderer_info');
        const renderer = debugInfo ? glCtx.getParameter(debugInfo.UNMASKED_RENDERER_WEBGL) : '';
        const vendor = debugInfo ? glCtx.getParameter(debugInfo.UNMASKED_VENDOR_WEBGL) : '';
        return `${vendor}~${renderer}`;
    }
    catch (_a) {
        return '';
    }
}
async function getAudioFingerprint() {
    try {
        const AudioCtx = window.AudioContext || window.webkitAudioContext;
        if (!AudioCtx)
            return '';
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
        const result = await new Promise((resolve) => {
            scriptProcessor.onaudioprocess = (event) => {
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
    }
    catch (_a) {
        return '';
    }
}
function detectFonts() {
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
    const baseWidths = {};
    for (const base of baseFonts) {
        span.style.fontFamily = base;
        baseWidths[base] = span.offsetWidth;
    }
    const detected = [];
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
async function sha256(message) {
    if (window.crypto && window.crypto.subtle) {
        const encoder = new TextEncoder();
        const data = encoder.encode(message);
        const hashBuffer = await window.crypto.subtle.digest('SHA-256', data);
        const hashArray = Array.from(new Uint8Array(hashBuffer));
        return hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
    }
    return fallbackSHA256(message);
}
function fallbackSHA256(str) {
    // Simplified fallback for environments without crypto.subtle
    let hash = 0;
    for (let i = 0; i < str.length; i++) {
        const chr = str.charCodeAt(i);
        hash = ((hash << 5) - hash) + chr;
        hash |= 0;
    }
    return Math.abs(hash).toString(16).padStart(64, '0');
}

const WORKER_CODE = `
'use strict';

function sha256(data) {
  var h0=0x6a09e667,h1=0xbb67ae85,h2=0x3c6ef372,h3=0xa54ff53a;
  var h4=0x510e527f,h5=0x9b05688c,h6=0x1f83d9ab,h7=0x5be0cd19;
  var k=[0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,
    0x923f82a4,0xab1c5ed5,0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,
    0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,0xe49b69c1,0xefbe4786,
    0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,
    0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,
    0x06ca6351,0x14292967,0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,
    0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,0xa2bfe8a1,0xa81a664b,
    0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,
    0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,
    0x5b9cca4f,0x682e6ff3,0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,
    0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2];
  var bytes=[];
  for(var i=0;i<data.length;i++){
    bytes.push(data.charCodeAt(i));
  }
  bytes.push(0x80);
  while((bytes.length%64)!==56) bytes.push(0);
  var bitLen=data.length*8;
  for(var i=7;i>=0;i--) bytes.push((bitLen>>>(i*8))&0xff);
  for(var chunk=0;chunk<bytes.length;chunk+=64){
    var w=[];
    for(var i=0;i<16;i++){
      w[i]=(bytes[chunk+i*4]<<24)|(bytes[chunk+i*4+1]<<16)|
            (bytes[chunk+i*4+2]<<8)|bytes[chunk+i*4+3];
    }
    for(var i=16;i<64;i++){
      var s0=rr(w[i-15],7)^rr(w[i-15],18)^(w[i-15]>>>3);
      var s1=rr(w[i-2],17)^rr(w[i-2],19)^(w[i-2]>>>10);
      w[i]=(w[i-16]+s0+w[i-7]+s1)|0;
    }
    var a=h0,b=h1,c=h2,d=h3,e=h4,f=h5,g=h6,hh=h7;
    for(var i=0;i<64;i++){
      var S1=rr(e,6)^rr(e,11)^rr(e,25);
      var ch=(e&f)^((~e)&g);
      var t1=(hh+S1+ch+k[i]+w[i])|0;
      var S0=rr(a,2)^rr(a,13)^rr(a,22);
      var maj=(a&b)^(a&c)^(b&c);
      var t2=(S0+maj)|0;
      hh=g;g=f;f=e;e=(d+t1)|0;d=c;c=b;b=a;a=(t1+t2)|0;
    }
    h0=(h0+a)|0;h1=(h1+b)|0;h2=(h2+c)|0;h3=(h3+d)|0;
    h4=(h4+e)|0;h5=(h5+f)|0;h6=(h6+g)|0;h7=(h7+hh)|0;
  }
  function rr(n,b){return(n>>>b)|(n<<(32-b));}
  function hex(n){var s='';for(var i=7;i>=0;i--)s+=((n>>>(i*4))&0xf).toString(16);return s;}
  return hex(h0)+hex(h1)+hex(h2)+hex(h3)+hex(h4)+hex(h5)+hex(h6)+hex(h7);
}

function hasLeadingZeroBits(hashHex, bits) {
  var fullNibbles = Math.floor(bits / 4);
  var remainBits = bits % 4;
  for (var i = 0; i < fullNibbles; i++) {
    if (hashHex[i] !== '0') return false;
  }
  if (remainBits > 0) {
    var nibble = parseInt(hashHex[fullNibbles], 16);
    var mask = (0xF << (4 - remainBits)) & 0xF;
    if ((nibble & mask) !== 0) return false;
  }
  return true;
}

self.onmessage = function(e) {
  var id = e.data.id;
  var nonce = e.data.nonce;
  var difficulty = e.data.difficulty;
  var batchSize = 10000;

  for (var i = 0; i < 100000000; i++) {
    var solution = i.toString(16);
    var input = id + ':' + nonce + ':' + solution;
    var hash = sha256(input);
    if (hasLeadingZeroBits(hash, difficulty)) {
      self.postMessage({ type: 'solved', solution: solution, iterations: i });
      return;
    }
    if (i % batchSize === 0 && i > 0) {
      self.postMessage({ type: 'progress', iterations: i });
    }
  }
  self.postMessage({ type: 'failed' });
};
`;
function solvePoW(id, nonce, difficulty, onProgress) {
    return new Promise((resolve, reject) => {
        const blob = new Blob([WORKER_CODE], { type: 'application/javascript' });
        const url = URL.createObjectURL(blob);
        const worker = new Worker(url);
        worker.onmessage = (e) => {
            const msg = e.data;
            if (msg.type === 'solved') {
                worker.terminate();
                URL.revokeObjectURL(url);
                resolve({ solution: msg.solution, iterations: msg.iterations });
            }
            else if (msg.type === 'progress') {
                onProgress === null || onProgress === void 0 ? void 0 : onProgress(msg.iterations);
            }
            else if (msg.type === 'failed') {
                worker.terminate();
                URL.revokeObjectURL(url);
                reject(new Error('PoW solving failed: max iterations exceeded'));
            }
        };
        worker.onerror = (err) => {
            worker.terminate();
            URL.revokeObjectURL(url);
            reject(err);
        };
        worker.postMessage({ id, nonce, difficulty });
    });
}

function serializeBehavior(behavior) {
    const result = {
        trajectory: behavior.trajectory,
        timestamps: behavior.timestamps,
        pressures: behavior.pressures,
    };
    if (behavior.features) {
        result.features = {
            avg_velocity: behavior.features.avgVelocity,
            max_velocity: behavior.features.maxVelocity,
            velocity_variance: behavior.features.velocityVariance,
            avg_acceleration: behavior.features.avgAcceleration,
            jerk_smoothness: behavior.features.jerkSmoothness,
            curvature: behavior.features.curvature,
            pause_count: behavior.features.pauseCount,
            pause_durations: behavior.features.pauseDurations,
            straightness: behavior.features.straightness,
            direction_changes: behavior.features.directionChanges,
            total_path_length: behavior.features.totalPathLength,
            displacement: behavior.features.displacement,
        };
    }
    return result;
}
class Transport {
    constructor(apiBase) {
        this.apiBase = apiBase.replace(/\/$/, '');
    }
    async fetchChallenge() {
        const resp = await fetch(`${this.apiBase}/api/challenge`, {
            method: 'GET',
            headers: { 'Content-Type': 'application/json' },
        });
        if (!resp.ok) {
            throw new Error(`Challenge fetch failed: ${resp.status}`);
        }
        return resp.json();
    }
    async fetchWidgetConfig() {
        const resp = await fetch(`${this.apiBase}/api/config`, {
            method: 'GET',
            headers: { 'Content-Type': 'application/json' },
        });
        if (!resp.ok) {
            return {};
        }
        return resp.json();
    }
    async submitVerification(payload) {
        const body = {
            challenge: payload.challenge,
            solution: payload.solution,
            fingerprint: payload.fingerprint,
            interaction: payload.interaction,
        };
        if (payload.behavior) {
            body.behavior = serializeBehavior(payload.behavior);
        }
        const resp = await fetch(`${this.apiBase}/api/verify`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        if (!resp.ok && resp.status !== 200) {
            throw new Error(`Verification request failed: ${resp.status}`);
        }
        return resp.json();
    }
}

const DEFAULT_THEME = {
    primaryColor: '#1890ff',
    successColor: '#52c41a',
    errorColor: '#ff4d4f',
    bgColor: '#ffffff',
    textColor: '#333333',
    borderRadius: 8,
    width: 380,
    height: 48,
    sliderShape: 'round',
};
class CaptchaWidget {
    constructor(container, theme = {}, callbacks = {}, locale) {
        this.status = 'idle';
        this.dragTrajectory = [];
        this.dragStartTime = 0;
        this.dragEndTime = 0;
        this.isDragging = false;
        this.thumbX = 0;
        this.container = container;
        this.theme = { ...DEFAULT_THEME, ...theme };
        this.callbacks = callbacks;
        this.locale = locale || {
            idle: '请向右滑动验证',
            solving: '正在计算中...',
            verifying: '验证中...',
            success: '验证通过',
            error: '验证失败，请重试',
        };
        this.render();
    }
    getInteractionData() {
        return {
            type: 'drag',
            start_time: this.dragStartTime,
            end_time: this.dragEndTime,
            trajectory: this.dragTrajectory,
        };
    }
    getTrajectory() {
        return this.dragTrajectory;
    }
    setStatus(status, message) {
        this.status = status;
        this.label.textContent = message || this.locale[status] || '';
        this.root.className = 'sc-widget sc-' + status;
        if (status === 'success') {
            this.fill.style.width = '100%';
            this.fill.style.backgroundColor = this.theme.successColor;
        }
        else if (status === 'error') {
            this.fill.style.backgroundColor = this.theme.errorColor;
            setTimeout(() => this.reset(), 2000);
        }
    }
    applyTheme(partial) {
        const changed = Object.keys(partial).some(k => partial[k] !== this.theme[k]);
        if (!changed)
            return;
        this.theme = { ...this.theme, ...partial };
        this.container.innerHTML = '';
        this.render();
    }
    reset() {
        this.thumbX = 0;
        this.thumb.style.left = '0px';
        this.fill.style.width = '0px';
        this.fill.style.backgroundColor = this.theme.primaryColor;
        this.dragTrajectory = [];
        this.setStatus('ready');
    }
    destroy() {
        this.container.innerHTML = '';
    }
    render() {
        var _a, _b;
        const { width, height, borderRadius, primaryColor, bgColor, textColor, sliderShape } = this.theme;
        this.root = document.createElement('div');
        this.root.className = 'sc-widget sc-idle';
        this.root.style.cssText = `
      width:${width}px; position:relative; user-select:none;
      font-family:-apple-system,BlinkMacSystemFont,sans-serif;
    `;
        this.track = document.createElement('div');
        this.track.style.cssText = `
      width:100%; height:${height}px; background:${bgColor};
      border:1px solid #ddd; border-radius:${borderRadius}px;
      position:relative; overflow:hidden;
    `;
        this.fill = document.createElement('div');
        this.fill.style.cssText = `
      position:absolute; top:0; left:0; height:100%;
      width:0; background:${primaryColor}; opacity:0.15;
      transition:background-color 0.3s;
    `;
        this.label = document.createElement('div');
        this.label.style.cssText = `
      position:absolute; top:0; left:0; width:100%; height:100%;
      display:flex; align-items:center; justify-content:center;
      color:${textColor}; font-size:14px; pointer-events:none;
    `;
        this.label.textContent = this.locale.idle;
        const thumbSize = height - 8;
        let thumbBorderRadius = '50%';
        let thumbContent = '→';
        if (sliderShape === 'square') {
            thumbBorderRadius = `${borderRadius}px`;
            thumbContent = '▶';
        }
        else if (sliderShape === 'arrow') {
            thumbBorderRadius = `${borderRadius}px`;
            thumbContent = '»';
        }
        this.thumb = document.createElement('div');
        this.thumb.style.cssText = `
      position:absolute; top:4px; left:0;
      width:${thumbSize}px; height:${thumbSize}px;
      background:${primaryColor}; border-radius:${thumbBorderRadius};
      display:flex; align-items:center; justify-content:center;
      color:white; font-size:18px; font-weight:bold;
      cursor:grab; z-index:10; touch-action:none;
      box-shadow:0 2px 6px rgba(0,0,0,0.2);
    `;
        this.thumb.textContent = thumbContent;
        this.track.appendChild(this.fill);
        this.track.appendChild(this.label);
        this.track.appendChild(this.thumb);
        this.root.appendChild(this.track);
        this.container.innerHTML = '';
        this.container.appendChild(this.root);
        this.bindDragEvents();
        this.setStatus('ready');
        (_b = (_a = this.callbacks).onReady) === null || _b === void 0 ? void 0 : _b.call(_a);
    }
    bindDragEvents() {
        const onStart = (clientX) => {
            var _a, _b;
            if (this.status !== 'ready')
                return;
            this.isDragging = true;
            this.dragStartTime = Date.now();
            this.dragTrajectory = [[0, 0]];
            this.thumb.style.cursor = 'grabbing';
            (_b = (_a = this.callbacks).onDragStart) === null || _b === void 0 ? void 0 : _b.call(_a);
        };
        const onMove = (clientX, clientY) => {
            if (!this.isDragging)
                return;
            const rect = this.track.getBoundingClientRect();
            const maxX = rect.width - this.thumb.offsetWidth - 8;
            let x = clientX - rect.left - this.thumb.offsetWidth / 2;
            x = Math.max(0, Math.min(x, maxX));
            this.thumbX = x;
            this.thumb.style.left = `${x}px`;
            this.fill.style.width = `${x + this.thumb.offsetWidth}px`;
            this.dragTrajectory.push([x, clientY - rect.top]);
        };
        const onEnd = () => {
            var _a, _b;
            if (!this.isDragging)
                return;
            this.isDragging = false;
            this.dragEndTime = Date.now();
            this.thumb.style.cursor = 'grab';
            const rect = this.track.getBoundingClientRect();
            const maxX = rect.width - this.thumb.offsetWidth - 8;
            const percent = this.thumbX / maxX;
            if (percent >= 0.85) {
                (_b = (_a = this.callbacks).onDragEnd) === null || _b === void 0 ? void 0 : _b.call(_a, this.thumbX);
            }
            else {
                this.reset();
            }
        };
        // Mouse events
        this.thumb.addEventListener('mousedown', (e) => {
            e.preventDefault();
            onStart(e.clientX);
        });
        document.addEventListener('mousemove', (e) => onMove(e.clientX, e.clientY));
        document.addEventListener('mouseup', () => onEnd());
        // Touch events
        this.thumb.addEventListener('touchstart', (e) => {
            e.preventDefault();
            const t = e.touches[0];
            onStart(t.clientX);
        }, { passive: false });
        document.addEventListener('touchmove', (e) => {
            const t = e.touches[0];
            if (t)
                onMove(t.clientX, t.clientY);
        }, { passive: true });
        document.addEventListener('touchend', () => onEnd());
    }
}

function extractFeaturesFromRaw(trajectory, timestamps) {
    if (trajectory.length < 2 || timestamps.length < 2) {
        return emptyFeatures();
    }
    const velocities = [];
    const accelerations = [];
    const angles = [];
    let totalPath = 0;
    let directionChanges = 0;
    const pauseDurations = [];
    for (let i = 1; i < trajectory.length; i++) {
        const dx = trajectory[i][0] - trajectory[i - 1][0];
        const dy = trajectory[i][1] - trajectory[i - 1][1];
        const dt = (timestamps[i] - timestamps[i - 1]) / 1000;
        const dist = Math.sqrt(dx * dx + dy * dy);
        totalPath += dist;
        if (dt > 0) {
            velocities.push(dist / dt);
        }
        if (dt * 1000 > 100 && dist < 2) {
            pauseDurations.push(dt * 1000);
        }
        if (i >= 2) {
            const prevDx = trajectory[i - 1][0] - trajectory[i - 2][0];
            const prevDy = trajectory[i - 1][1] - trajectory[i - 2][1];
            const angle = Math.atan2(dy, dx) - Math.atan2(prevDy, prevDx);
            angles.push(angle);
            if (angles.length > 1) {
                const prevAngle = angles[angles.length - 2];
                if (Math.sign(angle) !== Math.sign(prevAngle) && Math.abs(angle) > 0.1) {
                    directionChanges++;
                }
            }
        }
    }
    for (let i = 1; i < velocities.length; i++) {
        const dt = (timestamps[i + 1] - timestamps[i]) / 1000;
        if (dt > 0) {
            accelerations.push((velocities[i] - velocities[i - 1]) / dt);
        }
    }
    const avgVelocity = mean(velocities);
    const maxVelocity = velocities.length > 0 ? Math.max(...velocities) : 0;
    const velocityVariance = variance(velocities);
    const avgAcceleration = accelerations.length > 0
        ? accelerations.reduce((a, b) => a + Math.abs(b), 0) / accelerations.length : 0;
    let jerkSum = 0;
    for (let i = 1; i < accelerations.length; i++) {
        jerkSum += Math.abs(accelerations[i] - accelerations[i - 1]);
    }
    const jerkSmoothness = accelerations.length > 1 ? jerkSum / (accelerations.length - 1) : 0;
    const curvature = angles.length > 0
        ? angles.reduce((a, b) => a + Math.abs(b), 0) / angles.length : 0;
    const start = trajectory[0];
    const end = trajectory[trajectory.length - 1];
    const displacement = Math.sqrt(Math.pow(end[0] - start[0], 2) + Math.pow(end[1] - start[1], 2));
    const straightness = totalPath > 0 ? displacement / totalPath : 1;
    return {
        avgVelocity,
        maxVelocity,
        velocityVariance,
        avgAcceleration,
        jerkSmoothness,
        curvature,
        pauseCount: pauseDurations.length,
        pauseDurations,
        straightness,
        directionChanges,
        totalPathLength: totalPath,
        displacement,
    };
}
function mean(arr) {
    if (arr.length === 0)
        return 0;
    return arr.reduce((a, b) => a + b, 0) / arr.length;
}
function variance(arr) {
    if (arr.length === 0)
        return 0;
    const m = mean(arr);
    return arr.reduce((sum, v) => sum + Math.pow(v - m, 2), 0) / arr.length;
}
function emptyFeatures() {
    return {
        avgVelocity: 0, maxVelocity: 0, velocityVariance: 0,
        avgAcceleration: 0, jerkSmoothness: 0, curvature: 0,
        pauseCount: 0, pauseDurations: [], straightness: 1,
        directionChanges: 0, totalPathLength: 0, displacement: 0,
    };
}

function render(container, options = {}) {
    var _a;
    const el = typeof container === 'string'
        ? ((_a = document.querySelector(container)) !== null && _a !== void 0 ? _a : (() => { throw new Error('ShieldCaptcha: container not found'); })())
        : container;
    const apiBase = options.apiBase || window.CAPTCHA_API_BASE || '';
    const transport = new Transport(apiBase);
    const biometrics = new BiometricsCollector();
    const enableBiometrics = options.enableBiometrics !== false;
    let token = null;
    let currentChallenge = null;
    const widget = new CaptchaWidget(el, options.theme, {
        onReady: () => {
            if (enableBiometrics)
                biometrics.start(el);
            loadRemoteConfig();
            prefetchChallenge();
        },
        onDragEnd: async () => {
            await runVerification();
        },
    }, options.locale);
    async function loadRemoteConfig() {
        try {
            const config = await transport.fetchWidgetConfig();
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
                        themeToApply = { ...(themeToApply || {}), ...expConfig };
                    }
                }
            }
            if (themeToApply) {
                widget.applyTheme(themeToApply);
            }
        }
        catch (_a) {
            // Non-critical: continue with local/default theme
        }
    }
    function simpleHash(str) {
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
        }
        catch (err) {
            console.warn('[ShieldCaptcha] Failed to prefetch challenge:', err);
        }
    }
    async function runVerification() {
        var _a, _b, _c;
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
            let behavior;
            if (enableBiometrics) {
                biometrics.stop();
                behavior = biometrics.getPayload();
            }
            const result = await transport.submitVerification({
                challenge: currentChallenge,
                solution: powResult.solution,
                fingerprint,
                interaction,
                behavior,
            });
            if (result.success) {
                token = result.token || currentChallenge.id;
                widget.setStatus('success');
                (_a = options.onSuccess) === null || _a === void 0 ? void 0 : _a.call(options, token);
            }
            else {
                widget.setStatus('error', result.error || '验证失败');
                (_b = options.onError) === null || _b === void 0 ? void 0 : _b.call(options, result.error || 'verification_failed');
                currentChallenge = null;
                if (enableBiometrics) {
                    biometrics.reset();
                    biometrics.start(el);
                }
                prefetchChallenge();
            }
        }
        catch (err) {
            const message = err instanceof Error ? err.message : '网络错误';
            widget.setStatus('error', message);
            (_c = options.onError) === null || _c === void 0 ? void 0 : _c.call(options, message);
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

exports.BiometricsCollector = BiometricsCollector;
exports.CaptchaWidget = CaptchaWidget;
exports.Transport = Transport;
exports.collectFingerprint = collectFingerprint;
exports.extractFeaturesFromRaw = extractFeaturesFromRaw;
exports.render = render;
exports.solvePoW = solvePoW;
//# sourceMappingURL=shieldcaptcha.cjs.js.map
