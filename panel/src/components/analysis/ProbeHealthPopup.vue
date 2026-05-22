<script lang="ts" setup>
import { ref, computed, onMounted } from 'vue'
import { ProbeDataService } from '@/services/apiService'
import type { ProbeAnalysis } from './types'
import { gradeColors } from './types'

interface Props {
  probeId: number | string
  workspaceId: number | string
  trigger?: 'hover' | 'click'
  agentName?: string
  target?: string
}

const props = withDefaults(defineProps<Props>(), {
  trigger: 'hover'
})

const analysis = ref<ProbeAnalysis | null>(null)
const loading = ref(false)
const error = ref('')
const isVisible = ref(false)

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

const healthScore = computed(() => {
  if (!analysis.value) return 0
  return Math.round(analysis.value.health.overall_health)
})

const healthGrade = computed(() => analysis.value?.health.grade || 'unknown')

const healthColor = computed(() => {
  const score = healthScore.value
  if (score >= 90) return gradeColors.excellent
  if (score >= 70) return gradeColors.good
  if (score >= 50) return gradeColors.fair
  return gradeColors.poor
})

const healthScoreColor = computed(() => {
  const score = healthScore.value
  if (score >= 90) return 'var(--bs-success)'
  if (score >= 70) return 'var(--bs-primary)'
  if (score >= 50) return 'var(--bs-warning)'
  return 'var(--bs-danger)'
})

const hasBidirectional = computed(() => !!analysis.value?.reverse)

const reverseHealthScore = computed(() => {
  if (!analysis.value?.reverse) return 0
  return Math.round(analysis.value.reverse.health.overall_health)
})

const reverseHealthGrade = computed(() => analysis.value?.reverse?.health.grade || 'unknown')

const reverseHealthColor = computed(() => {
  const score = reverseHealthScore.value
  if (score >= 90) return gradeColors.excellent
  if (score >= 70) return gradeColors.good
  if (score >= 50) return gradeColors.fair
  return gradeColors.poor
})

function formatMs(ms: number) {
  return ms > 0 ? `${ms.toFixed(1)}ms` : '—'
}

function formatMos(mos: number) {
  return mos > 0 ? mos.toFixed(2) : '—'
}

function healthBarWidth(score: number) {
  return `${Math.max(0, Math.min(100, score))}%`
}

function showPopup() {
  isVisible.value = true
  if (!analysis.value && !loading.value) {
    fetchAnalysis()
  }
}

function hidePopup() {
  isVisible.value = false
}

onMounted(() => {
  // Pre-fetch on mount for hover trigger
  if (props.trigger === 'hover') {
    fetchAnalysis()
  }
})

defineExpose({ showPopup, hidePopup })
</script>

<template>
  <div class="probe-health-popup">
    <slot :show="showPopup" :hide="hidePopup" :loading="loading"></slot>

    <Teleport to="body">
      <div v-if="isVisible" class="popup-overlay" @click.self="hidePopup">
        <div class="popup-content" @mouseenter="() => {}">
          <!-- Header -->
          <div class="popup-header">
            <div class="popup-title">
              <i class="bi bi-robot me-1"></i>
              <span>AI Analysis</span>
              <span class="popup-subtitle" v-if="agentName">{{ agentName }} → {{ target }}</span>
            </div>
            <button class="popup-close" @click="hidePopup">
              <i class="bi bi-x-lg"></i>
            </button>
          </div>

          <!-- Loading -->
          <div v-if="loading" class="popup-loading">
            <div class="spinner-border spinner-border-sm text-primary" role="status"></div>
            <span>Analyzing...</span>
          </div>

          <!-- Error -->
          <div v-else-if="error" class="popup-error">
            <i class="bi bi-exclamation-triangle text-warning me-2"></i>
            <span>{{ error }}</span>
          </div>

          <!-- Content -->
          <div v-else-if="analysis" class="popup-body">
            <!-- Forward Direction -->
            <div class="direction-section">
              <div class="section-label">
                <i class="bi bi-arrow-right me-1"></i>
                <span>{{ analysis.agent_name || 'Source' }} → {{ analysis.target || 'Target' }}</span>
              </div>
              <div class="direction-header-row">
                <span class="health-score-large" :style="{ color: healthScoreColor }">{{ healthScore }}</span>
                <span class="grade-badge" :style="{ background: healthColor.bg, color: healthColor.text }">{{ healthGrade }}</span>
              </div>
              <div class="metrics-row">
                <div class="metric-item">
                  <span class="metric-val">{{ formatMs(analysis.metrics.avg_latency) }}</span>
                  <span class="metric-lbl">Avg</span>
                </div>
                <div class="metric-item">
                  <span class="metric-val">{{ formatMs(analysis.metrics.p95_latency) }}</span>
                  <span class="metric-lbl">P95</span>
                </div>
                <div class="metric-item">
                  <span class="metric-val">{{ analysis.metrics.packet_loss.toFixed(1) }}%</span>
                  <span class="metric-lbl">Loss</span>
                </div>
                <div class="metric-item">
                  <span class="metric-val">{{ formatMs(analysis.metrics.jitter_avg) }}</span>
                  <span class="metric-lbl">Jitter</span>
                </div>
                <div class="metric-item">
                  <span class="metric-val" :style="{ color: gradeColors[analysis.health.grade]?.text }">{{ formatMos(analysis.health.mos_score) }}</span>
                  <span class="metric-lbl">MOS</span>
                </div>
              </div>
              <div class="health-bars">
                <div class="hbr-row">
                  <span class="hbr-lbl">Latency</span>
                  <div class="hbr-track"><div class="hbr-fill" :style="{ width: healthBarWidth(analysis.health.latency_score), background: healthColor.border }"></div></div>
                  <span class="hbr-val">{{ Math.round(analysis.health.latency_score) }}</span>
                </div>
                <div class="hbr-row">
                  <span class="hbr-lbl">Loss</span>
                  <div class="hbr-track"><div class="hbr-fill" :style="{ width: healthBarWidth(analysis.health.packet_loss_score), background: healthColor.border }"></div></div>
                  <span class="hbr-val">{{ Math.round(analysis.health.packet_loss_score) }}</span>
                </div>
                <div class="hbr-row">
                  <span class="hbr-lbl">Stability</span>
                  <div class="hbr-track"><div class="hbr-fill" :style="{ width: healthBarWidth(analysis.health.route_stability), background: healthColor.border }"></div></div>
                  <span class="hbr-val">{{ Math.round(analysis.health.route_stability) }}</span>
                </div>
              </div>
              <div v-if="analysis.path_analysis?.latest_hops_detail?.length" class="path-hops">
                <div class="path-hops-header">
                  <i class="bi bi-signpost-split me-1"></i>
                  <span>Hops ({{ analysis.path_analysis.hop_count }})</span>
                  <span class="text-muted ms-auto me-1" style="font-size:10px">{{ analysis.path_analysis.unique_routes }} route{{ analysis.path_analysis.unique_routes !== 1 ? 's' : '' }}</span>
                </div>
                <div class="hops-list">
                  <div v-for="(hop, idx) in analysis.path_analysis.latest_hops_detail.slice(0, 6)" :key="idx" class="hop-item">
                    <span class="hop-num">{{ idx + 1 }}</span>
                    <span class="hop-ip">{{ hop.hostname || hop.ip }}</span>
                    <span v-if="hop.latency" class="hop-lat">{{ formatMs(hop.latency) }}</span>
                    <span v-if="hop.loss" class="hop-loss" :class="{ 'text-danger': hop.loss > 0 }">{{ hop.loss.toFixed(1) }}%</span>
                    <span v-if="hop.is_agent" class="hop-agent-badge">agent</span>
                  </div>
                  <div v-if="analysis.path_analysis.latest_hops_detail.length > 6" class="hops-more">
                    +{{ analysis.path_analysis.latest_hops_detail.length - 6 }} more hops
                  </div>
                </div>
              </div>
            </div>

            <!-- Return Path (Reverse Direction) -->
            <div v-if="analysis.reverse" class="direction-section reverse-section">
              <div class="section-label">
                <i class="bi bi-arrow-left me-1"></i>
                <span>{{ analysis.reverse.agent_name }} → {{ analysis.reverse.target }}</span>
                <span class="return-tag">return</span>
              </div>
              <div class="direction-header-row">
                <span class="health-score-large" :style="{ color: reverseHealthColor.text }">{{ reverseHealthScore }}</span>
                <span class="grade-badge" :style="{ background: reverseHealthColor.bg, color: reverseHealthColor.text }">{{ reverseHealthGrade }}</span>
              </div>
              <div class="metrics-row">
                <div class="metric-item">
                  <span class="metric-val">{{ formatMs(analysis.reverse.metrics.avg_latency) }}</span>
                  <span class="metric-lbl">Avg</span>
                </div>
                <div class="metric-item">
                  <span class="metric-val">{{ formatMs(analysis.reverse.metrics.p95_latency) }}</span>
                  <span class="metric-lbl">P95</span>
                </div>
                <div class="metric-item">
                  <span class="metric-val">{{ analysis.reverse.metrics.packet_loss.toFixed(1) }}%</span>
                  <span class="metric-lbl">Loss</span>
                </div>
                <div class="metric-item">
                  <span class="metric-val">{{ formatMs(analysis.reverse.metrics.jitter_avg) }}</span>
                  <span class="metric-lbl">Jitter</span>
                </div>
                <div class="metric-item">
                  <span class="metric-val" :style="{ color: gradeColors[analysis.reverse.health.grade]?.text }">{{ formatMos(analysis.reverse.health.mos_score) }}</span>
                  <span class="metric-lbl">MOS</span>
                </div>
              </div>
              <div class="health-bars">
                <div class="hbr-row">
                  <span class="hbr-lbl">Latency</span>
                  <div class="hbr-track"><div class="hbr-fill" :style="{ width: healthBarWidth(analysis.reverse.health.latency_score), background: reverseHealthColor.border }"></div></div>
                  <span class="hbr-val">{{ Math.round(analysis.reverse.health.latency_score) }}</span>
                </div>
                <div class="hbr-row">
                  <span class="hbr-lbl">Loss</span>
                  <div class="hbr-track"><div class="hbr-fill" :style="{ width: healthBarWidth(analysis.reverse.health.packet_loss_score), background: reverseHealthColor.border }"></div></div>
                  <span class="hbr-val">{{ Math.round(analysis.reverse.health.packet_loss_score) }}</span>
                </div>
                <div class="hbr-row">
                  <span class="hbr-lbl">Stability</span>
                  <div class="hbr-track"><div class="hbr-fill" :style="{ width: healthBarWidth(analysis.reverse.health.route_stability), background: reverseHealthColor.border }"></div></div>
                  <span class="hbr-val">{{ Math.round(analysis.reverse.health.route_stability) }}</span>
                </div>
              </div>
              <div v-if="analysis.reverse.path_analysis?.latest_hops_detail?.length" class="path-hops">
                <div class="path-hops-header">
                  <i class="bi bi-signpost-split me-1"></i>
                  <span>Hops ({{ analysis.reverse.path_analysis.hop_count }})</span>
                  <span class="text-muted ms-auto me-1" style="font-size:10px">{{ analysis.reverse.path_analysis.unique_routes }} route{{ analysis.reverse.path_analysis.unique_routes !== 1 ? 's' : '' }}</span>
                </div>
                <div class="hops-list">
                  <div v-for="(hop, idx) in analysis.reverse.path_analysis.latest_hops_detail.slice(0, 6)" :key="idx" class="hop-item">
                    <span class="hop-num">{{ idx + 1 }}</span>
                    <span class="hop-ip">{{ hop.hostname || hop.ip }}</span>
                    <span v-if="hop.latency" class="hop-lat">{{ formatMs(hop.latency) }}</span>
                    <span v-if="hop.loss" class="hop-loss" :class="{ 'text-danger': hop.loss > 0 }">{{ hop.loss.toFixed(1) }}%</span>
                    <span v-if="hop.is_agent" class="hop-agent-badge">agent</span>
                  </div>
                  <div v-if="analysis.reverse.path_analysis.latest_hops_detail.length > 6" class="hops-more">
                    +{{ analysis.reverse.path_analysis.latest_hops_detail.length - 6 }} more hops
                  </div>
                </div>
              </div>
            </div>

            <!-- Signals & Findings -->
            <div v-if="analysis.signals?.length || analysis.findings?.length" class="signals-findings-section">
              <div v-if="analysis.signals?.length" class="signals-block">
                <div class="block-header">
                  <i class="bi bi-broadcast me-1"></i>
                  <span>{{ analysis.signals.length }} Signal{{ analysis.signals.length > 1 ? 's' : '' }}</span>
                </div>
                <div class="signal-list">
                  <div v-for="sig in analysis.signals.slice(0, 4)" :key="sig.type" class="signal-item" :class="sig.severity">
                    <span class="signal-type">{{ sig.type.replace(/_/g, ' ') }}</span>
                    <span class="signal-title">{{ sig.title }}</span>
                  </div>
                </div>
              </div>
              <div v-if="analysis.findings?.length" class="findings-block">
                <div class="block-header">
                  <i class="bi bi-lightbulb me-1"></i>
                  <span>{{ analysis.findings.length }} Finding{{ analysis.findings.length > 1 ? 's' : '' }}</span>
                </div>
                <div class="finding-list">
                  <div v-for="f in analysis.findings.slice(0, 3)" :key="f.id" class="finding-item" :class="f.severity">
                    <span class="finding-title">{{ f.title }}</span>
                    <span v-if="f.recommended_steps?.length" class="finding-step">{{ f.recommended_steps[0] }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Footer -->
          <div class="popup-footer">
            <span class="text-muted small">
              <i class="bi bi-clock me-1"></i>
              Last hour analysis
            </span>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.probe-health-popup {
  display: inline;
}

.popup-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.3);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
  backdrop-filter: blur(2px);
}

.popup-content {
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 16px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.2);
  width: 340px;
  max-height: 90vh;
  overflow-y: auto;
  animation: popup-enter 0.2s ease-out;
}

@keyframes popup-enter {
  from {
    opacity: 0;
    transform: scale(0.95) translateY(-10px);
  }
  to {
    opacity: 1;
    transform: scale(1) translateY(0);
  }
}

.popup-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  border-bottom: 1px solid var(--bs-border-color);
  background: var(--bs-tertiary-bg);
  border-radius: 16px 16px 0 0;
}

.popup-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 14px;
  color: var(--bs-body-color);
}

.popup-subtitle {
  font-size: 11px;
  color: var(--bs-secondary-color);
  font-weight: 400;
}

.popup-close {
  background: none;
  border: none;
  padding: 4px 8px;
  cursor: pointer;
  color: var(--bs-secondary-color);
  font-size: 14px;
  border-radius: 4px;
  transition: background 0.15s;
}

.popup-close:hover {
  background: var(--bs-tertiary-bg);
  color: var(--bs-body-color);
}

.popup-loading,
.popup-error {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 24px;
  color: var(--bs-secondary-color);
}

.popup-body {
  padding: 16px;
}

.popup-footer {
  padding: 10px 16px;
  border-top: 1px solid var(--bs-border-color);
  background: var(--bs-tertiary-bg);
  border-radius: 0 0 16px 16px;
}

/* Health Score Circle */
.health-score-section {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  margin-bottom: 16px;
}

.health-circle {
  width: 80px;
  height: 80px;
  border-radius: 50%;
  border: 4px solid;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  background: var(--bs-body-bg);
}

.health-score-value {
  font-size: 24px;
  font-weight: 700;
  line-height: 1;
}

.health-score-label {
  font-size: 9px;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-top: 4px;
}

.health-grade-badge {
  font-size: 12px;
  font-weight: 600;
  padding: 6px 14px;
  border-radius: 20px;
  text-transform: capitalize;
}

/* Direction Section */
.direction-section {
  padding: 12px;
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  margin-bottom: 10px;
  background: var(--bs-secondary-bg);
}
.direction-section.reverse-section {
  border-color: var(--bs-primary);
  background: rgba(var(--bs-primary-rgb), 0.04);
}
.section-label {
  font-size: 11px;
  color: var(--bs-secondary-color);
  font-weight: 500;
  display: flex;
  align-items: center;
  margin-bottom: 6px;
}
.direction-header-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}
.health-score-large {
  font-size: 28px;
  font-weight: 700;
  font-family: 'SF Mono', 'Fira Code', monospace;
  line-height: 1;
}
.grade-badge {
  font-size: 11px;
  font-weight: 600;
  padding: 3px 10px;
  border-radius: 12px;
  text-transform: capitalize;
}
.return-tag {
  font-size: 9px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: 4px;
  background: rgba(var(--bs-primary-rgb), 0.15);
  color: var(--bs-primary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

/* Metrics Row */
.metrics-row {
  display: flex;
  gap: 4px;
  margin-bottom: 10px;
}
.metric-item {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 6px 2px;
  background: var(--bs-tertiary-bg);
  border-radius: 6px;
}
.metric-val {
  font-size: 12px;
  font-weight: 700;
  font-family: 'SF Mono', 'Fira Code', monospace;
  color: var(--bs-body-color);
}
.metric-lbl {
  font-size: 8px;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.3px;
  margin-top: 2px;
}

/* Health Bars */
.health-bars {
  display: flex;
  flex-direction: column;
  gap: 5px;
  margin-bottom: 8px;
}
.hbr-row {
  display: flex;
  align-items: center;
  gap: 6px;
}
.hbr-lbl {
  font-size: 10px;
  color: var(--bs-secondary-color);
  width: 48px;
  flex-shrink: 0;
  font-weight: 500;
}
.hbr-track {
  flex: 1;
  height: 5px;
  background: var(--bs-tertiary-bg);
  border-radius: 3px;
  overflow: hidden;
}
.hbr-fill {
  height: 100%;
  border-radius: 3px;
  transition: width 0.4s ease;
}
.hbr-val {
  font-size: 10px;
  font-weight: 600;
  width: 24px;
  text-align: right;
  color: var(--bs-body-color);
}

/* Path Hops */
.path-hops {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--bs-border-color);
}
.path-hops-header {
  display: flex;
  align-items: center;
  font-size: 10px;
  font-weight: 600;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 5px;
}
.hops-list {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.hop-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 10px;
  padding: 2px 4px;
  background: var(--bs-tertiary-bg);
  border-radius: 4px;
}
.hop-num {
  width: 14px;
  height: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bs-secondary-bg);
  border-radius: 50%;
  font-size: 8px;
  font-weight: 600;
  color: var(--bs-secondary-color);
  flex-shrink: 0;
}
.hop-ip {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: 'SF Mono', 'Fira Code', monospace;
}
.hop-lat {
  font-family: 'SF Mono', 'Fira Code', monospace;
  color: var(--bs-secondary-color);
}
.hop-loss {
  font-family: 'SF Mono', 'Fira Code', monospace;
}
.hop-agent-badge {
  font-size: 8px;
  font-weight: 600;
  padding: 1px 4px;
  border-radius: 3px;
  background: rgba(var(--bs-primary-rgb), 0.15);
  color: var(--bs-primary);
  text-transform: uppercase;
}
.hops-more {
  font-size: 9px;
  color: var(--bs-secondary-color);
  text-align: center;
  padding: 3px;
}

/* Signals & Findings */
.signals-findings-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--bs-border-color);
}
.signals-block, .findings-block {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.block-header {
  display: flex;
  align-items: center;
  font-size: 10px;
  font-weight: 600;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.signal-list, .finding-list {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.signal-item, .finding-item {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 5px 8px;
  border-radius: 6px;
  background: var(--bs-tertiary-bg);
}
.signal-item.warning { background: rgba(var(--bs-warning-rgb), 0.1); }
.signal-item.critical { background: rgba(var(--bs-danger-rgb), 0.1); }
.signal-item.info { background: rgba(var(--bs-info-rgb), 0.1); }
.signal-type {
  font-size: 9px;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--bs-secondary-color);
  letter-spacing: 0.3px;
}
.signal-title {
  font-size: 11px;
  color: var(--bs-body-color);
}
.finding-item.warning { background: rgba(var(--bs-warning-rgb), 0.1); }
.finding-item.critical { background: rgba(var(--bs-danger-rgb), 0.1); }
.finding-item.info { background: rgba(var(--bs-info-rgb), 0.1); }
.finding-title {
  font-size: 11px;
  color: var(--bs-body-color);
}
.finding-step {
  font-size: 9px;
  color: var(--bs-secondary-color);
}

/* Dark theme */
[data-theme="dark"] .popup-content {
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
}
</style>