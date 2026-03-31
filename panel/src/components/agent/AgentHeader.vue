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
    :history="[
      { title: 'Workspaces', link: '/workspaces' },
      { title: workspace.name || 'Loading...', link: `/workspaces/${workspace.id || ''}` }
    ]"
    :title="agent.name || 'Loading...'"
    :subtitle="agent.location || 'Agent Information'"
  >
    <div class="d-flex flex-wrap gap-2">
      <!-- Agent Status Badge -->
      <div class="status-badge" :class="isInitializing ? 'loading' : currentStatus">
        <i 
          :class="isInitializing 
            ? 'bi bi-arrow-repeat spin-animation' 
            : agentStatus.getStatusIcon(currentStatus)"
        ></i>
        {{ isInitializing ? 'Loading...' : agentStatus.getStatusLabel(currentStatus) }}
      </div>

      <!-- Live Data Indicator -->
      <div 
        v-if="wsConnected" 
        class="status-badge live" 
        :class="{ 'pulse': liveUpdating }"
      >
        <i class="bi bi-broadcast"></i>
        Live
      </div>
      <div 
        v-else 
        class="status-badge offline" 
        title="WebSocket disconnected - data may be stale"
      >
        <i class="bi bi-wifi-off"></i>
        Disconnected
      </div>

      <!-- Share Button -->
      <button
        v-if="agent.id && workspace.id"
        class="btn btn-outline-secondary"
        @click="emit('share')"
        title="Share this agent"
      >
        <i class="bi bi-share"></i>
        <span class="d-none d-sm-inline">&nbsp;Share</span>
      </button>

      <!-- Edit Probes Button -->
      <router-link
        v-if="agent.id && workspace.id && permissions.canEdit.value"
        :to="`/workspaces/${agent.workspace_id}/agents/${agent.id}/probes/edit`"
        class="btn btn-outline-primary"
      >
        <i class="bi bi-pencil-square"></i>
        <span class="d-none d-sm-inline">&nbsp;Edit Probes</span>
      </router-link>

      <!-- Add Probe Button -->
      <router-link
        v-if="agent.id && workspace.id && permissions.canEdit.value"
        :to="`/workspaces/${agent.workspace_id}/agents/${agent.id}/probes/new`"
        class="btn btn-primary"
      >
        <i class="bi bi-plus-lg"></i>&nbsp;Add Probe
      </router-link>
    </div>
  </Title>
</template>

<style scoped>
/* Status Badge Styles */
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.5rem 0.75rem;
  border-radius: 2rem;
  font-size: 0.875rem;
  font-weight: 500;
  white-space: nowrap;
}

.status-badge.online {
  background: rgba(25, 135, 84, 0.1);
  color: #198754;
  border: 1px solid rgba(25, 135, 84, 0.2);
}

.status-badge.stale {
  background: rgba(255, 193, 7, 0.1);
  color: #997404;
  border: 1px solid rgba(255, 193, 7, 0.2);
}

.status-badge.offline {
  background: rgba(220, 53, 69, 0.1);
  color: #dc3545;
  border: 1px solid rgba(220, 53, 69, 0.2);
}

.status-badge.loading {
  background: rgba(108, 117, 125, 0.1);
  color: #6c757d;
  border: 1px solid rgba(108, 117, 125, 0.2);
}

.status-badge.live {
  background: rgba(13, 202, 240, 0.1);
  color: #087990;
  border: 1px solid rgba(13, 202, 240, 0.2);
}

.status-badge.live.pulse {
  animation: pulse-badge 0.5s ease-out;
}

@keyframes pulse-badge {
  0% {
    transform: scale(1);
    box-shadow: 0 0 0 0 rgba(13, 202, 240, 0.4);
  }
  50% {
    transform: scale(1.05);
    box-shadow: 0 0 0 8px rgba(13, 202, 240, 0);
  }
  100% {
    transform: scale(1);
    box-shadow: 0 0 0 0 rgba(13, 202, 240, 0);
  }
}

/* Spin Animation */
.spin-animation {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

/* Responsive adjustments */
@media (max-width: 575px) {
  .status-badge {
    padding: 0.375rem 0.5rem;
    font-size: 0.8125rem;
  }
}
</style>
