<script lang="ts" setup>
import { ref, computed, onMounted } from 'vue'
import { ProbeDataService } from '@/services/apiService'
import type { ProbeAnalysis, HealthVector } from './types'
import { gradeColors } from './types'
import type { TrafficSimResult } from '@/types'

interface Props {
  probeId: number | string
  workspaceId: number | string
  trigger?: 'hover' | 'click'
  agentName?: string
  target?: string
  // When provided, summary metrics are derived from this TrafficSim data
  // (filtered to currentTimeRange if given) instead of the analysis API.
  // Use this on probe detail pages where the rich jitter/latency values
  // (jitterMedian, jitterP95, p95RTT, p99RTT, MOS) are already in scope.
  trafficSimData?: TrafficSimResult[]
  // Optional return-path data — when present, the popup shows a second
  // quick-glance section for the reverse direction in the same window.
  reverseTrafficSimData?: TrafficSimResult[]
  reverseAgentName?: string
  reverseTarget?: string
  currentTimeRange?: [Date, Date] | null
  // Emitted when the user clicks the "View Detailed Analysis" footer link.
  // Hosts that own a deeper analysis modal can wire this to open it.
}

const props = withDefaults(defineProps<Props>(), {
  trigger: 'hover',
  trafficSimData: undefined,
  reverseTrafficSimData: undefined,
  currentTimeRange: null
})

const emit = defineEmits<{
  (e: 'view-detail', probeId: number | string): void
}>()

const analysis = ref<ProbeAnalysis | null>(null)
const loading = ref(false)
const error = ref('')
const isVisible = ref(false)

async function fetchAnalysis() {
  // Always fetch: even when local TrafficSim rows drive the metric
  // display, the health score/grade must come from the backend's
  // computeHealthVector — the popup previously scored client-side
  // with a different curve and disagreed with the detailed modal.
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

// ── TrafficSim-derived summary ─────────────────────────────────────────
// Same aggregation approach as TrafficSimGraph.vue so the popup stays
// consistent with what the user sees in the chart.
function lossPercent(d: { lostPackets: number; totalPackets: number }): number {
  return d.totalPackets > 0 ? (d.lostPackets / d.totalPackets) * 100 : 0
}

function avgOf(vals: number[]): number {
  return vals.length > 0 ? vals.reduce((a, b) => a + b, 0) / vals.length : 0
}

function percentileOf(vals: number[], p: number): number {
  if (vals.length === 0) return 0
  const sorted = [...vals].sort((a, b) => a - b)
  const idx = (p / 100) * (sorted.length - 1)
  const lo = Math.floor(idx)
  const hi = Math.ceil(idx)
  if (lo === hi) return sorted[lo]
  return sorted[lo] + (sorted[hi] - sorted[lo]) * (idx - lo)
}

function filterByTimeRange(rows: TrafficSimResult[] | undefined): TrafficSimResult[] {
  if (!rows || rows.length === 0) return []
  if (!props.currentTimeRange) return rows
  const [from, to] = props.currentTimeRange
  if (!from || !to) return rows
  const fromMs = from.getTime()
  const toMs = to.getTime()
  return rows.filter(d => {
    const t = new Date(d.reportTime).getTime()
    return t >= fromMs && t <= toMs
  })
}

// Single source of truth for a direction's summary — used by both the
// forward and reverse direction sections so they stay in lockstep.
function buildSummary(rows: TrafficSimResult[]) {
  if (rows.length === 0) return null
  const latencies = rows.map(d => d.averageRTT).filter(v => v > 0)
  const medianRTTs = rows.map(d => d.medianRTT).filter((v): v is number => v != null && v > 0)
  const p95RTTs = rows.map(d => d.p95RTT).filter((v): v is number => v != null && v > 0)
  const jitterAvgs = rows.map(d => d.jitterAvg).filter((v): v is number => v != null && v > 0)
  const jitterMedians = rows.map(d => d.jitterMedian).filter((v): v is number => v != null && v > 0)
  const jitterP95s = rows.map(d => d.jitterP95).filter((v): v is number => v != null && v > 0)
  const mosVals = rows.map(d => d.mos ?? d.mosScore).filter((v): v is number => v != null && v > 0)
  const losses = rows.map(lossPercent)

  const avgLat = avgOf(latencies)
  const avgLoss = avgOf(losses)
  const avgJitter = avgOf(jitterAvgs)
  const latencyScore = avgLat <= 30 ? 100 : avgLat <= 80 ? 90 : avgLat <= 150 ? 75 : avgLat <= 300 ? 50 : 20
  const lossScore = avgLoss <= 0.5 ? 100 : avgLoss <= 1 ? 80 : avgLoss <= 3 ? 60 : 30
  const jitterScore = avgJitter <= 5 ? 100 : avgJitter <= 15 ? 80 : avgJitter <= 30 ? 60 : 30
  const overall = Math.round(latencyScore * 0.4 + lossScore * 0.4 + jitterScore * 0.2)
  const grade = overall >= 90 ? 'excellent' : overall >= 70 ? 'good' : overall >= 50 ? 'fair' : overall >= 30 ? 'poor' : 'critical'

  return {
    sampleCount: rows.length,
    currentRtt: latencies.length > 0 ? latencies[latencies.length - 1] : 0,
    avgLatency: avgLat,
    medianLatency: medianRTTs.length > 0 ? avgOf(medianRTTs) : percentileOf(latencies, 50),
    p95Latency: p95RTTs.length > 0 ? avgOf(p95RTTs) : percentileOf(latencies, 95),
    p99Latency: avgOf(rows.map(d => d.p99RTT).filter((v): v is number => v != null && v > 0)),
    avgJitter,
    medianJitter: jitterMedians.length > 0 ? avgOf(jitterMedians) : 0,
    p95Jitter: jitterP95s.length > 0 ? avgOf(jitterP95s) : 0,
    avgLoss,
    avgMos: avgOf(mosVals),
    healthLatencyScore: latencyScore,
    healthLossScore: lossScore,
    healthJitterScore: jitterScore,
    healthOverall: overall,
    healthGrade: grade,
  }
}

const forwardSummary = computed(() => buildSummary(filterByTimeRange(props.trafficSimData)))
const reverseSummary = computed(() => buildSummary(filterByTimeRange(props.reverseTrafficSimData)))

// Map a direction's data into a uniform "view" object for the template.
// Source can be: forward/reverse from API, or forward/reverse from local
// TrafficSim aggregation.
interface DirectionView {
  source: 'traffic' | 'api'
  agentName: string
  target: string
  grade: string
  healthScore: number
  healthColorText: string
  healthColorBg: string
  healthColorBorder: string
  sampleCount: number
  avgLatency: number
  medianLatency: number
  p95Latency: number
  avgJitter: number
  medianJitter: number
  p95Jitter: number
  packetLoss: number
  mos: number
  healthLatencyScore: number
  healthLossScore: number
  healthJitterScore: number
  // API-mode extras
  pathAnalysis?: ProbeAnalysis['path_analysis']
  signals?: ProbeAnalysis['signals']
}

function buildView(args: {
  source: 'traffic' | 'api'
  agentName: string
  target: string
  summary: ReturnType<typeof buildSummary>
  api?: ProbeAnalysis
  // Authoritative health from the analysis API. When present it
  // overrides the client-derived score so the popup always matches
  // the detailed analysis view.
  apiHealth?: HealthVector
}): DirectionView | null {
  if (args.source === 'traffic') {
    if (!args.summary) return null
    const useApi = !!args.apiHealth && args.apiHealth.grade !== '' && args.apiHealth.grade !== 'unknown'
    const grade = useApi ? args.apiHealth!.grade : args.summary.healthGrade
    return {
      source: 'traffic',
      agentName: args.agentName,
      target: args.target,
      grade,
      healthScore: useApi ? Math.round(args.apiHealth!.overall_health) : args.summary.healthOverall,
      healthColorText: (gradeColors[grade] || gradeColors.unknown).text,
      healthColorBg: (gradeColors[grade] || gradeColors.unknown).bg,
      healthColorBorder: (gradeColors[grade] || gradeColors.unknown).border,
      sampleCount: args.summary.sampleCount,
      avgLatency: args.summary.avgLatency,
      medianLatency: args.summary.medianLatency,
      p95Latency: args.summary.p95Latency,
      avgJitter: args.summary.avgJitter,
      medianJitter: args.summary.medianJitter,
      p95Jitter: args.summary.p95Jitter,
      packetLoss: args.summary.avgLoss,
      mos: args.summary.avgMos,
      healthLatencyScore: useApi ? args.apiHealth!.latency_score : args.summary.healthLatencyScore,
      healthLossScore: useApi ? args.apiHealth!.packet_loss_score : args.summary.healthLossScore,
      healthJitterScore: args.summary.healthJitterScore,
    }
  }
  if (!args.api) return null
  return {
    source: 'api',
    agentName: args.api.agent_name || args.agentName,
    target: args.api.target || args.target,
    grade: args.api.health.grade,
    healthScore: Math.round(args.api.health.overall_health),
    healthColorText: (gradeColors[args.api.health.grade] || gradeColors.unknown).text,
    healthColorBg: (gradeColors[args.api.health.grade] || gradeColors.unknown).bg,
    healthColorBorder: (gradeColors[args.api.health.grade] || gradeColors.unknown).border,
    sampleCount: args.api.metrics.sample_count,
    avgLatency: args.api.metrics.avg_latency,
    medianLatency: args.api.metrics.median_latency ?? 0,
    p95Latency: args.api.metrics.p95_latency,
    avgJitter: args.api.metrics.jitter_avg,
    medianJitter: args.api.metrics.jitter_median ?? 0,
    p95Jitter: args.api.metrics.jitter_p95 ?? 0,
    packetLoss: args.api.metrics.packet_loss,
    mos: args.api.health.mos_score,
    healthLatencyScore: args.api.health.latency_score,
    healthLossScore: args.api.health.packet_loss_score,
    healthJitterScore: 0,
    pathAnalysis: args.api.path_analysis,
    signals: args.api.signals,
  }
}

const forwardView = computed<DirectionView | null>(() => {
  if (props.trafficSimData) {
    return buildView({
      source: 'traffic',
      agentName: props.agentName || 'Source',
      target: props.target || 'Target',
      summary: forwardSummary.value,
      apiHealth: analysis.value?.health,
    })
  }
  if (analysis.value) {
    return buildView({
      source: 'api',
      agentName: props.agentName || 'Source',
      target: props.target || 'Target',
      summary: null,
      api: analysis.value,
    })
  }
  return null
})

const reverseView = computed<DirectionView | null>(() => {
  if (props.reverseTrafficSimData) {
    return buildView({
      source: 'traffic',
      agentName: props.reverseAgentName || props.agentName || 'Source',
      target: props.reverseTarget || props.target || 'Target',
      summary: reverseSummary.value,
      apiHealth: analysis.value?.reverse?.health,
    })
  }
  if (analysis.value?.reverse) {
    return buildView({
      source: 'api',
      agentName: props.reverseAgentName || analysis.value.reverse.agent_name,
      target: props.reverseTarget || analysis.value.reverse.target,
      summary: null,
      api: analysis.value.reverse,
    })
  }
  return null
})

// API-mode aggregated signals (forward + reverse mixed). Only shown in API mode
// since the TrafficSim path doesn't produce per-direction signals.
const apiSignals = computed(() => analysis.value?.signals || [])
const apiFindings = computed(() => analysis.value?.findings || [])

function showPopup() {
  isVisible.value = true
  if (!analysis.value && !loading.value) {
    fetchAnalysis()
  }
}

function hidePopup() {
  isVisible.value = false
}

function openDetail() {
  isVisible.value = false
  emit('view-detail', props.probeId)
}

function formatMs(ms: number) {
  return ms > 0 ? `${ms.toFixed(1)}ms` : '—'
}

function formatMos(mos: number) {
  return mos > 0 ? mos.toFixed(2) : '—'
}

function healthBarWidth(score: number) {
  return `${Math.max(0, Math.min(100, score))}%`
}

onMounted(() => {
  if (props.trigger === 'hover' && !analysis.value) {
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
          <div v-else-if="forwardView || reverseView" class="popup-body">
            <!-- Forward + Reverse: side-by-side when both fit, stacked otherwise -->
            <div class="direction-pair" :class="{ 'direction-pair-stacked': forwardView && reverseView }">
              <!-- Forward Direction -->
              <div v-if="forwardView" class="direction-section">
                <div class="section-label">
                  <i class="bi bi-arrow-right me-1"></i>
                  <span>
                    {{ forwardView.agentName }} → {{ forwardView.target }}
                    <span v-if="forwardView.source === 'traffic'" class="traffic-tag">trafficsim</span>
                  </span>
                </div>
                <div class="direction-header-row">
                  <span class="health-score-large" :style="{ color: forwardView.healthColorText }">{{ forwardView.healthScore }}</span>
                  <span class="grade-badge" :style="{ background: forwardView.healthColorBg, color: forwardView.healthColorText }">{{ forwardView.grade }}</span>
                  <span class="sample-count" v-if="forwardView.sampleCount > 0">{{ forwardView.sampleCount }} samples</span>
                </div>

                <div class="metric-group">
                  <div class="metric-group-label">Latency</div>
                  <div class="metric-row">
                    <div class="metric-item">
                      <span class="metric-val">{{ formatMs(forwardView.avgLatency) }}</span>
                      <span class="metric-lbl">Avg</span>
                    </div>
                    <div class="metric-item">
                      <span class="metric-val">{{ formatMs(forwardView.medianLatency) }}</span>
                      <span class="metric-lbl">Med</span>
                    </div>
                    <div class="metric-item">
                      <span class="metric-val">{{ formatMs(forwardView.p95Latency) }}</span>
                      <span class="metric-lbl">P95</span>
                    </div>
                  </div>
                </div>

                <div class="metric-group">
                  <div class="metric-group-label">Jitter</div>
                  <div class="metric-row">
                    <div class="metric-item">
                      <span class="metric-val">{{ formatMs(forwardView.avgJitter) }}</span>
                      <span class="metric-lbl">Avg</span>
                    </div>
                    <div class="metric-item">
                      <span class="metric-val" :class="{ 'text-muted': forwardView.medianJitter <= 0 }">
                        {{ formatMs(forwardView.medianJitter) }}
                      </span>
                      <span class="metric-lbl">Med</span>
                    </div>
                    <div class="metric-item">
                      <span class="metric-val" :class="{ 'text-muted': forwardView.p95Jitter <= 0 }">
                        {{ formatMs(forwardView.p95Jitter) }}
                      </span>
                      <span class="metric-lbl">P95</span>
                    </div>
                  </div>
                </div>

                <div class="metric-row single">
                  <div class="metric-item">
                    <span class="metric-val" :class="{ 'text-danger': forwardView.packetLoss > 3 }">
                      {{ forwardView.packetLoss.toFixed(1) }}%
                    </span>
                    <span class="metric-lbl">Loss</span>
                  </div>
                  <div class="metric-item">
                    <span class="metric-val" :style="{ color: gradeColors[forwardView.grade]?.text }">
                      {{ formatMos(forwardView.mos) }}
                    </span>
                    <span class="metric-lbl">MOS</span>
                  </div>
                </div>

                <div class="health-bars">
                  <div class="hbr-row">
                    <span class="hbr-lbl">Latency</span>
                    <div class="hbr-track"><div class="hbr-fill" :style="{ width: healthBarWidth(forwardView.healthLatencyScore), background: forwardView.healthColorBorder }"></div></div>
                    <span class="hbr-val">{{ Math.round(forwardView.healthLatencyScore) }}</span>
                  </div>
                  <div class="hbr-row">
                    <span class="hbr-lbl">Loss</span>
                    <div class="hbr-track"><div class="hbr-fill" :style="{ width: healthBarWidth(forwardView.healthLossScore), background: forwardView.healthColorBorder }"></div></div>
                    <span class="hbr-val">{{ Math.round(forwardView.healthLossScore) }}</span>
                  </div>
                  <div class="hbr-row" v-if="forwardView.healthJitterScore > 0">
                    <span class="hbr-lbl">Jitter</span>
                    <div class="hbr-track"><div class="hbr-fill" :style="{ width: healthBarWidth(forwardView.healthJitterScore), background: forwardView.healthColorBorder }"></div></div>
                    <span class="hbr-val">{{ Math.round(forwardView.healthJitterScore) }}</span>
                  </div>
                </div>

                <!-- Hop list (API mode only) -->
                <div v-if="forwardView.pathAnalysis?.latest_hops_detail?.length" class="path-hops">
                  <div class="path-hops-header">
                    <i class="bi bi-signpost-split me-1"></i>
                    <span>Hops ({{ forwardView.pathAnalysis.hop_count }})</span>
                  </div>
                  <div class="hops-list">
                    <div v-for="(hop, idx) in forwardView.pathAnalysis.latest_hops_detail.slice(0, 5)" :key="idx" class="hop-item">
                      <span class="hop-num">{{ idx + 1 }}</span>
                      <span class="hop-ip">{{ hop.hostname || hop.ip }}</span>
                      <span v-if="hop.latency" class="hop-lat">{{ formatMs(hop.latency) }}</span>
                      <span v-if="hop.loss" class="hop-loss" :class="{ 'text-danger': hop.loss > 0 }">{{ hop.loss.toFixed(1) }}%</span>
                    </div>
                    <div v-if="forwardView.pathAnalysis.latest_hops_detail.length > 5" class="hops-more">
                      +{{ forwardView.pathAnalysis.latest_hops_detail.length - 5 }} more in detail
                    </div>
                  </div>
                </div>
              </div>

              <!-- Return / Reverse Direction -->
              <div v-if="reverseView" class="direction-section reverse-section">
                <div class="section-label">
                  <i class="bi bi-arrow-left me-1"></i>
                  <span>
                    {{ reverseView.agentName }} → {{ reverseView.target }}
                    <span v-if="reverseView.source === 'traffic'" class="traffic-tag">trafficsim</span>
                    <span v-else class="return-tag">return</span>
                  </span>
                </div>
                <div class="direction-header-row">
                  <span class="health-score-large" :style="{ color: reverseView.healthColorText }">{{ reverseView.healthScore }}</span>
                  <span class="grade-badge" :style="{ background: reverseView.healthColorBg, color: reverseView.healthColorText }">{{ reverseView.grade }}</span>
                  <span class="sample-count" v-if="reverseView.sampleCount > 0">{{ reverseView.sampleCount }} samples</span>
                </div>

                <div class="metric-group">
                  <div class="metric-group-label">Latency</div>
                  <div class="metric-row">
                    <div class="metric-item">
                      <span class="metric-val">{{ formatMs(reverseView.avgLatency) }}</span>
                      <span class="metric-lbl">Avg</span>
                    </div>
                    <div class="metric-item">
                      <span class="metric-val">{{ formatMs(reverseView.medianLatency) }}</span>
                      <span class="metric-lbl">Med</span>
                    </div>
                    <div class="metric-item">
                      <span class="metric-val">{{ formatMs(reverseView.p95Latency) }}</span>
                      <span class="metric-lbl">P95</span>
                    </div>
                  </div>
                </div>

                <div class="metric-group">
                  <div class="metric-group-label">Jitter</div>
                  <div class="metric-row">
                    <div class="metric-item">
                      <span class="metric-val">{{ formatMs(reverseView.avgJitter) }}</span>
                      <span class="metric-lbl">Avg</span>
                    </div>
                    <div class="metric-item">
                      <span class="metric-val" :class="{ 'text-muted': reverseView.medianJitter <= 0 }">
                        {{ formatMs(reverseView.medianJitter) }}
                      </span>
                      <span class="metric-lbl">Med</span>
                    </div>
                    <div class="metric-item">
                      <span class="metric-val" :class="{ 'text-muted': reverseView.p95Jitter <= 0 }">
                        {{ formatMs(reverseView.p95Jitter) }}
                      </span>
                      <span class="metric-lbl">P95</span>
                    </div>
                  </div>
                </div>

                <div class="metric-row single">
                  <div class="metric-item">
                    <span class="metric-val" :class="{ 'text-danger': reverseView.packetLoss > 3 }">
                      {{ reverseView.packetLoss.toFixed(1) }}%
                    </span>
                    <span class="metric-lbl">Loss</span>
                  </div>
                  <div class="metric-item">
                    <span class="metric-val" :style="{ color: gradeColors[reverseView.grade]?.text }">
                      {{ formatMos(reverseView.mos) }}
                    </span>
                    <span class="metric-lbl">MOS</span>
                  </div>
                </div>

                <div class="health-bars">
                  <div class="hbr-row">
                    <span class="hbr-lbl">Latency</span>
                    <div class="hbr-track"><div class="hbr-fill" :style="{ width: healthBarWidth(reverseView.healthLatencyScore), background: reverseView.healthColorBorder }"></div></div>
                    <span class="hbr-val">{{ Math.round(reverseView.healthLatencyScore) }}</span>
                  </div>
                  <div class="hbr-row">
                    <span class="hbr-lbl">Loss</span>
                    <div class="hbr-track"><div class="hbr-fill" :style="{ width: healthBarWidth(reverseView.healthLossScore), background: reverseView.healthColorBorder }"></div></div>
                    <span class="hbr-val">{{ Math.round(reverseView.healthLossScore) }}</span>
                  </div>
                  <div class="hbr-row" v-if="reverseView.healthJitterScore > 0">
                    <span class="hbr-lbl">Jitter</span>
                    <div class="hbr-track"><div class="hbr-fill" :style="{ width: healthBarWidth(reverseView.healthJitterScore), background: reverseView.healthColorBorder }"></div></div>
                    <span class="hbr-val">{{ Math.round(reverseView.healthJitterScore) }}</span>
                  </div>
                </div>

                <div v-if="reverseView.pathAnalysis?.latest_hops_detail?.length" class="path-hops">
                  <div class="path-hops-header">
                    <i class="bi bi-signpost-split me-1"></i>
                    <span>Hops ({{ reverseView.pathAnalysis.hop_count }})</span>
                  </div>
                  <div class="hops-list">
                    <div v-for="(hop, idx) in reverseView.pathAnalysis.latest_hops_detail.slice(0, 5)" :key="idx" class="hop-item">
                      <span class="hop-num">{{ idx + 1 }}</span>
                      <span class="hop-ip">{{ hop.hostname || hop.ip }}</span>
                      <span v-if="hop.latency" class="hop-lat">{{ formatMs(hop.latency) }}</span>
                      <span v-if="hop.loss" class="hop-loss" :class="{ 'text-danger': hop.loss > 0 }">{{ hop.loss.toFixed(1) }}%</span>
                    </div>
                    <div v-if="reverseView.pathAnalysis.latest_hops_detail.length > 5" class="hops-more">
                      +{{ reverseView.pathAnalysis.latest_hops_detail.length - 5 }} more in detail
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <!-- Aggregated signals (API mode only) -->
            <div v-if="(apiSignals.length || apiFindings.length) && forwardView?.source === 'api'" class="signals-findings-section">
              <div v-if="apiSignals.length" class="signals-block">
                <div class="block-header">
                  <i class="bi bi-broadcast me-1"></i>
                  <span>{{ apiSignals.length }} Signal{{ apiSignals.length > 1 ? 's' : '' }}</span>
                </div>
                <div class="signal-list">
                  <div v-for="sig in apiSignals.slice(0, 3)" :key="sig.type" class="signal-item" :class="sig.severity">
                    <span class="signal-type">{{ sig.type.replace(/_/g, ' ') }}</span>
                    <span class="signal-title">{{ sig.title }}</span>
                  </div>
                </div>
              </div>
              <div v-if="apiFindings.length" class="findings-block">
                <div class="block-header">
                  <i class="bi bi-lightbulb me-1"></i>
                  <span>{{ apiFindings.length }} Finding{{ apiFindings.length > 1 ? 's' : '' }}</span>
                </div>
                <div class="finding-list">
                  <div v-for="f in apiFindings.slice(0, 2)" :key="f.id" class="finding-item" :class="f.severity">
                    <span class="finding-title">{{ f.title }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Footer -->
          <div class="popup-footer">
            <span class="text-muted small">
              <i class="bi bi-clock me-1"></i>
              <span v-if="forwardView?.source === 'traffic' || reverseView?.source === 'traffic'">
                <template v-if="forwardView && reverseView">
                  {{ forwardView.sampleCount + reverseView.sampleCount }} samples in range
                </template>
                <template v-else-if="forwardView">
                  {{ forwardView.sampleCount }} samples in range
                </template>
                <template v-else-if="reverseView">
                  {{ reverseView.sampleCount }} samples in range
                </template>
              </span>
              <span v-else>Last hour analysis</span>
            </span>
            <button class="detail-link" @click="openDetail">
              View Detailed Analysis <i class="bi bi-arrow-right ms-1"></i>
            </button>
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
  width: 720px;
  max-width: calc(100vw - 2rem);
  max-height: 90vh;
  overflow-y: auto;
  animation: popup-enter 0.2s ease-out;
}

@keyframes popup-enter {
  from { opacity: 0; transform: scale(0.95) translateY(-10px); }
  to { opacity: 1; transform: scale(1) translateY(0); }
}

.popup-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  border-bottom: 1px solid var(--bs-border-color);
  background: var(--bs-tertiary-bg);
  border-radius: 16px 16px 0 0;
  position: sticky;
  top: 0;
  z-index: 1;
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
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  position: sticky;
  bottom: 0;
}

.detail-link {
  background: none;
  border: 1px solid var(--bs-border-color);
  color: var(--bs-primary);
  font-size: 12px;
  font-weight: 500;
  padding: 4px 10px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s;
  display: inline-flex;
  align-items: center;
}

.detail-link:hover {
  background: var(--bs-primary);
  color: var(--bs-white);
  border-color: var(--bs-primary);
}

/* Direction pair layout — side-by-side when both are present */
.direction-pair {
  display: grid;
  grid-template-columns: 1fr;
  gap: 10px;
}
.direction-pair-stacked {
  grid-template-columns: 1fr 1fr;
}
@media (max-width: 640px) {
  .direction-pair-stacked { grid-template-columns: 1fr; }
}

/* Direction Section */
.direction-section {
  padding: 10px 12px;
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  background: var(--bs-secondary-bg);
  display: flex;
  flex-direction: column;
  gap: 8px;
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
  margin-bottom: 0;
  gap: 6px;
  flex-wrap: wrap;
}
.direction-header-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.health-score-large {
  font-size: 22px;
  font-weight: 700;
  font-family: 'SF Mono', 'Fira Code', monospace;
  line-height: 1;
}
.grade-badge {
  font-size: 10px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 10px;
  text-transform: capitalize;
}
.return-tag,
.traffic-tag {
  font-size: 8px;
  font-weight: 600;
  padding: 1px 5px;
  border-radius: 3px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.return-tag {
  background: rgba(var(--bs-primary-rgb), 0.15);
  color: var(--bs-primary);
}
.traffic-tag {
  background: rgba(var(--bs-info-rgb), 0.15);
  color: var(--bs-info);
}
.sample-count {
  font-size: 9px;
  color: var(--bs-secondary-color);
  margin-left: auto;
  background: var(--bs-tertiary-bg);
  padding: 1px 6px;
  border-radius: 3px;
}

/* Metric Groups (Latency / Jitter with Avg/Med/P95) */
.metric-group {
  margin-bottom: 4px;
}
.metric-group-label {
  font-size: 9px;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 600;
  margin-bottom: 3px;
}
.metric-row {
  display: flex;
  gap: 3px;
  margin-bottom: 4px;
}
.metric-row.single {
  margin-bottom: 0;
}
.metric-item {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 4px 2px;
  background: var(--bs-tertiary-bg);
  border-radius: 4px;
}
.metric-val {
  font-size: 11px;
  font-weight: 700;
  font-family: 'SF Mono', 'Fira Code', monospace;
  color: var(--bs-body-color);
  line-height: 1.1;
}
.metric-lbl {
  font-size: 8px;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.3px;
  margin-top: 1px;
}

/* Health Bars */
.health-bars {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.hbr-row {
  display: flex;
  align-items: center;
  gap: 4px;
}
.hbr-lbl {
  font-size: 9px;
  color: var(--bs-secondary-color);
  width: 42px;
  flex-shrink: 0;
  font-weight: 500;
}
.hbr-track {
  flex: 1;
  height: 4px;
  background: var(--bs-tertiary-bg);
  border-radius: 2px;
  overflow: hidden;
}
.hbr-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 0.4s ease;
}
.hbr-val {
  font-size: 9px;
  font-weight: 600;
  width: 20px;
  text-align: right;
  color: var(--bs-body-color);
}

/* Path Hops (compact, popup-only) */
.path-hops {
  margin-top: 4px;
  padding-top: 4px;
  border-top: 1px solid var(--bs-border-color);
}
.path-hops-header {
  display: flex;
  align-items: center;
  font-size: 9px;
  font-weight: 600;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 3px;
}
.hops-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.hop-item {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 9px;
  padding: 2px 3px;
  background: var(--bs-tertiary-bg);
  border-radius: 3px;
}
.hop-num {
  width: 12px;
  height: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bs-secondary-bg);
  border-radius: 50%;
  font-size: 7px;
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
  font-size: 9px;
}
.hop-lat {
  font-family: 'SF Mono', 'Fira Code', monospace;
  color: var(--bs-secondary-color);
  font-size: 9px;
}
.hop-loss {
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 9px;
}
.hops-more {
  font-size: 8px;
  color: var(--bs-secondary-color);
  text-align: center;
  padding: 2px;
}

/* Signals & Findings */
.signals-findings-section {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-top: 8px;
  margin-top: 4px;
  border-top: 1px solid var(--bs-border-color);
}
.signals-block, .findings-block {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.block-header {
  display: flex;
  align-items: center;
  font-size: 9px;
  font-weight: 600;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.signal-list, .finding-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.signal-item, .finding-item {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 4px 6px;
  border-radius: 4px;
  background: var(--bs-tertiary-bg);
}
.signal-item.warning { background: rgba(var(--bs-warning-rgb), 0.1); }
.signal-item.critical { background: rgba(var(--bs-danger-rgb), 0.1); }
.signal-item.info { background: rgba(var(--bs-info-rgb), 0.1); }
.signal-type {
  font-size: 8px;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--bs-secondary-color);
  letter-spacing: 0.3px;
}
.signal-title {
  font-size: 10px;
  color: var(--bs-body-color);
}
.finding-item.warning { background: rgba(var(--bs-warning-rgb), 0.1); }
.finding-item.critical { background: rgba(var(--bs-danger-rgb), 0.1); }
.finding-item.info { background: rgba(var(--bs-info-rgb), 0.1); }
.finding-title {
  font-size: 10px;
  color: var(--bs-body-color);
}

/* Dark theme */
[data-theme="dark"] .popup-content {
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
}
</style>
