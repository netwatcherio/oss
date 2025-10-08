<script lang="ts" setup>
import {onMounted, reactive, computed} from "vue";
import Title from "@/components/Title.vue";
import core from "@/core";
import {Agent, Probe, type Workspace} from "@/types";
import {AgentService, ProbeService, WorkspaceService} from "@/services/apiService";

const state = reactive({
  probes: [] as Probe[],
  workspace: {} as Workspace,
  ready: false,
  loading: true,
  agent: {} as Agent,
  agents: [] as Agent[],
  searchQuery: '',
  selectedType: 'all'
})


// todo load other

const router = core.router()

onMounted(async () => {
  let agentID = router.currentRoute.value.params["aID"] as string
  let workspaceID = router.currentRoute.value.params["wID"] as string
  if (!agentID || !workspaceID) return

  WorkspaceService.get(workspaceID).then(res => {
    state.workspace = res as Workspace
  })
  AgentService.get(workspaceID, agentID).then(res => {
    state.agent = res as Agent
  })

  AgentService.list(workspaceID).then(res => {
    state.agents = res.data as Agent[]
  })

  ProbeService.list(workspaceID, agentID).then(res => {
    state.probes = res as Probe[] || [];
  })


  try {

    state.ready = true;
    state.loading = false;
  } catch (error) {
    console.error('Error loading data:', error);
    state.loading = false;
  }
})

// Helper functions
function getProbeTypeLabel(probe: Probe): string {
  /*if (probe.type === 'RPERF' && probe.config?.server) {
    return 'RPERF SERVER';
  }
  if (probe.type === 'TRAFFICSIM' && probe.config?.server) {
    return 'TRAFFIC SIM SERVER';
  }*/
  return probe.type;
}

function getProbeIcon(probe: Probe): string {
  switch (probe.type) {
    case 'SYSINFO': return 'fa-solid fa-microchip';
    case 'NETINFO': return 'fa-solid fa-network-wired';
    case 'MTR': return 'fa-solid fa-route';
    case 'PING': return 'fa-solid fa-satellite-dish';
    /*case 'RPERF': return probe.config?.server ? 'fa-solid fa-server' : 'fa-solid fa-gauge-high';
    case 'TRAFFICSIM': return probe.config?.server ? 'fa-solid fa-tower-broadcast' : 'fa-solid fa-chart-line';*/
    case 'SPEEDTEST': return 'fa-solid fa-gauge-high';
    case 'SPEEDTEST_SERVERS': return 'fa-solid fa-list-check';
    default: return 'fa-solid fa-cube';
  }
}

function getProbeColor(probe: Probe): string {
  switch (probe.type) {
    case 'SYSINFO':
    case 'NETINFO': return 'blue';
    case 'SPEEDTEST':
    case 'SPEEDTEST_SERVERS': return 'teal';
    case 'MTR': return 'cyan';
    case 'PING': return 'green';
    /*case 'RPERF': return probe.config?.server ? 'purple' : 'orange';
    case 'TRAFFICSIM': return probe.config?.server ? 'indigo' : 'red';*/
    default: return 'gray';
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
   /* case 'RPERF':
      return probe.config?.server ? 'Performance test server' : 'Network performance testing';
    case 'TRAFFICSIM':
      return probe.config?.server ? 'Traffic simulation server' : 'Network traffic simulation';*/
    default: return 'Custom probe monitoring';
  }
}

function getTargetDisplay(probe: Probe): { type: string, name: string, value: string } | null {
 /* if (!probe.config?.target?.[0]) return null;

  const target = probe.config.target[0];*/

  /*if (target.agent && target.agent !== '000000000000000000000000') {
    return {
      type: 'agent',
      name: getAgentName(target.agent),
      value: target.target || 'N/A'
    };
  }*/

  /*if (target.group && target.group !== '000000000000000000000000') {
    return {
      type: 'group',
      name: getGroupName(target.group),
      value: target.target || 'N/A'
    };
  }*/

  /*if (target.target) {
    return {
      type: 'target',
      name: 'Direct',
      value: target.target
    };
  }
*/
  return null;
}

function isBuiltInProbe(probe: Probe): boolean {
  return ['SYSINFO', 'NETINFO', 'SPEEDTEST', 'SPEEDTEST_SERVERS'].includes(probe.type);
}

function isServerProbe(probe: Probe): boolean {
  return (probe.type === 'RPERF' || probe.type === 'TRAFFICSIM') && probe?.server;
}

function canDeleteProbe(probe: Probe): boolean {
  return !isBuiltInProbe(probe) && !isServerProbe(probe);
}

function getAgentName(id: number): string {
  if (!id || id == 0) return 'Unknown';
  const agent = state.agents.find(a => a.id == id);
  return agent?.name || 'Unknown Agent';
}
/*
function getGroupName(id: string): string {
  if (!id || id === '000000000000000000000000') return 'Unknown';
  const group = state.agentGroups.find(g => g.id === id);
  return group?.name || 'Unknown Group';
}*/

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
  /*if (state.searchQuery) {
    const query = state.searchQuery.toLowerCase();
    filtered = filtered.filter(p => {
      const type = getProbeTypeLabel(p).toLowerCase();
      const target = p.config?.target?.[0]?.target?.toLowerCase() || '';
      const agentName = getAgentName(p.config?.target?.[0]?.agent || '').toLowerCase();
      return type.includes(query) || target.includes(query) || agentName.includes(query);
    });
  }*/

  return filtered;
});

// Categorized probes
const builtInProbes = computed(() => {
  return filteredProbes.value.filter(p =>
      ['SYSINFO', 'NETINFO', 'SPEEDTEST', 'SPEEDTEST_SERVERS'].includes(p.type)
  );
});

const serverProbes = computed(() => {
  return filteredProbes.value.filter(p => {
    /*return (p.type === 'RPERF' && p.config?.server) ||
        (p.type === 'TRAFFICSIM' && p.config?.server);*/
  });
});

const generalProbes = computed(() => {
  return filteredProbes.value.filter(p => {
    if (['SYSINFO', 'NETINFO', 'SPEEDTEST', 'SPEEDTEST_SERVERS'].includes(p.type)) {
      return false;
    }
    /*if ((p.type === 'RPERF' && p.config?.server) ||
        (p.type === 'TRAFFICSIM' && p.config?.server)) {
      return false;
    }*/
    return true;
  });
});
</script>

<template>
  <div class="container-fluid">
    <Title
        title="Manage Probes"
        subtitle="Configure monitoring probes for this agent"
        :history="[
        {title: 'workspaces', link: '/workspaces'},
        {title: state.workspace.name || 'Loading...', link: `/workspaces/${state.workspace.id}`},
        {title: state.agent.name || 'Loading...', link: `/workspaces/${state.workspace.id}/agent/${state.agent.id}`}
      ]">
      <div class="d-flex gap-2">
        <router-link :to="`/workspaces/${state.workspace.id}/agent/${state.agent.id}/probe/new`" class="btn btn-primary" :class="{'disabled': state.loading}">
          <i class="fa-solid fa-plus"></i>&nbsp;Add Probe
        </router-link>
      </div>
    </Title>

    <!-- Main Content Container - Always visible -->

    <div class="content-wrapper">
      <!-- Filters Section - Always show structure -->
      <div class="filters-section">
        <div class="filters-row">
          <div class="search-box">
            <i class="fa-solid fa-search search-icon"></i>
            <input
                v-model="state.searchQuery"
                type="text"
                class="form-control search-input"
                placeholder="Search probes by type, target, or agent..."
                :disabled="state.loading"
            >
          </div>

          <div class="type-filter">
            <select v-model="state.selectedType" class="form-select" :disabled="state.loading">
              <option value="all">All Types</option>
              <option v-for="type in probeTypes.slice(1)" :key="type" :value="type">
                {{ type }}
              </option>
            </select>
          </div>
        </div>

        <div class="stats-row">
          <div class="stat-chip" :class="{'loading': state.loading}">
            <i class="fa-solid fa-cube"></i>
            <span v-if="state.loading" class="skeleton-text">-- Total</span>
            <span v-else>{{ filteredProbes.length }} Total Probes</span>
          </div>
          <div class="stat-chip" :class="{'loading': state.loading}">
            <i class="fa-solid fa-cog"></i>
            <span v-if="state.loading" class="skeleton-text">-- Built-in</span>
            <span v-else>{{ builtInProbes.length }} Built-in</span>
          </div>
          <div class="stat-chip" :class="{'loading': state.loading}">
            <i class="fa-solid fa-server"></i>
            <span v-if="state.loading" class="skeleton-text">-- Servers</span>
            <span v-else>{{ serverProbes.length }} Servers</span>
          </div>
          <div class="stat-chip" :class="{'loading': state.loading}">
            <i class="fa-solid fa-cubes"></i>
            <span v-if="state.loading" class="skeleton-text">-- General</span>
            <span v-else>{{ generalProbes.length }} General</span>
          </div>
        </div>
      </div>

      <!-- Loading State -->
      <div v-if="state.loading" class="probes-container">
        <!-- Built-in Section Skeleton -->
        <div class="probe-section">
          <h6 class="section-title">
            <i class="fa-solid fa-cog"></i>
            Built-in
          </h6>
          <div class="probes-grid">
            <div v-for="i in 2" :key="`built-in-skeleton-${i}`" class="probe-card skeleton">
              <div class="probe-header">
                <div class="probe-icon skeleton-box"></div>
                <div class="probe-info">
                  <div class="skeleton-text probe-type-skeleton"></div>
                  <div class="skeleton-text probe-desc-skeleton"></div>
                </div>
                <div class="skeleton-badge"></div>
              </div>
            </div>
          </div>
        </div>

        <!-- General Section Skeleton -->
        <div class="probe-section">
          <h6 class="section-title">
            <i class="fa-solid fa-cubes"></i>
            Probes
          </h6>
          <div class="probes-grid">
            <div v-for="i in 3" :key="`general-skeleton-${i}`" class="probe-card skeleton">
              <div class="probe-header">
                <div class="probe-icon skeleton-box"></div>
                <div class="probe-info">
                  <div class="skeleton-text probe-type-skeleton"></div>
                  <div class="skeleton-text probe-desc-skeleton"></div>
                  <div class="probe-target">
                    <span class="skeleton-text target-skeleton"></span>
                  </div>
                </div>
                <div class="skeleton-action"></div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Empty State -->
      <div v-else-if="state.probes.length === 0" class="empty-state-card">
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

      <!-- Actual Probes List -->
      <div v-else class="probes-container">
        <!-- Built-in/Agent Probes -->
        <div v-if="builtInProbes.length > 0" class="probe-section">
          <h6 class="section-title">
            <i class="fa-solid fa-cog"></i>
            Built-in
            <span class="section-count">{{ builtInProbes.length }}</span>
          </h6>
          <div class="probes-grid">
            <div v-for="probe in builtInProbes" :key="probe.id" class="probe-card built-in">
              <div class="probe-header">
                <div class="probe-icon" :class="`icon-${getProbeColor(probe)}`">
                  <i :class="getProbeIcon(probe)"></i>
                </div>
                <div class="probe-info">
                  <h6 class="probe-type">{{ getProbeTypeLabel(probe) }}</h6>
                  <p class="probe-description">{{ getProbeDescription(probe) }}</p>
                </div>
                <div class="probe-badge">
                  <span class="badge badge-secondary">
                    <i class="fa-solid fa-lock"></i>
                    System
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
            <span class="section-count">{{ serverProbes.length }}</span>
          </h6>
          <div class="probes-grid">
            <div v-for="probe in serverProbes" :key="probe.id" class="probe-card server">
              <div class="probe-header">
                <div class="probe-icon" :class="`icon-${getProbeColor(probe)}`">
                  <i :class="getProbeIcon(probe)"></i>
                </div>
                <div class="probe-info">
                  <h6 class="probe-type">{{ getProbeTypeLabel(probe) }}</h6>
                  <p class="probe-description">{{ getProbeDescription(probe) }}</p>
                  <div v-if="probe.config?.port" class="probe-target">
                    <i class="fa-solid fa-ethernet"></i>
                    <span class="target-label">Port:</span>
                    <span class="target-value">{{ probe.config.port }}</span>
                  </div>
                </div>
                <div class="probe-badge">
                  <span class="badge badge-primary">
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
            <span class="section-count">{{ generalProbes.length }}</span>
          </h6>
          <div class="probes-grid">
            <div v-for="probe in generalProbes" :key="probe.id" class="probe-card">
              <div class="probe-header">
                <div class="probe-icon" :class="`icon-${getProbeColor(probe)}`">
                  <i :class="getProbeIcon(probe)"></i>
                </div>
                <div class="probe-info">
                  <h6 class="probe-type">{{ getProbeTypeLabel(probe) }}</h6>
                  <p class="probe-description">{{ getProbeDescription(probe) }}</p>
                  <div v-if="getTargetDisplay(probe)" class="probe-target">
                    <i :class="getTargetDisplay(probe).type === 'agent' ? 'fa-solid fa-robot' : 'fa-solid fa-bullseye'"></i>
                    <span class="target-label">{{ getTargetDisplay(probe).name }}:</span>
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
/* Content Wrapper */
.content-wrapper {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

/* Loading Animations */
@keyframes skeleton-shimmer {
  0% {
    transform: translateX(-100%);
  }
  100% {
    transform: translateX(100%);
  }
}

.skeleton-text {
  display: inline-block;
  background: #e5e7eb;
  border-radius: 4px;
  position: relative;
  overflow: hidden;
}

.skeleton-text::after {
  content: "";
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  left: 0;
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.4), transparent);
  animation: skeleton-shimmer 1.5s infinite;
}

.skeleton-box {
  background: #e5e7eb;
  position: relative;
  overflow: hidden;
  width: 100%;
  height: 100%;
}

.skeleton-box::after {
  content: "";
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  left: 0;
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.4), transparent);
  animation: skeleton-shimmer 1.5s infinite;
}

.probe-type-skeleton {
  width: 120px;
  height: 20px;
  margin-bottom: 0.5rem;
}

.probe-desc-skeleton {
  width: 180px;
  height: 16px;
}

.target-skeleton {
  width: 150px;
  height: 16px;
}

.skeleton-badge {
  width: 80px;
  height: 28px;
  background: #e5e7eb;
  border-radius: 999px;
  position: relative;
  overflow: hidden;
}

.skeleton-badge::after {
  content: "";
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  left: 0;
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.4), transparent);
  animation: skeleton-shimmer 1.5s infinite;
}

.skeleton-action {
  width: 36px;
  height: 36px;
  background: #e5e7eb;
  border-radius: 6px;
  position: relative;
  overflow: hidden;
}

.skeleton-action::after {
  content: "";
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  left: 0;
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.4), transparent);
  animation: skeleton-shimmer 1.5s infinite;
}

/* Filters Section */
.filters-section {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 1.25rem;
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
  pointer-events: none;
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

.search-input:disabled {
  background: #f9fafb;
  cursor: not-allowed;
}

.type-filter {
  min-width: 200px;
}

.type-filter .form-select {
  border-radius: 6px;
  border: 1px solid #e5e7eb;
}

.type-filter .form-select:disabled {
  background: #f9fafb;
  cursor: not-allowed;
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

.stat-chip.loading {
  min-width: 100px;
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
  padding: 4rem 2rem;
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
  font-weight: 600;
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

.section-count {
  margin-left: auto;
  padding: 0.125rem 0.5rem;
  background: #f3f4f6;
  border-radius: 999px;
  font-size: 0.75rem;
  font-weight: 500;
  color: #6b7280;
}

/* Probes Grid */
.probes-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(380px, 1fr));
  gap: 1rem;
}

.probe-card {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 1.25rem;
  background: white;
  transition: all 0.2s;
}

.probe-card:hover:not(.skeleton) {
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
  transform: translateY(-2px);
}

.probe-card.built-in {
  background: #f9fafb;
}

.probe-card.server {
  background: #f0f9ff;
}

.probe-card.skeleton {
  pointer-events: none;
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

/* Icon Colors */
.icon-blue {
  background: #3b82f6;
}

.icon-green {
  background: #10b981;
}

.icon-purple {
  background: #8b5cf6;
}

.icon-orange {
  background: #f59e0b;
}

.icon-red {
  background: #ef4444;
}

.icon-teal {
  background: #14b8a6;
}

.icon-cyan {
  background: #06b6d4;
}

.icon-indigo {
  background: #6366f1;
}

.icon-gray {
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
  line-height: 1.4;
}

.probe-description {
  margin: 0.25rem 0 0 0;
  font-size: 0.813rem;
  color: #6b7280;
  line-height: 1.4;
}

.probe-target {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  margin-top: 0.5rem;
  font-size: 0.813rem;
  color: #4b5563;
}

.probe-target i {
  font-size: 0.75rem;
  color: #9ca3af;
}

.target-label {
  font-weight: 500;
  color: #374151;
}

.target-value {
  padding: 0.125rem 0.5rem;
  background: #e5e7eb;
  border-radius: 4px;
  font-family: monospace;
  font-size: 0.75rem;
  color: #1f2937;
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Badges */
.probe-badge {
  flex-shrink: 0;
}

.badge {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.25rem 0.75rem;
  border-radius: 999px;
  font-size: 0.75rem;
  font-weight: 500;
}

.badge-secondary {
  background: #e5e7eb;
  color: #4b5563;
}

.badge-primary {
  background: #dbeafe;
  color: #1e40af;
}

.badge i {
  font-size: 0.75rem;
}

/* Probe Actions */
.probe-action {
  padding: 0.5rem;
  border-radius: 6px;
  color: #6b7280;
  transition: all 0.2s;
  text-decoration: none;
  display: flex;
  align-items: center;
  justify-content: center;
}

.probe-action:hover {
  background: #fee2e2;
  color: #dc2626;
}

/* Disabled state */
.btn.disabled {
  opacity: 0.6;
  cursor: not-allowed;
  pointer-events: none;
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

  .probe-card {
    padding: 1rem;
  }

  .probe-target {
    flex-wrap: wrap;
  }

  .target-value {
    max-width: 100%;
  }
}

@media (max-width: 576px) {
  .stat-chip {
    font-size: 0.813rem;
    padding: 0.25rem 0.625rem;
  }

  .empty-state-card {
    padding: 3rem 1.5rem;
  }
}
</style>