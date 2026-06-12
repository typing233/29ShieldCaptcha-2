export interface ThemeConfig {
  primaryColor: string;
  successColor: string;
  errorColor: string;
  bgColor: string;
  textColor: string;
  borderRadius: number;
  width: number;
  height: number;
  sliderShape: 'round' | 'square' | 'arrow';
}

const DEFAULT_THEME: ThemeConfig = {
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

export type WidgetStatus = 'idle' | 'ready' | 'solving' | 'verifying' | 'success' | 'error';

export interface WidgetCallbacks {
  onReady?: () => void;
  onStart?: () => void;
  onProgress?: (percent: number) => void;
  onSuccess?: (token: string) => void;
  onError?: (error: string) => void;
  onDragStart?: () => void;
  onDragEnd?: (x: number) => void;
}

export class CaptchaWidget {
  private container: HTMLElement;
  private theme: ThemeConfig;
  private callbacks: WidgetCallbacks;
  private status: WidgetStatus = 'idle';
  private root!: HTMLElement;
  private track!: HTMLElement;
  private thumb!: HTMLElement;
  private fill!: HTMLElement;
  private label!: HTMLElement;
  private dragTrajectory: number[][] = [];
  private dragStartTime = 0;
  private dragEndTime = 0;
  private isDragging = false;
  private thumbX = 0;
  private locale: Record<string, string>;

  constructor(
    container: HTMLElement,
    theme: Partial<ThemeConfig> = {},
    callbacks: WidgetCallbacks = {},
    locale?: Record<string, string>
  ) {
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
      type: 'drag' as const,
      start_time: this.dragStartTime,
      end_time: this.dragEndTime,
      trajectory: this.dragTrajectory,
    };
  }

  getTrajectory(): number[][] {
    return this.dragTrajectory;
  }

  setStatus(status: WidgetStatus, message?: string): void {
    this.status = status;
    this.label.textContent = message || this.locale[status] || '';

    this.root.className = 'sc-widget sc-' + status;
    if (status === 'success') {
      this.fill.style.width = '100%';
      this.fill.style.backgroundColor = this.theme.successColor;
    } else if (status === 'error') {
      this.fill.style.backgroundColor = this.theme.errorColor;
      setTimeout(() => this.reset(), 2000);
    }
  }

  applyTheme(partial: Partial<ThemeConfig>): void {
    const changed = Object.keys(partial).some(
      k => (partial as any)[k] !== (this.theme as any)[k]
    );
    if (!changed) return;
    this.theme = { ...this.theme, ...partial };
    this.container.innerHTML = '';
    this.render();
  }

  reset(): void {
    this.thumbX = 0;
    this.thumb.style.left = '0px';
    this.fill.style.width = '0px';
    this.fill.style.backgroundColor = this.theme.primaryColor;
    this.dragTrajectory = [];
    this.setStatus('ready');
  }

  destroy(): void {
    this.container.innerHTML = '';
  }

  private render(): void {
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
    } else if (sliderShape === 'arrow') {
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
    this.callbacks.onReady?.();
  }

  private bindDragEvents(): void {
    const onStart = (clientX: number) => {
      if (this.status !== 'ready') return;
      this.isDragging = true;
      this.dragStartTime = Date.now();
      this.dragTrajectory = [[0, 0]];
      this.thumb.style.cursor = 'grabbing';
      this.callbacks.onDragStart?.();
    };

    const onMove = (clientX: number, clientY: number) => {
      if (!this.isDragging) return;
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
      if (!this.isDragging) return;
      this.isDragging = false;
      this.dragEndTime = Date.now();
      this.thumb.style.cursor = 'grab';

      const rect = this.track.getBoundingClientRect();
      const maxX = rect.width - this.thumb.offsetWidth - 8;
      const percent = this.thumbX / maxX;

      if (percent >= 0.85) {
        this.callbacks.onDragEnd?.(this.thumbX);
      } else {
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
      if (t) onMove(t.clientX, t.clientY);
    }, { passive: true });
    document.addEventListener('touchend', () => onEnd());
  }
}
