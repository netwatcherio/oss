# Example: Header Component Extraction

## Before (in parent component)

```vue
<template>
  <Title
    :history="[
      { title: 'Workspaces', link: '/workspaces' },
      { title: state.workspace.name || 'Loading...', link: `/workspaces/${state.workspace.id || ''}` }
    ]"
    :title="state.agent.name || 'Loading...'"
    :subtitle="state.agent.location || 'Agent Information'"
  >
    <div class="d-flex flex-wrap gap-2">
      <div class="status-badge" :class="isInitializing ? 'loading' : currentAgentStatus">
        <i :class="isInitializing ? 'bi bi-arrow-repeat spin-animation' : agentStatus.getStatusIcon(currentAgentStatus)"></i>
        {{ isInitializing ? 'Loading...' : agentStatus.getStatusLabel(currentAgentStatus) }}
      </div>
      <!-- Live Data Indicator -->
      <div v-if="wsConnected" class="status-badge live" :class="{ 'pulse': liveUpdating }">
        <i class="bi bi-broadcast"></i>
        Live
      </div>
      <button @click="showShareModal = true" class="btn btn-outline-secondary">
        <i class="bi bi-share"></i><span class="d-none d-sm-inline">&nbsp;Share</span>
      </button>
      <router-link :to="`/workspaces/${state.agent.workspace_id}/agents/${state.agent.id}/probes/edit`" class="btn btn-outline-primary">
        <i class="bi bi-pencil-square"></i><span class="d-none d-sm-inline">&nbsp;Edit Probes</span>
      </router-link>
      <router-link :to="`/workspaces/${state.agent.workspace_id}/agents/${state.agent.id}/probes/new`" class="btn btn-primary">
        <i class="bi bi-plus-lg"></i>&nbsp;Add Probe
      </router-link>
    </div>
  </Title>
</template>
```

## After (extracted component)

**AgentHeader.vue:**
```vue
<script setup lang="ts">
import type { Agent, Workspace } from "@/types";
import type { PermissionFlags } from "@/composables/usePermissions";
import { useAgentStatus, type AgentStatusTier } from "@/composables/useAgentStatus";
import Title from "@/components/Title.vue";

interface Props {
  agent: Agent;
  workspace: Workspace;
  permissions: PermissionFlags;
  isInitializing: boolean;
  currentStatus: AgentStatusTier;
  wsConnected: boolean;
  liveUpdating: boolean;
}

const props = defineProps<Props>();
const emit = defineEmits<{
  share: [];
  editProbes: [];
  addProbe: [];
}>();

const agentStatus = useAgentStatus();
</script>

<template>
  <Title
    :history="[...]"
    :title="agent.name || 'Loading...'"
    :subtitle="agent.location || 'Agent Information'"
  >
    <div class="d-flex flex-wrap gap-2">
      <!-- Status badges -->
      ...
      <button @click="emit('share')" class="btn btn-outline-secondary">
        <i class="bi bi-share"></i>
      </button>
    </div>
  </Title>
</template>

<style scoped>
/* Component-specific styles only */
</style>
```

**Usage in parent:**
```vue
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
```

## Key Points

1. **Props extracted:** agent, workspace, permissions, status booleans
2. **Emits defined:** share, editProbes, addProbe
3. **Logic preserved:** Status calculations using composables
4. **Styles moved:** Only relevant styles in component
