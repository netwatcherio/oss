<script lang="ts" setup>
import { onMounted, reactive, computed, ref } from "vue";
import type { Workspace, Agent } from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import { AgentService, WorkspaceService } from "@/services/apiService";

const router = core.router();

const state = reactive({
  workspace: {} as Workspace,
  agent: {} as Agent,
  ready: false,
  error: "",
  agentPin: "",
  loadingPin: false,
});

const copied = ref<string | null>(null);

// GitHub repository for install scripts
const agentGitHubUrl = 'https://github.com/netwatcherio/agent';
const agentReleasesUrl = 'https://github.com/netwatcherio/agent/releases';

// Get controller host from environment or window
function getControllerInfo() {
  const anyWindow = window as any;
  let endpoint = anyWindow?.CONTROLLER_ENDPOINT 
    || import.meta.env?.CONTROLLER_ENDPOINT 
    || `${window.location.protocol}//${window.location.host}`;
  try {
    const url = new URL(endpoint);
    return { host: url.host, ssl: url.protocol === 'https:' };
  } catch {
    return { host: window.location.host, ssl: window.location.protocol === 'https:' };
  }
}

// Linux/macOS install script command
const linuxInstallCommand = computed(() => {
  if (!state.agent.id || !state.agentPin) return "";
  const { host, ssl } = getControllerInfo();
  return `curl -fsSL https://raw.githubusercontent.com/netwatcherio/agent/refs/heads/master/install.sh | sudo bash -s -- \\
  --host ${host} \\
  --ssl ${ssl} \\
  --workspace ${state.workspace.id} \\
  --id ${state.agent.id} \\
  --pin ${state.agentPin}`;
});

// Windows PowerShell install command  
const windowsInstallCommand = computed(() => {
  if (!state.agent.id || !state.agentPin) return "";
  const { host, ssl } = getControllerInfo();
  return `# Download the installer
Invoke-WebRequest -Uri "https://raw.githubusercontent.com/netwatcherio/agent/refs/heads/master/install.ps1" -OutFile "install.ps1"
# Run the installer
.\\install.ps1 -ControllerHost "${host}" -SSL $${ssl ? 'true' : 'false'} -Workspace ${state.workspace.id} -Id ${state.agent.id} -Pin "${state.agentPin}"`;
});

// Docker command
const dockerInstallCommand = computed(() => {
  if (!state.agent.id || !state.agentPin) return "";
  const { host, ssl } = getControllerInfo();
  return `docker run -d --name netwatcher-agent \\
  -e CONTROLLER_HOST="${host}" \\
  -e CONTROLLER_SSL="${ssl}" \\
  -e WORKSPACE_ID="${state.workspace.id}" \\
  -e AGENT_ID="${state.agent.id}" \\
  -e AGENT_PIN="${state.agentPin}" \\
  --restart unless-stopped \\
  netwatcher/agent:latest`;
});

onMounted(async () => {
  const workspaceID = router.currentRoute.value.params["wID"] as string;
  const agentID = router.currentRoute.value.params["aID"] as string;
  if (!workspaceID || !agentID) {
    state.error = "Missing workspace or agent ID";
    return;
  }

  try {
    const [workspace, agent] = await Promise.all([
      WorkspaceService.get(workspaceID),
      AgentService.get(workspaceID, agentID),
    ]);
    state.workspace = workspace as Workspace;
    state.agent = agent as Agent;

    // If agent is already initialized, go straight to the agent page
    if (state.agent.initialized) {
      router.replace(`/workspaces/${workspaceID}/agents/${agentID}`);
      return;
    }

    // Fetch the pending PIN
    state.loadingPin = true;
    try {
      const pinResult = await AgentService.getPendingPin(workspaceID, agentID);
      state.agentPin = pinResult.pin || "";
    } catch {
      console.log("No pending PIN available");
    } finally {
      state.loadingPin = false;
    }

    state.ready = true;
  } catch (err) {
    console.error("Failed to load agent setup data:", err);
    state.error = "Failed to load agent setup information";
  }
});

async function copyToClipboard(text: string, field: string) {
  try {
    await navigator.clipboard.writeText(text);
    copied.value = field;
    setTimeout(() => { copied.value = null; }, 2000);
  } catch (err) {
    console.error("Failed to copy:", err);
  }
}

function goToAgent() {
  router.push(`/workspaces/${state.workspace.id}/agents/${state.agent.id}`);
}
</script>

<template>
  <div class="container-fluid">
    <!-- Error state -->
    <div v-if="state.error" class="alert alert-danger mt-3">
      <i class="bi bi-exclamation-circle me-2"></i>{{ state.error }}
    </div>

    <!-- Loading state -->
    <div v-else-if="!state.ready" class="d-flex justify-content-center py-5">
      <div class="spinner-border text-primary" role="status">
        <span class="visually-hidden">Loading...</span>
      </div>
    </div>

    <div v-if="state.ready">
      <Title 
        title="Agent Setup" 
        subtitle="Connect your agent to NetWatcher" 
        :history="[
          { title: 'Workspaces', link: '/workspaces' }, 
          { title: state.workspace.name, link: `/workspaces/${state.workspace.id}` },
          { title: state.agent.name, link: `/workspaces/${state.workspace.id}/agents/${state.agent.id}` }
        ]"
      />

      <!-- Success banner -->
      <div class="setup-banner">
        <div class="setup-banner-icon">
          <i class="bi bi-check-circle-fill"></i>
        </div>
        <div>
          <h4 class="mb-1">Agent Created Successfully!</h4>
          <p class="mb-0 text-muted">Use the information below to connect your agent to NetWatcher.</p>
        </div>
      </div>

      <div class="row mt-4">
        <div class="col-12 col-lg-8">
          <!-- Agent PIN -->
          <div class="card mb-3">
            <div class="card-header">
              <h6 class="mb-0"><i class="bi bi-key me-2"></i>Agent PIN</h6>
            </div>
            <div class="card-body">
              <div v-if="state.loadingPin" class="text-center py-3">
                <div class="spinner-border spinner-border-sm text-primary"></div>
                <span class="ms-2">Loading PIN...</span>
              </div>
              <div v-else-if="state.agentPin">
                <div class="pin-display">
                  <code class="pin-code">{{ state.agentPin }}</code>
                  <button 
                    class="btn btn-sm btn-outline-primary"
                    @click="copyToClipboard(state.agentPin, 'pin')"
                  >
                    <i class="bi" :class="copied === 'pin' ? 'bi-check' : 'bi-clipboard'"></i>
                  </button>
                </div>
                <p class="text-muted small mt-2 mb-0">
                  <i class="bi bi-clock me-1"></i>
                  This PIN is available until the agent connects and activates. You can revisit this page to view it.
                </p>
              </div>
              <div v-else class="text-muted">
                <i class="bi bi-info-circle me-1"></i>
                No pending PIN available. The agent may have already been initialized.
              </div>
            </div>
          </div>

          <!-- Installation Resources -->
          <div class="card mb-3">
            <div class="card-header">
              <h6 class="mb-0"><i class="bi bi-github me-2"></i>Installation Resources</h6>
            </div>
            <div class="card-body">
              <div class="d-flex gap-2 flex-wrap">
                <a :href="agentGitHubUrl" target="_blank" class="btn btn-outline-dark btn-sm">
                  <i class="bi bi-github me-1"></i>Agent Repository
                </a>
                <a :href="agentReleasesUrl" target="_blank" class="btn btn-outline-primary btn-sm">
                  <i class="bi bi-download me-1"></i>Download Releases
                </a>
              </div>
            </div>
          </div>

          <!-- Linux/macOS Install -->
          <div class="card mb-3">
            <div class="card-header">
              <h6 class="mb-0"><i class="bi bi-terminal me-2"></i>Linux / macOS (Recommended)</h6>
            </div>
            <div class="card-body p-0">
              <div class="command-block">
                <pre class="command-code">{{ linuxInstallCommand }}</pre>
                <button 
                  class="btn btn-sm copy-btn"
                  @click="copyToClipboard(linuxInstallCommand, 'linux')"
                >
                  <i class="bi" :class="copied === 'linux' ? 'bi-check' : 'bi-clipboard'"></i> Copy
                </button>
              </div>
            </div>
          </div>

          <!-- Windows PowerShell -->
          <div class="card mb-3">
            <div class="card-header">
              <h6 class="mb-0"><i class="bi bi-windows me-2"></i>Windows PowerShell</h6>
            </div>
            <div class="card-body p-0">
              <div class="command-block">
                <pre class="command-code">{{ windowsInstallCommand }}</pre>
                <button 
                  class="btn btn-sm copy-btn"
                  @click="copyToClipboard(windowsInstallCommand, 'windows')"
                >
                  <i class="bi" :class="copied === 'windows' ? 'bi-check' : 'bi-clipboard'"></i> Copy
                </button>
              </div>
            </div>
          </div>

          <!-- Docker -->
          <div class="card mb-3">
            <div class="card-header">
              <h6 class="mb-0"><i class="bi bi-box me-2"></i>Docker (Alternative)</h6>
            </div>
            <div class="card-body p-0">
              <div class="command-block">
                <pre class="command-code">{{ dockerInstallCommand }}</pre>
                <button 
                  class="btn btn-sm copy-btn"
                  @click="copyToClipboard(dockerInstallCommand, 'docker')"
                >
                  <i class="bi" :class="copied === 'docker' ? 'bi-check' : 'bi-clipboard'"></i> Copy
                </button>
              </div>
            </div>
          </div>

          <!-- Done button -->
          <div class="d-flex justify-content-end mb-4">
            <button class="btn btn-primary" @click="goToAgent">
              <i class="bi bi-check-lg me-1"></i>Done
            </button>
          </div>
        </div>

        <!-- Sidebar -->
        <div class="col-12 col-lg-4">
          <!-- Agent Details -->
          <div class="card mb-3">
            <div class="card-header">
              <h6 class="mb-0"><i class="bi bi-info-circle me-2"></i>Agent Details</h6>
            </div>
            <div class="card-body">
              <table class="table table-sm mb-0">
                <tbody>
                  <tr>
                    <td class="text-muted">Agent ID</td>
                    <td><code>{{ state.agent.id }}</code></td>
                  </tr>
                  <tr>
                    <td class="text-muted">Name</td>
                    <td>{{ state.agent.name }}</td>
                  </tr>
                  <tr v-if="state.agent.location">
                    <td class="text-muted">Location</td>
                    <td>{{ state.agent.location }}</td>
                  </tr>
                  <tr>
                    <td class="text-muted">Workspace ID</td>
                    <td><code>{{ state.workspace.id }}</code></td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- Help -->
          <div class="card">
            <div class="card-body">
              <h6 class="card-title">
                <i class="bi bi-lightbulb me-2"></i>Next Steps
              </h6>
              <ol class="small text-muted mb-0">
                <li>Copy the install command for your OS</li>
                <li>Run it on the machine where the agent will run</li>
                <li>The agent will automatically connect using the PIN</li>
                <li>Once connected, configure probes to start monitoring</li>
              </ol>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.setup-banner {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px 24px;
  background: var(--bs-success-bg-subtle, #d1e7dd);
  border: 1px solid var(--bs-success-border-subtle, #badbcc);
  border-radius: 12px;
  color: var(--bs-success-text-emphasis, #0a3622);
}

.setup-banner-icon {
  font-size: 2.5rem;
  color: var(--bs-success, #198754);
  flex-shrink: 0;
}

.setup-banner h4 {
  color: var(--bs-success-text-emphasis, #0a3622);
}

.pin-display {
  display: flex;
  align-items: center;
  gap: 12px;
  background: var(--bs-tertiary-bg, #f8f9fa);
  padding: 12px 16px;
  border-radius: 8px;
  border: 1px solid var(--bs-border-color, #dee2e6);
}

.pin-code {
  font-size: 1.5rem;
  font-weight: 700;
  letter-spacing: 3px;
  color: var(--bs-primary, #0d6efd);
  flex: 1;
}

.command-block {
  position: relative;
  background: #1e1e1e;
  border-radius: 0 0 var(--bs-card-inner-border-radius) var(--bs-card-inner-border-radius);
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

.table td {
  padding: 8px 0;
}

.card-header {
  background-color: var(--bs-tertiary-bg, #f8f9fa);
  border-bottom: 1px solid var(--bs-border-color, #e9ecef);
}
</style>
