<script lang="ts" setup>
import { onMounted, reactive, computed } from "vue";
import type { Role, Workspace } from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import { WorkspaceService } from "@/services/apiService";

const router = core.router();

const state = reactive({
  workspace: {} as Workspace,
  email: "",
  role: "USER" as Role,
  ready: false,
  loading: false,
  error: "",
  success: "",
  touched: {
    email: false
  }
});

// Validation
const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const validation = computed(() => ({
  email: {
    valid: emailRegex.test(state.email),
    message: "Please enter a valid email address"
  }
}));

const isFormValid = computed(() => validation.value.email.valid);

onMounted(async () => {
  const id = router.currentRoute.value.params["wID"] as string;
  if (!id) {
    state.error = "Missing workspace ID";
    return;
  }

  try {
    const workspace = await WorkspaceService.get(id);
    state.workspace = workspace as Workspace;
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
  state.touched.email = true;
  
  if (!isFormValid.value) {
    return;
  }

  state.loading = true;
  state.error = "";
  state.success = "";

  try {
    await WorkspaceService.addMember(state.workspace.id, {
      email: state.email.trim(),
      role: state.role
    });
    
    state.success = `Invitation sent to ${state.email}`;
    
    // Clear form for another invite
    state.email = "";
    state.touched.email = false;
    
    // Redirect after short delay
    setTimeout(() => {
      router.push(`/workspaces/${state.workspace.id}/members`);
    }, 1500);
  } catch (err: any) {
    console.error("Failed to invite member:", err);
    const msg = err?.response?.data?.message || err?.response?.data?.error;
    if (msg?.includes("already")) {
      state.error = "This email is already a member of this workspace.";
    } else {
      state.error = msg || "Failed to send invitation. Please try again.";
    }
    state.loading = false;
  }
}

const roleDescriptions: Record<Role, string> = {
  OWNER: "Full control including workspace deletion and ownership transfer",
  ADMIN: "Manage agents, probes, and members",
  USER: "View and edit agents and probes",
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
        title="Invite Member" 
        :subtitle="`Add a new member to ${state.workspace.name}`" 
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
                <i class="bi bi-person-plus me-2"></i>Invite Details
              </h5>
            </div>
            <div class="card-body">
              <div class="row">
                <!-- Email -->
                <div class="col-12 col-md-8 mb-3">
                  <label class="form-label" for="memberEmail">
                    Email Address <span class="text-danger">*</span>
                  </label>
                  <input 
                    id="memberEmail" 
                    type="email" 
                    class="form-control" 
                    :class="{ 
                      'is-invalid': state.touched.email && !validation.email.valid,
                      'is-valid': state.touched.email && validation.email.valid 
                    }"
                    v-model="state.email" 
                    @blur="markTouched('email')"
                    placeholder="colleague@example.com"
                    :disabled="state.loading"
                  >
                  <div class="invalid-feedback" v-if="state.touched.email && !validation.email.valid">
                    {{ validation.email.message }}
                  </div>
                  <div class="form-text">
                    If they have a NetWatcher account, they'll be added immediately. 
                    Otherwise, they'll receive an invitation email.
                  </div>
                </div>

                <!-- Role -->
                <div class="col-12 col-md-4 mb-3">
                  <label class="form-label" for="memberRole">Role</label>
                  <select 
                    id="memberRole" 
                    class="form-select" 
                    v-model="state.role"
                    :disabled="state.loading"
                  >
                    <option value="VIEWER">Viewer</option>
                    <option value="USER">User</option>
                    <option value="ADMIN">Admin</option>
                  </select>
                  <div class="form-text">
                    {{ roleDescriptions[state.role] }}
                  </div>
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
                :disabled="state.loading || !isFormValid"
              >
                <span v-if="state.loading">
                  <span class="spinner-border spinner-border-sm me-1"></span>
                  Sending...
                </span>
                <span v-else>
                  <i class="bi bi-envelope me-1"></i>Send Invitation
                </span>
              </button>
            </div>
          </div>
        </div>

        <!-- Role explanation sidebar -->
        <div class="col-12 col-lg-4 mt-3 mt-lg-0">
          <div class="card bg-light">
            <div class="card-body">
              <h6 class="card-title">
                <i class="bi bi-shield-check me-2"></i>Role Permissions
              </h6>
              <ul class="list-unstyled small mb-0">
                <li class="mb-2">
                  <strong class="text-primary">Viewer</strong>
                  <p class="text-muted mb-0">Read-only access to monitoring data</p>
                </li>
                <li class="mb-2">
                  <strong class="text-success">User</strong>
                  <p class="text-muted mb-0">Can create and modify agents and probes</p>
                </li>
                <li class="mb-0">
                  <strong class="text-warning">Admin</strong>
                  <p class="text-muted mb-0">Full access including member management</p>
                </li>
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