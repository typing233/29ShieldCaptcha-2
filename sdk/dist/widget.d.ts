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
export declare class CaptchaWidget {
    private container;
    private theme;
    private callbacks;
    private status;
    private root;
    private track;
    private thumb;
    private fill;
    private label;
    private dragTrajectory;
    private dragStartTime;
    private dragEndTime;
    private isDragging;
    private thumbX;
    private locale;
    constructor(container: HTMLElement, theme?: Partial<ThemeConfig>, callbacks?: WidgetCallbacks, locale?: Record<string, string>);
    getInteractionData(): {
        type: "drag";
        start_time: number;
        end_time: number;
        trajectory: number[][];
    };
    getTrajectory(): number[][];
    setStatus(status: WidgetStatus, message?: string): void;
    reset(): void;
    destroy(): void;
    private render;
    private bindDragEvents;
}
