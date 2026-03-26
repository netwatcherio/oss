<template>
  <div class="admin-global-agents">
    <div class="page-header">
      <div>
        <router-link to="/admin" class="back-link">
          <i class="bi bi-arrow-left"></i> Back to Admin
        </router-link>
        <h1><i class="bi bi-globe2"></i> Global Agents</h1>
        <p class="text-muted">Agents visible to all workspaces for cross-workspace monitoring</p>
      </div>
    </div>

    <div class="loading-state" v-if="loading">
      <div class="spinner-border text-primary" role="status"></div>
    </div>

    <div class="alert alert-danger" v-if="error">{{ error }}</div>

    <!-- Active Global Agents -->
    <div class="section" v-if="!loading">
      <div class="section-header">
        <h3><i class="bi bi-broadcast"></i> Active Global Agents ({{ globalAgents.length }})</h3>
      </div>

      <div class="empty-state" v-if="globalAgents.length === 0">
        <i class="bi bi-globe"></i>
        <p>No global agents configured yet</p>
        <p class="text-muted small">Mark an agent as global from the agents list below to make it available to all workspaces</p>
      </div>

      <div class="table-container" v-else>
        <table class="table" id="global-agents-table">
          <thead>
            <tr>
              <th>Agent</th>
              <th>Home Workspace</th>
              <th>Status</th>
              <th>Bidirectional</th>
              <th>Cross-WS Probes</th>
              <th>Version</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="agent in globalAgents" :key="agent.id">
              <td>
                <div class="agent-name">
                  <i class="bi bi-globe2 global-icon"></i>
                  <div>
                    <strong>{{ agent.name }}</strong>
                    <small class="text-muted d-block">ID: {{ agent.id }}</small>
                  </div>
                </div>
              </td>
              <td>
                <router-link :to="`/admin/workspaces/${agent.workspace_id}`">
                  {{ agent.workspace_name }}
                </router-link>
              </td>
              <td>
                <span class="status-badge" :class="agent.is_online ? 'online' : 'offline'">
                  <i class="bi" :class="agent.is_online ? 'bi-circle-fill' : 'bi-circle'"></i>
                  {{ agent.is_online ? 'Online' : 'Offline' }}
                </span>
              </td>
              <td>
                <label class="toggle-switch" :title="agent.bidirectional_default ? 'Auto-creates reverse probes' : 'One-way only'">
                  <input
                    type="checkbox"
                    :checked="agent.bidirectional_default"
                    @change="toggleBidirectional(agent)"
                    :id="`bidir-toggle-${agent.id}`"
                  />
                  <span class="slider"></span>
                </label>
              </td>
              <td>
                <span class="badge probe-count" :class="agent.cross_workspace_probes > 0 ? 'has-probes' : ''">
                  {{ agent.cross_workspace_probes }}
                </span>
              </td>
              <td><code>{{ agent.version || '-' }}</code></td>
              <td>
                <button
                  class="btn btn-sm btn-outline-danger"
                  @click="removeGlobal(agent)"
                  :disabled="saving"
                  :id="`remove-global-${agent.id}`"
                >
                  <i class="bi bi-globe-americas"></i> Remove Global
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- All Agents (for promoting to global) -->
    <div class="section" v-if="!loading">
      <div class="section-header">
        <h3><i class="bi bi-hdd-network"></i> All Agents</h3>
        <p class="text-muted small">Select agents to promote to global status</p>
      </div>

      <div class="table-container">
        <table class="table" id="all-agents-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>Name</th>
              <th>Workspace</th>
              <th>Status</th>
              <th>Version</th>
              <th>Global</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="agent in allAgents" :key="agent.id">
              <td>{{ agent.id }}</td>
              <td>
                <span class="agent-name-inline">
                  {{ agent.name }}
                  <i v-if="agent.is_online" class="bi bi-globe2 global-icon ms-1" title="Global Agent" style="display:none" :style="isAgentGlobal(agent.id) ? 'display:inline !important' : ''"></i>
                </span>
              </td>
              <td>
                <router-link :to="`/admin/workspaces/${agent.workspace_id}`">
                  {{ agent.workspace_name }}
                </router-link>
              </td>
              <td>
                <span class="status-badge" :class="agent.is_online ? 'online' : 'offline'">
                  <i class="bi" :class="agent.is_online ? 'bi-circle-fill' : 'bi-circle'"></i>
                  {{ agent.is_online ? 'Online' : 'Offline' }}
                </span>
              </td>
              <td><code>{{ agent.version || '-' }}</code></td>
              <td>
                <label class="toggle-switch">
                  <input
                    type="checkbox"
                    :checked="isAgentGlobal(agent.id)"
                    @change="toggleGlobal(agent)"
                    :disabled="saving"
                    :id="`global-toggle-${agent.id}`"
                  />
                  <span class="slider"></span>
                </label>
              </td>
            </tr>
          </tbody>
        </table>

        <div class="pagination-controls" v-if="totalAgents > agentLimit">
          <button class="btn btn-outline-secondary btn-sm" @click="prevPage" :disabled="agentOffset === 0">
            <i class="bi bi-chevron-left"></i> Previous
          </button>
          <span class="page-info">{{ agentOffset + 1 }}-{{ Math.min(agentOffset + agentLimit, totalAgents) }} of {{ totalAgents }}</span>
          <button class="btn btn-outline-secondary btn-sm" @click="nextPage" :disabled="agentOffset + agentLimit >= totalAgents">
            Next <i class="bi bi-chevron-right"></i>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import * as adminService from '@/services/adminService';
import type { GlobalAgentInfo, AdminAgent } from '@/services/adminService';

const globalAgents = ref<GlobalAgentInfo[]>([]);
const allAgents = ref<AdminAgent[]>([]);
const loading = ref(true);
const saving = ref(false);
const error = ref('');
const agentLimit = ref(50);
const agentOffset = ref(0);
const totalAgents = ref(0);

const globalAgentIds = computed(() => new Set(globalAgents.value.map(a => a.id)));

function isAgentGlobal(id: number): boolean {
  return globalAgentIds.value.has(id);
}

async function loadData() {
  loading.value = true;
  error.value = '';
  try {
    const [globalRes, allRes] = await Promise.all([
      adminService.listGlobalAgents(),
      adminService.listAgents(agentLimit.value, agentOffset.value),
    ]);
    globalAgents.value = globalRes.data || [];
    allAgents.value = allRes.data || [];
    totalAgents.value = allRes.total || 0;
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load data';
  } finally {
    loading.value = false;
  }
}

async function toggleGlobal(agent: AdminAgent) {
  saving.value = true;
  try {
    const isCurrentlyGlobal = isAgentGlobal(agent.id);
    await adminService.setAgentGlobalStatus(agent.id, !isCurrentlyGlobal, true);
    await loadData();
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to update agent';
  } finally {
    saving.value = false;
  }
}

async function removeGlobal(agent: GlobalAgentInfo) {
  saving.value = true;
  try {
    await adminService.setAgentGlobalStatus(agent.id, false, agent.bidirectional_default);
    await loadData();
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to update agent';
  } finally {
    saving.value = false;
  }
}

async function toggleBidirectional(agent: GlobalAgentInfo) {
  saving.value = true;
  try {
    await adminService.setAgentGlobalStatus(agent.id, true, !agent.bidirectional_default);
    await loadData();
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to update agent';
  } finally {
    saving.value = false;
  }
}

function prevPage() {
  agentOffset.value = Math.max(0, agentOffset.value - agentLimit.value);
  loadData();
}

function nextPage() {
  agentOffset.value += agentLimit.value;
  loadData();
}

onMounted(loadData);
</script>

<style scoped>
.admin-global-agents {
  max-width: 1400px;
  margin: 0 auto;
  padding: 2rem;
}

.page-header {
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

.page-header h1 i { color: #8b5cf6; }

.section {
  margin-bottom: 2.5rem;
}

.section-header {
  margin-bottom: 1rem;
}

.section-header h3 {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: var(--color-text);
  margin: 0 0 0.25rem 0;
  font-size: 1.1rem;
}

.table-container {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  overflow: hidden;
}

.table { color: var(--color-text); margin: 0; }
.table th, .table td { border-color: var(--color-border); vertical-align: middle; }

.agent-name {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.global-icon { color: #8b5cf6; font-size: 1.1rem; }

.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.25rem 0.75rem;
  border-radius: 999px;
  font-size: 0.75rem;
  font-weight: 500;
}

.status-badge.online { background: rgba(16, 185, 129, 0.15); color: #10b981; }
.status-badge.offline { background: rgba(107, 114, 128, 0.15); color: #6b7280; }
.status-badge i { font-size: 0.5rem; }

.probe-count {
  background: rgba(107, 114, 128, 0.15);
  color: var(--color-text-muted);
  padding: 0.25rem 0.5rem;
  border-radius: 6px;
  font-size: 0.8rem;
}

.probe-count.has-probes {
  background: rgba(139, 92, 246, 0.15);
  color: #8b5cf6;
  font-weight: 600;
}

/* Toggle switch */
.toggle-switch {
  position: relative;
  display: inline-block;
  width: 40px;
  height: 22px;
  cursor: pointer;
}

.toggle-switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.slider {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: rgba(107, 114, 128, 0.3);
  border-radius: 22px;
  transition: 0.3s;
}

.slider::before {
  position: absolute;
  content: "";
  height: 16px;
  width: 16px;
  left: 3px;
  bottom: 3px;
  background-color: white;
  border-radius: 50%;
  transition: 0.3s;
}

input:checked + .slider {
  background-color: #8b5cf6;
}

input:checked + .slider::before {
  transform: translateX(18px);
}

.empty-state {
  text-align: center;
  padding: 3rem 2rem;
  background: var(--color-surface);
  border: 1px dashed var(--color-border);
  border-radius: 12px;
}

.empty-state i {
  font-size: 3rem;
  color: var(--color-text-muted);
  opacity: 0.5;
  margin-bottom: 1rem;
  display: block;
}

.pagination-controls {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 1rem;
  padding: 1rem;
  border-top: 1px solid var(--color-border);
}

.page-info { color: var(--color-text-muted); font-size: 0.875rem; }
.loading-state { display: flex; justify-content: center; padding: 3rem; }
</style>
