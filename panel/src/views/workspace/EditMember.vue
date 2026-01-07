<script lang="ts" setup>
import { onMounted, reactive, computed } from "vue";
import type { Workspace, Member, Role } from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import { WorkspaceService } from "@/services/apiService";

const router = core.router();

const state = reactive({
  workspace: {} as Workspace,
  member: {} as Member,
  originalRole: "" as Role,
  ready: false,
  loading: false,
  error: "",
  success: ""
});

const isOwner = computed(() => state.member.role === "OWNER");
const hasChanges = computed(() => state.member.role !== state.originalRole);

onMounted(async () => {
  const workspaceId = router.currentRoute.value.params["wID"] as string;
  const memberId = router.currentRoute.value.params["userId"] as string;

  if (!workspaceId || !memberId) {
    state.error = "Missing workspace or member ID";
    return;
  }

  try {
    const [workspace, members] = await Promise.all([
      WorkspaceService.get(workspaceId),
      WorkspaceService.listMembers(workspaceId)
    ]);
    
    state.workspace = workspace as Workspace;
    
    const member = (members as Member[]).find(m => String(m.id) === memberId);
    if (!member) {
      state.error = "Member not found";
      return;
    }
    
    state.member = member;
    state.originalRole = member.role;
    state.ready = true;
  } catch (err) {
    console.error("Failed to load member:", err);
    state.error = "Failed to load member data";
  }
});

async function submit() {
  if (!hasChanges.value || !state.member.id || isOwner.value) {
    return;
  }

  state.loading = true;
  state.error = "";
  state.success = "";

  try {
    await WorkspaceService.updateMemberRole(
      state.workspace.id,
      state.member.id,
      state.member.role as Exclude<Role, "OWNER">
    );
    
    state.success = "Member role updated successfully";
    state.originalRole = state.member.role;
    
    setTimeout(() => {
      router.push(`/workspaces/${state.workspace.id}/members`);
    }, 1000);
  } catch (err: any) {
    console.error("Failed to update member:", err);
    state.error = err?.response?.data?.message || "Failed to update member role. Please try again.";
    state.loading = false;
  }
}

const roleDescriptions: Record<Role, string> = {
  OWNER: "Full control including workspace deletion and ownership transfer",
  ADMIN: "Manage agents, probes, and workspace members",
  USER: "Create and manage agents and probes",
  VIEWER: "Read-only access to workspace data"
};
</script>

<template>
  <div class="container-fluid">
    <!-- Error state -->
    <div v-if="state.error && !state.ready" class="alert alert-danger mt-3">
      <i class="bi bi-exclamation-circle me-2"></i>{{ state.error }}
    </div>

    <div v-if="state.ready">
      <Title
        title="Edit Member"
        :subtitle="`Change role for ${state.member.email}`"
        :history="[
          { title: 'Workspaces', link: '/workspaces' },
          { title: state.workspace.name, link: `/workspaces/${state.workspace.id}` },
          { title: 'Members', link: `/workspaces/${state.workspace.id}/members` }
        ]"
      />

      <div class="row">
        <div class="col-12 col-lg-8">
          <div class="card">
            <div class="card-header">
              <h5 class="mb-0">
                <i class="bi bi-person-gear me-2"></i>Member Settings
              </h5>
            </div>
            <div class="card-body">
              <!-- Email (read-only) -->
              <div class="mb-3">
                <label class="form-label">Email</label>
                <input 
                  type="email" 
                  class="form-control" 
                  :value="state.member.email"
                  disabled
                >
              </div>

              <!-- Role -->
              <div class="mb-3">
                <label class="form-label" for="memberRole">Role</label>
                <select 
                  v-if="!isOwner"
                  id="memberRole" 
                  class="form-select" 
                  v-model="state.member.role"
                  :disabled="state.loading"
                >
                  <option value="VIEWER">Viewer</option>
                  <option value="USER">User</option>
                  <option value="ADMIN">Admin</option>
                </select>
                <select 
                  v-else
                  class="form-select" 
                  disabled
                >
                  <option value="OWNER" selected>Owner</option>
                </select>
                <div class="form-text">
                  {{ roleDescriptions[state.member.role] }}
                </div>
                <div v-if="isOwner" class="alert alert-warning mt-2 mb-0">
                  <i class="bi bi-info-circle me-2"></i>
                  Owner role cannot be changed. Use ownership transfer instead.
                </div>
              </div>

              <!-- Success message -->
              <div v-if="state.success" class="alert alert-success mb-0">
                <i class="bi bi-check-circle me-2"></i>{{ state.success }}
              </div>

              <!-- Error display -->
              <div v-if="state.error" class="alert alert-danger mb-0">
                <i class="bi bi-x-circle me-2"></i>{{ state.error }}
              </div>
            </div>

            <div class="card-footer d-flex justify-content-between">
              <router-link 
                :to="`/workspaces/${state.workspace.id}/members`" 
                class="btn btn-outline-secondary"
              >
                <i class="bi bi-arrow-left me-1"></i>Cancel
              </router-link>
              <button 
                class="btn btn-primary" 
                @click="submit"
                :disabled="state.loading || !hasChanges || isOwner"
              >
                <span v-if="state.loading">
                  <span class="spinner-border spinner-border-sm me-1"></span>
                  Saving...
                </span>
                <span v-else>
                  <i class="bi bi-check-lg me-1"></i>Save Changes
                </span>
              </button>
            </div>
          </div>
        </div>

        <!-- Info sidebar -->
        <div class="col-12 col-lg-4 mt-3 mt-lg-0">
          <div class="card bg-light">
            <div class="card-body">
              <h6 class="card-title">
                <i class="bi bi-shield-check me-2"></i>Role Permissions
              </h6>
              <ul class="list-unstyled small mb-0">
                <li class="mb-2">
                  <strong class="text-muted">Viewer</strong>
                  <p class="text-muted mb-0">View monitoring data only</p>
                </li>
                <li class="mb-2">
                  <strong class="text-success">User</strong>
                  <p class="text-muted mb-0">Create and manage agents/probes</p>
                </li>
                <li class="mb-2">
                  <strong class="text-warning">Admin</strong>
                  <p class="text-muted mb-0">Full access including members</p>
                </li>
                <li class="mb-0">
                  <strong class="text-danger">Owner</strong>
                  <p class="text-muted mb-0">Control ownership and deletion</p>
                </li>
              </ul>
            </div>
          </div>

          <div class="card border-danger mt-3" v-if="!isOwner">
            <div class="card-body">
              <h6 class="card-title text-danger">
                <i class="bi bi-person-x me-2"></i>Remove Member
              </h6>
              <p class="card-text small text-muted">
                Need to remove this member from the workspace?
              </p>
              <router-link 
                :to="`/workspaces/${state.workspace.id}/members/remove/${state.member.id}`" 
                class="btn btn-outline-danger btn-sm"
              >
                <i class="bi bi-trash me-1"></i>Remove Member
              </router-link>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Loading state -->
    <div v-else-if="!state.error" class="d-flex justify-content-center py-5">
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

.form-label {
  font-weight: 500;
}
</style>