<template>
  <div class="signals-panel">
    <!-- ICMP Artifacts Section -->
    <div class="signal-section">
      <div class="section-header">
        <i class="bi bi-broadcast"></i>
        <span>ICMP Artifacts</span>
        <span class="section-badge info">{{ icmpArtifactsCount }} detected</span>
      </div>
      <div class="section-content">
        <div class="artifact-grid">
          <!-- Rate Limited Hops -->
          <div v-if="signals.icmp_artifacts.rate_limit_suspected_hops.length" class="artifact-card">
            <div class="artifact-header">
              <i class="bi bi-speedometer"></i>
              <span>Rate Limited Hops</span>
            </div>
            <div class="artifact-hops">
              <span 
                v-for="hop in signals.icmp_artifacts.rate_limit_suspected_hops" 
                :key="hop" 
                class="hop-chip"
              >
                {{ hop }}
              </span>
            </div>
            <p class="artifact-desc">
              These hops appear to rate-limit ICMP TTL-exceeded responses, 
              causing apparent packet loss that doesn't affect actual traffic.
            </p>
          </div>

          <!-- Non-Propagating Loss Hops -->
          <div v-if="signals.icmp_artifacts.non_propagating_loss_hops.length" class="artifact-card">
            <div class="artifact-header">
              <i class="bi bi-arrow-bar-down"></i>
              <span>Non-Propagating Loss</span>
            </div>
            <div class="artifact-hops">
              <span 
                v-for="hop in signals.icmp_artifacts.non_propagating_loss_hops" 
                :key="hop" 
                class="hop-chip warning"
              >
                {{ hop }}
              </span>
            </div>
            <p class="artifact-desc">
              Loss at these hops doesn't propagate to the destination, 
              strongly indicating measurement artifacts rather than real packet loss.
            </p>
          </div>

          <!-- Timeout Segments -->
          <div v-if="signals.icmp_artifacts.timeout_only_segments.length" class="artifact-card">
            <div class="artifact-header">
              <i class="bi bi-clock-history"></i>
              <span>Timeout Segments</span>
            </div>
            <div v-for="(seg, idx) in signals.icmp_artifacts.timeout_only_segments" :key="idx" class="timeout-segment">
              <div class="segment-range">
                Hops {{ seg.from_hop }} → {{ seg.to_hop }}
              </div>
              <ul class="segment-notes">
                <li v-for="(note, nIdx) in seg.notes" :key="nIdx">{{ note }}</li>
              </ul>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Latency Anomalies -->
    <div v-if="signals.latency_anomalies.length" class="signal-section">
      <div class="section-header">
        <i class="bi bi-graph-up"></i>
        <span>Latency Anomalies</span>
        <span class="section-badge warning">{{ signals.latency_anomalies.length }}</span>
      </div>
      <div class="section-content">
        <div class="anomaly-list">
          <div 
            v-for="anomaly in signals.latency_anomalies" 
            :key="'lat-' + anomaly.hop"
            class="anomaly-card"
          >
            <div class="anomaly-header">
              <span class="anomaly-hop">Hop {{ anomaly.hop }}</span>
              <span class="anomaly-type">{{ formatAnomalyType(anomaly.type) }}</span>
              <span class="confidence-meter" :style="{ '--confidence': anomaly.confidence }">
                {{ (anomaly.confidence * 100).toFixed(0) }}% confidence
              </span>
            </div>
            <p class="anomaly-evidence">
              <i class="bi bi-quote"></i>
              {{ anomaly.evidence }}
            </p>
          </div>
        </div>
      </div>
    </div>

    <!-- Jitter Anomalies -->
    <div v-if="signals.jitter_anomalies.length" class="signal-section">
      <div class="section-header">
        <i class="bi bi-activity"></i>
        <span>Jitter Anomalies</span>
        <span class="section-badge warning">{{ signals.jitter_anomalies.length }}</span>
      </div>
      <div class="section-content">
        <div class="anomaly-list">
          <div 
            v-for="anomaly in signals.jitter_anomalies" 
            :key="'jit-' + anomaly.hop"
            class="anomaly-card jitter"
          >
            <div class="anomaly-header">
              <span class="anomaly-hop">Hop {{ anomaly.hop }}</span>
              <span class="confidence-meter" :style="{ '--confidence': anomaly.confidence }">
                {{ (anomaly.confidence * 100).toFixed(0) }}% confidence
              </span>
            </div>
            <p class="anomaly-evidence">
              <i class="bi bi-quote"></i>
              {{ anomaly.evidence }}
            </p>
          </div>
        </div>
      </div>
    </div>

    <!-- Path Policy Flags -->
    <div v-if="signals.path_policy_flags.length" class="signal-section">
      <div class="section-header">
        <i class="bi bi-signpost-split"></i>
        <span>Path Policy Flags</span>
        <span class="section-badge danger">{{ signals.path_policy_flags.length }}</span>
      </div>
      <div class="section-content">
        <div class="policy-list">
          <div 
            v-for="(flag, idx) in signals.path_policy_flags" 
            :key="idx"
            class="policy-card"
          >
            <div class="policy-header">
              <span class="policy-flag">{{ formatPolicyFlag(flag.flag) }}</span>
              <span class="confidence-meter" :style="{ '--confidence': flag.confidence }">
                {{ (flag.confidence * 100).toFixed(0) }}%
              </span>
            </div>
            <div class="policy-body">
              <div class="policy-row">
                <span class="policy-label">Evidence</span>
                <p>{{ flag.evidence }}</p>
              </div>
              <div class="policy-row impact">
                <span class="policy-label">Impact</span>
                <p>{{ flag.impact }}</p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <div v-if="!hasSignals" class="empty-state">
      <i class="bi bi-check-circle"></i>
      <p>No significant signals detected in this measurement.</p>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { computed } from 'vue';
import type { MtrSignals } from './types';

const props = defineProps<{
  signals: MtrSignals;
}>();

const icmpArtifactsCount = computed(() => {
  return (
    props.signals.icmp_artifacts.rate_limit_suspected_hops.length +
    props.signals.icmp_artifacts.non_propagating_loss_hops.length +
    props.signals.icmp_artifacts.timeout_only_segments.length
  );
});

const hasSignals = computed(() => {
  return (
    icmpArtifactsCount.value > 0 ||
    props.signals.latency_anomalies.length > 0 ||
    props.signals.jitter_anomalies.length > 0 ||
    props.signals.path_policy_flags.length > 0
  );
});

function formatAnomalyType(type: string): string {
  return type.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase());
}

function formatPolicyFlag(flag: string): string {
  return flag.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase());
}
</script>

<style scoped>
.signals-panel {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

/* Section */
.signal-section {
  background: var(--bs-tertiary-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 12px;
  overflow: hidden;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 1rem 1.25rem;
  background: var(--bs-secondary-bg);
  font-weight: 600;
  font-size: 0.95rem;
  color: var(--bs-body-color);
}

.section-badge {
  margin-left: auto;
  padding: 0.2rem 0.6rem;
  border-radius: 4px;
  font-size: 0.7rem;
  font-weight: 600;
}

.section-badge.info {
  background: rgba(var(--bs-info-rgb), 0.15);
  color: var(--bs-info);
}

.section-badge.warning {
  background: rgba(var(--bs-warning-rgb), 0.15);
  color: var(--bs-warning);
}

.section-badge.danger {
  background: rgba(var(--bs-danger-rgb), 0.15);
  color: var(--bs-danger);
}

.section-content {
  padding: 1.25rem;
}

/* Artifact Grid */
.artifact-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 1rem;
}

.artifact-card {
  background: var(--bs-body-bg);
  border-radius: 10px;
  padding: 1rem;
  border: 1px solid var(--bs-border-color);
}

.artifact-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 600;
  font-size: 0.9rem;
  color: var(--bs-info);
  margin-bottom: 0.75rem;
}

.artifact-hops {
  display: flex;
  gap: 0.35rem;
  flex-wrap: wrap;
  margin-bottom: 0.75rem;
}

.hop-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  background: var(--bs-primary);
  color: white;
  border-radius: 6px;
  font-weight: 600;
  font-size: 0.8rem;
}

.hop-chip.warning {
  background: var(--bs-warning);
  color: var(--bs-dark);
}

.artifact-desc {
  margin: 0;
  font-size: 0.8rem;
  line-height: 1.5;
  color: var(--bs-secondary-color);
}

.timeout-segment {
  padding: 0.75rem;
  background: var(--bs-tertiary-bg);
  border-radius: 8px;
  margin-bottom: 0.5rem;
}

.timeout-segment:last-child {
  margin-bottom: 0;
}

.segment-range {
  font-weight: 600;
  font-size: 0.85rem;
  color: var(--bs-body-color);
  margin-bottom: 0.5rem;
}

.segment-notes {
  margin: 0;
  padding-left: 1rem;
  font-size: 0.8rem;
  color: var(--bs-secondary-color);
}

/* Anomaly List */
.anomaly-list, .policy-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.anomaly-card, .policy-card {
  background: var(--bs-body-bg);
  border-radius: 10px;
  padding: 1rem;
  border: 1px solid var(--bs-border-color);
}

.anomaly-card {
  border-left: 3px solid var(--bs-warning);
}

.anomaly-card.jitter {
  border-left-color: var(--bs-info);
}

.anomaly-header, .policy-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-wrap: wrap;
  margin-bottom: 0.75rem;
}

.anomaly-hop {
  font-weight: 700;
  font-size: 0.9rem;
  color: var(--bs-primary);
}

.anomaly-type {
  font-size: 0.75rem;
  padding: 0.2rem 0.5rem;
  background: rgba(var(--bs-warning-rgb), 0.15);
  color: var(--bs-warning);
  border-radius: 4px;
  font-weight: 500;
}

.confidence-meter {
  margin-left: auto;
  font-size: 0.7rem;
  color: var(--bs-secondary-color);
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

.confidence-meter::before {
  content: '';
  width: 40px;
  height: 4px;
  background: var(--bs-border-color);
  border-radius: 2px;
  position: relative;
  overflow: hidden;
}

.confidence-meter::after {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  height: 100%;
  width: calc(var(--confidence) * 100%);
  background: var(--bs-success);
  border-radius: 2px;
}

.anomaly-evidence {
  margin: 0;
  font-size: 0.85rem;
  color: var(--bs-body-color);
  padding: 0.75rem;
  background: var(--bs-tertiary-bg);
  border-radius: 8px;
  line-height: 1.5;
}

.anomaly-evidence i {
  color: var(--bs-secondary-color);
  margin-right: 0.35rem;
}

/* Policy Cards */
.policy-card {
  border-left: 3px solid var(--bs-danger);
}

.policy-flag {
  font-weight: 700;
  font-size: 0.9rem;
  color: var(--bs-danger);
}

.policy-body {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.policy-row {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.policy-label {
  font-size: 0.7rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--bs-secondary-color);
  letter-spacing: 0.5px;
}

.policy-row p {
  margin: 0;
  font-size: 0.85rem;
  color: var(--bs-body-color);
  line-height: 1.5;
}

.policy-row.impact {
  padding: 0.75rem;
  background: rgba(var(--bs-danger-rgb), 0.05);
  border-radius: 8px;
  border: 1px solid rgba(var(--bs-danger-rgb), 0.15);
}

.policy-row.impact .policy-label {
  color: var(--bs-danger);
}

/* Empty State */
.empty-state {
  text-align: center;
  padding: 3rem;
  color: var(--bs-secondary-color);
}

.empty-state i {
  font-size: 3rem;
  color: var(--bs-success);
  margin-bottom: 1rem;
}

.empty-state p {
  margin: 0;
  font-size: 1rem;
}
</style>
