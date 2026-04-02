<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Agent, Workspace } from '@/types';

interface ControllerInfo {
  host: string;
  ssl: boolean;
}

interface Props {
  agent: Agent;
  workspace: Workspace;
  pendingPin: string;
  isLoadingPin: boolean;
}

const props = defineProps<Props>();

const copiedPinField = ref<string | null>(null);

function getControllerInfo(): ControllerInfo {
  const anyWindow = window as any;
  let endpoint = anyWindow?.CONTROLLER_ENDPOINT 
    || (import.meta as any).env?.CONTROLLER_ENDPOINT 
    || `${window.location.protocol}//${window.location.host}`;
  
  try {
    const url = new URL(endpoint);
    return { host: url.host, ssl: url.protocol === 'https:' };
  } catch {
    return { host: window.location.host, ssl: window.location.protocol === 'https:' };
  }
}

const controllerInfo = computed(() => getControllerInfo());

const linuxInstallCmd = computed(() => {
  if (!props.agent.id || !props.pendingPin) return '';
  const { host, ssl } = controllerInfo.value;
  return `curl -fsSL https://raw.githubusercontent.com/netwatcherio/agent/refs/heads/master/install.sh | sudo bash -s -- \\
  --host ${host} \\
  --ssl ${ssl} \\
  --workspace ${props.workspace.id} \\
  --id ${props.agent.id} \\
  --pin ${props.pendingPin}`;
});

const windowsInstallCmd = computed(() => {
  if (!props.agent.id || !props.pendingPin) return '';
  const { host, ssl } = controllerInfo.value;
  return `Invoke-WebRequest -Uri "https://raw.githubusercontent.com/netwatcherio/agent/refs/heads/master/install.ps1" -OutFile "install.ps1"
powershell -ExecutionPolicy Bypass -File install.ps1 -ControllerHost "${host}" -SSL $${ssl ? 'true' : 'false'} -Workspace ${props.workspace.id} -Id ${props.agent.id} -Pin "${props.pendingPin}"`;
});

const dockerInstallCmd = computed(() => {
  if (!props.agent.id || !props.pendingPin) return '';
  const { host, ssl } = controllerInfo.value;
  return `docker run -d --name netwatcher-agent \\
  -e CONTROLLER_HOST="${host}" \\
  -e CONTROLLER_SSL="${ssl}" \\
  -e WORKSPACE_ID="${props.workspace.id}" \\
  -e AGENT_ID="${props.agent.id}" \\
  -e AGENT_PIN="${props.pendingPin}" \\
  --restart unless-stopped \\
  netwatcher/agent:latest`;
});

async function copyPinText(text: string, field: string) {
  try {
    await navigator.clipboard.writeText(text);
    copiedPinField.value = field;
    setTimeout(() => { copiedPinField.value = null; }, 2000);
  } catch (err) {
    console.error('Failed to copy:', err);
  }
}
</script>

<template>
  <div class="empty-state">
    <i class="bi bi-exclamation-triangle-fill text-warning"></i>
    <h5>Agent Not Initialized</h5>
    <p>This agent needs to be initialized before it can be used.</p>
    
    <!-- Loading PIN -->
    <div v-if="isLoadingPin" class="mt-3">
      <div class="spinner-border spinner-border-sm text-primary" role="status"></div>
      <span class="ms-2">Loading PIN...</span>
    </div>

    <!-- PIN Display and Install Commands -->
    <div v-else-if="pendingPin" class="card mt-3 text-start init-card">
      <div class="card-header">
        <h6 class="mb-0">
          <i class="bi bi-key me-2"></i>Bootstrap PIN
        </h6>
      </div>
      
      <div class="card-body">
        <!-- PIN Code -->
        <div class="pin-display">
          <code class="pin-code">{{ pendingPin }}</code>
          <button 
            class="btn btn-sm btn-outline-primary" 
            @click="copyPinText(pendingPin, 'pin')"
          >
            <i :class="copiedPinField === 'pin' ? 'bi bi-check' : 'bi bi-clipboard'"></i>
          </button>
        </div>
        
        <p class="text-muted small mb-3">
          <i class="bi bi-info-circle me-1"></i>
          This PIN is available until the agent connects and activates. You can revisit this page to view it.
        </p>

        <!-- Linux Install -->
        <h6 class="mt-3">
          <i class="bi bi-terminal me-2"></i>Linux / macOS
        </h6>
        <div class="code-block">
          <pre class="mb-0">{{ linuxInstallCmd }}</pre>
          <button 
            class="btn btn-sm copy-btn" 
            @click="copyPinText(linuxInstallCmd, 'linux')"
          >
            <i :class="copiedPinField === 'linux' ? 'bi bi-check' : 'bi bi-clipboard'"></i> 
            Copy
          </button>
        </div>

        <!-- Windows Install -->
        <h6 class="mt-3">
          <i class="bi bi-windows me-2"></i>Windows PowerShell
        </h6>
        <div class="code-block">
          <pre class="mb-0">{{ windowsInstallCmd }}</pre>
          <button 
            class="btn btn-sm copy-btn" 
            @click="copyPinText(windowsInstallCmd, 'windows')"
          >
            <i :class="copiedPinField === 'windows' ? 'bi bi-check' : 'bi bi-clipboard'"></i> 
            Copy
          </button>
        </div>

        <!-- Docker Install -->
        <h6 class="mt-3">
          <i class="bi bi-box me-2"></i>Docker
        </h6>
        <div class="code-block">
          <pre class="mb-0">{{ dockerInstallCmd }}</pre>
          <button 
            class="btn btn-sm copy-btn" 
            @click="copyPinText(dockerInstallCmd, 'docker')"
          >
            <i :class="copiedPinField === 'docker' ? 'bi bi-check' : 'bi bi-clipboard'"></i> 
            Copy
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.empty-state {
  text-align: center;
  padding: 3rem 1rem;
}

.empty-state > i {
  font-size: 3rem;
  margin-bottom: 1rem;
  display: block;
}

.empty-state h5 {
  margin-bottom: 0.5rem;
  color: #212529;
}

.empty-state p {
  color: #6c757d;
  margin-bottom: 1rem;
}

.init-card {
  max-width: 720px;
  margin: 0 auto;
  border: 1px solid rgba(0, 0, 0, 0.1);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
}

.init-card .card-header {
  background: rgba(13, 110, 253, 0.05);
  border-bottom: 1px solid rgba(13, 110, 253, 0.1);
}

.pin-display {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 1rem;
  padding: 1rem;
  background: var(--bs-tertiary-bg, #f8f9fa);
  border: 1px solid var(--bs-border-color, #dee2e6);
  border-radius: 0.5rem;
}

.pin-code {
  font-size: 1.5rem;
  font-weight: 700;
  letter-spacing: 3px;
  color: var(--bs-primary, #0d6efd);
  flex: 1;
  background: transparent;
}

.code-block {
  position: relative;
  background: #1e1e1e;
  border-radius: 8px;
  overflow: hidden;
}

.code-block pre {
  padding: 1rem;
  color: #d4d4d4;
  font-size: 0.85rem;
  overflow-x: auto;
  white-space: pre;
  margin: 0;
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
  color: #ffffff;
}

h6 {
  font-size: 0.875rem;
  font-weight: 600;
  color: #495057;
  margin-bottom: 0.5rem;
}

/* Spinner alignment */
.spinner-border {
  vertical-align: middle;
}

/* Responsive adjustments */
@media (max-width: 575px) {
  .empty-state {
    padding: 2rem 0.5rem;
  }

  .init-card {
    margin: 0 -0.5rem;
    border-radius: 0;
  }

  .pin-code {
    font-size: 1.25rem;
    letter-spacing: 2px;
  }

  .code-block pre {
    font-size: 0.75rem;
    padding: 0.75rem;
  }
}
</style>
