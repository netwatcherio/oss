<script lang="ts" setup>
import { onMounted, reactive, computed, watch } from "vue";
import {
  Agent,
  type Probe,
  type ProbeCreateInput,
  type ProbeType,
  type SelectOption,
  type Target,
  type Workspace
} from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import {AgentService, ProbeService, WorkspaceService} from "@/services/apiService";

interface DnsConfig {
  queryAll: boolean;
  selectedRecordTypes: string[];
  dnsServer: string;
  dnssecValidation: boolean;
}

interface ProbeState {
  workspace: Workspace;
  ready: boolean;
  loading: boolean;
  agent: Agent;
  selected: SelectOption;
  options: SelectOption[];
  probe: ProbeCreateInput;
  agents: Agent[];
  customServer: boolean;
  targetAgent: boolean;
  targetAgentSelected: Agent | null;
  target: Target;
  validAgents: Agent[];
  existingProbes: Probe[];
  duplicateWarning: string;
  errors: string[];
  hostInput: string;
  portInput: string;
  dnsConfig: DnsConfig;
}

// Get probe description
function getProbeDescription(probeType: string): string {
  const descriptions: Record<string, string> = {
    AGENT: "Monitor the health, connectivity, and performance of other agents in your network",
    PING: "Test basic connectivity and measure round-trip time to a target",
    MTR: "Combine traceroute and ping to diagnose network paths and identify packet loss",
    TRAFFICSIM: "Generate simulated UDP traffic to test network throughput and performance",
    DNS: "Monitor DNS resolution performance and availability",
    SPEEDTEST: "Measure bandwidth performance between locations",
    RPERF: "Advanced UDP performance testing with detailed metrics"
  };
  return descriptions[probeType] || 'No description available';
}

const state = reactive<ProbeState>({
  workspace: {} as Workspace,
  ready: false,
  loading: false,
  agent: {} as Agent,
  selected: {} as SelectOption,
  options: [],
  probe: {
    workspace_id: 0,
    agent_id: 0,
    type: '',
    enabled: true,
    interval_sec: 300,
    timeout_sec: 30,
    count: 10,
    duration_sec: 60,
    labels: {},
    metadata: {},
    server: false,
    targets: [],
    agent_targets: []
  } as ProbeCreateInput,
  agents: [],
  customServer: false,
  target: {} as Target,
  targetAgent: true,
  targetAgentSelected: null,
  validAgents: [],
  existingProbes: [],
  duplicateWarning: "",
  errors: [],
  hostInput: "",
  portInput: "5000",
  dnsConfig: {
    queryAll: false,
    selectedRecordTypes: ['A', 'AAAA'],
    dnsServer: "",
    dnssecValidation: false
  }
});

const router = core.router();

// DNS Record Types
const dnsRecordTypes = [
  { value: 'A', description: 'IPv4 Address' },
  { value: 'AAAA', description: 'IPv6 Address' },
  { value: 'CNAME', description: 'Canonical Name' },
  { value: 'MX', description: 'Mail Exchange' },
  { value: 'TXT', description: 'Text Records' },
  { value: 'NS', description: 'Name Servers' },
  { value: 'SOA', description: 'Start of Authority' },
  { value: 'PTR', description: 'Pointer (Reverse DNS)' },
  { value: 'SRV', description: 'Service Records' },
  { value: 'CAA', description: 'Certificate Authority' }
];

// Handle DNS query all change
function handleDnsQueryAllChange() {
  if (state.dnsConfig.queryAll) {
    // When querying all, clear specific selections
    state.dnsConfig.selectedRecordTypes = [];
  } else {
    // When not querying all, default to A and AAAA
    state.dnsConfig.selectedRecordTypes = ['A', 'AAAA'];
  }
}

// Computed properties
const showTargetAgentOption = computed(() => {
  const validTypes = ['MTR', 'PING', 'RPERF', 'TRAFFICSIM', 'AGENT'];
  return validTypes.includes(state.selected.value || '');
});

const showCustomTargetInput = computed(() => {
  return !state.targetAgent || !showTargetAgentOption.value;
});

const availableAgentsForSelection = computed(() => {
  // Filter out the current agent from the list
  return state.agents.filter(agent => agent.id !== state.agent.id);
});

const isValidProbe = computed(() => {
  if (!state.selected.value) return false;

  // Common validation
  if (state.probe.interval_sec <= 0) return false;

  switch (state.selected.value) {
    case "AGENT":
      // For agent monitoring, we need a target agent selected
      return state.targetAgentSelected !== null;

    case "MTR":
    case "PING":
      // Need either a target agent or a custom host
      if (state.targetAgent && showTargetAgentOption.value) {
        return state.targetAgentSelected !== null;
      } else {
        return state.hostInput !== "";
      }

    case "TRAFFICSIM":
      // Server mode doesn't need a target
      if (state.probe.server) return true;
      // Client mode needs a target
      if (state.targetAgent) {
        return state.targetAgentSelected !== null;
      } else {
        return state.hostInput !== "";
      }

    case "DNS":
      // DNS needs a domain to resolve
      const hasValidDomain = state.hostInput !== "" && state.hostInput.includes('.');
      const hasValidRecordTypes = state.dnsConfig.queryAll ||
          (state.dnsConfig.selectedRecordTypes && state.dnsConfig.selectedRecordTypes.length > 0);
      return hasValidDomain && hasValidRecordTypes;

    case "RPERF":
    case "SPEEDTEST":
      // These might need special handling
      if (state.targetAgent && showTargetAgentOption.value) {
        return state.targetAgentSelected !== null;
      } else {
        return state.hostInput !== "";
      }

    default:
      return false;
  }
});

const probeTypeConfig = computed(() => {
  const config: Record<string, any> = {
    AGENT: {
      description: "Monitor the health, connectivity, and performance of other agents in your network",
      icon: "fa-heartbeat",
      recommended: true,
      requiresTargetAgent: true,
      supportsCustomTarget: false,
      defaultInterval: 60,
      defaultTimeout: 30,
      defaultCount: 1
    },
    PING: {
      description: "Test basic connectivity and measure round-trip time to a target",
      icon: "fa-signal",
      requiresTargetAgent: false,
      supportsCustomTarget: true,
      defaultInterval: 300,
      defaultTimeout: 10,
      defaultCount: 5
    },
    MTR: {
      description: "Combine traceroute and ping to diagnose network paths and identify packet loss",
      icon: "fa-route",
      requiresTargetAgent: false,
      supportsCustomTarget: true,
      defaultInterval: 300,
      defaultTimeout: 30,
      defaultCount: 10
    },
    TRAFFICSIM: {
      description: "Generate simulated UDP traffic to test network throughput and performance",
      icon: "fa-stream",
      requiresTargetAgent: false,
      supportsCustomTarget: true,
      supportsServer: true,
      defaultInterval: 0,
      defaultTimeout: 0,
      defaultDuration: 60
    },
    DNS: {
      description: "Monitor DNS resolution performance and availability",
      icon: "fa-map-signs",
      requiresTargetAgent: false,
      supportsCustomTarget: true,
      requiresHostOnly: true,
      defaultInterval: 300,
      defaultTimeout: 10,
      defaultCount: 1
    },
    SPEEDTEST: {
      description: "Measure bandwidth performance between locations",
      icon: "fa-tachometer-alt",
      requiresTargetAgent: false,
      supportsCustomTarget: true,
      defaultInterval: 3600,
      defaultTimeout: 60,
      defaultCount: 1
    },
    RPERF: {
      description: "Advanced UDP performance testing with detailed metrics",
      icon: "fa-chart-line",
      requiresTargetAgent: false,
      supportsCustomTarget: true,
      defaultInterval: 600,
      defaultTimeout: 30,
      defaultDuration: 30
    }
  };

  return config[state.selected.value] || {};
});

// Initialize probe type options
function initializeOptions() {
  state.options = [
    {
      value: "AGENT",
      text: "Agent Monitoring",
      icon: "fa-heartbeat",
      recommended: true,
      agentAvailable: true,
      disabled: false
    },
    {
      value: "PING",
      text: "PING",
      icon: "fa-signal",
      agentAvailable: true,
      disabled: false,
      recommended: false
    },
    {
      value: "MTR",
      text: "MTR (My Traceroute)",
      icon: "fa-route",
      agentAvailable: true,
      disabled: false,
      recommended: false
    },
    {
      value: "TRAFFICSIM",
      text: "Traffic Simulator",
      icon: "fa-stream",
      agentAvailable: true,
      disabled: false,
      recommended: false
    },
    {
      value: "DNS",
      text: "DNS Monitor",
      icon: "fa-map-signs",
      agentAvailable: false,
      disabled: false,
      recommended: false
    },
    {
      value: "SPEEDTEST",
      text: "Speed Test",
      icon: "fa-tachometer-alt",
      agentAvailable: true,
      disabled: false,
      recommended: false
    },
    {
      value: "RPERF",
      text: "RPERF (UDP Performance)",
      icon: "fa-chart-line",
      agentAvailable: true,
      disabled: false,
      recommended: false
    }
  ];
}

// Apply default configuration when probe type changes
function applyProbeDefaults() {
  const config = probeTypeConfig.value;
  if (!config) return;

  if (config.defaultInterval !== undefined) {
    state.probe.interval_sec = config.defaultInterval;
  }
  if (config.defaultTimeout !== undefined) {
    state.probe.timeout_sec = config.defaultTimeout;
  }
  if (config.defaultCount !== undefined) {
    state.probe.count = config.defaultCount;
  }
  if (config.defaultDuration !== undefined) {
    state.probe.duration_sec = config.defaultDuration;
  }

  // Set target mode based on probe type
  if (config.requiresTargetAgent) {
    state.targetAgent = true;
  } else if (config.requiresHostOnly) {
    state.targetAgent = false;
  }
}

// Submit probe creation
async function submit() {
  state.errors = [];
  state.loading = true;

  try {
    if (!isValidProbe.value) {
      state.errors.push("Please fill in all required fields");
      return;
    }

    // Prepare the probe for submission
    const newProbe: ProbeCreateInput = {
      ...state.probe,
      workspace_id: state.workspace.id,
      agent_id: state.agent.id,
      type: state.selected.value as ProbeType
    };

    // Set targets based on selection mode
    if (state.targetAgent && state.targetAgentSelected && showTargetAgentOption.value) {
      // Use agent targets for agent-based monitoring
      newProbe.agent_targets = [state.targetAgentSelected.id];
      newProbe.targets = [];
    } else if (!state.targetAgent || !showTargetAgentOption.value) {
      // Use string targets for custom hosts/IPs
      if (state.probe.server && state.selected.value === 'TRAFFICSIM') {
        // Server mode may not need targets
        newProbe.targets = state.hostInput ? [`${state.hostInput}:${state.portInput}`] : [];
      } else {
        // Regular target
        const target = state.hostInput.includes(':') ? state.hostInput : state.hostInput;
        newProbe.targets = [target];
      }
      newProbe.agent_targets = [];
    }

    // Handle special cases for TRAFFICSIM
    if (state.selected.value === 'TRAFFICSIM' && state.probe.server) {
      newProbe.server = true;
      // Server mode might need listening address
      if (state.hostInput) {
        newProbe.metadata = {
          ...newProbe.metadata,
          listenAddress: `${state.hostInput}:${state.portInput}`
        };
      }
    }

    // Handle DNS configuration
    if (state.selected.value === 'DNS') {
      // Store DNS-specific configuration in metadata
      const dnsMetadata: any = {
        queryAll: state.dnsConfig.queryAll,
        dnssecValidation: state.dnsConfig.dnssecValidation
      };

      if (!state.dnsConfig.queryAll && state.dnsConfig.selectedRecordTypes.length > 0) {
        dnsMetadata.recordTypes = state.dnsConfig.selectedRecordTypes;
      }

      if (state.dnsConfig.dnsServer) {
        dnsMetadata.dnsServer = state.dnsConfig.dnsServer;
      }

      newProbe.metadata = {
        ...newProbe.metadata,
        ...dnsMetadata
      };

      // DNS probes always use the domain as the target
      newProbe.targets = [state.hostInput];
      newProbe.agent_targets = [];
    }

    console.log('Creating probe:', newProbe);

    const response = await ProbeService.create(
        state.workspace.id,
        state.agent.id,
        newProbe
    );

    console.log('Probe created successfully:', response);

    // Navigate back to agent page
    await router.push(`/workspace/${state.workspace.id}/agent/${state.agent.id}`);

  } catch (error) {
    console.error("Error creating probe:", error);
    state.errors.push("Failed to create probe. Please try again.");
  } finally {
    state.loading = false;
  }
}

// Check for duplicate probes
async function checkForDuplicates() {
  state.duplicateWarning = "";

  if (!state.selected.value) return;

  try {
    // Get existing probes for this agent
    const response = await ProbeService.list(state.workspace.id, state.agent.id);
    const existingProbes = response as Probe[];

    // Check for duplicates based on type and target
    for (const probe of existingProbes) {
      if (probe.type !== state.selected.value) continue;

      // Check server mode duplicates
      if (state.probe.server && probe.server) {
        state.duplicateWarning = `A ${state.selected.value} server probe already exists for this agent`;
        return;
      }

      // Check target duplicates
      if (state.targetAgent && state.targetAgentSelected) {
        // Check if any existing probe targets this agent
        const hasTargetAgent = probe.targets?.some(
            t => t.agentId === state.targetAgentSelected?.id
        );
        if (hasTargetAgent) {
          state.duplicateWarning = `A ${state.selected.value} probe already exists for target agent: ${state.targetAgentSelected.name}`;
          return;
        }
      } else if (state.hostInput) {
        // Check if any existing probe targets this host
        const targetToCheck = state.hostInput;
        const hasTargetHost = probe.targets?.some(
            t => t.target === targetToCheck || t.target === `${targetToCheck}:${state.portInput}`
        );
        if (hasTargetHost) {
          state.duplicateWarning = `A ${state.selected.value} probe already exists for target: ${targetToCheck}`;
          return;
        }
      }
    }
  } catch (error) {
    console.error("Error checking for duplicates:", error);
  }
}

// Watch for probe type changes
watch(() => state.selected.value, async (newType) => {
  if (!newType) return;

  // Reset form state
  state.targetAgentSelected = null;
  state.hostInput = "";
  state.portInput = "5000";
  state.duplicateWarning = "";
  state.probe.server = false;

  // Reset DNS configuration
  state.dnsConfig = {
    queryAll: false,
    selectedRecordTypes: ['A', 'AAAA'],
    dnsServer: "",
    dnssecValidation: false
  };

  // Apply defaults for the new probe type
  applyProbeDefaults();

  // Check for duplicates
  await checkForDuplicates();
});

// Watch for target changes to check duplicates
watch([
  () => state.targetAgentSelected,
  () => state.hostInput,
  () => state.probe.server
], () => {
  checkForDuplicates();
});

// Initialize component
onMounted(async () => {
  const agentID = router.currentRoute.value.params["aID"] as string;
  const workspaceID = router.currentRoute.value.params["wID"] as string;

  if (!agentID || !workspaceID) {
    state.errors.push("Missing agent or workspace ID");
    return;
  }

  try {
    // Load workspace
    const workspaceResponse = await WorkspaceService.get(workspaceID);
    state.workspace = workspaceResponse as Workspace;

    // Load agent
    const agentResponse = await AgentService.get(workspaceID, agentID);
    state.agent = agentResponse as Agent;

    // Load all agents for target selection
    const agentsResponse = await AgentService.list(workspaceID);
    state.agents = agentsResponse.data as Agent[];

    // Initialize probe options
    initializeOptions();

    state.ready = true;

  } catch (error) {
    console.error("Error loading data:", error);
    state.errors.push("Failed to load agent or workspace data");
  }
});
</script>

<template>
  <div class="container-fluid">
    <Title
        :history="[
          {title: 'workspaces', link: '/workspaces'},
          {title: state.workspace.name, link: `/workspace/${state.workspace.id}`},
          {title: state.agent.name, link: `/workspace/${state.workspace.id}/agent/${state.agent.id}`}
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
          <button type="button" class="btn-close" data-bs-dismiss="alert" aria-label="Close"></button>
        </div>

        <!-- Duplicate Warning -->
        <div v-if="state.duplicateWarning" class="alert alert-warning alert-dismissible fade show mb-3" role="alert">
          <div class="d-flex align-items-center">
            <i class="fas fa-exclamation-triangle me-2"></i>
            <span>{{ state.duplicateWarning }}</span>
          </div>
          <button type="button" class="btn-close" data-bs-dismiss="alert" aria-label="Close"></button>
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
                    'recommended': option.recommended,
                    'disabled': option.disabled
                  }"
                    @click="!option.disabled && (state.selected = option)">
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
                    {{ getProbeDescription(option.value) }}
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

              <!-- Target Mode Toggle -->
              <div v-if="showTargetAgentOption && probeTypeConfig.supportsCustomTarget" class="mb-4">
                <div class="btn-group w-100" role="group">
                  <input
                      type="radio"
                      class="btn-check"
                      name="targetMode"
                      id="targetModeAgent"
                      :checked="state.targetAgent"
                      @change="state.targetAgent = true">
                  <label class="btn btn-outline-primary" for="targetModeAgent">
                    <i class="fas fa-server me-2"></i>Target Agent
                  </label>

                  <input
                      type="radio"
                      class="btn-check"
                      name="targetMode"
                      id="targetModeCustom"
                      :checked="!state.targetAgent"
                      @change="state.targetAgent = false">
                  <label class="btn btn-outline-primary" for="targetModeCustom">
                    <i class="fas fa-globe me-2"></i>Custom Target
                  </label>
                </div>
              </div>

              <!-- Agent Selection -->
              <div v-if="state.targetAgent && showTargetAgentOption" class="mb-4">
                <label class="form-label fw-semibold" for="targetAgent">
                  <i class="fas fa-server me-2"></i>Target Agent
                </label>
                <select
                    id="targetAgent"
                    v-model="state.targetAgentSelected"
                    class="form-select form-select-lg"
                    :disabled="state.loading || availableAgentsForSelection.length === 0">
                  <option :value="null" disabled>Select an agent</option>
                  <option
                      v-for="agent in availableAgentsForSelection"
                      :key="agent.id"
                      :value="agent">
                    {{ agent.name }}
                    <span v-if="agent.location">({{ agent.location }})</span>
                  </option>
                </select>
                <small v-if="availableAgentsForSelection.length === 0" class="text-warning">
                  <i class="fas fa-info-circle me-1"></i>No other agents available in this workspace
                </small>
              </div>

              <!-- Custom Target Input for PING/MTR -->
              <div v-if="(state.selected.value === 'PING' || state.selected.value === 'MTR') && showCustomTargetInput" class="mb-4">
                <label class="form-label fw-semibold" for="pingTarget">
                  <i class="fas fa-bullseye me-2"></i>Target Address
                </label>
                <div class="input-group">
                  <span class="input-group-text"><i class="fas fa-globe"></i></span>
                  <input
                      id="pingTarget"
                      v-model="state.hostInput"
                      class="form-control"
                      type="text"
                      placeholder="1.1.1.1 or google.com">
                </div>
                <small class="text-muted">Enter an IP address or domain name</small>
              </div>

              <!-- DNS Configuration -->
              <div v-if="state.selected.value === 'DNS'" class="mb-4">
                <label class="form-label fw-semibold" for="dnsTarget">
                  <i class="fas fa-globe me-2"></i>Domain to Monitor
                </label>
                <div class="input-group mb-3">
                  <span class="input-group-text"><i class="fas fa-globe"></i></span>
                  <input
                      id="dnsTarget"
                      v-model="state.hostInput"
                      class="form-control"
                      type="text"
                      placeholder="example.com">
                </div>

                <!-- DNS Record Type Selection -->
                <label class="form-label fw-semibold">
                  <i class="fas fa-list me-2"></i>DNS Record Types
                </label>
                <div class="dns-record-types mb-3">
                  <div class="form-check form-switch mb-2">
                    <input
                        id="dnsRecordAll"
                        v-model="state.dnsConfig.queryAll"
                        class="form-check-input"
                        type="checkbox"
                        @change="handleDnsQueryAllChange">
                    <label class="form-check-label" for="dnsRecordAll">
                      Query All Record Types
                      <small class="text-muted d-block">Monitor all DNS record types for comprehensive coverage</small>
                    </label>
                  </div>

                  <div v-if="!state.dnsConfig.queryAll" class="dns-record-grid">
                    <div v-for="recordType in dnsRecordTypes" :key="recordType.value" class="form-check">
                      <input
                          :id="`dnsRecord${recordType.value}`"
                          v-model="state.dnsConfig.selectedRecordTypes"
                          class="form-check-input"
                          type="checkbox"
                          :value="recordType.value">
                      <label :for="`dnsRecord${recordType.value}`" class="form-check-label">
                        <span class="record-type-name">{{ recordType.value }}</span>
                        <small class="text-muted">{{ recordType.description }}</small>
                      </label>
                    </div>
                  </div>
                </div>

                <!-- DNS Server Configuration -->
                <label class="form-label fw-semibold" for="dnsServer">
                  <i class="fas fa-server me-2"></i>DNS Server (Optional)
                </label>
                <div class="input-group mb-2">
                  <span class="input-group-text"><i class="fas fa-server"></i></span>
                  <input
                      id="dnsServer"
                      v-model="state.dnsConfig.dnsServer"
                      class="form-control"
                      type="text"
                      placeholder="8.8.8.8 or 1.1.1.1">
                </div>
                <small class="text-muted">Leave empty to use system default DNS server</small>

                <!-- Advanced DNS Options -->
                <div class="mt-3">
                  <div class="form-check">
                    <input
                        id="dnssecValidation"
                        v-model="state.dnsConfig.dnssecValidation"
                        class="form-check-input"
                        type="checkbox">
                    <label class="form-check-label" for="dnssecValidation">
                      Enable DNSSEC Validation
                      <small class="text-muted d-block">Verify DNS responses are authenticated</small>
                    </label>
                  </div>
                </div>
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
                        v-model="state.probe.server"
                        class="form-check-input"
                        type="checkbox">
                    <label class="form-check-label" for="trafficSimServer">
                      Enable Server Mode
                      <small class="text-muted d-block">Run as a traffic receiver (only one server per agent allowed)</small>
                    </label>
                  </div>
                </div>

                <!-- Server Listening Configuration -->
                <div v-if="state.probe.server" class="mt-3">
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

              <!-- Probe Timing Configuration -->
              <div class="configuration-section">
                <h6 class="section-title">Probe Settings</h6>

                <!-- Interval -->
                <div class="mb-4">
                  <label class="form-label fw-semibold" for="probeInterval">
                    <i class="fas fa-clock me-2"></i>Probe Interval
                  </label>
                  <div class="input-group">
                    <input
                        id="probeInterval"
                        v-model.number="state.probe.interval_sec"
                        class="form-control"
                        type="number"
                        min="10"
                        max="3600">
                    <span class="input-group-text">seconds</span>
                  </div>
                  <small class="text-muted">
                    How often to run the probe (recommended: {{ probeTypeConfig.defaultInterval }} seconds)
                  </small>
                </div>

                <!-- Count (for PING/MTR) -->
                <div v-if="['PING', 'MTR'].includes(state.selected.value)" class="mb-4">
                  <label class="form-label fw-semibold" for="probeCount">
                    <i class="fas fa-hashtag me-2"></i>Packet Count
                  </label>
                  <div class="input-group">
                    <input
                        id="probeCount"
                        v-model.number="state.probe.count"
                        class="form-control"
                        type="number"
                        min="1"
                        max="100">
                    <span class="input-group-text">packets</span>
                  </div>
                  <small class="text-muted">
                    Number of packets to send per probe run (recommended: {{ probeTypeConfig.defaultCount }})
                  </small>
                </div>

                <!-- Duration (for TRAFFICSIM/RPERF) -->
                <div v-if="['TRAFFICSIM', 'RPERF'].includes(state.selected.value)" class="mb-4">
                  <label class="form-label fw-semibold" for="probeDuration">
                    <i class="fas fa-stopwatch me-2"></i>Test Duration
                  </label>
                  <div class="input-group">
                    <input
                        id="probeDuration"
                        v-model.number="state.probe.duration_sec"
                        class="form-control"
                        type="number"
                        min="10"
                        max="300">
                    <span class="input-group-text">seconds</span>
                  </div>
                  <small class="text-muted">
                    How long to run each test (recommended: {{ probeTypeConfig.defaultDuration }} seconds)
                  </small>
                </div>

                <!-- Timeout -->
                <div class="mb-4">
                  <label class="form-label fw-semibold" for="probeTimeout">
                    <i class="fas fa-hourglass-half me-2"></i>Timeout
                  </label>
                  <div class="input-group">
                    <input
                        id="probeTimeout"
                        v-model.number="state.probe.timeout_sec"
                        class="form-control"
                        type="number"
                        min="5"
                        max="300">
                    <span class="input-group-text">seconds</span>
                  </div>
                  <small class="text-muted">
                    Maximum time to wait for probe completion (recommended: {{ probeTypeConfig.defaultTimeout }} seconds)
                  </small>
                </div>

                <!-- Enable/Disable -->
                <div class="mb-4">
                  <div class="form-check form-switch">
                    <input
                        id="probeEnabled"
                        v-model="state.probe.enabled"
                        class="form-check-input"
                        type="checkbox">
                    <label class="form-check-label" for="probeEnabled">
                      Enable probe immediately after creation
                      <small class="text-muted d-block">You can enable/disable the probe later from the agent dashboard</small>
                    </label>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Form Actions -->
          <div class="card-footer">
            <div class="d-flex justify-content-between align-items-center">
              <router-link
                  :to="`/workspace/${state.workspace.id}/agent/${state.agent.id}`"
                  class="btn btn-outline-secondary">
                <i class="fas fa-arrow-left me-2"></i>Cancel
              </router-link>
              <button
                  class="btn btn-primary btn-lg px-5"
                  type="submit"
                  @click="submit"
                  :disabled="!isValidProbe || state.loading || state.duplicateWarning">
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

.probe-type-card:hover:not(.disabled) {
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

.probe-type-card.disabled {
  opacity: 0.6;
  cursor: not-allowed;
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

.configuration-section:last-child {
  margin-bottom: 0;
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

/* DNS Configuration Styles */
.dns-record-types {
  background-color: #f8f9fa;
  padding: 1rem;
  border-radius: 0.5rem;
  border: 1px solid #e9ecef;
}

.dns-record-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 0.75rem;
  margin-top: 1rem;
}

.dns-record-grid .form-check {
  padding: 0.5rem;
  background-color: white;
  border: 1px solid #e9ecef;
  border-radius: 0.25rem;
  transition: all 0.2s ease;
}

.dns-record-grid .form-check:hover {
  border-color: #0d6efd;
  box-shadow: 0 0.125rem 0.25rem rgba(0, 0, 0, 0.075);
}

.record-type-name {
  font-weight: 600;
  color: #495057;
  margin-right: 0.25rem;
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

/* Button Group */
.btn-group {
  box-shadow: 0 0.125rem 0.25rem rgba(0, 0, 0, 0.075);
}

.btn-check:checked + .btn {
  background-color: #0d6efd;
  color: white;
  border-color: #0d6efd;
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

.btn-primary:disabled {
  opacity: 0.65;
  cursor: not-allowed;
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

  .btn-group {
    flex-direction: column;
  }

  .btn-group .btn {
    border-radius: 0.375rem !important;
    margin-bottom: 0.5rem;
  }

  .btn-group .btn:last-child {
    margin-bottom: 0;
  }

  .dns-record-grid {
    grid-template-columns: 1fr;
  }
}
</style>