<template>
  <div class="admin-agents">
    <div class="page-header">
      <div>
        <router-link to="/admin" class="back-link">
          <i class="bi bi-arrow-left"></i> Back to Admin
        </router-link>
        <h1><i class="bi bi-hdd-network"></i> All Agents</h1>
      </div>
    </div>

    <div class="loading-state" v-if="loading">
      <div class="spinner-border text-primary" role="status"></div>
    </div>

    <div class="alert alert-danger" v-if="error">{{ error }}</div>

    <div class="table-container" v-if="!loading">
      <table class="table">
        <thead>
          <tr>
            <th>ID</th>
            <th>Name</th>
            <th>Workspace</th>
            <th>Version</th>
            <th>Location</th>
            <th>Status</th>
            <th>Last Seen</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="agent in agents" :key="agent.id">
            <td>{{ agent.id }}</td>
            <td>{{ agent.name }}</td>
            <td>
              <router-link :to="`/admin/workspaces/${agent.workspace_id}`">
                {{ agent.workspace_name }}
              </router-link>
            </td>
            <td><code>{{ agent.version || '-' }}</code></td>
            <td>{{ agent.location || '-' }}</td>
            <td>
              <span class="status-badge" :class="agent.is_online ? 'online' : 'offline'">
                <i class="bi" :class="agent.is_online ? 'bi-circle-fill' : 'bi-circle'"></i>
                {{ agent.is_online ? 'Online' : 'Offline' }}
              </span>
            </td>
            <td>{{ formatRelativeTime(agent.last_seen_at) }}</td>
          </tr>
        </tbody>
      </table>

      <div class="pagination-controls" v-if="total > limit">
        <button class="btn btn-outline-secondary" @click="prevPage" :disabled="offset === 0">
          <i class="bi bi-chevron-left"></i> Previous
        </button>
        <span class="page-info">{{ offset + 1 }}-{{ Math.min(offset + limit, total) }} of {{ total }}</span>
        <button class="btn btn-outline-secondary" @click="nextPage" :disabled="offset + limit >= total">
          Next <i class="bi bi-chevron-right"></i>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import * as adminService from '@/services/adminService';
import type { AdminAgent } from '@/services/adminService';

const agents = ref<AdminAgent[]>([]);
const loading = ref(true);
const error = ref('');
const limit = ref(50);
const offset = ref(0);
const total = ref(0);

async function loadAgents() {
  loading.value = true;
  error.value = '';
  try {
    const res = await adminService.listAgents(limit.value, offset.value);
    agents.value = res.data || [];
    total.value = res.total || 0;
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load agents';
  } finally {
    loading.value = false;
  }
}

function prevPage() {
  offset.value = Math.max(0, offset.value - limit.value);
  loadAgents();
}

function nextPage() {
  offset.value += limit.value;
  loadAgents();
}

function formatRelativeTime(date: string): string {
  const d = new Date(date);
  const now = new Date();
  const diff = (now.getTime() - d.getTime()) / 1000;
  
  if (diff < 60) return 'Just now';
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return d.toLocaleDateString();
}

onMounted(loadAgents);
</script>

<style scoped>
.admin-agents {
  max-width: 1400px;
  margin: 0 auto;
  padding: 2rem;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 2rem;
}

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

.table-container {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  overflow: hidden;
}

.table { color: var(--color-text); margin: 0; }
.table th, .table td { border-color: var(--color-border); vertical-align: middle; }

.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.25rem 0.75rem;
  border-radius: 999px;
  font-size: 0.75rem;
  font-weight: 500;
}

.status-badge.online {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
}

.status-badge.offline {
  background: rgba(107, 114, 128, 0.15);
  color: #6b7280;
}

.status-badge i { font-size: 0.5rem; }

.pagination-controls {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 1rem;
  padding: 1rem;
  border-top: 1px solid var(--color-border);
}

.page-info { color: var(--color-text-muted); }
.loading-state { display: flex; justify-content: center; padding: 3rem; }
</style>
