<script lang="ts" setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { ProbeDataService } from '@/services/apiService'
import type { WorkspaceRouteAnalysis, AgentRouteInfo, SharedHopInfo, RouteIncident } from './types'
import { severityIcons } from './types'

const props = defineProps<{
  workspaceId: number | string
}>()

const analysis = ref<WorkspaceRouteAnalysis | null>(null)
const loading = ref(true)
const error = ref('')
const expandedAgents = ref<Set<number>>(new Set())
const expandedIncidents = ref<Set<string>>(new Set())
let refreshInterval: ReturnType<typeof setInterval> | null = null

async function fetchAnalysis() {
  try {
    analysis.value = await ProbeDataService.workspaceRouteAnalysis(props.workspaceId)
    error.value = ''
  } catch (e: any) {
    error.value = e?.message || 'Failed to fetch route analysis'
  } finally {
    loading.value = false
  }
}

function toggleAgent(id: number) {
  if (expandedAgents.value.has(id)) {
    expandedAgents.value.delete(id)
  } else {
    expandedAgents.value.add(id)
  }
}

function toggleIncident(id: string) {
  if (expandedIncidents.value.has(id)) {
    expandedIncidents.value.delete(id)
  } else {
    expandedIncidents.value.add(id)
  }
}

function severityClass(severity: string) {
  switch (severity) {
    case 'critical': return 'critical'
    case 'warning': return 'warning'
    case 'info': return 'info'
    default: return 'info'
  }
}

function incidentIcon(type_: string) {
  switch (type_) {
    case 'ip_change': return 'bi-globe2'
    case 'isp_change': return 'bi-building'
    case 'route_change': return 'bi-shuffle'
    default: return 'bi-exclamation-circle'
  }
}

const sharedHopAgents = computed(() => {
  if (!analysis.value?.shared_hops?.length) return {}
  const map: Record<string, string[]> = {}
  for (const sh of analysis.value.shared_hops) {
    map[sh.hop_ip] = sh.agent_names
  }
  return map
})

// Get display info for a hop - uses detail if available, otherwise falls back to IP
function getHopDisplay(route: any, idx: number): { ip: string; label: string; isAgent: boolean } {
  if (route.latest_hops_detail && route.latest_hops_detail[idx]) {
    const detail = route.latest_hops_detail[idx]
    return {
      ip: detail.ip,
      label: detail.hostname || detail.ip,
      isAgent: detail.is_agent
    }
  }
  return {
    ip: route.latest_hops[idx],
    label: route.latest_hops[idx],
    isAgent: false
  }
}

const routeChangeCount = computed(() =>
  analysis.value?.incidents?.filter(i => i.type === 'route_change').length || 0
)

const ipChangeCount = computed(() =>
  analysis.value?.incidents?.filter(i => i.type === 'ip_change').length || 0
)

const ispChangeCount = computed(() =>
  analysis.value?.incidents?.filter(i => i.type === 'isp_change').length || 0
)

const totalHops = computed(() =>
  analysis.value?.shared_hops?.reduce((sum, sh) => sum + sh.hop_count, 0) || 0
)

onMounted(() => {
  fetchAnalysis()
  refreshInterval = setInterval(fetchAnalysis, 60000)
})

onUnmounted(() => {
  if (refreshInterval) clearInterval(refreshInterval)
})
</script>

<template>
  <div class="route-analysis">
    <!-- Loading -->
    <div v-if="loading" class="text-center py-5">
      <div class="spinner-border text-primary" role="status">
        <span class="visually-hidden">Loading...</span>
      </div>
      <p class="text-muted mt-2">Analyzing route paths...</p>
    </div>

    <!-- Error -->
    <div v-else-if="error" class="text-center py-5">
      <i class="bi bi-exclamation-triangle fs-1 text-warning mb-3"></i>
      <h5 class="text-muted">Route Analysis Unavailable</h5>
      <p class="text-muted small">{{ error }}</p>
      <button class="btn btn-sm btn-outline-primary" @click="fetchAnalysis">Retry</button>
    </div>

    <!-- Content -->
    <div v-else-if="analysis">
      <!-- Stats Bar -->
      <div class="stats-bar mb-4">
        <div class="stat-pill">
          <i class="bi bi-hdd-network"></i>
          <span class="stat-value">{{ analysis.total_agents }}</span>
          <span class="stat-label">agents</span>
        </div>
        <div class="stat-pill">
          <i class="bi bi-diagram-2"></i>
          <span class="stat-value">{{ analysis.total_routes }}</span>
          <span class="stat-label">routes</span>
        </div>
        <div class="stat-pill">
          <i class="bi bi-share"></i>
          <span class="stat-value">{{ analysis.shared_hops?.length || 0 }}</span>
          <span class="stat-label">shared hops</span>
        </div>
        <div v-if="routeChangeCount > 0" class="stat-pill warning">
          <i class="bi bi-shuffle"></i>
          <span class="stat-value">{{ routeChangeCount }}</span>
          <span class="stat-label">route changes</span>
        </div>
        <div v-if="ipChangeCount > 0" class="stat-pill info">
          <i class="bi bi-globe2"></i>
          <span class="stat-value">{{ ipChangeCount }}</span>
          <span class="stat-label">IP changes</span>
        </div>
        <div v-if="ispChangeCount > 0" class="stat-pill danger">
          <i class="bi bi-building"></i>
          <span class="stat-value">{{ ispChangeCount }}</span>
          <span class="stat-label">ISP changes</span>
        </div>
      </div>

      <!-- Incidents -->
      <div v-if="analysis.incidents?.length" class="incidents-section mb-4">
        <h6 class="section-title">
          <i class="bi bi-lightning-charge me-1"></i>
          Path Issues
          <span class="badge-count">{{ analysis.incidents.length }}</span>
        </h6>
        <div v-for="incident in analysis.incidents" :key="incident.id"
          class="incident-card" :class="severityClass(incident.severity)"
          @click="toggleIncident(incident.id)"
        >
          <div class="incident-header">
            <i :class="['bi', incidentIcon(incident.type), 'incident-type-icon']"></i>
            <div class="incident-info">
              <div class="incident-title">{{ incident.message }}</div>
              <div class="incident-meta">
                <span class="meta-badge">{{ incident.agent_name }}</span>
                <span v-if="incident.target" class="meta-badge target">→ {{ incident.target }}</span>
              </div>
            </div>
            <i :class="['bi', expandedIncidents.has(incident.id) ? 'bi-chevron-up' : 'bi-chevron-down', 'expand-icon']"></i>
          </div>
          <div v-if="expandedIncidents.has(incident.id)" class="incident-details" @click.stop>
            <div v-if="incident.evidence?.length" class="detail-section">
              <div class="detail-label">Evidence</div>
              <div v-for="(e, i) in incident.evidence" :key="i" class="evidence-item">
                <i class="bi bi-dot"></i>{{ e }}
              </div>
            </div>
            <div v-if="incident.detected_at" class="detail-section">
              <div class="detail-label">Detected</div>
              <span class="text-muted small">{{ new Date(incident.detected_at).toLocaleString() }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Shared Hops -->
      <div v-if="analysis.shared_hops?.length" class="shared-hops-section mb-4">
        <h6 class="section-title">
          <i class="bi bi-share me-1"></i>
          Shared Network Hops
          <span class="badge-count">{{ analysis.shared_hops.length }}</span>
        </h6>
        <div class="shared-hops-grid">
          <div v-for="hop in analysis.shared_hops" :key="hop.hop_ip" class="hop-card">
            <div class="hop-ip">
              <i class="bi bi-router me-1"></i>
              <code>{{ hop.hop_ip }}</code>
            </div>
            <div class="hop-agents">
              <span v-for="name in hop.agent_names" :key="name" class="agent-tag">
                {{ name }}
              </span>
            </div>
            <div class="hop-count">
              <span class="count-badge">{{ hop.hop_count }} agent{{ hop.hop_count !== 1 ? 's' : '' }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Agent Routes -->
      <div class="agent-routes-section">
        <h6 class="section-title">
          <i class="bi bi-diagram-3 me-1"></i>
          Agent Routes
        </h6>
        <div v-if="!analysis.agents?.length" class="empty-state">
          <i class="bi bi-diagram-3 fs-1 text-muted"></i>
          <p class="text-muted">No route data available</p>
        </div>
        <div v-for="agent in analysis.agents" :key="agent.agent_id" class="agent-card">
          <div class="agent-header" @click="toggleAgent(agent.agent_id)">
            <div class="agent-main">
              <i class="bi bi-hdd-network agent-icon"></i>
              <div>
                <div class="agent-name">{{ agent.agent_name }}</div>
                <div class="agent-meta">
                  <span v-if="agent.public_ip" class="meta-item">
                    <i class="bi bi-globe2"></i> {{ agent.public_ip }}
                  </span>
                  <span v-if="agent.isp" class="meta-item">
                    <i class="bi bi-building"></i> {{ agent.isp }}
                  </span>
                </div>
              </div>
            </div>
            <div class="agent-badges">
              <span v-if="agent.has_ip_change" class="badge ip-change" title="IP changed recently">
                <i class="bi bi-globe2"></i> IP Change
              </span>
              <span v-if="agent.has_isp_change" class="badge isp-change" title="ISP changed recently">
                <i class="bi bi-building"></i> ISP Change
              </span>
              <span class="route-count">{{ agent.routes?.length || 0 }} route{{ agent.routes?.length !== 1 ? 's' : '' }}</span>
              <i :class="['bi', expandedAgents.has(agent.agent_id) ? 'bi-chevron-up' : 'bi-chevron-down', 'expand-icon']"></i>
            </div>
          </div>

          <div v-if="expandedAgents.has(agent.agent_id)" class="agent-routes">
            <div v-if="!agent.routes?.length" class="empty-routes">
              <span class="text-muted small">No MTR routes configured</span>
            </div>
            <div v-for="route in agent.routes" :key="route.probe_id" class="route-item"
              :class="{ 'has-issue': route.has_route_change }"
            >
              <div class="route-header">
                <div class="route-target">
                  <i class="bi bi-arrow-right-circle"></i>
                  <span>{{ route.target || 'Unknown target' }}</span>
                </div>
                <div class="route-badges">
                  <span v-if="route.has_route_change" class="badge route-change">
                    <i class="bi bi-shuffle"></i> Route Change
                  </span>
                  <span v-if="route.route_stability_pct != null" class="badge stability"
                    :class="{ warning: route.route_stability_pct < 90, good: route.route_stability_pct >= 90 }"
                  >
                    <i class="bi bi-activity"></i> {{ Math.round(route.route_stability_pct) }}% stable
                  </span>
                </div>
              </div>

              <!-- Route Hops Visualization -->
              <div v-if="route.latest_hops?.length" class="route-hops">
                <div class="hops-chain">
                  <div class="hop-node source">
                    <i class="bi bi-pc-display"></i>
                    <span class="hop-label">{{ agent.agent_name }}</span>
                  </div>
                  <div v-for="(hop, idx) in route.latest_hops" :key="idx" class="hop-wrapper">
                    <div class="hop-arrow">
                      <i class="bi bi-arrow-right"></i>
                    </div>
                    <div class="hop-node" :class="{ shared: sharedHopAgents[getHopDisplay(route, idx).ip], 'agent-hop': getHopDisplay(route, idx).isAgent }">
                      <i :class="getHopDisplay(route, idx).isAgent ? 'bi bi-hdd-network' : 'bi bi-router'"></i>
                      <span class="hop-label" :title="getHopDisplay(route, idx).ip">{{ getHopDisplay(route, idx).label }}</span>
                      <span v-if="sharedHopAgents[getHopDisplay(route, idx).ip]" class="shared-badge"
                        :title="`Shared with: ${sharedHopAgents[getHopDisplay(route, idx).ip].join(', ')}`"
                      >
                        <i class="bi bi-share"></i> {{ sharedHopAgents[getHopDisplay(route, idx).ip].length }}
                      </span>
                    </div>
                  </div>
                  <div class="hop-wrapper">
                    <div class="hop-arrow">
                      <i class="bi bi-arrow-right"></i>
                    </div>
                    <div class="hop-node dest">
                      <i class="bi bi-bullseye"></i>
                      <span class="hop-label">{{ route.target || 'Target' }}</span>
                    </div>
                  </div>
                </div>
              </div>

              <div v-else class="route-no-hops">
                <span class="text-muted small">No hop data available</span>
              </div>

              <!-- Route Metrics -->
              <div v-if="route.avg_end_hop_latency != null || route.avg_end_hop_loss != null" class="route-metrics">
                <div v-if="route.avg_end_hop_latency != null" class="metric">
                  <span class="metric-value">{{ route.avg_end_hop_latency.toFixed(1) }}ms</span>
                  <span class="metric-label">End-hop latency</span>
                </div>
                <div v-if="route.avg_end_hop_loss != null" class="metric">
                  <span class="metric-value">{{ route.avg_end_hop_loss.toFixed(2) }}%</span>
                  <span class="metric-label">End-hop loss</span>
                </div>
                <div v-if="route.trace_count" class="metric">
                  <span class="metric-value">{{ route.trace_count }}</span>
                  <span class="metric-label">Traces</span>
                </div>
                <div v-if="route.baseline_hop_count" class="metric">
                  <span class="metric-value">{{ route.baseline_hop_count }}</span>
                  <span class="metric-label">Baseline hops</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="text-muted small mt-3 text-end timestamp">
        <i class="bi bi-clock me-1"></i>
        {{ new Date(analysis.generated_at).toLocaleTimeString() }} · Auto-refreshes every 60s
      </div>
    </div>
  </div>
</template>

<style scoped>
.route-analysis {
  padding: 0;
}

/* Stats Bar */
.stats-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  padding: 0.75rem;
  background: var(--bs-secondary-bg);
  border-radius: 12px;
  border: 1px solid var(--bs-border-color);
}

.stat-pill {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.4rem 0.75rem;
  background: var(--bs-body-bg);
  border-radius: 8px;
  border: 1px solid var(--bs-border-color);
  font-size: 0.85rem;
}

.stat-pill i {
  color: var(--bs-primary);
}

.stat-pill.warning i { color: #f59e0b; }
.stat-pill.info i { color: #3b82f6; }
.stat-pill.danger i { color: #ef4444; }

.stat-value {
  font-weight: 700;
  color: var(--bs-body-color);
}

.stat-label {
  font-size: 0.75rem;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

/* Section Title */
.section-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin: 1.5rem 0 10px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.badge-count {
  font-size: 10px;
  font-weight: 700;
  background: var(--bs-primary);
  color: white;
  padding: 1px 6px;
  border-radius: 8px;
}

/* Incident Cards */
.incident-card {
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  padding: 12px 16px;
  margin-bottom: 8px;
  cursor: pointer;
  transition: transform 0.15s, box-shadow 0.15s;
}

.incident-card:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(0,0,0,0.1);
}

.incident-card.critical { border-left: 4px solid #ef4444; }
.incident-card.warning { border-left: 4px solid #f59e0b; }
.incident-card.info { border-left: 4px solid #3b82f6; }

.incident-header {
  display: flex;
  align-items: center;
  gap: 10px;
}

.incident-type-icon {
  font-size: 18px;
  flex-shrink: 0;
}

.incident-card.critical .incident-type-icon { color: #ef4444; }
.incident-card.warning .incident-type-icon { color: #f59e0b; }
.incident-card.info .incident-type-icon { color: #3b82f6; }

.incident-info { flex: 1; min-width: 0; }
.incident-title { font-weight: 600; font-size: 14px; color: var(--bs-body-color); }
.incident-meta { display: flex; gap: 6px; margin-top: 4px; flex-wrap: wrap; }

.meta-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 6px;
  background: var(--bs-secondary-bg);
  color: var(--bs-secondary-color);
}

.meta-badge.target {
  background: rgba(var(--bs-primary-rgb), 0.1);
  color: var(--bs-primary);
}

.expand-icon {
  color: var(--bs-secondary-color);
  font-size: 14px;
  flex-shrink: 0;
}

.incident-details {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--bs-border-color);
}

.detail-section { margin-bottom: 10px; }
.detail-label {
  font-size: 11px;
  font-weight: 600;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 4px;
}

.evidence-item {
  font-size: 13px;
  color: var(--bs-body-color);
  padding: 2px 0;
}

/* Shared Hops Grid */
.shared-hops-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 0.75rem;
}

.hop-card {
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  padding: 12px;
  transition: transform 0.15s;
}

.hop-card:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(0,0,0,0.08);
}

.hop-ip {
  font-size: 13px;
  font-weight: 600;
  color: var(--bs-body-color);
  margin-bottom: 6px;
  display: flex;
  align-items: center;
  gap: 4px;
}

.hop-ip code {
  font-size: 12px;
  background: var(--bs-secondary-bg);
  padding: 2px 6px;
  border-radius: 4px;
}

.hop-agents {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: 6px;
}

.agent-tag {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 6px;
  background: rgba(var(--bs-primary-rgb), 0.1);
  color: var(--bs-primary);
}

.count-badge {
  font-size: 10px;
  font-weight: 600;
  color: var(--bs-secondary-color);
  background: var(--bs-secondary-bg);
  padding: 2px 8px;
  border-radius: 6px;
}

/* Agent Cards */
.agent-card {
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 12px;
  margin-bottom: 12px;
  overflow: hidden;
}

.agent-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  cursor: pointer;
  gap: 1rem;
  flex-wrap: wrap;
}

.agent-main {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  min-width: 0;
}

.agent-icon {
  font-size: 20px;
  color: var(--bs-primary);
  flex-shrink: 0;
}

.agent-name {
  font-weight: 600;
  font-size: 15px;
  color: var(--bs-body-color);
}

.agent-meta {
  display: flex;
  gap: 12px;
  margin-top: 2px;
  flex-wrap: wrap;
}

.meta-item {
  font-size: 12px;
  color: var(--bs-secondary-color);
  display: flex;
  align-items: center;
  gap: 4px;
}

.agent-badges {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
  flex-wrap: wrap;
}

.agent-badges .badge {
  font-size: 11px;
  padding: 3px 8px;
  border-radius: 6px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.badge.ip-change {
  background: rgba(59, 130, 246, 0.15);
  color: #3b82f6;
}

.badge.isp-change {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

.badge.route-change {
  background: rgba(245, 158, 11, 0.15);
  color: #f59e0b;
}

.badge.stability {
  background: var(--bs-secondary-bg);
  color: var(--bs-secondary-color);
}

.badge.stability.warning {
  background: rgba(245, 158, 11, 0.15);
  color: #f59e0b;
}

.badge.stability.good {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
}

.route-count {
  font-size: 12px;
  color: var(--bs-secondary-color);
}

/* Routes inside agent card */
.agent-routes {
  border-top: 1px solid var(--bs-border-color);
  padding: 12px 16px;
}

.empty-routes {
  padding: 12px 0;
  text-align: center;
}

.route-item {
  background: var(--bs-secondary-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  padding: 12px;
  margin-bottom: 10px;
}

.route-item.has-issue {
  border-left: 3px solid #f59e0b;
}

.route-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  flex-wrap: wrap;
  margin-bottom: 10px;
}

.route-target {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
  font-size: 14px;
  color: var(--bs-body-color);
}

.route-badges {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

/* Hops Chain */
.route-hops {
  overflow-x: auto;
  padding-bottom: 8px;
  margin-bottom: 10px;
}

.hops-chain {
  display: flex;
  align-items: center;
  gap: 0;
  min-width: max-content;
}

.hop-wrapper {
  display: flex;
  align-items: center;
}

.hop-arrow {
  padding: 0 6px;
  color: var(--bs-secondary-color);
  font-size: 12px;
}

.hop-node {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 8px 10px;
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 8px;
  min-width: 80px;
  max-width: 140px;
  position: relative;
}

.hop-node.source,
.hop-node.dest {
  background: rgba(var(--bs-primary-rgb), 0.1);
  border-color: rgba(var(--bs-primary-rgb), 0.3);
}

.hop-node.shared {
  background: rgba(16, 185, 129, 0.1);
  border-color: rgba(16, 185, 129, 0.3);
}

.hop-node i {
  font-size: 16px;
  color: var(--bs-secondary-color);
}

.hop-node.source i,
.hop-node.dest i {
  color: var(--bs-primary);
}

.hop-node.shared i {
  color: #10b981;
}

.hop-label {
  font-size: 10px;
  color: var(--bs-body-color);
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}

.shared-badge {
  position: absolute;
  top: -6px;
  right: -6px;
  font-size: 9px;
  background: #10b981;
  color: white;
  padding: 1px 5px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  gap: 2px;
}

.route-no-hops {
  padding: 8px 0;
}

/* Route Metrics */
.route-metrics {
  display: flex;
  gap: 1rem;
  flex-wrap: wrap;
  padding-top: 10px;
  border-top: 1px solid var(--bs-border-color);
}

.metric {
  display: flex;
  flex-direction: column;
}

.metric-value {
  font-weight: 700;
  font-size: 14px;
  color: var(--bs-body-color);
}

.metric-label {
  font-size: 11px;
  color: var(--bs-secondary-color);
}

/* Empty State */
.empty-state {
  text-align: center;
  padding: 2rem;
  color: var(--bs-secondary-color);
}

.timestamp {
  font-size: 11px;
  opacity: 0.7;
}

/* Dark mode compatibility */
[data-theme="dark"] .hop-node {
  background: var(--bs-tertiary-bg);
}

[data-theme="dark"] .hop-node.source,
[data-theme="dark"] .hop-node.dest {
  background: rgba(var(--bs-primary-rgb), 0.15);
}

[data-theme="dark"] .hop-node.shared {
  background: rgba(16, 185, 129, 0.15);
}
</style>
