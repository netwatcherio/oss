<template>
  <div class="admin-users">
    <div class="page-header">
      <div>
        <router-link to="/admin" class="back-link">
          <i class="bi bi-arrow-left"></i> Back to Admin
        </router-link>
        <h1><i class="bi bi-people"></i> User Management</h1>
      </div>
      <div class="search-box">
        <input 
          type="text" 
          v-model="searchQuery" 
          @input="debouncedSearch"
          placeholder="Search users..." 
          class="form-control"
        />
      </div>
    </div>

    <!-- Loading -->
    <div class="loading-state" v-if="loading">
      <div class="spinner-border text-primary" role="status"></div>
    </div>

    <!-- Error -->
    <div class="alert alert-danger" v-if="error">{{ error }}</div>

    <!-- Users Table -->
    <div class="table-container" v-if="!loading">
      <table class="table">
        <thead>
          <tr>
            <th>ID</th>
            <th>Email</th>
            <th>Name</th>
            <th>Role</th>
            <th>Verified</th>
            <th>Last Login</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="user in users" :key="user.id">
            <td>{{ user.id }}</td>
            <td>{{ user.email }}</td>
            <td>{{ user.name || '-' }}</td>
            <td>
              <span class="badge" :class="user.role === 'SITE_ADMIN' ? 'bg-danger' : 'bg-secondary'">
                {{ user.role }}
              </span>
            </td>
            <td>
              <i class="bi" :class="user.verified ? 'bi-check-circle-fill text-success' : 'bi-x-circle text-muted'"></i>
            </td>
            <td>{{ formatDate(user.last_login_at) }}</td>
            <td>
              <div class="action-buttons">
                <button class="btn btn-sm btn-outline-primary" @click="editUser(user)">
                  <i class="bi bi-pencil"></i>
                </button>
                <button 
                  class="btn btn-sm" 
                  :class="user.role === 'SITE_ADMIN' ? 'btn-outline-warning' : 'btn-outline-success'"
                  @click="toggleAdmin(user)"
                  :disabled="user.id === currentUserId"
                  :title="user.id === currentUserId ? 'Cannot change own role' : ''"
                >
                  <i class="bi" :class="user.role === 'SITE_ADMIN' ? 'bi-shield-minus' : 'bi-shield-plus'"></i>
                </button>
                <button 
                  class="btn btn-sm btn-outline-danger" 
                  @click="confirmDelete(user)"
                  :disabled="user.id === currentUserId"
                >
                  <i class="bi bi-trash"></i>
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>

      <!-- Pagination -->
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

    <!-- Edit Modal -->
    <div class="modal-overlay" v-if="editingUser" @click.self="editingUser = null">
      <div class="modal-content">
        <h3>Edit User</h3>
        <form @submit.prevent="saveUser">
          <div class="mb-3">
            <label class="form-label">Email</label>
            <input type="email" v-model="editForm.email" class="form-control" required />
          </div>
          <div class="mb-3">
            <label class="form-label">Name</label>
            <input type="text" v-model="editForm.name" class="form-control" />
          </div>
          <div class="form-check mb-3">
            <input type="checkbox" v-model="editForm.verified" class="form-check-input" id="verified" />
            <label class="form-check-label" for="verified">Verified</label>
          </div>
          <div class="modal-actions">
            <button type="button" class="btn btn-secondary" @click="editingUser = null">Cancel</button>
            <button type="submit" class="btn btn-primary" :disabled="saving">
              {{ saving ? 'Saving...' : 'Save' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- Delete Confirmation -->
    <div class="modal-overlay" v-if="deletingUser" @click.self="deletingUser = null">
      <div class="modal-content">
        <h3>Delete User</h3>
        <p>Are you sure you want to delete <strong>{{ deletingUser.email }}</strong>?</p>
        <p class="text-danger">This action cannot be undone.</p>
        <div class="modal-actions">
          <button class="btn btn-secondary" @click="deletingUser = null">Cancel</button>
          <button class="btn btn-danger" @click="doDelete" :disabled="saving">
            {{ saving ? 'Deleting...' : 'Delete' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import * as adminService from '@/services/adminService';
import type { AdminUser } from '@/services/adminService';

const users = ref<AdminUser[]>([]);
const loading = ref(true);
const error = ref('');
const searchQuery = ref('');
const limit = ref(50);
const offset = ref(0);
const total = ref(0);

const editingUser = ref<AdminUser | null>(null);
const deletingUser = ref<AdminUser | null>(null);
const saving = ref(false);
const editForm = ref({ email: '', name: '', verified: false });

// Get current user ID from JWT
const currentUserId = computed(() => {
  try {
    const token = localStorage.getItem('token');
    if (token) {
      const payload = JSON.parse(atob(token.split('.')[1]));
      return payload.uid;
    }
  } catch { /* ignore */ }
  return 0;
});

let searchTimeout: number;
const debouncedSearch = () => {
  clearTimeout(searchTimeout);
  searchTimeout = window.setTimeout(() => {
    offset.value = 0;
    loadUsers();
  }, 300);
};

async function loadUsers() {
  loading.value = true;
  error.value = '';
  try {
    const res = await adminService.listUsers(limit.value, offset.value, searchQuery.value);
    users.value = res.data || [];
    total.value = res.total || 0;
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load users';
  } finally {
    loading.value = false;
  }
}

function prevPage() {
  offset.value = Math.max(0, offset.value - limit.value);
  loadUsers();
}

function nextPage() {
  offset.value += limit.value;
  loadUsers();
}

function editUser(user: AdminUser) {
  editingUser.value = user;
  editForm.value = { email: user.email, name: user.name, verified: user.verified };
}

async function saveUser() {
  if (!editingUser.value) return;
  saving.value = true;
  try {
    await adminService.updateUser(editingUser.value.id, editForm.value);
    editingUser.value = null;
    loadUsers();
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to save user';
  } finally {
    saving.value = false;
  }
}

async function toggleAdmin(user: AdminUser) {
  const newRole = user.role === 'SITE_ADMIN' ? 'USER' : 'SITE_ADMIN';
  try {
    await adminService.setUserRole(user.id, newRole);
    loadUsers();
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to update role';
  }
}

function confirmDelete(user: AdminUser) {
  deletingUser.value = user;
}

async function doDelete() {
  if (!deletingUser.value) return;
  saving.value = true;
  try {
    await adminService.deleteUser(deletingUser.value.id);
    deletingUser.value = null;
    loadUsers();
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to delete user';
  } finally {
    saving.value = false;
  }
}

function formatDate(date: string | null): string {
  if (!date) return 'Never';
  return new Date(date).toLocaleDateString();
}

onMounted(loadUsers);
</script>

<style scoped>
.admin-users {
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

.search-box {
  width: 300px;
}

.table-container {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  overflow: hidden;
}

.table {
  color: var(--color-text);
  margin: 0;
}

.table th, .table td {
  border-color: var(--color-border);
  vertical-align: middle;
}

.action-buttons {
  display: flex;
  gap: 0.5rem;
}

.pagination-controls {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 1rem;
  padding: 1rem;
  border-top: 1px solid var(--color-border);
}

.page-info {
  color: var(--color-text-muted);
}

.loading-state {
  display: flex;
  justify-content: center;
  padding: 3rem;
}

.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0,0,0,0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal-content {
  background: var(--color-surface);
  border-radius: 12px;
  padding: 2rem;
  width: 100%;
  max-width: 400px;
}

.modal-content h3 {
  margin-bottom: 1.5rem;
  color: var(--color-text);
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 1rem;
  margin-top: 1.5rem;
}
</style>
