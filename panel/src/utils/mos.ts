/**
 * MOS (Mean Opinion Score) Calculation Utility
 * 
 * Implements a simplified ITU-T G.107 E-Model for VoIP quality estimation.
 * The E-Model calculates an R-factor (0-100) which maps to MOS (1.0-5.0).
 * 
 * Reference: ITU-T Recommendation G.107 (06/2015)
 */

export type MosQuality = 'excellent' | 'good' | 'fair' | 'poor' | 'bad';

export interface MosResult {
  /** Mean Opinion Score (1.0 - 5.0) */
  mos: number;
  /** R-factor from E-Model (0 - 100) */
  rFactor: number;
  /** Quality classification */
  quality: MosQuality;
}

/**
 * Quality thresholds based on R-factor ranges
 * 
 * | R-Factor | MOS     | Quality   | User Satisfaction      |
 * |----------|---------|-----------|------------------------|
 * | 93+      | 4.3+    | Excellent | Very satisfied         |
 * | 80-93    | 4.0-4.3 | Good      | Satisfied              |
 * | 70-80    | 3.6-4.0 | Fair      | Some users dissatisfied|
 * | 60-70    | 3.1-3.6 | Poor      | Many users dissatisfied|
 * | <60      | <3.1    | Bad       | Nearly all dissatisfied|
 */
const QUALITY_THRESHOLDS = {
  excellent: { minR: 93, minMos: 4.3 },
  good: { minR: 80, minMos: 4.0 },
  fair: { minR: 70, minMos: 3.6 },
  poor: { minR: 60, minMos: 3.1 },
  bad: { minR: 0, minMos: 1.0 }
} as const;

/**
 * Calculate MOS from network metrics using simplified E-Model
 * 
 * @param latencyMs - Round-trip time in milliseconds (will be halved for one-way delay)
 * @param jitterMs - Jitter (standard deviation of RTT) in milliseconds
 * @param packetLossPercent - Packet loss as percentage (0-100)
 * @returns MosResult with mos, rFactor, and quality classification
 * 
 * @example
 * ```ts
 * const result = calculateMOS(50, 5, 0.5);
 * // { mos: 4.35, rFactor: 93.2, quality: 'excellent' }
 * ```
 */
export function calculateMOS(
  latencyMs: number,
  jitterMs: number,
  packetLossPercent: number
): MosResult {
  // Ensure non-negative values
  const rtt = Math.max(0, latencyMs);
  const jitter = Math.max(0, jitterMs);
  const loss = Math.max(0, Math.min(100, packetLossPercent));

  // Convert RTT to one-way delay (assume symmetric path)
  const oneWayDelay = rtt / 2;

  // Effective latency includes jitter buffer impact
  // Typical jitter buffer adds 2x jitter to delay
  const effectiveLatency = oneWayDelay + (jitter * 2) + 10; // 10ms codec delay

  // Calculate R-factor using simplified E-Model
  // R = R0 - Id - Ie
  // R0 = 93.2 (base value for G.711 codec)
  const R0 = 93.2;

  // Id = delay impairment factor
  // Simplified formula based on one-way delay
  let Id = 0;
  if (effectiveLatency > 177.3) {
    // High delay impairment
    Id = 0.024 * effectiveLatency + 0.11 * (effectiveLatency - 177.3);
  } else if (effectiveLatency > 0) {
    // Low delay impairment
    Id = 0.024 * effectiveLatency;
  }

  // Ie = equipment impairment factor (packet loss impact)
  // Simplified formula for random packet loss
  // Higher loss has exponentially worse impact on quality
  let Ie = 0;
  if (loss > 0) {
    // Burstiness factor for random loss (Bpl = 1 for random, higher for bursty)
    const Bpl = 1;
    // Packet loss robustness factor for G.711 (Ie-eff = 0-40 range)
    Ie = 30 * Math.log(1 + 15 * loss / (Bpl * 100));
  }

  // Calculate R-factor (clamp to 0-100)
  const R = Math.max(0, Math.min(100, R0 - Id - Ie));

  // Convert R-factor to MOS using standard formula
  // MOS = 1 + 0.035R + R(R-60)(100-R) × 7×10^-6
  let mos: number;
  if (R <= 0) {
    mos = 1.0;
  } else if (R >= 100) {
    mos = 4.5;
  } else {
    mos = 1 + 0.035 * R + R * (R - 60) * (100 - R) * 7e-6;
  }

  // Clamp MOS to valid range
  mos = Math.max(1.0, Math.min(5.0, mos));

  // Determine quality classification
  const quality = getMosQuality(mos);

  return {
    mos: Number(mos.toFixed(2)),
    rFactor: Number(R.toFixed(1)),
    quality
  };
}

/**
 * Get quality classification from MOS value
 */
export function getMosQuality(mos: number): MosQuality {
  if (mos >= QUALITY_THRESHOLDS.excellent.minMos) return 'excellent';
  if (mos >= QUALITY_THRESHOLDS.good.minMos) return 'good';
  if (mos >= QUALITY_THRESHOLDS.fair.minMos) return 'fair';
  if (mos >= QUALITY_THRESHOLDS.poor.minMos) return 'poor';
  return 'bad';
}

/**
 * Get CSS color class for MOS quality
 */
export function getMosColorClass(quality: MosQuality): string {
  switch (quality) {
    case 'excellent': return 'mos-excellent';
    case 'good': return 'mos-good';
    case 'fair': return 'mos-fair';
    case 'poor': return 'mos-poor';
    case 'bad': return 'mos-bad';
  }
}

/**
 * Get hex color for MOS quality (for charts/visualizations)
 */
export function getMosColor(quality: MosQuality): string {
  switch (quality) {
    case 'excellent': return '#10b981'; // Green
    case 'good': return '#3b82f6';      // Blue
    case 'fair': return '#eab308';      // Yellow
    case 'poor': return '#f97316';      // Orange
    case 'bad': return '#ef4444';       // Red
  }
}

/**
 * Get quality label for display
 */
export function getMosQualityLabel(quality: MosQuality): string {
  switch (quality) {
    case 'excellent': return 'Excellent';
    case 'good': return 'Good';
    case 'fair': return 'Fair';
    case 'poor': return 'Poor';
    case 'bad': return 'Bad';
  }
}
