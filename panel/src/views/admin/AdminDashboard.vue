<template>
  <div class="admin-dashboard">
    <div class="page-header">
      <h1><i class="bi bi-shield-lock"></i> Site Administration</h1>
      <p class="text-muted">System overview and management</p>
    </div>

    <!-- Stats Cards -->
    <div class="stats-grid" v-if="stats">
      <div class="stat-card">
        <div class="stat-icon users"><i class="bi bi-people"></i></div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.total_users }}</div>
          <div class="stat-label">Total Users</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon workspaces"><i class="bi bi-folder2-open"></i></div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.total_workspaces }}</div>
          <div class="stat-label">Workspaces</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon agents"><i class="bi bi-hdd-network"></i></div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.total_agents }}</div>
          <div class="stat-label">Total Agents</div>
        </div>
      </div>
      <div class="stat-card active">
        <div class="stat-icon active"><i class="bi bi-activity"></i></div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.active_agents }}</div>
          <div class="stat-label">Active Agents</div>
        </div>
      </div>
    </div>

    <!-- Loading State -->
    <div class="loading-state" v-else-if="loading">
      <div class="spinner-border text-primary" role="status">
        <span class="visually-hidden">Loading...</span>
      </div>
    </div>

    <!-- Error State -->
    <div class="alert alert-danger" v-if="error">
      {{ error }}
    </div>

    <!-- Quick Links -->
    <div class="quick-links">
      <h3>Quick Actions</h3>
      <div class="link-grid">
        <router-link to="/admin/users" class="quick-link">
          <i class="bi bi-people-fill"></i>
          <span>Manage Users</span>
        </router-link>
        <router-link to="/admin/workspaces" class="quick-link">
          <i class="bi bi-folder-fill"></i>
          <span>Manage Workspaces</span>
        </router-link>
        <router-link to="/admin/agents" class="quick-link">
          <i class="bi bi-hdd-network-fill"></i>
          <span>View All Agents</span>
        </router-link>
      </div>
    </div>

    <!-- Workspace Stats Table -->
    <div class="workspace-stats" v-if="workspaceStats.length > 0">
      <h3>Workspace Overview</h3>
      <table class="table">
        <thead>
          <tr>
            <th>Workspace</th>
            <th>Members</th>
            <th>Agents</th>
            <th>Active</th>
            <th>Probes</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="ws in workspaceStats" :key="ws.workspace_id">
            <td>
              <router-link :to="`/admin/workspaces/${ws.workspace_id}`">
                {{ ws.workspace_name }}
              </router-link>
            </td>
            <td>{{ ws.member_count }}</td>
            <td>{{ ws.agent_count }}</td>
            <td>
              <span class="badge" :class="ws.active_agents > 0 ? 'bg-success' : 'bg-secondary'">
                {{ ws.active_agents }}
              </span>
            </td>
            <td>{{ ws.probe_count }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import * as adminService from '@/services/adminService';
import type { AdminStats, WorkspaceStats } from '@/services/adminService';

const stats = ref<AdminStats | null>(null);
const workspaceStats = ref<WorkspaceStats[]>([]);
const loading = ref(true);
const error = ref('');

onMounted(async () => {
  try {
    const [statsRes, wsStatsRes] = await Promise.all([
      adminService.getStats(),
      adminService.getWorkspaceStats()
    ]);
    stats.value = statsRes;
    workspaceStats.value = wsStatsRes.data || [];
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load stats';
  } finally {
    loading.value = false;
  }
});
</script>

<style scoped>
.admin-dashboard {
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

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 1.5rem;
  margin-bottom: 2rem;
}

.stat-card {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  padding: 1.5rem;
  display: flex;
  align-items: center;
  gap: 1rem;
  transition: transform 0.2s, box-shadow 0.2s;
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0,0,0,0.1);
}

.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.5rem;
}

.stat-icon.users { background: rgba(99, 102, 241, 0.15); color: #6366f1; }
.stat-icon.workspaces { background: rgba(34, 197, 94, 0.15); color: #22c55e; }
.stat-icon.agents { background: rgba(59, 130, 246, 0.15); color: #3b82f6; }
.stat-icon.active { background: rgba(16, 185, 129, 0.15); color: #10b981; }

.stat-value {
  font-size: 1.75rem;
  font-weight: 700;
  color: var(--color-text);
}

.stat-label {
  font-size: 0.875rem;
  color: var(--color-text-muted);
}

.quick-links {
  margin-bottom: 2rem;
}

.quick-links h3 {
  margin-bottom: 1rem;
  color: var(--color-text);
}

.link-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 1rem;
}

.quick-link {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 1.5rem;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  text-decoration: none;
  color: var(--color-text);
  transition: all 0.2s;
}

.quick-link:hover {
  border-color: var(--color-primary);
  background: var(--color-primary-alpha);
}

.quick-link i {
  font-size: 2rem;
  color: var(--color-primary);
}

.workspace-stats {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  padding: 1.5rem;
}

.workspace-stats h3 {
  margin-bottom: 1rem;
  color: var(--color-text);
}

.table {
  color: var(--color-text);
}

.table th {
  border-color: var(--color-border);
  font-weight: 600;
}

.table td {
  border-color: var(--color-border);
}

.loading-state {
  display: flex;
  justify-content: center;
  padding: 3rem;
}
</style>
