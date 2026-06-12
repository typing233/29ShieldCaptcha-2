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

export class Transport {
  private apiBase: string;

  constructor(apiBase: string) {
    this.apiBase = apiBase.replace(/\/$/, '');
  }

  async fetchChallenge(): Promise<Challenge> {
    const resp = await fetch(`${this.apiBase}/api/challenge`, {
      method: 'GET',
      headers: { 'Content-Type': 'application/json' },
    });
    if (!resp.ok) {
      throw new Error(`Challenge fetch failed: ${resp.status}`);
    }
    return resp.json();
  }

  async submitVerification(payload: VerifyPayload): Promise<VerifyResult> {
    const resp = await fetch(`${this.apiBase}/api/verify`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!resp.ok && resp.status !== 200) {
      throw new Error(`Verification request failed: ${resp.status}`);
    }
    return resp.json();
  }
}
