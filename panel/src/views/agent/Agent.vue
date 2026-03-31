<script lang="ts" setup>
import {onMounted, reactive, computed, ref, watch} from "vue";
import type {
  Agent,
  CPUTimes,
  HostInfo,
  HostMemoryInfo,
  NetInfoPayload,
  OSInfo,
  OUIEntry,
  Probe,
  ProbeData,
  ProbeType,
  SysInfoPayload,
  Target,
  Workspace,
  PingResult,
  Role
} from "@/types";
import {usePermissions} from "@/composables/usePermissions";
import {useWebSocket, type ProbeDataEvent} from "@/composables/useWebSocket";
import {useAgentStatus, type AgentStatusTier} from "@/composables/useAgentStatus";
import core from "@/core";
import Title from "@/components/Title.vue";
import Chart from "@/components/Chart.vue"
import ElementLink from "@/components/ElementLink.vue";
import Element from "@/components/Element.vue";
import List from "@/components/List.vue";
import {since} from "@/time";
import ElementPair from "@/components/ElementPair.vue";
import FillChart from "@/components/FillChart.vue";
import ElementExpand from "@/components/ElementExpand.vue";
import {AgentService, ProbeService, WorkspaceService, ProbeDataService, OUIService} from "@/services/apiService";
import {groupProbesByTarget, type TargetGroupKind, type ProbeGroupByTarget} from "@/utils/probeGrouping";
import ShareAgentModal from "@/components/ShareAgentModal.vue";
import DnsDashboard from "@/views/agent/DNS.vue";
import WebDashboard from "@/views/agent/Web.vue";
import AgentHeader from "@/components/agent/AgentHeader.vue";
import QuickStatsBar from "@/components/agent/QuickStatsBar.vue";
import UninitializedState from "@/components/agent/UninitializedState.vue";
import OverviewTab from "@/components/agent/OverviewTab.vue";
import ProbesTab from "@/components/agent/ProbesTab.vue";
import SystemTab from "@/components/agent/SystemTab.vue";

interface OrganizedProbe {
  key: string;
  probes: Probe[];
}

interface CpuUsage {
  idle: number
  system: number
  user: number
}

interface MemoryUsage {
  used: number
  free: number
  total: number
}

interface SystemData {
  cpu: CpuUsage
  ram: MemoryUsage
  virtual: MemoryUsage
}

interface LoadingState {
  agent: boolean
  workspace: boolean
  probes: boolean
  systemInfo: boolean
  networkInfo: boolean
}

interface ProbeGroupStats {
  lastRun?: string
  successRate?: number
  avgResponseTime?: number
  status?: 'healthy' | 'warning' | 'critical' | 'unknown'
  isLoading?: boolean
  hasData?: boolean
}

interface PingStats {
  probeId: number
  successRate: number
  avgResponseTime: number
  lastRun: string
  status: 'healthy' | 'warning' | 'critical' | 'unknown'
}

// Loading state management
const loadingState = reactive<LoadingState>({
  agent: true,
  workspace: true,
  probes: true,
  systemInfo: true,
  networkInfo: true
})

// Active tab for the new layout
const activeTab = ref<'overview' | 'probes' | 'system' | 'dns' | 'web'>('overview')

// Overall loading computed
const isInitializing = computed(() => {
  return loadingState.agent || loadingState.workspace
})

const isLoadingData = computed(() => {
  return loadingState.systemInfo || loadingState.networkInfo || loadingState.probes
})

// Data ready states
const hasSystemData = computed(() => {
  return state.systemInfo && state.systemInfo.hostInfo && !loadingState.systemInfo
})

const hasNetworkData = computed(() => {
  return state.networkInfo && state.networkInfo.public_address && !loadingState.networkInfo
})

// Interface/route data checks (with nil safety)
const hasP11Interfaces = computed(() => {
  return state.networkInfo?.interfaces && state.networkInfo.interfaces.length > 0
})

const hasP11Routes = computed(() => {
  return state.networkInfo?.routes && state.networkInfo.routes.length > 0
})

// Error states
const errors = reactive({
  agent: null as string | null,
  workspace: null as string | null,
  probes: null as string | null,
  systemInfo: null as string | null,
  networkInfo: null as string | null
})

// Agent status composable for consistent status logic
const agentStatus = useAgentStatus();

// Computed properties for agent status using shared composable
const currentAgentStatus = computed(() => {
  if (!state.agent.updated_at || loadingState.agent) return 'offline';
  return agentStatus.getAgentStatus(state.agent);
});

const isOnline = computed(() => {
  return currentAgentStatus.value === 'online';
});

const isStale = computed(() => {
  return currentAgentStatus.value === 'stale';
});

const cpuUsagePercent = computed(() => {
  if (!hasSystemData.value || !state.systemData?.cpu) return 0;
  return ((state.systemData.cpu.user + state.systemData.cpu.system) * 100).toFixed(1);
});

const memoryUsagePercent = computed(() => {
  if (!hasSystemData.value || !state.systemData?.ram) return 0;
  return (state.systemData.ram.used * 100).toFixed(1);
});

// Status level based on usage percentage
const cpuStatusLevel = computed(() => {
  const value = parseFloat(cpuUsagePercent.value as string) || 0;
  if (value >= 90) return 'critical';
  if (value >= 70) return 'warning';
  return 'healthy';
});

const memoryStatusLevel = computed(() => {
  const value = parseFloat(memoryUsagePercent.value as string) || 0;
  if (value >= 90) return 'critical';
  if (value >= 70) return 'warning';
  return 'healthy';
});

// Circular progress ring calculations (circumference = 2 * PI * radius)
// Copy to clipboard helper

// Copy to clipboard helper
const copiedField = ref<string | null>(null);
async function copyToClipboard(text: string, fieldName: string) {
  try {
    await navigator.clipboard.writeText(text);
    copiedField.value = fieldName;
    setTimeout(() => {
      copiedField.value = null;
    }, 2000);
  } catch (err) {
    console.error('Failed to copy:', err);
  }
}

function roundTo(value: number): number {
  return Math.round(value * 1000) / 1000
}

// Computed property for active probes count
const activeProbesCount = computed(() => {
  return state.activeProbes || 0;
});

// Computed property for total probes count
const totalProbesCount = computed(() => {
  return state.totalProbes || 0;
});

// Computed property for probe statistics
const probeStats = computed(() => {
  return {
    active: state.activeProbes || 0,
    total: state.totalProbes || 0,
    percentage: state.totalProbes > 0
        ? Math.round((state.activeProbes / state.totalProbes) * 100)
        : 0,
    targets: state.totalTargets || 0,
    byType: state.totalsByType || {}
  };
});

// Function to get active probes by type
function getActiveProbesByType(type: string): number {
  return state.totalsByType[type]?.enabled || 0;
}

// Function to get total probes by type
function getTotalProbesByType(type: string): number {
  return state.totalsByType[type]?.probes || 0;
}

function updateSystemData(info: SysInfoPayload): SystemData {
  // Guard against missing data (e.g., agent is offline)
  if (!info?.CPUTimes || !info?.memoryInfo) {
    return {
      cpu: { idle: 0, system: 0, user: 0 },
      ram: { used: 0, free: 0, total: 0 },
      virtual: { used: 0, free: 0, total: 0 }
    } as SystemData;
  }
  
  let cpuCapacity: number = (info.CPUTimes.idle || 0) + (info.CPUTimes.system || 0) + (info.CPUTimes.user || 0);
  // Avoid division by zero
  if (cpuCapacity === 0) cpuCapacity = 1;
  
  let ramCapacity: number = info.memoryInfo.total_bytes || 1;
  let virtualCapacity: number = info.memoryInfo.virtual_total_bytes || 1;
  
  return {
    cpu: {
      idle: roundTo((info.CPUTimes.idle || 0) / cpuCapacity),
      system: roundTo((info.CPUTimes.system || 0) / cpuCapacity),
      user: roundTo((info.CPUTimes.user || 0) / cpuCapacity),
    },
    ram: {
      used: roundTo((info.memoryInfo.used_bytes || 0) / ramCapacity),
      free: roundTo((info.memoryInfo.available_bytes || 0) / ramCapacity),
      total: roundTo((info.memoryInfo.total_bytes || 0) / ramCapacity),
    },
    virtual: {
      used: roundTo((info.memoryInfo.virtual_used_bytes || 0) / ramCapacity),
      free: roundTo((info.memoryInfo.virtual_free_bytes || 0) / virtualCapacity),
      total: roundTo((info.memoryInfo.virtual_total_bytes || 0) / virtualCapacity),
    }
  } as SystemData
}

// OUI vendor cache: MAC -> vendor name
const ouiCache = reactive<Record<string, string>>({});

async function getVendorFromMac(macAddress: string): Promise<string> {
  if (!macAddress) return 'Unknown';
  
  const normalizedMac = macAddress.replace(/[:-]/g, '').toUpperCase();
  
  // Check cache first
  if (ouiCache[normalizedMac]) {
    return ouiCache[normalizedMac];
  }
  
  // Mark as loading
  ouiCache[normalizedMac] = 'Looking up...';
  
  try {
    const result = await OUIService.lookup(macAddress);
    ouiCache[normalizedMac] = result.found ? result.vendor : 'Unknown Vendor';
  } catch (err) {
    ouiCache[normalizedMac] = 'Unknown Vendor';
  }
  
  return ouiCache[normalizedMac];
}

// Computed for synchronous template access (uses cache)
function getVendorSync(macAddress: string): string {
  if (!macAddress) return 'Unknown';
  const normalizedMac = macAddress.replace(/[:-]/g, '').toUpperCase();
  return ouiCache[normalizedMac] || 'Loading...';
}



// Calculate probe health status based on metrics
function calculateProbeStatus(successRate: number, avgResponseTime: number): 'healthy' | 'warning' | 'critical' | 'unknown' {
  if (successRate >= 95 && avgResponseTime < 100) return 'healthy';
  if (successRate >= 80 && avgResponseTime < 200) return 'warning';
  if (successRate < 80 || avgResponseTime >= 200) return 'critical';
  return 'unknown';
}

// Fetch real probe statistics for ping probes using latest endpoint
async function fetchPingStats(workspaceId: string, agentId: string, probes: Probe[]): Promise<PingStats[]> {
  // Include PING probes and AGENT probes (which expand to include PING)
  const pingProbes = probes.filter(p => (p.type === 'PING' || p.type === 'AGENT') && p.enabled);
  if (pingProbes.length === 0) return [];

  try {
    // Fetch latest data for all ping probes in parallel
    const statsPromises = pingProbes.map(async (probe) => {
      try {
        // Get the latest ping data for this probe
        const latestData = await ProbeDataService.latest(
            workspaceId,
            {
              type: 'PING',
              agentId: agentId,
              probeId: probe.id
            }
        );

        if (!latestData || !latestData.payload) {
          return null;
        }

        // Extract ping result from the latest data
        const pingResult = latestData.payload as PingResult;

        // Calculate success rate from the ping result
        const successRate = pingResult.packets_sent > 0
            ? (pingResult.packets_recv / pingResult.packets_sent) * 100
            : 0;

        const avgResponseTime = pingResult.avg_rtt / 1000000 || 0;
        const lastRun = latestData.created_at;
        const status = calculateProbeStatus(successRate, avgResponseTime);

        return {
          probeId: probe.id,
          successRate,
          avgResponseTime,
          lastRun,
          status
        } as PingStats;
      } catch (err) {
        // 404 means no data yet for this probe
        console.log(`No ping data yet for probe ${probe.id}`);
        return null;
      }
    });

    const results = await Promise.all(statsPromises);
    return results.filter(r => r !== null) as PingStats[];
  } catch (err) {
    console.error('Failed to fetch ping stats:', err);
    return [];
  }
}

// Aggregate stats for a probe group
function aggregateGroupStats(group: ProbeGroupByTarget, pingStats: PingStats[]): ProbeGroupStats {
  // Check if this group has any ping or agent probes (AGENT expands to include PING)
  const hasPingProbes = group.probes.some(p => p.type === 'PING');
  const hasAgentProbes = group.probes.some(p => p.type === 'AGENT');

  if (!hasPingProbes && !hasAgentProbes) {
    return {
      hasData: false,
      status: 'unknown',
      isLoading: false
    };
  }

  const groupPingStats = pingStats.filter(stat =>
      group.probes.some(p => p.id === stat.probeId)
  );

  if (groupPingStats.length === 0) {
    return {
      hasData: false,
      status: 'unknown',
      isLoading: false
    };
  }

  // Calculate weighted averages
  const totalSuccessRate = groupPingStats.reduce((sum, stat) => sum + stat.successRate, 0);
  const totalResponseTime = groupPingStats.reduce((sum, stat) => sum + stat.avgResponseTime, 0);
  const avgSuccessRate = totalSuccessRate / groupPingStats.length;
  const avgResponseTime = totalResponseTime / groupPingStats.length;

  // Get the most recent run time
  const lastRun = groupPingStats
      .map(stat => new Date(stat.lastRun))
      .sort((a, b) => b.getTime() - a.getTime())[0]
      .toISOString();

  // Determine overall status (use worst status in group)
  const statuses = groupPingStats.map(s => s.status);
  let overallStatus: 'healthy' | 'warning' | 'critical' | 'unknown' = 'healthy';
  if (statuses.includes('critical')) overallStatus = 'critical';
  else if (statuses.includes('warning')) overallStatus = 'warning';
  else if (statuses.includes('unknown')) overallStatus = 'unknown';

  return {
    lastRun,
    successRate: avgSuccessRate,
    avgResponseTime,
    status: overallStatus,
    hasData: true,
    isLoading: false
  };
}

// Initialize placeholder stats for loading state
function initializeGroupStats(group: ProbeGroupByTarget): ProbeGroupStats {
  // This is just a placeholder while real stats are loading
  return {
    isLoading: true,
    status: 'unknown'
  };
}

function getStatusColor(status?: string): string {
  switch (status) {
    case 'healthy':
      return 'text-success';
    case 'warning':
      return 'text-warning';
    case 'critical':
      return 'text-danger';
    default:
      return 'text-muted';
  }
}

function getStatusIcon(status?: string): string {
  switch (status) {
    case 'healthy':
      return 'bi-check-circle-fill';
    case 'warning':
      return 'bi-exclamation-triangle-fill';
    case 'critical':
      return 'bi-x-circle-fill';
    default:
      return 'bi-question-circle';
  }
}

const router = core.router()

let state = reactive({
  workspace: {} as Workspace & { my_role?: Role },
  probes: [] as Probe[],

  // group-driven UI
  // grouped by target (type-agnostic)
  targetGroups: [] as ProbeGroupByTarget[],
  targetGroupsByKey: {} as Record<string, ProbeGroupByTarget>,
  groupKinds: [] as TargetGroupKind[],

  // Group stats
  groupStats: {} as Record<string, ProbeGroupStats>,
  pingStats: [] as PingStats[],
  loadingPingStats: true,

  // totals for badges
  totalProbes: 0,
  activeProbes: 0,
  totalTargets: 0,
  totalsByType: {} as Record<string, { probes: number; enabled: number; targets: number }>,

  ready: false,
  agent: {} as Agent,
  agents: [] as Agent[],
  agentNames: {} as Record<number, string>,  // Cache of agent ID → name for target display
  targetAgents: {} as Record<number, Agent>,  // Full agent objects for target status checks
  networkInfo: {} as NetInfoPayload,
  systemInfo: {} as SysInfoPayload,
  systemData: {} as SystemData,
  hasData: false,
  // Pending PIN for uninitialized agents
  pendingPin: '',
  loadingPendingPin: false,
})

// Permissions based on user's role in this workspace
const permissions = computed(() => usePermissions(state.workspace.my_role));

// Real-time data state
const liveUpdating = ref(false);

// Share modal state
const showShareModal = ref(false);
const lastLiveUpdate = ref<Date | null>(null);

// Get workspace/agent IDs as refs for the WebSocket composable
const workspaceIdRef = computed(() => state.workspace.id);

// WebSocket for real-time updates - subscribe to all probes in workspace (probeId=0)
const { connected: wsConnected, subscribe: wsSubscribe } = useWebSocket({ autoConnect: true });

// Handle real-time probe data updates
function handleLiveProbeData(data: ProbeDataEvent) {
  // Only process data for this agent
  if (data.agent_id !== state.agent.id) return;

  liveUpdating.value = true;
  lastLiveUpdate.value = new Date();

  // Update agent's last seen (shows it's still active)
  state.agent.updated_at = new Date().toISOString();

  // Handle different probe types
  switch (data.type) {
    case 'SYSINFO':
      const sysPayload = data.payload as unknown as SysInfoPayload;
      if (sysPayload) {
        state.systemInfo = sysPayload;
        state.systemData = updateSystemData(sysPayload);
        state.hasData = true;
        loadingState.systemInfo = false;
      }
      break;

    case 'NETINFO':
      const netPayload = data.payload as unknown as NetInfoPayload;
      if (netPayload) {
        state.networkInfo = netPayload;
        loadingState.networkInfo = false;
      }
      break;

    case 'PING':
      // Update ping stats for the relevant probe
      const pingPayload = data.payload as unknown as PingResult;
      if (pingPayload && data.probe_id) {
        const existingIdx = state.pingStats.findIndex(s => s.probeId === data.probe_id);
        const successRate = pingPayload.packets_sent > 0
          ? (pingPayload.packets_recv / pingPayload.packets_sent) * 100
          : 0;
        const avgResponseTime = pingPayload.avg_rtt / 1000000 || 0;
        const newStat: PingStats = {
          probeId: data.probe_id,
          successRate,
          avgResponseTime,
          lastRun: data.created_at,
          status: calculateProbeStatus(successRate, avgResponseTime)
        };

        if (existingIdx >= 0) {
          state.pingStats[existingIdx] = newStat;
        } else {
          state.pingStats.push(newStat);
        }

        // Re-aggregate group stats
        state.targetGroups.forEach(group => {
          const stats = aggregateGroupStats(group, state.pingStats);
          state.groupStats[group.key] = { ...stats, isLoading: false };
        });
      }
      break;
  }

  // Clear the live updating indicator after a short delay
  setTimeout(() => {
    liveUpdating.value = false;
  }, 500);
}

// Subscribe to workspace updates when workspace ID is available
watch(workspaceIdRef, (wsId) => {
  if (wsId) {
    wsSubscribe(wsId, 0, handleLiveProbeData);
  }
}, { immediate: true });

onMounted(async () => {
  let agentID = router.currentRoute.value.params["aID"] as string
  let workspaceID = router.currentRoute.value.params["wID"] as string
  if (!agentID || !workspaceID) return

  // Load OUI vendor info for system MACs (non-blocking)
  if (state.systemInfo?.hostInfo?.mac) {
    state.systemInfo.hostInfo.mac.forEach(mac => getVendorFromMac(mac));
  }

  // Load workspace and agent info first (required for page title)
  try {
    const [workspaceRes, agentRes] = await Promise.all([
      WorkspaceService.get(workspaceID),
      AgentService.get(workspaceID, agentID)
    ]);

    state.workspace = workspaceRes as Workspace;
    state.agent = agentRes as Agent;
    loadingState.workspace = false;
    loadingState.agent = false;

    // If agent is not initialized, fetch pending PIN
    if (!state.agent.initialized) {
      state.loadingPendingPin = true;
      try {
        const pinResult = await AgentService.getPendingPin(workspaceID, agentID);
        state.pendingPin = pinResult.pin || '';
      } catch (err) {
        console.log('No pending PIN available');
      } finally {
        state.loadingPendingPin = false;
      }
    }
  } catch (err) {
    console.error('Failed to load workspace/agent:', err);
    errors.workspace = 'Failed to load workspace';
    errors.agent = 'Failed to load agent';
    loadingState.workspace = false;
    loadingState.agent = false;
  }

  // Load system data in parallel (non-blocking)
  ProbeService.sysInfo(workspaceID, agentID)
      .then(res => {
        let pD = res as ProbeData
        // Guard against empty/null payload (agent offline or no data yet)
        if (!pD?.payload) {
          console.log('No system info data available (agent may be offline)');
          return;
        }
        state.systemInfo = pD.payload as SysInfoPayload
        state.systemData = updateSystemData(state.systemInfo)
        state.hasData = true
        // Trigger OUI lookups for MACs
        if (state.systemInfo?.hostInfo?.mac) {
          state.systemInfo.hostInfo.mac.forEach(mac => getVendorFromMac(mac));
        }
      })
      .catch(err => {
        console.error('Failed to load system info:', err);
        errors.systemInfo = 'Failed to load system information';
      })
      .finally(() => {
        loadingState.systemInfo = false;
      });

  ProbeService.netInfo(workspaceID, agentID)
      .then(res => {
        let pD = res as ProbeData
        state.networkInfo = pD.payload as NetInfoPayload
        // Trigger OUI lookups for interface MACs
        if (state.networkInfo?.interfaces) {
          state.networkInfo.interfaces.forEach(iface => {
            if (iface.mac) getVendorFromMac(iface.mac);
          });
        }
      })
      .catch(err => {
        console.error('Failed to load network info:', err);
        errors.networkInfo = 'Failed to load network information';
      })
      .finally(() => {
        loadingState.networkInfo = false;
      });

  // Load & group probes
  ProbeService.list(workspaceID, agentID)
      .then(async (res) => {
        const pL = res as Probe[];
        state.probes = pL;

        // Exclude DNS, HTTP, and TLS probes from the normal probe list — they have dedicated tabs
        const nonDnsProbes = pL.filter(p => p.type !== 'DNS' && p.type !== 'HTTP' && p.type !== 'TLS');
        const grouped = groupProbesByTarget(nonDnsProbes, { excludeDefaults: true, excludeServers: true });

        state.targetGroups = grouped.groups;
        state.targetGroupsByKey = grouped.byKey;
        state.groupKinds = grouped.kinds;

        state.totalProbes = grouped.totals.probes;
        state.activeProbes = grouped.totals.enabled;
        state.totalTargets = grouped.totals.targets;
        state.totalsByType = grouped.totals.byType;

        // Initialize loading states for each group
        grouped.groups.forEach(group => {
          state.groupStats[group.key] = initializeGroupStats(group);
        });

        // Lookup agent names and store full agent objects for agent-type groups
        const agentGroups = grouped.groups.filter(g => g.kind === 'agent');
        for (const g of agentGroups) {
          const targetAgentId = Number(g.id);
          if (targetAgentId && !state.agentNames[targetAgentId]) {
            try {
              const targetAgent = await AgentService.get(workspaceID, targetAgentId) as Agent;
              state.agentNames[targetAgentId] = targetAgent.name || `Agent #${targetAgentId}`;
              state.targetAgents[targetAgentId] = targetAgent;
            } catch {
              state.agentNames[targetAgentId] = `Agent #${targetAgentId}`;
            }
          }
        }

        // Fetch ping statistics if there are ping or agent probes
        const hasPingProbes = pL.some(p => p.type === 'PING' || p.type === 'AGENT');
        if (hasPingProbes) {
          state.loadingPingStats = true;
          try {
            const pingStats = await fetchPingStats(workspaceID, agentID, pL);
            state.pingStats = pingStats;

            // Update group stats with real data
            grouped.groups.forEach(group => {
              const stats = aggregateGroupStats(group, pingStats);
              state.groupStats[group.key] = {
                ...stats,
                isLoading: false
              };
            });
          } catch (err) {
            console.error('Failed to load ping statistics:', err);
            // Set all groups to error state
            grouped.groups.forEach(group => {
              state.groupStats[group.key] = {
                isLoading: false,
                hasData: false,
                status: 'unknown'
              };
            });
          } finally {
            state.loadingPingStats = false;
          }
        } else {
          // No ping probes, set all groups to no data
          grouped.groups.forEach(group => {
            state.groupStats[group.key] = {
              isLoading: false,
              hasData: false,
              status: 'unknown'
            };
          });
          state.loadingPingStats = false;
        }
      })
      .catch(err => {
        console.error('Failed to load probes:', err);
        errors.probes = 'Failed to load probes';
        state.loadingPingStats = false;
      })
      .finally(() => {
        loadingState.probes = false;
      });

  state.ready = true;
})

</script>

<template>
  <div class="container-fluid">
    <AgentHeader
      :agent="state.agent"
      :workspace="state.workspace"
      :permissions="permissions"
      :is-initializing="isInitializing"
      :current-status="currentAgentStatus"
      :ws-connected="wsConnected"
      :live-updating="liveUpdating"
      @share="showShareModal = true"
      @edit-probes="null"
      @add-probe="null"
    />

    <QuickStatsBar
      :loading-state="loadingState"
      :system-info="state.systemInfo"
      :system-data="state.systemData"
      :total-probes="totalProbesCount"
      :target-groups-length="state.targetGroups.length"
    />


    <!-- Error Messages -->
    <div v-if="Object.values(errors).some(e => e !== null)" class="alert alert-warning mt-3">
      <i class="bi bi-exclamation-triangle"></i>
      <strong>Some data could not be loaded:</strong>
      <ul class="mb-0 mt-2">
        <li v-for="(error, key) in errors" v-if="error" :key="key">{{ error }}</li>
      </ul>
    </div>

    <!-- Main Content -->
    <UninitializedState
      v-if="!state.agent.initialized && !loadingState.agent"
      :agent="state.agent"
      :workspace="state.workspace"
      :pending-pin="state.pendingPin"
      :is-loading-pin="state.loadingPendingPin"
    />

    <div v-else class="agent-content">
      <!-- Tab Navigation -->
      <div class="agent-tabs">
        <button type="button" class="tab-btn" :class="{ active: activeTab === 'overview' }" @click="activeTab = 'overview'"><i class="bi bi-grid-3x3-gap"></i> Overview</button>
        <button type="button" class="tab-btn" :class="{ active: activeTab === 'probes' }" @click="activeTab = 'probes'"><i class="bi bi-diagram-2"></i> Probes <span class="tab-badge" v-if="!loadingState.probes">{{ totalProbesCount }}</span></button>
        <button type="button" class="tab-btn" :class="{ active: activeTab === 'system' }" @click="activeTab = 'system'"><i class="bi bi-cpu"></i> System</button>
        <button type="button" class="tab-btn" :class="{ active: activeTab === 'dns' }" @click="activeTab = 'dns'"><i class="bi bi-globe2"></i> DNS</button>
        <button type="button" class="tab-btn" :class="{ active: activeTab === 'web' }" @click="activeTab = 'web'"><i class="bi bi-globe"></i> Web</button>
      </div>

      <!-- OVERVIEW TAB -->
      <OverviewTab
        v-show="activeTab === 'overview'"
        :agent="state.agent"
        :workspace-id="state.workspace.id"
        :loading-state="{ systemInfo: loadingState.systemInfo, networkInfo: loadingState.networkInfo }"
        :system-info="state.systemInfo"
        :network-info="state.networkInfo"
        :is-online="isOnline"
        :oui-cache="ouiCache"
      />

      <!-- PROBES TAB -->
      <ProbesTab
        v-show="activeTab === 'probes'"
        :loading-probes="loadingState.probes"
        :total-probes="totalProbesCount"
        :target-groups="state.targetGroups"
        :group-stats="state.groupStats"
        :target-agents="state.targetAgents"
        :agent-names="state.agentNames"
        :workspace-id="state.workspace.id"
        :agent-id="state.agent.id"
      />

      <!-- DNS TAB - Dedicated DNS Monitoring Dashboard -->
      <div v-show="activeTab === 'dns'" class="tab-panel">
        <DnsDashboard
          v-if="state.workspace.id && state.agent.id"
          :workspace-id="String(state.workspace.id)"
          :agent-id="String(state.agent.id)"
        />
      </div>

      <!-- WEB TAB - Dedicated HTTP/TLS Monitoring Dashboard -->
      <div v-show="activeTab === 'web'" class="tab-panel">
        <WebDashboard
          v-if="state.workspace.id && state.agent.id"
          :workspace-id="String(state.workspace.id)"
          :agent-id="String(state.agent.id)"
        />
      </div>

      <!-- SYSTEM TAB -->
      <SystemTab
        v-show="activeTab === 'system'"
        :loading-state="{ systemInfo: loadingState.systemInfo, networkInfo: loadingState.networkInfo, agent: loadingState.agent }"
        :system-info="state.systemInfo"
        :system-data="state.systemData"
        :network-info="state.networkInfo"
        :agent="state.agent"
        :is-online="isOnline"
        :cpu-usage-percent="cpuUsagePercent"
        :memory-usage-percent="memoryUsagePercent"
        :cpu-status-level="cpuStatusLevel"
        :memory-status-level="memoryStatusLevel"
      />
    </div>
  </div>

  <!-- Share Agent Modal -->
  <ShareAgentModal
      v-if="showShareModal && state.agent.id && state.workspace.id"
      :workspace-id="state.workspace.id"
      :agent-id="state.agent.id"
      :agent-name="state.agent.name || 'Agent'"
      @close="showShareModal = false"
  />
</template>

<style scoped>
/* Loading States */
.skeleton {
  position: relative;
  overflow: hidden;
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

@keyframes skeleton-shimmer {
  0% {
    transform: translateX(-100%);
  }
  100% {
    transform: translateX(100%);
  }
}

.probe-title-skeleton {
  width: 150px;
  height: 20px;
  margin-bottom: 0.5rem;
}

.probe-type-skeleton {
  width: 60px;
  height: 18px;
}

.probe-stat-skeleton {
  width: 80px;
  height: 16px;
}

.stat-item.loading .stat-value {
  min-width: 60px;
}

.info-card.loading .info-value .skeleton-text {
  float: right;
}

.mac-item.skeleton {
  background: transparent;
}

/* Status Badge */
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.375rem 0.875rem;
  border-radius: 999px;
  font-size: 0.875rem;
  font-weight: 500;
}

.status-badge.online {
  background: #f0fdf4;
  color: #16a34a;
}

.status-badge.stale {
  background: #fffbeb;
  color: #d97706;
}

.status-badge.offline {
  background: #fef2f2;
  color: #dc2626;
}

.status-badge.loading {
  background: #f3f4f6;
  color: #6b7280;
}

.status-badge.live {
  background: #ecfdf5;
  color: #059669;
  transition: all 0.3s ease;
}

.status-badge.live.pulse {
  background: #34d399;
  color: white;
  box-shadow: 0 0 0 4px rgba(52, 211, 153, 0.3);
}

.status-badge.live i {
  font-size: 0.75rem;
}

@keyframes live-pulse {
  0% { box-shadow: 0 0 0 0 rgba(52, 211, 153, 0.4); }
  70% { box-shadow: 0 0 0 8px rgba(52, 211, 153, 0); }
  100% { box-shadow: 0 0 0 0 rgba(52, 211, 153, 0); }
}

.status-badge i {
  font-size: 0.5rem;
}

/* Mobile-first Quick Stats */
.quick-stats {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0.75rem;
  margin-bottom: 1.5rem;
}

@media (min-width: 640px) {
  .quick-stats {
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 1rem;
  }
}

.stat-item {
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 8px;
  padding: 1rem;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  transition: all 0.2s;
}

@media (min-width: 640px) {
  .stat-item {
    padding: 1.25rem;
    gap: 1rem;
  }
}

.stat-item:hover:not(.loading) {
  transform: translateY(-2px);
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
}

@media (prefers-reduced-motion: reduce) {
  .stat-item:hover:not(.loading) {
    transform: none;
  }
}

.stat-icon {
  width: 3rem;
  height: 3rem;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.5rem;
}

.stat-icon.cpu {
  background: #dbeafe;
  color: #3b82f6;
}

.stat-icon.memory {
  background: #d1fae5;
  color: #10b981;
}

.stat-icon.network {
  background: #fef3c7;
  color: #f59e0b;
}

.stat-icon.uptime {
  background: #ede9fe;
  color: #8b5cf6;
}

.stat-content {
  flex: 1;
}

.stat-value {
  font-size: 1.5rem;
  font-weight: 700;
  color: #1f2937;
  line-height: 1;
}

.stat-label {
  font-size: 0.875rem;
  color: #6b7280;
  margin-top: 0.25rem;
}

/* Content Sections */
.agent-content {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.content-section {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  overflow: hidden;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1.25rem;
  border-bottom: 1px solid #e5e7eb;
}

.section-title {
  margin: 0;
  font-size: 1.125rem;
  font-weight: 600;
  color: #1f2937;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.section-title i {
  color: #6b7280;
}

/* Content Sections */
.agent-content {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.content-section {
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 8px;
  overflow: hidden;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem;
  border-bottom: 1px solid var(--bs-border-color);
  background: var(--bs-tertiary-bg);
  flex-wrap: wrap;
  gap: 0.75rem;
}

@media (min-width: 640px) {
  .section-header {
    padding: 1.25rem;
  }
}

.section-title {
  margin: 0;
  font-size: 1.125rem;
  font-weight: 600;
  color: var(--bs-body-color);
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.section-title i {
  color: var(--bs-secondary-color);
}

/* Probes Grid - mobile first (defined in responsive section above) */
.probe-card {
  border: 1px solid var(--bs-border-color);
  border-radius: 8px;
  transition: all 0.2s;
  overflow: hidden;
  background: var(--bs-body-bg);
}

.probe-card:hover:not(.skeleton) {
  border-color: #3b82f6;
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
  transform: translateY(-2px);
}

.probe-card.has-issues {
  border-color: #fee2e2;
  background: #fef2f2;
}

.probe-card.has-issues:hover {
  border-color: #ef4444;
}

.probe-link {
  display: flex;
  align-items: flex-start;
  gap: 1rem;
  padding: 1rem;
  text-decoration: none;
  color: inherit;
  min-height: 120px;
}

.probe-header {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
}

.probe-icon {
  width: 2.5rem;
  height: 2.5rem;
  background: #eff6ff;
  color: #3b82f6;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.125rem;
}

.probe-status {
  font-size: 0.875rem;
}

.probe-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.probe-title {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
  color: #1f2937;
  line-height: 1.4;
}

.probe-types {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
}

.probe-type-badge {
  display: inline-block;
  padding: 0.125rem 0.5rem;
  background: #f3f4f6;
  color: #6b7280;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 500;
}

.probe-type-badge.inactive {
  background: #fee2e2;
  color: #dc2626;
}

.probe-stats {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  margin-top: 0.5rem;
}

.probe-stat {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.813rem;
  color: #6b7280;
}

.probe-stat i {
  font-size: 0.75rem;
  width: 1rem;
}

.probe-arrow {
  color: #9ca3af;
  margin-top: 0.25rem;
}

/* Info Grid */
.info-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(350px, 1fr));
  gap: 1.5rem;
}

.info-card {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  overflow: hidden;
}

.card-header {
  padding: 1.25rem;
  border-bottom: 1px solid #e5e7eb;
  background: #f9fafb;
}

.card-title {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
  color: #1f2937;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.card-title i {
  color: #6b7280;
}

.card-content {
  padding: 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.card-footer {
  padding: 1rem 1.25rem;
  border-top: 1px solid #e5e7eb;
  background: #f9fafb;
}

/* Info Rows */
.info-row {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 1rem;
}

.info-label {
  font-size: 0.875rem;
  color: #6b7280;
  font-weight: 500;
  min-width: 120px;
}

.info-value {
  font-size: 0.875rem;
  color: #1f2937;
  font-family: monospace;
  text-align: right;
  flex: 1;
}

/* Resource Meters */
.resource-meter {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.resource-header {
  display: flex;
  justify-content: space-between;
  font-size: 0.875rem;
  font-weight: 500;
}

.progress {
  height: 0.5rem;
  background: #f3f4f6;
  border-radius: 999px;
  overflow: hidden;
}

.progress-bar {
  height: 100%;
  transition: width 0.3s ease;
}

.resource-details {
  display: flex;
  justify-content: space-between;
  font-size: 0.75rem;
  color: #6b7280;
}

/* Memory Details */
.memory-details {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  margin-top: 0.5rem;
}

.detail-row {
  display: flex;
  justify-content: space-between;
  font-size: 0.813rem;
  padding: 0.25rem 0;
}

/* MAC List */
.mac-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin-top: 0.75rem;
}

.mac-item {
  padding: 0.5rem;
  background: #f9fafb;
  border-radius: 6px;
}

.mac-address {
  font-family: monospace;
  font-size: 0.875rem;
  color: #1f2937;
  margin-bottom: 0.25rem;
}

.mac-vendor {
  font-size: 0.75rem;
  color: #6b7280;
}

/* Empty State */
.empty-state {
  text-align: center;
  padding: 4rem 2rem;
  color: #6b7280;
}

.empty-state i {
  font-size: 3rem;
  margin-bottom: 1rem;
  display: block;
}

.empty-state h5 {
  color: #1f2937;
  margin-bottom: 0.5rem;
}

.empty-state p {
  margin-bottom: 1.5rem;
}

/* Alert */
.alert {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
}

.alert i {
  margin-top: 0.125rem;
}

.alert ul {
  padding-left: 1.25rem;
}

/* Text utilities */
.text-muted {
  color: #6b7280;
}

.text-success {
  color: #16a34a;
}

.text-warning {
  color: #f59e0b;
}

.text-danger {
  color: #dc2626;
}

/* Mobile-first Responsive Adjustments */

/* Info grid - single column on mobile */
.info-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 1rem;
}

@media (min-width: 768px) {
  .info-grid {
    grid-template-columns: repeat(auto-fit, minmax(350px, 1fr));
    gap: 1.5rem;
  }
}

/* Probes grid - mobile first */
.probes-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 1rem;
  padding: 1rem;
}

@media (min-width: 640px) {
  .probes-grid {
    grid-template-columns: repeat(2, 1fr);
    padding: 1.25rem;
  }
}

@media (min-width: 1024px) {
  .probes-grid {
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  }
}

/* Info rows - stack on mobile */
.info-row {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

@media (min-width: 640px) {
  .info-row {
    flex-direction: row;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1rem;
  }
}

.info-label {
  font-size: 0.875rem;
  color: #6b7280;
  font-weight: 500;
  min-width: auto;
}

@media (min-width: 640px) {
  .info-label {
    min-width: 120px;
  }
}

.info-value {
  font-size: 0.875rem;
  color: #1f2937;
  font-family: monospace;
  text-align: left;
  flex: 1;
}

@media (min-width: 640px) {
  .info-value {
    text-align: right;
  }
}

/* Mobile-specific adjustments */
@media (max-width: 576px) {
  .quick-stats {
    grid-template-columns: 1fr;
  }

  .stat-item {
    padding: 0.875rem;
  }
  
  .progress-ring-container {
    width: 56px;
    height: 56px;
  }
  
  .ring-icon {
    font-size: 1rem;
  }
}

/* Spin Animation for Bootstrap Icons */
.spin-animation {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* ========================================
   ENHANCED COMPONENTS
   ======================================== */

/* Circular Progress Ring */
.progress-ring-container {
  position: relative;
  width: 68px;
  height: 68px;
  flex-shrink: 0;
}

.progress-ring {
  transform: rotate(-90deg);
}

.progress-ring-bg {
  stroke: #e5e7eb;
}

.progress-ring-fill {
  stroke-linecap: round;
  transition: stroke-dashoffset 0.5s ease;
}

.progress-ring-fill.healthy {
  stroke: #10b981;
}

.progress-ring-fill.warning {
  stroke: #f59e0b;
}

.progress-ring-fill.critical {
  stroke: #ef4444;
}

.ring-icon {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  font-size: 1.25rem;
  color: #6b7280;
}

/* Glassmorphism Stat Items */
.stat-item.glass {
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(10px);
  border: 1px solid rgba(255, 255, 255, 0.3);
}

.stat-item.status-healthy {
  border-left: 3px solid #10b981;
}

.stat-item.status-warning {
  border-left: 3px solid #f59e0b;
}

.stat-item.status-critical {
  border-left: 3px solid #ef4444;
}

/* Stat Breakdown */
.stat-breakdown {
  display: flex;
  gap: 0.75rem;
  margin-top: 0.25rem;
  font-size: 0.75rem;
  color: #9ca3af;
}

/* Large Stat Icon */
.stat-icon-large {
  width: 68px;
  height: 68px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.75rem;
  flex-shrink: 0;
  position: relative;
}

.stat-icon-large.probes {
  background: linear-gradient(135deg, #fef3c7, #fde68a);
  color: #d97706;
}

.stat-icon-large.uptime {
  background: linear-gradient(135deg, #ede9fe, #ddd6fe);
  color: #7c3aed;
}

.stat-badge {
  position: absolute;
  bottom: -4px;
  right: -4px;
  background: #10b981;
  color: white;
  font-size: 0.625rem;
  padding: 0.125rem 0.375rem;
  border-radius: 999px;
  font-weight: 600;
}

.uptime-value {
  font-size: 1.25rem;
}

/* Copy Button */
.copy-btn {
  background: none;
  border: none;
  padding: 0.25rem 0.5rem;
  cursor: pointer;
  color: #9ca3af;
  border-radius: 4px;
  transition: all 0.2s;
}

.copy-btn:hover {
  background: #f3f4f6;
  color: #3b82f6;
}

.copy-btn.copied {
  color: #10b981;
}

/* Info Value with Copy */
.info-value-with-copy {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  justify-content: flex-end;
}

.ip-value {
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.875rem;
}

/* IP Chips */
.ip-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  justify-content: flex-end;
}

.ip-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.25rem 0.625rem;
  background: var(--ip-chip-bg, #f3f4f6);
  color: var(--ip-chip-color, #374151);
  border-radius: 999px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.2s;
}

.ip-chip:hover {
  background: #e5e7eb;
}

.ip-chip.copied {
  background: #d1fae5;
  color: #059669;
}

.ip-chip i {
  font-size: 0.625rem;
  opacity: 0.5;
}

.ip-chip.copied i {
  opacity: 1;
}

/* Connection Status */
.connection-status {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.75rem;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  animation: pulse-dot 2s ease-in-out infinite;
}

.status-dot.online {
  background: #10b981;
  box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.2);
}

.status-dot.offline {
  background: #ef4444;
  animation: none;
}

.status-text {
  color: #6b7280;
  font-weight: 500;
}

@keyframes pulse-dot {
  0%, 100% { transform: scale(1); opacity: 1; }
  50% { transform: scale(1.1); opacity: 0.8; }
}

/* Refresh Indicator */
.refresh-indicator {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  font-size: 0.75rem;
  color: #9ca3af;
}

/* Enhanced Resource Meter */
.resource-meter.enhanced {
  padding: 0.75rem;
  background: #f9fafb;
  border-radius: 8px;
  margin-bottom: 0.75rem;
}

.resource-meter.enhanced .resource-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}

.resource-label {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 500;
  color: #374151;
}

.resource-label i {
  font-size: 0.875rem;
  color: #6b7280;
}

.resource-value {
  font-weight: 600;
  font-size: 1rem;
}

.status-healthy { color: #10b981; }
.status-warning { color: #f59e0b; }
.status-critical { color: #ef4444; }

/* Gradient Progress Bar */
.progress-bar-container {
  margin-bottom: 0.5rem;
}

.progress.gradient {
  height: 8px;
  border-radius: 4px;
  background: #e5e7eb;
  overflow: hidden;
}

.progress.gradient .progress-bar {
  height: 100%;
  border-radius: 4px;
  transition: width 0.5s ease, background 0.3s ease;
}

.progress.gradient .progress-bar.healthy {
  background: linear-gradient(90deg, #10b981, #34d399);
}

.progress.gradient .progress-bar.warning {
  background: linear-gradient(90deg, #f59e0b, #fbbf24);
}

.progress.gradient .progress-bar.critical {
  background: linear-gradient(90deg, #ef4444, #f87171);
}

/* Resource Breakdown */
.resource-breakdown {
  display: flex;
  gap: 1rem;
  font-size: 0.75rem;
}

.breakdown-item {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.breakdown-label {
  color: #9ca3af;
}

.breakdown-value {
  color: #374151;
  font-weight: 500;
}

/* OS Info Styling */
.os-value {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  justify-content: flex-end;
}

.os-icon {
  font-size: 1.125rem;
  color: #6b7280;
}

.arch-badge {
  display: inline-block;
  padding: 0.125rem 0.5rem;
  background: #f3f4f6;
  border-radius: 4px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.75rem;
}

/* Environment Badge */
.env-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.25rem 0.625rem;
  border-radius: 999px;
  font-size: 0.75rem;
  font-weight: 500;
}

.env-badge.physical {
  background: #d1fae5;
  color: #059669;
}

.env-badge.virtual {
  background: #dbeafe;
  color: #2563eb;
}

/* Location Link */
.location-link {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  color: #3b82f6;
  text-decoration: none;
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.75rem;
}

.location-link:hover {
  text-decoration: underline;
}

/* Footer Subtle */
.card-footer.subtle {
  background: #f9fafb;
  border-top: 1px solid #e5e7eb;
  padding: 0.75rem 1rem;
}

.footer-info {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.75rem;
  color: #9ca3af;
}

/* Interface Cards */
.interfaces-content {
  padding: 0.75rem;
}

.interfaces-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.interface-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem;
  background: #f9fafb;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s;
}

.interface-item:hover {
  background: #f3f4f6;
}

.interface-item.copied {
  background: #d1fae5;
}

.interface-icon {
  width: 40px;
  height: 40px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.125rem;
  flex-shrink: 0;
}

.interface-icon.ethernet {
  background: #dbeafe;
  color: #3b82f6;
}

.interface-icon.wifi {
  background: #d1fae5;
  color: #10b981;
}

.interface-icon.loopback {
  background: #f3f4f6;
  color: #6b7280;
}

.interface-icon.virtual {
  background: #fef3c7;
  color: #d97706;
}

.interface-icon.vpn {
  background: #ede9fe;
  color: #7c3aed;
}

.interface-icon.other {
  background: #f3f4f6;
  color: #6b7280;
}

.interface-info {
  flex: 1;
  min-width: 0;
}

.interface-name {
  font-weight: 600;
  color: #1f2937;
  font-size: 0.875rem;
}

.interface-mac {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.125rem;
}

.interface-mac code {
  font-size: 0.75rem;
  color: #6b7280;
  background: none;
  padding: 0;
}

.vendor-name {
  font-size: 0.75rem;
  color: #9ca3af;
}

.copy-indicator {
  color: #9ca3af;
  font-size: 0.875rem;
}

.interface-item.copied .copy-indicator {
  color: #10b981;
}

.empty-interfaces {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 2rem;
  color: #9ca3af;
}

.empty-interfaces i {
  font-size: 1.5rem;
}

/* Enhanced Interface Display */
.p11-interfaces .interface-item.p11 {
  padding: 0.875rem;
  border-radius: 0.5rem;
  background: rgba(255, 255, 255, 0.5);
  border: 1px solid rgba(0, 0, 0, 0.05);
}

.interface-item.p11.is-default {
  background: linear-gradient(135deg, rgba(59, 130, 246, 0.1), rgba(139, 92, 246, 0.1));
  border-color: rgba(59, 130, 246, 0.3);
}

.interface-details {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.interface-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.interface-ips {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
  font-size: 0.75rem;
  color: #6b7280;
}

.ip-badge {
  background: rgba(59, 130, 246, 0.1);
  color: #3b82f6;
  padding: 0.125rem 0.375rem;
  border-radius: 0.25rem;
  font-family: monospace;
}

.interface-gateway {
  font-size: 0.75rem;
  color: #6b7280;
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

/* Routing Table Styles */
.routes-content {
  padding: 0;
  overflow-x: auto;
}

.routes-table {
  width: 100%;
  min-width: 400px;
  font-size: 0.8125rem;
  border-collapse: collapse;
}

.routes-table th,
.routes-table td {
  padding: 0.5rem 0.75rem;
  text-align: left;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
}

.routes-table th {
  background: rgba(0, 0, 0, 0.02);
  font-weight: 600;
  color: #6b7280;
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.routes-table tr.default-route {
  background: rgba(59, 130, 246, 0.08);
}

.routes-table code {
  background: rgba(0, 0, 0, 0.03);
  padding: 0.125rem 0.25rem;
  border-radius: 0.25rem;
  font-size: 0.75rem;
}

.routes-more {
  padding: 0.5rem 0.75rem;
  text-align: center;
  color: #9ca3af;
  font-size: 0.75rem;
  background: rgba(0, 0, 0, 0.02);
}

/* Light Mode Improvements for Interface Display */
.p11-interfaces .interface-item.p11 {
  padding: 0.875rem;
  border-radius: 0.5rem;
  background: rgba(0, 0, 0, 0.02);
  border: 1px solid rgba(0, 0, 0, 0.08);
}

.interface-item.p11.is-default {
  background: linear-gradient(135deg, rgba(59, 130, 246, 0.08), rgba(139, 92, 246, 0.06));
  border-color: rgba(59, 130, 246, 0.25);
}

.interface-name {
  font-weight: 600;
  color: #1f2937;
}

.interface-mac code {
  color: #374151;
  background: rgba(0, 0, 0, 0.04);
  padding: 0.125rem 0.25rem;
  border-radius: 0.25rem;
}

.vendor-name {
  color: #6b7280;
}

.ip-badge {
  background: rgba(59, 130, 246, 0.12);
  color: #1d4ed8;
  padding: 0.125rem 0.5rem;
  border-radius: 0.25rem;
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.75rem;
}

.interface-gateway {
  font-size: 0.75rem;
  color: #6b7280;
}

.routes-table th {
  background: rgba(0, 0, 0, 0.03);
  color: #4b5563;
}

.routes-table td {
  color: #374151;
}

.routes-table code {
  background: rgba(0, 0, 0, 0.04);
  color: #1d4ed8;
}

/* Enhanced card styling */
.info-card.enhanced {
  overflow: hidden;
}

.info-card.enhanced .card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.info-label i {
  margin-right: 0.5rem;
  color: #9ca3af;
  font-size: 0.875rem;
}

/* ========================================
   DARK MODE STYLES
   ======================================== */


/* Stat Items */
:global([data-theme="dark"]) .stat-item {
  background: #1e293b;
  border-color: #374151;
}

:global([data-theme="dark"]) .stat-item:hover:not(.loading) {
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.3);
}

:global([data-theme="dark"]) .stat-value {
  color: #f9fafb !important;
}

:global([data-theme="dark"]) .stat-label {
  color: #9ca3af !important;
}

/* Stat icons - darker backgrounds */
:global([data-theme="dark"]) .stat-icon.cpu {
  background: #1e3a5f;
  color: #60a5fa;
}

:global([data-theme="dark"]) .stat-icon.memory {
  background: #064e3b;
  color: #34d399;
}

:global([data-theme="dark"]) .stat-icon.network {
  background: #78350f;
  color: #fbbf24;
}

:global([data-theme="dark"]) .stat-icon.uptime {
  background: #4c1d95;
  color: #a78bfa;
}

/* Content Sections */
:global([data-theme="dark"]) .content-section {
  background: #1e293b;
  border-color: #374151;
}

:global([data-theme="dark"]) .section-header {
  border-bottom-color: #374151;
}

:global([data-theme="dark"]) .section-title {
  color: #f9fafb !important;
}

:global([data-theme="dark"]) .section-title i {
  color: #9ca3af !important;
}

/* Probe Cards */
:global([data-theme="dark"]) .probe-card {
  background: #1e293b;
  border-color: #374151;
}

:global([data-theme="dark"]) .probe-card:hover:not(.skeleton) {
  border-color: #3b82f6;
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.3);
}

:global([data-theme="dark"]) .probe-card.has-issues {
  border-color: #991b1b;
  background: #1f1515;
}

:global([data-theme="dark"]) .probe-icon {
  background: #1e3a5f;
  color: #60a5fa;
}

:global([data-theme="dark"]) .probe-title {
  color: #f9fafb !important;
}

:global([data-theme="dark"]) .probe-type-badge {
  background: #374151;
  color: #d1d5db;
}

:global([data-theme="dark"]) .probe-type-badge.inactive {
  background: #7f1d1d;
  color: #fca5a5;
}

:global([data-theme="dark"]) .probe-stat {
  color: #9ca3af;
}

:global([data-theme="dark"]) .probe-arrow {
  color: #6b7280;
}

/* Info Cards */
:global([data-theme="dark"]) .info-card {
  background: #1e293b !important;
  border-color: #374151 !important;
}

:global([data-theme="dark"]) .card-header {
  background: #0f172a !important;
  border-bottom-color: #374151 !important;
}

:global([data-theme="dark"]) .card-title {
  color: #f9fafb !important;
}

:global([data-theme="dark"]) .card-title i {
  color: #9ca3af !important;
}

:global([data-theme="dark"]) .card-content {
  background: #1e293b;
}

:global([data-theme="dark"]) .card-footer {
  background: #0f172a;
  border-top-color: #374151;
}

/* Info Rows */
:global([data-theme="dark"]) .info-label {
  color: #9ca3af !important;
}

:global([data-theme="dark"]) .info-value {
  color: #e5e7eb !important;
}

/* Resource Meters */
:global([data-theme="dark"]) .resource-header {
  color: #e5e7eb;
}

:global([data-theme="dark"]) .resource-details {
  color: #9ca3af;
}

:global([data-theme="dark"]) .progress {
  background: #374151;
}

/* MAC list and details */
:global([data-theme="dark"]) .mac-item {
  background: #0f172a;
  border-color: #374151;
}

:global([data-theme="dark"]) .mac-address {
  color: #f9fafb;
}

:global([data-theme="dark"]) .mac-vendor {
  color: #9ca3af;
}

:global([data-theme="dark"]) .memory-details {
  background: #0f172a;
}

:global([data-theme="dark"]) .detail-row {
  color: #e5e7eb;
  border-bottom-color: #374151;
}

/* Skeleton loading */
:global([data-theme="dark"]) .skeleton-text {
  background: #374151;
}

:global([data-theme="dark"]) .skeleton-text::after {
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.1), transparent);
}

:global([data-theme="dark"]) .skeleton-box {
  background: #374151;
}

/* Empty state */
:global([data-theme="dark"]) .empty-state {
  color: #9ca3af;
}

:global([data-theme="dark"]) .empty-state h5 {
  color: #e5e7eb !important;
}

/* Status badges */
:global([data-theme="dark"]) .status-badge.online {
  background: #064e3b;
  color: #22c55e;
}

:global([data-theme="dark"]) .status-badge.offline {
  background: #7f1d1d;
  color: #ef4444;
}

:global([data-theme="dark"]) .status-badge.loading {
  background: #374151;
  color: #9ca3af;
}

:global([data-theme="dark"]) .status-badge.live {
  background: #064e3b;
  color: #34d399;
}

/* Horizontal rules */
:global([data-theme="dark"]) hr {
  border-color: #374151;
}

/* Enhanced Components Dark Mode */
:global([data-theme="dark"]) .stat-item.glass {
  background: rgba(30, 41, 59, 0.9);
  border-color: #374151;
}

:global([data-theme="dark"]) .progress-ring-bg {
  stroke: #374151;
}

:global([data-theme="dark"]) .ring-icon {
  color: #9ca3af;
}

:global([data-theme="dark"]) .stat-breakdown {
  color: #6b7280;
}

:global([data-theme="dark"]) .stat-icon-large.probes {
  background: linear-gradient(135deg, #78350f, #92400e);
}

:global([data-theme="dark"]) .stat-icon-large.uptime {
  background: linear-gradient(135deg, #4c1d95, #5b21b6);
}

:global([data-theme="dark"]) .copy-btn:hover {
  background: #374151;
}

:global([data-theme="dark"]) .ip-chip {
  --ip-chip-bg: #374151;
  --ip-chip-color: #e5e7eb;
  background: #374151 !important;
  color: #e5e7eb !important;
}

:global([data-theme="dark"]) .ip-chip:hover {
  background: #4b5563 !important;
}

:global([data-theme="dark"]) .ip-chip.copied {
  background: #064e3b !important;
  color: #34d399 !important;
}

:global([data-theme="dark"]) .resource-meter.enhanced {
  background: #0f172a;
}

:global([data-theme="dark"]) .resource-label {
  color: #e5e7eb;
}

:global([data-theme="dark"]) .progress.gradient {
  background: #374151;
}

:global([data-theme="dark"]) .breakdown-value {
  color: #e5e7eb;
}

:global([data-theme="dark"]) .arch-badge {
  background: #374151;
  color: #e5e7eb;
}

:global([data-theme="dark"]) .env-badge.physical {
  background: #064e3b;
  color: #34d399;
}

:global([data-theme="dark"]) .env-badge.virtual {
  background: #1e3a5f;
  color: #60a5fa;
}

:global([data-theme="dark"]) .card-footer.subtle {
  background: #0f172a;
  border-top-color: #374151;
}

:global([data-theme="dark"]) .interface-item {
  background: #0f172a;
}

:global([data-theme="dark"]) .interface-item:hover {
  background: #1e293b;
}

:global([data-theme="dark"]) .interface-item.copied {
  background: #064e3b;
}

:global([data-theme="dark"]) .interface-icon.ethernet {
  background: #1e3a5f;
  color: #60a5fa;
}

:global([data-theme="dark"]) .interface-icon.wifi {
  background: #064e3b;
  color: #34d399;
}

:global([data-theme="dark"]) .interface-icon.loopback {
  background: #374151;
  color: #9ca3af;
}

:global([data-theme="dark"]) .interface-icon.virtual {
  background: #78350f;
  color: #fbbf24;
}

:global([data-theme="dark"]) .interface-icon.vpn {
  background: #4c1d95;
  color: #a78bfa;
}

:global([data-theme="dark"]) .interface-icon.other {
  background: #374151;
  color: #9ca3af;
}

:global([data-theme="dark"]) .interface-name {
  color: #f9fafb;
}

:global([data-theme="dark"]) .interface-mac code {
  color: #9ca3af;
}

:global([data-theme="dark"]) .empty-interfaces {
  color: #6b7280;
}

:global([data-theme="dark"]) .info-label i {
  color: #6b7280;
}

:global([data-theme="dark"]) .status-dot.online {
  box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.3);
}
</style>

<!-- Unscoped dark mode overrides for agent view cards -->
<style>
[data-theme="dark"] .resource-meter.enhanced {
  background: #1e293b !important;
}

[data-theme="dark"] .resource-label {
  color: #e5e7eb !important;
}

[data-theme="dark"] .progress.gradient {
  background: #374151 !important;
}

[data-theme="dark"] .resource-breakdown {
  background: transparent !important;
}

[data-theme="dark"] .breakdown-label {
  color: #9ca3af !important;
}

[data-theme="dark"] .breakdown-value {
  color: #e5e7eb !important;
}

[data-theme="dark"] .resource-value {
  color: #e5e7eb !important;
}

[data-theme="dark"] .memory-details {
  background: #1e293b !important;
}

[data-theme="dark"] .memory-details .detail-row {
  color: #e5e7eb !important;
  border-color: #374151 !important;
}

[data-theme="dark"] .interface-item {
  background: #1e293b !important;
}

[data-theme="dark"] .interface-item:hover {
  background: #334155 !important;
}

/* Architecture and Environment badges */
[data-theme="dark"] .arch-badge {
  background: #374151 !important;
  color: #e5e7eb !important;
}

[data-theme="dark"] .env-badge {
  background: #374151 !important;
  color: #e5e7eb !important;
}

[data-theme="dark"] .env-badge.physical {
  background: #064e3b !important;
  color: #34d399 !important;
}

[data-theme="dark"] .env-badge.virtual {
  background: #1e3a5f !important;
  color: #60a5fa !important;
}

/* OS icon and value */
[data-theme="dark"] .os-icon {
  color: #60a5fa !important;
}

[data-theme="dark"] .os-value {
  color: #e5e7eb !important;
}

[data-theme="dark"] .os-value small {
  color: #9ca3af !important;
}

/* Dark Mode Overrides for Interface Display */
[data-theme="dark"] .p11-interfaces .interface-item.p11 {
  background: rgba(255, 255, 255, 0.05) !important;
  border-color: rgba(255, 255, 255, 0.1) !important;
}

[data-theme="dark"] .interface-item.p11.is-default {
  background: linear-gradient(135deg, rgba(59, 130, 246, 0.2), rgba(139, 92, 246, 0.15)) !important;
  border-color: rgba(59, 130, 246, 0.4) !important;
}

[data-theme="dark"] .interface-name {
  color: #f3f4f6 !important;
}

[data-theme="dark"] .interface-mac code {
  color: #d1d5db !important;
  background: rgba(255, 255, 255, 0.1) !important;
}

[data-theme="dark"] .vendor-name {
  color: #9ca3af !important;
}

[data-theme="dark"] .interface-ips {
  color: #9ca3af !important;
}

[data-theme="dark"] .ip-badge {
  background: rgba(59, 130, 246, 0.25) !important;
  color: #93c5fd !important;
}

[data-theme="dark"] .interface-gateway {
  color: #9ca3af !important;
}

[data-theme="dark"] .routes-table th {
  background: rgba(255, 255, 255, 0.05) !important;
  color: #9ca3af !important;
  border-bottom-color: rgba(255, 255, 255, 0.1) !important;
}

[data-theme="dark"] .routes-table td {
  color: #e5e7eb !important;
  border-bottom-color: rgba(255, 255, 255, 0.05) !important;
}

[data-theme="dark"] .routes-table tr.default-route {
  background: rgba(59, 130, 246, 0.15) !important;
}

[data-theme="dark"] .routes-table code {
  background: rgba(255, 255, 255, 0.1) !important;
  color: #93c5fd !important;
}

[data-theme="dark"] .routes-more {
  color: #6b7280 !important;
  background: rgba(255, 255, 255, 0.02) !important;
}

/* Agent Tabs */
.agent-tabs { display: flex; gap: 0.5rem; margin-bottom: 1.5rem; padding: 0.25rem; background: var(--bs-secondary-bg); border-radius: 12px; border: 1px solid var(--bs-border-color); }
.tab-btn { flex: 1; display: flex; align-items: center; justify-content: center; gap: 0.5rem; padding: 0.75rem 1rem; border: none; border-radius: 10px; background: transparent; color: var(--bs-secondary-color); font-weight: 500; font-size: 0.9rem; cursor: pointer; transition: all 0.2s; white-space: nowrap; }
.tab-btn:hover { color: var(--bs-body-color); background: var(--bs-tertiary-bg); }
.tab-btn.active { background: var(--bs-body-bg); color: var(--bs-body-color); box-shadow: 0 2px 8px rgba(var(--bs-dark-rgb), 0.1); font-weight: 600; }
.tab-badge { padding: 0.125rem 0.5rem; background: var(--bs-primary); color: var(--bs-white); border-radius: 999px; font-size: 0.7rem; font-weight: 600; margin-left: 0.25rem; }
.tab-panel { animation: fade-in 0.2s ease-out; }
@keyframes fade-in { from { opacity: 0; transform: translateY(4px); } to { opacity: 1; transform: translateY(0); } }

/* Overview Stats */
.overview-stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 1rem; margin-bottom: 1.5rem; }
.stat-card { display: flex; align-items: center; gap: 0.75rem; padding: 1rem; background: var(--bs-body-bg); border: 1px solid var(--bs-border-color); border-radius: 12px; transition: all 0.2s; }
.stat-card:hover { transform: translateY(-2px); box-shadow: 0 4px 12px rgba(0,0,0,0.08); }
.stat-card.online { border-left: 3px solid var(--bs-success); }
.stat-card.offline { border-left: 3px solid var(--bs-danger); }
.stat-icon { width: 40px; height: 40px; display: flex; align-items: center; justify-content: center; background: var(--bs-secondary-bg); border-radius: 10px; color: var(--bs-primary); font-size: 1.25rem; }
.stat-card.online .stat-icon { background: rgba(var(--bs-success-rgb), 0.1); color: var(--bs-success); }
.stat-card.offline .stat-icon { background: rgba(var(--bs-danger-rgb), 0.1); color: var(--bs-danger); }
.stat-info { display: flex; flex-direction: column; min-width: 0; }
.stat-value { font-weight: 600; font-size: 0.95rem; color: var(--bs-body-color); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.stat-label { font-size: 0.75rem; color: var(--bs-secondary-color); text-transform: uppercase; letter-spacing: 0.5px; }

@media (max-width: 640px) { .agent-tabs { flex-wrap: wrap; } .tab-btn { flex: 1 1 calc(50% - 0.25rem); padding: 0.625rem 0.5rem; font-size: 0.8rem; } .overview-stats { grid-template-columns: repeat(2, 1fr); } }
</style>