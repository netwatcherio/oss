<script lang="ts" setup>
import { onMounted, reactive, computed, watch } from "vue";
import type { AgentGroup, Probe, ProbeConfig, ProbeTarget, ProbeType, SelectOption, Site } from "@/types";
import { Agent } from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import agentService from "@/services/agentService";
import probeService from "@/services/probeService";
import siteService from "@/services/workspaceService";

interface ProbeState {
  site: Site;
  ready: boolean;
  loading: boolean;
  agent: Agent;
  selected: SelectOption;
  options: SelectOption[];
  probe: Probe;
  probeConfig: ProbeConfig;
  probeTarget: ProbeTarget;
  targetGroup: boolean;
  agentGroupSelected: AgentGroup[];
  agents: Agent[];
  customServer: boolean;
  targetAgent: boolean;
  targetAgentSelected: Agent | null;
  validAgents: Agent[];
  existingProbes: Probe[];
  duplicateWarning: string;
  errors: string[];
  hostInput: string;
  portInput: string;
}

const state = reactive<ProbeState>({
  site: {} as Site,
  ready: false,
  loading: false,
  agent: {} as Agent,
  selected: {} as SelectOption,
  options: [],
  probe: {} as Probe,
  probeConfig: {} as ProbeConfig,
  probeTarget: {} as ProbeTarget,
  targetGroup: false,
  agentGroupSelected: [],
  agents: [],
  customServer: false,
  targetAgent: true,
  targetAgentSelected: null,
  validAgents: [],
  existingProbes: [],
  duplicateWarning: "",
  errors: [],
  hostInput: "0.0.0.0",
  portInput: "5000"
});

const router = core.router();

// Computed properties
const showTargetAgentOption = computed(() => {
  const validTypes = ['MTR', 'PING', 'RPERF', 'TRAFFICSIM', 'AGENT'];
  return state.selected.value && validTypes.includes(state.selected.value) && state.agents.length >= 1;
});

const showTargetInput = computed(() => {
  return !state.targetGroup && !state.targetAgent;
});

const isValidProbe = computed(() => {
  if (!state.selected.value) return false;

  if (state.targetAgent && !state.targetAgentSelected) return false;

  if (state.targetGroup && state.agentGroupSelected.length === 0) return false;

  if (!state.targetAgent && !state.targetGroup) {
    if (!state.probeConfig.server && !state.probeTarget.target) return false;
  }

  return state.duplicateWarning === "";
});

// Probe type descriptions
const probeDescriptions = {
  AGENT: "Monitor the health, connectivity, and performance of other agents in your network",
  MTR: "Combine traceroute and ping to diagnose network paths and identify packet loss",
  PING: "Test basic connectivity and measure round-trip time to a target",
  TRAFFICSIM: "Generate simulated UDP traffic to test network throughput and performance",
  SPEEDTEST: "Measure bandwidth performance between locations",
  RPERF: "Advanced UDP performance testing with detailed metrics"
};

// Initialize component
onMounted(async () => {
  const id = router.currentRoute.value.params["idParam"] as string;
  if (!id) return;

  state.probeConfig = {
    duration: 60,
    count: 60,
    interval: 5,
    server: false,
  } as ProbeConfig;

  state.probeTarget = {
    target: ""
  } as ProbeTarget;

  try {
    // Load agent data
    const agentRes = await agentService.getAgent(id);
    state.agent = agentRes.data as Agent;

    // Load existing probes for duplicate checking
    const probesRes = await probeService.getAgentProbes(id);
    state.existingProbes = probesRes.data as Probe[];

    // Load site data
    const siteRes = await siteService.getSite(state.agent.site);
    state.site = siteRes.data as Site;

    // Load all agents for the site
    const agentsRes = await agentService.getSiteAgents(state.agent.site);
    if (agentsRes.data.length > 0) {
      const agents = agentsRes.data as Agent[];
      state.agents = agents.filter(a => a.id !== id);
      state.ready = true;
    }

    // Initialize probe type options
    initializeOptions();

  } catch (error) {
    console.error("Error loading data:", error);
    state.errors.push("Failed to load agent data");
  }
});

// Initialize probe type options with AGENT as preferred
function initializeOptions() {
  state.options = [
    { value: "AGENT", text: "Agent Monitoring", icon: "fa-heartbeat", recommended: true },
    { value: "PING", text: "PING (Packet Internet Groper)", icon: "fa-signal" },
    { value: "MTR", text: "MTR (My Traceroute)", icon: "fa-route" },
    { value: "TRAFFICSIM", text: "Simulated Traffic (UDP)", icon: "fa-stream" },
    // { value: "SPEEDTEST", text: "Speed Test", icon: "fa-tachometer-alt" },
    // { value: "RPERF", text: "RPERF (UDP)", icon: "fa-chart-line" }
  ];
}

// Watch for probe type changes
watch(() => state.selected.value, async (newType) => {
  if (newType === 'TRAFFICSIM') {
    await getValidAgents('TRAFFICSIM');
  } else if (newType === 'AGENT') {
    // For AGENT type, all other agents are valid targets
    state.validAgents = state.agents;
  }

  // Reset target selection when changing probe type
  state.targetAgentSelected = null;
  state.duplicateWarning = "";

  // Check for duplicates when type changes
  if (state.targetAgent && state.targetAgentSelected) {
    checkForDuplicates();
  }
});

// Watch for host/port changes to update target
watch([() => state.hostInput, () => state.portInput], () => {
  if (state.hostInput && state.portInput) {
    state.probeTarget.target = `${state.hostInput}:${state.portInput}`;
  }
});

// Watch for target changes to check duplicates
watch([
  () => state.targetAgentSelected,
  () => state.probeTarget.target,
  () => state.probeConfig.server
], () => {
  checkForDuplicates();
});

// Check for duplicate probes
function checkForDuplicates() {
  state.duplicateWarning = "";

  if (!state.selected.value) return;

  const probeType = state.selected.value as ProbeType;

  for (const existingProbe of state.existingProbes) {
    if (existingProbe.type !== probeType) continue;

    // Check server probes
    if (state.probeConfig.server && existingProbe.config.server) {
      if (probeType === 'TRAFFICSIM' || probeType === 'RPERF') {
        state.duplicateWarning = `A ${probeType} server probe already exists for this agent`;
        return;
      }
    }

    // Check target-based probes
    if (state.targetAgent && state.targetAgentSelected) {
      const existingTargets = existingProbe.config.target || [];
      for (const target of existingTargets) {
        if (target.agent === state.targetAgentSelected.id) {
          state.duplicateWarning = `A ${probeType} probe already exists for target agent: ${state.targetAgentSelected.name}`;
          return;
        }
      }
    }

    // Check custom target probes
    if (!state.targetAgent && !state.targetGroup && state.probeTarget.target) {
      const existingTargets = existingProbe.config.target || [];
      for (const target of existingTargets) {
        if (target.target === state.probeTarget.target) {
          state.duplicateWarning = `A ${probeType} probe already exists for target: ${state.probeTarget.target}`;
          return;
        }
      }
    }
  }
}

// Get valid agents for specific probe types
async function getValidAgents(probeType: ProbeType) {
  const validAgents: Agent[] = [];
  state.loading = true;

  console.log(`Getting valid agents for probe type: ${probeType}...`);

  try {
    for (const agent of state.agents) {
      if (agent.id === state.agent.id) continue;

      try {
        const res = await probeService.getAgentProbes(agent.id);
        const agentProbes = res.data as Probe[];

        // For TRAFFICSIM, only agents with server enabled are valid
        if (probeType === "TRAFFICSIM") {
          const hasTrafficSimServer = agentProbes.some(
              probe => probe.type === "TRAFFICSIM" && probe.config.server
          );
          if (hasTrafficSimServer) {
            validAgents.push(agent);
          }
        }
      } catch (error) {
        console.error(`Error fetching probes for agent ${agent.id}:`, error);
      }
    }
  } finally {
    state.loading = false;
  }

  state.validAgents = validAgents;
  console.log(`Found ${validAgents.length} valid agents for ${probeType}`);
}

// Create probe
async function submit() {
  state.errors = [];

  if (!isValidProbe.value) {
    state.errors.push("Please fill in all required fields");
    return;
  }

  const id = router.currentRoute.value.params["idParam"] as string;
  if (!id) return;

  state.loading = true;

  try {
    // Build probe targets
    if (state.targetGroup && state.agentGroupSelected.length > 0) {
      // Group targets
      state.probeConfig.target = state.agentGroupSelected.map(
          group => ({ group: group.id } as ProbeTarget)
      );
    } else if (state.targetAgent && state.targetAgentSelected) {
      // Agent target
      state.probeConfig.target = [{ agent: state.targetAgentSelected.id } as ProbeTarget];
    } else if (!state.probeConfig.server) {
      // Custom target
      state.probeConfig.target = [state.probeTarget];
    }

    // Special handling for TRAFFICSIM client mode
    if (state.selected.value === 'TRAFFICSIM' && state.targetAgent && state.probeConfig.target.length >= 1) {
      state.probeConfig.server = false;
    }

    // Set probe configuration
    state.probe.config = state.probeConfig;
    state.probe.type = state.selected.value as ProbeType;

    // Create the probe
    await probeService.createProbe(id, state.probe);
    router.push(`/agent/${id}`);

  } catch (error) {
    console.error("Error creating probe:", error);
    state.errors.push("Failed to create probe. Please try again.");
  } finally {
    state.loading = false;
  }
}

// Helper function to get available agents based on probe type
const availableAgentsForSelection = computed(() => {
  if (state.selected.value === 'TRAFFICSIM') {
    return state.validAgents;
  } else if (state.selected.value === 'AGENT') {
    return state.agents;
  }
  return state.agents;
});
</script>

<template>
  <div class="container-fluid">
    <Title
        :history="[
          {title: 'workspaces', link: '/workspaces'},
          {title: state.site.name, link: `/workspace/${state.site.id}`},
          {title: state.agent.name, link: `/agent/${state.agent.id}`}
        ]"
        :subtitle="`create a new probe for agent '${state.agent.name}'`"
        title="New Probe">
    </Title>

    <div class="row">
      <div class="col-12">
        <!-- Error Messages -->
        <div v-if="state.errors.length > 0" class="alert alert-danger alert-dismissible fade show mb-3" role="alert">
          <div class="d-flex align-items-center">
            <i class="fas fa-exclamation-circle me-2"></i>
            <div>
              <div v-for="(error, index) in state.errors" :key="index">{{ error }}</div>
            </div>
          </div>
        </div>

        <!-- Duplicate Warning -->
        <div v-if="state.duplicateWarning" class="alert alert-warning alert-dismissible fade show mb-3" role="alert">
          <div class="d-flex align-items-center">
            <i class="fas fa-exclamation-triangle me-2"></i>
            <span>{{ state.duplicateWarning }}</span>
          </div>
        </div>

        <!-- Probe Type Selection Card -->
        <div class="card mb-4">
          <div class="card-header bg-primary text-white">
            <h5 class="mb-0"><i class="fas fa-list-check me-2"></i>Select Probe Type</h5>
          </div>
          <div class="card-body">
            <div class="row g-3">
              <div 
                v-for="option in state.options" 
                :key="option.value"
                class="col-lg-6 col-xl-4">
                <div 
                  class="probe-type-card"
                  :class="{ 
                    'selected': state.selected.value === option.value,
                    'recommended': option.recommended
                  }"
                  @click="state.selected = option">
                  <div class="probe-type-header">
                    <div class="d-flex align-items-center justify-content-between">
                      <div class="d-flex align-items-center">
                        <i :class="`fas ${option.icon} probe-icon`"></i>
                        <h6 class="mb-0">{{ option.text }}</h6>
                      </div>
                      <span v-if="option.recommended" class="badge bg-success">Recommended</span>
                    </div>
                  </div>
                  <p class="probe-description mb-0">
                    {{ probeDescriptions[option.value] || 'No description available' }}
                  </p>
                  <div class="selection-indicator">
                    <i class="fas fa-check-circle"></i>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Configuration Card -->
        <div v-if="state.selected && state.selected.value" class="card">
          <div class="card-header">
            <h5 class="mb-0">
              <i :class="`fas ${state.selected.icon} me-2`"></i>
              {{ state.selected.text }} Configuration
            </h5>
          </div>
          <div class="card-body">
            <!-- Target Selection -->
            <div class="configuration-section">
              <h6 class="section-title">Target Configuration</h6>
              
              <!-- Target Mode Toggle (if applicable) -->
              <div v-if="showTargetAgentOption" class="mb-4">
                <div class="form-check form-switch">
                  <input
                      id="useAgentTarget"
                      v-model="state.targetAgent"
                      class="form-check-input"
                      type="checkbox">
                  <label class="form-check-label" for="useAgentTarget">
                    Use Agent as Target
                    <small class="text-muted d-block">Select another agent as the probe target</small>
                  </label>
                </div>
              </div>

              <!-- Agent Selection -->
              <div v-if="state.targetAgent" class="mb-4">
                <label class="form-label fw-semibold" for="targetAgent">
                  <i class="fas fa-server me-2"></i>Target Agent
                </label>
                <select
                    id="targetAgent"
                    v-model="state.targetAgentSelected"
                    class="form-select form-select-lg"
                    :disabled="state.loading">
                  <option :value="null" disabled>Select an agent</option>
                  <option
                      v-for="agent in availableAgentsForSelection"
                      :key="agent.id"
                      :value="agent">
                    {{ agent.name }} 
                    <span v-if="agent.location">({{ agent.location }})</span>
                  </option>
                </select>
                <small v-if="state.selected.value === 'TRAFFICSIM' && state.validAgents.length === 0" class="text-warning">
                  <i class="fas fa-info-circle me-1"></i>No agents with TrafficSim server enabled found
                </small>
              </div>

              <!-- AGENT Probe Specific Info -->
              <div v-if="state.selected.value === 'AGENT'" class="info-box mb-4">
                <i class="fas fa-info-circle me-2"></i>
                <div>
                  <strong>Agent Monitoring</strong> will continuously check the health, connectivity, and performance 
                  metrics of the selected target agent. This includes uptime, response times, and system resources.
                </div>
              </div>

              <!-- TRAFFICSIM Options -->
              <div v-if="state.selected.value === 'TRAFFICSIM'">
                <div v-if="!state.targetAgent && !state.targetGroup" class="mb-4">
                  <div class="form-check form-switch">
                    <input
                        id="trafficSimServer"
                        v-model="state.probeConfig.server"
                        class="form-check-input"
                        type="checkbox"
                        :disabled="state.existingProbes.some(p => p.type === 'TRAFFICSIM' && p.config.server)">
                    <label class="form-check-label" for="trafficSimServer">
                      Enable Server Mode
                      <small class="text-muted d-block">Run as a traffic receiver (only one server per agent allowed)</small>
                    </label>
                  </div>
                </div>

                <div v-if="state.probeConfig.server && showTargetInput" class="mb-4">
                  <label class="form-label fw-semibold">
                    <i class="fas fa-network-wired me-2"></i>Server Listening Configuration
                  </label>
                  <div class="host-port-input">
                    <div class="row g-3">
                      <div class="col-md-8">
                        <label class="form-label text-muted small">Host / IP Address</label>
                        <div class="input-group">
                          <span class="input-group-text"><i class="fas fa-globe"></i></span>
                          <input
                              v-model="state.hostInput"
                              class="form-control"
                              type="text"
                              placeholder="0.0.0.0"
                              aria-label="Host address">
                        </div>
                        <small class="text-muted">Use 0.0.0.0 to listen on all interfaces</small>
                      </div>
                      <div class="col-md-4">
                        <label class="form-label text-muted small">Port</label>
                        <div class="input-group">
                          <span class="input-group-text"><i class="fas fa-ethernet"></i></span>
                          <input
                              v-model="state.portInput"
                              class="form-control"
                              type="number"
                              min="1"
                              max="65535"
                              placeholder="5000"
                              aria-label="Port number">
                        </div>
                        <small class="text-muted">Range: 1-65535</small>
                      </div>
                    </div>
                    <div class="mt-2">
                      <code class="text-primary">{{ state.hostInput || '0.0.0.0' }}:{{ state.portInput || '5000' }}</code>
                    </div>
                  </div>
                </div>
              </div>

              <!-- PING Options -->
              <div v-if="state.selected.value === 'PING' && showTargetInput" class="mb-4">
                <label class="form-label fw-semibold" for="pingTarget">
                  <i class="fas fa-bullseye me-2"></i>Target Address
                </label>
                <div class="input-group">
                  <span class="input-group-text"><i class="fas fa-globe"></i></span>
                  <input
                      id="pingTarget"
                      v-model="state.probeTarget.target"
                      class="form-control"
                      type="text"
                      placeholder="1.1.1.1 or google.com">
                </div>
                <small class="text-muted">Enter an IP address or domain name</small>
              </div>

              <!-- MTR Options -->
              <div v-if="state.selected.value === 'MTR'">
                <div v-if="showTargetInput" class="mb-4">
                  <label class="form-label fw-semibold" for="mtrTarget">
                    <i class="fas fa-route me-2"></i>Target Address
                  </label>
                  <div class="input-group">
                    <span class="input-group-text"><i class="fas fa-globe"></i></span>
                    <input
                        id="mtrTarget"
                        v-model="state.probeTarget.target"
                        class="form-control"
                        type="text"
                        placeholder="1.1.1.1 or google.com">
                  </div>
                  <small class="text-muted">Enter an IP address or domain name</small>
                </div>
                <div class="mb-4">
                  <label class="form-label fw-semibold" for="mtrInterval">
                    <i class="fas fa-clock me-2"></i>Probe Interval
                  </label>
                  <div class="input-group">
                    <input
                        id="mtrInterval"
                        v-model.number="state.probeConfig.interval"
                        class="form-control"
                        type="number"
                        min="1"
                        max="60">
                    <span class="input-group-text">minutes</span>
                  </div>
                  <small class="text-muted">How often to run the MTR trace</small>
                </div>
              </div>
            </div>
          </div>

          <!-- Form Actions -->
          <div class="card-footer">
            <div class="d-flex justify-content-between align-items-center">
              <router-link
                  :to="`/agent/${state.agent.id}`"
                  class="btn btn-outline-secondary">
                <i class="fas fa-arrow-left me-2"></i>Cancel
              </router-link>
              <button
                  class="btn btn-primary btn-lg px-5"
                  type="submit"
                  @click="submit"
                  :disabled="!isValidProbe || state.loading">
                <span v-if="state.loading">
                  <i class="fas fa-spinner fa-spin me-2"></i>Creating Probe...
                </span>
                <span v-else>
                  <i class="fas fa-plus-circle me-2"></i>Create Probe
                </span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Card Styles */
.card {
  border: none;
  box-shadow: 0 0.125rem 0.25rem rgba(0, 0, 0, 0.075);
  border-radius: 0.5rem;
  overflow: hidden;
}

.card-header {
  border-bottom: 1px solid rgba(0, 0, 0, 0.125);
  padding: 1.25rem;
}

.card-body {
  padding: 1.5rem;
}

.card-footer {
  background-color: #f8f9fa;
  border-top: 1px solid #e9ecef;
  padding: 1.25rem 1.5rem;
}

/* Probe Type Cards */
.probe-type-card {
  border: 2px solid #e9ecef;
  border-radius: 0.5rem;
  padding: 1.25rem;
  cursor: pointer;
  transition: all 0.3s ease;
  position: relative;
  height: 100%;
  background: white;
}

.probe-type-card:hover {
  border-color: #0d6efd;
  box-shadow: 0 0.25rem 0.5rem rgba(13, 110, 253, 0.15);
  transform: translateY(-2px);
}

.probe-type-card.selected {
  border-color: #0d6efd;
  background-color: #f0f6ff;
}

.probe-type-card.recommended {
  border-color: #198754;
}

.probe-type-header {
  margin-bottom: 0.75rem;
}

.probe-icon {
  font-size: 1.25rem;
  margin-right: 0.75rem;
  color: #0d6efd;
}

.probe-type-card.selected .probe-icon {
  color: #0d6efd;
}

.probe-description {
  font-size: 0.875rem;
  color: #6c757d;
  line-height: 1.5;
}

.selection-indicator {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  color: #0d6efd;
  font-size: 1.25rem;
  opacity: 0;
  transition: opacity 0.3s ease;
}

.probe-type-card.selected .selection-indicator {
  opacity: 1;
}

/* Configuration Sections */
.configuration-section {
  margin-bottom: 2rem;
}

.section-title {
  color: #495057;
  font-weight: 600;
  margin-bottom: 1.25rem;
  padding-bottom: 0.5rem;
  border-bottom: 2px solid #e9ecef;
}

/* Info Box */
.info-box {
  background-color: #e7f3ff;
  border-left: 4px solid #0d6efd;
  padding: 1rem;
  border-radius: 0.25rem;
  display: flex;
  align-items: start;
}

.info-box i {
  color: #0d6efd;
  margin-top: 0.125rem;
}

/* Host/Port Input */
.host-port-input {
  background-color: #f8f9fa;
  padding: 1.25rem;
  border-radius: 0.5rem;
  border: 1px solid #e9ecef;
}

/* Form Controls */
.form-label {
  font-weight: 500;
  color: #495057;
  margin-bottom: 0.5rem;
}

.form-control, .form-select {
  border: 1px solid #ced4da;
  transition: border-color 0.15s ease-in-out, box-shadow 0.15s ease-in-out;
}

.form-control:focus, .form-select:focus {
  border-color: #86b7fe;
  box-shadow: 0 0 0 0.25rem rgba(13, 110, 253, 0.25);
}

.form-select-lg {
  padding: 0.75rem 1rem;
  font-size: 1rem;
}

.form-check-input:checked {
  background-color: #0d6efd;
  border-color: #0d6efd;
}

/* Input Groups */
.input-group-text {
  background-color: #e9ecef;
  border: 1px solid #ced4da;
  color: #6c757d;
}

/* Alerts */
.alert {
  border: none;
  border-radius: 0.5rem;
}

.alert-danger {
  background-color: #f8d7da;
  color: #721c24;
}

.alert-warning {
  background-color: #fff3cd;
  color: #856404;
}

/* Buttons */
.btn {
  padding: 0.5rem 1rem;
  font-weight: 500;
  border-radius: 0.375rem;
  transition: all 0.15s ease-in-out;
}

.btn-primary {
  background-color: #0d6efd;
  border-color: #0d6efd;
}

.btn-primary:hover:not(:disabled) {
  background-color: #0b5ed7;
  border-color: #0a58ca;
  transform: translateY(-1px);
  box-shadow: 0 0.25rem 0.5rem rgba(13, 110, 253, 0.2);
}

.btn-outline-secondary {
  color: #6c757d;
  border-color: #6c757d;
}

.btn-outline-secondary:hover {
  color: #fff;
  background-color: #6c757d;
  border-color: #6c757d;
}

/* Utilities */
.fw-semibold {
  font-weight: 600;
}

code {
  padding: 0.25rem 0.5rem;
  font-size: 0.875rem;
  color: #0d6efd;
  background-color: #e7f3ff;
  border-radius: 0.25rem;
}

.text-muted {
  color: #6c757d !important;
}

.text-warning {
  color: #ffc107 !important;
}

/* Responsive */
@media (max-width: 768px) {
  .probe-type-card {
    margin-bottom: 1rem;
  }
  
  .btn-lg {
    padding: 0.5rem 1rem;
    font-size: 1rem;
  }
}
</style>