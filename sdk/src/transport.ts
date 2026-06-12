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

export interface WidgetConfig {
  theme?: {
    primaryColor?: string;
    sliderShape?: string;
    width?: number;
    height?: number;
  };
  experiment?: {
    name: string;
    traffic_pct: number;
    config_a: Record<string, unknown>;
    config_b: Record<string, unknown>;
  } | null;
}

function serializeBehavior(behavior: BehaviorPayload): Record<string, unknown> {
  const result: Record<string, unknown> = {
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

  async fetchWidgetConfig(): Promise<WidgetConfig> {
    const resp = await fetch(`${this.apiBase}/api/config`, {
      method: 'GET',
      headers: { 'Content-Type': 'application/json' },
    });
    if (!resp.ok) {
      return {};
    }
    return resp.json();
  }

  async submitVerification(payload: VerifyPayload): Promise<VerifyResult> {
    const body: Record<string, unknown> = {
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
