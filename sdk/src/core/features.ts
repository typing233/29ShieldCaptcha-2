import { BehaviorFeatures } from './biometrics';

export { BehaviorFeatures };

export function extractFeaturesFromRaw(
  trajectory: number[][],
  timestamps: number[]
): BehaviorFeatures {
  if (trajectory.length < 2 || timestamps.length < 2) {
    return emptyFeatures();
  }

  const velocities: number[] = [];
  const accelerations: number[] = [];
  const angles: number[] = [];
  let totalPath = 0;
  let directionChanges = 0;
  const pauseDurations: number[] = [];

  for (let i = 1; i < trajectory.length; i++) {
    const dx = trajectory[i][0] - trajectory[i - 1][0];
    const dy = trajectory[i][1] - trajectory[i - 1][1];
    const dt = (timestamps[i] - timestamps[i - 1]) / 1000;
    const dist = Math.sqrt(dx * dx + dy * dy);
    totalPath += dist;

    if (dt > 0) {
      velocities.push(dist / dt);
    }

    if (dt * 1000 > 100 && dist < 2) {
      pauseDurations.push(dt * 1000);
    }

    if (i >= 2) {
      const prevDx = trajectory[i - 1][0] - trajectory[i - 2][0];
      const prevDy = trajectory[i - 1][1] - trajectory[i - 2][1];
      const angle = Math.atan2(dy, dx) - Math.atan2(prevDy, prevDx);
      angles.push(angle);

      if (angles.length > 1) {
        const prevAngle = angles[angles.length - 2];
        if (Math.sign(angle) !== Math.sign(prevAngle) && Math.abs(angle) > 0.1) {
          directionChanges++;
        }
      }
    }
  }

  for (let i = 1; i < velocities.length; i++) {
    const dt = (timestamps[i + 1] - timestamps[i]) / 1000;
    if (dt > 0) {
      accelerations.push((velocities[i] - velocities[i - 1]) / dt);
    }
  }

  const avgVelocity = mean(velocities);
  const maxVelocity = velocities.length > 0 ? Math.max(...velocities) : 0;
  const velocityVariance = variance(velocities);
  const avgAcceleration = accelerations.length > 0
    ? accelerations.reduce((a, b) => a + Math.abs(b), 0) / accelerations.length : 0;

  let jerkSum = 0;
  for (let i = 1; i < accelerations.length; i++) {
    jerkSum += Math.abs(accelerations[i] - accelerations[i - 1]);
  }
  const jerkSmoothness = accelerations.length > 1 ? jerkSum / (accelerations.length - 1) : 0;

  const curvature = angles.length > 0
    ? angles.reduce((a, b) => a + Math.abs(b), 0) / angles.length : 0;

  const start = trajectory[0];
  const end = trajectory[trajectory.length - 1];
  const displacement = Math.sqrt(
    Math.pow(end[0] - start[0], 2) + Math.pow(end[1] - start[1], 2)
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

function mean(arr: number[]): number {
  if (arr.length === 0) return 0;
  return arr.reduce((a, b) => a + b, 0) / arr.length;
}

function variance(arr: number[]): number {
  if (arr.length === 0) return 0;
  const m = mean(arr);
  return arr.reduce((sum, v) => sum + Math.pow(v - m, 2), 0) / arr.length;
}

function emptyFeatures(): BehaviorFeatures {
  return {
    avgVelocity: 0, maxVelocity: 0, velocityVariance: 0,
    avgAcceleration: 0, jerkSmoothness: 0, curvature: 0,
    pauseCount: 0, pauseDurations: [], straightness: 1,
    directionChanges: 0, totalPathLength: 0, displacement: 0,
  };
}
