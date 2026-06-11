(function() {
    'use strict';

    const API_BASE = window.CAPTCHA_API_BASE || '';
    let currentChallenge = null;
    let interactionData = null;
    let worker = null;

    const widget = document.getElementById('captchaWidget');
    const slider = document.getElementById('slider');
    const label = document.getElementById('widgetLabel');
    const status = document.getElementById('status');
    const progressBar = document.getElementById('progressBar');
    const retryBtn = document.getElementById('retryBtn');

    init();

    async function init() {
        resetUI();
        setupInteraction();
        retryBtn.addEventListener('click', function() { init(); });
        await fetchChallenge();
    }

    function resetUI() {
        widget.className = 'captcha-widget';
        slider.style.width = '50px';
        label.textContent = '向右拖动滑块';
        status.textContent = '';
        status.className = 'status';
        progressBar.style.width = '0%';
        retryBtn.style.display = 'none';
    }

    async function fetchChallenge() {
        try {
            setStatus('正在获取验证质询...', '');
            const resp = await fetch(API_BASE + '/api/challenge');
            if (!resp.ok) {
                throw new Error('HTTP ' + resp.status);
            }
            currentChallenge = await resp.json();
            setStatus('请拖动滑块完成验证', '');
        } catch (e) {
            setStatus('获取质询失败: ' + e.message, 'error');
            retryBtn.style.display = 'inline-block';
        }
    }

    function setupInteraction() {
        let isDragging = false;
        let startX = 0;
        let trajectory = [];
        let startTime = 0;
        const trackWidth = widget.offsetWidth - 50;

        slider.addEventListener('mousedown', onStart);
        slider.addEventListener('touchstart', onStart);

        function onStart(e) {
            if (!currentChallenge) return;
            e.preventDefault();
            isDragging = true;
            startX = e.type === 'touchstart' ? e.touches[0].clientX : e.clientX;
            startTime = Date.now();
            trajectory = [[0, 0, 0]];
            slider.classList.add('dragging');
            widget.classList.add('active');

            document.addEventListener('mousemove', onMove);
            document.addEventListener('mouseup', onEnd);
            document.addEventListener('touchmove', onMove);
            document.addEventListener('touchend', onEnd);
        }

        function onMove(e) {
            if (!isDragging) return;
            const clientX = e.type === 'touchmove' ? e.touches[0].clientX : e.clientX;
            const clientY = e.type === 'touchmove' ? e.touches[0].clientY : e.clientY;
            let dx = clientX - startX;
            dx = Math.max(0, Math.min(dx, trackWidth));

            slider.style.width = (50 + dx) + 'px';
            trajectory.push([dx, clientY - widget.getBoundingClientRect().top, Date.now() - startTime]);
        }

        function onEnd(e) {
            if (!isDragging) return;
            isDragging = false;
            slider.classList.remove('dragging');

            document.removeEventListener('mousemove', onMove);
            document.removeEventListener('mouseup', onEnd);
            document.removeEventListener('touchmove', onMove);
            document.removeEventListener('touchend', onEnd);

            const sliderWidth = parseInt(slider.style.width) - 50;

            if (sliderWidth >= trackWidth * 0.85) {
                widget.classList.remove('active');
                widget.classList.add('success');
                label.textContent = '';

                interactionData = {
                    type: 'drag',
                    start_time: startTime,
                    end_time: Date.now(),
                    trajectory: trajectory.map(function(p) { return [p[0], p[1]]; })
                };

                startVerification();
            } else {
                slider.style.width = '50px';
                widget.classList.remove('active');
            }
        }
    }

    async function startVerification() {
        setStatus('正在采集指纹...', '');
        progressBar.style.width = '20%';

        const fingerprint = await collectFingerprint();
        progressBar.style.width = '40%';

        setStatus('正在计算 PoW...', '');
        const solution = await solvePoW(currentChallenge);
        progressBar.style.width = '80%';

        setStatus('正在提交验证...', '');
        await submitVerification(fingerprint, solution);
    }

    async function collectFingerprint() {
        const components = {};

        try {
            const canvas = document.createElement('canvas');
            canvas.width = 200;
            canvas.height = 50;
            const ctx = canvas.getContext('2d');
            ctx.textBaseline = 'top';
            ctx.font = '14px Arial';
            ctx.fillStyle = '#f60';
            ctx.fillRect(10, 1, 62, 20);
            ctx.fillStyle = '#069';
            ctx.fillText('ShieldCaptcha', 2, 15);
            ctx.fillStyle = 'rgba(102, 204, 0, 0.7)';
            ctx.fillText('fingerprint', 4, 17);
            components.canvas = canvas.toDataURL();
        } catch (e) {
            components.canvas = 'unavailable';
        }

        try {
            const glCanvas = document.createElement('canvas');
            const gl = glCanvas.getContext('webgl') || glCanvas.getContext('experimental-webgl');
            if (gl) {
                components.webgl_vendor = gl.getParameter(gl.VENDOR);
                components.webgl_renderer = gl.getParameter(gl.RENDERER);
                const ext = gl.getExtension('WEBGL_debug_renderer_info');
                if (ext) {
                    components.webgl_unmasked_vendor = gl.getParameter(ext.UNMASKED_VENDOR_WEBGL);
                    components.webgl_unmasked_renderer = gl.getParameter(ext.UNMASKED_RENDERER_WEBGL);
                }
            }
        } catch (e) {
            components.webgl = 'unavailable';
        }

        const testFonts = ['Arial', 'Verdana', 'Times New Roman', 'Courier New', 'Georgia',
                           'Comic Sans MS', 'Impact', 'Trebuchet MS', 'Palatino', 'Lucida Console'];
        const detectedFonts = [];
        const span = document.createElement('span');
        span.style.cssText = 'position:absolute;left:-9999px;font-size:72px;';
        span.textContent = 'mmmmmmmmmmlli';
        document.body.appendChild(span);
        span.style.fontFamily = 'monospace';
        const baseWidth = span.offsetWidth;
        for (let i = 0; i < testFonts.length; i++) {
            span.style.fontFamily = '"' + testFonts[i] + '", monospace';
            if (span.offsetWidth !== baseWidth) {
                detectedFonts.push(testFonts[i]);
            }
        }
        document.body.removeChild(span);
        components.fonts = detectedFonts.join(',');

        components.screen = screen.width + 'x' + screen.height + 'x' + screen.colorDepth;
        components.timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
        components.language = navigator.language;
        components.platform = navigator.platform;
        components.cores = navigator.hardwareConcurrency || 0;
        components.memory = navigator.deviceMemory || 0;
        components.touch = navigator.maxTouchPoints || 0;

        const raw = JSON.stringify(components);
        const hash = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(raw));
        return Array.from(new Uint8Array(hash)).map(function(b) { return b.toString(16).padStart(2, '0'); }).join('');
    }

    function solvePoW(challenge) {
        return new Promise(function(resolve, reject) {
            if (worker) worker.terminate();

            worker = new Worker('worker.js');
            worker.onmessage = function(e) {
                if (e.data.found) {
                    worker.terminate();
                    worker = null;
                    resolve(e.data.solution);
                } else {
                    const percent = Math.min(40 + (e.data.iterations / 1000000) * 40, 79);
                    progressBar.style.width = percent + '%';
                }
            };
            worker.onerror = function(e) {
                reject(new Error('Worker error: ' + e.message));
            };
            worker.postMessage({
                challengeId: challenge.id,
                nonce: challenge.nonce,
                difficulty: challenge.difficulty
            });
        });
    }

    async function submitVerification(fingerprint, solution) {
        try {
            const body = {
                challenge: currentChallenge,
                solution: solution,
                fingerprint: fingerprint,
                interaction: interactionData
            };

            const resp = await fetch(API_BASE + '/api/verify', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body)
            });

            if (!resp.ok) {
                throw new Error('HTTP ' + resp.status);
            }

            const result = await resp.json();
            progressBar.style.width = '100%';

            if (result.success) {
                setStatus('✓ 验证通过 (Token: ' + result.token.substring(0, 8) + '...)', 'success');
                widget.classList.add('success');
                slider.style.width = '100%';
            } else {
                setStatus('✗ 验证失败: ' + result.error, 'error');
                widget.className = 'captcha-widget error';
                retryBtn.style.display = 'inline-block';
            }
        } catch (e) {
            setStatus('提交失败: ' + e.message, 'error');
            widget.className = 'captcha-widget error';
            retryBtn.style.display = 'inline-block';
        }
    }

    function setStatus(msg, type) {
        status.textContent = msg;
        status.className = 'status' + (type ? ' ' + type : '');
    }
})();
