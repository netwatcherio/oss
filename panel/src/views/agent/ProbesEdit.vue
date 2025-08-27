<script lang="ts" setup>
import type {AgentGroup, MemberInfo, Probe, Site, SiteMember} from "@/types";
import {onMounted, reactive, computed} from "vue";
import siteService from "@/services/siteService";
import Title from "@/components/Title.vue";
import core from "@/core";
import {Agent} from "@/types";
import agentService from "@/services/agentService";
import probeService from "@/services/probeService";

const state = reactive({
  probes: [] as Probe[],
  site: {} as Site,
  ready: false,
  loading: true,
  agent: {} as Agent,
  agents: [] as Agent[],
  agentGroups: [] as AgentGroup[],
  searchQuery: '',
  selectedType: 'all'
})

// Helper functions that need to be accessible in template
function getProbeTypeLabel(probe: Probe): string {
  if (probe.type === 'RPERF' && probe.config?.server) {
    return 'RPERF SERVER';
  }
  if (probe.type === 'TRAFFICSIM' && probe.config?.server) {
    return 'TRAFFIC SIM SERVER';
  }
  return probe.type;
}

function getProbeIcon(probe: Probe): string {
  switch (probe.type) {
    case 'SYSINFO': return 'fa-solid fa-microchip';
    case 'NETINFO': return 'fa-solid fa-network-wired';
    case 'MTR': return 'fa-solid fa-route';
    case 'PING': return 'fa-solid fa-satellite-dish';
    case 'RPERF': return probe.config?.server ? 'fa-solid fa-server' : 'fa-solid fa-gauge-high';
    case 'TRAFFICSIM': return probe.config?.server ? 'fa-solid fa-tower-broadcast' : 'fa-solid fa-chart-line';
    case 'SPEEDTEST': return 'fa-solid fa-gauge-high';
    case 'SPEEDTEST_SERVERS': return 'fa-solid fa-list-check';
    default: return 'fa-solid fa-cube';
  }
}

function getProbeColor(probe: Probe): string {
  switch (probe.type) {
    case 'SYSINFO': 
    case 'NETINFO': 
    case 'SPEEDTEST':
    case 'SPEEDTEST_SERVERS': return 'secondary';
    case 'MTR': return 'info';
    case 'PING': return 'success';
    case 'RPERF': return probe.config?.server ? 'primary' : 'warning';
    case 'TRAFFICSIM': return probe.config?.server ? 'purple' : 'danger';
    default: return 'dark';
  }
}

function getProbeDescription(probe: Probe): string {
  switch (probe.type) {
    case 'SYSINFO': return 'System information monitoring';
    case 'NETINFO': return 'Network interface monitoring';
    case 'SPEEDTEST': return 'Network speed testing';
    case 'SPEEDTEST_SERVERS': return 'Speed test server discovery';
    case 'MTR': return 'Multi-hop network trace';
    case 'PING': return 'Network latency monitoring';
    case 'RPERF': 
      return probe.config?.server ? 'Performance test server' : 'Network performance testing';
    case 'TRAFFICSIM': 
      return probe.config?.server ? 'Traffic simulation server' : 'Network traffic simulation';
    default: return 'Custom probe monitoring';
  }
}

function getTargetDisplay(probe: Probe): { type: string, name: string, value: string } | null {
  if (!probe.config?.target?.[0]) return null;
  
  const target = probe.config.target[0];
  
  if (target.agent && target.agent !== '000000000000000000000000') {
    return {
      type: 'agent',
      name: getAgentName(target.agent),
      value: target.target || 'N/A'
    };
  }
  
  if (target.group && target.group !== '000000000000000000000000') {
    return {
      type: 'group',
      name: getGroupName(target.group),
      value: target.target || 'N/A'
    };
  }
  
  if (target.target) {
    return {
      type: 'target',
      name: 'Direct',
      value: target.target
    };
  }
  
  return null;
}

function isBuiltInProbe(probe: Probe): boolean {
  return ['SYSINFO', 'NETINFO', 'SPEEDTEST', 'SPEEDTEST_SERVERS'].includes(probe.type);
}

function isServerProbe(probe: Probe): boolean {
  return (probe.type === 'RPERF' || probe.type === 'TRAFFICSIM') && probe.config?.server;
}

function canDeleteProbe(probe: Probe): boolean {
  return !isBuiltInProbe(probe) && !isServerProbe(probe);
}

function getAgentName(id: string): string {
  if (!id || id === '000000000000000000000000') return 'Unknown';
  const agent = state.agents.find(a => a.id === id);
  return agent?.name || 'Unknown Agent';
}

function getGroupName(id: string): string {
  if (!id || id === '000000000000000000000000') return 'Unknown';
  const group = state.agentGroups.find(g => g.id === id);
  return group?.name || 'Unknown Group';
}

// Computed properties
const probeTypes = computed(() => {
  const types = new Set(state.probes.map(p => getProbeTypeLabel(p)));
  return ['all', ...Array.from(types)];
});

const filteredProbes = computed(() => {
  let filtered = state.probes;
  
  // Filter by type
  if (state.selectedType !== 'all') {
    filtered = filtered.filter(p => getProbeTypeLabel(p) === state.selectedType);
  }
  
  // Filter by search
  if (state.searchQuery) {
    const query = state.searchQuery.toLowerCase();
    filtered = filtered.filter(p => {
      const type = getProbeTypeLabel(p).toLowerCase();
      const target = p.config?.target?.[0]?.target?.toLowerCase() || '';
      const agentName = getAgentName(p.config?.target?.[0]?.agent || '').toLowerCase();
      return type.includes(query) || target.includes(query) || agentName.includes(query);
    });
  }
  
  return filtered;
});

// Built-in/Agent probes (non-removable)
const builtInProbes = computed(() => {
  return filteredProbes.value.filter(p => 
    ['SYSINFO', 'NETINFO', 'SPEEDTEST', 'SPEEDTEST_SERVERS'].includes(p.type)
  );
});

// Server/Collector probes
const serverProbes = computed(() => {
  return filteredProbes.value.filter(p => {
    // RPERF servers and TRAFFICSIM servers
    return (p.type === 'RPERF' && p.config?.server) || 
           (p.type === 'TRAFFICSIM' && p.config?.server);
  });
});

// General probes (everything else)
const generalProbes = computed(() => {
  return filteredProbes.value.filter(p => {
    // Not a built-in probe
    if (['SYSINFO', 'NETINFO', 'SPEEDTEST', 'SPEEDTEST_SERVERS'].includes(p.type)) {
      return false;
    }
    // Not a server probe
    if ((p.type === 'RPERF' && p.config?.server) || 
        (p.type === 'TRAFFICSIM' && p.config?.server)) {
      return false;
    }
    return true;
  });
});

function isSystemProbe(probe: Probe): boolean {
  return probe.type === 'SYSINFO' || probe.type === 'NETINFO';
}

onMounted(async () => {
  let id = router.currentRoute.value.params["idParam"] as string
  if (!id) return

  try {
    const agentRes = await agentService.getAgent(id);
    state.agent = agentRes.data as Agent;

    const [agentsRes, siteRes] = await Promise.all([
      agentService.getSiteAgents(state.agent.site),
      siteService.getSite(state.agent.site)
    ]);

    state.agents = agentsRes.data as Agent[];
    state.site = siteRes.data as Site;

    // Get agent groups if available
    try {
      const groupsRes = await siteService.getAgentGroups(state.agent.site);
      state.agentGroups = groupsRes.data as AgentGroup[];
    } catch (e) {
      console.log('Agent groups not available');
    }

    const probesRes = await probeService.getAgentProbes(state.agent.id);
    state.probes = probesRes.data as Probe[] || [];
    
    state.ready = true;
    state.loading = false;
  } catch (error) {
    console.error('Error loading data:', error);
    state.loading = false;
  }
})

const router = core.router()
</script>

<template>
  <div class="container-fluid">
    <Title 
      title="Manage Probes" 
      subtitle="Configure monitoring probes for this agent" 
      :history="[
        {title: 'workspaces', link: '/workspaces'},
        {title: state.site.name || 'Loading...', link: `/workspace/${state.site.id}`},
        {title: state.agent.name || 'Loading...', link: `/agent/${state.agent.id}`}
      ]">
      <div class="d-flex gap-2">
        <router-link :to="`/probe/${state.agent.id}/new`" class="btn btn-primary">
          <i class="fa-solid fa-plus"></i>&nbsp;Add Probe
        </router-link>
      </div>
    </Title>

    <!-- Loading State -->
    <div v-if="state.loading" class="loading-container">
      <div class="spinner-border text-primary" role="status">
        <span class="visually-hidden">Loading...</span>
      </div>
      <p class="loading-text">Loading probes...</p>
    </div>

    <!-- Main Content -->
    <div v-else-if="state.ready">
      <!-- Filters and Stats -->
      <div class="filters-section" v-if="state.probes.length > 0">
        <div class="filters-row">
          <div class="search-box">
            <i class="fa-solid fa-search search-icon"></i>
            <input 
              v-model="state.searchQuery" 
              type="text" 
              class="form-control search-input" 
              placeholder="Search probes by type, target, or agent..."
            >
          </div>
          
          <div class="type-filter">
            <select v-model="state.selectedType" class="form-select">
              <option value="all">All Types</option>
              <option v-for="type in probeTypes.slice(1)" :key="type" :value="type">
                {{ type }}
              </option>
            </select>
          </div>
        </div>
        
        <div class="stats-row">
          <div class="stat-chip">
            <i class="fa-solid fa-cube"></i>
            <span>{{ filteredProbes.length }} Total Probes</span>
          </div>
          <div class="stat-chip" v-if="builtInProbes.length > 0">
            <i class="fa-solid fa-cog"></i>
            <span>{{ builtInProbes.length }} Built-in</span>
          </div>
          <div class="stat-chip" v-if="serverProbes.length > 0">
            <i class="fa-solid fa-server"></i>
            <span>{{ serverProbes.length }} Servers</span>
          </div>
          <div class="stat-chip" v-if="generalProbes.length > 0">
            <i class="fa-solid fa-cubes"></i>
            <span>{{ generalProbes.length }} General</span>
          </div>
        </div>
      </div>

      <!-- Empty State -->
      <div v-if="state.probes.length === 0" class="empty-state-card">
        <div class="empty-state">
          <i class="fa-solid fa-cube"></i>
          <h5>No Probes Configured</h5>
          <p>Add probes to start monitoring this agent's performance and connectivity.</p>
          <router-link :to="`/probe/${state.agent.id}/new`" class="btn btn-primary">
            <i class="fa-solid fa-plus"></i> Add First Probe
          </router-link>
        </div>
      </div>

      <!-- No Results State -->
      <div v-else-if="filteredProbes.length === 0" class="empty-state-card">
        <div class="empty-state">
          <i class="fa-solid fa-search"></i>
          <h5>No Probes Found</h5>
          <p>Try adjusting your search or filter criteria.</p>
          <button @click="state.searchQuery = ''; state.selectedType = 'all'" class="btn btn-outline-primary">
            Clear Filters
          </button>
        </div>
      </div>

      <!-- Probes List -->
      <div v-else class="probes-container">
        <!-- Built-in/Agent Probes -->
        <div v-if="builtInProbes.length > 0" class="probe-section">
          <h6 class="section-title">
            <i class="fa-solid fa-cog"></i>
            Built-in
            <span class="section-subtitle"></span>
          </h6>
          <div class="probes-grid">
            <div v-for="probe in builtInProbes" :key="probe.id" class="probe-card built-in">
              <div class="probe-header">
                <div class="probe-icon" :class="`bg-${getProbeColor(probe)}`">
                  <i :class="getProbeIcon(probe)"></i>
                </div>
                <div class="probe-info">
                  <h6 class="probe-type">{{ getProbeTypeLabel(probe) }}</h6>
                  <p class="probe-description">{{ getProbeDescription(probe) }}</p>
                </div>
                <div class="probe-badge">
                  <span class="badge bg-secondary">
                    <i class="fa-solid fa-lock"></i>
                    Built-in
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Server/Collector Probes -->
        <div v-if="serverProbes.length > 0" class="probe-section">
          <h6 class="section-title">
            <i class="fa-solid fa-server"></i>
            Servers & Collectors
            <span class="section-subtitle"></span>
          </h6>
          <div class="probes-grid">
            <div v-for="probe in serverProbes" :key="probe.id" class="probe-card server">
              <div class="probe-header">
                <div class="probe-icon" :class="`bg-${getProbeColor(probe)}`">
                  <i :class="getProbeIcon(probe)"></i>
                </div>
                <div class="probe-info">
                  <h6 class="probe-type">{{ getProbeTypeLabel(probe) }}</h6>
                  <p class="probe-description">{{ getProbeDescription(probe) }}</p>
                  <div v-if="probe.config?.port" class="probe-target">
                    <i class="fa-solid fa-ethernet"></i>
                    <span class="target-name">Port</span>
                    <span class="target-value">{{ probe.config.port }}</span>
                  </div>
                </div>
                <div class="probe-badge">
                  <span class="badge bg-primary">
                    <i class="fa-solid fa-tower-broadcast"></i>
                    Server
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- General Probes -->
        <div v-if="generalProbes.length > 0" class="probe-section">
          <h6 class="section-title">
            <i class="fa-solid fa-cubes"></i>
            Probes
            <span class="section-subtitle"></span>
          </h6>
          <div class="probes-grid">
            <div v-for="probe in generalProbes" :key="probe.id" class="probe-card">
              <div class="probe-header">
                <div class="probe-icon" :class="`bg-${getProbeColor(probe)}`">
                  <i :class="getProbeIcon(probe)"></i>
                </div>
                <div class="probe-info">
                  <h6 class="probe-type">{{ getProbeTypeLabel(probe) }}</h6>
                  <p class="probe-description">{{ getProbeDescription(probe) }}</p>
                  <div v-if="getTargetDisplay(probe)" class="probe-target">
                    <i :class="getTargetDisplay(probe).type === 'agent' ? 'fa-solid fa-robot' : 'fa-solid fa-bullseye'"></i>
                    <span class="target-name">{{ getTargetDisplay(probe).name }}</span>
                    <span class="target-value">{{ getTargetDisplay(probe).value }}</span>
                  </div>
                </div>
                <router-link 
                  :to="`/probe/${probe.id}/delete`" 
                  class="probe-action delete"
                  title="Remove probe"
                >
                  <i class="fa-solid fa-trash"></i>
                </router-link>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Loading State */
.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 400px;
  background: white;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
}

.loading-text {
  margin-top: 1rem;
  color: #6b7280;
}

/* Filters Section */
.filters-section {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 1.25rem;
  margin-bottom: 1.5rem;
}

.filters-row {
  display: flex;
  gap: 1rem;
  margin-bottom: 1rem;
  flex-wrap: wrap;
}

.search-box {
  flex: 1;
  min-width: 300px;
  position: relative;
}

.search-icon {
  position: absolute;
  left: 1rem;
  top: 50%;
  transform: translateY(-50%);
  color: #6b7280;
}

.search-input {
  padding-left: 2.75rem;
  border-radius: 6px;
  border: 1px solid #e5e7eb;
}

.search-input:focus {
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.type-filter {
  min-width: 200px;
}

.type-filter .form-select {
  border-radius: 6px;
  border: 1px solid #e5e7eb;
}

.stats-row {
  display: flex;
  gap: 1rem;
  flex-wrap: wrap;
}

.stat-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.375rem 0.875rem;
  background: #f3f4f6;
  border-radius: 999px;
  font-size: 0.875rem;
  color: #4b5563;
}

.stat-chip i {
  font-size: 0.875rem;
  color: #6b7280;
}

/* Empty States */
.empty-state-card {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 3rem 2rem;
}

.empty-state {
  text-align: center;
}

.empty-state i {
  font-size: 3rem;
  color: #e5e7eb;
  margin-bottom: 1rem;
}

.empty-state h5 {
  color: #1f2937;
  margin-bottom: 0.5rem;
}

.empty-state p {
  color: #6b7280;
  margin-bottom: 1.5rem;
}

/* Probes Container */
.probes-container {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.probe-section {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 1.25rem;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin: 0 0 1rem 0;
  font-size: 1rem;
  font-weight: 600;
  color: #374151;
}

.section-title i {
  color: #6b7280;
}

.section-subtitle {
  margin-left: auto;
  font-size: 0.813rem;
  font-weight: 400;
  color: #6b7280;
}

.section-subtitle {
  margin-left: auto;
  font-size: 0.813rem;
  font-weight: 400;
  color: #6b7280;
}

.section-subtitle {
  margin-left: auto;
  font-size: 0.813rem;
  font-weight: 400;
  color: #6b7280;
}

/* Probes Grid */
.probes-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
  gap: 1rem;
}

.probe-card {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 1rem;
  background: #f9fafb;
  transition: all 0.2s;
}

.probe-card:hover {
  background: white;
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
  transform: translateY(-2px);
}

.probe-card.built-in {
  background: #f3f4f6;
  border-color: #d1d5db;
}

.probe-card.server {
  background: #eff6ff;
  border-color: #dbeafe;
}

.probe-header {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.probe-icon {
  width: 3rem;
  height: 3rem;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-size: 1.25rem;
  flex-shrink: 0;
}

.probe-icon.bg-dark {
  background: #374151;
}

.probe-icon.bg-info {
  background: #3b82f6;
}

.probe-icon.bg-success {
  background: #10b981;
}

.probe-icon.bg-primary {
  background: #6366f1;
}

.probe-icon.bg-warning {
  background: #f59e0b;
}

.probe-icon.bg-danger {
  background: #ef4444;
}

.probe-icon.bg-purple {
  background: #8b5cf6;
}

.probe-icon.bg-secondary {
  background: #6b7280;
}

.probe-info {
  flex: 1;
  min-width: 0;
}

.probe-type {
  margin: 0;
  font-size: 0.875rem;
  font-weight: 600;
  color: #1f2937;
}

.probe-description {
  margin: 0.25rem 0 0 0;
  font-size: 0.813rem;
  color: #6b7280;
}

.probe-target {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.25rem;
  font-size: 0.813rem;
  color: #4b5563;
}

.probe-target i {
  font-size: 0.75rem;
  color: #9ca3af;
}

.target-name {
  font-weight: 500;
  color: #374151;
}

.target-value {
  padding: 0.125rem 0.5rem;
  background: #e5e7eb;
  border-radius: 4px;
  font-family: monospace;
  font-size: 0.75rem;
  color: #4b5563;
}

.probe-badge {
  flex-shrink: 0;
}

.probe-action {
  padding: 0.5rem;
  border-radius: 6px;
  color: #6b7280;
  transition: all 0.2s;
  text-decoration: none;
}

.probe-action:hover {
  background: #fee2e2;
  color: #dc2626;
}

.probe-action.delete:hover {
  background: #fee2e2;
  color: #dc2626;
}

/* Responsive */
@media (max-width: 768px) {
  .filters-row {
    flex-direction: column;
  }
  
  .search-box {
    min-width: 100%;
  }
  
  .type-filter {
    width: 100%;
  }
  
  .probes-grid {
    grid-template-columns: 1fr;
  }
}
</style>