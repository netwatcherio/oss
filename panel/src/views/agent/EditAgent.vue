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
  agent: {} as Agent,
  error: "",
  touched: {
    name: false
  }
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
  const agentId = router.currentRoute.value.params["aID"] as string;
  const workspaceId = router.currentRoute.value.params["wID"] as string;
  
  if (!agentId || !workspaceId) {
    state.error = "Missing agent or workspace ID";
    return;
  }

  try {
    const [workspace, agent] = await Promise.all([
      WorkspaceService.get(workspaceId),
      AgentService.get(workspaceId, agentId)
    ]);
    
    state.workspace = workspace as Workspace;
    state.agent = agent as Agent;
    state.ready = true;
  } catch (err) {
    console.error("Failed to load data:", err);
    state.error = "Failed to load agent data";
  }
});

function markTouched(field: keyof typeof state.touched) {
  state.touched[field] = true;
}

async function submit() {
  state.touched.name = true;
  
  if (!isFormValid.value || !state.agent.id) {
    return;
  }

  state.loading = true;
  state.error = "";

  try {
    await AgentService.update(state.workspace.id, state.agent.id, {
      name: state.agent.name,
      location: state.agent.location,
      description: state.agent.description,
      public_ip_override: state.agent.public_ip_override,
      trafficsim_enabled: state.agent.trafficsim_enabled,
      trafficsim_host: state.agent.trafficsim_host,
      trafficsim_port: state.agent.trafficsim_port,
    });
    router.push(`/workspaces/${state.workspace.id}/agents/${state.agent.id}`);
  } catch (err: any) {
    console.error("Failed to update agent:", err);
    state.error = err?.response?.data?.message || "Failed to update agent. Please try again.";
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
        title="Edit Agent"
        subtitle="Update agent details and configuration"
        :history="[
          { title: 'Workspaces', link: '/workspaces' },
          { title: state.workspace.name, link: `/workspaces/${state.workspace.id}` },
          { title: state.agent.name, link: `/workspaces/${state.workspace.id}/agents/${state.agent.id}` }
        ]"
      >
        <router-link 
          :to="`/workspaces/${state.workspace.id}/agents/${state.agent.id}/delete`" 
          class="btn btn-outline-danger"
        >
          <i class="bi bi-trash me-1"></i>Delete
        </router-link>
      </Title>

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
                  placeholder="Agent name"
                  type="text"
                  :disabled="state.loading"
                >
                <div class="invalid-feedback" v-if="state.touched.name && !validation.name.valid">
                  {{ validation.name.message }}
                </div>
              </div>

              <!-- Location -->
              <div class="mb-3">
                <label class="form-label" for="agentLocation">Location</label>
                <input 
                  id="agentLocation" 
                  class="form-control" 
                  v-model="state.agent.location" 
                  placeholder="e.g., New York, Building A"
                  type="text"
                  :disabled="state.loading"
                >
                <div class="form-text">Physical or logical location of this agent</div>
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

              <hr>

              <!-- Public IP Override -->
              <div class="mb-3">
                <label class="form-label" for="agentPublicIP">
                  Public IP Override
                  <span class="badge bg-secondary ms-1">Advanced</span>
                </label>
                <input 
                  id="agentPublicIP" 
                  class="form-control" 
                  v-model="state.agent.public_ip_override" 
                  placeholder="e.g., 203.0.113.50"
                  type="text"
                  :disabled="state.loading"
                >
                <div class="form-text">
                  Override the auto-detected public IP. Used when other agents target this agent.
                </div>
              </div>

              <hr>

              <!-- TrafficSim Server Configuration -->
              <h6 class="mb-3">
                <i class="bi bi-speedometer2 me-2"></i>TrafficSim Server
                <span class="badge bg-info ms-1">Optional</span>
              </h6>

              <div class="mb-3">
                <div class="form-check form-switch">
                  <input
                    class="form-check-input"
                    type="checkbox"
                    role="switch"
                    id="trafficsimEnabled"
                    v-model="state.agent.trafficsim_enabled"
                    :disabled="state.loading"
                  >
                  <label class="form-check-label" for="trafficsimEnabled">
                    Enable TrafficSim Server
                  </label>
                </div>
                <div class="form-text">Run a TrafficSim server on this agent for inter-agent traffic simulation testing.</div>
              </div>

              <div v-if="state.agent.trafficsim_enabled" class="row mb-3">
                <div class="col-8">
                  <label class="form-label" for="trafficsimHost">Listen Host</label>
                  <input
                    id="trafficsimHost"
                    class="form-control"
                    v-model="state.agent.trafficsim_host"
                    placeholder="0.0.0.0"
                    type="text"
                    :disabled="state.loading"
                  >
                  <div class="form-text">IP address to bind the server to (default: 0.0.0.0)</div>
                </div>
                <div class="col-4">
                  <label class="form-label" for="trafficsimPort">Port</label>
                  <input
                    id="trafficsimPort"
                    class="form-control"
                    v-model.number="state.agent.trafficsim_port"
                    placeholder="5000"
                    type="number"
                    min="1"
                    max="65535"
                    :disabled="state.loading"
                  >
                  <div class="form-text">Default: 5000</div>
                </div>
              </div>

              <!-- Error display -->
              <div v-if="state.error" class="alert alert-danger mb-0">
                <i class="bi bi-x-circle me-2"></i>{{ state.error }}
              </div>
            </div>

            <div class="card-footer d-flex justify-content-between">
              <router-link 
                :to="`/workspaces/${state.workspace.id}/agents/${state.agent.id}`" 
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
                <i class="bi bi-info-circle me-2"></i>Agent Info
              </h6>
              <table class="table table-sm table-borderless mb-0">
                <tbody>
                  <tr>
                    <td class="text-muted">ID</td>
                    <td><code>{{ state.agent.id }}</code></td>
                  </tr>
                  <tr v-if="state.agent.version">
                    <td class="text-muted">Version</td>
                    <td>{{ state.agent.version }}</td>
                  </tr>
                  <tr v-if="state.agent.created_at">
                    <td class="text-muted">Created</td>
                    <td>{{ new Date(state.agent.created_at).toLocaleDateString() }}</td>
                  </tr>
                  <tr v-if="state.agent.updated_at">
                    <td class="text-muted">Last Seen</td>
                    <td>{{ new Date(state.agent.updated_at).toLocaleString() }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="card bg-light mt-3">
            <div class="card-body">
              <h6 class="card-title">
                <i class="bi bi-globe me-2"></i>Public IP Override
              </h6>
              <p class="card-text small text-muted mb-0">
                When other agents target this agent (e.g., for PING or MTR probes), 
                the controller resolves the target IP from NETINFO data. 
                Set an override if auto-detection doesn't work for your network.
              </p>
            </div>
          </div>

          <div class="card bg-light mt-3">
            <div class="card-body">
              <h6 class="card-title">
                <i class="bi bi-speedometer2 me-2"></i>TrafficSim Server
              </h6>
              <p class="card-text small text-muted mb-0">
                Enable a TrafficSim server to allow other agents to measure 
                bidirectional latency, packet loss, and jitter between this agent 
                and others in the same workspace. Only one server can run per agent.
              </p>
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

.table-sm td {
  padding: 0.25rem 0;
}
</style>