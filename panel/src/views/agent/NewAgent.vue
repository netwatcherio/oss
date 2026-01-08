<script lang="ts" setup>
import { onMounted, reactive, computed, ref } from "vue";
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
  // Modal state
  showSetupModal: false,
  createdAgent: null as Agent | null,
  agentPin: "",
  pinExpiresAt: "",
  copied: false
});

// Validation
const validation = computed(() => ({
  name: {
    valid: state.agent.name && state.agent.name.trim().length >= 2,
    message: "Agent name must be at least 2 characters"
  }
}));

const isFormValid = computed(() => validation.value.name.valid);

// Installation command with PIN
const installCommand = computed(() => {
  if (!state.createdAgent || !state.agentPin) return "";
  const baseUrl = window.location.origin;
  return `docker run -d --name netwatcher-agent \\
  -e CONTROLLER_URL="${baseUrl}" \\
  -e WORKSPACE_ID="${state.workspace.id}" \\
  -e AGENT_ID="${state.createdAgent.id}" \\
  -e AGENT_PIN="${state.agentPin}" \\
  --restart unless-stopped \\
  netwatcher/agent:latest`;
});

const binaryCommand = computed(() => {
  if (!state.createdAgent || !state.agentPin) return "";
  const baseUrl = window.location.origin;
  return `./netwatcher-agent \\
  --controller-url "${baseUrl}" \\
  --workspace-id ${state.workspace.id} \\
  --agent-id ${state.createdAgent.id} \\
  --pin "${state.agentPin}"`;
});

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

async function copyToClipboard(text: string) {
  try {
    await navigator.clipboard.writeText(text);
    state.copied = true;
    setTimeout(() => {
      state.copied = false;
    }, 2000);
  } catch (err) {
    console.error("Failed to copy:", err);
  }
}

function closeModal() {
  state.showSetupModal = false;
  router.push(`/workspaces/${state.workspace.id}`);
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
    
    // Check if we got a PIN back for bootstrap
    if (result && typeof result === 'object' && 'pin' in result) {
      const bundledResult = result as { agent: Agent; pin: string; expiresAt?: string };
      state.createdAgent = bundledResult.agent;
      state.agentPin = bundledResult.pin;
      state.pinExpiresAt = bundledResult.expiresAt || "";
      state.showSetupModal = true;
      state.loading = false;
    } else {
      // No PIN returned, redirect directly
      router.push(`/workspaces/${state.workspace.id}`);
    }
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

    <!-- Agent Setup Modal -->
    <div v-if="state.showSetupModal" class="setup-modal-overlay" @click.self="closeModal">
      <div class="setup-modal">
        <div class="setup-modal-header">
          <div class="setup-modal-icon">
            <i class="bi bi-check-circle-fill text-success"></i>
          </div>
          <h4 class="setup-modal-title">Agent Created Successfully!</h4>
          <p class="setup-modal-subtitle">
            Use the information below to connect your agent to NetWatcher.
          </p>
        </div>
        
        <div class="setup-modal-body">
          <!-- Agent PIN -->
          <div class="setup-section">
            <h6><i class="bi bi-key me-2"></i>Agent PIN</h6>
            <div class="pin-display">
              <code class="pin-code">{{ state.agentPin }}</code>
              <button 
                class="btn btn-sm btn-outline-primary"
                @click="copyToClipboard(state.agentPin)"
              >
                <i class="bi" :class="state.copied ? 'bi-check' : 'bi-clipboard'"></i>
              </button>
            </div>
            <p class="text-muted small mt-2 mb-0">
              <i class="bi bi-clock me-1"></i>
              This PIN expires in 24 hours. Use it to authenticate your agent.
            </p>
          </div>

          <!-- Docker Command -->
          <div class="setup-section">
            <h6><i class="bi bi-box me-2"></i>Docker Installation</h6>
            <div class="command-block">
              <pre class="command-code">{{ installCommand }}</pre>
              <button 
                class="btn btn-sm btn-outline-secondary copy-btn"
                @click="copyToClipboard(installCommand)"
              >
                <i class="bi bi-clipboard"></i> Copy
              </button>
            </div>
          </div>

          <!-- Binary Command -->
          <div class="setup-section">
            <h6><i class="bi bi-terminal me-2"></i>Binary Installation</h6>
            <div class="command-block">
              <pre class="command-code">{{ binaryCommand }}</pre>
              <button 
                class="btn btn-sm btn-outline-secondary copy-btn"
                @click="copyToClipboard(binaryCommand)"
              >
                <i class="bi bi-clipboard"></i> Copy
              </button>
            </div>
          </div>

          <!-- Agent Details -->
          <div class="setup-section">
            <h6><i class="bi bi-info-circle me-2"></i>Agent Details</h6>
            <table class="table table-sm mb-0">
              <tbody>
                <tr>
                  <td class="text-muted">Agent ID</td>
                  <td><code>{{ state.createdAgent?.id }}</code></td>
                </tr>
                <tr>
                  <td class="text-muted">Name</td>
                  <td>{{ state.createdAgent?.name }}</td>
                </tr>
                <tr>
                  <td class="text-muted">Workspace ID</td>
                  <td><code>{{ state.workspace.id }}</code></td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <div class="setup-modal-footer">
          <button class="btn btn-primary" @click="closeModal">
            <i class="bi bi-check-lg me-1"></i>Done
          </button>
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

/* Modal Styles */
.setup-modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1050;
  backdrop-filter: blur(4px);
}

.setup-modal {
  background: white;
  border-radius: 12px;
  max-width: 640px;
  width: 90%;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
}

.setup-modal-header {
  padding: 24px;
  text-align: center;
  border-bottom: 1px solid #e9ecef;
  background: linear-gradient(135deg, #f8f9fa 0%, #e9ecef 100%);
  border-radius: 12px 12px 0 0;
}

.setup-modal-icon {
  font-size: 3rem;
  margin-bottom: 12px;
}

.setup-modal-title {
  margin: 0;
  font-weight: 600;
  color: #212529;
}

.setup-modal-subtitle {
  margin: 8px 0 0 0;
  color: #6c757d;
}

.setup-modal-body {
  padding: 24px;
}

.setup-section {
  margin-bottom: 24px;
}

.setup-section:last-child {
  margin-bottom: 0;
}

.setup-section h6 {
  font-weight: 600;
  color: #495057;
  margin-bottom: 12px;
}

.pin-display {
  display: flex;
  align-items: center;
  gap: 12px;
  background: #f8f9fa;
  padding: 12px 16px;
  border-radius: 8px;
  border: 1px solid #dee2e6;
}

.pin-code {
  font-size: 1.5rem;
  font-weight: 700;
  letter-spacing: 3px;
  color: #0d6efd;
  flex: 1;
}

.command-block {
  position: relative;
  background: #1e1e1e;
  border-radius: 8px;
  overflow: hidden;
}

.command-code {
  padding: 16px;
  margin: 0;
  color: #d4d4d4;
  font-size: 0.85rem;
  overflow-x: auto;
  white-space: pre;
}

.copy-btn {
  position: absolute;
  top: 8px;
  right: 8px;
  background: rgba(255, 255, 255, 0.1);
  border-color: rgba(255, 255, 255, 0.2);
  color: #d4d4d4;
}

.copy-btn:hover {
  background: rgba(255, 255, 255, 0.2);
  border-color: rgba(255, 255, 255, 0.3);
  color: white;
}

.setup-modal-footer {
  padding: 16px 24px;
  border-top: 1px solid #e9ecef;
  text-align: center;
  background: #f8f9fa;
  border-radius: 0 0 12px 12px;
}

.setup-modal-footer .btn {
  min-width: 120px;
}

.table td {
  padding: 8px 0;
}
</style>