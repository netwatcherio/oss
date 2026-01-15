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
  // Regeneration result
  regenerated: false,
  newPin: "",
  copied: false
});

// Get controller host from environment or window
function getControllerInfo() {
  const anyWindow = window as any;
  let endpoint = anyWindow?.CONTROLLER_ENDPOINT 
    || import.meta.env?.CONTROLLER_ENDPOINT 
    || `${window.location.protocol}//${window.location.host}`;
  
  // Parse the URL to extract host and SSL
  try {
    const url = new URL(endpoint);
    return {
      host: url.host,
      ssl: url.protocol === 'https:'
    };
  } catch {
    // Fallback if URL parsing fails
    return {
      host: window.location.host,
      ssl: window.location.protocol === 'https:'
    };
  }
}

// Linux/macOS install script command
const linuxInstallCommand = computed(() => {
  if (!state.agent || !state.newPin) return "";
  const { host, ssl } = getControllerInfo();
  return `curl -fsSL https://raw.githubusercontent.com/netwatcherio/agent/refs/heads/master/install.sh | sudo bash -s -- \\
  --host ${host} \\
  --ssl ${ssl} \\
  --workspace ${state.workspace.id} \\
  --id ${state.agent.id} \\
  --pin ${state.newPin}`;
});

// Windows PowerShell install command  
const windowsInstallCommand = computed(() => {
  if (!state.agent || !state.newPin) return "";
  const { host, ssl } = getControllerInfo();
  return `# Download the installer
Invoke-WebRequest -Uri "https://raw.githubusercontent.com/netwatcherio/agent/refs/heads/master/install.ps1" -OutFile "install.ps1"
# Run the installer
.\\install.ps1 -ControllerHost "${host}" -SSL $${ssl ? 'true' : 'false'} -Workspace ${state.workspace.id} -Id ${state.agent.id} -Pin "${state.newPin}"`;
});

// Docker command
const dockerInstallCommand = computed(() => {
  if (!state.agent || !state.newPin) return "";
  const { host, ssl } = getControllerInfo();
  return `docker run -d --name netwatcher-agent \\
  -e CONTROLLER_HOST="${host}" \\
  -e CONTROLLER_SSL="${ssl}" \\
  -e WORKSPACE_ID="${state.workspace.id}" \\
  -e AGENT_ID="${state.agent.id}" \\
  -e AGENT_PIN="${state.newPin}" \\
  --restart unless-stopped \\
  netwatcher/agent:latest`;
});

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

function cancel() {
  router.push(`/workspaces/${state.workspace.id}`);
}

function done() {
  router.push(`/workspaces/${state.workspace.id}`);
}

async function submit() {
  if (!state.agent.id || !state.workspace.id) return;
  
  state.loading = true;
  state.error = "";
  
  try {
    const result = await AgentService.regenerate(state.workspace.id, state.agent.id);
    state.newPin = result.pin;
    state.agent = result.agent;
    state.regenerated = true;
  } catch (err: any) {
    console.error("Failed to regenerate agent:", err);
    state.error = err?.response?.data?.message || "Failed to regenerate agent. Please try again.";
  } finally {
    state.loading = false;
  }
}

async function copyCommand(command: string) {
  try {
    await navigator.clipboard.writeText(command);
    state.copied = true;
    setTimeout(() => state.copied = false, 2000);
  } catch (err) {
    console.error("Failed to copy:", err);
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
        :title="state.regenerated ? 'Agent Regenerated' : 'Regenerate Agent'"
        :subtitle="state.regenerated ? 'Install the agent on your new machine' : 'Reset credentials and get new install commands'"
        :history="[
          { title: 'Workspaces', link: '/workspaces' },
          { title: state.workspace.name, link: `/workspaces/${state.workspace.id}` },
          { title: state.agent.name, link: `/workspaces/${state.workspace.id}/agents/${state.agent.id}` }
        ]"
      />
      
      <!-- Pre-regeneration view -->
      <div v-if="!state.regenerated" class="row">
        <div class="col-12 col-lg-8">
          <div class="card border-warning">
            <div class="card-header bg-warning text-dark">
              <h5 class="mb-0">
                <i class="bi bi-arrow-repeat me-2"></i>Regenerate Agent Credentials
              </h5>
            </div>
            <div class="card-body">
              <div class="alert alert-warning mb-3">
                <i class="bi bi-exclamation-triangle me-2"></i>
                <strong>Warning:</strong> This will immediately disconnect any currently connected agent and invalidate its credentials.
              </div>
              
              <p class="mb-3">
                Regenerating the agent <strong>{{ state.agent.name }}</strong> will:
              </p>
              
              <ul class="mb-3">
                <li>Invalidate the current security credentials (PSK)</li>
                <li>Disconnect the currently connected agent immediately</li>
                <li>Issue a new bootstrap PIN for reinstallation</li>
                <li>Preserve all historical probe data</li>
              </ul>
              
              <div v-if="state.agent.location" class="text-muted mb-3">
                <i class="bi bi-geo-alt me-1"></i>Location: {{ state.agent.location }}
              </div>
              
              <!-- Error message -->
              <div v-if="state.error" class="alert alert-danger mb-3">
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
                class="btn btn-warning" 
                @click="submit"
                :disabled="state.loading"
              >
                <span v-if="state.loading">
                  <span class="spinner-border spinner-border-sm me-1"></span>
                  Regenerating...
                </span>
                <span v-else>
                  <i class="bi bi-arrow-repeat me-1"></i>Regenerate Agent
                </span>
              </button>
            </div>
          </div>
        </div>
        
        <!-- Info Sidebar -->
        <div class="col-12 col-lg-4 mt-3 mt-lg-0">
          <div class="card bg-light">
            <div class="card-body">
              <h6 class="card-title">
                <i class="bi bi-question-circle me-2"></i>When to use this?
              </h6>
              <ul class="small text-muted mb-0">
                <li>Moving the agent to a different machine</li>
                <li>Reinstalling the agent after a system wipe</li>
                <li>Security concern requiring credential rotation</li>
                <li>Agent is stuck and needs a fresh start</li>
              </ul>
            </div>
          </div>
        </div>
      </div>
      
      <!-- Post-regeneration view with install commands -->
      <div v-else class="row">
        <div class="col-12 col-lg-8">
          <div class="card border-success">
            <div class="card-header bg-success text-white">
              <h5 class="mb-0">
                <i class="bi bi-check-circle me-2"></i>Agent Regenerated Successfully
              </h5>
            </div>
            <div class="card-body">
              <div class="alert alert-success mb-4">
                <i class="bi bi-shield-check me-2"></i>
                New credentials have been generated. Use one of the commands below to install the agent on your machine.
              </div>
              
              <!-- PIN Display -->
              <div class="mb-4">
                <label class="form-label fw-bold">Bootstrap PIN (one-time use)</label>
                <div class="input-group">
                  <span class="input-group-text"><i class="bi bi-key"></i></span>
                  <input type="text" class="form-control font-monospace" :value="state.newPin" readonly />
                  <button class="btn btn-outline-secondary" @click="copyCommand(state.newPin)">
                    <i class="bi bi-clipboard"></i>
                  </button>
                </div>
              </div>
              
              <!-- Install Commands Tabs -->
              <ul class="nav nav-tabs" role="tablist">
                <li class="nav-item">
                  <button class="nav-link active" data-bs-toggle="tab" data-bs-target="#linux-tab" type="button">
                    <i class="bi bi-terminal me-1"></i>Linux/macOS
                  </button>
                </li>
                <li class="nav-item">
                  <button class="nav-link" data-bs-toggle="tab" data-bs-target="#windows-tab" type="button">
                    <i class="bi bi-windows me-1"></i>Windows
                  </button>
                </li>
                <li class="nav-item">
                  <button class="nav-link" data-bs-toggle="tab" data-bs-target="#docker-tab" type="button">
                    <i class="bi bi-box me-1"></i>Docker
                  </button>
                </li>
              </ul>
              
              <div class="tab-content border border-top-0 rounded-bottom p-3">
                <!-- Linux/macOS -->
                <div class="tab-pane fade show active" id="linux-tab">
                  <p class="text-muted small mb-2">Run this command in your terminal:</p>
                  <div class="position-relative">
                    <pre class="bg-dark text-light p-3 rounded mb-0 small"><code>{{ linuxInstallCommand }}</code></pre>
                    <button 
                      class="btn btn-sm btn-outline-light position-absolute top-0 end-0 m-2"
                      @click="copyCommand(linuxInstallCommand)"
                    >
                      <i :class="state.copied ? 'bi bi-check' : 'bi bi-clipboard'"></i>
                    </button>
                  </div>
                </div>
                
                <!-- Windows -->
                <div class="tab-pane fade" id="windows-tab">
                  <p class="text-muted small mb-2">Run these commands in PowerShell (as Administrator):</p>
                  <div class="position-relative">
                    <pre class="bg-dark text-light p-3 rounded mb-0 small"><code>{{ windowsInstallCommand }}</code></pre>
                    <button 
                      class="btn btn-sm btn-outline-light position-absolute top-0 end-0 m-2"
                      @click="copyCommand(windowsInstallCommand)"
                    >
                      <i :class="state.copied ? 'bi bi-check' : 'bi bi-clipboard'"></i>
                    </button>
                  </div>
                </div>
                
                <!-- Docker -->
                <div class="tab-pane fade" id="docker-tab">
                  <p class="text-muted small mb-2">Run the agent in a Docker container:</p>
                  <div class="position-relative">
                    <pre class="bg-dark text-light p-3 rounded mb-0 small"><code>{{ dockerInstallCommand }}</code></pre>
                    <button 
                      class="btn btn-sm btn-outline-light position-absolute top-0 end-0 m-2"
                      @click="copyCommand(dockerInstallCommand)"
                    >
                      <i :class="state.copied ? 'bi bi-check' : 'bi bi-clipboard'"></i>
                    </button>
                  </div>
                </div>
              </div>
            </div>
            
            <div class="card-footer d-flex justify-content-end">
              <button class="btn btn-success" @click="done">
                <i class="bi bi-check-lg me-1"></i>Done
              </button>
            </div>
          </div>
        </div>
        
        <!-- Info Sidebar -->
        <div class="col-12 col-lg-4 mt-3 mt-lg-0">
          <div class="card bg-light">
            <div class="card-body">
              <h6 class="card-title">
                <i class="bi bi-info-circle me-2"></i>Next Steps
              </h6>
              <ol class="small text-muted mb-0">
                <li>Copy the install command for your platform</li>
                <li>Run the command on your target machine</li>
                <li>The agent will connect automatically</li>
                <li>Existing probes will resume data collection</li>
              </ol>
            </div>
          </div>
          
          <div class="card bg-warning bg-opacity-10 border-warning mt-3">
            <div class="card-body">
              <h6 class="card-title text-warning">
                <i class="bi bi-exclamation-triangle me-2"></i>Important
              </h6>
              <p class="small text-muted mb-0">
                The bootstrap PIN can only be used once. If the installation fails, you'll need to regenerate the agent again.
              </p>
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
.card.border-warning {
  border-width: 2px;
}
.card.border-success {
  border-width: 2px;
}
pre {
  white-space: pre-wrap;
  word-break: break-all;
}
</style>