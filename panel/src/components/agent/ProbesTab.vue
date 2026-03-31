<script setup lang="ts">
import { computed } from 'vue';
import type { Agent, Probe } from '@/types';
import { useAgentStatus, type AgentStatusTier } from '@/composables/useAgentStatus';
import { since } from '@/time';

interface ProbeGroup {
  key: string;
  probes: Probe[];
  kind: 'agent' | 'host' | 'local';
  id: string;
  label: string;
  types: string[];
  perType: Record<string, { count: number; enabled: number }>;
}

interface ProbeGroupStats {
  lastRun?: string;
  successRate?: number;
  avgResponseTime?: number;
  status?: 'healthy' | 'warning' | 'critical' | 'unknown';
  isLoading?: boolean;
  hasData?: boolean;
}

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

function getStatusColor(status?: string): string {
  switch (status) {
    case 'healthy': return 'text-success';
    case 'warning': return 'text-warning';
    case 'critical': return 'text-danger';
    default: return 'text-muted';
  }
}

function getStatusIcon(status?: string): string {
  switch (status) {
    case 'healthy': return 'bi-check-circle-fill';
    case 'warning': return 'bi-exclamation-triangle-fill';
    case 'critical': return 'bi-x-circle-fill';
    default: return 'bi-question-circle';
  }
}
</script>

<template>
  <div class="tab-panel">
    <div class="content-section probes-section">
      <div class="section-header">
        <h5 class="section-title">
          <i class="bi bi-diagram-2"></i>
          Monitoring Probes
        </h5>
        <span class="badge bg-primary" v-if="!loadingProbes">
          {{ totalProbes }} Probes
        </span>
        <span class="badge bg-secondary" v-else>
          <i class="bi bi-arrow-repeat spin-animation"></i> Loading
        </span>
      </div>

      <div v-if="loadingProbes" class="probes-grid">
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
            <i class="bi bi-chevron-right probe-arrow"></i>
          </div>
        </div>
      </div>

      <div v-else-if="targetGroups.length > 0" class="probes-grid">
        <div 
          v-for="g in targetGroups" 
          :key="g.key" 
          class="probe-card" 
          :class="{'has-issues': groupStats[g.key]?.status === 'critical'}"
        >
          <router-link 
            :to="`/workspaces/${workspaceId}/agents/${agentId}/probes/${g.probes[0]?.id || ''}`" 
            class="probe-link"
          >
            <div class="probe-header">
              <div class="probe-icon">
                <i :class="g.kind === 'agent' ? 'bi bi-robot'
                : g.kind === 'host' ? 'bi bi-diagram-2'
                : 'bi bi-cpu'"></i>
              </div>
              <div class="probe-status">
                <template v-if="g.kind === 'agent' && targetAgents[Number(g.id)] && agentStatus.getAgentStatus(targetAgents[Number(g.id)]) !== 'online'">
                  <i :class="agentStatus.getStatusIcon(agentStatus.getAgentStatus(targetAgents[Number(g.id)]))"></i>
                </template>
                <template v-else>
                  <i :class="`bi ${getStatusIcon(groupStats[g.key]?.status)} ${getStatusColor(groupStats[g.key]?.status)}`"></i>
                </template>
              </div>
            </div>

            <div class="probe-content">
              <h6 class="probe-title">
                <span v-if="g.kind==='host'">{{ g.label }}</span>
                <span v-else-if="g.kind==='agent'">{{ agentNames[Number(g.id)] || `Agent #${g.id}` }}</span>
                <span v-else>Local on Agent {{ g.id }}</span>
              </h6>

              <div class="probe-types">
                <span v-for="t in g.types" :key="t" class="probe-type-badge">
                  {{ t }} ({{ g.perType[t].count }})
                </span>
              </div>

              <div class="probe-stats" v-if="groupStats[g.key]">
                <div v-if="groupStats[g.key].isLoading" class="probe-stat">
                  <i class="bi bi-arrow-repeat spin-animation"></i>
                  <span>Loading stats...</span>
                </div>
                <template v-else-if="groupStats[g.key].hasData">
                  <div class="probe-stat" v-if="groupStats[g.key].successRate !== undefined">
                    <i class="bi bi-graph-up"></i>
                    <span>{{ groupStats[g.key].successRate.toFixed(1) }}% success</span>
                  </div>
                  <div class="probe-stat" v-if="groupStats[g.key].avgResponseTime !== undefined">
                    <i class="bi bi-stopwatch"></i>
                    <span>{{ groupStats[g.key].avgResponseTime.toFixed(0) }}ms avg</span>
                  </div>
                  <div class="probe-stat" v-if="groupStats[g.key].lastRun">
                    <i class="bi bi-clock"></i>
                    <span>{{ since(groupStats[g.key].lastRun, true) }}</span>
                  </div>
                </template>
                <div v-else class="probe-stat text-muted">
                  <i class="bi bi-info-circle"></i>
                  <span>No ping data available</span>
                </div>
                <!-- Target agent connectivity status for agent-type groups -->
                <div v-if="g.kind === 'agent' && targetAgents[Number(g.id)]" 
                     class="probe-stat"
                     :class="agentStatus.getStatusColor(agentStatus.getAgentStatus(targetAgents[Number(g.id)]))">
                  <i :class="agentStatus.getStatusIcon(agentStatus.getAgentStatus(targetAgents[Number(g.id)]))"></i>
                  <span>Target {{ agentStatus.getStatusLabel(agentStatus.getAgentStatus(targetAgents[Number(g.id)])) }}</span>
                  <span class="text-muted" style="margin-left: 0.25rem;">· {{ agentStatus.getLastSeenText(targetAgents[Number(g.id)]) }}</span>
                </div>
              </div>
            </div>

            <i class="bi bi-chevron-right probe-arrow"></i>
          </router-link>
        </div>
      </div>

      <div v-else-if="!loadingProbes" class="empty-state">
        <i class="bi bi-diagram-2"></i>
        <h5>No Probes Configured</h5>
        <p>Create your first probe to start monitoring</p>
        <router-link
          v-if="agentId && workspaceId"
          :to="`/workspaces/${workspaceId}/agents/${agentId}/probes/new`"
          class="btn btn-primary"
        >
          <i class="bi bi-plus-lg"></i> Create Probe
        </router-link>
      </div>
    </div>
  </div>
</template>
