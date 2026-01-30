<template>
  <div class="system-debug">
    <div class="page-header">
      <h1><i class="bi bi-terminal"></i> System Debug</h1>
      <p class="text-muted">Real-time WebSocket connections and system diagnostics</p>
    </div>

    <!-- Connection Stats Card -->
    <div class="stats-row" v-if="connections">
      <div class="stat-card highlight">
        <div class="stat-icon"><i class="bi bi-broadcast"></i></div>
        <div class="stat-content">
          <div class="stat-value">{{ connections.connected_count }}</div>
          <div class="stat-label">Active Agents</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon ws"><i class="bi bi-folder2-open"></i></div>
        <div class="stat-content">
          <div class="stat-value">{{ connections.workspace_count }}</div>
          <div class="stat-label">Workspaces</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon ip"><i class="bi bi-globe"></i></div>
        <div class="stat-content">
          <div class="stat-value">{{ uniqueIPs }}</div>
          <div class="stat-label">Unique IPs</div>
        </div>
      </div>
    </div>

    <!-- Actions Bar -->
    <div class="actions-bar">
      <button class="btn btn-outline-primary" @click="refresh" :disabled="loading">
        <i class="bi bi-arrow-clockwise" :class="{ 'spin': loading }"></i>
        Refresh
      </button>
      <div class="view-toggle">
        <button 
          class="btn btn-sm" 
          :class="viewMode === 'grouped' ? 'btn-primary' : 'btn-outline-secondary'"
          @click="viewMode = 'grouped'"
        >
          <i class="bi bi-collection"></i> By Workspace
        </button>
        <button 
          class="btn btn-sm" 
          :class="viewMode === 'flat' ? 'btn-primary' : 'btn-outline-secondary'"
          @click="viewMode = 'flat'"
        >
          <i class="bi bi-list"></i> All Connections
        </button>
      </div>
      <div class="auto-refresh">
        <input type="checkbox" id="autoRefresh" v-model="autoRefresh" />
        <label for="autoRefresh">Auto-refresh (10s)</label>
      </div>
      <span class="last-updated" v-if="lastUpdated">
        Updated: {{ lastUpdated }}
      </span>
    </div>

    <!-- Loading State -->
    <div class="loading-state" v-if="loading && !connections">
      <div class="spinner-border text-primary" role="status">
        <span class="visually-hidden">Loading...</span>
      </div>
    </div>

    <!-- Error State -->
    <div class="alert alert-danger" v-if="error">
      <i class="bi bi-exclamation-triangle"></i> {{ error }}
    </div>

    <!-- Grouped View -->
    <div v-if="viewMode === 'grouped' && connections && connections.by_workspace.length > 0">
      <div class="workspace-group" v-for="ws in connections.by_workspace" :key="ws.workspace_id">
        <div class="workspace-header">
          <div class="workspace-info">
            <i class="bi bi-folder2"></i>
            <router-link :to="`/admin/workspaces/${ws.workspace_id}`" class="workspace-name">
              {{ ws.workspace_name || `Workspace ${ws.workspace_id}` }}
            </router-link>
            <span class="badge bg-primary">{{ ws.agent_count }} agent{{ ws.agent_count > 1 ? 's' : '' }}</span>
          </div>
        </div>
        <div class="agent-cards">
          <div class="agent-card" v-for="conn in ws.connections" :key="conn.conn_id">
            <div class="agent-main">
              <div class="agent-name">
                <i class="bi bi-hdd-network"></i>
                {{ conn.agent_name || `Agent ${conn.agent_id}` }}
              </div>
              <span class="agent-id">#{{ conn.agent_id }}</span>
            </div>
            <div class="agent-details">
              <div class="detail">
                <i class="bi bi-globe"></i>
                <span class="ip">{{ conn.client_ip }}</span>
              </div>
              <div class="detail">
                <i class="bi bi-clock"></i>
                <span>{{ getDuration(conn.connected_at) }}</span>
              </div>
              <div class="detail conn-id-detail">
                <i class="bi bi-link-45deg"></i>
                <code>{{ conn.conn_id.substring(0, 8) }}</code>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Flat View -->
    <div class="connections-panel" v-if="viewMode === 'flat' && connections && connections.connections.length > 0">
      <h3><i class="bi bi-plug"></i> All Active Connections</h3>
      <div class="table-responsive">
        <table class="table table-hover">
          <thead>
            <tr>
              <th>Agent</th>
              <th>Workspace</th>
              <th>Client IP</th>
              <th>Connected</th>
              <th>Duration</th>
              <th>Connection ID</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="conn in sortedConnections" :key="conn.conn_id">
              <td>
                <div class="agent-cell">
                  <span class="name">{{ conn.agent_name || 'Unknown' }}</span>
                  <span class="badge bg-secondary">{{ conn.agent_id }}</span>
                </div>
              </td>
              <td>
                <router-link :to="`/admin/workspaces/${conn.workspace_id}`" class="ws-link">
                  {{ conn.workspace_name || `WS ${conn.workspace_id}` }}
                </router-link>
              </td>
              <td>
                <span class="ip-badge">{{ conn.client_ip }}</span>
              </td>
              <td>{{ formatDate(conn.connected_at) }}</td>
              <td>
                <span class="duration">{{ getDuration(conn.connected_at) }}</span>
              </td>
              <td>
                <code class="conn-id">{{ conn.conn_id.substring(0, 12) }}...</code>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Empty State -->
    <div class="empty-state" v-if="connections && connections.connections.length === 0">
      <i class="bi bi-plug"></i>
      <p>No active agent connections</p>
    </div>

    <!-- IP Distribution -->
    <div class="ip-distribution" v-if="connections && connections.connections.length > 0">
      <h3><i class="bi bi-geo-alt"></i> Connections by IP</h3>
      <div class="ip-list">
        <div class="ip-row" v-for="(agents, ip) in ipGroups" :key="ip">
          <div class="ip-info">
            <span class="ip">{{ ip }}</span>
            <span class="badge" :class="agents.length > 1 ? 'bg-warning text-dark' : 'bg-secondary'">
              {{ agents.length }} agent{{ agents.length > 1 ? 's' : '' }}
            </span>
          </div>
          <div class="ip-agents">
            <span v-for="a in agents" :key="a.agent_id" class="agent-tag">
              {{ a.agent_name || `Agent ${a.agent_id}` }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue';
import * as adminService from '@/services/adminService';
import type { DebugConnectionsResponse, AgentConnection } from '@/services/adminService';

const connections = ref<DebugConnectionsResponse | null>(null);
const loading = ref(true);
const error = ref('');
const lastUpdated = ref('');
const autoRefresh = ref(false);
const viewMode = ref<'grouped' | 'flat'>('grouped');
let refreshInterval: ReturnType<typeof setInterval> | null = null;

const uniqueIPs = computed(() => {
  if (!connections.value) return 0;
  const ipSet = new Set(connections.value.connections.map(c => c.client_ip));
  return ipSet.size;
});

const sortedConnections = computed(() => {
  if (!connections.value) return [];
  return [...connections.value.connections].sort((a, b) => 
    new Date(b.connected_at).getTime() - new Date(a.connected_at).getTime()
  );
});

const ipGroups = computed(() => {
  if (!connections.value) return {};
  const groups: Record<string, AgentConnection[]> = {};
  connections.value.connections.forEach(c => {
    if (!groups[c.client_ip]) groups[c.client_ip] = [];
    groups[c.client_ip].push(c);
  });
  return groups;
});

async function refresh() {
  loading.value = true;
  error.value = '';
  try {
    connections.value = await adminService.getDebugConnections();
    lastUpdated.value = new Date().toLocaleTimeString();
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load connections';
  } finally {
    loading.value = false;
  }
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleString();
}

function getDuration(dateStr: string): string {
  const start = new Date(dateStr).getTime();
  const now = Date.now();
  const diff = Math.floor((now - start) / 1000);
  
  if (diff < 60) return `${diff}s`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ${Math.floor((diff % 3600) / 60)}m`;
  return `${Math.floor(diff / 86400)}d`;
}

function startAutoRefresh() {
  if (refreshInterval) clearInterval(refreshInterval);
  refreshInterval = setInterval(refresh, 10000);
}

function stopAutoRefresh() {
  if (refreshInterval) {
    clearInterval(refreshInterval);
    refreshInterval = null;
  }
}

onMounted(() => {
  refresh();
});

onUnmounted(() => {
  stopAutoRefresh();
});

watch(autoRefresh, (val) => {
  if (val) startAutoRefresh();
  else stopAutoRefresh();
});
</script>

<style scoped>
.system-debug {
  max-width: 1400px;
  margin: 0 auto;
  padding: 2rem;
}

.page-header {
  margin-bottom: 2rem;
}

.page-header h1 {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  color: var(--color-text);
}

.stats-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 1rem;
  margin-bottom: 1.5rem;
}

.stat-card {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  padding: 1.25rem;
  display: flex;
  align-items: center;
  gap: 1rem;
}

.stat-card.highlight {
  border-color: var(--color-primary);
  background: var(--color-primary-alpha);
}

.stat-icon {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.25rem;
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
}

.stat-icon.ws { background: rgba(99, 102, 241, 0.15); color: #6366f1; }
.stat-icon.ip { background: rgba(59, 130, 246, 0.15); color: #3b82f6; }

.stat-value {
  font-size: 1.5rem;
  font-weight: 700;
  color: var(--color-text);
}

.stat-label {
  font-size: 0.8rem;
  color: var(--color-text-muted);
}

.actions-bar {
  display: flex;
  align-items: center;
  gap: 1rem;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
}

.view-toggle {
  display: flex;
  gap: 0.25rem;
}

.auto-refresh {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
  color: var(--color-text-muted);
}

.last-updated {
  margin-left: auto;
  font-size: 0.8rem;
  color: var(--color-text-muted);
}

/* Grouped View Styles */
.workspace-group {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  margin-bottom: 1rem;
  overflow: hidden;
}

.workspace-header {
  padding: 1rem 1.25rem;
  background: var(--color-surface-elevated);
  border-bottom: 1px solid var(--color-border);
}

.workspace-info {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.workspace-info i {
  color: var(--color-primary);
  font-size: 1.1rem;
}

.workspace-name {
  font-weight: 600;
  color: var(--color-text);
  text-decoration: none;
}

.workspace-name:hover {
  color: var(--color-primary);
}

.agent-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 0.75rem;
  padding: 1rem;
}

.agent-card {
  background: var(--color-surface-elevated);
  border: 1px solid var(--color-border);
  border-radius: 8px;
  padding: 0.875rem;
}

.agent-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.5rem;
}

.agent-name {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 500;
  color: var(--color-text);
}

.agent-name i {
  color: var(--color-primary);
}

.agent-id {
  font-size: 0.75rem;
  color: var(--color-text-muted);
}

.agent-details {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  font-size: 0.8rem;
  color: var(--color-text-muted);
}

.detail {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

.detail .ip {
  font-family: monospace;
}

.conn-id-detail code {
  font-size: 0.7rem;
  background: var(--color-surface);
  padding: 0.15rem 0.35rem;
  border-radius: 3px;
}

/* Flat View Styles */
.connections-panel, .ip-distribution {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  padding: 1.5rem;
  margin-bottom: 1.5rem;
}

.connections-panel h3, .ip-distribution h3 {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 1rem;
  font-size: 1.1rem;
  color: var(--color-text);
}

.table {
  color: var(--color-text);
  margin-bottom: 0;
}

.table th {
  border-color: var(--color-border);
  font-weight: 600;
  font-size: 0.8rem;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.table td {
  border-color: var(--color-border);
  vertical-align: middle;
}

.agent-cell {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.agent-cell .name {
  font-weight: 500;
}

.conn-id {
  font-size: 0.75rem;
  background: var(--color-surface-elevated);
  padding: 0.2rem 0.4rem;
  border-radius: 4px;
}

.ip-badge {
  font-family: monospace;
  font-size: 0.85rem;
}

.ws-link {
  color: var(--color-primary);
  text-decoration: none;
}

.ws-link:hover {
  text-decoration: underline;
}

.duration {
  color: var(--color-text-muted);
  font-size: 0.85rem;
}

/* IP Distribution */
.ip-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.ip-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 1rem;
  background: var(--color-surface-elevated);
  border-radius: 6px;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.ip-info {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.ip-info .ip {
  font-family: monospace;
  font-weight: 500;
}

.ip-agents {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
}

.agent-tag {
  font-size: 0.75rem;
  padding: 0.2rem 0.5rem;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 4px;
}

.empty-state {
  text-align: center;
  padding: 3rem;
  color: var(--color-text-muted);
}

.empty-state i {
  font-size: 3rem;
  margin-bottom: 1rem;
  display: block;
}

.loading-state {
  display: flex;
  justify-content: center;
  padding: 3rem;
}

.spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
