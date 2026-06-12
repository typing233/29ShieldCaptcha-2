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
export declare function collectFingerprint(): Promise<string>;
