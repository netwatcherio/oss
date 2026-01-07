<script lang="ts" setup>
import { onMounted, reactive, computed } from "vue";
import type { Workspace, Member } from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import { WorkspaceService } from "@/services/apiService";

const router = core.router();

const state = reactive({
  workspace: {} as Workspace,
  member: {} as Member,
  ready: false,
  loading: false,
  error: ""
});

const isOwner = computed(() => state.member.role === "OWNER");

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
    state.ready = true;
  } catch (err) {
    console.error("Failed to load member:", err);
    state.error = "Failed to load member data";
  }
});

function cancel() {
  router.push(`/workspaces/${state.workspace.id}/members`);
}

async function submit() {
  if (!state.member.id || !state.workspace.id || isOwner.value) {
    return;
  }

  state.loading = true;
  state.error = "";

  try {
    await WorkspaceService.removeMember(state.workspace.id, state.member.id);
    router.push(`/workspaces/${state.workspace.id}/members`);
  } catch (err: any) {
    console.error("Failed to remove member:", err);
    state.error = err?.response?.data?.message || "Failed to remove member. Please try again.";
    state.loading = false;
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
        title="Remove Member"
        subtitle="Remove a member from this workspace"
        :history="[
          { title: 'Workspaces', link: '/workspaces' },
          { title: state.workspace.name, link: `/workspaces/${state.workspace.id}` },
          { title: 'Members', link: `/workspaces/${state.workspace.id}/members` }
        ]"
      />

      <div class="row">
        <div class="col-12 col-lg-8">
          <div class="card border-danger">
            <div class="card-header bg-danger text-white">
              <h5 class="mb-0">
                <i class="bi bi-person-x me-2"></i>Remove Member
              </h5>
            </div>
            <div class="card-body">
              <!-- Owner warning -->
              <div v-if="isOwner" class="alert alert-danger mb-3">
                <i class="bi bi-exclamation-octagon me-2"></i>
                <strong>Cannot remove owner.</strong> 
                Transfer ownership to another member first before removal.
              </div>

              <div v-else>
                <div class="alert alert-warning mb-3">
                  <i class="bi bi-exclamation-triangle me-2"></i>
                  <strong>Warning:</strong> This action cannot be undone.
                </div>

                <p class="mb-3">
                  Are you sure you want to remove <strong>{{ state.member.email }}</strong> from this workspace?
                </p>

                <table class="table table-sm mb-3">
                  <tbody>
                    <tr>
                      <th style="width: 100px;">Email</th>
                      <td>{{ state.member.email }}</td>
                    </tr>
                    <tr>
                      <th>Role</th>
                      <td>
                        <span class="badge" :class="{
                          'bg-secondary': state.member.role === 'VIEWER',
                          'bg-success': state.member.role === 'USER',
                          'bg-warning': state.member.role === 'ADMIN',
                          'bg-danger': state.member.role === 'OWNER'
                        }">{{ state.member.role }}</span>
                      </td>
                    </tr>
                  </tbody>
                </table>

                <p class="text-muted small mb-0">
                  The member will lose access to all agents and probes in this workspace.
                  They can be re-invited later if needed.
                </p>
              </div>

              <!-- Error display -->
              <div v-if="state.error" class="alert alert-danger mt-3 mb-0">
                <i class="bi bi-x-circle me-2"></i>{{ state.error }}
              </div>
            </div>

            <div class="card-footer d-flex justify-content-between">
              <button 
                class="btn btn-secondary" 
                @click="cancel"
                :disabled="state.loading"
              >
                <i class="bi bi-arrow-left me-1"></i>Cancel
              </button>
              <button 
                class="btn btn-danger" 
                @click="submit"
                :disabled="state.loading || isOwner"
              >
                <span v-if="state.loading">
                  <span class="spinner-border spinner-border-sm me-1"></span>
                  Removing...
                </span>
                <span v-else>
                  <i class="bi bi-person-x me-1"></i>Remove Member
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
                <i class="bi bi-info-circle me-2"></i>What happens?
              </h6>
              <ul class="small text-muted mb-0">
                <li>Member immediately loses access</li>
                <li>All their permissions are revoked</li>
                <li>They can be re-invited later</li>
                <li>Their past actions remain in logs</li>
              </ul>
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
  border-bottom: 0;
}

.card.border-danger {
  border-width: 2px;
}

.table th {
  font-weight: 600;
  color: #6c757d;
}
</style>