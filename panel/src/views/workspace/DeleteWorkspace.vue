<script lang="ts" setup>
import { onMounted, reactive } from "vue";
import type { Workspace } from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import { WorkspaceService } from "@/services/apiService";

const router = core.router();

const state = reactive({
  workspace: {} as Workspace,
  ready: false,
  loading: false,
  error: "",
  confirmName: ""
});

onMounted(async () => {
  const wID = router.currentRoute.value.params["wID"] as string;
  if (!wID) {
    state.error = "Missing workspace ID";
    return;
  }

  try {
    const workspace = await WorkspaceService.get(wID);
    state.workspace = workspace as Workspace;
    state.ready = true;
  } catch (err) {
    console.error("Failed to load workspace:", err);
    state.error = "Failed to load workspace";
  }
});

function cancel() {
  router.push(`/workspaces/${state.workspace.id}/edit`);
}

async function submit() {
  if (!state.workspace.id) return;
  if (state.confirmName !== state.workspace.name) return;

  state.loading = true;
  state.error = "";

  try {
    await WorkspaceService.remove(state.workspace.id);
    router.push("/workspaces");
  } catch (err: any) {
    console.error("Failed to delete workspace:", err);
    state.error = err?.response?.data?.message || err?.response?.data?.error || "Failed to delete workspace. Please try again.";
    state.loading = false;
  }
}
</script>

<template>
  <div class="container-fluid">
    <!-- Error State -->
    <div v-if="state.error && !state.ready" class="alert alert-danger mt-3">
      <i class="bi bi-exclamation-circle me-2"></i>{{ state.error }}
    </div>

    <div v-if="state.ready">
      <Title
        title="Delete Workspace"
        subtitle="Permanently remove this workspace"
        :history="[
          { title: 'Workspaces', link: '/workspaces' },
          { title: state.workspace.name, link: `/workspaces/${state.workspace.id}` },
          { title: 'Edit', link: `/workspaces/${state.workspace.id}/edit` }
        ]"
      />

      <div class="row">
        <div class="col-12 col-lg-8">
          <div class="card border-danger">
            <div class="card-header bg-danger text-white">
              <h5 class="mb-0">
                <i class="bi bi-exclamation-triangle me-2"></i>Danger Zone
              </h5>
            </div>
            <div class="card-body">
              <div class="alert alert-warning mb-3">
                <i class="bi bi-info-circle me-2"></i>
                <strong>Warning:</strong> This action cannot be undone. All agents, probes, members,
                and historical data associated with this workspace will be permanently deleted.
              </div>

              <p class="mb-3">
                Are you sure you want to delete the workspace <strong>{{ state.workspace.name }}</strong>?
              </p>

              <p class="text-muted small mb-2">
                To confirm, type the workspace name below:
              </p>
              <input
                class="form-control mb-3"
                v-model="state.confirmName"
                :placeholder="state.workspace.name"
                type="text"
                :disabled="state.loading"
              >

              <!-- Error message -->
              <div v-if="state.error" class="alert alert-danger mb-0">
                <i class="bi bi-x-circle me-2"></i>{{ state.error }}
              </div>
            </div>

            <div class="card-footer d-flex justify-content-end gap-2">
              <button
                class="btn btn-secondary"
                @click="cancel"
                :disabled="state.loading"
              >
                <i class="bi bi-x-lg me-1"></i>Cancel
              </button>
              <button
                class="btn btn-danger"
                @click="submit"
                :disabled="state.loading || state.confirmName !== state.workspace.name"
              >
                <span v-if="state.loading">
                  <span class="spinner-border spinner-border-sm me-1"></span>
                  Deleting...
                </span>
                <span v-else>
                  <i class="bi bi-trash me-1"></i>Delete Workspace
                </span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Loading State -->
    <div v-else-if="!state.error" class="d-flex justify-content-center py-5">
      <div class="spinner-border text-primary" role="status">
        <span class="visually-hidden">Loading...</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.card.border-danger {
  border-width: 2px;
}
</style>
