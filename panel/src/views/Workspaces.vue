<script lang="ts" setup>
import type { Workspace } from "@/types";
import { onMounted, reactive, computed } from "vue";
import Title from "@/components/Title.vue";
import { WorkspaceService, AgentService, AlertService } from "@/services/apiService";

interface WorkspaceStats {
  workspace: Workspace;
  agentCount: number;
  onlineAgents: number;
  memberCount: number;
  alertCount: number;
}

const state = reactive({
  workspaces: [] as WorkspaceStats[],
  ready: false,
  loading: true,
  error: null as string | null,
  searchQuery: "",
});

// Computed properties
const filteredWorkspaces = computed(() => {
  if (!state.searchQuery) return state.workspaces;
  const query = state.searchQuery.toLowerCase();
  return state.workspaces.filter(ws =>
    ws.workspace.name.toLowerCase().includes(query) ||
    ws.workspace.description?.toLowerCase().includes(query)
  );
});

// Aggregate stats
const totalAlerts = computed(() => 
  state.workspaces.reduce((sum, ws) => sum + ws.alertCount, 0)
);

// Check if agent is online (updated within last 2 minutes)
function isAgentOnline(updatedAt: string): boolean {
  const now = new Date();
  const lastSeen = new Date(updatedAt);
  const diffMs = now.getTime() - lastSeen.getTime();
  return diffMs < 2 * 60 * 1000; // 2 minutes
}

async function loadWorkspaceStats(workspace: Workspace): Promise<WorkspaceStats> {
  const stats: WorkspaceStats = {
    workspace,
    agentCount: 0,
    onlineAgents: 0,
    memberCount: 0,
    alertCount: 0,
  };

  try {
    // Fetch agents
    const agentsRes = await AgentService.list(workspace.id);
    stats.agentCount = agentsRes.total || agentsRes.data?.length || 0;
    stats.onlineAgents = agentsRes.data?.filter(a => isAgentOnline(a.updated_at)).length || 0;
  } catch (e) {
    console.warn(`Failed to load agents for workspace ${workspace.id}`, e);
  }

  try {
    // Fetch members
    const members = await WorkspaceService.listMembers(workspace.id);
    stats.memberCount = members?.length || 0;
  } catch (e) {
    console.warn(`Failed to load members for workspace ${workspace.id}`, e);
  }

  return stats;
}

onMounted(async () => {
  try {
    // Get workspace list
    const workspaces = await WorkspaceService.list() as Workspace[];

    if (!workspaces || workspaces.length === 0) {
      state.loading = false;
      return;
    }

    // Get alerts count (global - we'll filter per workspace)
    let alertsByWorkspace: Record<number, number> = {};
    try {
      const alerts = await AlertService.list({ status: 'active' });
      for (const alert of alerts) {
        alertsByWorkspace[alert.workspace_id] = (alertsByWorkspace[alert.workspace_id] || 0) + 1;
      }
    } catch (e) {
      console.warn("Failed to load alerts", e);
    }

    // Load stats for each workspace in parallel
    const statsPromises = workspaces.map(ws => loadWorkspaceStats(ws));
    const allStats = await Promise.all(statsPromises);

    // Add alert counts
    for (const stats of allStats) {
      stats.alertCount = alertsByWorkspace[stats.workspace.id] || 0;
    }

    state.workspaces = allStats;
    state.ready = true;
  } catch (error) {
    console.error("Failed to load workspaces:", error);
    state.error = "Failed to load workspaces. Please try again.";
  } finally {
    state.loading = false;
  }
});
</script>

<template>
  <div class="container-fluid">
    <Title
      title="workspaces"
      subtitle="an overview of the workspaces you have access to"
    >
      <div class="d-flex gap-2">
        <router-link
          to="/workspaces/alerts"
          class="btn btn-outline-danger"
        >
          <i class="bi bi-exclamation-triangle me-2"></i>Alerts
          <span v-if="totalAlerts > 0" class="badge bg-danger ms-1">{{ totalAlerts }}</span>
        </router-link>
        <router-link
          to="/workspaces/new"
          class="btn btn-primary"
        >
          <i class="bi bi-plus-circle me-2"></i>New Workspace
        </router-link>
      </div>
    </Title>

    <!-- Loading State -->
    <div v-if="state.loading" class="text-center py-5">
      <div class="spinner-border text-primary mb-3" role="status">
        <span class="visually-hidden">Loading...</span>
      </div>
      <p class="text-muted mb-0">Loading workspaces...</p>
    </div>

    <!-- Error State -->
    <div v-else-if="state.error" class="alert alert-danger">
      <i class="bi bi-exclamation-circle me-2"></i>{{ state.error }}
    </div>

    <!-- Workspaces List -->
    <div v-else-if="state.ready && state.workspaces.length > 0">
      <!-- Search -->
      <div class="mb-4">
        <div class="input-group" style="max-width: 400px;">
          <span class="input-group-text">
            <i class="bi bi-search"></i>
          </span>
          <input
            v-model="state.searchQuery"
            type="text"
            class="form-control"
            placeholder="Search workspaces..."
          >
        </div>
      </div>

      <!-- Workspace Cards Grid -->
      <div class="workspace-grid">
        <router-link
          v-for="ws in filteredWorkspaces"
          :key="ws.workspace.id"
          :to="`/workspaces/${ws.workspace.id}`"
          class="workspace-card card"
        >
          <div class="card-body">
            <div class="d-flex align-items-start mb-3">
              <div class="workspace-icon me-3">
                <i class="bi bi-building"></i>
              </div>
              <div class="flex-grow-1 min-width-0">
                <h5 class="card-title mb-1">{{ ws.workspace.name }}</h5>
                <p class="card-text text-muted small mb-0">{{ ws.workspace.description || "No description" }}</p>
              </div>
              <span v-if="ws.alertCount > 0" class="badge bg-danger">
                {{ ws.alertCount }} alert{{ ws.alertCount > 1 ? 's' : '' }}
              </span>
            </div>

            <div class="stats-row">
              <div class="stat-item">
                <i class="bi bi-server"></i>
                <span><strong>{{ ws.agentCount }}</strong> agents</span>
              </div>
              <div class="stat-item">
                <i class="bi bi-check-circle text-success"></i>
                <span><strong>{{ ws.onlineAgents }}</strong> online</span>
              </div>
              <div class="stat-item">
                <i class="bi bi-people"></i>
                <span><strong>{{ ws.memberCount }}</strong> members</span>
              </div>
            </div>
          </div>
        </router-link>
      </div>

      <!-- No Results -->
      <div v-if="filteredWorkspaces.length === 0 && state.searchQuery" class="alert alert-info mt-3">
        <i class="bi bi-info-circle me-2"></i>
        No workspaces found matching "{{ state.searchQuery }}"
      </div>
    </div>

    <!-- Empty State -->
    <div v-else class="text-center py-5">
      <div class="empty-icon mb-3">
        <i class="bi bi-building"></i>
      </div>
      <h4>No Workspaces Yet</h4>
      <p class="text-muted mb-4">Create your first workspace to start monitoring.</p>
      <router-link to="/workspaces/new" class="btn btn-primary">
        <i class="bi bi-plus-circle me-2"></i>Create Workspace
      </router-link>
    </div>
  </div>
</template>

<style scoped>
.workspace-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 1rem;
}

.workspace-card {
  text-decoration: none;
  color: inherit;
  transition: transform 0.15s, box-shadow 0.15s;
}

.workspace-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.workspace-icon {
  width: 40px;
  height: 40px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-size: 1.1rem;
  flex-shrink: 0;
}

.stats-row {
  display: flex;
  gap: 1.5rem;
  flex-wrap: wrap;
  padding-top: 0.75rem;
  border-top: 1px solid #eee;
}

.stat-item {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.85rem;
  color: #666;
}

.stat-item i {
  font-size: 0.9rem;
}

.min-width-0 {
  min-width: 0;
}

.card-title {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.empty-icon {
  font-size: 4rem;
  color: #ddd;
}

@media (max-width: 576px) {
  .workspace-grid {
    grid-template-columns: 1fr;
  }
  
  .stats-row {
    gap: 1rem;
  }
}
</style>