import { BehaviorPayload } from './core/biometrics';
export interface Challenge {
    id: string;
    nonce: string;
    difficulty: number;
    timestamp: number;
    expires_at: number;
    signature: string;
}
export interface VerifyPayload {
    challenge: Challenge;
    solution: string;
    fingerprint: string;
    interaction: {
        type: string;
        start_time: number;
        end_time: number;
        trajectory: number[][];
    };
    behavior?: BehaviorPayload;
}
export interface VerifyResult {
    success: boolean;
    token?: string;
    error?: string;
    timestamp: number;
    risk_score?: number;
    risk_reasons?: string[];
}
export declare class Transport {
    private apiBase;
    constructor(apiBase: string);
    fetchChallenge(): Promise<Challenge>;
    submitVerification(payload: VerifyPayload): Promise<VerifyResult>;
}
