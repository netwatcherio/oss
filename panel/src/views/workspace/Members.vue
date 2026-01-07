<script lang="ts" setup>
import type { Workspace, Member, Role } from "@/types";
import { onMounted, reactive, computed } from "vue";
import core from "@/core";
import { WorkspaceService } from "@/services/apiService";
import Title from "@/components/Title.vue";
import { usePermissions } from "@/composables/usePermissions";

const router = core.router();

const state = reactive({
  workspace: {} as Workspace & { my_role?: Role },
  members: [] as Member[],
  ready: false,
  loading: true,
  error: ""
});

// Permissions based on user's role in this workspace
const permissions = computed(() => usePermissions(state.workspace.my_role));

onMounted(async () => {
  const id = router.currentRoute.value.params["wID"] as string;
  if (!id) {
    state.error = "Missing workspace ID";
    state.loading = false;
    return;
  }

  try {
    const workspace = await WorkspaceService.get(id);
    state.workspace = workspace as Workspace & { my_role?: Role };

    const members = await WorkspaceService.listMembers(state.workspace.id);
    state.members = members as Member[];

    state.ready = true;
  } catch (err) {
    console.error("Failed to load members:", err);
    state.error = "Failed to load workspace members";
  } finally {
    state.loading = false;
  }
});

const memberCount = computed(() => state.members.length);
const ownerCount = computed(() => state.members.filter(m => m.role === "OWNER").length);
const adminCount = computed(() => state.members.filter(m => m.role === "ADMIN").length);

function getRoleBadgeClass(role: string): string {
  switch (role) {
    case "OWNER": return "bg-danger";
    case "ADMIN": return "bg-warning text-dark";
    case "USER": return "bg-success";
    case "VIEWER": return "bg-secondary";
    default: return "bg-secondary";
  }
}
</script>

<template>
  <div class="container-fluid">
    <!-- Error state -->
    <div v-if="state.error && !state.ready" class="alert alert-danger mt-3">
      <i class="bi bi-exclamation-circle me-2"></i>{{ state.error }}
    </div>

    <div v-if="state.ready">
      <Title 
        title="Members" 
        :subtitle="`${memberCount} member${memberCount !== 1 ? 's' : ''} in ${state.workspace.name}`" 
        :history="[
          { title: 'Workspaces', link: '/workspaces' }, 
          { title: state.workspace.name, link: `/workspaces/${state.workspace.id}` }
        ]"
      >
        <router-link 
          v-if="permissions.canManage.value"
          :to="`/workspaces/${state.workspace.id}/members/invite`" 
          class="btn btn-primary"
        >
          <i class="bi bi-person-plus me-1"></i>Invite Member
        </router-link>
      </Title>

      <div class="row">
        <div class="col-12 col-lg-9">
          <!-- Members Table -->
          <div class="card">
            <div class="card-header d-flex justify-content-between align-items-center">
              <h5 class="mb-0">
                <i class="bi bi-people me-2"></i>Workspace Members
              </h5>
              <span class="badge bg-primary">{{ memberCount }}</span>
            </div>
            <div class="table-responsive">
              <table class="table table-hover mb-0">
                <thead class="table-light">
                  <tr>
                    <th>Email</th>
                    <th>Role</th>
                    <th class="text-end">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="member in state.members" :key="member.id">
                    <td>
                      <div class="d-flex align-items-center">
                        <div class="avatar-sm bg-light rounded-circle d-flex align-items-center justify-content-center me-2">
                          <i class="bi bi-person text-muted"></i>
                        </div>
                        <div>
                          <span>{{ member.email }}</span>
                          <small v-if="member.role === 'OWNER'" class="d-block text-muted">Workspace owner</small>
                        </div>
                      </div>
                    </td>
                    <td>
                      <span class="badge" :class="getRoleBadgeClass(member.role)">
                        {{ member.role }}
                      </span>
                    </td>
                    <td class="text-end">
                      <div class="btn-group btn-group-sm" v-if="permissions.canManage.value">
                        <router-link 
                          :to="`/workspaces/${state.workspace.id}/members/edit/${member.id}`" 
                          class="btn btn-outline-primary"
                          title="Edit member"
                        >
                          <i class="bi bi-pencil"></i>
                        </router-link>
                        <router-link 
                          v-if="member.role !== 'OWNER'"
                          :to="`/workspaces/${state.workspace.id}/members/remove/${member.id}`" 
                          class="btn btn-outline-danger"
                          title="Remove member"
                        >
                          <i class="bi bi-person-x"></i>
                        </router-link>
                      </div>
                      <span v-else class="text-muted small">View only</span>
                    </td>
                  </tr>
                  <tr v-if="state.members.length === 0">
                    <td colspan="3" class="text-center text-muted py-4">
                      No members found. <router-link :to="`/workspaces/${state.workspace.id}/members/invite`">Invite the first member</router-link>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>

        <!-- Sidebar -->
        <div class="col-12 col-lg-3 mt-3 mt-lg-0">
          <div class="card bg-light">
            <div class="card-body">
              <h6 class="card-title">
                <i class="bi bi-pie-chart me-2"></i>Role Summary
              </h6>
              <ul class="list-unstyled mb-0">
                <li class="d-flex justify-content-between mb-1">
                  <span><span class="badge bg-danger">OWNER</span></span>
                  <span class="text-muted">{{ ownerCount }}</span>
                </li>
                <li class="d-flex justify-content-between mb-1">
                  <span><span class="badge bg-warning text-dark">ADMIN</span></span>
                  <span class="text-muted">{{ adminCount }}</span>
                </li>
                <li class="d-flex justify-content-between">
                  <span><span class="badge bg-success">USER</span></span>
                  <span class="text-muted">{{ memberCount - ownerCount - adminCount }}</span>
                </li>
              </ul>
            </div>
          </div>

          <div class="card bg-light mt-3">
            <div class="card-body">
              <h6 class="card-title">
                <i class="bi bi-shield-check me-2"></i>Permissions
              </h6>
              <p class="card-text small text-muted mb-2">
                <strong>Owner</strong> - Full control including deletion
              </p>
              <p class="card-text small text-muted mb-2">
                <strong>Admin</strong> - Manage agents, probes, members
              </p>
              <p class="card-text small text-muted mb-0">
                <strong>User</strong> - Create and manage probes
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Loading state -->
    <div v-else-if="state.loading" class="d-flex justify-content-center py-5">
      <div class="spinner-border text-primary" role="status">
        <span class="visually-hidden">Loading...</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.card-header {
  background-color: #f8f9fa;
  border-bottom: 1px solid #e9ecef;
}

.avatar-sm {
  width: 32px;
  height: 32px;
}

.table th {
  font-weight: 600;
  font-size: 0.875rem;
}

.btn-group-sm .btn {
  padding: 0.25rem 0.5rem;
}
</style>