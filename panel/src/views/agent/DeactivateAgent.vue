<script lang="ts" setup>
import { onMounted, reactive } from "vue";
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
  error: ""
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

async function submit() {
  if (!state.agent.id || !state.workspace.id) return;
  
  state.loading = true;
  state.error = "";
  
  try {
    await AgentService.update(state.workspace.id, state.agent.id, { active: false });
    router.push(`/workspaces/${state.workspace.id}`);
  } catch (err: any) {
    console.error("Failed to deactivate agent:", err);
    state.error = err?.response?.data?.message || "Failed to deactivate agent. Please try again.";
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
        title="Deactivate Agent"
        subtitle="Temporarily disable this agent"
        :history="[
          { title: 'Workspaces', link: '/workspaces' },
          { title: state.workspace.name, link: `/workspaces/${state.workspace.id}` },
          { title: state.agent.name, link: `/workspaces/${state.workspace.id}/agents/${state.agent.id}` }
        ]"
      />
      
      <div class="row">
        <div class="col-12 col-lg-8">
          <div class="card border-warning">
            <div class="card-header bg-warning text-dark">
              <h5 class="mb-0">
                <i class="bi bi-pause-circle me-2"></i>Deactivate Agent
              </h5>
            </div>
            <div class="card-body">
              <div class="alert alert-info mb-3">
                <i class="bi bi-info-circle me-2"></i>
                <strong>Note:</strong> Deactivating an agent will stop it from collecting data. You can reactivate it later.
              </div>
              
              <p class="mb-3">
                Are you sure you want to deactivate the agent <strong>{{ state.agent.name }}</strong>?
              </p>
              
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
                  Deactivating...
                </span>
                <span v-else>
                  <i class="bi bi-pause-circle me-1"></i>Deactivate Agent
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
                <i class="bi bi-question-circle me-2"></i>What happens next?
              </h6>
              <ul class="small text-muted mb-0">
                <li>The agent will stop collecting monitoring data</li>
                <li>Existing historical data will be preserved</li>
                <li>You can reactivate the agent at any time</li>
                <li>Probes will remain configured but inactive</li>
              </ul>
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
</style>