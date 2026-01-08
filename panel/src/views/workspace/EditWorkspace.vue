<script lang="ts" setup>
import { onMounted, reactive, computed } from "vue";
import type { Workspace } from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import { WorkspaceService } from "@/services/apiService";

const router = core.router();

const state = reactive({
  workspace: {} as Workspace,
  originalName: "",
  originalDescription: "",
  ready: false,
  loading: false,
  error: "",
  touched: {
    name: false
  }
});

// Validation
const validation = computed(() => ({
  name: {
    valid: state.workspace.name && state.workspace.name.trim().length >= 2,
    message: "Workspace name must be at least 2 characters"
  }
}));

const isFormValid = computed(() => validation.value.name.valid);
const hasChanges = computed(() => 
  state.workspace.name !== state.originalName || 
  state.workspace.description !== state.originalDescription
);

onMounted(async () => {
  const wID = router.currentRoute.value.params["wID"] as string;
  if (!wID) {
    state.error = "Missing workspace ID";
    return;
  }

  try {
    const workspace = await WorkspaceService.get(wID);
    state.workspace = workspace as Workspace;
    state.originalName = state.workspace.name;
    state.originalDescription = state.workspace.description || "";
    state.ready = true;
  } catch (err) {
    console.error("Failed to load workspace:", err);
    state.error = "Failed to load workspace";
  }
});

function markTouched(field: keyof typeof state.touched) {
  state.touched[field] = true;
}

async function submit() {
  state.touched.name = true;
  
  if (!isFormValid.value || !state.workspace.id) {
    return;
  }

  state.loading = true;
  state.error = "";

  try {
    await WorkspaceService.update(state.workspace.id, {
      name: state.workspace.name,
      description: state.workspace.description
    });
    router.push(`/workspaces/${state.workspace.id}`);
  } catch (err: any) {
    console.error("Failed to update workspace:", err);
    state.error = err?.response?.data?.message || "Failed to update workspace. Please try again.";
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
        title="Edit Workspace"
        subtitle="Update workspace settings"
        :history="[
          { title: 'Workspaces', link: '/workspaces' },
          { title: state.workspace.name, link: `/workspaces/${state.workspace.id}` }
        ]"
      />
      
      <div class="row">
        <div class="col-12 col-lg-8">
          <div class="card">
            <div class="card-header">
              <h5 class="mb-0">
                <i class="bi bi-folder me-2"></i>Workspace Settings
              </h5>
            </div>
            <div class="card-body">
              <!-- Workspace Name -->
              <div class="mb-3">
                <label class="form-label" for="workspaceName">
                  Workspace Name <span class="text-danger">*</span>
                </label>
                <input 
                  id="workspaceName" 
                  class="form-control" 
                  :class="{ 
                    'is-invalid': state.touched.name && !validation.name.valid,
                    'is-valid': state.touched.name && validation.name.valid 
                  }"
                  v-model="state.workspace.name" 
                  @blur="markTouched('name')"
                  placeholder="Workspace name"
                  type="text"
                  :disabled="state.loading"
                >
                <div class="invalid-feedback" v-if="state.touched.name && !validation.name.valid">
                  {{ validation.name.message }}
                </div>
              </div>

              <!-- Description -->
              <div class="mb-3">
                <label class="form-label" for="workspaceDesc">Description</label>
                <textarea 
                  id="workspaceDesc" 
                  class="form-control" 
                  v-model="state.workspace.description" 
                  placeholder="Optional workspace description..."
                  rows="3"
                  :disabled="state.loading"
                ></textarea>
              </div>

              <!-- Error display -->
              <div v-if="state.error" class="alert alert-danger mb-0">
                <i class="bi bi-x-circle me-2"></i>{{ state.error }}
              </div>
            </div>

            <div class="card-footer d-flex justify-content-between">
              <router-link 
                :to="`/workspaces/${state.workspace.id}`" 
                class="btn btn-outline-secondary"
              >
                <i class="bi bi-arrow-left me-1"></i>Cancel
              </router-link>
              <button 
                class="btn btn-primary" 
                @click="submit"
                :disabled="state.loading || !isFormValid"
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
                <i class="bi bi-info-circle me-2"></i>Workspace Info
              </h6>
              <table class="table table-sm table-borderless mb-0">
                <tbody>
                  <tr>
                    <td class="text-muted">ID</td>
                    <td><code>{{ state.workspace.id }}</code></td>
                  </tr>
                  <tr v-if="state.workspace.created_at">
                    <td class="text-muted">Created</td>
                    <td>{{ new Date(state.workspace.created_at).toLocaleDateString() }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="card border-warning mt-3">
            <div class="card-body">
              <h6 class="card-title text-warning">
                <i class="bi bi-exclamation-triangle me-2"></i>Danger Zone
              </h6>
              <p class="card-text small text-muted">
                Need to delete this workspace? This action cannot be undone.
              </p>
              <router-link 
                :to="`/workspaces/${state.workspace.id}/delete`" 
                class="btn btn-outline-danger btn-sm"
              >
                <i class="bi bi-trash me-1"></i>Delete Workspace
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

.table-sm td {
  padding: 0.25rem 0;
}
</style>
