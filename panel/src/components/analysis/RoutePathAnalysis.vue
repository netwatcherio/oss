<script lang="ts" setup>
import { ref, onMounted, onUnmounted, computed, watch } from 'vue'
import { ProbeDataService } from '@/services/apiService'
import type { WorkspaceRouteAnalysis, AgentRouteInfo, SharedHopInfo, RouteIncident } from './types'

const props = defineProps<{
  workspaceId: number | string
}>()

const analysis = ref<WorkspaceRouteAnalysis | null>(null)
const loading = ref(true)
const error = ref('')
const expandedAgents = ref<Set<number>>(new Set())
const expandedIncidents = ref<Set<string>>(new Set())
const refreshInterval = ref<ReturnType<typeof setInterval> | null>(null)
const secondsUntilRefresh = ref(60)
const countdownInterval = ref<ReturnType<typeof setInterval> | null>(null)

// Filter/sort state
const searchQuery = ref('')
const filterType = ref<'all' | 'issues' | 'route_change' | 'ip_change' | 'isp_change'>('all')
const sortBy = ref<'name' | 'issues' | 'latency' | 'routes'>('name')
const currentPage = ref(1)
const pageSize = ref(25)
const compactMode = ref(false)

// Selected hop for detail panel
const selectedHop = ref<{ ip: string; label: string; metrics: { latency: number; loss: number } | null; sharedWith: string[] } | null>(null)

async function fetchAnalysis() {
  try {
    loading.value = true
    analysis.value = await ProbeDataService.workspaceRouteAnalysis(props.workspaceId)
    error.value = ''
  } catch (e: any) {
    error.value = e?.message || 'Failed to fetch route analysis'
  } finally {
    loading.value = false
    secondsUntilRefresh.value = 60
  }
}

function startRefreshTimer() {
  if (refreshInterval.value) clearInterval(refreshInterval.value)
  if (countdownInterval.value) clearInterval(countdownInterval.value)
  
  refreshInterval.value = setInterval(fetchAnalysis, 60000)
  countdownInterval.value = setInterval(() => {
    if (secondsUntilRefresh.value > 0) {
      secondsUntilRefresh.value--
    }
  }, 1000)
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

// Group incidents by severity
const groupedIncidents = computed(() => {
  if (!analysis.value?.incidents?.length) return { critical: [], warning: [], info: [] }
  const groups = { critical: [] as RouteIncident[], warning: [] as RouteIncident[], info: [] as RouteIncident[] }
  for (const inc of analysis.value.incidents) {
    if (inc.severity === 'critical') groups.critical.push(inc)
    else if (inc.severity === 'warning') groups.warning.push(inc)
    else groups.info.push(inc)
  }
  return groups
})

// Shared hop agents map
const sharedHopAgents = computed(() => {
  if (!analysis.value?.shared_hops?.length) return {}
  const map: Record<string, string[]> = {}
  for (const sh of analysis.value.shared_hops) {
    map[sh.hop_ip] = sh.agent_names
  }
  return map
})

// Filtered and sorted agents
const filteredAgents = computed(() => {
  if (!analysis.value?.agents?.length) return []
  
  let agents = [...analysis.value.agents]
  
  // Apply search filter
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    agents = agents.filter(a => 
      a.agent_name.toLowerCase().includes(query) ||
      a.public_ip?.toLowerCase().includes(query) ||
      a.isp?.toLowerCase().includes(query)
    )
  }
  
  // Apply type filter
  if (filterType.value !== 'all') {
    agents = agents.filter(a => {
      switch (filterType.value) {
        case 'issues': return a.has_ip_change || a.has_isp_change || a.routes?.some(r => r.has_route_change)
        case 'route_change': return a.routes?.some(r => r.has_route_change)
        case 'ip_change': return a.has_ip_change
        case 'isp_change': return a.has_isp_change
        default: return true
      }
    })
  }
  
  // Apply sorting
  agents.sort((a, b) => {
    switch (sortBy.value) {
      case 'name': return a.agent_name.localeCompare(b.agent_name)
      case 'issues': {
        const aIssues = (a.has_ip_change ? 1 : 0) + (a.has_isp_change ? 1 : 0) + (a.routes?.filter(r => r.has_route_change).length || 0)
        const bIssues = (b.has_ip_change ? 1 : 0) + (b.has_isp_change ? 1 : 0) + (b.routes?.filter(r => r.has_route_change).length || 0)
        return bIssues - aIssues
      }
      case 'latency': {
        const aLat = Math.min(...(a.routes?.map(r => r.avg_end_hop_latency || Infinity) || [Infinity]))
        const bLat = Math.min(...(b.routes?.map(r => r.avg_end_hop_latency || Infinity) || [Infinity]))
        return aLat - bLat
      }
      case 'routes': return (b.routes?.length || 0) - (a.routes?.length || 0)
      default: return 0
    }
  })
  
  return agents
})

// Paginated agents
const paginatedAgents = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return filteredAgents.value.slice(start, start + pageSize.value)
})

const totalPages = computed(() => Math.ceil(filteredAgents.value.length / pageSize.value))

// Stats computed
const routeChangeCount = computed(() =>
  analysis.value?.incidents?.filter(i => i.type === 'route_change').length || 0
)
const ipChangeCount = computed(() =>
  analysis.value?.incidents?.filter(i => i.type === 'ip_change').length || 0
)
const ispChangeCount = computed(() =>
  analysis.value?.incidents?.filter(i => i.type === 'isp_change').length || 0
)

// Time ago helper
function timeAgo(date: string): string {
  const seconds = Math.floor((Date.now() - new Date(date).getTime()) / 1000)
  if (seconds < 60) return 'just now'
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
  return `${Math.floor(seconds / 86400)}d ago`
}

// Hop display helpers
function getHopDisplay(route: any, idx: number): { ip: string; label: string; isAgent: boolean } {
  if (route.latest_hops_detail && route.latest_hops_detail[idx]) {
    const detail = route.latest_hops_detail[idx]
    return { ip: detail.ip, label: detail.hostname || detail.ip, isAgent: detail.is_agent }
  }
  return { ip: route.latest_hops[idx], label: route.latest_hops[idx], isAgent: false }
}

function getHopMetrics(route: any, ip: string): { latency: number; loss: number } | null {
  if (!route.intermediate_hops) return null
  const m = route.intermediate_hops.find((h: any) => h.ip === ip)
  return m ? { latency: m.latency, loss: m.loss } : null
}

function hopHealthClass(latency: number, loss: number): string {
  if (latency > 150 || loss > 3) return 'poor'
  if (latency >= 50 || loss > 0) return 'degraded'
  return 'healthy'
}

function stabilityClass(pct: number | null): string {
  if (pct === null) return ''
  if (pct < 70) return 'poor'
  if (pct < 90) return 'warning'
  return 'good'
}

// Hop click handler
function showHopDetail(route: any, idx: number) {
  const hopInfo = getHopDisplay(route, idx)
  const metrics = getHopMetrics(route, hopInfo.ip)
  const sharedWith = sharedHopAgents.value[hopInfo.ip] || []
  selectedHop.value = { ...hopInfo, metrics, sharedWith }
}

function closeHopDetail() {
  selectedHop.value = null
}

// Expand/collapse all
function expandAll() {
  filteredAgents.value.forEach(a => expandedAgents.value.add(a.agent_id))
}

function collapseAll() {
  expandedAgents.value.clear()
}

// Reset page on filter change
watch([searchQuery, filterType, sortBy], () => {
  currentPage.value = 1
})

onMounted(() => {
  fetchAnalysis()
  startRefreshTimer()
})

onUnmounted(() => {
  if (refreshInterval.value) clearInterval(refreshInterval.value)
  if (countdownInterval.value) clearInterval(countdownInterval.value)
})
</script>

<template>
  <div class="route-analysis">
    <!-- Loading skeleton -->
    <div v-if="loading && !analysis" class="loading-state">
      <div class="skeleton-stats"></div>
      <div class="skeleton-cards">
        <div class="skeleton-card" v-for="i in 6" :key="i"></div>
      </div>
    </div>

    <!-- Error -->
    <div v-else-if="error && !analysis" class="error-state">
      <i class="bi bi-exclamation-triangle fs-1 text-warning mb-3"></i>
      <h5 class="text-muted">Route Analysis Unavailable</h5>
      <p class="text-muted small">{{ error }}</p>
      <button class="btn btn-sm btn-outline-primary" @click="fetchAnalysis">
        <i class="bi bi-arrow-clockwise me-1"></i>Retry
      </button>
    </div>

    <!-- Content -->
    <div v-else-if="analysis">
      <!-- Header with controls -->
      <div class="analysis-header">
        <div class="header-top">
          <h5 class="section-title mb-0">
            <i class="bi bi-diagram-3 me-1"></i>Route Analysis
          </h5>
          <div class="header-actions">
            <button class="btn btn-sm btn-outline-secondary" @click="compactMode = !compactMode" :title="compactMode ? 'Expand all' : 'Collapse all'">
              <i :class="compactMode ? 'bi bi-arrows-expand' : 'bi bi-arrows-collapse'"></i>
            </button>
            <button class="btn btn-sm btn-outline-secondary" @click="expandedAgents.size ? collapseAll() : expandAll()">
              {{ expandedAgents.size ? 'Collapse All' : 'Expand All' }}
            </button>
            <button class="btn btn-sm btn-outline-primary" @click="fetchAnalysis" :disabled="loading">
              <i class="bi bi-arrow-clockwise" :class="{ 'spinning': loading }"></i>
            </button>
          </div>
        </div>
        
        <!-- Filter controls -->
        <div class="filter-bar">
          <div class="search-input-wrapper">
            <i class="bi bi-search"></i>
            <input v-model="searchQuery" type="text" placeholder="Search agents..." class="search-input" />
          </div>
          <select v-model="filterType" class="filter-select">
            <option value="all">All routes</option>
            <option value="issues">Has issues</option>
            <option value="route_change">Route changes</option>
            <option value="ip_change">IP changes</option>
            <option value="isp_change">ISP changes</option>
          </select>
          <select v-model="sortBy" class="sort-select">
            <option value="name">Name A-Z</option>
            <option value="issues">Most issues</option>
            <option value="latency">Worst latency</option>
            <option value="routes">Most routes</option>
          </select>
        </div>

        <!-- Stats bar -->
        <div class="stats-bar">
          <div class="stat-pill primary">
            <div class="stat-inner">
              <i class="bi bi-hdd-network"></i>
              <span class="stat-value">{{ analysis.total_agents }}</span>
              <span class="stat-label">agents</span>
            </div>
          </div>
          <div class="stat-pill primary">
            <div class="stat-inner">
              <i class="bi bi-diagram-2"></i>
              <span class="stat-value">{{ analysis.total_routes }}</span>
              <span class="stat-label">routes</span>
            </div>
          </div>
          <div class="stat-pill primary">
            <div class="stat-inner">
              <i class="bi bi-share"></i>
              <span class="stat-value">{{ analysis.shared_hops?.length || 0 }}</span>
              <span class="stat-label">shared hops</span>
            </div>
          </div>
          <div v-if="routeChangeCount > 0" class="stat-pill warning">
            <div class="stat-inner">
              <i class="bi bi-shuffle"></i>
              <span class="stat-value">{{ routeChangeCount }}</span>
              <span class="stat-label">route changes</span>
            </div>
          </div>
          <div v-if="ipChangeCount > 0" class="stat-pill info">
            <div class="stat-inner">
              <i class="bi bi-globe2"></i>
              <span class="stat-value">{{ ipChangeCount }}</span>
              <span class="stat-label">IP changes</span>
            </div>
          </div>
          <div v-if="ispChangeCount > 0" class="stat-pill danger">
            <div class="stat-inner">
              <i class="bi bi-building"></i>
              <span class="stat-value">{{ ispChangeCount }}</span>
              <span class="stat-label">ISP changes</span>
            </div>
          </div>
          <div class="stat-pill ml-auto">
            <div class="stat-inner timestamp">
              <i class="bi bi-clock"></i>
              <span>{{ timeAgo(analysis.generated_at) }}</span>
              <span class="refresh-countdown" v-if="!loading">· {{ secondsUntilRefresh }}s</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Incidents grouped by severity -->
      <div v-if="analysis.incidents?.length" class="incidents-section">
        <div v-for="(incidents, severity) in groupedIncidents" :key="severity">
          <div v-if="incidents.length > 0" class="severity-group">
            <h6 class="severity-header" :class="severity">
              <i :class="severity === 'critical' ? 'bi bi-x-octagon-fill' : severity === 'warning' ? 'bi bi-exclamation-triangle-fill' : 'bi bi-info-circle-fill'"></i>
              {{ severity === 'critical' ? 'Critical' : severity === 'warning' ? 'Warning' : 'Info' }}
              <span class="badge-count">{{ incidents.length }}</span>
            </h6>
            <div v-for="incident in incidents" :key="incident.id" class="incident-card" :class="[severity, { expanded: expandedIncidents.has(incident.id) }]" @click="toggleIncident(incident.id)">
              <div class="incident-header">
                <div class="incident-icon">
                  <i :class="['bi', incidentIcon(incident.type)]"></i>
                </div>
                <div class="incident-content">
                  <div class="incident-title">{{ incident.message }}</div>
                  <div class="incident-meta">
                    <span class="meta-badge">{{ incident.agent_name }}</span>
                    <span v-if="incident.target" class="meta-badge target">
                      <i class="bi bi-arrow-right"></i> {{ incident.target }}
                    </span>
                    <span v-if="incident.detected_at" class="meta-badge time">
                      {{ timeAgo(incident.detected_at) }}
                    </span>
                  </div>
                </div>
                <i :class="['bi', expandedIncidents.has(incident.id) ? 'bi-chevron-up' : 'bi-chevron-down', 'expand-icon']"></i>
              </div>
              <div v-if="expandedIncidents.has(incident.id)" class="incident-details">
                <div v-if="incident.evidence?.length" class="evidence-list">
                  <div v-for="(e, i) in incident.evidence" :key="i" class="evidence-item">
                    <i class="bi bi-check-circle"></i>{{ e }}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Shared Hops -->
      <div v-if="analysis.shared_hops?.length" class="shared-hops-section">
        <h6 class="section-title">
          <i class="bi bi-share me-1"></i>Shared Network Hops
          <span class="badge-count">{{ analysis.shared_hops.length }}</span>
        </h6>
        <div class="shared-hops-grid">
          <div v-for="hop in analysis.shared_hops" :key="hop.hop_ip" class="hop-card" :class="hopHealthClass(hop.avg_latency || 0, hop.avg_loss || 0)">
            <div class="hop-header">
              <div class="hop-icon">
                <i class="bi bi-router"></i>
              </div>
              <div class="hop-info">
                <code class="hop-ip">{{ hop.hop_ip }}</code>
                <div v-if="hop.avg_latency != null || hop.avg_loss != null" class="hop-metrics">
                  <span v-if="hop.avg_latency != null" class="metric-badge" :class="hopHealthClass(hop.avg_latency, hop.avg_loss || 0)">
                    {{ hop.avg_latency.toFixed(0) }}ms
                  </span>
                  <span v-if="hop.avg_loss != null && hop.avg_loss > 0" class="metric-badge loss" :class="hopHealthClass(hop.avg_latency || 0, hop.avg_loss)">
                    {{ hop.avg_loss.toFixed(1) }}%
                  </span>
                </div>
              </div>
              <div class="hop-agents-count">
                <span class="count-badge">{{ hop.hop_count }}</span>
              </div>
            </div>
            <div class="hop-agents">
              <span v-for="name in hop.agent_names.slice(0, 3)" :key="name" class="agent-tag">{{ name }}</span>
              <span v-if="hop.agent_names.length > 3" class="agent-tag more">+{{ hop.agent_names.length - 3 }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Agent Routes -->
      <div class="agent-routes-section">
        <div class="section-header">
          <h6 class="section-title">
            <i class="bi bi-diagram-3 me-1"></i>Agent Routes
            <span class="badge-count">{{ filteredAgents.length }}</span>
          </h6>
          <span v-if="filteredAgents.length !== analysis.total_agents" class="filter-info">
            of {{ analysis.total_agents }} total
          </span>
        </div>

        <div v-if="!filteredAgents.length" class="empty-state">
          <i class="bi bi-search fs-1 text-muted"></i>
          <p class="text-muted">No agents match your filters</p>
          <button class="btn btn-sm btn-outline-secondary" @click="searchQuery = ''; filterType = 'all'">Clear filters</button>
        </div>

        <div v-else class="agent-list">
          <div v-for="agent in paginatedAgents" :key="agent.agent_id" class="agent-card" :class="{ 'has-issues': agent.has_ip_change || agent.has_isp_change || agent.routes?.some(r => r.has_route_change) }">
            <div class="agent-header" @click="toggleAgent(agent.agent_id)" :class="{ expanded: expandedAgents.has(agent.agent_id) }">
              <div class="agent-score" :class="agent.has_ip_change || agent.has_isp_change ? 'warning' : 'healthy'">
                <i class="bi" :class="agent.has_ip_change || agent.has_isp_change ? 'bi-exclamation-triangle' : 'bi bi-hdd-network'"></i>
              </div>
              <div class="agent-info">
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
              <div class="agent-badges">
                <span v-if="agent.has_ip_change" class="badge ip-change">
                  <i class="bi bi-globe2"></i>
                </span>
                <span v-if="agent.has_isp_change" class="badge isp-change">
                  <i class="bi bi-building"></i>
                </span>
                <span v-if="agent.routes?.some(r => r.has_route_change)" class="badge route-change">
                  <i class="bi bi-shuffle"></i> {{ agent.routes.filter(r => r.has_route_change).length }}
                </span>
                <span class="route-count">{{ agent.routes?.length || 0 }}</span>
              </div>
              <i :class="['bi', expandedAgents.has(agent.agent_id) ? 'bi-chevron-up' : 'bi-chevron-down', 'expand-icon']"></i>
            </div>

            <div v-if="expandedAgents.has(agent.agent_id)" class="agent-routes" :class="{ compact: compactMode }">
              <div v-if="!agent.routes?.length" class="empty-routes">
                <i class="bi bi-info-circle"></i> No MTR routes configured
              </div>
              <div v-for="route in agent.routes" :key="route.probe_id" class="route-item" :class="{ 'has-issue': route.has_route_change }">
                <div class="route-header">
                  <div class="route-target">
                    <i class="bi bi-bullseye"></i>
                    <span class="target-name">{{ route.target || 'Unknown target' }}</span>
                  </div>
                  <div class="route-vitals">
                    <span v-if="route.route_stability_pct != null" class="stability-badge" :class="stabilityClass(route.route_stability_pct)">
                      <i class="bi bi-activity"></i> {{ Math.round(route.route_stability_pct) }}%
                    </span>
                    <span v-if="route.avg_end_hop_latency != null" class="latency-badge">
                      {{ route.avg_end_hop_latency.toFixed(1) }}ms
                    </span>
                    <span v-if="route.avg_end_hop_loss != null && route.avg_end_hop_loss > 0" class="loss-badge">
                      {{ route.avg_end_hop_loss.toFixed(2) }}%
                    </span>
                  </div>
                </div>

                <div v-if="route.has_route_change" class="route-change-banner">
                  <i class="bi bi-shuffle"></i>
                  Route changed
                  <span v-if="route.baseline_hop_count">(was {{ route.baseline_hop_count }} hops)</span>
                </div>

                <!-- Route Hops -->
                <div v-if="route.latest_hops?.length && !compactMode" class="route-hops">
                  <div class="hops-chain">
                    <div class="hop-node source">
                      <i class="bi bi-pc-display"></i>
                      <span class="hop-label">{{ agent.agent_name }}</span>
                    </div>
                    <template v-for="(hop, idx) in route.latest_hops" :key="idx">
                      <div class="hop-arrow"><i class="bi bi-arrow-right"></i></div>
                      <div class="hop-node" 
                           :class="[hopHealthClass(getHopMetrics(route, getHopDisplay(route, idx).ip)?.latency || 0, getHopMetrics(route, getHopDisplay(route, idx).ip)?.loss || 0), { shared: sharedHopAgents[getHopDisplay(route, idx).ip], 'agent-hop': getHopDisplay(route, idx).isAgent }]"
                           @click="showHopDetail(route, idx)">
                        <i :class="getHopDisplay(route, idx).isAgent ? 'bi bi-hdd-network' : 'bi bi-router'"></i>
                        <span class="hop-label" :title="getHopDisplay(route, idx).ip">{{ getHopDisplay(route, idx).label }}</span>
                        <span v-if="sharedHopAgents[getHopDisplay(route, idx).ip]" class="shared-badge">
                          <i class="bi bi-share"></i>{{ sharedHopAgents[getHopDisplay(route, idx).ip].length }}
                        </span>
                        <div v-if="getHopMetrics(route, getHopDisplay(route, idx).ip)" class="hop-node-metrics">
                          <span class="hop-metric-value">{{ getHopMetrics(route, getHopDisplay(route, idx).ip)!.latency.toFixed(0) }}ms</span>
                          <span v-if="getHopMetrics(route, getHopDisplay(route, idx).ip)!.loss > 0" class="hop-metric-value loss">{{ getHopMetrics(route, getHopDisplay(route, idx).ip)!.loss.toFixed(1) }}%</span>
                        </div>
                      </div>
                    </template>
                    <div class="hop-arrow"><i class="bi bi-arrow-right"></i></div>
                    <div class="hop-node dest">
                      <i class="bi bi-bullseye"></i>
                      <span class="hop-label">{{ route.target || 'Target' }}</span>
                    </div>
                  </div>
                </div>

                <div v-if="!compactMode && route.avg_end_hop_latency != null" class="route-metrics">
                  <div class="metric">
                    <span class="metric-value">{{ route.avg_end_hop_latency.toFixed(1) }}</span>
                    <span class="metric-label">ms latency</span>
                  </div>
                  <div v-if="route.avg_end_hop_loss != null" class="metric">
                    <span class="metric-value">{{ route.avg_end_hop_loss.toFixed(2) }}</span>
                    <span class="metric-label">% loss</span>
                  </div>
                  <div v-if="route.trace_count" class="metric">
                    <span class="metric-value">{{ route.trace_count }}</span>
                    <span class="metric-label">traces</span>
                  </div>
                  <div v-if="route.baseline_hop_count" class="metric">
                    <span class="metric-value">{{ route.baseline_hop_count }}</span>
                    <span class="metric-label">baseline hops</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Pagination -->
        <div v-if="totalPages > 1" class="pagination-wrapper">
          <div class="pagination-info">
            Showing {{ (currentPage - 1) * pageSize + 1 }}-{{ Math.min(currentPage * pageSize, filteredAgents.length) }} of {{ filteredAgents.length }}
          </div>
          <div class="pagination-controls">
            <button class="btn btn-sm btn-outline-secondary" :disabled="currentPage === 1" @click="currentPage--">
              <i class="bi bi-chevron-left"></i>
            </button>
            <span class="page-indicator">{{ currentPage }} / {{ totalPages }}</span>
            <button class="btn btn-sm btn-outline-secondary" :disabled="currentPage === totalPages" @click="currentPage++">
              <i class="bi bi-chevron-right"></i>
            </button>
          </div>
        </div>
      </div>

      <!-- Hop detail panel -->
      <div v-if="selectedHop" class="hop-detail-overlay" @click.self="closeHopDetail">
        <div class="hop-detail-panel">
          <div class="panel-header">
            <h5>{{ selectedHop.ip }}</h5>
            <button class="btn btn-sm btn-close" @click="closeHopDetail">
              <i class="bi bi-x"></i>
            </button>
          </div>
          <div class="panel-body">
            <div v-if="selectedHop.metrics" class="metrics-grid">
              <div class="metric-card" :class="hopHealthClass(selectedHop.metrics.latency, selectedHop.metrics.loss)">
                <span class="metric-value">{{ selectedHop.metrics.latency.toFixed(1) }}ms</span>
                <span class="metric-label">Latency</span>
              </div>
              <div v-if="selectedHop.metrics.loss > 0" class="metric-card" :class="hopHealthClass(selectedHop.metrics.latency, selectedHop.metrics.loss)">
                <span class="metric-value">{{ selectedHop.metrics.loss.toFixed(2) }}%</span>
                <span class="metric-label">Loss</span>
              </div>
            </div>
            <div v-if="selectedHop.sharedWith.length" class="shared-section">
              <h6>Shared with {{ selectedHop.sharedWith.length }} agents</h6>
              <div class="agent-list-compact">
                <span v-for="name in selectedHop.sharedWith" :key="name" class="agent-tag">{{ name }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="analysis-footer">
        <span><i class="bi bi-clock"></i> Updated {{ new Date(analysis.generated_at).toLocaleString() }}</span>
        <span>· Auto-refreshes every 60s</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.route-analysis {
  padding: 0;
}

/* Loading skeleton */
.loading-state {
  padding: 1rem;
}

.skeleton-stats {
  height: 60px;
  background: linear-gradient(90deg, var(--bs-secondary-bg) 25%, var(--bs-tertiary-bg) 50%, var(--bs-secondary-bg) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
  border-radius: 12px;
  margin-bottom: 1rem;
}

.skeleton-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 0.75rem;
}

.skeleton-card {
  height: 100px;
  background: linear-gradient(90deg, var(--bs-secondary-bg) 25%, var(--bs-tertiary-bg) 50%, var(--bs-secondary-bg) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
  border-radius: 10px;
}

@keyframes shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}

/* Header */
.analysis-header {
  margin-bottom: 1rem;
}

.header-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.75rem;
}

.header-actions {
  display: flex;
  gap: 0.5rem;
}

.header-actions .btn {
  padding: 0.25rem 0.5rem;
}

.header-actions .btn i {
  font-size: 14px;
}

/* Filter bar */
.filter-bar {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
  flex-wrap: wrap;
}

.search-input-wrapper {
  position: relative;
  flex: 1;
  min-width: 200px;
}

.search-input-wrapper i {
  position: absolute;
  left: 10px;
  top: 50%;
  transform: translateY(-50%);
  color: var(--bs-secondary-color);
}

.search-input {
  width: 100%;
  padding: 0.4rem 0.75rem 0.4rem 2rem;
  border: 1px solid var(--bs-border-color);
  border-radius: 8px;
  background: var(--bs-body-bg);
  font-size: 0.85rem;
}

.search-input:focus {
  outline: none;
  border-color: var(--bs-primary);
}

.filter-select, .sort-select {
  padding: 0.4rem 0.75rem;
  border: 1px solid var(--bs-border-color);
  border-radius: 8px;
  background: var(--bs-body-bg);
  font-size: 0.85rem;
  cursor: pointer;
}

/* Stats bar */
.stats-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  padding: 0.75rem;
  background: var(--bs-secondary-bg);
  border-radius: 12px;
  border: 1px solid var(--bs-border-color);
  align-items: center;
}

.stat-pill {
  display: flex;
  align-items: center;
  padding: 0.5rem 0.75rem;
  background: var(--bs-body-bg);
  border-radius: 8px;
  border: 1px solid var(--bs-border-color);
}

.stat-pill.primary i { color: var(--bs-primary); }
.stat-pill.warning i { color: #f59e0b; }
.stat-pill.info i { color: #3b82f6; }
.stat-pill.danger i { color: #ef4444; }

.stat-inner {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.stat-value {
  font-weight: 700;
  font-size: 1rem;
  color: var(--bs-body-color);
}

.stat-label {
  font-size: 0.7rem;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.stat-pill.ml-auto {
  margin-left: auto;
}

.timestamp {
  font-size: 0.8rem;
  color: var(--bs-secondary-color);
}

.refresh-countdown {
  color: var(--bs-primary);
  font-weight: 500;
}

/* Section title */
.section-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.badge-count {
  font-size: 10px;
  font-weight: 700;
  background: var(--bs-primary);
  color: white;
  padding: 2px 6px;
  border-radius: 8px;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
}

.filter-info {
  font-size: 0.75rem;
  color: var(--bs-secondary-color);
}

/* Incidents */
.incidents-section {
  margin-bottom: 1.5rem;
}

.severity-group {
  margin-bottom: 1rem;
}

.severity-header {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 0.5rem;
  padding: 0.25rem 0;
}

.severity-header.critical { color: #ef4444; }
.severity-header.warning { color: #f59e0b; }
.severity-header.info { color: #3b82f6; }

.incident-card {
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  margin-bottom: 6px;
  cursor: pointer;
  transition: all 0.15s;
  overflow: hidden;
}

.incident-card:hover {
  box-shadow: 0 2px 8px rgba(0,0,0,0.08);
}

.incident-card.critical { border-left: 3px solid #ef4444; }
.incident-card.warning { border-left: 3px solid #f59e0b; }
.incident-card.info { border-left: 3px solid #3b82f6; }

.incident-card.expanded {
  margin-bottom: 8px;
}

.incident-header {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
}

.incident-icon {
  font-size: 16px;
  width: 24px;
  text-align: center;
}

.incident-card.critical .incident-icon { color: #ef4444; }
.incident-card.warning .incident-icon { color: #f59e0b; }
.incident-card.info .incident-icon { color: #3b82f6; }

.incident-content { flex: 1; min-width: 0; }

.incident-title {
  font-weight: 600;
  font-size: 13px;
  color: var(--bs-body-color);
}

.incident-meta {
  display: flex;
  gap: 6px;
  margin-top: 4px;
  flex-wrap: wrap;
}

.meta-badge {
  font-size: 10px;
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--bs-secondary-bg);
  color: var(--bs-secondary-color);
  display: inline-flex;
  align-items: center;
  gap: 3px;
}

.meta-badge.target {
  background: rgba(var(--bs-primary-rgb), 0.1);
  color: var(--bs-primary);
}

.meta-badge.time {
  color: var(--bs-secondary-color);
}

.expand-icon {
  color: var(--bs-secondary-color);
  font-size: 12px;
}

.incident-details {
  padding: 0 12px 10px;
  border-top: 1px solid var(--bs-border-color);
}

.evidence-list {
  padding-top: 8px;
}

.evidence-item {
  font-size: 12px;
  color: var(--bs-body-color);
  padding: 3px 0;
  display: flex;
  align-items: flex-start;
  gap: 6px;
}

.evidence-item i {
  color: var(--bs-success);
  margin-top: 2px;
}

/* Shared hops grid */
.shared-hops-section {
  margin-bottom: 1.5rem;
}

.shared-hops-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 0.75rem;
}

.hop-card {
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  padding: 12px;
  transition: all 0.15s;
}

.hop-card:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(0,0,0,0.08);
}

.hop-card.healthy { border-left: 3px solid #10b981; }
.hop-card.degraded { border-left: 3px solid #f59e0b; }
.hop-card.poor { border-left: 3px solid #ef4444; }

.hop-header {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-bottom: 8px;
}

.hop-icon {
  font-size: 16px;
  color: var(--bs-secondary-color);
}

.hop-info {
  flex: 1;
}

.hop-ip {
  font-size: 12px;
  font-weight: 600;
  background: var(--bs-secondary-bg);
  padding: 2px 6px;
  border-radius: 4px;
}

.hop-metrics {
  display: flex;
  gap: 4px;
  margin-top: 4px;
}

.metric-badge {
  font-size: 10px;
  font-weight: 600;
  padding: 2px 6px;
  border-radius: 4px;
}

.metric-badge.healthy {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
}

.metric-badge.degraded {
  background: rgba(245, 158, 11, 0.15);
  color: #f59e0b;
}

.metric-badge.poor {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

.metric-badge.loss {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

.hop-agents-count {
  text-align: right;
}

.count-badge {
  font-size: 11px;
  font-weight: 600;
  background: var(--bs-secondary-bg);
  color: var(--bs-secondary-color);
  padding: 2px 8px;
  border-radius: 6px;
}

.hop-agents {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.agent-tag {
  font-size: 10px;
  padding: 2px 6px;
  border-radius: 4px;
  background: rgba(var(--bs-primary-rgb), 0.1);
  color: var(--bs-primary);
}

.agent-tag.more {
  background: var(--bs-secondary-bg);
  color: var(--bs-secondary-color);
}

/* Agent cards */
.agent-routes-section {
  margin-bottom: 1rem;
}

.agent-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.agent-card {
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 12px;
  overflow: hidden;
}

.agent-card.has-issues {
  border-left: 3px solid #f59e0b;
}

.agent-header {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  cursor: pointer;
  transition: background 0.15s;
}

.agent-header:hover {
  background: var(--bs-secondary-bg);
}

.agent-header.expanded {
  background: var(--bs-secondary-bg);
}

.agent-score {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
}

.agent-score.healthy {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
}

.agent-score.warning {
  background: rgba(245, 158, 11, 0.15);
  color: #f59e0b;
}

.agent-info {
  flex: 1;
  min-width: 0;
}

.agent-name {
  font-weight: 600;
  font-size: 14px;
  color: var(--bs-body-color);
}

.agent-meta {
  display: flex;
  gap: 10px;
  margin-top: 2px;
  flex-wrap: wrap;
}

.meta-item {
  font-size: 11px;
  color: var(--bs-secondary-color);
  display: flex;
  align-items: center;
  gap: 3px;
}

.agent-badges {
  display: flex;
  align-items: center;
  gap: 6px;
}

.agent-badges .badge {
  font-size: 10px;
  padding: 3px 6px;
  border-radius: 4px;
  display: inline-flex;
  align-items: center;
  gap: 3px;
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

.route-count {
  font-size: 11px;
  color: var(--bs-secondary-color);
  background: var(--bs-secondary-bg);
  padding: 2px 8px;
  border-radius: 4px;
}

.expand-icon {
  color: var(--bs-secondary-color);
  font-size: 14px;
}

/* Agent routes */
.agent-routes {
  border-top: 1px solid var(--bs-border-color);
  padding: 12px;
}

.agent-routes.compact {
  padding: 8px 12px;
}

.empty-routes {
  text-align: center;
  padding: 1rem;
  color: var(--bs-secondary-color);
  font-size: 0.85rem;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
}

/* Route item */
.route-item {
  background: var(--bs-secondary-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  padding: 12px;
  margin-bottom: 8px;
}

.route-item.has-issue {
  border-left: 3px solid #f59e0b;
}

.route-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  margin-bottom: 8px;
  flex-wrap: wrap;
}

.route-target {
  display: flex;
  align-items: center;
  gap: 6px;
}

.route-target i {
  color: var(--bs-primary);
}

.target-name {
  font-weight: 600;
  font-size: 13px;
  color: var(--bs-body-color);
}

.route-vitals {
  display: flex;
  align-items: center;
  gap: 6px;
}

.stability-badge {
  font-size: 10px;
  font-weight: 600;
  padding: 3px 8px;
  border-radius: 4px;
  display: inline-flex;
  align-items: center;
  gap: 3px;
}

.stability-badge.good {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
}

.stability-badge.warning {
  background: rgba(245, 158, 11, 0.15);
  color: #f59e0b;
}

.stability-badge.poor {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

.latency-badge {
  font-size: 11px;
  font-weight: 600;
  color: var(--bs-body-color);
}

.loss-badge {
  font-size: 10px;
  font-weight: 600;
  padding: 2px 6px;
  border-radius: 4px;
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

.route-change-banner {
  font-size: 11px;
  color: #f59e0b;
  background: rgba(245, 158, 11, 0.1);
  padding: 4px 8px;
  border-radius: 4px;
  margin-bottom: 8px;
  display: flex;
  align-items: center;
  gap: 4px;
}

/* Hop chain */
.route-hops {
  overflow-x: auto;
  padding-bottom: 6px;
  margin-bottom: 8px;
}

.hops-chain {
  display: flex;
  align-items: center;
  min-width: max-content;
}

.hop-node {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 3px;
  padding: 6px 8px;
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 6px;
  min-width: 70px;
  max-width: 120px;
  position: relative;
  cursor: pointer;
  transition: all 0.15s;
}

.hop-node:hover {
  border-color: var(--bs-primary);
  transform: translateY(-2px);
}

.hop-node.source, .hop-node.dest {
  background: rgba(var(--bs-primary-rgb), 0.1);
  border-color: rgba(var(--bs-primary-rgb), 0.3);
}

.hop-node.shared {
  background: rgba(16, 185, 129, 0.1);
  border-color: rgba(16, 185, 129, 0.3);
}

.hop-node.healthy { border-color: #10b981; }
.hop-node.degraded { border-color: #f59e0b; }
.hop-node.poor { border-color: #ef4444; }

.hop-node i {
  font-size: 14px;
  color: var(--bs-secondary-color);
}

.hop-node.source i, .hop-node.dest i { color: var(--bs-primary); }
.hop-node.shared i { color: #10b981; }

.hop-label {
  font-size: 9px;
  color: var(--bs-body-color);
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}

.hop-arrow {
  padding: 0 4px;
  color: var(--bs-secondary-color);
  font-size: 10px;
}

.hop-node-metrics {
  display: flex;
  gap: 3px;
  margin-top: 2px;
}

.hop-metric-value {
  font-size: 8px;
  font-weight: 600;
  padding: 1px 4px;
  border-radius: 3px;
  background: var(--bs-secondary-bg);
}

.hop-metric-value.loss {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

.shared-badge {
  position: absolute;
  top: -5px;
  right: -5px;
  font-size: 8px;
  font-weight: 600;
  background: #10b981;
  color: white;
  padding: 1px 4px;
  border-radius: 6px;
  display: flex;
  align-items: center;
  gap: 1px;
}

.shared-badge i {
  font-size: 8px;
}

/* Route metrics */
.route-metrics {
  display: flex;
  gap: 1rem;
  padding-top: 8px;
  border-top: 1px solid var(--bs-border-color);
}

.metric {
  display: flex;
  flex-direction: column;
}

.metric-value {
  font-weight: 700;
  font-size: 13px;
  color: var(--bs-body-color);
}

.metric-label {
  font-size: 10px;
  color: var(--bs-secondary-color);
}

/* Pagination */
.pagination-wrapper {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 1rem;
  padding-top: 1rem;
  border-top: 1px solid var(--bs-border-color);
}

.pagination-info {
  font-size: 0.8rem;
  color: var(--bs-secondary-color);
}

.pagination-controls {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.page-indicator {
  font-size: 0.85rem;
  color: var(--bs-secondary-color);
  min-width: 60px;
  text-align: center;
}

/* Hop detail panel */
.hop-detail-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0,0,0,0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.hop-detail-panel {
  background: var(--bs-body-bg);
  border-radius: 12px;
  width: 90%;
  max-width: 400px;
  max-height: 80vh;
  overflow: hidden;
  box-shadow: 0 20px 60px rgba(0,0,0,0.3);
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--bs-border-color);
}

.panel-header h5 {
  margin: 0;
  font-size: 14px;
  font-family: monospace;
}

.panel-body {
  padding: 16px;
  overflow-y: auto;
}

.metrics-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  margin-bottom: 16px;
}

.metric-card {
  padding: 12px;
  border-radius: 8px;
  text-align: center;
  background: var(--bs-secondary-bg);
}

.metric-card.healthy { border-left: 3px solid #10b981; }
.metric-card.degraded { border-left: 3px solid #f59e0b; }
.metric-card.poor { border-left: 3px solid #ef4444; }

.metric-card .metric-value {
  font-size: 18px;
  font-weight: 700;
}

.metric-card .metric-label {
  font-size: 10px;
  text-transform: uppercase;
}

.shared-section h6 {
  font-size: 11px;
  text-transform: uppercase;
  color: var(--bs-secondary-color);
  margin-bottom: 8px;
}

.agent-list-compact {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

/* Footer */
.analysis-footer {
  margin-top: 1rem;
  padding-top: 1rem;
  border-top: 1px solid var(--bs-border-color);
  font-size: 0.75rem;
  color: var(--bs-secondary-color);
  display: flex;
  gap: 0.5rem;
}

/* Empty state */
.empty-state {
  text-align: center;
  padding: 2rem;
  color: var(--bs-secondary-color);
}

.empty-state i {
  margin-bottom: 0.5rem;
}

.empty-state p {
  margin-bottom: 0.75rem;
}

/* Error state */
.error-state {
  text-align: center;
  padding: 3rem;
}

/* Spinning animation */
.spinning {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* Dark mode adjustments */
[data-theme="dark"] .skeleton-stats,
[data-theme="dark"] .skeleton-card {
  background: linear-gradient(90deg, var(--bs-secondary-bg) 25%, var(--bs-tertiary-bg) 50%, var(--bs-secondary-bg) 75%);
}

[data-theme="dark"] .hop-node {
  background: var(--bs-tertiary-bg);
}

[data-theme="dark"] .route-item {
  background: var(--bs-tertiary-bg);
}

[data-theme="dark"] .hop-metric-value {
  background: var(--bs-secondary-bg);
}
</style>