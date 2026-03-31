# Example: Tab Component Extraction

## Before (in parent)

```vue
<template>
  <div class="agent-content">
    <!-- Tab Navigation -->
    <div class="agent-tabs">
      <button :class="{ active: activeTab === 'overview' }" @click="activeTab = 'overview'">Overview</button>
      <button :class="{ active: activeTab === 'probes' }" @click="activeTab = 'probes'">Probes</button>
      <button :class="{ active: activeTab === 'system' }" @click="activeTab = 'system'">System</button>
    </div>

    <!-- OVERVIEW TAB -->
    <div v-show="activeTab === 'overview'" class="tab-panel">
      <div class="info-grid">
        <!-- Network Information Card -->
        <div class="info-card">
          <div class="card-header">
            <h5><i class="bi bi-globe2"></i> Network Information</h5>
          </div>
          <div class="card-content">
            <div class="info-row">
              <span class="info-label">Public IP</span>
              <span class="info-value">{{ state.networkInfo.public_address }}</span>
            </div>
            <!-- More rows... -->
          </div>
        </div>
        
        <!-- Network Interfaces Card -->
        <div class="info-card">
          <div class="card-header">
            <h5><i class="bi bi-ethernet"></i> Network Interfaces</h5>
          </div>
          <div class="card-content">
            <div v-for="iface in state.networkInfo.interfaces" :key="iface.name" class="interface-item">
              <div class="interface-icon"><i :class="getInterfaceIcon(iface.name)"></i></div>
              <div class="interface-info">
                <div class="interface-name">{{ iface.name }}</div>
                <div class="interface-mac"><code>{{ iface.mac }}</code></div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- PROBES TAB -->
    <div v-show="activeTab === 'probes'" class="tab-panel">
      <!-- 200+ lines of probe grid... -->
    </div>

    <!-- SYSTEM TAB -->
    <div v-show="activeTab === 'system'" class="tab-panel">
      <!-- 300+ lines of system info... -->
    </div>
  </div>
</template>

<script>
// Helper functions used across tabs
function getInterfaceIcon(ifaceName: string): string {
  const type = getInterfaceType(ifaceName);
  switch (type) {
    case 'wifi': return 'bi bi-wifi';
    case 'ethernet': return 'bi bi-ethernet';
    default: return 'bi bi-hdd-network';
  }
}

function getInterfaceType(ifaceName: string): string {
  const lower = ifaceName.toLowerCase();
  if (lower.includes('wifi')) return 'wifi';
  if (lower.includes('eth')) return 'ethernet';
  return 'other';
}

function bytesToString(bytes: number): string {
  // Format bytes to human readable
}

function getStatusColor(status?: string): string {
  switch (status) {
    case 'healthy': return 'text-success';
    case 'warning': return 'text-warning';
    default: return 'text-muted';
  }
}
</script>
```

## After (extracted components)

**OverviewTab.vue:**
```vue
<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Agent, NetInfoPayload, SysInfoPayload } from '@/types';

interface Props {
  agent: Agent;
  workspaceId: string | number;
  loadingState: { systemInfo: boolean; networkInfo: boolean };
  systemInfo: SysInfoPayload;
  networkInfo: NetInfoPayload;
  isOnline: boolean;
  ouiCache: Record<string, string>;
}

const props = defineProps<Props>();
const copiedField = ref<string | null>(null);

// Helper functions moved from parent
function getInterfaceType(ifaceName: string): string {
  // Implementation
}

function getInterfaceIcon(ifaceName: string): string {
  // Implementation
}

async function copyToClipboard(text: string, fieldName: string) {
  // Implementation
}
</script>

<template>
  <div class="tab-panel">
    <div class="info-grid">
      <!-- Network Information Card -->
      <div class="info-card" :class="{'loading': loadingState.networkInfo}">
        <div class="card-header">
          <h5><i class="bi bi-globe2"></i> Network Information</h5>
          <div class="connection-status" v-if="!loadingState.networkInfo">
            <span class="status-dot" :class="isOnline ? 'online' : 'offline'"></span>
            <span class="status-text">{{ isOnline ? 'Connected' : 'Offline' }}</span>
          </div>
        </div>
        <div class="card-content">
          <!-- Content -->
        </div>
      </div>

      <!-- Network Interfaces Card -->
      <div class="info-card">
        <div class="card-header">
          <h5><i class="bi bi-ethernet"></i> Network Interfaces</h5>
        </div>
        <div class="card-content">
          <!-- Interface list -->
        </div>
      </div>
    </div>
  </div>
</template>
<style scoped>
/* Tab-specific styles */
</style>
```

**ProbesTab.vue:**
```vue
<script setup lang="ts">
import type { Agent, Probe } from '@/types';
import { useAgentStatus } from '@/composables/useAgentStatus';

interface Props {
  loadingProbes: boolean;
  totalProbes: number;
  targetGroups: ProbeGroup[];
  groupStats: Record<string, ProbeGroupStats>;
  targetAgents: Record<number, Agent>;
  agentNames: Record<number, string>;
  workspaceId: string | number;
  agentId: string | number;
}

const props = defineProps<Props>();
const agentStatus = useAgentStatus();

// Status helpers specific to probes
function getStatusColor(status?: string): string {
  // Implementation
}

function getStatusIcon(status?: string): string {
  // Implementation
}
</script>

<template>
  <div class="tab-panel">
    <div class="content-section probes-section">
      <div class="section-header">
        <h5 class="section-title">
          <i class="bi bi-diagram-2"></i> Monitoring Probes
        </h5>
        <span class="badge bg-primary" v-if="!loadingProbes">
          {{ totalProbes }} Probes
        </span>
      </div>

      <!-- Probe grid -->
      <div v-if="targetGroups.length > 0" class="probes-grid">
        <div v-for="g in targetGroups" :key="g.key" class="probe-card">
          <router-link :to="`/workspaces/${workspaceId}/agents/${agentId}/probes/${g.probes[0]?.id || ''}`">
            <!-- Probe card content -->
          </router-link>
        </div>
      </div>
    </div>
  </div>
</template>
```

**Parent component after extraction:**
```vue
<template>
  <div class="agent-content">
    <!-- Tab Navigation (keep in parent) -->
    <div class="agent-tabs">
      <button :class="{ active: activeTab === 'overview' }" @click="activeTab = 'overview'">Overview</button>
      <button :class="{ active: activeTab === 'probes' }" @click="activeTab = 'probes'">Probes</button>
      <button :class="{ active: activeTab === 'system' }" @click="activeTab = 'system'">System</button>
    </div>

    <!-- Tab Components -->
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

    <SystemTab
      v-show="activeTab === 'system'"
      :loading-state="loadingState"
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
</template>

<script setup>
import OverviewTab from '@/components/agent/OverviewTab.vue'
import ProbesTab from '@/components/agent/ProbesTab.vue'
import SystemTab from '@/components/agent/SystemTab.vue'

// Helper functions that were moved to components are now removed
// Only keep shared logic used by multiple tabs
</script>
```

## Key Points

1. **Tab switching stays in parent** - `v-show` logic and tab navigation buttons
2. **Each tab is self-contained** - Own props, styles, and helper functions
3. **Helper functions duplicated** - When used by only one tab, move to that tab
4. **Shared data passed as props** - State from parent flows down to tabs
5. **Massive reduction** - Parent goes from 800+ lines to ~100 lines

## Benefits

- **Single Responsibility** - Each tab handles one concern
- **Easier testing** - Can test tabs in isolation
- **Better code review** - Smaller, focused files
- **Parallel development** - Multiple devs can work on different tabs
