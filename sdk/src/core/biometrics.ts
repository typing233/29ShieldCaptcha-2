export interface Point {
  x: number;
  y: number;
  t: number;
  pressure?: number;
  eventType: 'move' | 'down' | 'up' | 'touch';
}

export interface BehaviorFeatures {
  avgVelocity: number;
  maxVelocity: number;
  velocityVariance: number;
  avgAcceleration: number;
  jerkSmoothness: number;
  curvature: number;
  pauseCount: number;
  pauseDurations: number[];
  straightness: number;
  directionChanges: number;
  totalPathLength: number;
  displacement: number;
}

export interface BehaviorPayload {
  trajectory: number[][];
  timestamps: number[];
  pressures: number[];
  features: BehaviorFeatures;
}

const MAX_BUFFER_SIZE = 2000;
const PAUSE_THRESHOLD_MS = 100;

export class BiometricsCollector {
  private points: Point[] = [];
  private collecting = false;
  private element: HTMLElement | null = null;

  start(element: HTMLElement): void {
    this.element = element;
    this.collecting = true;
    this.points = [];
    this.bindEvents();
  }

  stop(): void {
    this.collecting = false;
    this.unbindEvents();
  }

  reset(): void {
    this.points = [];
  }

  getPayload(): BehaviorPayload {
    const trajectory = this.points.map(p => [p.x, p.y]);
    const timestamps = this.points.map(p => p.t);
    const pressures = this.points.map(p => p.pressure ?? 0);
    const features = this.extractFeatures();
    return { trajectory, timestamps, pressures, features };
  }

  extractFeatures(): BehaviorFeatures {
    const pts = this.points;
    if (pts.length < 2) {
      return this.emptyFeatures();
    }

    const velocities: number[] = [];
    const accelerations: number[] = [];
    const angles: number[] = [];
    let totalPath = 0;
    let directionChanges = 0;
    const pauseDurations: number[] = [];

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
    const displacement = Math.sqrt(
      Math.pow(endPt.x - startPt.x, 2) + Math.pow(endPt.y - startPt.y, 2)
    );
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

  private addPoint(x: number, y: number, eventType: Point['eventType'], pressure?: number): void {
    if (!this.collecting) return;
    if (this.points.length >= MAX_BUFFER_SIZE) {
      this.points.shift();
    }
    this.points.push({ x, y, t: Date.now(), pressure, eventType });
  }

  private onMouseMove = (e: MouseEvent): void => {
    this.addPoint(e.clientX, e.clientY, 'move');
  };

  private onMouseDown = (e: MouseEvent): void => {
    this.addPoint(e.clientX, e.clientY, 'down');
  };

  private onMouseUp = (e: MouseEvent): void => {
    this.addPoint(e.clientX, e.clientY, 'up');
  };

  private onTouchMove = (e: TouchEvent): void => {
    const touch = e.touches[0];
    if (touch) {
      this.addPoint(touch.clientX, touch.clientY, 'touch', (touch as any).force ?? 0);
    }
  };

  private onTouchStart = (e: TouchEvent): void => {
    const touch = e.touches[0];
    if (touch) {
      this.addPoint(touch.clientX, touch.clientY, 'down', (touch as any).force ?? 0);
    }
  };

  private onTouchEnd = (e: TouchEvent): void => {
    const touch = e.changedTouches[0];
    if (touch) {
      this.addPoint(touch.clientX, touch.clientY, 'up');
    }
  };

  private bindEvents(): void {
    const el = this.element ?? document;
    el.addEventListener('mousemove', this.onMouseMove as EventListener);
    el.addEventListener('mousedown', this.onMouseDown as EventListener);
    el.addEventListener('mouseup', this.onMouseUp as EventListener);
    el.addEventListener('touchmove', this.onTouchMove as EventListener, { passive: true });
    el.addEventListener('touchstart', this.onTouchStart as EventListener, { passive: true });
    el.addEventListener('touchend', this.onTouchEnd as EventListener);
  }

  private unbindEvents(): void {
    const el = this.element ?? document;
    el.removeEventListener('mousemove', this.onMouseMove as EventListener);
    el.removeEventListener('mousedown', this.onMouseDown as EventListener);
    el.removeEventListener('mouseup', this.onMouseUp as EventListener);
    el.removeEventListener('touchmove', this.onTouchMove as EventListener);
    el.removeEventListener('touchstart', this.onTouchStart as EventListener);
    el.removeEventListener('touchend', this.onTouchEnd as EventListener);
  }

  private variance(arr: number[]): number {
    if (arr.length === 0) return 0;
    const mean = arr.reduce((a, b) => a + b, 0) / arr.length;
    return arr.reduce((sum, v) => sum + Math.pow(v - mean, 2), 0) / arr.length;
  }

  private emptyFeatures(): BehaviorFeatures {
    return {
      avgVelocity: 0, maxVelocity: 0, velocityVariance: 0,
      avgAcceleration: 0, jerkSmoothness: 0, curvature: 0,
      pauseCount: 0, pauseDurations: [], straightness: 1,
      directionChanges: 0, totalPathLength: 0, displacement: 0,
    };
  }
}
