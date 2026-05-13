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
const loading = ref(true)
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
            <!-- Direction Cards for Forward and Reverse -->
            <div class="direction-cards-mini">
              <!-- Forward Direction -->
              <div class="direction-card-mini" :style="{ borderColor: healthColor.border }">
                <div class="direction-label-mini">
                  <i class="bi bi-arrow-right me-1"></i>
                  <span>{{ analysis.agent_name || 'Source' }} → {{ analysis.target || 'Target' }}</span>
                </div>
                <div class="direction-score-mini">
                  <span class="score-value-mini" :style="{ color: healthScoreColor }">{{ healthScore }}</span>
                  <span class="grade-mini" :style="{ background: healthColor.bg, color: healthColor.text }">{{ healthGrade }}</span>
                </div>
              </div>

              <!-- Reverse Direction -->
              <div v-if="hasBidirectional && analysis.reverse" class="direction-card-mini" :style="{ borderColor: reverseHealthColor.border }">
                <div class="direction-label-mini">
                  <i class="bi bi-arrow-left me-1"></i>
                  <span>{{ analysis.reverse.agent_name }} → {{ analysis.reverse.target }}</span>
                </div>
                <div class="direction-score-mini">
                  <span class="score-value-mini" :style="{ color: reverseHealthColor.text }">{{ reverseHealthScore }}</span>
                  <span class="grade-mini" :style="{ background: reverseHealthColor.bg, color: reverseHealthColor.text }">{{ reverseHealthGrade }}</span>
                </div>
              </div>
            </div>

            <!-- Metrics Grid -->
            <div class="metrics-mini-grid">
              <div class="mini-metric">
                <div class="mini-metric-value">{{ formatMs(analysis.metrics.avg_latency) }}</div>
                <div class="mini-metric-label">Avg Latency</div>
              </div>
              <div class="mini-metric">
                <div class="mini-metric-value">{{ analysis.metrics.packet_loss.toFixed(2) }}%</div>
                <div class="mini-metric-label">Packet Loss</div>
              </div>
              <div class="mini-metric">
                <div class="mini-metric-value">{{ formatMs(analysis.metrics.jitter) }}</div>
                <div class="mini-metric-label">Jitter</div>
              </div>
              <div class="mini-metric">
                <div class="mini-metric-value" :style="{ color: gradeColors[analysis.health.grade]?.text }">
                  {{ formatMos(analysis.health.mos_score) }}
                </div>
                <div class="mini-metric-label">MOS</div>
              </div>
            </div>

            <!-- Health Vector Bars -->
            <div class="health-vectors-mini">
              <div class="vector-row">
                <span class="vector-label">Latency</span>
                <div class="vector-bar-track">
                  <div class="vector-bar-fill" :style="{ width: healthBarWidth(analysis.health.latency_score), background: healthColor.border }"></div>
                </div>
                <span class="vector-score">{{ Math.round(analysis.health.latency_score) }}</span>
              </div>
              <div class="vector-row">
                <span class="vector-label">Loss</span>
                <div class="vector-bar-track">
                  <div class="vector-bar-fill" :style="{ width: healthBarWidth(analysis.health.packet_loss_score), background: healthColor.border }"></div>
                </div>
                <span class="vector-score">{{ Math.round(analysis.health.packet_loss_score) }}</span>
              </div>
              <div class="vector-row">
                <span class="vector-label">Stability</span>
                <div class="vector-bar-track">
                  <div class="vector-bar-fill" :style="{ width: healthBarWidth(analysis.health.route_stability), background: healthColor.border }"></div>
                </div>
                <span class="vector-score">{{ Math.round(analysis.health.route_stability) }}</span>
              </div>
            </div>

            <!-- Signals Summary -->
            <div v-if="analysis.signals?.length" class="signals-summary">
              <div class="signals-title">
                <i class="bi bi-broadcast me-1"></i>
                <span>{{ analysis.signals.length }} Signal{{ analysis.signals.length > 1 ? 's' : '' }}</span>
              </div>
              <div class="signal-badges">
                <span v-for="signal in analysis.signals.slice(0, 3)" :key="signal.type" 
                      class="signal-badge" :class="signal.severity">
                  {{ signal.type.replace(/_/g, ' ') }}
                </span>
                <span v-if="analysis.signals.length > 3" class="signal-badge more">
                  +{{ analysis.signals.length - 3 }} more
                </span>
              </div>
            </div>

            <!-- Bidirectional Info -->
            <div v-if="analysis.reverse" class="reverse-indicator">
              <i class="bi bi-arrow-left-right"></i>
              <span>Bidirectional probe detected</span>
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

/* Direction Cards Mini */
.direction-cards-mini {
  display: flex;
  gap: 10px;
  margin-bottom: 16px;
}

.direction-card-mini {
  flex: 1;
  border: 1px solid;
  border-radius: 10px;
  padding: 10px 12px;
  background: var(--bs-secondary-bg);
}

.direction-label-mini {
  font-size: 11px;
  color: var(--bs-secondary-color);
  display: flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 6px;
}

.direction-score-mini {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.score-value-mini {
  font-size: 18px;
  font-weight: 700;
  font-family: 'SF Mono', 'Fira Code', monospace;
}

.grade-mini {
  font-size: 10px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 8px;
  text-transform: capitalize;
}

/* Metrics Mini Grid */
.metrics-mini-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
  margin-bottom: 16px;
}

.mini-metric {
  text-align: center;
  background: var(--bs-tertiary-bg);
  padding: 10px 6px;
  border-radius: 8px;
}

.mini-metric-value {
  font-size: 13px;
  font-weight: 700;
  color: var(--bs-body-color);
  font-family: 'SF Mono', 'Fira Code', monospace;
}

.mini-metric-label {
  font-size: 9px;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.3px;
  margin-top: 2px;
}

/* Health Vectors Mini */
.health-vectors-mini {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 14px;
}

.vector-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.vector-label {
  font-size: 11px;
  color: var(--bs-secondary-color);
  width: 55px;
  flex-shrink: 0;
  font-weight: 500;
}

.vector-bar-track {
  flex: 1;
  height: 6px;
  background: var(--bs-tertiary-bg);
  border-radius: 3px;
  overflow: hidden;
}

.vector-bar-fill {
  height: 100%;
  border-radius: 3px;
  transition: width 0.4s ease;
}

.vector-score {
  font-size: 11px;
  font-weight: 600;
  width: 28px;
  text-align: right;
  color: var(--bs-body-color);
}

/* Signals Summary */
.signals-summary {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--bs-border-color);
}

.signals-title {
  font-size: 11px;
  color: var(--bs-secondary-color);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 8px;
  display: flex;
  align-items: center;
}

.signal-badges {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.signal-badge {
  font-size: 10px;
  padding: 3px 8px;
  border-radius: 6px;
  background: var(--bs-tertiary-bg);
  color: var(--bs-secondary-color);
  font-weight: 500;
  text-transform: capitalize;
}

.signal-badge.warning {
  background: rgba(var(--bs-warning-rgb), 0.15);
  color: var(--bs-warning);
}

.signal-badge.critical {
  background: rgba(var(--bs-danger-rgb), 0.15);
  color: var(--bs-danger);
}

.signal-badge.info {
  background: rgba(var(--bs-info-rgb), 0.15);
  color: var(--bs-info);
}

.signal-badge.more {
  background: var(--bs-secondary-bg);
  color: var(--bs-secondary-color);
}

/* Reverse Indicator */
.reverse-indicator {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 12px;
  padding: 8px 12px;
  background: rgba(var(--bs-primary-rgb), 0.1);
  border-radius: 8px;
  font-size: 11px;
  color: var(--bs-primary);
  font-weight: 500;
}

.reverse-indicator i {
  font-size: 14px;
}

/* Dark theme adjustments */
[data-theme="dark"] .popup-content {
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
}

[data-theme="dark"] .health-circle {
  background: var(--bs-tertiary-bg);
}
</style>