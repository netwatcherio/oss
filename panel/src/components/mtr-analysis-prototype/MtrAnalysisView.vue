<template>
  <div class="mtr-analysis-view">
    <!-- Header Section -->
    <div class="analysis-header">
      <div class="header-main">
        <div class="header-icon">
          <i class="bi bi-diagram-3-fill"></i>
        </div>
        <div class="header-info">
          <h2 class="header-title">MTR Path Analysis</h2>
          <div class="header-meta">
            <span class="meta-item">
              <i class="bi bi-geo-alt-fill"></i>
              {{ data.meta.source_region }}
            </span>
            <i class="bi bi-arrow-right"></i>
            <span class="meta-item">
              <i class="bi bi-bullseye"></i>
              {{ data.meta.dest_region }}
            </span>
          </div>
          <div class="header-details">
            <span class="detail-badge">
              <i class="bi bi-hdd-network"></i>
              {{ data.meta.target_ip }}
            </span>
            <span class="detail-badge">
              <i class="bi bi-clock-history"></i>
              {{ formatMeasurementTime(data.meta.measurement_time) }}
            </span>
            <span class="detail-badge traffic-type">
              <i class="bi bi-broadcast"></i>
              {{ data.meta.traffic_type.toUpperCase() }}
            </span>
          </div>
        </div>
      </div>
      
      <!-- End-to-End Summary Stats -->
      <div class="e2e-stats">
        <div class="stat-card" :class="getLossClass(data.signals.end_to_end.loss_pct)">
          <span class="stat-value">{{ data.signals.end_to_end.loss_pct }}%</span>
          <span class="stat-label">E2E Loss</span>
        </div>
        <div class="stat-card" :class="getLatencyClass(data.signals.end_to_end.rtt_avg_ms)">
          <span class="stat-value">{{ data.signals.end_to_end.rtt_avg_ms.toFixed(1) }}ms</span>
          <span class="stat-label">E2E Latency</span>
        </div>
        <div class="stat-card" :class="getJitterClass(data.signals.end_to_end.jitter_indicator)">
          <span class="stat-value">{{ data.signals.end_to_end.jitter_indicator.toUpperCase() }}</span>
          <span class="stat-label">Jitter</span>
        </div>
      </div>
    </div>

    <!-- Tab Navigation -->
    <div class="analysis-tabs">
      <button 
        v-for="tab in tabs" 
        :key="tab.id"
        class="tab-button"
        :class="{ active: activeTab === tab.id }"
        @click="activeTab = tab.id"
      >
        <i :class="tab.icon"></i>
        {{ tab.label }}
        <span v-if="tab.badge" class="tab-badge" :class="tab.badgeClass">{{ tab.badge }}</span>
      </button>
    </div>

    <!-- Tab Content -->
    <div class="tab-content">
      <!-- Path & Hops Tab -->
      <div v-if="activeTab === 'path'" class="tab-panel">
        <PathVisualization :path="data.path" />
      </div>

      <!-- Findings Tab -->
      <div v-if="activeTab === 'findings'" class="tab-panel">
        <FindingsList :findings="data.findings" />
      </div>

      <!-- Signals Tab -->
      <div v-if="activeTab === 'signals'" class="tab-panel">
        <SignalsPanel :signals="data.signals" />
      </div>

      <!-- Recommendations Tab -->
      <div v-if="activeTab === 'recommendations'" class="tab-panel">
        <RecommendationsPanel 
          :questions="data.questions_for_upstream" 
          :tests="data.recommended_tests" 
        />
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed } from 'vue';
import PathVisualization from './PathVisualization.vue';
import FindingsList from './FindingsList.vue';
import SignalsPanel from './SignalsPanel.vue';
import RecommendationsPanel from './RecommendationsPanel.vue';
import type { MtrAnalysisData } from './types';

const props = defineProps<{
  data: MtrAnalysisData;
}>();

const activeTab = ref('path');

const tabs = computed(() => [
  { 
    id: 'path', 
    label: 'Path & Hops', 
    icon: 'bi bi-diagram-3',
    badge: props.data.path.hops.length,
    badgeClass: 'badge-primary'
  },
  { 
    id: 'findings', 
    label: 'Findings', 
    icon: 'bi bi-clipboard-check',
    badge: props.data.findings.length,
    badgeClass: getFindingsBadgeClass()
  },
  { 
    id: 'signals', 
    label: 'Signals', 
    icon: 'bi bi-activity',
    badge: getSignalsCount(),
    badgeClass: 'badge-info'
  },
  { 
    id: 'recommendations', 
    label: 'Recommendations', 
    icon: 'bi bi-lightbulb',
    badge: props.data.recommended_tests.length + props.data.questions_for_upstream.length,
    badgeClass: 'badge-secondary'
  },
]);

function getFindingsBadgeClass(): string {
  const hasWarnings = props.data.findings.some(f => f.severity === 'warning' || f.severity === 'critical');
  return hasWarnings ? 'badge-warning' : 'badge-success';
}

function getSignalsCount(): number {
  return (
    props.data.signals.latency_anomalies.length +
    props.data.signals.jitter_anomalies.length +
    props.data.signals.path_policy_flags.length
  );
}

function formatMeasurementTime(timestamp: string): string {
  const date = new Date(timestamp);
  return date.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    timeZoneName: 'short'
  });
}

function getLossClass(loss: number): string {
  if (loss === 0) return 'stat-success';
  if (loss <= 5) return 'stat-warning';
  return 'stat-danger';
}

function getLatencyClass(latency: number): string {
  if (latency < 50) return 'stat-success';
  if (latency < 100) return 'stat-warning';
  return 'stat-danger';
}

function getJitterClass(jitter: string): string {
  if (jitter === 'low') return 'stat-success';
  if (jitter === 'medium') return 'stat-warning';
  return 'stat-danger';
}
</script>

<style scoped>
.mtr-analysis-view {
  font-family: 'JetBrains Mono', var(--bs-font-monospace);
}

/* Header */
.analysis-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 2rem;
  padding: 1.5rem;
  background: linear-gradient(135deg, var(--bs-tertiary-bg) 0%, var(--bs-body-bg) 100%);
  border: 1px solid var(--bs-border-color);
  border-radius: 12px;
  margin-bottom: 1.5rem;
}

.header-main {
  display: flex;
  gap: 1rem;
  align-items: flex-start;
}

.header-icon {
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bs-primary);
  color: white;
  border-radius: 12px;
  font-size: 1.5rem;
}

.header-title {
  margin: 0 0 0.25rem 0;
  font-size: 1.25rem;
  font-weight: 600;
  color: var(--bs-body-color);
}

.header-meta {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.9rem;
  color: var(--bs-body-color);
  margin-bottom: 0.75rem;
}

.header-meta > i {
  color: var(--bs-secondary-color);
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

.meta-item i {
  color: var(--bs-primary);
}

.header-details {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.detail-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.25rem 0.6rem;
  background: var(--bs-secondary-bg);
  border-radius: 6px;
  font-size: 0.75rem;
  color: var(--bs-secondary-color);
}

.detail-badge.traffic-type {
  background: rgba(var(--bs-info-rgb), 0.15);
  color: var(--bs-info);
}

/* E2E Stats */
.e2e-stats {
  display: flex;
  gap: 1rem;
}

.stat-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 0.75rem 1.25rem;
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  min-width: 100px;
}

.stat-value {
  font-size: 1.25rem;
  font-weight: 700;
}

.stat-label {
  font-size: 0.65rem;
  text-transform: uppercase;
  color: var(--bs-secondary-color);
  letter-spacing: 0.5px;
}

.stat-success { border-color: var(--bs-success); }
.stat-success .stat-value { color: var(--bs-success); }

.stat-warning { border-color: var(--bs-warning); }
.stat-warning .stat-value { color: var(--bs-warning); }

.stat-danger { border-color: var(--bs-danger); }
.stat-danger .stat-value { color: var(--bs-danger); }

/* Tabs */
.analysis-tabs {
  display: flex;
  gap: 0.5rem;
  border-bottom: 1px solid var(--bs-border-color);
  margin-bottom: 1.5rem;
  padding-bottom: 0;
}

.tab-button {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem 1.25rem;
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  color: var(--bs-secondary-color);
  font-size: 0.9rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
  margin-bottom: -1px;
}

.tab-button:hover {
  color: var(--bs-body-color);
  background: var(--bs-tertiary-bg);
}

.tab-button.active {
  color: var(--bs-primary);
  border-bottom-color: var(--bs-primary);
}

.tab-badge {
  padding: 0.15rem 0.5rem;
  border-radius: 10px;
  font-size: 0.7rem;
  font-weight: 600;
}

.badge-primary {
  background: rgba(var(--bs-primary-rgb), 0.2);
  color: var(--bs-primary);
}

.badge-success {
  background: rgba(var(--bs-success-rgb), 0.2);
  color: var(--bs-success);
}

.badge-warning {
  background: rgba(var(--bs-warning-rgb), 0.2);
  color: var(--bs-warning);
}

.badge-info {
  background: rgba(var(--bs-info-rgb), 0.2);
  color: var(--bs-info);
}

.badge-secondary {
  background: rgba(var(--bs-secondary-rgb), 0.2);
  color: var(--bs-secondary);
}

/* Tab Content */
.tab-content {
  min-height: 400px;
}

.tab-panel {
  animation: fadeIn 0.2s ease-out;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(10px); }
  to { opacity: 1; transform: translateY(0); }
}

/* Responsive */
@media (max-width: 992px) {
  .analysis-header {
    flex-direction: column;
  }
  
  .e2e-stats {
    width: 100%;
    justify-content: center;
  }
}

@media (max-width: 768px) {
  .analysis-tabs {
    flex-wrap: wrap;
  }
  
  .tab-button {
    flex: 1 1 45%;
    justify-content: center;
  }
}
</style>
