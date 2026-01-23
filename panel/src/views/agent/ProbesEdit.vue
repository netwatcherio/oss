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
  selectedType: 'all',
  // Modal state
  showProbeModal: false,
  selectedProbe: null as Probe | null,
  // Copy modal state
  showCopyModal: false,
  copySelectedProbes: [] as number[],
  copyDestAgents: [] as number[],
  copyMatchTargets: false,
  copySkipDuplicates: true,
  copyBidirectional: false, // For agent probes: create reverse probes too
  copyLoading: false,
  copyResults: null as {
    created: number;
    skipped: number;
    errors: number;
    results: Array<{
      source_probe_id: number;
      dest_agent_id: number;
      new_probe_id?: number;
      skipped: boolean;
      skip_reason?: string;
      error?: string;
    }>;
  } | null
})

// Modal functions
function openProbeDetails(probe: Probe) {
  state.selectedProbe = probe;
  state.showProbeModal = true;
}

function closeProbeModal() {
  state.showProbeModal = false;
  state.selectedProbe = null;
}

function formatInterval(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
}

function formatDate(dateStr: string): string {
  if (!dateStr) return 'N/A';
  return new Date(dateStr).toLocaleString();
}

// Copy modal functions
function openCopyModal() {
  // Start with no probes selected - user must explicitly choose
  state.copySelectedProbes = [];
  state.copyDestAgents = [];
  state.copyMatchTargets = false;
  state.copySkipDuplicates = true;
  state.copyBidirectional = false;
  state.copyResults = null;
  state.showCopyModal = true;
}

function selectAllProbes() {
  state.copySelectedProbes = generalProbes.value.map(p => p.id);
}

function deselectAllProbes() {
  state.copySelectedProbes = [];
}

function closeCopyModal() {
  state.showCopyModal = false;
  state.copyResults = null;
}

function toggleProbeSelection(probeId: number) {
  const idx = state.copySelectedProbes.indexOf(probeId);
  if (idx >= 0) {
    state.copySelectedProbes.splice(idx, 1);
  } else {
    state.copySelectedProbes.push(probeId);
  }
}

function toggleAgentSelection(agentId: number) {
  const idx = state.copyDestAgents.indexOf(agentId);
  if (idx >= 0) {
    state.copyDestAgents.splice(idx, 1);
  } else {
    state.copyDestAgents.push(agentId);
  }
}

function isProbeSelected(probeId: number): boolean {
  return state.copySelectedProbes.includes(probeId);
}

function isAgentSelected(agentId: number): boolean {
  return state.copyDestAgents.includes(agentId);
}

// Get other agents (exclude current agent)
const otherAgents = computed(() => {
  return state.agents.filter(a => a.id !== state.agent.id);
});

async function executeCopy() {
  if (state.copySelectedProbes.length === 0 || state.copyDestAgents.length === 0) {
    return;
  }

  state.copyLoading = true;
  state.copyResults = null;

  try {
    const result = await ProbeService.copy(state.workspace.id, {
      source_agent_id: state.agent.id,
      dest_agent_ids: state.copyDestAgents,
      probe_ids: state.copySelectedProbes,
      match_targets: state.copyMatchTargets,
      skip_duplicates: state.copySkipDuplicates
    });
    state.copyResults = result;
  } catch (error) {
    console.error('Copy failed:', error);
    state.copyResults = {
      created: 0,
      skipped: 0,
      errors: 1,
      results: []
    };
  } finally {
    state.copyLoading = false;
  }
}


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
    case 'SYSINFO': return 'bi bi-cpu';
    case 'NETINFO': return 'bi bi-diagram-3';
    case 'MTR': return 'bi bi-signpost-split';
    case 'PING': return 'bi bi-broadcast';
    /*case 'RPERF': return probe.config?.server ? 'bi bi-server' : 'bi bi-speedometer2';
    case 'TRAFFICSIM': return probe.config?.server ? 'bi bi-broadcast-pin' : 'bi bi-graph-up';*/
    case 'SPEEDTEST': return 'bi bi-speedometer2';
    case 'SPEEDTEST_SERVERS': return 'bi bi-list-check';
    default: return 'bi bi-box';
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

interface TargetInfo {
  type: 'agent' | 'group' | 'literal';
  label: string;
  value: string;
  icon: string;
}

function getTargetInfos(probe: Probe): TargetInfo[] {
  if (!probe.targets || probe.targets.length === 0) return [];

  const results: TargetInfo[] = [];

  for (const t of probe.targets) {
    if (t.agent_id && t.agent_id !== 0) {
      // Agent target
      results.push({
        type: 'agent',
        label: 'Agent',
        value: getAgentName(t.agent_id),
        icon: 'bi bi-cpu'
      });
    } else if (t.group_id && t.group_id !== 0) {
      // Group target
      results.push({
        type: 'group',
        label: 'Group',
        value: `Group #${t.group_id}`,
        icon: 'bi bi-collection'
      });
    } else if (t.target) {
      // Literal target (IP/host)
      results.push({
        type: 'literal',
        label: 'Target',
        value: t.target,
        icon: 'bi bi-bullseye'
      });
    }
  }

  return results;
}

function hasTargets(probe: Probe): boolean {
  return probe.targets && probe.targets.length > 0;
}

function truncateText(text: string, maxLength: number = 24): string {
  if (text.length <= maxLength) return text;
  return text.substring(0, maxLength - 1) + '…';
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

// Check if any selected probes are AGENT type (for bidirectional option)
const hasSelectedAgentProbes = computed(() => {
  return state.copySelectedProbes.some(probeId => {
    const probe = state.probes.find(p => p.id === probeId);
    return probe?.type === 'AGENT';
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
        {title: state.agent.name || 'Loading...', link: `/workspaces/${state.workspace.id}/agents/${state.agent.id}`}
      ]">
      <div class="d-flex gap-2">
        <button @click="openCopyModal" class="btn btn-outline-secondary" :class="{'disabled': state.loading || generalProbes.length === 0}">
          <i class="bi bi-copy"></i>&nbsp;Copy to Agents
        </button>
        <router-link :to="`/workspaces/${state.workspace.id}/agents/${state.agent.id}/probes/new`" class="btn btn-primary" :class="{'disabled': state.loading}">
          <i class="bi bi-plus-lg"></i>&nbsp;Add Probe
        </router-link>
      </div>
    </Title>

    <!-- Main Content Container - Always visible -->

    <div class="content-wrapper">
      <!-- Filters Section - Always show structure -->
      <div class="filters-section">
        <div class="filters-row">
          <div class="search-box">
            <i class="bi bi-search search-icon"></i>
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
            <i class="bi bi-box"></i>
            <span v-if="state.loading" class="skeleton-text">-- Total</span>
            <span v-else>{{ filteredProbes.length }} Total Probes</span>
          </div>
          <div class="stat-chip" :class="{'loading': state.loading}">
            <i class="bi bi-gear"></i>
            <span v-if="state.loading" class="skeleton-text">-- Built-in</span>
            <span v-else>{{ builtInProbes.length }} Built-in</span>
          </div>
          <div class="stat-chip" :class="{'loading': state.loading}">
            <i class="bi bi-server"></i>
            <span v-if="state.loading" class="skeleton-text">-- Servers</span>
            <span v-else>{{ serverProbes.length }} Servers</span>
          </div>
          <div class="stat-chip" :class="{'loading': state.loading}">
            <i class="bi bi-boxes"></i>
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
            <i class="bi bi-gear"></i>
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
            <i class="bi bi-boxes"></i>
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
          <i class="bi bi-box"></i>
          <h5>No Probes Configured</h5>
          <p>Add probes to start monitoring this agent's performance and connectivity.</p>
          <router-link :to="`/probe/${state.agent.id}/new`" class="btn btn-primary">
            <i class="bi bi-plus-lg"></i> Add First Probe
          </router-link>
        </div>
      </div>

      <!-- No Results State -->
      <div v-else-if="filteredProbes.length === 0" class="empty-state-card">
        <div class="empty-state">
          <i class="bi bi-search"></i>
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
            <i class="bi bi-gear"></i>
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
                    <i class="bi bi-lock"></i>
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
            <i class="bi bi-server"></i>
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
                    <i class="bi bi-ethernet"></i>
                    <span class="target-label">Port:</span>
                    <span class="target-value">{{ probe.config.port }}</span>
                  </div>
                </div>
                <div class="probe-badge">
                  <span class="badge badge-primary">
                    <i class="bi bi-broadcast-pin"></i>
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
            <i class="bi bi-boxes"></i>
            Probes
            <span class="section-count">{{ generalProbes.length }}</span>
          </h6>
          <div class="probes-grid">
            <div v-for="probe in generalProbes" :key="probe.id" class="probe-card" @click="openProbeDetails(probe)">
              <div class="probe-header">
                <div class="probe-icon" :class="`icon-${getProbeColor(probe)}`">
                  <i :class="getProbeIcon(probe)"></i>
                </div>
                <div class="probe-info">
                  <h6 class="probe-type">{{ getProbeTypeLabel(probe) }}</h6>
                  <p class="probe-description">{{ getProbeDescription(probe) }}</p>
                  <!-- Target display section -->
                  <div v-if="hasTargets(probe)" class="probe-targets">
                    <div 
                      v-for="(targetInfo, idx) in getTargetInfos(probe).slice(0, 2)" 
                      :key="idx" 
                      class="target-pill"
                      :class="`target-${targetInfo.type}`"
                      :title="targetInfo.value"
                    >
                      <i :class="targetInfo.icon"></i>
                      <span class="target-type">{{ targetInfo.label }}:</span>
                      <span class="target-text">{{ truncateText(targetInfo.value) }}</span>
                    </div>
                    <div v-if="getTargetInfos(probe).length > 2" class="target-pill target-more">
                      +{{ getTargetInfos(probe).length - 2 }} more
                    </div>
                  </div>
                </div>
                <div class="probe-actions">
                  <button 
                    class="probe-action info" 
                    title="View details"
                    @click.stop="openProbeDetails(probe)"
                  >
                    <i class="bi bi-info-circle"></i>
                  </button>
                  <button 
                      class="probe-action delete"
                      title="View & delete probe"
                      @click.stop="openProbeDetails(probe)"
                  >
                    <i class="bi bi-trash"></i>
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Probe Details Modal -->
    <Teleport to="body">
      <div v-if="state.showProbeModal && state.selectedProbe" class="modal-backdrop" @click="closeProbeModal">
        <div class="modal-dialog" @click.stop>
          <div class="modal-header">
            <div class="modal-title-row">
              <div class="modal-icon" :class="`icon-${getProbeColor(state.selectedProbe)}`">
                <i :class="getProbeIcon(state.selectedProbe)"></i>
              </div>
              <div>
                <h5 class="modal-title">{{ getProbeTypeLabel(state.selectedProbe) }}</h5>
                <p class="modal-subtitle">{{ getProbeDescription(state.selectedProbe) }}</p>
              </div>
            </div>
            <button class="modal-close" @click="closeProbeModal">
              <i class="bi bi-x-lg"></i>
            </button>
          </div>
          
          <div class="modal-body">

            <!-- Configuration Details -->
            <div class="detail-section">
              <h6 class="detail-label">Configuration</h6>
              <div class="detail-grid">
                <div class="detail-item">
                  <span class="detail-key">Interval</span>
                  <span class="detail-value">{{ formatInterval(state.selectedProbe.interval_sec) }}</span>
                </div>
                <div class="detail-item">
                  <span class="detail-key">Timeout</span>
                  <span class="detail-value">{{ state.selectedProbe.timeout_sec }}s</span>
                </div>
                <div v-if="state.selectedProbe.count" class="detail-item">
                  <span class="detail-key">Count</span>
                  <span class="detail-value">{{ state.selectedProbe.count }}</span>
                </div>
                <div v-if="state.selectedProbe.duration_sec" class="detail-item">
                  <span class="detail-key">Duration</span>
                  <span class="detail-value">{{ state.selectedProbe.duration_sec }}s</span>
                </div>
              </div>
            </div>

            <!-- Targets -->
            <div v-if="hasTargets(state.selectedProbe)" class="detail-section">
              <h6 class="detail-label">Targets ({{ state.selectedProbe.targets.length }})</h6>
              <div class="targets-list">
                <div 
                  v-for="(targetInfo, idx) in getTargetInfos(state.selectedProbe)" 
                  :key="idx" 
                  class="target-row"
                  :class="`target-${targetInfo.type}`"
                >
                  <i :class="targetInfo.icon"></i>
                  <span class="target-label-modal">{{ targetInfo.label }}</span>
                  <span class="target-value-modal">{{ targetInfo.value }}</span>
                </div>
              </div>
            </div>

            <!-- Timestamps -->
            <div class="detail-section">
              <h6 class="detail-label">Timestamps</h6>
              <div class="detail-grid">
                <div class="detail-item">
                  <span class="detail-key">Created</span>
                  <span class="detail-value">{{ formatDate(state.selectedProbe.created_at) }}</span>
                </div>
                <div class="detail-item">
                  <span class="detail-key">Updated</span>
                  <span class="detail-value">{{ formatDate(state.selectedProbe.updated_at) }}</span>
                </div>
              </div>
            </div>

            <!-- Probe ID -->
            <div class="detail-section">
              <div class="detail-item">
                <span class="detail-key">Probe ID</span>
                <span class="detail-value mono">{{ state.selectedProbe.id }}</span>
              </div>
            </div>
          </div>

          <div class="modal-footer">
            <p class="modal-hint">
              <i class="bi bi-info-circle"></i>
              To modify this probe, delete it and create a new one.
            </p>
            <router-link
                :to="`/workspaces/${state.workspace.id}/agents/${state.agent.id}/probes/${state.selectedProbe.id}/delete`"
                class="btn btn-outline-danger"
                @click="closeProbeModal"
            >
              <i class="bi bi-trash"></i> Delete Probe
            </router-link>
            <button class="btn btn-secondary" @click="closeProbeModal">Close</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Copy to Agents Modal -->
    <Teleport to="body">
      <div v-if="state.showCopyModal" class="modal-backdrop" @click="closeCopyModal">
        <div class="modal-dialog copy-modal" @click.stop>
          <div class="modal-header">
            <div class="modal-title-row">
              <div class="modal-icon icon-blue">
                <i class="bi bi-copy"></i>
              </div>
              <div>
                <h5 class="modal-title">Copy Probes to Other Agents</h5>
                <p class="modal-subtitle">Select probes and destination agents</p>
              </div>
            </div>
            <button class="modal-close" @click="closeCopyModal">
              <i class="bi bi-x-lg"></i>
            </button>
          </div>
          
          <div class="modal-body">
            <!-- Results Display (shown after copy) -->
            <div v-if="state.copyResults" class="copy-results">
              <div class="results-summary">
                <div class="result-stat success" v-if="state.copyResults.created > 0">
                  <i class="bi bi-check-circle-fill"></i>
                  <span>{{ state.copyResults.created }} Created</span>
                </div>
                <div class="result-stat warning" v-if="state.copyResults.skipped > 0">
                  <i class="bi bi-dash-circle-fill"></i>
                  <span>{{ state.copyResults.skipped }} Skipped (duplicates)</span>
                </div>
                <div class="result-stat error" v-if="state.copyResults.errors > 0">
                  <i class="bi bi-x-circle-fill"></i>
                  <span>{{ state.copyResults.errors }} Errors</span>
                </div>
              </div>
              
              <button class="btn btn-primary w-100 mt-3" @click="closeCopyModal">
                Done
              </button>
            </div>

            <!-- Selection Form (shown before copy) -->
            <div v-else class="copy-form">
              <!-- Probe Selection -->
              <div class="selection-section">
                <h6 class="section-label">
                  <i class="bi bi-box"></i>
                  Select Probes to Copy
                  <span class="badge bg-secondary">{{ state.copySelectedProbes.length }}</span>
                </h6>
                <div class="selection-list">
                  <div 
                    v-for="probe in generalProbes" 
                    :key="probe.id" 
                    class="selection-item"
                    :class="{ selected: isProbeSelected(probe.id) }"
                    @click="toggleProbeSelection(probe.id)"
                  >
                    <div class="item-check">
                      <i :class="isProbeSelected(probe.id) ? 'bi bi-check-square-fill' : 'bi bi-square'"></i>
                    </div>
                    <div class="item-info">
                      <span class="item-type">{{ probe.type }}</span>
                      <span v-if="probe.targets && probe.targets.length > 0" class="item-target">
                        → {{ probe.targets[0].target || getAgentName(probe.targets[0].agent_id || 0) }}
                      </span>
                    </div>
                  </div>
                  <div v-if="generalProbes.length === 0" class="empty-selection">
                    No probes available to copy
                  </div>
                </div>
              </div>

              <!-- Agent Selection -->
              <div class="selection-section">
                <h6 class="section-label">
                  <i class="bi bi-cpu"></i>
                  Select Destination Agents
                  <span class="badge bg-secondary">{{ state.copyDestAgents.length }}</span>
                </h6>
                <div class="selection-list">
                  <div 
                    v-for="agent in otherAgents" 
                    :key="agent.id" 
                    class="selection-item"
                    :class="{ selected: isAgentSelected(agent.id) }"
                    @click="toggleAgentSelection(agent.id)"
                  >
                    <div class="item-check">
                      <i :class="isAgentSelected(agent.id) ? 'bi bi-check-square-fill' : 'bi bi-square'"></i>
                    </div>
                    <div class="item-info">
                      <span class="item-name">{{ agent.name }}</span>
                      <span v-if="agent.location" class="item-location">{{ agent.location }}</span>
                    </div>
                  </div>
                  <div v-if="otherAgents.length === 0" class="empty-selection">
                    No other agents in workspace
                  </div>
                </div>
              </div>

              <!-- Options -->
              <div class="copy-options-wrapper">
                <div v-if="hasSelectedAgentProbes" class="copy-options">
                  <label class="option-item bidirectional-option">
                    <input type="checkbox" v-model="state.copyBidirectional">
                    <span>Create bidirectional probes (reverse probes from destination agents back to source)</span>
                    <small class="option-hint">Only applies to Agent-to-Agent probes</small>
                  </label>
                </div>
                <div class="copy-options">
                  <label class="option-item">
                    <input type="checkbox" v-model="state.copySkipDuplicates">
                    <span>Skip probes that already exist on destination</span>
                  </label>
                </div>
              </div>
            </div>
          </div>

          <div class="modal-footer" v-if="!state.copyResults">
            <button class="btn btn-secondary" @click="closeCopyModal">Cancel</button>
            <button 
              class="btn btn-primary" 
              @click="executeCopy"
              :disabled="state.copyLoading || state.copySelectedProbes.length === 0 || state.copyDestAgents.length === 0"
            >
              <span v-if="state.copyLoading">
                <i class="bi bi-arrow-repeat spin"></i> Copying...
              </span>
              <span v-else>
                <i class="bi bi-copy"></i> Copy {{ state.copySelectedProbes.length }} Probes to {{ state.copyDestAgents.length }} Agents
              </span>
            </button>
          </div>
        </div>
      </div>
    </Teleport>

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

/* Target display - container for multiple targets */
.probe-targets {
  display: flex;
  flex-wrap: wrap;
  gap: 0.375rem;
  margin-top: 0.5rem;
}

/* Target pill base styles */
.target-pill {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.25rem 0.5rem;
  border-radius: 999px;
  font-size: 0.75rem;
  max-width: 220px;
  cursor: default;
  transition: all 0.15s ease;
}

.target-pill:hover {
  transform: translateY(-1px);
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
}

.target-pill i {
  font-size: 0.625rem;
  flex-shrink: 0;
}

.target-type {
  font-weight: 500;
  flex-shrink: 0;
}

.target-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Agent targets - blue theme */
.target-agent {
  background: #dbeafe;
  color: #1e40af;
}

.target-agent i {
  color: #3b82f6;
}

/* Group targets - purple theme */
.target-group {
  background: #ede9fe;
  color: #5b21b6;
}

.target-group i {
  color: #7c3aed;
}

/* Literal targets - gray/neutral theme */
.target-literal {
  background: #f3f4f6;
  color: #374151;
}

.target-literal i {
  color: #6b7280;
}

/* Legacy single target display (keep for backward compatibility) */
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

/* Probe Actions Container */
.probe-actions {
  display: flex;
  gap: 0.25rem;
  flex-shrink: 0;
}

.probe-action.info {
  background: none;
  border: none;
  cursor: pointer;
}

.probe-action.info:hover {
  background: #dbeafe;
  color: #2563eb;
}

/* More pill for truncated targets */
.target-more {
  background: #f9fafb;
  color: #6b7280;
  font-style: italic;
}

/* Modal Styles */
.modal-backdrop {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1050;
  padding: 1rem;
  animation: fadeIn 0.15s ease;
}

@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

.modal-dialog {
  background: white;
  border-radius: 12px;
  width: 100%;
  max-width: 520px;
  max-height: 90vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.25);
  animation: slideUp 0.2s ease;
}

@keyframes slideUp {
  from { 
    opacity: 0;
    transform: translateY(20px);
  }
  to { 
    opacity: 1;
    transform: translateY(0);
  }
}

.modal-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 1.25rem;
  border-bottom: 1px solid #e5e7eb;
  gap: 1rem;
}

.modal-title-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.modal-icon {
  width: 2.5rem;
  height: 2.5rem;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-size: 1rem;
  flex-shrink: 0;
}

.modal-title {
  margin: 0;
  font-size: 1.125rem;
  font-weight: 600;
  color: #1f2937;
}

.modal-subtitle {
  margin: 0.25rem 0 0 0;
  font-size: 0.813rem;
  color: #6b7280;
}

.modal-close {
  background: none;
  border: none;
  padding: 0.5rem;
  cursor: pointer;
  color: #9ca3af;
  border-radius: 6px;
  transition: all 0.15s;
  flex-shrink: 0;
}

.modal-close:hover {
  background: #f3f4f6;
  color: #374151;
}

.modal-body {
  padding: 1.25rem;
  overflow-y: auto;
  flex: 1;
}

.detail-section {
  margin-bottom: 1.25rem;
}

.detail-section:last-child {
  margin-bottom: 0;
}

.detail-label {
  font-size: 0.75rem;
  font-weight: 600;
  color: #6b7280;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin: 0 0 0.5rem 0;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0.75rem;
}

.detail-item {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.detail-key {
  font-size: 0.75rem;
  color: #9ca3af;
}

.detail-value {
  font-size: 0.875rem;
  color: #1f2937;
  font-weight: 500;
}

.detail-value.mono {
  font-family: 'SF Mono', Consolas, monospace;
  font-size: 0.813rem;
}

/* Status Badge */
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.375rem 0.75rem;
  border-radius: 999px;
  font-size: 0.813rem;
  font-weight: 500;
}

.status-badge.enabled {
  background: #d1fae5;
  color: #065f46;
}

.status-badge.disabled {
  background: #fef3c7;
  color: #92400e;
}

.status-badge i {
  font-size: 0.875rem;
}

/* Targets List in Modal */
.targets-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.target-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  border-radius: 6px;
  font-size: 0.813rem;
}

.target-row i {
  font-size: 0.75rem;
  flex-shrink: 0;
}

.target-label-modal {
  font-weight: 500;
  flex-shrink: 0;
}

.target-value-modal {
  flex: 1;
  word-break: break-all;
}

/* Target row colors */
.target-row.target-agent {
  background: #eff6ff;
  color: #1e40af;
}

.target-row.target-agent i {
  color: #3b82f6;
}

.target-row.target-group {
  background: #f5f3ff;
  color: #5b21b6;
}

.target-row.target-group i {
  color: #7c3aed;
}

.target-row.target-literal {
  background: #f9fafb;
  color: #374151;
}

.target-row.target-literal i {
  color: #6b7280;
}

/* Modal Footer */
.modal-footer {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 1rem 1.25rem;
  border-top: 1px solid #e5e7eb;
  background: #f9fafb;
}

.modal-hint {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 0.375rem;
  margin: 0;
  font-size: 0.75rem;
  color: #6b7280;
}

.modal-hint i {
  color: #9ca3af;
}

.modal-footer .btn {
  padding: 0.5rem 1rem;
  font-size: 0.875rem;
}

/* Make probe cards clickable */
.probe-card:not(.skeleton) {
  cursor: pointer;
}

/* ==================== Copy Modal Styles ==================== */
.copy-modal {
  width: 600px;
  max-width: 95vw;
  max-height: 80vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  pointer-events: auto;
  position: relative;
  z-index: 1;
}

.copy-modal .modal-body {
  overflow-y: auto;
  flex: 1;
  pointer-events: auto;
}

.copy-form {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.selection-section {
  background: #f9fafb;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 1rem;
}

.section-label {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin: 0 0 0.75rem 0;
  font-size: 0.875rem;
  font-weight: 600;
  color: #374151;
}

.section-label i {
  color: #6b7280;
}

.section-label .badge {
  margin-left: auto;
  font-size: 0.75rem;
}

.selection-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  max-height: 180px;
  overflow-y: auto;
  pointer-events: auto;
}

.selection-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.625rem 0.75rem;
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s;
  pointer-events: auto;
}

.selection-item:hover {
  border-color: #3b82f6;
  background: #eff6ff;
}

.selection-item.selected {
  border-color: #3b82f6;
  background: #dbeafe;
}

.item-check {
  font-size: 1.125rem;
  color: #9ca3af;
}

.selection-item.selected .item-check {
  color: #3b82f6;
}

.item-info {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
  flex: 1;
  min-width: 0;
}

.item-type {
  font-size: 0.875rem;
  font-weight: 500;
  color: #111827;
}

.item-target,
.item-location {
  font-size: 0.75rem;
  color: #6b7280;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.item-name {
  font-size: 0.875rem;
  font-weight: 500;
  color: #111827;
}

.empty-selection {
  text-align: center;
  padding: 1.5rem;
  color: #9ca3af;
  font-size: 0.875rem;
}

.copy-options-wrapper {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.copy-options {
  padding: 0.75rem;
  background: #f3f4f6;
  border-radius: 6px;
}

.option-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
  color: #374151;
  cursor: pointer;
}

.option-item input[type="checkbox"] {
  width: 16px;
  height: 16px;
  accent-color: #3b82f6;
}

.bidirectional-option {
  flex-wrap: wrap;
}

.option-hint {
  width: 100%;
  margin-left: 24px;
  font-size: 0.75rem;
  color: #6b7280;
  margin-top: 0.125rem;
}

/* Copy Results */
.copy-results {
  text-align: center;
  padding: 1rem;
}

.results-summary {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 1rem;
}

.result-stat {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 0.75rem 1rem;
  border-radius: 8px;
  font-size: 1rem;
  font-weight: 500;
}

.result-stat.success {
  background: #d1fae5;
  color: #065f46;
}

.result-stat.success i {
  color: #10b981;
}

.result-stat.warning {
  background: #fef3c7;
  color: #92400e;
}

.result-stat.warning i {
  color: #f59e0b;
}

.result-stat.error {
  background: #fee2e2;
  color: #991b1b;
}

.result-stat.error i {
  color: #ef4444;
}

/* Spin animation for loading */
@keyframes spin-animation {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.spin {
  display: inline-block;
  animation: spin-animation 1s linear infinite;
}

/* Dark Mode Overrides */
[data-theme="dark"] .modal-dialog {
  background: #1e293b !important;
  border-color: rgba(255, 255, 255, 0.1) !important;
}

[data-theme="dark"] .modal-header {
  border-bottom-color: rgba(255, 255, 255, 0.1) !important;
}

[data-theme="dark"] .modal-title {
  color: #f1f5f9 !important;
}

[data-theme="dark"] .modal-subtitle {
  color: #94a3b8 !important;
}

[data-theme="dark"] .modal-body {
  background: #1e293b !important;
}

[data-theme="dark"] .modal-footer {
  background: #0f172a !important;
  border-top-color: rgba(255, 255, 255, 0.1) !important;
}

[data-theme="dark"] .selection-section {
  background: #0f172a !important;
  border-color: rgba(255, 255, 255, 0.1) !important;
}

[data-theme="dark"] .section-label {
  color: #e2e8f0 !important;
}

[data-theme="dark"] .selection-item {
  background: #1e293b !important;
  border-color: rgba(255, 255, 255, 0.08) !important;
  color: #e2e8f0 !important;
}

[data-theme="dark"] .selection-item:hover {
  background: #334155 !important;
}

[data-theme="dark"] .selection-item.selected {
  background: rgba(59, 130, 246, 0.2) !important;
  border-color: rgba(59, 130, 246, 0.5) !important;
}

[data-theme="dark"] .item-type {
  color: #f1f5f9 !important;
}

[data-theme="dark"] .item-target {
  color: #94a3b8 !important;
}

[data-theme="dark"] .item-info-secondary {
  color: #64748b !important;
}

[data-theme="dark"] .option-row {
  color: #e2e8f0 !important;
}

[data-theme="dark"] .option-description {
  color: #94a3b8 !important;
}

[data-theme="dark"] .probe-card {
  background: rgba(255, 255, 255, 0.03) !important;
  border-color: rgba(255, 255, 255, 0.08) !important;
}

[data-theme="dark"] .probe-card:hover {
  background: rgba(255, 255, 255, 0.06) !important;
  border-color: rgba(255, 255, 255, 0.15) !important;
}

[data-theme="dark"] .probe-card > div:first-child,
[data-theme="dark"] .probe-header {
  background: transparent !important;
}

[data-theme="dark"] .probe-type {
  color: #f1f5f9 !important;
}

[data-theme="dark"] .probe-description {
  color: #94a3b8 !important;
}

[data-theme="dark"] .section-title {
  color: #e2e8f0 !important;
}

[data-theme="dark"] .target-pill {
  background: rgba(255, 255, 255, 0.08) !important;
  color: #e2e8f0 !important;
}

[data-theme="dark"] .filters-section {
  background: rgba(255, 255, 255, 0.02) !important;
}

[data-theme="dark"] .stat-chip {
  background: rgba(255, 255, 255, 0.05) !important;
  color: #e2e8f0 !important;
}

/* Dark Mode - Selection List Items */
[data-theme="dark"] .selection-list {
  background: transparent !important;
}

[data-theme="dark"] .item-check i {
  color: #94a3b8 !important;
}

[data-theme="dark"] .selection-item.selected .item-check i {
  color: #3b82f6 !important;
}

/* Dark Mode - Probe Details Modal */
[data-theme="dark"] .detail-section {
  background: transparent !important;
}

[data-theme="dark"] .detail-label {
  color: #94a3b8 !important;
}

[data-theme="dark"] .detail-grid {
  background: rgba(255, 255, 255, 0.03) !important;
  border-color: rgba(255, 255, 255, 0.08) !important;
}

[data-theme="dark"] .detail-key {
  color: #94a3b8 !important;
}

[data-theme="dark"] .detail-value {
  color: #f1f5f9 !important;
}

[data-theme="dark"] .detail-value.mono {
  color: #93c5fd !important;
}

[data-theme="dark"] .targets-list {
  background: transparent !important;
}

[data-theme="dark"] .target-row {
  background: rgba(255, 255, 255, 0.03) !important;
  border-color: rgba(255, 255, 255, 0.08) !important;
  color: #e2e8f0 !important;
}

[data-theme="dark"] .target-label-modal {
  color: #94a3b8 !important;
}

[data-theme="dark"] .target-value-modal {
  color: #f1f5f9 !important;
}

[data-theme="dark"] .status-badge.enabled {
  background: rgba(16, 185, 129, 0.15) !important;
  color: #34d399 !important;
}

[data-theme="dark"] .status-badge.disabled {
  background: rgba(239, 68, 68, 0.15) !important;
  color: #f87171 !important;
}

[data-theme="dark"] .modal-hint {
  color: #94a3b8 !important;
}

[data-theme="dark"] .option-row label {
  color: #e2e8f0 !important;
}

/* Dark Mode - Agent Names in Destination List */
[data-theme="dark"] .item-name {
  color: #f1f5f9 !important;
}

[data-theme="dark"] .item-secondary {
  color: #94a3b8 !important;
}

/* Dark Mode - Copy Options (Bidirectional/Skip) */
[data-theme="dark"] .copy-options {
  background: rgba(255, 255, 255, 0.05) !important;
}

[data-theme="dark"] .option-item {
  color: #e2e8f0 !important;
}

[data-theme="dark"] .option-hint {
  color: #94a3b8 !important;
}

[data-theme="dark"] .empty-selection {
  color: #64748b !important;
}
</style>