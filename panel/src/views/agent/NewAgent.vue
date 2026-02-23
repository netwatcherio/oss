<script lang="ts" setup>
import { onMounted, reactive, computed } from "vue";
import type { Workspace, Agent } from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import { AgentService, WorkspaceService } from "@/services/apiService";

const router = core.router();

const state = reactive({
  workspace: {} as Workspace,
  ready: false,
  loading: false,
  agent: {
    name: "",
    location: "",
    description: ""
  } as Partial<Agent>,
  error: "",
  touched: {
    name: false
  },
});

// Validation
const validation = computed(() => ({
  name: {
    valid: state.agent.name && state.agent.name.trim().length >= 2,
    message: "Agent name must be at least 2 characters"
  }
}));

const isFormValid = computed(() => validation.value.name.valid);

onMounted(async () => {
  const id = router.currentRoute.value.params["wID"] as string;
  if (!id) {
    state.error = "Missing workspace ID";
    return;
  }

  try {
    const workspace = await WorkspaceService.get(id);
    state.workspace = workspace as Workspace;
    state.agent.workspace_id = state.workspace.id;
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
  // Mark all fields as touched to show validation
  state.touched.name = true;
  
  if (!isFormValid.value) {
    return;
  }

  state.loading = true;
  state.error = "";

  try {
    const result = await AgentService.create(state.workspace.id, state.agent);
    
    // Get the agent ID from the response
    let agentId: number | string;
    if (result && typeof result === 'object' && 'agent' in result) {
      agentId = (result as { agent: Agent }).agent.id;
    } else {
      agentId = (result as Agent).id;
    }

    // Redirect to the dedicated setup page
    router.push(`/workspaces/${state.workspace.id}/agents/${agentId}/setup`);
  } catch (err: any) {
    console.error("Failed to create agent:", err);
    state.error = err?.response?.data?.message || "Failed to create agent. Please try again.";
    state.loading = false;
  }
}
</script>

<template>
  <div class="container-fluid">
    <!-- Error state before ready -->
    <div v-if="state.error && !state.ready" class="alert alert-danger mt-3">
      <i class="bi bi-exclamation-circle me-2"></i>{{ state.error }}
    </div>

    <div v-if="state.ready">
      <Title 
        title="Create Agent" 
        subtitle="Add a new monitoring agent to this workspace" 
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
                <i class="bi bi-cpu me-2"></i>Agent Details
              </h5>
            </div>
            <div class="card-body">
              <!-- Agent Name -->
              <div class="mb-3">
                <label class="form-label" for="agentName">
                  Agent Name <span class="text-danger">*</span>
                </label>
                <input 
                  id="agentName" 
                  class="form-control" 
                  :class="{ 
                    'is-invalid': state.touched.name && !validation.name.valid,
                    'is-valid': state.touched.name && validation.name.valid 
                  }"
                  v-model="state.agent.name" 
                  @blur="markTouched('name')"
                  placeholder="e.g., Office Router, Data Center 1"
                  type="text"
                  :disabled="state.loading"
                >
                <div class="invalid-feedback" v-if="state.touched.name && !validation.name.valid">
                  {{ validation.name.message }}
                </div>
                <div class="form-text">Choose a descriptive name to identify this agent</div>
              </div>

              <!-- Location -->
              <div class="mb-3">
                <label class="form-label" for="agentLocation">Location</label>
                <input 
                  id="agentLocation" 
                  class="form-control" 
                  v-model="state.agent.location" 
                  placeholder="e.g., New York, Building A, Rack 3"
                  type="text"
                  :disabled="state.loading"
                >
                <div class="form-text">Optional physical or logical location</div>
              </div>

              <!-- Description -->
              <div class="mb-3">
                <label class="form-label" for="agentDescription">Description</label>
                <textarea 
                  id="agentDescription" 
                  class="form-control" 
                  v-model="state.agent.description" 
                  placeholder="Optional notes about this agent..."
                  rows="2"
                  :disabled="state.loading"
                ></textarea>
              </div>

              <!-- Error display -->
              <div v-if="state.error" class="alert alert-danger mb-3">
                <i class="bi bi-x-circle me-2"></i>{{ state.error }}
              </div>

              <!-- Info box -->
              <div class="alert alert-info mb-0">
                <i class="bi bi-info-circle me-2"></i>
                After creating the agent, you'll receive a PIN to connect the agent software.
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
                  Creating...
                </span>
                <span v-else>
                  <i class="bi bi-plus-lg me-1"></i>Create Agent
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
                <i class="bi bi-lightbulb me-2"></i>What is an Agent?
              </h6>
              <p class="card-text small text-muted">
                An agent is a lightweight monitoring software that runs on your network devices 
                or servers. It collects performance metrics and sends them to NetWatcher.
              </p>
              <h6 class="card-title mt-3">
                <i class="bi bi-gear me-2"></i>Next Steps
              </h6>
              <ol class="small text-muted mb-0">
                <li>Create the agent entry</li>
                <li>Download and install the agent software</li>
                <li>Use the PIN to connect the agent</li>
                <li>Configure probes to start monitoring</li>
              </ol>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Loading skeleton -->
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

</style>