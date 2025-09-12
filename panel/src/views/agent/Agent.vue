<script lang="ts" setup>
import {onMounted, reactive, computed, ref} from "vue";
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
  PingResult
} from "@/types";
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
import {AgentService, ProbeService, WorkspaceService, ProbeDataService} from "@/services/apiService";
import {groupProbesByTarget, type TargetGroupKind, type ProbeGroupByTarget} from "@/utils/probeGrouping";

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

// Error states
const errors = reactive({
  agent: null as string | null,
  workspace: null as string | null,
  probes: null as string | null,
  systemInfo: null as string | null,
  networkInfo: null as string | null
})

// Computed properties for better organization
const isOnline = computed(() => {
  if (!state.agent.updatedAt || loadingState.agent) return false;
  const lastSeen = new Date(state.agent.updatedAt);
  const now = new Date();
  const diffMinutes = (now.getTime() - lastSeen.getTime()) / 60000;
  return diffMinutes <= 5; // Consider online if seen in last 5 minutes
});

const cpuUsagePercent = computed(() => {
  if (!hasSystemData.value || !state.systemData?.cpu) return 0;
  return ((state.systemData.cpu.user + state.systemData.cpu.system) * 100).toFixed(1);
});

const memoryUsagePercent = computed(() => {
  if (!hasSystemData.value || !state.systemData?.ram) return 0;
  return (state.systemData.ram.used * 100).toFixed(1);
});

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
  let cpuCapacity: number = (info.CPUTimes?.idle || 0) + info.CPUTimes.system + info.CPUTimes.user;
  let ramCapacity: number = info.memoryInfo.total_bytes;
  let virtualCapacity: number = info.memoryInfo.virtual_total_bytes;
  return {
    cpu: {
      idle: roundTo((info.CPUTimes?.idle || 0) / cpuCapacity),
      system: roundTo((info.CPUTimes?.system || 0) / cpuCapacity),
      user: roundTo((info.CPUTimes?.user || 0) / cpuCapacity),
    },
    ram: {
      used: roundTo(info.memoryInfo.used_bytes / ramCapacity),
      free: roundTo(info.memoryInfo.available_bytes / ramCapacity),
      total: roundTo(info.memoryInfo.total_bytes / ramCapacity),
    },
    virtual: {
      used: roundTo(info.memoryInfo.virtual_used_bytes / ramCapacity),
      free: roundTo(info.memoryInfo.virtual_free_bytes / virtualCapacity),
      total: roundTo(info.memoryInfo.virtual_total_bytes / virtualCapacity),
    }
  } as SystemData
}

function getVendorFromMac(macAddress: string) {
  const normalizedMac = macAddress.replace(/[:-]/g, '').toUpperCase();
  const oui = normalizedMac.substring(0, 6);
  const entry = state.ouiList.find(item => item.Assignment == oui);
  return entry ? (entry as OUIEntry)["Organization Name"] : "Unknown Vendor";
}

function bytesToString(bytes: number, si: boolean = true, dp: number = 2): string {
  const thresh = si ? 1000 : 1024;

  if (Math.abs(bytes) < thresh) {
    return bytes + ' B';
  }

  const units = si
      ? ['kB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB']
      : ['KiB', 'MiB', 'GiB', 'TiB', 'PiB', 'EiB', 'ZiB', 'YiB'];
  let u = -1;
  const r = 10 ** dp;

  do {
    bytes /= thresh;
    ++u;
  } while (Math.round(Math.abs(bytes) * r) / r >= thresh && u < units.length - 1);

  return bytes.toFixed(dp) + ' ' + units[u];
}

function getLocalAddresses(addresses: string[]): string[] {
  let ipv4s = addresses.filter(f => f.split(".").length == 4)
  let nonLocal = ipv4s.filter(i => !i.includes("127.0.0.1"))
  return nonLocal.map(l => l.split('/')[0])
}

function formatSnakeCaseToHumanCase(name: string): string {
  let words = name.split("_")
  words = words.filter(w => w != "bytes")
  words = words.map(w => w[0].toUpperCase() + w.substring(1))
  return words.join(" ")
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
  const pingProbes = probes.filter(p => p.type === 'PING' && p.enabled);
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
  // Check if this group has any ping probes
  const hasPingProbes = group.probes.some(p => p.type === 'PING');

  if (!hasPingProbes) {
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
      return 'fa-check-circle';
    case 'warning':
      return 'fa-exclamation-triangle';
    case 'critical':
      return 'fa-times-circle';
    default:
      return 'fa-question-circle';
  }
}

const router = core.router()

let state = reactive({
  workspace: {} as Workspace,
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
  networkInfo: {} as NetInfoPayload,
  systemInfo: {} as SysInfoPayload,
  systemData: {} as SystemData,
  hasData: false,
  ouiList: [] as OUIEntry[]
})

onMounted(async () => {
  let agentID = router.currentRoute.value.params["aID"] as string
  let workspaceID = router.currentRoute.value.params["wID"] as string
  if (!agentID || !workspaceID) return

  // Load OUI list early (non-blocking)
  fetch('/ouiList.json')
      .then(response => response.json())
      .then(data => state.ouiList = data as OUIEntry[])
      .catch(err => console.error('Failed to load OUI list:', err));

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
        state.systemInfo = pD.payload as SysInfoPayload
        state.systemData = updateSystemData(state.systemInfo)
        state.hasData = true
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

        const grouped = groupProbesByTarget(pL, { excludeDefaults: true });

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

        // Fetch ping statistics if there are ping probes
        const hasPingProbes = pL.some(p => p.type === 'PING');
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
    <Title
        :history="[
        {title: 'workspaces', link: '/workspaces'},
        {title: state.workspace.name || 'Loading...', link: `/workspace/${state.workspace.id || ''}`}
      ]"
        :title="state.agent.name || 'Loading...'"
        :subtitle="state.agent.location || 'Agent Information'">
      <div class="d-flex flex-wrap gap-2">
        <div class="status-badge" :class="isInitializing ? 'loading' : (isOnline ? 'online' : 'offline')">
          <i :class="isInitializing ? 'fa-solid fa-spinner fa-spin' : 'fa-solid fa-circle'"></i>
          {{ isInitializing ? 'Loading...' : (isOnline ? 'Online' : 'Offline') }}
        </div>
        <router-link
            v-if="state.agent.id && state.workspace.id"
            :to="`/workspace/${state.agent.workspace_id}/agent/${state.agent.id}/probes`"
            class="btn btn-outline-primary">
          <i class="fa-regular fa-pen-to-square"></i>
          <span class="d-none d-sm-inline">&nbsp;Edit Probes</span>
        </router-link>
        <router-link
            v-if="state.agent.id && state.workspace.id"
            :to="`/workspace/${state.agent.workspace_id}/agent/${state.agent.id}/probe/new`"
            class="btn btn-primary">
          <i class="fa-solid fa-plus"></i>&nbsp;Add Probe
        </router-link>
      </div>
    </Title>

    <!-- Quick Stats Bar - Always visible with loading state -->
    <div class="quick-stats">
      <div class="stat-item" :class="{'loading': loadingState.systemInfo}">
        <div class="stat-icon cpu">
          <i class="fa-solid fa-microchip"></i>
        </div>
        <div class="stat-content">
          <div class="stat-value">
            <span v-if="loadingState.systemInfo" class="skeleton-text">--</span>
            <span v-else>{{ cpuUsagePercent }}%</span>
          </div>
          <div class="stat-label">CPU Usage</div>
        </div>
      </div>
      <div class="stat-item" :class="{'loading': loadingState.systemInfo}">
        <div class="stat-icon memory">
          <i class="fa-solid fa-memory"></i>
        </div>
        <div class="stat-content">
          <div class="stat-value">
            <span v-if="loadingState.systemInfo" class="skeleton-text">--</span>
            <span v-else>{{ memoryUsagePercent }}%</span>
          </div>
          <div class="stat-label">Memory Usage</div>
        </div>
      </div>
      <div class="stat-item" :class="{'loading': loadingState.probes}">
        <div class="stat-icon network">
          <i class="fa-solid fa-network-wired"></i>
        </div>
        <div class="stat-content">
          <div class="stat-value">
            <span v-if="loadingState.probes" class="skeleton-text">-</span>
            <span v-else>{{ activeProbesCount }}</span>
          </div>
          <div class="stat-label">Active Probes</div>
        </div>
      </div>
      <div class="stat-item" :class="{'loading': loadingState.systemInfo}">
        <div class="stat-icon uptime">
          <i class="fa-solid fa-clock"></i>
        </div>
        <div class="stat-content">
          <div class="stat-value">
            <span v-if="loadingState.systemInfo" class="skeleton-text">--</span>
            <span v-else>{{ hasSystemData ? since(state.systemInfo.hostInfo?.boot_time + "", false) : 'N/A' }}</span>
          </div>
          <div class="stat-label">Uptime</div>
        </div>
      </div>
    </div>

    <!-- Error Messages -->
    <div v-if="Object.values(errors).some(e => e !== null)" class="alert alert-warning mt-3">
      <i class="fa-solid fa-exclamation-triangle"></i>
      <strong>Some data could not be loaded:</strong>
      <ul class="mb-0 mt-2">
        <li v-for="(error, key) in errors" v-if="error" :key="key">{{ error }}</li>
      </ul>
    </div>

    <!-- Main Content -->
    <div v-if="!state.agent.initialized && !loadingState.agent" class="empty-state">
      <i class="fa-solid fa-exclamation-triangle text-warning"></i>
      <h5>Agent Not Initialized</h5>
      <p>This agent needs to be initialized before it can be used.</p>
    </div>

    <div v-else class="agent-content">
      <!-- Probes Section -->
      <div class="content-section probes-section">
        <div class="section-header">
          <h5 class="section-title">
            <i class="fa-solid fa-diagram-project"></i>
            Monitoring Probes
          </h5>
          <span class="badge bg-primary" v-if="!loadingState.probes">
            {{ activeProbesCount }}/{{ totalProbesCount }} Active ({{ probeStats.percentage }}%)
          </span>
          <span class="badge bg-secondary" v-else>
            <i class="fa-solid fa-spinner fa-spin"></i> Loading
          </span>
        </div>

        <div v-if="loadingState.probes" class="probes-grid">
          <!-- Loading skeleton probes -->
          <div v-for="i in 3" :key="`skeleton-${i}`" class="probe-card skeleton">
            <div class="probe-link">
              <div class="probe-icon skeleton-box"></div>
              <div class="probe-content">
                <div class="skeleton-text probe-title-skeleton"></div>
                <div class="probe-types">
                  <span class="skeleton-text probe-type-skeleton"></span>
                  <span class="skeleton-text probe-type-skeleton"></span>
                </div>
                <div class="probe-stats">
                  <div class="skeleton-text probe-stat-skeleton"></div>
                  <div class="skeleton-text probe-stat-skeleton"></div>
                </div>
              </div>
              <i class="fa-solid fa-chevron-right probe-arrow"></i>
            </div>
          </div>
        </div>

        <div v-else-if="state.targetGroups.length > 0" class="probes-grid">
          <div v-for="g in state.targetGroups" :key="g.key" class="probe-card" :class="{'has-issues': state.groupStats[g.key]?.status === 'critical'}">
            <router-link :to="`/workspace/${state.workspace.id}/agent/${state.agent.id}/probe/${g.probes[0]?.id || ''}`" class="probe-link">
              <div class="probe-header">
                <div class="probe-icon">
                  <i :class="g.kind === 'agent' ? 'fa-solid fa-robot'
                  : g.kind === 'host' ? 'fa-solid fa-diagram-project'
                  : 'fa-solid fa-microchip'"></i>
                </div>
                <div class="probe-status">
                  <i :class="`fa-solid ${getStatusIcon(state.groupStats[g.key]?.status)} ${getStatusColor(state.groupStats[g.key]?.status)}`"></i>
                </div>
              </div>

              <div class="probe-content">
                <h6 class="probe-title">
                  <span v-if="g.kind==='host'">{{ g.label }}</span>
                  <span v-else-if="g.kind==='agent'">Agent {{ g.id }}</span>
                  <span v-else>Local on Agent {{ g.id }}</span>
                </h6>

                <div class="probe-types">
                  <span v-for="t in g.types" :key="t" class="probe-type-badge">
                    {{ t }} ({{ g.perType[t].count }})
                  </span>
                  <span class="probe-type-badge" :class="{'inactive': g.countEnabled === 0}">
                    {{ g.countEnabled }}/{{ g.countProbes }} enabled
                  </span>
                </div>

                <div class="probe-stats" v-if="state.groupStats[g.key]">
                  <div v-if="state.groupStats[g.key].isLoading" class="probe-stat">
                    <i class="fa-solid fa-spinner fa-spin"></i>
                    <span>Loading stats...</span>
                  </div>
                  <template v-else-if="state.groupStats[g.key].hasData">
                    <div class="probe-stat" v-if="state.groupStats[g.key].successRate !== undefined">
                      <i class="fa-solid fa-chart-line"></i>
                      <span>{{ state.groupStats[g.key].successRate.toFixed(1) }}% success</span>
                    </div>
                    <div class="probe-stat" v-if="state.groupStats[g.key].avgResponseTime !== undefined">
                      <i class="fa-solid fa-stopwatch"></i>
                      <span>{{ state.groupStats[g.key].avgResponseTime.toFixed(0) }}ms avg</span>
                    </div>
                    <div class="probe-stat" v-if="state.groupStats[g.key].lastRun">
                      <i class="fa-regular fa-clock"></i>
                      <span>{{ since(state.groupStats[g.key].lastRun, true) }}</span>
                    </div>
                  </template>
                  <div v-else class="probe-stat text-muted">
                    <i class="fa-solid fa-info-circle"></i>
                    <span>No ping data available</span>
                  </div>
                </div>
              </div>

              <i class="fa-solid fa-chevron-right probe-arrow"></i>
            </router-link>
          </div>
        </div>

        <div v-else-if="!loadingState.probes" class="empty-state">
          <i class="fa-solid fa-diagram-project"></i>
          <h5>No Probes Configured</h5>
          <p>Create your first probe to start monitoring</p>
          <router-link
              v-if="state.agent.id && state.workspace.id"
              :to="`/workspace/${state.workspace.id}/agent/${state.agent.id}/probe/new`"
              class="btn btn-primary">
            <i class="fa-solid fa-plus"></i> Create Probe
          </router-link>
        </div>
      </div>

      <!-- System Information Grid -->
      <div class="info-grid">
        <!-- Network Information -->
        <div class="info-card" :class="{'loading': loadingState.networkInfo}">
          <div class="card-header">
            <h5 class="card-title">
              <i class="fa-solid fa-network-wired"></i>
              Network Information
            </h5>
          </div>
          <div class="card-content">
            <div class="info-row" v-if="hasNetworkData">
              <span class="info-label">Last updated</span>
              <span class="info-value">
                <span>{{ since(state.networkInfo.timestamp, true) }}</span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">Hostname</span>
              <span class="info-value">
                <span v-if="loadingState.systemInfo" class="skeleton-text">--------------------</span>
                <span v-else>{{ state.systemInfo.hostInfo?.name || 'Unknown' }}</span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">Public IP</span>
              <span class="info-value">
                <span v-if="loadingState.networkInfo" class="skeleton-text">---------------</span>
                <span v-else>{{ state.networkInfo.public_address || 'Unknown' }}</span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">ISP</span>
              <span class="info-value">
                <span v-if="loadingState.networkInfo" class="skeleton-text">-------------------------</span>
                <span v-else>{{ state.networkInfo.internet_provider || 'Unknown' }}</span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">Gateway</span>
              <span class="info-value">
                <span v-if="loadingState.networkInfo" class="skeleton-text">---------------</span>
                <span v-else>{{ state.networkInfo.default_gateway || 'Unknown' }}</span>
              </span>
            </div>
            <div class="info-row expandable">
              <span class="info-label">Local IPs</span>
              <div class="info-value">
                <div v-if="loadingState.systemInfo" class="skeleton-text">---------------</div>
                <div v-else-if="hasSystemData && state.systemInfo.hostInfo?.ip">
                  <div v-for="ip in getLocalAddresses(state.systemInfo.hostInfo.ip)" :key="ip">
                    {{ ip }}
                  </div>
                </div>
                <div v-else>No IPs found</div>
              </div>
            </div>
          </div>
          <div class="card-footer" v-if="state.agent.id">
            <router-link :to="`/workspace/${state.workspace.id}/agent/${state.agent.id}/speedtests`" class="btn btn-sm btn-outline-secondary">
              <i class="fa-solid fa-gauge-high"></i> View Speedtests
            </router-link>
          </div>
        </div>

        <!-- System Resources -->
        <div class="info-card" :class="{'loading': loadingState.systemInfo}">
          <div class="card-header">
            <h5 class="card-title">
              <i class="fa-solid fa-server"></i>
              System Resources
            </h5>
          </div>
          <div class="card-content">
            <div class="info-row" v-if="hasSystemData">
              <span class="info-label">Last updated</span>
              <span class="info-value">
                <span>{{ since(state.systemInfo.timestamp + "", true) }}</span>
              </span>
            </div>
            <div class="resource-meter">
              <div class="resource-header">
                <span>CPU Usage</span>
                <span v-if="loadingState.systemInfo" class="skeleton-text">---%</span>
                <span v-else>{{ cpuUsagePercent }}%</span>
              </div>
              <div class="progress">
                <div class="progress-bar bg-primary" :style="{width: loadingState.systemInfo ? '0%' : cpuUsagePercent + '%'}"></div>
              </div>
              <div class="resource-details">
                <span v-if="loadingState.systemInfo" class="skeleton-text">User: ---%</span>
                <span v-else>User: {{ hasSystemData ? (state.systemData.cpu?.user * 100).toFixed(1) : '0' }}%</span>
                <span v-if="loadingState.systemInfo" class="skeleton-text">System: ---%</span>
                <span v-else>System: {{ hasSystemData ? (state.systemData.cpu?.system * 100).toFixed(1) : '0' }}%</span>
              </div>
            </div>

            <div class="resource-meter">
              <div class="resource-header">
                <span>Memory Usage</span>
                <span v-if="loadingState.systemInfo" class="skeleton-text">---%</span>
                <span v-else>{{ memoryUsagePercent }}%</span>
              </div>
              <div class="progress">
                <div class="progress-bar bg-success" :style="{width: loadingState.systemInfo ? '0%' : memoryUsagePercent + '%'}"></div>
              </div>
              <div class="resource-details">
                <span v-if="loadingState.systemInfo" class="skeleton-text">Used: --- GB</span>
                <span v-else>Used: {{ hasSystemData ? bytesToString(state.systemInfo.memoryInfo?.used_bytes || 0) : 'N/A' }}</span>
                <span v-if="loadingState.systemInfo" class="skeleton-text">Total: --- GB</span>
                <span v-else>Total: {{ hasSystemData ? bytesToString(state.systemInfo.memoryInfo?.total_bytes || 0) : 'N/A' }}</span>
              </div>
            </div>

            <ElementExpand title="Memory Details" code :disabled="loadingState.systemInfo || !hasSystemData">
              <template v-slot:expanded>
                <div class="memory-details">
                  <div v-if="loadingState.systemInfo" v-for="i in 4" :key="`mem-skeleton-${i}`" class="detail-row">
                    <span class="skeleton-text">--------------</span>
                    <span class="skeleton-text">--- GB</span>
                  </div>
                  <div v-else-if="hasSystemData && state.systemInfo.memoryInfo?.raw" v-for="(value, key) in state.systemInfo.memoryInfo.raw" :key="key" class="detail-row">
                    <span>{{ formatSnakeCaseToHumanCase(key) }}</span>
                    <span>{{ bytesToString(value) }}</span>
                  </div>
                  <div v-else class="text-muted">No memory details available</div>
                </div>
              </template>
            </ElementExpand>
          </div>
        </div>

        <!-- System Information -->
        <div class="info-card" :class="{'loading': loadingState.systemInfo}">
          <div class="card-header">
            <h5 class="card-title">
              <i class="fa-solid fa-desktop"></i>
              System Information
            </h5>
          </div>
          <div class="card-content">
            <div class="info-row">
              <span class="info-label">Operating System</span>
              <span class="info-value">
                <span v-if="loadingState.systemInfo" class="skeleton-text">-------------------------</span>
                <span v-else-if="hasSystemData">
                  {{ state.systemInfo.hostInfo?.os?.name || 'Unknown' }}
                  {{ state.systemInfo.hostInfo?.os?.version || '' }}
                </span>
                <span v-else>Unknown</span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">Architecture</span>
              <span class="info-value">
                <span v-if="loadingState.systemInfo" class="skeleton-text">-----------</span>
                <span v-else>{{ state.systemInfo.hostInfo?.architecture || 'Unknown' }}</span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">Environment</span>
              <span class="info-value">
                <span v-if="loadingState.systemInfo" class="skeleton-text">-----------</span>
                <span v-else-if="hasSystemData">
                  {{ state.systemInfo.hostInfo?.containerized ? 'Virtualized' : 'Physical' }}
                </span>
                <span v-else>Unknown</span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">Timezone</span>
              <span class="info-value">
                <span v-if="loadingState.systemInfo" class="skeleton-text">-------------------</span>
                <span v-else>{{ state.systemInfo.hostInfo?.timezone || 'Unknown' }}</span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">Location</span>
              <span class="info-value">
                <span v-if="loadingState.networkInfo" class="skeleton-text">------------------</span>
                <span v-else-if="hasNetworkData">{{ state.networkInfo.lat }}, {{ state.networkInfo.long }}</span>
                <span v-else>Unknown</span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">Last Seen</span>
              <span class="info-value">
                <span v-if="loadingState.agent" class="skeleton-text">------------</span>
                <span v-else>{{ state.agent.updatedAt ? since(state.agent.updatedAt, true) : 'Never' }}</span>
              </span>
            </div>
          </div>
          <hr v-if="hasSystemData">
          <div class="info-row px-3 pb-3" v-if="hasSystemData">
            <span class="info-label">System data from</span>
            <span class="info-value">
              <span>{{ since(state.systemInfo.timestamp + "", true) }}</span>
            </span>
          </div>
        </div>

        <!-- Network Interfaces -->
        <div class="info-card" :class="{'loading': loadingState.systemInfo}">
          <div class="card-header">
            <h5 class="card-title">
              <i class="fa-solid fa-ethernet"></i>
              Network Interfaces
            </h5>
          </div>
          <div class="card-content">
            <ElementExpand title="MAC Addresses" code :disabled="loadingState.systemInfo || !hasSystemData">
              <span v-if="loadingState.systemInfo" class="badge bg-secondary">
                <i class="fa-solid fa-spinner fa-spin"></i> Loading
              </span>
              <span v-else-if="hasSystemData && state.systemInfo.hostInfo?.mac" class="badge bg-secondary">
                {{ Object.keys(state.systemInfo.hostInfo.mac).length }} interfaces
              </span>
              <span v-else class="badge bg-secondary">0 interfaces</span>
              <template v-slot:expanded>
                <div class="mac-list">
                  <div v-if="loadingState.systemInfo" v-for="i in 2" :key="`mac-skeleton-${i}`" class="mac-item skeleton">
                    <div class="mac-address skeleton-text">--:--:--:--:--:--</div>
                    <div class="mac-vendor skeleton-text">--------------------------</div>
                  </div>
                  <div v-else-if="hasSystemData && state.systemInfo.hostInfo?.mac" v-for="(mac, iface) in state.systemInfo.hostInfo.mac" :key="iface" class="mac-item">
                    <div class="mac-address">{{ mac }}</div>
                    <div class="mac-vendor">{{ getVendorFromMac(mac) }}</div>
                  </div>
                  <div v-else class="text-muted">No MAC addresses found</div>
                </div>
              </template>
            </ElementExpand>
          </div>
        </div>
      </div>
    </div>
  </div>
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

.status-badge.offline {
  background: #fef2f2;
  color: #dc2626;
}

.status-badge.loading {
  background: #f3f4f6;
  color: #6b7280;
}

.status-badge i {
  font-size: 0.5rem;
}

/* Quick Stats */
.quick-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 1rem;
  margin-bottom: 1.5rem;
}

.stat-item {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 1.25rem;
  display: flex;
  align-items: center;
  gap: 1rem;
  transition: all 0.2s;
}

.stat-item:hover:not(.loading) {
  transform: translateY(-2px);
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
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

/* Probes Grid */
.probes-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 1rem;
  padding: 1.25rem;
}

.probe-card {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  transition: all 0.2s;
  overflow: hidden;
  background: white;
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

/* Responsive */
@media (max-width: 768px) {
  .quick-stats {
    grid-template-columns: repeat(2, 1fr);
  }

  .info-grid {
    grid-template-columns: 1fr;
  }

  .probes-grid {
    grid-template-columns: 1fr;
  }

  .info-row {
    flex-direction: column;
    gap: 0.25rem;
  }

  .info-label {
    min-width: auto;
  }

  .info-value {
    text-align: left;
  }
}

@media (max-width: 576px) {
  .quick-stats {
    grid-template-columns: 1fr;
  }

  .stat-item {
    padding: 1rem;
  }

  .probes-grid {
    padding: 1rem;
  }
}
</style>