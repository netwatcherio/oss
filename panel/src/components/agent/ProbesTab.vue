<script setup lang="ts">
import { computed } from 'vue';
import type { Agent, Probe } from '@/types';
import { useAgentStatus, type AgentStatusTier } from '@/composables/useAgentStatus';
import { since } from '@/time';
import ProbeHealthPopup from '@/components/analysis/ProbeHealthPopup.vue';

interface ProbeGroup {
  key: string;
  probes: Probe[];
  kind: 'agent' | 'host' | 'local';
  id: string;
  label: string;
  types: string[];
  perType: Record<string, { count: number; enabled: number }>;
  /**
   * Present for reverse-probe groups (probes owned by another agent that
   * target this one). Drives the "configured on Agent X" affordance and
   * the cross-agent navigation link.
   */
  reverseOwner?: {
    agentId: number;
    agentName: string;
    workspaceId: number;
    bidirectional: boolean;
  };
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
  /**
   * AGENT-type probes owned by other agents in the same workspace whose
   * targets include this agent. Rendered as a read-only "configured
   * elsewhere" section below the owned-probe grid.
   */
  reverseGroups?: ProbeGroup[];
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

const anyBidirectional = computed(() => {
  return (props.reverseGroups ?? []).some(g => g.reverseOwner?.bidirectional);
});
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
          <ProbeHealthPopup
            :probe-id="g.probes[0]?.id || ''"
            :workspace-id="workspaceId"
            :agent-name="g.kind === 'agent' ? (agentNames[Number(g.id)] || `Agent #${g.id}`) : undefined"
            :target="g.kind === 'host' ? g.label : undefined"
            trigger="click"
          >
            <template #default="{ show }">
              <div class="probe-link-wrapper">
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

                <!-- AI Analysis Button -->
                <button class="ai-analysis-btn" @click.prevent="show" title="AI Analysis">
                  <i class="bi bi-robot"></i>
                </button>
              </div>
            </template>
          </ProbeHealthPopup>
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

    <!-- REVERSE PROBES SECTION: AGENT-type probes owned by OTHER agents in the
         same workspace whose targets include this agent. Read-only by design —
         editing/deletion must happen on the owning agent. -->
    <div
      v-if="(reverseGroups?.length ?? 0) > 0"
      class="content-section reverse-probes-section"
    >
      <div class="section-header">
        <h5 class="section-title">
          <i class="bi bi-link-45deg"></i>
          Probes Targeting This Agent
        </h5>
        <span class="badge bg-secondary">{{ reverseGroups?.length ?? 0 }}</span>
      </div>
      <p class="section-subtitle text-muted">
        These probes are configured on other agents and target this one.
        <span v-if="anyBidirectional">Bidirectional ones will run return-path tests against this agent automatically.</span>
        <span v-else>None are bidirectional, so this agent is not running anything for them.</span>
      </p>
      <div class="probes-grid">
        <div
          v-for="g in reverseGroups"
          :key="g.key"
          class="probe-card reverse-probe-card"
        >
          <router-link
            v-if="g.reverseOwner"
            :to="`/workspaces/${g.reverseOwner.workspaceId}/agents/${g.reverseOwner.agentId}/probes/${g.probes[0]?.id || ''}`"
            class="probe-link"
          >
            <div class="probe-header">
              <div class="probe-icon reverse-icon">
                <i class="bi bi-link-45deg"></i>
              </div>
              <div class="probe-status">
                <i class="bi bi-box-arrow-up-right reverse-external"></i>
              </div>
            </div>

            <div class="probe-content">
              <h6 class="probe-title">
                <span class="reverse-owner-label">Configured on</span>
                <span class="reverse-owner-name">{{ g.reverseOwner.agentName }}</span>
              </h6>

              <div class="probe-types">
                <span
                  v-for="t in g.types"
                  :key="t"
                  class="probe-type-badge"
                >
                  {{ t }} ({{ g.perType[t].count }})
                </span>
              </div>

              <div class="probe-stats">
                <div class="probe-stat">
                  <i
                    :class="g.reverseOwner.bidirectional ? 'bi bi-arrow-left-right' : 'bi bi-arrow-right'"
                  ></i>
                  <span v-if="g.reverseOwner.bidirectional">Bidirectional — return-path probes are auto-generated</span>
                  <span v-else>One-way — this agent is not running return-path tests</span>
                </div>
              </div>
            </div>

            <i class="bi bi-box-arrow-up-right probe-arrow reverse-arrow"></i>
          </router-link>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* AI Analysis Button */
.probe-link-wrapper {
  position: relative;
  display: flex;
  align-items: center;
}

.probe-link-wrapper .probe-link {
  flex: 1;
}

.ai-analysis-btn {
  position: absolute;
  top: 50%;
  right: 40px;
  transform: translateY(-50%);
  background: var(--bs-primary-bg-subtle);
  border: 1px solid var(--bs-border-color);
  border-radius: 8px;
  padding: 6px 10px;
  cursor: pointer;
  color: var(--bs-primary);
  font-size: 14px;
  opacity: 0;
  transition: opacity 0.2s, background 0.2s;
  display: flex;
  align-items: center;
  gap: 4px;
  z-index: 5;
}

.probe-card:hover .ai-analysis-btn {
  opacity: 1;
}

.ai-analysis-btn:hover {
  background: var(--bs-primary);
  color: var(--bs-body-bg);
}

.ai-analysis-btn i {
  font-size: 14px;
}

/* --- Reverse probes section --- */
.reverse-probes-section {
  margin-top: 1.5rem;
  padding-top: 1.25rem;
  border-top: 1px dashed var(--bs-border-color);
}

.reverse-probes-section .section-subtitle {
  font-size: 0.85rem;
  margin-bottom: 0.75rem;
}

.reverse-probe-card {
  border-style: dashed;
  opacity: 0.95;
}

.reverse-icon {
  background: var(--bs-info-bg-subtle);
  color: var(--bs-info);
}

.reverse-arrow {
  color: var(--bs-info);
}

.reverse-external {
  font-size: 0.9rem;
  color: var(--bs-info);
}

.reverse-owner-label {
  display: block;
  font-size: 0.7rem;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--bs-secondary);
}

.reverse-owner-name {
  display: block;
  color: var(--bs-body-color);
}
</style>
