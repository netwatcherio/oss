<template>
  <div class="mtr-summary">
    <!-- Summary Header -->
    <div class="summary-header">
      <div class="summary-stats">
        <div class="stat-item">
          <span class="stat-value">{{ totalTraceCount }}</span>
          <span class="stat-label">Traces</span>
        </div>
        <div class="stat-item">
          <span class="stat-value">{{ uniqueRoutes }}</span>
          <span class="stat-label">Unique Routes</span>
        </div>
        <div class="stat-item" v-if="notableCount > 0" :class="{ warning: true }">
          <span class="stat-value">{{ notableCount }}</span>
          <span class="stat-label">Issues</span>
        </div>
        <div class="stat-item" v-if="routeChangeCount > 0" :class="{ info: true }">
          <span class="stat-value">{{ routeChangeCount }}</span>
          <span class="stat-label">Route Changes</span>
        </div>
      </div>
    </div>

    <!-- No Data State -->
    <div v-if="parsedData.length === 0 && !loading" class="no-data">
      <i class="bi bi-diagram-3"></i>
      <p>No traceroute data available</p>
    </div>

    <!-- Loading State -->
    <div v-else-if="loading" class="no-data">
      <div class="spinner-border spinner-border-sm text-primary me-2"></div>
      <p>Loading traceroute data...</p>
    </div>

    <!-- Route Cards -->
    <div v-else class="route-groups">
      <!-- Notable Traces Section -->
      <div v-if="notableTraces.length > 0" class="notable-section">
        <h6 class="section-title">
          <i class="bi bi-exclamation-triangle-fill text-warning me-2"></i>
          Notable Events
        </h6>
        
        <div 
          v-for="(item, index) in notableTraces" 
          :key="`notable-${index}`"
          class="route-card has-issues"
        >
          <div class="route-header" @click="toggleTrace(`notable-${index}`)">
            <div class="route-info">
              <div class="route-badge" :class="getReasonBadgeClass(item.notableReason)">
                <i :class="getReasonIcon(item.notableReason)"></i>
                <span>{{ formatReason(item.notableReason) }}</span>
              </div>
              <div class="route-path">
                <span class="hop-count">{{ item.hops.length }} hops</span>
                <span class="route-endpoints">
                  {{ item.hops[0] || '?' }} 
                  <i class="bi bi-arrow-right"></i> 
                  {{ item.hops[item.hops.length - 1] || '?' }}
                </span>
              </div>
            </div>
            
            <div class="route-metrics">
              <div class="metric" :class="getLatencyClass(item.finalLatency)">
                <span class="metric-value">{{ item.finalLatency.toFixed(1) }}ms</span>
                <span class="metric-label">Latency</span>
              </div>
              <div class="metric" :class="getLossClass(item.maxLoss)">
                <span class="metric-value">{{ item.maxLoss.toFixed(1) }}%</span>
                <span class="metric-label">Max Loss</span>
              </div>
              <div class="metric time-range">
                <span class="metric-value">{{ formatTimestamp(item.timestamp) }}</span>
                <span class="metric-label">Time</span>
              </div>
            </div>
            
            <i :class="expandedTraces[`notable-${index}`] ? 'bi bi-chevron-up' : 'bi bi-chevron-down'" class="expand-icon"></i>
          </div>

          <!-- Route Change Diff -->
          <div v-if="item.notableReason === 'route-change' && item.previousRoute" class="route-diff">
            <div class="diff-header">
              <i class="bi bi-arrow-repeat me-2"></i>Route Change Detected
            </div>
            <div class="diff-content">
              <div class="diff-line removed">
                <span class="diff-marker">-</span>
                <span class="diff-text">{{ item.previousRoute }}</span>
              </div>
              <div class="diff-line added">
                <span class="diff-marker">+</span>
                <span class="diff-text">{{ item.routeSignature }}</span>
              </div>
            </div>
          </div>

          <!-- Expanded Trace Details -->
          <div v-if="expandedTraces[`notable-${index}`]" class="route-traces">
            <MtrTable :probe-data="item.originalTrace" :show-copy="true" />
          </div>
        </div>
      </div>

      <!-- Aggregated Routes Section -->
      <div v-if="aggregatedRoutes.length > 0" class="aggregated-section">
        <h6 class="section-title">
          <i class="bi bi-graph-up me-2"></i>
          Aggregated Routes
        </h6>
        
        <div 
          v-for="(item, index) in aggregatedRoutes" 
          :key="`agg-${index}`"
          class="route-card"
          :class="{ 'is-primary': index === 0 }"
        >
          <div class="route-header" @click="toggleTrace(`agg-${index}`)">
            <div class="route-info">
              <div class="route-badge aggregated">
                <i class="bi bi-layers"></i>
                <span>{{ item.traceCount }} traces</span>
              </div>
              <div class="route-path">
                <span class="hop-count">{{ item.hops.length }} hops</span>
                <span class="route-endpoints">
                  {{ item.hops[0] || '?' }} 
                  <i class="bi bi-arrow-right"></i> 
                  {{ item.hops[item.hops.length - 1] || '?' }}
                </span>
              </div>
            </div>
            
            <div class="route-metrics">
              <div class="metric" :class="getLatencyClass(item.avgLatency)">
                <span class="metric-value">{{ item.avgLatency.toFixed(1) }}ms</span>
                <span class="metric-label">Avg Latency</span>
              </div>
              <div class="metric" :class="getLossClass(item.maxLoss)">
                <span class="metric-value">{{ item.maxLoss.toFixed(1) }}%</span>
                <span class="metric-label">Max Loss</span>
              </div>
              <div class="metric time-range">
                <span class="metric-value">{{ formatTimestamp(item.timestamp) }}</span>
                <span class="metric-label">Bucket Time</span>
              </div>
            </div>
            
            <i :class="expandedTraces[`agg-${index}`] ? 'bi bi-chevron-up' : 'bi bi-chevron-down'" class="expand-icon"></i>
          </div>

          <!-- Expanded Trace Details -->
          <div v-if="expandedTraces[`agg-${index}`]" class="route-traces">
            <MtrTable :probe-data="item.originalTrace" :show-copy="true" />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { computed, reactive } from 'vue';
import type { ProbeData, MtrResult } from '@/types';
import MtrTable from '@/components/MtrTable.vue';

interface ParsedMtrData {
  hops: string[];
  routeSignature: string;
  previousRoute: string;
  traceCount: number;
  isAggregated: boolean;
  notableReason: string;
  finalLatency: number;
  avgLatency: number;
  maxLoss: number;
  timestamp: Date;
  originalTrace: ProbeData;
}

const props = defineProps<{
  mtrData: ProbeData[];
  loading?: boolean;
}>();

const emit = defineEmits<{
  (e: 'show-all-traces'): void;
}>();

// State
const expandedTraces = reactive<Record<string, boolean>>({});

// Parse the MTR data (handles both old and new formats)
const parsedData = computed<ParsedMtrData[]>(() => {
  if (!props.mtrData || props.mtrData.length === 0) return [];
  
  const result: ParsedMtrData[] = [];
  
  for (const trace of props.mtrData) {
    const payload = trace.payload as any;
    if (!payload?.report?.hops) continue;
    
    // Check if this is an aggregated payload
    const isAggregated = payload.is_aggregated === true;
    const notableReason = payload.notable_reason || '';
    const traceCount = payload.trace_count || 1;
    const routeSignature = payload.route_signature || '';
    const previousRoute = payload.previous_route_signature || '';
    
    // Extract hops
    const hops = payload.report.hops.map((hop: any) => 
      hop.hosts && hop.hosts.length > 0 ? hop.hosts[0].ip : '*'
    );
    
    // Calculate metrics
    const finalHop = payload.report.hops[payload.report.hops.length - 1];
    const finalLatency = parseFloat(finalHop?.avg || '0');
    
    let maxLoss = 0;
    for (const hop of payload.report.hops) {
      const loss = parseFloat(String(hop.loss_pct || '0').replace('%', ''));
      if (!isNaN(loss)) maxLoss = Math.max(maxLoss, loss);
    }
    
    const timestamp = new Date(payload.stop_timestamp || trace.created_at);
    
    result.push({
      hops,
      routeSignature,
      previousRoute,
      traceCount,
      isAggregated,
      notableReason,
      finalLatency,
      avgLatency: finalLatency, // For aggregated, this is already the avg
      maxLoss,
      timestamp,
      originalTrace: trace,
    });
  }
  
  // Sort: notable traces first (by time desc), then aggregated (by trace count desc)
  return result.sort((a, b) => {
    if (a.notableReason && !b.notableReason) return -1;
    if (!a.notableReason && b.notableReason) return 1;
    if (a.notableReason && b.notableReason) {
      return b.timestamp.getTime() - a.timestamp.getTime();
    }
    return b.traceCount - a.traceCount;
  });
});

// Separate notable and aggregated traces
const notableTraces = computed(() => parsedData.value.filter(d => d.notableReason));
const aggregatedRoutes = computed(() => parsedData.value.filter(d => d.isAggregated && !d.notableReason));

// Stats
const totalTraceCount = computed(() => {
  return parsedData.value.reduce((sum, d) => sum + d.traceCount, 0);
});

const uniqueRoutes = computed(() => {
  const signatures = new Set(parsedData.value.map(d => d.routeSignature || d.hops.join('->')));
  return signatures.size;
});

const notableCount = computed(() => notableTraces.value.length);
const routeChangeCount = computed(() => notableTraces.value.filter(t => t.notableReason === 'route-change').length);

// Methods
const toggleTrace = (key: string) => {
  expandedTraces[key] = !expandedTraces[key];
};

const formatTimestamp = (date: Date): string => {
  return date.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
};

const formatReason = (reason: string): string => {
  const map: Record<string, string> = {
    'triggered': 'Alert Triggered',
    'route-change': 'Route Change',
    'high-loss': 'High Loss',
    'high-latency': 'High Latency',
  };
  return map[reason] || reason;
};

const getReasonIcon = (reason: string): string => {
  const map: Record<string, string> = {
    'triggered': 'bi bi-bell-fill',
    'route-change': 'bi bi-arrow-repeat',
    'high-loss': 'bi bi-exclamation-triangle-fill',
    'high-latency': 'bi bi-speedometer',
  };
  return map[reason] || 'bi bi-info-circle';
};

const getReasonBadgeClass = (reason: string): string => {
  const map: Record<string, string> = {
    'triggered': 'badge-triggered',
    'route-change': 'badge-route-change',
    'high-loss': 'badge-high-loss',
    'high-latency': 'badge-high-latency',
  };
  return map[reason] || '';
};

const getLatencyClass = (latency: number): string => {
  if (latency < 50) return 'good';
  if (latency < 100) return 'warning';
  return 'critical';
};

const getLossClass = (loss: number): string => {
  if (loss === 0) return 'good';
  if (loss <= 5) return 'warning';
  return 'critical';
};
</script>

<style scoped>
.mtr-summary {
  padding: 1rem 0;
}

.summary-header {
  margin-bottom: 1rem;
}

.summary-stats {
  display: flex;
  gap: 1.5rem;
  flex-wrap: wrap;
}

.stat-item {
  display: flex;
  flex-direction: column;
  padding: 0.75rem 1.25rem;
  background: linear-gradient(135deg, #1e1f2e 0%, #252636 100%);
  border-radius: 10px;
  border: 1px solid #2a2b3d;
}

.stat-item.warning {
  border-color: rgba(255, 158, 100, 0.4);
  background: linear-gradient(135deg, rgba(255, 158, 100, 0.1) 0%, rgba(255, 158, 100, 0.05) 100%);
}

.stat-item.info {
  border-color: rgba(125, 207, 255, 0.4);
  background: linear-gradient(135deg, rgba(125, 207, 255, 0.1) 0%, rgba(125, 207, 255, 0.05) 100%);
}

.stat-value {
  font-size: 1.5rem;
  font-weight: 700;
  color: #c0caf5;
}

.stat-item.warning .stat-value {
  color: #ff9e64;
}

.stat-item.info .stat-value {
  color: #7dcfff;
}

.stat-label {
  font-size: 0.75rem;
  color: #565f89;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.section-title {
  margin: 1.5rem 0 0.75rem;
  color: #a9b1d6;
  font-size: 0.9rem;
  font-weight: 600;
  display: flex;
  align-items: center;
}

.no-data {
  text-align: center;
  padding: 3rem;
  color: #565f89;
}

.no-data i {
  font-size: 3rem;
  margin-bottom: 1rem;
  opacity: 0.5;
}

.route-groups {
  display: flex;
  flex-direction: column;
}

.notable-section,
.aggregated-section {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.route-card {
  background: #1a1b26;
  border: 1px solid #2a2b3d;
  border-radius: 12px;
  overflow: hidden;
  transition: all 0.2s;
}

.route-card:hover {
  border-color: #3d59a1;
}

.route-card.has-issues {
  border-color: rgba(255, 158, 100, 0.4);
}

.route-card.is-primary {
  border-color: rgba(158, 206, 106, 0.4);
}

.route-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem 1.25rem;
  cursor: pointer;
  background: linear-gradient(135deg, #1e1f2e 0%, #252636 100%);
}

.route-header:hover {
  background: linear-gradient(135deg, #252636 0%, #2d2e40 100%);
}

.route-info {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.route-badge {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.4rem 0.8rem;
  background: #3d59a1;
  border-radius: 6px;
  color: #c0caf5;
  font-size: 0.85rem;
  font-weight: 500;
}

.route-badge.aggregated {
  background: rgba(61, 89, 161, 0.3);
}

.route-badge.badge-triggered {
  background: rgba(247, 118, 142, 0.3);
  color: #f7768e;
}

.route-badge.badge-route-change {
  background: rgba(125, 207, 255, 0.2);
  color: #7dcfff;
}

.route-badge.badge-high-loss {
  background: rgba(255, 158, 100, 0.3);
  color: #ff9e64;
}

.route-badge.badge-high-latency {
  background: rgba(224, 175, 104, 0.3);
  color: #e0af68;
}

.route-path {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
}

.hop-count {
  font-size: 0.75rem;
  color: #565f89;
}

.route-endpoints {
  color: #7dcfff;
  font-family: 'SF Mono', Monaco, monospace;
  font-size: 0.85rem;
}

.route-endpoints i {
  margin: 0 0.3rem;
  color: #565f89;
}

.route-metrics {
  display: flex;
  gap: 1.5rem;
}

.metric {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
}

.metric-value {
  font-size: 0.95rem;
  font-weight: 600;
  color: #c0caf5;
  font-variant-numeric: tabular-nums;
}

.metric-label {
  font-size: 0.7rem;
  color: #565f89;
  text-transform: uppercase;
}

.metric.good .metric-value { color: #9ece6a; }
.metric.warning .metric-value { color: #e0af68; }
.metric.critical .metric-value { color: #f7768e; }

.expand-icon {
  color: #565f89;
  margin-left: 1rem;
}

/* Route Diff Styles */
.route-diff {
  border-top: 1px solid rgba(125, 207, 255, 0.2);
  background: rgba(125, 207, 255, 0.05);
  padding: 0.75rem 1.25rem;
}

.diff-header {
  color: #7dcfff;
  font-size: 0.8rem;
  font-weight: 500;
  margin-bottom: 0.5rem;
}

.diff-content {
  font-family: 'SF Mono', Monaco, monospace;
  font-size: 0.8rem;
}

.diff-line {
  display: flex;
  align-items: flex-start;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  margin-bottom: 0.25rem;
}

.diff-line.removed {
  background: rgba(247, 118, 142, 0.15);
}

.diff-line.added {
  background: rgba(158, 206, 106, 0.15);
}

.diff-marker {
  font-weight: 700;
  margin-right: 0.5rem;
  width: 1rem;
}

.diff-line.removed .diff-marker {
  color: #f7768e;
}

.diff-line.added .diff-marker {
  color: #9ece6a;
}

.diff-text {
  color: #a9b1d6;
  word-break: break-all;
}

.route-traces {
  border-top: 1px solid #2a2b3d;
  padding: 1rem;
  background: #16161e;
}

@media (max-width: 768px) {
  .route-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 0.75rem;
  }
  
  .route-metrics {
    width: 100%;
    justify-content: space-between;
  }
  
  .expand-icon {
    display: none;
  }
}
</style>
