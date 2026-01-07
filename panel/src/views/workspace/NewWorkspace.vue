<script lang="ts" setup>
import { reactive, computed } from "vue";
import type { Workspace } from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import { WorkspaceService } from "@/services/apiService";

const router = core.router();

const state = reactive({
  name: "",
  description: "",
  loading: false,
  error: "",
  touched: {
    name: false
  }
});

// Validation
const validation = computed(() => ({
  name: {
    valid: state.name.trim().length >= 2,
    message: "Workspace name must be at least 2 characters"
  }
}));

const isFormValid = computed(() => validation.value.name.valid);

function markTouched(field: keyof typeof state.touched) {
  state.touched[field] = true;
}

async function submit() {
  state.touched.name = true;
  
  if (!isFormValid.value) {
    return;
  }

  state.loading = true;
  state.error = "";

  try {
    const workspace = await WorkspaceService.create({
      name: state.name.trim(),
      description: state.description.trim() || undefined,
    });
    
    // Navigate to the newly created workspace
    router.push(`/workspaces/${(workspace as Workspace).id}`);
  } catch (err: any) {
    console.error("Failed to create workspace:", err);
    state.error = err?.response?.data?.message || "Failed to create workspace. Please try again.";
    state.loading = false;
  }
}
</script>

<template>
  <div class="container-fluid">
    <Title 
      title="New Workspace" 
      subtitle="Create a new workspace to organize your agents" 
      :history="[{ title: 'Workspaces', link: '/workspaces' }]"
    />
    
    <div class="row">
      <div class="col-12 col-lg-8">
        <div class="card">
          <div class="card-header">
            <h5 class="mb-0">
              <i class="bi bi-folder-plus me-2"></i>Workspace Details
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
                v-model="state.name" 
                @blur="markTouched('name')"
                @keyup.enter="submit"
                placeholder="e.g., Production Network, Office Branch"
                type="text"
                :disabled="state.loading"
                autofocus
              >
              <div class="invalid-feedback" v-if="state.touched.name && !validation.name.valid">
                {{ validation.name.message }}
              </div>
              <div class="form-text">Choose a descriptive name for this workspace</div>
            </div>

            <!-- Description -->
            <div class="mb-3">
              <label class="form-label" for="workspaceDesc">Description</label>
              <textarea 
                id="workspaceDesc" 
                class="form-control" 
                v-model="state.description" 
                placeholder="Optional description to help identify this workspace..."
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
            <router-link to="/workspaces" class="btn btn-outline-secondary">
              <i class="bi bi-arrow-left me-1"></i>Cancel
            </router-link>
            <button 
              class="btn btn-primary" 
              @click="submit"
              :disabled="state.loading || !isFormValid"
            >
              <span v-if="state.loading">
                <span class="spinner-border spinner-border-sm me-1"></span>
                Creating...
              </span>
              <span v-else>
                <i class="bi bi-plus-lg me-1"></i>Create Workspace
              </span>
            </button>
          </div>
        </div>
      </div>

      <!-- Help sidebar -->
      <div class="col-12 col-lg-4 mt-3 mt-lg-0">
        <div class="card bg-light">
          <div class="card-body">
            <h6 class="card-title">
              <i class="bi bi-lightbulb me-2"></i>What is a Workspace?
            </h6>
            <p class="card-text small text-muted">
              Workspaces help you organize your monitoring agents and probes. 
              Each workspace can have its own team members with different access levels.
            </p>
            <h6 class="card-title mt-3">
              <i class="bi bi-people me-2"></i>Team Access
            </h6>
            <p class="card-text small text-muted mb-0">
              After creating a workspace, you can invite team members and assign roles:
            </p>
            <ul class="small text-muted mb-0 mt-1">
              <li><strong>Admin</strong> - Full access to all settings</li>
              <li><strong>Member</strong> - View and manage agents</li>
              <li><strong>Viewer</strong> - Read-only access</li>
            </ul>
          </div>
        </div>
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

.form-text {
  font-size: 0.875rem;
}
</style>