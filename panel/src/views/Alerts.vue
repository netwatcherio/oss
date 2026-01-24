<script lang="ts" setup>
import { onMounted, reactive, computed } from "vue";
import { useRouter } from "vue-router";
import Title from "@/components/Title.vue";
import { AlertService, WorkspaceService, type Alert } from "@/services/apiService";
import type { Workspace } from "@/types";

const router = useRouter();

const state = reactive({
  alerts: [] as Alert[],
  workspaces: [] as Workspace[],
  loading: true,
  error: null as string | null,
  filter: 'active' as 'active' | 'acknowledged' | 'resolved' | 'all',
});

const filteredAlerts = computed(() => {
  if (state.filter === 'all') return state.alerts;
  return state.alerts.filter(a => a.status === state.filter);
});

const activeCount = computed(() => state.alerts.filter(a => a.status === 'active').length);
const acknowledgedCount = computed(() => state.alerts.filter(a => a.status === 'acknowledged').length);
const resolvedCount = computed(() => state.alerts.filter(a => a.status === 'resolved').length);

function getWorkspaceName(workspaceId: number) {
  const ws = state.workspaces.find((w: Workspace) => w.id === workspaceId);
  return ws?.name || `Workspace ${workspaceId}`;
}

function formatMetric(metric: string): string {
  return metric.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
}

function formatTime(timestamp: string): string {
  return new Date(timestamp).toLocaleString();
}

function getSeverityClass(severity: string): string {
  return severity === 'critical' ? 'bg-danger' : 'bg-warning';
}

function getStatusClass(status: string): string {
  switch (status) {
    case 'active': return 'bg-danger';
    case 'acknowledged': return 'bg-warning text-dark';
    case 'resolved': return 'bg-success';
    default: return 'bg-secondary';
  }
}

function getProbeTypeClass(probeType: string): string {
  switch (probeType?.toUpperCase()) {
    case 'PING': return 'bg-info';
    case 'MTR': return 'bg-primary';
    case 'TRAFFICSIM': return 'bg-purple';
    default: return 'bg-secondary';
  }
}

function navigateToAgent(alert: Alert) {
  if (alert.agent_id) {
    router.push(`/workspaces/${alert.workspace_id}/agents/${alert.agent_id}`);
  }
}

function navigateToProbe(alert: Alert) {
  if (alert.agent_id && alert.probe_id) {
    // Center a 1-hour window around the alert trigger time
    const triggerTime = new Date(alert.triggered_at);
    const from = new Date(triggerTime.getTime() - 30 * 60 * 1000); // 30 min before
    const to = new Date(triggerTime.getTime() + 30 * 60 * 1000);   // 30 min after
    
    router.push({
      path: `/workspaces/${alert.workspace_id}/agents/${alert.agent_id}`,
      query: {
        probe: alert.probe_id.toString(),
        from: from.toISOString(),
        to: to.toISOString()
      }
    });
  }
}

async function acknowledge(alert: Alert) {
  try {
    await AlertService.acknowledge(alert.id);
    alert.status = 'acknowledged';
    alert.acknowledged_at = new Date().toISOString();
  } catch (e) {
    console.error('Failed to acknowledge alert:', e);
  }
}

async function resolve(alert: Alert) {
  try {
    await AlertService.resolve(alert.id);
    alert.status = 'resolved';
    alert.resolved_at = new Date().toISOString();
  } catch (e) {
    console.error('Failed to resolve alert:', e);
  }
}

async function loadData() {
  try {
    state.loading = true;
    const [alerts, workspaces] = await Promise.all([
      AlertService.list({ limit: 100 }),
      WorkspaceService.list()
    ]);
    state.alerts = alerts;
    state.workspaces = workspaces;
  } catch (e: any) {
    state.error = e.message || 'Failed to load alerts';
  } finally {
    state.loading = false;
  }
}

onMounted(loadData);
</script>

<template>
  <div class="container-fluid">
    <Title 
      title="Alerts" 
      subtitle="Monitor and manage network alerts across all workspaces"
      :history="[{title: 'Workspaces', link: '/workspaces'}]"
    >
      <button class="btn btn-outline-secondary" @click="loadData" :disabled="state.loading">
        <i class="bi bi-arrow-clockwise me-1"></i> Refresh
      </button>
    </Title>

    <!-- Stats Cards -->
    <div class="row g-3 mb-4">
      <div class="col-md-4">
        <div class="stat-card" :class="{ active: state.filter === 'active' }" @click="state.filter = 'active'">
          <div class="stat-icon bg-danger">
            <i class="bi bi-exclamation-triangle-fill"></i>
          </div>
          <div class="stat-content">
            <h3 class="stat-value">{{ activeCount }}</h3>
            <p class="stat-label">Active Alerts</p>
          </div>
        </div>
      </div>
      <div class="col-md-4">
        <div class="stat-card" :class="{ active: state.filter === 'acknowledged' }" @click="state.filter = 'acknowledged'">
          <div class="stat-icon bg-warning">
            <i class="bi bi-eye-fill"></i>
          </div>
          <div class="stat-content">
            <h3 class="stat-value">{{ acknowledgedCount }}</h3>
            <p class="stat-label">Acknowledged</p>
          </div>
        </div>
      </div>
      <div class="col-md-4">
        <div class="stat-card" :class="{ active: state.filter === 'resolved' }" @click="state.filter = 'resolved'">
          <div class="stat-icon bg-success">
            <i class="bi bi-check-circle-fill"></i>
          </div>
          <div class="stat-content">
            <h3 class="stat-value">{{ resolvedCount }}</h3>
            <p class="stat-label">Resolved</p>
          </div>
        </div>
      </div>
    </div>

    <!-- Filter Tabs -->
    <div class="card mb-3">
      <div class="card-body py-2">
        <div class="btn-group" role="group">
          <button 
            type="button" 
            class="btn" 
            :class="state.filter === 'all' ? 'btn-primary' : 'btn-outline-secondary'"
            @click="state.filter = 'all'"
          >All ({{ state.alerts.length }})</button>
          <button 
            type="button" 
            class="btn" 
            :class="state.filter === 'active' ? 'btn-danger' : 'btn-outline-secondary'"
            @click="state.filter = 'active'"
          >Active ({{ activeCount }})</button>
          <button 
            type="button" 
            class="btn" 
            :class="state.filter === 'acknowledged' ? 'btn-warning' : 'btn-outline-secondary'"
            @click="state.filter = 'acknowledged'"
          >Acknowledged ({{ acknowledgedCount }})</button>
          <button 
            type="button" 
            class="btn" 
            :class="state.filter === 'resolved' ? 'btn-success' : 'btn-outline-secondary'"
            @click="state.filter = 'resolved'"
          >Resolved ({{ resolvedCount }})</button>
        </div>
      </div>
    </div>

    <!-- Loading State -->
    <div v-if="state.loading" class="card">
      <div class="card-body text-center py-5">
        <div class="spinner-border text-primary mb-3"></div>
        <p class="text-muted">Loading alerts...</p>
      </div>
    </div>

    <!-- Error State -->
    <div v-else-if="state.error" class="alert alert-danger">
      <i class="bi bi-exclamation-circle me-2"></i>{{ state.error }}
    </div>

    <!-- Empty State -->
    <div v-else-if="filteredAlerts.length === 0" class="card">
      <div class="card-body text-center py-5">
        <i class="bi bi-bell-slash display-4 text-muted mb-3"></i>
        <h5>No {{ state.filter !== 'all' ? state.filter : '' }} alerts</h5>
        <p class="text-muted">Configure alert rules in workspace settings to start monitoring.</p>
      </div>
    </div>

    <!-- Alerts List -->
    <div v-else class="card">
      <div class="table-responsive">
        <table class="table table-hover mb-0">
          <thead>
            <tr>
              <th>Status</th>
              <th>Severity</th>
              <th>Workspace</th>
              <th>Agent</th>
              <th>Probe</th>
              <th>Target</th>
              <th>Metric</th>
              <th>Value</th>
              <th>Triggered</th>
              <th class="text-end">Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="alert in filteredAlerts" :key="alert.id">
              <td>
                <span class="badge" :class="getStatusClass(alert.status)">
                  {{ alert.status }}
                </span>
              </td>
              <td>
                <span class="badge" :class="getSeverityClass(alert.severity)">
                  {{ alert.severity }}
                </span>
              </td>
              <td>{{ getWorkspaceName(alert.workspace_id) }}</td>
              <td>
                <a 
                  v-if="alert.agent_id" 
                  href="#"
                  class="text-decoration-none agent-link"
                  @click.prevent="navigateToAgent(alert)"
                >
                  <i class="bi bi-hdd-network me-1"></i>
                  {{ alert.agent_name || `Agent ${alert.agent_id}` }}
                </a>
                <span v-else class="text-muted">-</span>
              </td>
              <td>
                <a
                  v-if="alert.probe_id && alert.agent_id"
                  href="#"
                  class="text-decoration-none probe-link"
                  @click.prevent="navigateToProbe(alert)"
                >
                  <span 
                    class="badge me-1" 
                    :class="getProbeTypeClass(alert.probe_type || '')"
                  >{{ alert.probe_type || 'PROBE' }}</span>
                  <span class="probe-name">{{ alert.probe_name || `#${alert.probe_id}` }}</span>
                </a>
                <span v-else class="text-muted">-</span>
              </td>
              <td class="text-truncate target-cell" :title="alert.probe_target">
                <code v-if="alert.probe_target">{{ alert.probe_target }}</code>
                <span v-else class="text-muted">-</span>
              </td>
              <td>{{ formatMetric(alert.metric) }}</td>
              <td>
                <strong>{{ alert.value.toFixed(2) }}</strong>
                <span class="text-muted"> / {{ alert.threshold }}</span>
              </td>
              <td>
                <small>{{ formatTime(alert.triggered_at) }}</small>
              </td>
              <td class="text-end">
                <div class="btn-group btn-group-sm">
                  <button 
                    v-if="alert.status === 'active'"
                    class="btn btn-outline-warning"
                    @click="acknowledge(alert)"
                    title="Acknowledge"
                  >
                    <i class="bi bi-eye"></i>
                  </button>
                  <button 
                    v-if="alert.status !== 'resolved'"
                    class="btn btn-outline-success"
                    @click="resolve(alert)"
                    title="Resolve"
                  >
                    <i class="bi bi-check-lg"></i>
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.stat-card {
  background: var(--bg-card, white);
  border-radius: 0.75rem;
  padding: 1.25rem;
  box-shadow: 0 0.125rem 0.25rem rgba(0, 0, 0, 0.075);
  display: flex;
  align-items: center;
  gap: 1rem;
  cursor: pointer;
  transition: all 0.2s;
  border: 2px solid transparent;
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 0.25rem 0.5rem rgba(0, 0, 0, 0.1);
}

.stat-card.active {
  border-color: var(--bs-primary);
}

.stat-icon {
  width: 50px;
  height: 50px;
  border-radius: 0.75rem;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-size: 1.25rem;
}

.stat-value {
  font-size: 1.5rem;
  font-weight: 700;
  margin-bottom: 0;
}

.stat-label {
  color: var(--text-muted, #6b7280);
  margin-bottom: 0;
  font-size: 0.875rem;
}

.card {
  border: none;
  border-radius: 0.75rem;
  box-shadow: 0 0.125rem 0.25rem rgba(0, 0, 0, 0.075);
}

.table th {
  background-color: var(--bg-subtle, #f8f9fa);
  font-weight: 600;
  text-transform: uppercase;
  font-size: 0.75rem;
  letter-spacing: 0.05em;
  border-bottom: 2px solid var(--border-color, #e9ecef);
}

.agent-link, .probe-link {
  color: var(--bs-primary);
  transition: color 0.15s;
}

.agent-link:hover, .probe-link:hover {
  color: var(--bs-primary-dark, #0056b3);
  text-decoration: underline !important;
}

.probe-name {
  font-size: 0.875rem;
}

.target-cell {
  max-width: 150px;
}

.target-cell code {
  font-size: 0.8rem;
  background: var(--bg-subtle, #f8f9fa);
  padding: 0.15rem 0.35rem;
  border-radius: 0.25rem;
}

.bg-purple {
  background-color: #7c3aed !important;
}

/* Dark mode support */
:global([data-theme="dark"]) .stat-card {
  background: #1f2937;
}

:global([data-theme="dark"]) .card {
  background: #1f2937;
}

:global([data-theme="dark"]) .table th {
  background-color: #374151;
}

:global([data-theme="dark"]) .target-cell code {
  background: #374151;
}
</style>
