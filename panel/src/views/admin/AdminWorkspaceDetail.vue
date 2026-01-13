<template>
  <div class="admin-workspace-detail">
    <div class="page-header">
      <div>
        <router-link to="/admin/workspaces" class="back-link">
          <i class="bi bi-arrow-left"></i> Back to Workspaces
        </router-link>
        <h1 v-if="workspace">
          <i class="bi bi-folder2-open"></i> 
          {{ workspace.name }}
        </h1>
      </div>
    </div>

    <div class="loading-state" v-if="loading">
      <div class="spinner-border text-primary" role="status"></div>
    </div>

    <div class="alert alert-danger" v-if="error">{{ error }}</div>

    <div class="detail-grid" v-if="workspace && !loading">
      <!-- Workspace Info -->
      <div class="info-card">
        <h3>Workspace Details</h3>
        <dl>
          <dt>ID</dt><dd>{{ workspace.id }}</dd>
          <dt>Name</dt><dd>{{ workspace.name }}</dd>
          <dt>Description</dt><dd>{{ workspace.description || '-' }}</dd>
          <dt>Owner ID</dt><dd>{{ workspace.owner_id }}</dd>
          <dt>Created</dt><dd>{{ formatDate(workspace.created_at) }}</dd>
        </dl>
      </div>

      <!-- Members -->
      <div class="info-card">
        <h3>Members ({{ members.length }})</h3>
        <table class="table table-sm" v-if="members.length">
          <thead>
            <tr>
              <th>User ID</th>
              <th>Email</th>
              <th>Role</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="m in members" :key="m.id">
              <td>{{ m.user_id || '-' }}</td>
              <td>{{ m.email }}</td>
              <td><span class="badge bg-secondary">{{ m.role }}</span></td>
            </tr>
          </tbody>
        </table>
        <p class="text-muted" v-else>No members</p>
      </div>

      <!-- Agents -->
      <div class="info-card full-width">
        <h3>Agents ({{ agents.length }})</h3>
        <table class="table table-sm" v-if="agents.length">
          <thead>
            <tr>
              <th>ID</th>
              <th>Name</th>
              <th>Version</th>
              <th>Status</th>
              <th>Last Seen</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="agent in agents" :key="agent.id">
              <td>{{ agent.id }}</td>
              <td>{{ agent.name }}</td>
              <td><code>{{ agent.version || '-' }}</code></td>
              <td>
                <span class="status-badge" :class="isOnline(agent) ? 'online' : 'offline'">
                  {{ isOnline(agent) ? 'Online' : 'Offline' }}
                </span>
              </td>
              <td>{{ formatRelativeTime(agent.last_seen_at) }}</td>
            </tr>
          </tbody>
        </table>
        <p class="text-muted" v-else>No agents</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useRoute } from 'vue-router';
import * as adminService from '@/services/adminService';

const route = useRoute();
const workspace = ref<any>(null);
const members = ref<any[]>([]);
const agents = ref<any[]>([]);
const loading = ref(true);
const error = ref('');

async function loadWorkspace() {
  loading.value = true;
  error.value = '';
  try {
    const id = Number(route.params.wID);
    const res = await adminService.getWorkspace(id);
    workspace.value = res.workspace;
    members.value = res.members || [];
    agents.value = res.agents || [];
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load workspace';
  } finally {
    loading.value = false;
  }
}

function isOnline(agent: any): boolean {
  if (!agent.last_seen_at) return false;
  const diff = (Date.now() - new Date(agent.last_seen_at).getTime()) / 1000;
  return diff < 300; // 5 minutes
}

function formatDate(date: string): string {
  return new Date(date).toLocaleDateString();
}

function formatRelativeTime(date: string): string {
  if (!date) return 'Never';
  const d = new Date(date);
  const diff = (Date.now() - d.getTime()) / 1000;
  if (diff < 60) return 'Just now';
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return d.toLocaleDateString();
}

onMounted(loadWorkspace);
</script>

<style scoped>
.admin-workspace-detail {
  max-width: 1400px;
  margin: 0 auto;
  padding: 2rem;
}

.page-header { margin-bottom: 2rem; }

.back-link {
  font-size: 0.875rem;
  color: var(--color-text-muted);
  text-decoration: none;
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  margin-bottom: 0.5rem;
}

.page-header h1 {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  color: var(--color-text);
  margin: 0;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(350px, 1fr));
  gap: 1.5rem;
}

.info-card {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  padding: 1.5rem;
}

.info-card.full-width { grid-column: 1 / -1; }

.info-card h3 {
  margin-bottom: 1rem;
  color: var(--color-text);
  font-size: 1.1rem;
}

.info-card dl {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 0.5rem 1rem;
  margin: 0;
}

.info-card dt { color: var(--color-text-muted); font-weight: 500; }
.info-card dd { margin: 0; color: var(--color-text); }

.table { color: var(--color-text); margin: 0; }
.table th, .table td { border-color: var(--color-border); }

.status-badge {
  display: inline-block;
  padding: 0.2rem 0.5rem;
  border-radius: 4px;
  font-size: 0.75rem;
}

.status-badge.online { background: rgba(16, 185, 129, 0.15); color: #10b981; }
.status-badge.offline { background: rgba(107, 114, 128, 0.15); color: #6b7280; }

.loading-state { display: flex; justify-content: center; padding: 3rem; }
</style>
