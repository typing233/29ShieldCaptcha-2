import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Counter } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

const verifySuccess = new Rate('verify_success');
const challenges = new Counter('challenges_fetched');

export const options = {
  scenarios: {
    // Normal traffic: 80% human-like
    human_traffic: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 50 },
        { duration: '2m', target: 100 },
        { duration: '1m', target: 200 },
        { duration: '30s', target: 0 },
      ],
      exec: 'humanFlow',
    },
    // Bot traffic: 20% automated
    bot_traffic: {
      executor: 'constant-arrival-rate',
      rate: 20,
      timeUnit: '1s',
      duration: '3m',
      preAllocatedVUs: 50,
      exec: 'botFlow',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<200', 'p(99)<500'],
    verify_success: ['rate>0.7'],
    http_req_failed: ['rate<0.05'],
  },
};

function sha256Hex(data) {
  // k6 doesn't have native sha256, use a placeholder fingerprint
  return 'a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2';
}

function generateHumanTrajectory() {
  const points = [];
  for (let i = 0; i < 25; i++) {
    const x = i * 12 + Math.random() * 5 - 2.5;
    const y = 24 + Math.random() * 8 - 4;
    points.push([x, y]);
  }
  return points;
}

function generateBotTrajectory() {
  const points = [];
  for (let i = 0; i < 5; i++) {
    points.push([i * 60, 24]);
  }
  return points;
}

export function humanFlow() {
  // 1. Get challenge
  const challengeResp = http.get(`${BASE_URL}/api/challenge`);
  check(challengeResp, { 'challenge 200': (r) => r.status === 200 });
  challenges.add(1);

  if (challengeResp.status !== 200) return;
  const challenge = JSON.parse(challengeResp.body);

  // Simulate human thinking time
  sleep(Math.random() * 2 + 0.5);

  // 2. Submit verification (PoW would normally be solved, here we test latency)
  const now = Date.now();
  const payload = JSON.stringify({
    challenge: challenge,
    solution: '0', // Will fail PoW but tests the pipeline
    fingerprint: sha256Hex('test'),
    interaction: {
      type: 'drag',
      start_time: now - 1500,
      end_time: now,
      trajectory: generateHumanTrajectory(),
    },
    behavior: {
      trajectory: generateHumanTrajectory(),
      timestamps: Array.from({ length: 25 }, (_, i) => now - 1500 + i * 60),
      pressures: Array.from({ length: 25 }, () => 0),
      features: {
        avg_velocity: 150 + Math.random() * 50,
        max_velocity: 300 + Math.random() * 100,
        velocity_variance: 80 + Math.random() * 40,
        avg_acceleration: 50,
        jerk_smoothness: 25 + Math.random() * 20,
        curvature: 0.1 + Math.random() * 0.1,
        pause_count: Math.floor(Math.random() * 3),
        straightness: 0.7 + Math.random() * 0.15,
        direction_changes: 5 + Math.floor(Math.random() * 5),
        total_path_length: 250 + Math.random() * 50,
        displacement: 200,
      },
    },
  });

  const verifyResp = http.post(`${BASE_URL}/api/verify`, payload, {
    headers: { 'Content-Type': 'application/json' },
  });

  check(verifyResp, { 'verify 200': (r) => r.status === 200 });
  if (verifyResp.status === 200) {
    const result = JSON.parse(verifyResp.body);
    verifySuccess.add(result.success ? 1 : 0);
  }

  sleep(Math.random() * 1);
}

export function botFlow() {
  const challengeResp = http.get(`${BASE_URL}/api/challenge`);
  if (challengeResp.status !== 200) return;
  challenges.add(1);

  const challenge = JSON.parse(challengeResp.body);
  const now = Date.now();

  const payload = JSON.stringify({
    challenge: challenge,
    solution: '0',
    fingerprint: sha256Hex('bot'),
    interaction: {
      type: 'drag',
      start_time: now - 100,
      end_time: now,
      trajectory: generateBotTrajectory(),
    },
  });

  http.post(`${BASE_URL}/api/verify`, payload, {
    headers: { 'Content-Type': 'application/json' },
  });
}
