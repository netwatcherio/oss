<script lang="ts" setup>
import { onMounted, reactive } from "vue";
import type { Probe, Workspace, Agent } from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import { AgentService, ProbeService, WorkspaceService } from "@/services/apiService";

const router = core.router();

const state = reactive({
  workspace: {} as Workspace,
  agent: {} as Agent,
  probe: {} as Probe,
  ready: false,
  loading: false,
  error: ""
});

onMounted(async () => {
  const workspaceId = router.currentRoute.value.params["wID"] as string;
  const agentId = router.currentRoute.value.params["aID"] as string;
  const probeId = router.currentRoute.value.params["pID"] as string;

  if (!workspaceId || !agentId || !probeId) {
    state.error = "Missing workspace, agent, or probe ID";
    return;
  }

  try {
    const [workspace, agent, probe] = await Promise.all([
      WorkspaceService.get(workspaceId),
      AgentService.get(workspaceId, agentId),
      ProbeService.get(workspaceId, agentId, probeId)
    ]);

    state.workspace = workspace as Workspace;
    state.agent = agent as Agent;
    state.probe = probe as Probe;
    state.ready = true;
  } catch (err) {
    console.error("Failed to load data:", err);
    state.error = "Failed to load probe data";
  }
});

function cancel() {
  router.push(`/workspaces/${state.workspace.id}/agents/${state.agent.id}`);
}

async function submit() {
  if (!state.probe.id || !state.workspace.id || !state.agent.id) return;

  state.loading = true;
  state.error = "";

  try {
    await ProbeService.remove(state.workspace.id, state.agent.id, state.probe.id);
    router.push(`/workspaces/${state.workspace.id}/agents/${state.agent.id}`);
  } catch (err: any) {
    console.error("Failed to delete probe:", err);
    state.error = err?.response?.data?.message || "Failed to delete probe. Please try again.";
    state.loading = false;
  }
}

function getProbeTypeLabel(type: string): string {
  const labels: Record<string, string> = {
    PING: "Ping",
    MTR: "MTR Traceroute",
    TRAFFICSIM: "Traffic Simulation",
    SPEEDTEST: "Speed Test",
    DNS: "DNS Monitor",
    RPERF: "RPERF",
    AGENT: "Agent Monitor",
    NETINFO: "Network Info",
    SYSINFO: "System Info"
  };
  return labels[type] || type;
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
        title="Delete Probe"
        subtitle="Remove this probe from the agent"
        :history="[
          { title: 'Workspaces', link: '/workspaces' },
          { title: state.workspace.name, link: `/workspaces/${state.workspace.id}` },
          { title: state.agent.name, link: `/workspaces/${state.workspace.id}/agents/${state.agent.id}` }
        ]"
      />

      <div class="row">
        <div class="col-12 col-lg-8">
          <div class="card border-danger">
            <div class="card-header bg-danger text-white">
              <h5 class="mb-0">
                <i class="bi bi-exclamation-triangle me-2"></i>Delete Probe
              </h5>
            </div>
            <div class="card-body">
              <div class="alert alert-warning mb-3">
                <i class="bi bi-info-circle me-2"></i>
                <strong>Warning:</strong> This will permanently delete the probe and all associated data.
              </div>

              <div class="mb-3">
                <p class="mb-2">Are you sure you want to delete this probe?</p>
                <table class="table table-sm">
                  <tbody>
                    <tr>
                      <th scope="row" style="width: 120px;">Type</th>
                      <td>
                        <span class="badge bg-primary">{{ getProbeTypeLabel(state.probe.type) }}</span>
                      </td>
                    </tr>
                    <tr>
                      <th scope="row">ID</th>
                      <td>{{ state.probe.id }}</td>
                    </tr>
                    <tr v-if="state.probe.targets?.length">
                      <th scope="row">Target</th>
                      <td>{{ state.probe.targets[0]?.target || `Agent #${state.probe.targets[0]?.agent_id}` }}</td>
                    </tr>
                    <tr>
                      <th scope="row">Status</th>
                      <td>
                        <span :class="state.probe.enabled ? 'text-success' : 'text-muted'">
                          {{ state.probe.enabled ? 'Enabled' : 'Disabled' }}
                        </span>
                      </td>
                    </tr>
                  </tbody>
                </table>
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
                class="btn btn-danger"
                @click="submit"
                :disabled="state.loading"
              >
                <span v-if="state.loading">
                  <span class="spinner-border spinner-border-sm me-1"></span>
                  Deleting...
                </span>
                <span v-else>
                  <i class="bi bi-trash me-1"></i>Delete Probe
                </span>
              </button>
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
.card.border-danger {
  border-width: 2px;
}
.table th {
  font-weight: 600;
  color: #6c757d;
}
</style>