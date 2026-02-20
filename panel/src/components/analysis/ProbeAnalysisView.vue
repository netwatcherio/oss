<script lang="ts" setup>
import { ref, onMounted, watch, computed } from 'vue'
import { ProbeDataService } from '@/services/apiService'
import type { ProbeAnalysis, HealthVector } from './types'
import { gradeColors, severityIcons } from './types'

const props = defineProps<{
  workspaceId: number | string
  probeId: number | string
}>()

const analysis = ref<ProbeAnalysis | null>(null)
const loading = ref(true)
const error = ref('')
const activeTab = ref<'overview' | 'signals' | 'findings'>('overview')

async function fetchAnalysis() {
  loading.value = true
  try {
    analysis.value = await ProbeDataService.probeAnalysis(props.workspaceId, props.probeId, { lookback: 60 })
    error.value = ''
  } catch (e: any) {
    error.value = e?.message || 'Failed to fetch analysis'
  } finally {
    loading.value = false
  }
}

function gradeColor(grade: string) {
  return gradeColors[grade] || gradeColors.unknown
}

function healthBarWidth(score: number) {
  return `${Math.max(0, Math.min(100, score))}%`
}

function formatMs(ms: number) {
  return ms > 0 ? `${ms.toFixed(1)}ms` : '—'
}

function formatPct(pct: number) {
  return pct > 0 ? `${pct.toFixed(2)}%` : '0%'
}

function formatMos(mos: number) {
  return mos > 0 ? mos.toFixed(2) : '—'
}

const hasBidirectional = computed(() => !!analysis.value?.reverse)

onMounted(fetchAnalysis)

watch(() => props.probeId, fetchAnalysis)
</script>

<template>
  <div class="probe-analysis">
    <!-- Loading -->
    <div v-if="loading" class="text-center py-4">
      <div class="spinner-border spinner-border-sm text-primary" role="status"></div>
      <span class="text-muted ms-2">Analyzing probe data...</span>
    </div>

    <!-- Error -->
    <div v-else-if="error" class="text-center py-4">
      <i class="bi bi-exclamation-triangle text-warning me-2"></i>
      <span class="text-muted">{{ error }}</span>
      <button class="btn btn-sm btn-link" @click="fetchAnalysis">Retry</button>
    </div>

    <!-- Analysis Content -->
    <div v-else-if="analysis">
      <!-- Header with direction badges -->
      <div class="analysis-header mb-3">
        <div class="d-flex align-items-center gap-3 flex-wrap">
          <!-- Forward direction -->
          <div class="direction-card" :style="{ borderColor: gradeColor(analysis.health.grade).border }">
            <div class="direction-label">
              <i class="bi bi-arrow-right me-1"></i>
              {{ analysis.agent_name || 'Source' }} → {{ analysis.target || 'Target' }}
            </div>
            <div class="d-flex align-items-center gap-2 mt-1">
              <span class="grade-badge" :style="{ background: gradeColor(analysis.health.grade).bg, color: gradeColor(analysis.health.grade).text }">
                {{ Math.round(analysis.health.overall_health) }}/100 · {{ analysis.health.grade }}
              </span>
              <span class="mos-badge">MOS {{ formatMos(analysis.health.mos_score) }}</span>
            </div>
          </div>

          <!-- Reverse direction (if bidirectional) -->
          <div v-if="hasBidirectional && analysis.reverse" class="direction-card" :style="{ borderColor: gradeColor(analysis.reverse.health.grade).border }">
            <div class="direction-label">
              <i class="bi bi-arrow-left me-1"></i>
              {{ analysis.reverse.agent_name }} → {{ analysis.reverse.target }}
            </div>
            <div class="d-flex align-items-center gap-2 mt-1">
              <span class="grade-badge" :style="{ background: gradeColor(analysis.reverse.health.grade).bg, color: gradeColor(analysis.reverse.health.grade).text }">
                {{ Math.round(analysis.reverse.health.overall_health) }}/100 · {{ analysis.reverse.health.grade }}
              </span>
              <span class="mos-badge">MOS {{ formatMos(analysis.reverse.health.mos_score) }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Tab Navigation -->
      <div class="analysis-tabs mb-3">
        <button :class="['tab-btn', activeTab === 'overview' && 'active']" @click="activeTab = 'overview'">
          <i class="bi bi-speedometer2 me-1"></i>Metrics
        </button>
        <button :class="['tab-btn', activeTab === 'signals' && 'active']" @click="activeTab = 'signals'">
          <i class="bi bi-broadcast me-1"></i>Signals
          <span v-if="analysis.signals?.length" class="tab-count">{{ analysis.signals.length }}</span>
        </button>
        <button :class="['tab-btn', activeTab === 'findings' && 'active']" @click="activeTab = 'findings'">
          <i class="bi bi-clipboard-data me-1"></i>Findings
          <span v-if="analysis.findings?.length" class="tab-count">{{ analysis.findings.length }}</span>
        </button>
      </div>

      <!-- Overview Tab -->
      <div v-if="activeTab === 'overview'" class="tab-content">
        <!-- Metric Comparison Table -->
        <div class="metrics-grid" :class="{ bidirectional: hasBidirectional }">
          <div class="metric-header">Metric</div>
          <div class="metric-header">
            <i class="bi bi-arrow-right me-1"></i>Forward
          </div>
          <div v-if="hasBidirectional" class="metric-header">
            <i class="bi bi-arrow-left me-1"></i>Reverse
          </div>

          <!-- Avg Latency -->
          <div class="metric-label">Avg Latency</div>
          <div class="metric-value">{{ formatMs(analysis.metrics.avg_latency) }}</div>
          <div v-if="hasBidirectional" class="metric-value">{{ formatMs(analysis.reverse!.metrics.avg_latency) }}</div>

          <!-- P95 Latency -->
          <div class="metric-label">P95 Latency</div>
          <div class="metric-value">{{ formatMs(analysis.metrics.p95_latency) }}</div>
          <div v-if="hasBidirectional" class="metric-value">{{ formatMs(analysis.reverse!.metrics.p95_latency) }}</div>

          <!-- Packet Loss -->
          <div class="metric-label">Packet Loss</div>
          <div class="metric-value" :class="{ 'text-danger': analysis.metrics.packet_loss > 3 }">
            {{ formatPct(analysis.metrics.packet_loss) }}
          </div>
          <div v-if="hasBidirectional" class="metric-value" :class="{ 'text-danger': analysis.reverse!.metrics.packet_loss > 3 }">
            {{ formatPct(analysis.reverse!.metrics.packet_loss) }}
          </div>

          <!-- Jitter -->
          <div class="metric-label">Jitter</div>
          <div class="metric-value">{{ formatMs(analysis.metrics.jitter) }}</div>
          <div v-if="hasBidirectional" class="metric-value">{{ formatMs(analysis.reverse!.metrics.jitter) }}</div>

          <!-- MOS -->
          <div class="metric-label">MOS Score</div>
          <div class="metric-value" :style="{ color: gradeColor(analysis.health.grade).text }">
            {{ formatMos(analysis.health.mos_score) }}
          </div>
          <div v-if="hasBidirectional" class="metric-value" :style="{ color: gradeColor(analysis.reverse!.health.grade).text }">
            {{ formatMos(analysis.reverse!.health.mos_score) }}
          </div>

          <!-- Samples -->
          <div class="metric-label">Samples</div>
          <div class="metric-value text-muted">{{ analysis.metrics.sample_count }}</div>
          <div v-if="hasBidirectional" class="metric-value text-muted">{{ analysis.reverse!.metrics.sample_count }}</div>
        </div>

        <!-- Health Vector Bars -->
        <div class="health-vectors mt-3">
          <h6 class="small text-muted mb-2">Health Scores</h6>
          <div class="vector-comparison">
            <div class="vector-side">
              <div class="vector-bar-row" v-for="dim in [
                { label: 'Latency', score: analysis.health.latency_score },
                { label: 'Loss', score: analysis.health.packet_loss_score },
                { label: 'Route Stability', score: analysis.health.route_stability },
              ]" :key="dim.label">
                <span class="vector-label">{{ dim.label }}</span>
                <div class="vector-bar-track">
                  <div class="vector-bar-fill" :style="{ width: healthBarWidth(dim.score), background: gradeColor(analysis.health.grade).border }"></div>
                </div>
                <span class="vector-score">{{ Math.round(dim.score) }}</span>
              </div>
            </div>
            <div v-if="hasBidirectional && analysis.reverse" class="vector-side">
              <div class="vector-bar-row" v-for="dim in [
                { label: 'Latency', score: analysis.reverse!.health.latency_score },
                { label: 'Loss', score: analysis.reverse!.health.packet_loss_score },
                { label: 'Route Stability', score: analysis.reverse!.health.route_stability },
              ]" :key="dim.label">
                <span class="vector-label">{{ dim.label }}</span>
                <div class="vector-bar-track">
                  <div class="vector-bar-fill" :style="{ width: healthBarWidth(dim.score), background: gradeColor(analysis.reverse!.health.grade).border }"></div>
                </div>
                <span class="vector-score">{{ Math.round(dim.score) }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Path Analysis (if available) -->
        <div v-if="analysis.path_analysis" class="path-analysis mt-3">
          <h6 class="small text-muted mb-2">Path Analysis</h6>
          <div class="path-stats">
            <div class="path-stat">
              <div class="stat-value">{{ analysis.path_analysis.hop_count }}</div>
              <div class="stat-label">Hops</div>
            </div>
            <div class="path-stat">
              <div class="stat-value">{{ analysis.path_analysis.unique_routes }}</div>
              <div class="stat-label">Routes</div>
            </div>
            <div class="path-stat">
              <div class="stat-value">{{ Math.round(analysis.path_analysis.route_stability_pct) }}%</div>
              <div class="stat-label">Stability</div>
            </div>
            <div class="path-stat" v-if="analysis.path_analysis.rate_limited_hops?.length">
              <div class="stat-value text-info">{{ analysis.path_analysis.rate_limited_hops.length }}</div>
              <div class="stat-label">ICMP Limited</div>
            </div>
          </div>
        </div>
      </div>

      <!-- Signals Tab -->
      <div v-if="activeTab === 'signals'" class="tab-content">
        <div v-if="!analysis.signals?.length" class="text-center text-muted py-3">
          <i class="bi bi-check-circle text-success me-2"></i>No anomalies detected
        </div>
        <div v-for="(signal, i) in analysis.signals" :key="i" class="signal-card" :class="signal.severity">
          <div class="d-flex align-items-start gap-2">
            <i :class="['bi', severityIcons[signal.severity] || 'bi-info-circle', 'signal-icon']"></i>
            <div class="flex-grow-1">
              <div class="signal-title">{{ signal.title }}</div>
              <div class="signal-evidence">{{ signal.evidence }}</div>
              <div class="signal-meta">
                <span class="signal-type">{{ signal.type }}</span>
                <span class="signal-confidence">{{ Math.round(signal.confidence * 100) }}% confidence</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Findings Tab -->
      <div v-if="activeTab === 'findings'" class="tab-content">
        <div v-if="!analysis.findings?.length" class="text-center text-muted py-3">
          <i class="bi bi-check-circle text-success me-2"></i>No findings
        </div>
        <div v-for="finding in analysis.findings" :key="finding.id" class="finding-card" :class="finding.severity">
          <div class="finding-header">
            <i :class="['bi', severityIcons[finding.severity] || 'bi-info-circle']"></i>
            <span class="finding-title">{{ finding.title }}</span>
            <span class="finding-category">{{ finding.category }}</span>
          </div>
          <p class="finding-summary">{{ finding.summary }}</p>
          <div v-if="finding.evidence?.length" class="finding-evidence">
            <div v-for="(e, j) in finding.evidence" :key="j" class="evidence-item">
              <i class="bi bi-dot"></i> {{ e }}
            </div>
          </div>
          <div v-if="finding.recommended_steps?.length" class="finding-steps">
            <small class="text-muted">Recommended:</small>
            <ol>
              <li v-for="(step, j) in finding.recommended_steps" :key="j">{{ step }}</li>
            </ol>
          </div>
        </div>
      </div>

      <div class="text-muted small mt-2 text-end">
        <i class="bi bi-clock me-1"></i>
        {{ new Date(analysis.generated_at).toLocaleTimeString() }}
      </div>
    </div>
  </div>
</template>

<style scoped>
.probe-analysis {
  padding: 0;
}
/* Direction Cards */
.direction-card {
  border: 1px solid;
  border-radius: 8px;
  padding: 8px 14px;
  background: var(--card-bg, #1e1e2e);
  flex: 1;
  min-width: 200px;
}
.direction-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-color, #fff);
}
.grade-badge {
  font-size: 11px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 10px;
  text-transform: capitalize;
}
.mos-badge {
  font-size: 11px;
  color: var(--text-muted, #888);
  background: var(--border-color, #333);
  padding: 2px 6px;
  border-radius: 4px;
}

/* Tabs */
.analysis-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--border-color, #333);
  padding-bottom: 0;
}
.tab-btn {
  font-size: 13px;
  padding: 6px 14px;
  border: none;
  background: none;
  color: var(--text-muted, #888);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: all 0.2s;
}
.tab-btn:hover { color: var(--text-color, #fff); }
.tab-btn.active {
  color: var(--primary, #3b82f6);
  border-bottom-color: var(--primary, #3b82f6);
}
.tab-count {
  font-size: 10px;
  font-weight: 700;
  background: var(--primary, #3b82f6);
  color: #fff;
  padding: 0 5px;
  border-radius: 8px;
  margin-left: 4px;
}

/* Metrics Grid */
.metrics-grid {
  display: grid;
  grid-template-columns: 120px 1fr;
  gap: 0;
  font-size: 13px;
}
.metrics-grid.bidirectional {
  grid-template-columns: 120px 1fr 1fr;
}
.metric-header {
  font-weight: 600;
  font-size: 12px;
  color: var(--text-muted, #888);
  padding: 4px 8px;
  border-bottom: 1px solid var(--border-color, #333);
  text-transform: uppercase;
}
.metric-label {
  color: var(--text-muted, #888);
  padding: 6px 8px;
  border-bottom: 1px solid var(--border-color, #282838);
}
.metric-value {
  font-weight: 600;
  font-family: 'SF Mono', 'Fira Code', monospace;
  padding: 6px 8px;
  border-bottom: 1px solid var(--border-color, #282838);
  color: var(--text-color, #fff);
}

/* Health Vectors */
.vector-comparison {
  display: flex;
  gap: 24px;
}
.vector-side {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.vector-bar-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.vector-label {
  font-size: 12px;
  color: var(--text-muted, #888);
  width: 100px;
  flex-shrink: 0;
}
.vector-bar-track {
  flex: 1;
  height: 6px;
  background: var(--border-color, #333);
  border-radius: 3px;
  overflow: hidden;
}
.vector-bar-fill {
  height: 100%;
  border-radius: 3px;
  transition: width 0.6s ease;
}
.vector-score {
  font-size: 12px;
  font-weight: 600;
  width: 30px;
  text-align: right;
  color: var(--text-color, #fff);
}

/* Path Analysis */
.path-stats {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
}
.path-stat {
  background: var(--card-bg, #1e1e2e);
  border: 1px solid var(--border-color, #333);
  border-radius: 8px;
  padding: 8px 16px;
  text-align: center;
  min-width: 70px;
}
.stat-value {
  font-size: 18px;
  font-weight: 700;
  color: var(--text-color, #fff);
}
.stat-label {
  font-size: 11px;
  color: var(--text-muted, #888);
  text-transform: uppercase;
}

/* Signals */
.signal-card {
  background: var(--card-bg, #1e1e2e);
  border: 1px solid var(--border-color, #333);
  border-radius: 8px;
  padding: 10px 14px;
  margin-bottom: 8px;
}
.signal-card.warning { border-left: 3px solid #f59e0b; }
.signal-card.critical { border-left: 3px solid #ef4444; }
.signal-card.info { border-left: 3px solid #3b82f6; }
.signal-icon { font-size: 16px; margin-top: 2px; }
.signal-card.warning .signal-icon { color: #f59e0b; }
.signal-card.critical .signal-icon { color: #ef4444; }
.signal-card.info .signal-icon { color: #3b82f6; }
.signal-title { font-weight: 600; font-size: 13px; color: var(--text-color, #fff); }
.signal-evidence { font-size: 12px; color: var(--text-muted, #888); margin-top: 2px; }
.signal-meta { font-size: 11px; color: var(--text-muted, #666); margin-top: 4px; display: flex; gap: 12px; }
.signal-type { background: var(--border-color, #333); padding: 1px 6px; border-radius: 3px; }

/* Findings */
.finding-card {
  background: var(--card-bg, #1e1e2e);
  border: 1px solid var(--border-color, #333);
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 10px;
}
.finding-card.warning { border-left: 3px solid #f59e0b; }
.finding-card.critical { border-left: 3px solid #ef4444; }
.finding-card.info { border-left: 3px solid #3b82f6; }
.finding-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}
.finding-header i { font-size: 16px; }
.finding-card.warning .finding-header i { color: #f59e0b; }
.finding-card.critical .finding-header i { color: #ef4444; }
.finding-card.info .finding-header i { color: #3b82f6; }
.finding-title { font-weight: 600; font-size: 14px; color: var(--text-color, #fff); }
.finding-category {
  font-size: 10px;
  text-transform: uppercase;
  background: var(--border-color, #333);
  padding: 1px 6px;
  border-radius: 3px;
  color: var(--text-muted, #888);
  margin-left: auto;
}
.finding-summary { font-size: 13px; color: var(--text-muted, #aaa); margin-bottom: 6px; }
.finding-evidence { font-size: 12px; color: var(--text-muted, #888); }
.evidence-item { padding: 1px 0; }
.finding-steps { margin-top: 8px; font-size: 12px; }
.finding-steps ol { padding-left: 18px; margin-bottom: 0; color: var(--text-muted, #aaa); }

/* ===== LIGHT theme overrides ===== */
[data-theme="light"] .direction-card {
  background: #f9fafb;
}
[data-theme="light"] .direction-label {
  color: #1f2937;
}
[data-theme="light"] .mos-badge {
  background: #e5e7eb;
  color: #4b5563;
}
[data-theme="light"] .metric-header {
  color: #6b7280;
  border-bottom-color: #e5e7eb;
}
[data-theme="light"] .metric-label {
  color: #6b7280;
  border-bottom-color: #f3f4f6;
}
[data-theme="light"] .metric-value {
  color: #1f2937;
  border-bottom-color: #f3f4f6;
}
[data-theme="light"] .vector-bar-track {
  background: #e5e7eb;
}
[data-theme="light"] .vector-label {
  color: #6b7280;
}
[data-theme="light"] .vector-score {
  color: #1f2937;
}
[data-theme="light"] .path-stat {
  background: #f9fafb;
  border-color: #e5e7eb;
}
[data-theme="light"] .stat-value {
  color: #1f2937;
}
[data-theme="light"] .stat-label {
  color: #6b7280;
}
[data-theme="light"] .signal-card {
  background: #f9fafb;
  border-color: #e5e7eb;
}
[data-theme="light"] .signal-title {
  color: #1f2937;
}
[data-theme="light"] .signal-evidence {
  color: #6b7280;
}
[data-theme="light"] .signal-meta {
  color: #9ca3af;
}
[data-theme="light"] .signal-type {
  background: #e5e7eb;
}
[data-theme="light"] .finding-card {
  background: #f9fafb;
  border-color: #e5e7eb;
}
[data-theme="light"] .finding-title {
  color: #1f2937;
}
[data-theme="light"] .finding-category {
  background: #e5e7eb;
  color: #6b7280;
}
[data-theme="light"] .finding-summary {
  color: #4b5563;
}
[data-theme="light"] .finding-evidence {
  color: #6b7280;
}
[data-theme="light"] .finding-steps ol {
  color: #4b5563;
}
[data-theme="light"] .tab-btn {
  color: #6b7280;
}
[data-theme="light"] .tab-btn:hover {
  color: #1f2937;
}
[data-theme="light"] .analysis-tabs {
  border-bottom-color: #e5e7eb;
}
</style>
