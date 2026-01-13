<template>
  <div class="admin-workspaces">
    <div class="page-header">
      <div>
        <router-link to="/admin" class="back-link">
          <i class="bi bi-arrow-left"></i> Back to Admin
        </router-link>
        <h1><i class="bi bi-folder2-open"></i> Workspace Management</h1>
      </div>
      <div class="search-box">
        <input 
          type="text" 
          v-model="searchQuery" 
          @input="debouncedSearch"
          placeholder="Search workspaces..." 
          class="form-control"
        />
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
            <th>Description</th>
            <th>Owner ID</th>
            <th>Created</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="ws in workspaces" :key="ws.id">
            <td>{{ ws.id }}</td>
            <td>
              <router-link :to="`/admin/workspaces/${ws.id}`">{{ ws.name }}</router-link>
            </td>
            <td>{{ ws.description || '-' }}</td>
            <td>{{ ws.owner_id }}</td>
            <td>{{ formatDate(ws.created_at) }}</td>
            <td>
              <div class="action-buttons">
                <router-link :to="`/admin/workspaces/${ws.id}`" class="btn btn-sm btn-outline-primary">
                  <i class="bi bi-eye"></i>
                </router-link>
                <button class="btn btn-sm btn-outline-danger" @click="confirmDelete(ws)">
                  <i class="bi bi-trash"></i>
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>

      <div class="pagination-controls" v-if="workspaces.length >= limit">
        <button class="btn btn-outline-secondary" @click="prevPage" :disabled="offset === 0">
          <i class="bi bi-chevron-left"></i> Previous
        </button>
        <button class="btn btn-outline-secondary" @click="nextPage">
          Next <i class="bi bi-chevron-right"></i>
        </button>
      </div>
    </div>

    <!-- Delete Modal -->
    <div class="modal-overlay" v-if="deletingWorkspace" @click.self="deletingWorkspace = null">
      <div class="modal-content">
        <h3>Delete Workspace</h3>
        <p>Are you sure you want to delete <strong>{{ deletingWorkspace.name }}</strong>?</p>
        <p class="text-danger">This will remove all agents and probes in this workspace.</p>
        <div class="modal-actions">
          <button class="btn btn-secondary" @click="deletingWorkspace = null">Cancel</button>
          <button class="btn btn-danger" @click="doDelete" :disabled="saving">
            {{ saving ? 'Deleting...' : 'Delete' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import * as adminService from '@/services/adminService';
import type { AdminWorkspace } from '@/services/adminService';

const workspaces = ref<AdminWorkspace[]>([]);
const loading = ref(true);
const error = ref('');
const searchQuery = ref('');
const limit = ref(50);
const offset = ref(0);
const deletingWorkspace = ref<AdminWorkspace | null>(null);
const saving = ref(false);

let searchTimeout: number;
const debouncedSearch = () => {
  clearTimeout(searchTimeout);
  searchTimeout = window.setTimeout(() => {
    offset.value = 0;
    loadWorkspaces();
  }, 300);
};

async function loadWorkspaces() {
  loading.value = true;
  error.value = '';
  try {
    const res = await adminService.listWorkspaces(limit.value, offset.value, searchQuery.value);
    workspaces.value = res.data || [];
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load workspaces';
  } finally {
    loading.value = false;
  }
}

function prevPage() {
  offset.value = Math.max(0, offset.value - limit.value);
  loadWorkspaces();
}

function nextPage() {
  offset.value += limit.value;
  loadWorkspaces();
}

function confirmDelete(ws: AdminWorkspace) {
  deletingWorkspace.value = ws;
}

async function doDelete() {
  if (!deletingWorkspace.value) return;
  saving.value = true;
  try {
    await adminService.deleteWorkspace(deletingWorkspace.value.id);
    deletingWorkspace.value = null;
    loadWorkspaces();
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to delete workspace';
  } finally {
    saving.value = false;
  }
}

function formatDate(date: string): string {
  return new Date(date).toLocaleDateString();
}

onMounted(loadWorkspaces);
</script>

<style scoped>
.admin-workspaces {
  max-width: 1400px;
  margin: 0 auto;
  padding: 2rem;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 2rem;
  flex-wrap: wrap;
  gap: 1rem;
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

.search-box { width: 300px; }

.table-container {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  overflow: hidden;
}

.table { color: var(--color-text); margin: 0; }
.table th, .table td { border-color: var(--color-border); vertical-align: middle; }
.action-buttons { display: flex; gap: 0.5rem; }
.pagination-controls { display: flex; justify-content: center; gap: 1rem; padding: 1rem; border-top: 1px solid var(--color-border); }
.loading-state { display: flex; justify-content: center; padding: 3rem; }

.modal-overlay {
  position: fixed; top: 0; left: 0; right: 0; bottom: 0;
  background: rgba(0,0,0,0.5); display: flex; align-items: center; justify-content: center; z-index: 1000;
}

.modal-content {
  background: var(--color-surface); border-radius: 12px; padding: 2rem; width: 100%; max-width: 400px;
}

.modal-content h3 { margin-bottom: 1.5rem; color: var(--color-text); }
.modal-actions { display: flex; justify-content: flex-end; gap: 1rem; margin-top: 1.5rem; }
</style>
