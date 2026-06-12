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
export declare class BiometricsCollector {
    private points;
    private collecting;
    private element;
    start(element: HTMLElement): void;
    stop(): void;
    reset(): void;
    getPayload(): BehaviorPayload;
    extractFeatures(): BehaviorFeatures;
    private addPoint;
    private onMouseMove;
    private onMouseDown;
    private onMouseUp;
    private onTouchMove;
    private onTouchStart;
    private onTouchEnd;
    private bindEvents;
    private unbindEvents;
    private variance;
    private emptyFeatures;
}
