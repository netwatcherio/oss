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
          <div class="stat-label">Active Connections</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon ws"><i class="bi bi-diagram-3"></i></div>
        <div class="stat-content">
          <div class="stat-value">{{ uniqueWorkspaces }}</div>
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
      <div class="auto-refresh">
        <input type="checkbox" id="autoRefresh" v-model="autoRefresh" />
        <label for="autoRefresh">Auto-refresh (10s)</label>
      </div>
      <span class="last-updated" v-if="lastUpdated">
        Last updated: {{ lastUpdated }}
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

    <!-- Connections Table -->
    <div class="connections-panel" v-if="connections && connections.connections.length > 0">
      <h3><i class="bi bi-plug"></i> Active Agent Connections</h3>
      <div class="table-responsive">
        <table class="table table-hover">
          <thead>
            <tr>
              <th>Agent ID</th>
              <th>Workspace</th>
              <th>Connection ID</th>
              <th>Client IP</th>
              <th>Connected</th>
              <th>Duration</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="conn in sortedConnections" :key="conn.conn_id">
              <td>
                <span class="badge bg-primary">{{ conn.agent_id }}</span>
              </td>
              <td>
                <router-link :to="`/admin/workspaces/${conn.workspace_id}`" class="ws-link">
                  WS {{ conn.workspace_id }}
                </router-link>
              </td>
              <td>
                <code class="conn-id">{{ conn.conn_id.substring(0, 12) }}...</code>
              </td>
              <td>
                <span class="ip-badge">{{ conn.client_ip }}</span>
              </td>
              <td>{{ formatDate(conn.connected_at) }}</td>
              <td>
                <span class="duration">{{ getDuration(conn.connected_at) }}</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Empty State -->
    <div class="empty-state" v-else-if="connections && connections.connections.length === 0">
      <i class="bi bi-plug"></i>
      <p>No active agent connections</p>
    </div>

    <!-- IP Distribution -->
    <div class="ip-distribution" v-if="connections && connections.connections.length > 0">
      <h3><i class="bi bi-geo-alt"></i> Connections by IP</h3>
      <div class="ip-list">
        <div class="ip-row" v-for="(count, ip) in ipCounts" :key="ip">
          <span class="ip">{{ ip }}</span>
          <span class="count badge" :class="count > 1 ? 'bg-warning' : 'bg-secondary'">
            {{ count }} agent{{ count > 1 ? 's' : '' }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import * as adminService from '@/services/adminService';
import type { DebugConnectionsResponse, AgentConnection } from '@/services/adminService';

const connections = ref<DebugConnectionsResponse | null>(null);
const loading = ref(true);
const error = ref('');
const lastUpdated = ref('');
const autoRefresh = ref(false);
let refreshInterval: ReturnType<typeof setInterval> | null = null;

const uniqueWorkspaces = computed(() => {
  if (!connections.value) return 0;
  const wsSet = new Set(connections.value.connections.map(c => c.workspace_id));
  return wsSet.size;
});

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

const ipCounts = computed(() => {
  if (!connections.value) return {};
  const counts: Record<string, number> = {};
  connections.value.connections.forEach(c => {
    counts[c.client_ip] = (counts[c.client_ip] || 0) + 1;
  });
  return counts;
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

// Watch autoRefresh toggle
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

// React to autoRefresh changes
import { watch } from 'vue';
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
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
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
  font-size: 0.85rem;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.table td {
  border-color: var(--color-border);
  vertical-align: middle;
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

.ip-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.ip-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem 0.75rem;
  background: var(--color-surface-elevated);
  border-radius: 6px;
}

.ip-row .ip {
  font-family: monospace;
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
