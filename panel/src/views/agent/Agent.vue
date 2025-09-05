<script lang="ts" setup>
import {onMounted, reactive, computed} from "vue";
import type {
  Agent,
  AgentGroup,
  CompleteSystemInfo,
  CPUTimes,
  HostInfo,
  HostMemoryInfo, NetInfoPayload,
  OSInfo,
  Probe,
  ProbeData,
  ProbeType,
  Workspace
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
import agent from "@/views/agent/index";
import {AgentService, ProbeService, WorkspaceService} from "@/services/apiService";

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

// Computed properties for better organization
const isOnline = computed(() => {
  if (!state.systemInfoComplete?.timestamp) return false;
  const lastSeen = new Date(state.systemInfoComplete.timestamp);
  const now = new Date();
  const diffMinutes = (now.getTime() - lastSeen.getTime()) / 60000;
  return diffMinutes <= 5; // Consider online if seen in last 5 minutes
});

const cpuUsagePercent = computed(() => {
  if (!state.systemData?.cpu) return 0;
  return ((state.systemData.cpu.user + state.systemData.cpu.system) * 100).toFixed(1);
});

const memoryUsagePercent = computed(() => {
  if (!state.systemData?.ram) return 0;
  return (state.systemData.ram.used * 100).toFixed(1);
});

function roundTo(value: number): number {
  return Math.round(value * 1000) / 1000
}

function updateSystemData(info: CompleteSystemInfo): SystemData {
  let cpuCapacity: number = (info.CPUTimes?.idle || 0) + info.CPUTimes.system + info.CPUTimes.user;
  let ramCapacity: number = info.memoryInfo.totalBytes;
  let virtualCapacity: number = info.memoryInfo.virtualTotalBytes;
  return {
    cpu: {
      idle: roundTo((info.CPUTimes?.idle || 0) / cpuCapacity),
      system: roundTo((info.CPUTimes?.system || 0) / cpuCapacity),
      user: roundTo((info.CPUTimes?.user || 0) / cpuCapacity),
    },
    ram: {
      used: roundTo(info.memoryInfo.usedBytes / ramCapacity),
      free: roundTo(info.memoryInfo.availableBytes / ramCapacity),
      total: roundTo(info.memoryInfo.totalBytes / ramCapacity),
    },
    virtual: {
      used: roundTo(info.memoryInfo.virtualUsedBytes / ramCapacity),
      free: roundTo(info.memoryInfo.virtualFreeBytes / virtualCapacity),
      total: roundTo(info.memoryInfo.virtualTotalBytes / virtualCapacity),
    }
  } as SystemData
}

function getVendorFromMac(macAddress: string) {
  const normalizedMac = macAddress.replace(/[:-]/g, '').toUpperCase();
  const oui = normalizedMac.substring(0, 6);
  const entry = state.ouiList.find(item => item.Assignment == oui);
  return entry ? (entry as OUIEntry)["Organization Name"] : "Unknown Vendor";
}

let state = reactive({
  workspace: {} as Workspace,
  ready: false,
  loading: true,
  agent: {} as Agent,
  agents: [] as Agent[],
  probes: [] as Probe[],
  networkInfo: {} as NetInfoPayload,
  systemInfoComplete: {} as CompleteSystemInfo,
  systemData: {} as SystemData,
  hasData: false,
  ouiList: [] as OUIEntry[]
})

function convertToCompleteSystemInfo(data: any[]): CompleteSystemInfo {
  let completeSystemInfo: any = {};

  data.forEach(item => {
    switch (item.Key) {
      case 'hostInfo':
        completeSystemInfo.hostInfo = convertToHostInfo(item.Value);
        break;
      case 'memoryInfo':
        completeSystemInfo.memoryInfo = convertToHostMemoryInfo(item.Value);
        break;
      case 'CPUTimes':
        completeSystemInfo.CPUTimes = convertToCPUTimes(item.Value);
        break;
      case 'timestamp':
        completeSystemInfo.timestamp = new Date(item.Value);
        break;
    }
  });

  return completeSystemInfo as CompleteSystemInfo;
}

function convertToHostInfo(data: any[]): HostInfo {
  let hostInfo: any = {IPs: [], MACs: [], os: {}};

  data.forEach(item => {
    switch (item.Key) {
      case 'architecture':
      case 'bootTime':
      case 'containerized':
      case 'hostname':
      case 'kernelVersion':
      case 'timezone':
      case 'timezoneOffsetSec':
      case 'uniqueID':
        hostInfo[item.Key] = item.Value;
        break;
      case 'IPs':
        hostInfo.IPs = item.Value;
        break;
      case 'MACs':
        hostInfo.MACs = item.Value;
        break;
      case 'OS':
        hostInfo.os = convertToOSInfo(item.Value);
        break;
    }
  });

  hostInfo.bootTime = new Date(hostInfo.bootTime);

  return hostInfo as HostInfo;
}

function convertToOSInfo(data: any[]): OSInfo {
  let osInfo: any = {};

  data.forEach(item => {
    osInfo[item.Key] = item.Value;
  });

  return osInfo as OSInfo;
}

function convertToHostMemoryInfo(data: any[]): HostMemoryInfo {
  let memoryInfo: any = {metrics: {}};

  data.forEach(item => {
    if (item.Key === 'metrics' && item.Value != null) {
      item.Value.forEach((metric: any) => {
        memoryInfo.metrics[metric.Key] = metric.Value;
      });
    } else {
      memoryInfo[item.Key + 'Bytes'] = item.Value;
    }
  });

  return memoryInfo as HostMemoryInfo;
}

function convertToCPUTimes(data: any[]): CPUTimes {
  let cpuTimes: any = {};

  data.forEach(item => {
    cpuTimes[item.Key] = item.Value;
  });

  return cpuTimes as CPUTimes;
}

function reloadData(id: string) {
  state.probes = [] as Probe[]
  state.organizedProbes = [] as OrganizedProbe[];
  state.ready = false;

  probeService.getSystemInfo(id).then(res => {
    let sysInfo = (res.data as ProbeData)
    state.systemInfoComplete = convertToCompleteSystemInfo(sysInfo.data)
    state.systemData = updateSystemData(state.systemInfoComplete);
  })

  probeService.getNetworkInfo(id).then(res => {
    state.networkInfo = res.data as ProbeData
    state.netData = transformNetData(state.networkInfo.data)
  })

  agentService.getWorkspaceAgents(state.agent.workspaceId.toString()).then(res => {
    state.agents = res.data as Agent[]
  })

  siteService.getAgentGroups(state.agent.workspaceId.toString()).then(res => {
    state.agentGroups = res.data as AgentGroup[]

    /*probeService.getAgentProbes(id).then(res => {
      state.probes = res.data as Probe[]
      let organizedProbesMap = new Map<string, Probe[]>();

      for (let probe of state.probes) {
        if (probe.type == "SYSINFO" as ProbeType || probe.type == "NETINFO" as ProbeType) {
          continue
        }

        if (probe.config && probe.config.target) {
          for (let target of probe.config.target) {
            let key = target.target;

            if (probe.type == "SPEEDTEST") {
              continue
            }

            if (target.group && target.group != "000000000000000000000000") {
              key = `group:${target.group}`;
            } else if (target.agent && target.agent != "000000000000000000000000") {
              key = `agent:${target.agent}`;
            } else if (probe.type == "RPERF" && !probe.config.server && probe.config.target[0].agent != "000000000000000000000000") {
              key = target.target.split(':')[0]
            } else if (probe.type == "TRAFFICSIM" && !probe.config.server && probe.config.target[0].agent != "000000000000000000000000") {
              key = target.target.split(':')[0]
            } else if (probe.type == "TRAFFICSIM") {
              key = probe.type + " SERVER"
            }

            if (!organizedProbesMap.has(key)) {
              organizedProbesMap.set(key, []);
            }

            organizedProbesMap.get(key).push(probe);
          }
        }
      }

      state.organizedProbes = Array.from(organizedProbesMap, ([key, probes]) => ({key, probes}));
      state.ready = true
      state.hasData = true
      state.loading = false
    })*/
  })
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

function getGroupName(id: string): string {
  const group = state.agentGroups.find(group => group.id === id);
  return group ? group.name : 'Unknown Group';
}

function getAgentName(id: string) {
  let agent = state.agents.find(a => a.id == id);
  return agent ? agent.name : 'Unknown Agent';
}

function probeTitle(probeKey: string): string {
  if (probeKey.startsWith("group:")) {
    return getGroupName(probeKey.split(":")[1]);
  } else if (probeKey.startsWith("agent:")) {
    return getAgentName(probeKey.split(":")[1]);
  } else {
    return probeKey;
  }
}

function getRandomProbeId(list: Probe[]): string | undefined {
  if (list.length === 0) {
    return undefined;
  }
  const randomIndex = Math.floor(Math.random() * list.length);
  return list[randomIndex].id;
}

const router = core.router()

onMounted(() => {
  let agentID = router.currentRoute.value.params["aID"] as string
  let workspaceID = router.currentRoute.value.params["wID"] as string
  if (!agentID || !workspaceID) return

  WorkspaceService.get(workspaceID).then(res => {
    state.workspace = res as Workspace
  })

  AgentService.get(workspaceID, agentID).then(res => {
    state.agent = res as Agent
  })

  ProbeService.netInfo(workspaceID, agentID).then(res => {
    let pD = res as ProbeData
    state.networkInfo = pD.payload as NetInfoPayload
  })

  fetch('/ouiList.json')
      .then(response => response.json())
      .then(data => state.ouiList = data as OUIEntry[]);
})
</script>

<template>
  <div class="container-fluid">
    <Title
        :history="[
        {title: 'workspaces', link: '/workspaces'},
        {title: state.workspace.name || 'Loading...', link: `/workspace/${state.workspace.id}`}
      ]"
        :title="state.agent.name || 'Loading...'"
        :subtitle="state.agent.location || 'Agent Information'">
      <div class="d-flex flex-wrap gap-2">
        <div class="status-badge" :class="state.loading ? 'loading' : (isOnline ? 'online' : 'offline')">
          <i :class="state.loading ? 'fa-solid fa-spinner fa-spin' : 'fa-solid fa-circle'"></i>
          {{ state.loading ? 'Loading...' : (isOnline ? 'Online' : 'Offline') }}
        </div>
        <router-link :to="`/workspace/${state.agent.workspaceId}/agent/${state.agent.id}/probes`" class="btn btn-outline-primary" :class="{'': state.loading}">
          <i class="fa-regular fa-pen-to-square"></i>
          <span class="d-none d-sm-inline">&nbsp;Edit Probes</span>
        </router-link>
        <router-link :to="`/workspace/${state.agent.workspaceId}/agent/${state.agent.id}/probe/new`" class="btn btn-primary" :class="{'': state.loading}">
          <i class="fa-solid fa-plus"></i>&nbsp;Add Probe
        </router-link>
      </div>
    </Title>

    <!-- Quick Stats Bar - Always visible with loading state -->
    <div class="quick-stats">
      <div class="stat-item" :class="{'loading': state.loading}">
        <div class="stat-icon cpu">
          <i class="fa-solid fa-microchip"></i>
        </div>
        <div class="stat-content">
          <div class="stat-value">
            <span v-if="state.loading" class="skeleton-text">--</span>
            <span v-else>{{ cpuUsagePercent }}%</span>
          </div>
          <div class="stat-label">CPU Usage</div>
        </div>
      </div>
      <div class="stat-item" :class="{'loading': state.loading}">
        <div class="stat-icon memory">
          <i class="fa-solid fa-memory"></i>
        </div>
        <div class="stat-content">
          <div class="stat-value">
            <span v-if="state.loading" class="skeleton-text">--</span>
            <span v-else>{{ memoryUsagePercent }}%</span>
          </div>
          <div class="stat-label">Memory Usage</div>
        </div>
      </div>
      <div class="stat-item" :class="{'loading': state.loading}">
        <div class="stat-icon network">
          <i class="fa-solid fa-network-wired"></i>
        </div>
        <div class="stat-content">
          <div class="stat-value">
            <span v-if="state.loading" class="skeleton-text">-</span>
            <span v-else>{{ state.organizedProbes.length }}</span>
          </div>
          <div class="stat-label">Active Probes</div>
        </div>
      </div>
      <div class="stat-item" :class="{'loading': state.loading}">
        <div class="stat-icon uptime">
          <i class="fa-solid fa-clock"></i>
        </div>
        <div class="stat-content">
          <div class="stat-value">
            <span v-if="state.loading" class="skeleton-text">--</span>
            <span v-else>{{ since(state.systemInfoComplete.hostInfo?.bootTime + "", false) }}</span>
          </div>
          <div class="stat-label">Uptime</div>
        </div>
      </div>
    </div>

    <!-- Main Content - Always show layout -->
    <div v-if="!state.agent.initialized && !state.loading" class="empty-state">
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
          <span class="badge bg-primary" v-if="!state.loading">{{ state.organizedProbes.length }} Active</span>
          <span class="badge bg-secondary" v-else>
            <i class="fa-solid fa-spinner fa-spin"></i> Loading
          </span>
        </div>

        <div v-if="state.loading" class="probes-grid">
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
              </div>
              <i class="fa-solid fa-chevron-right probe-arrow"></i>
            </div>
          </div>
        </div>

        <div v-else-if="state.organizedProbes.length > 0" class="probes-grid">
          <div v-for="(organized, index) in state.organizedProbes" :key="index" class="probe-card">
            <router-link :to="`/probe/${getRandomProbeId(organized.probes)}`" class="probe-link">
              <div class="probe-icon">
                <i :class="organized.key.startsWith('agent:') ? 'fa-solid fa-robot' : 'fa-solid fa-diagram-project'"></i>
              </div>
              <div class="probe-content">
                <h6 class="probe-title">{{ probeTitle(organized.key) }}</h6>
                <div class="probe-types">
                  <span v-for="probe in organized.probes" :key="probe.id" class="probe-type-badge">
                    {{ probe.type }}
                  </span>
                </div>
              </div>
              <i class="fa-solid fa-chevron-right probe-arrow"></i>
            </router-link>
          </div>
        </div>

        <div v-else class="empty-state">
          <i class="fa-solid fa-diagram-project"></i>
          <h5>No Probes Configured</h5>
          <p>Create your first probe to start monitoring</p>
          <router-link :to="`/probe/${state.agent.id}/new`" class="btn btn-primary">
            <i class="fa-solid fa-plus"></i> Create Probe
          </router-link>
        </div>
      </div>

      <!-- System Information Grid -->
      <div class="info-grid">
        <!-- Network Information -->
        <div class="info-card" :class="{'loading': state.loading}">
          <div class="card-header">
            <h5 class="card-title">
              <i class="fa-solid fa-network-wired"></i>
              Network Information
            </h5>
          </div>
          <div class="card-content">
            <div class="info-row">
              <span class="info-label">Hostname</span>
              <span class="info-value">
                <span v-if="state.loading" class="skeleton-text">--------------------</span>
                <span v-else>{{ state.systemInfoComplete.hostInfo?.hostname || 'Unknown' }}</span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">Public IP</span>
              <span class="info-value">
                <span v-if="state.loading" class="skeleton-text">---------------</span>
                <span v-else>{{ state.networkInfo.public_address || 'Loading...' }}</span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">ISP</span>
              <span class="info-value">
                <span v-if="state.loading" class="skeleton-text">-------------------------</span>
                <span v-else>{{ state.netData.internetProvider || 'Loading...' }}</span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">Gateway</span>
              <span class="info-value">
                <span v-if="state.loading" class="skeleton-text">---------------</span>
                <span v-else>{{ state.netData.defaultGateway || 'Loading...' }}</span>
              </span>
            </div>
            <div class="info-row expandable">
              <span class="info-label">Local IPs</span>
              <div class="info-value">
                <div v-if="state.loading" class="skeleton-text">---------------</div>
                <div v-else v-for="ip in getLocalAddresses(state.systemInfoComplete.hostInfo?.IPs || [])" :key="ip">
                  {{ ip }}
                </div>
              </div>
            </div>
          </div>
          <div class="card-footer">
            <router-link :to="`/agent/${state.agent.id}/speedtests`" class="btn btn-sm btn-outline-secondary" :class="{'disabled': state.loading}">
              <i class="fa-solid fa-gauge-high"></i> View Speedtests
            </router-link>
          </div>
        </div>

        <!-- System Resources -->
        <div class="info-card" :class="{'loading': state.loading}">
          <div class="card-header">
            <h5 class="card-title">
              <i class="fa-solid fa-server"></i>
              System Resources
            </h5>
          </div>
          <div class="card-content">
            <div class="resource-meter">
              <div class="resource-header">
                <span>CPU Usage</span>
                <span v-if="state.loading" class="skeleton-text">---%</span>
                <span v-else>{{ cpuUsagePercent }}%</span>
              </div>
              <div class="progress">
                <div class="progress-bar bg-primary" :style="{width: state.loading ? '0%' : cpuUsagePercent + '%'}"></div>
              </div>
              <div class="resource-details">
                <span v-if="state.loading" class="skeleton-text">User: ---%</span>
                <span v-else>User: {{ (state.systemData.cpu?.user * 100).toFixed(1) }}%</span>
                <span v-if="state.loading" class="skeleton-text">System: ---%</span>
                <span v-else>System: {{ (state.systemData.cpu?.system * 100).toFixed(1) }}%</span>
              </div>
            </div>

            <div class="resource-meter">
              <div class="resource-header">
                <span>Memory Usage</span>
                <span v-if="state.loading" class="skeleton-text">---%</span>
                <span v-else>{{ memoryUsagePercent }}%</span>
              </div>
              <div class="progress">
                <div class="progress-bar bg-success" :style="{width: state.loading ? '0%' : memoryUsagePercent + '%'}"></div>
              </div>
              <div class="resource-details">
                <span v-if="state.loading" class="skeleton-text">Used: --- GB</span>
                <span v-else>Used: {{ bytesToString(state.systemInfoComplete.memoryInfo?.usedBytes || 0) }}</span>
                <span v-if="state.loading" class="skeleton-text">Total: --- GB</span>
                <span v-else>Total: {{ bytesToString(state.systemInfoComplete.memoryInfo?.totalBytes || 0) }}</span>
              </div>
            </div>

            <ElementExpand title="Memory Details" code :disabled="state.loading">
              <template v-slot:expanded>
                <div class="memory-details">
                  <div v-if="state.loading" v-for="i in 4" :key="`mem-skeleton-${i}`" class="detail-row">
                    <span class="skeleton-text">--------------</span>
                    <span class="skeleton-text">--- GB</span>
                  </div>
                  <div v-else v-for="(value, key) in state.systemInfoComplete.memoryInfo?.metrics" :key="key" class="detail-row">
                    <span>{{ formatSnakeCaseToHumanCase(key) }}</span>
                    <span>{{ bytesToString(value) }}</span>
                  </div>
                </div>
              </template>
            </ElementExpand>
          </div>
        </div>

        <!-- System Information -->
        <div class="info-card" :class="{'loading': state.loading}">
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
                <span v-if="state.loading" class="skeleton-text">-------------------------</span>
                <span v-else>
                  {{ state.systemInfoComplete.hostInfo?.os?.name }}
                  {{ state.systemInfoComplete.hostInfo?.os?.version }}
                </span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">Architecture</span>
              <span class="info-value">
                <span v-if="state.loading" class="skeleton-text">-----------</span>
                <span v-else>{{ state.systemInfoComplete.hostInfo?.architecture }}</span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">Environment</span>
              <span class="info-value">
                <span v-if="state.loading" class="skeleton-text">-----------</span>
                <span v-else>
                  {{ state.systemInfoComplete.hostInfo?.containerized ? 'Virtualized' : 'Physical' }}
                </span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">Timezone</span>
              <span class="info-value">
                <span v-if="state.loading" class="skeleton-text">-------------------</span>
                <span v-else>{{ state.systemInfoComplete.hostInfo?.timezone }}</span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">Location</span>
              <span class="info-value">
                <span v-if="state.loading" class="skeleton-text">------------------</span>
                <span v-else>{{ state.netData.lat }}, {{ state.netData.long }}</span>
              </span>
            </div>
            <div class="info-row">
              <span class="info-label">Last Seen</span>
              <span class="info-value">
                <span v-if="state.loading" class="skeleton-text">------------</span>
                <span v-else>{{ since(state.systemInfoComplete.timestamp + "", true) }}</span>
              </span>
            </div>
          </div>
        </div>

        <!-- Network Interfaces -->
        <div class="info-card" :class="{'loading': state.loading}">
          <div class="card-header">
            <h5 class="card-title">
              <i class="fa-solid fa-ethernet"></i>
              Network Interfaces
            </h5>
          </div>
          <div class="card-content">
            <ElementExpand title="MAC Addresses" code :disabled="state.loading">
              <span v-if="state.loading" class="badge bg-secondary">
                <i class="fa-solid fa-spinner fa-spin"></i> Loading
              </span>
              <span v-else class="badge bg-secondary">{{ Object.keys(state.systemInfoComplete.hostInfo?.MACs || {}).length }} interfaces</span>
              <template v-slot:expanded>
                <div class="mac-list">
                  <div v-if="state.loading" v-for="i in 2" :key="`mac-skeleton-${i}`" class="mac-item skeleton">
                    <div class="mac-address skeleton-text">--:--:--:--:--:--</div>
                    <div class="mac-vendor skeleton-text">--------------------------</div>
                  </div>
                  <div v-else v-for="(mac, iface) in state.systemInfoComplete.hostInfo?.MACs" :key="iface" class="mac-item">
                    <div class="mac-address">{{ mac }}</div>
                    <div class="mac-vendor">{{ getVendorFromMac(mac) }}</div>
                  </div>
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
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 1rem;
  padding: 1.25rem;
}

.probe-card {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  transition: all 0.2s;
  overflow: hidden;
}

.probe-card:hover:not(.skeleton) {
  border-color: #3b82f6;
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
}

.probe-link {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1rem;
  text-decoration: none;
  color: inherit;
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

.probe-content {
  flex: 1;
}

.probe-title {
  margin: 0 0 0.5rem 0;
  font-size: 1rem;
  font-weight: 600;
  color: #1f2937;
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

.probe-arrow {
  color: #9ca3af;
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

/* Disabled state for buttons */
.btn.disabled {
  opacity: 0.6;
  cursor: not-allowed;
  pointer-events: none;
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