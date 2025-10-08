<script lang="ts" setup>
import type { Workspace } from "@/services/apiService";
import { onMounted, reactive, computed } from "vue";
import Title from "@/components/Title.vue";
import Element from "@/components/Element.vue";
import {WorkspaceService} from "@/services/apiService";

declare interface AgentCountInfo {
  site_id: string;
  count: number;
}

declare interface sitesList {
  sites: Workspace[];
}

const state = reactive({
  workspaces: [] as Workspace[],
  agent_counts: [] as AgentCountInfo[],
  ready: false,
  loading: true,
  error: null as string | null,
  searchQuery: "",
  sortBy: "name" as "name" | "location" | "members",
  sortOrder: "asc" as "asc" | "desc"
});

// Computed properties
const filteredSites = computed(() => {
  let filtered = state.workspaces.filter(site => {
    const query = state.searchQuery.toLowerCase();
    return (
      site.name.toLowerCase().includes(query) ||
      site.description?.toLowerCase().includes(query)
    );
  });

  // Sort sites
  filtered.sort((a, b) => {
    let aVal: any, bVal: any;

    switch (state.sortBy) {
      case "name":
        aVal = a.name.toLowerCase();
        bVal = b.name.toLowerCase();
        break;
      /*case "location":
        aVal = a.location?.toLowerCase() || "";
        bVal = b.location?.toLowerCase() || "";
        break;
      case "members":
        aVal = a.members?.length || 0;
        bVal = b.members?.length || 0;
        break;*/
    }

    if (state.sortOrder === "asc") {
      return aVal > bVal ? 1 : -1;
    } else {
      return aVal < bVal ? 1 : -1;
    }
  });

  return filtered;
});

const totalMembers = computed(() => {
  return state.workspaces.reduce((total, site) => total + (69 || 0), 0); // todo
});

// Functions
function toggleSort(column: "name" | "location" | "members") {
  if (state.sortBy === column) {
    state.sortOrder = state.sortOrder === "asc" ? "desc" : "asc";
  } else {
    state.sortBy = column;
    state.sortOrder = "asc";
  }
}

function getSortIcon(column: string) {
  if (state.sortBy !== column) return "fa-sort";
  return state.sortOrder === "asc" ? "fa-sort-up" : "fa-sort-down";
}

onMounted(async () => {
  try {
    const res = await WorkspaceService.list()
    const data = res as Workspace[];

    if (data && data.length > 0) {
      state.workspaces = data;
      state.ready = true;
    }
  } catch (error) {
    console.error("Failed to load workspace:", error);
    state.error = "Failed to load workspace. Please try again.";
  } finally {
    state.loading = false;
  }
});
</script>

<template>
  <div class="container-fluid">
    <Title
      title="workspaces"
      subtitle="an overview of the workspaces you have access to"
    >
      <div class="d-flex gap-2">
        <router-link
          to="/workspaces/alerts"
          class="btn btn-outline-danger"
        >
          <i class="fa-solid fa-exclamation-triangle me-2"></i>View Alerts
        </router-link>
        <router-link
          to="/workspaces/new"
          class="btn btn-primary"
        >
          <i class="fa-solid fa-plus-circle me-2"></i>Create Workspace
        </router-link>
      </div>
    </Title>

    <!-- Loading State -->
    <div v-if="state.loading" class="row">
      <div class="col-12">
        <div class="card">
          <div class="card-body text-center py-5">
            <div class="spinner-border text-primary mb-3" role="status">
              <span class="visually-hidden">Loading...</span>
            </div>
            <p class="text-muted mb-0">Loading workspaces...</p>
          </div>
        </div>
      </div>
    </div>

    <!-- Error State -->
    <div v-else-if="state.error" class="row">
      <div class="col-12">
        <div class="alert alert-danger d-flex align-items-center" role="alert">
          <i class="fas fa-exclamation-circle me-2"></i>
          <div>{{ state.error }}</div>
        </div>
      </div>
    </div>

    <!-- Workspaces List -->
    <div v-else-if="state.ready && state.workspaces.length > 0" class="row">
      <!-- Stats Cards -->
      <div class="col-12 mb-4">
        <div class="row g-3">
          <div class="col-md-4">
            <div class="stat-card">
              <div class="stat-icon bg-primary bg-gradient">
                <i class="fas fa-building"></i>
              </div>
              <div class="stat-content">
                <h3 class="stat-value">{{ state.workspaces.length }}</h3>
                <p class="stat-label">Total Workspaces</p>
              </div>
            </div>
          </div>
          <div class="col-md-4">
            <div class="stat-card">
              <div class="stat-icon bg-success bg-gradient">
                <i class="fas fa-users"></i>
              </div>
              <div class="stat-content">
                <h3 class="stat-value">{{ totalMembers }}</h3>
                <p class="stat-label">Total Members</p>
              </div>
            </div>
          </div>
          <div class="col-md-4">
            <div class="stat-card">
              <div class="stat-icon bg-info bg-gradient">
                <i class="fas fa-network-wired"></i>
              </div>
              <div class="stat-content">
                <h3 class="stat-value">Active</h3>
                <p class="stat-label">Status</p>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Search and Filter -->
      <div class="col-12 mb-3">
        <div class="card">
          <div class="card-body">
            <div class="row align-items-center">
              <div class="col-md-6">
                <div class="input-group">
                  <span class="input-group-text bg-light border-end-0">
                    <i class="fas fa-search text-muted"></i>
                  </span>
                  <input
                    v-model="state.searchQuery"
                    type="text"
                    class="form-control border-start-0 ps-0"
                    placeholder="Search workspaces by name, description, or location..."
                  >
                </div>
              </div>
              <div class="col-md-6 text-md-end mt-3 mt-md-0">
                <span class="text-muted">
                  Showing {{ filteredSites.length }} of {{ state.workspaces.length }} workspaces
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Workspaces Table -->
      <div class="col-12">
        <div class="card">
          <div class="card-body p-0">
            <div class="table-responsive">
              <table class="table table-hover mb-0">
                <thead>
                  <tr>
                    <th class="sortable" @click="toggleSort('name')">
                      <div class="d-flex align-items-center">
                        <span>Name</span>
                        <i :class="`fas ${getSortIcon('name')} ms-2 text-muted`"></i>
                      </div>
                    </th>
                    <th>Description</th>
                    <th class="sortable" @click="toggleSort('location')">
                      <div class="d-flex align-items-center">
                        <span>Location</span>
                        <i :class="`fas ${getSortIcon('location')} ms-2 text-muted`"></i>
                      </div>
                    </th>
                    <th class="sortable text-center" @click="toggleSort('members')">
                      <div class="d-flex align-items-center justify-content-center">
                        <span>Members</span>
                        <i :class="`fas ${getSortIcon('members')} ms-2 text-muted`"></i>
                      </div>
                    </th>
                    <th>Status</th>
                    <th class="text-end">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="site in filteredSites"
                    :key="site.id"
                    class="workspace-row"
                  >
                    <td>
                      <div class="d-flex align-items-center">
                        <div class="workspace-icon me-3">
                          <i class="fas fa-building"></i>
                        </div>
                        <div>
                          <router-link
                            :to="`/workspaces/${site.id}`"
                            class="text-decoration-none fw-semibold text-dark"
                          >
                            {{ site.name }}
                          </router-link>
                          <div class="text-muted small">
                            ID: {{ site.id }}...
                          </div>
                        </div>
                      </div>
                    </td>
                    <td>
                      <span class="text-muted">
                        {{ site.description || "No description" }}
                      </span>
                    </td>
                    <td>
                      <div class="d-flex align-items-center">
                        <i class="fas fa-map-marker-alt text-muted me-2"></i>
                        {{ site.location || "Not specified" }}
                      </div>
                    </td>
                    <td class="text-center">
                      <span class="badge bg-secondary rounded-pill">
                        {{ site.members?.length || 0 }}
                      </span>
                    </td>
                    <td>
                      <span class="badge bg-success-subtle text-success">
                        <i class="fas fa-circle me-1" style="font-size: 0.5rem;"></i>
                        Active
                      </span>
                    </td>
                    <td class="text-end">
                      <div class="btn-group" role="group">
                        <router-link
                          :to="`/workspaces/${site.id}`"
                          class="btn btn-sm btn-outline-primary"
                          title="View workspace"
                        >
                          <i class="fas fa-eye"></i>
                        </router-link>
                        <router-link
                          :to="`/workspaces/${site.id}/edit`"
                          class="btn btn-sm btn-outline-secondary"
                          title="Edit"
                        >
                          <i class="fas fa-cog"></i>
                        </router-link>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>

      <!-- No Results -->
      <div v-if="filteredSites.length === 0 && state.searchQuery" class="col-12 mt-3">
        <div class="alert alert-info d-flex align-items-center" role="alert">
          <i class="fas fa-info-circle me-2"></i>
          <div>No workspaces found matching "{{ state.searchQuery }}"</div>
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <div v-else class="row">
      <div class="col-12">
        <div class="card">
          <div class="card-body text-center py-5">
            <div class="empty-state-icon mb-4">
              <i class="fas fa-building text-muted"></i>
            </div>
            <h3 class="mb-3">No Workspaces Yet</h3>
            <p class="text-muted mb-4">
              Get started by creating your first workspace to begin monitoring your network infrastructure.
            </p>
            <router-link to="/workspace/new" class="btn btn-primary btn-lg">
              <i class="fas fa-plus-circle me-2"></i>Create Your First Workspace
            </router-link>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Stat Cards */
.stat-card {
  background: white;
  border-radius: 0.75rem;
  padding: 1.5rem;
  box-shadow: 0 0.125rem 0.25rem rgba(0, 0, 0, 0.075);
  display: flex;
  align-items: center;
  gap: 1.25rem;
  transition: transform 0.2s, box-shadow 0.2s;
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 0.25rem 0.5rem rgba(0, 0, 0, 0.1);
}

.stat-icon {
  width: 60px;
  height: 60px;
  border-radius: 0.75rem;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-size: 1.5rem;
}

.stat-content {
  flex: 1;
}

.stat-value {
  font-size: 1.75rem;
  font-weight: 700;
  margin-bottom: 0.25rem;
  color: #1f2937;
}

.stat-label {
  color: #6b7280;
  margin-bottom: 0;
  font-size: 0.875rem;
}

/* Cards */
.card {
  border: none;
  border-radius: 0.75rem;
  box-shadow: 0 0.125rem 0.25rem rgba(0, 0, 0, 0.075);
}

/* Table */
.table {
  margin-bottom: 0;
}

.table thead th {
  background-color: #f8f9fa;
  border-bottom: 2px solid #e9ecef;
  font-weight: 600;
  text-transform: uppercase;
  font-size: 0.75rem;
  letter-spacing: 0.05em;
  color: #6b7280;
  padding: 1rem;
}

.table tbody tr {
  border-bottom: 1px solid #e9ecef;
  transition: background-color 0.15s;
}

.table tbody tr:hover {
  background-color: #f8f9fa;
}

.table tbody td {
  padding: 1rem;
  vertical-align: middle;
}

.sortable {
  cursor: pointer;
  user-select: none;
}

.sortable:hover {
  background-color: #e9ecef;
}

/* Workspace Icon */
.workspace-icon {
  width: 40px;
  height: 40px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  border-radius: 0.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-size: 1.125rem;
}

.workspace-row:hover .workspace-icon {
  transform: scale(1.05);
  transition: transform 0.2s;
}

/* Search Input */
.input-group-text {
  background-color: #f8f9fa;
  border: 1px solid #dee2e6;
}

.form-control:focus {
  border-color: #667eea;
  box-shadow: none;
}

.form-control:focus + .input-group-text {
  border-color: #667eea;
}

/* Badges */
.badge {
  font-weight: 500;
  padding: 0.375rem 0.75rem;
}

.bg-success-subtle {
  background-color: #d1fae5;
}

/* Buttons */
.btn {
  font-weight: 500;
  transition: all 0.2s;
}

.btn-outline-primary:hover {
  transform: translateY(-1px);
}

.btn-group .btn {
  padding: 0.25rem 0.75rem;
}

/* Empty State */
.empty-state-icon {
  font-size: 5rem;
  opacity: 0.1;
}

/* Utilities */
.fw-semibold {
  font-weight: 600;
}

/* Responsive */
@media (max-width: 768px) {
  .stat-card {
    margin-bottom: 1rem;
  }

  .table-responsive {
    border-radius: 0.75rem;
  }

  .btn-group {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .btn-group .btn {
    border-radius: 0.375rem !important;
  }
}

/* Animations */
@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.workspace-row {
  animation: fadeIn 0.3s ease-out;
}
</style>