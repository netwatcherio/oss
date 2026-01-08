<script lang="ts" setup>
import {computed, onMounted, reactive} from "vue";
import core from "@/core";
import Title from "@/components/Title.vue";
import Loader from "@/components/Loader.vue";
import Code from "@/components/Code.vue";
import AgentCard from "@/components/AgentCard.vue";
import {AgentService, ProbeService, WorkspaceService} from "@/services/apiService";
import type {Agent, NetInfoPayload, Workspace, Role} from "@/types"
import {usePermissions} from "@/composables/usePermissions";
// --- STATE: add stores for net-info by agent ---
const state = reactive({
  workspace: {} as Workspace & { my_role?: Role },
  agents: [] as Agent[],
  netInfoByAgent: {} as Record<number, NetInfoPayload>,
  ready: false,
  loading: true,
  loadingNetInfo: false,
  searchQuery: '',
  sortBy: 'status' as 'status' | 'name' | 'description' | 'updated'
})

// Permissions based on user's role in this workspace
const permissions = computed(() => usePermissions(state.workspace.my_role));

// Fetch all net-infos for currently loaded agents and populate state
async function fetchAllNetInfo(workspaceId: string) {
  state.loadingNetInfo = true
  try {
    return await Promise.all(
        state.agents.map(async (agent) => {
          try {
            // Assumes ProbeService.netInfo returns an array of ProbeData for that agent
            const res = await ProbeService.netInfo(workspaceId, agent.id)
            // Store
            state.netInfoByAgent[agent.id] = res.payload as NetInfoPayload
          } catch (e) {
            // On per-agent error, just initialize empty so callers can rely on key presence
            state.netInfoByAgent[agent.id] = {} as NetInfoPayload
          }
        })
    )
  } finally {
    state.loadingNetInfo = false
  }
}

let router = core.router()

// Computed properties for filtering and sorting
const filteredAgents = computed(() => {
  let filtered = state.agents;

  // Apply search filter
  if (state.searchQuery) {
    const query = state.searchQuery.toLowerCase();
    filtered = filtered.filter(agent =>
      agent.name.toLowerCase().includes(query) ||
      agent.location?.toLowerCase().includes(query) ||
      agent.id
    );
  }

  // Apply sorting
  return filtered.sort((a, b) => {
    switch (state.sortBy) {
      case 'status':
        return getOnlineStatus(b) - getOnlineStatus(a);
      case 'name':
        return a.name.localeCompare(b.name);
      case 'description':
        return (a.description || '').localeCompare(b.description || '');
      case 'updated':
        return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime();
      default:
        return 0;
    }
  });
});

const onlineAgentsCount = computed(() =>
    state.agents.filter(agent => getOnlineStatus(agent)).length
);

const offlineAgentsCount = computed(() =>
    state.agents.filter(agent => !getOnlineStatus(agent)).length
);

function getOnlineStatus(agent: Agent) {
  const currentTime = new Date();
  const agentTime = new Date(agent.updated_at)
  const timeDifference = (currentTime.getTime() - agentTime.getTime()) / 60000;
  return timeDifference <= 1;
}

function getLastSeenText(agent: Agent) {
  const now = new Date();
  const lastSeen = new Date(agent.updated_at?.toString());
  const diffMs = now.getTime() - lastSeen.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);
  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${diffDays}d ago`;
}

onMounted(async () => {
  const id = router.currentRoute.value.params["wID"] as string
  if (!id) return

  try {
    state.loading = true

    const ws = await WorkspaceService.get(id)
    state.workspace = ws as Workspace

    const res = await AgentService.list(id)
    state.agents = res.data
    state.ready = state.agents.length > 0

    // fetch all net infos for loaded agents
    await fetchAllNetInfo(id)
  } catch (e) {
    // swallow/log as needed
    console.log(state.netInfoByAgent)
  } finally {
    state.loading = false
  }
})
</script>

<template>
  <div class="container-fluid">
    <Title :title="state.workspace.name || 'Loading...'" :history="[{title: 'workspaces', link: '/workspaces'}]">
      <div class="d-flex flex-wrap gap-2">
        <router-link 
          v-if="permissions.canManage.value" 
          :to="`/workspaces/${state.workspace.id}/edit`" 
          class="btn btn-outline-dark"
        >
          <i class="bi bi-pencil"></i>
          <span class="d-none d-sm-inline">&nbsp;Edit</span>
        </router-link>
        <router-link :to="`/workspaces/${state.workspace.id}/members`" class="btn btn-outline-dark">
          <i class="bi bi-people"></i>
          <span class="d-none d-sm-inline">&nbsp;Members</span>
        </router-link>
        <router-link 
          v-if="permissions.canEdit.value" 
          :to="`/workspaces/${state.workspace.id}/agents/new`" 
          class="btn btn-primary"
        >
          <i class="bi bi-plus-lg"></i>&nbsp;Create Agent
        </router-link>
      </div>
    </Title>

    <!-- Stats Cards -->
    <div class="stats-container mb-4" v-if="!state.loading">
      <div class="stat-card">
        <div class="stat-icon">
          <i class="bi bi-server"></i>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ state.agents.length }}</div>
          <div class="stat-label">Total Agents</div>
        </div>
      </div>
      <div class="stat-card success">
        <div class="stat-icon">
          <i class="bi bi-check-circle-fill"></i>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ onlineAgentsCount }}</div>
          <div class="stat-label">Online</div>
        </div>
      </div>
      <div class="stat-card danger">
        <div class="stat-icon">
          <i class="bi bi-x-circle-fill"></i>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ offlineAgentsCount }}</div>
          <div class="stat-label">Offline</div>
        </div>
      </div>
    </div>

    <!-- Search and Filter Bar -->
    <div class="filter-bar mb-3" v-if="state.agents.length > 0">
      <div class="search-box">
        <i class="bi bi-search search-icon"></i>
        <input 
          v-model="state.searchQuery" 
          type="text" 
          class="form-control" 
          placeholder="Search agents by name, location, or ID..."
        >
      </div>
      <div class="sort-dropdown">
        <label class="d-none d-md-inline me-2">Sort by:</label>
        <select v-model="state.sortBy" class="form-select">
          <option value="status">Status</option>
          <option value="name">Name</option>
          <option value="location">Location</option>
          <option value="updated">Last Updated</option>
        </select>
      </div>
    </div>

    <!-- Loading State -->
    <div v-if="state.loading" class="text-center py-5">
      <Loader />
      <p class="text-muted mt-3">Loading agents...</p>
    </div>

    <!-- Empty State -->
    <div v-else-if="state.agents.length === 0" class="empty-state card">
      <div class="empty-state-icon">
        <i class="bi bi-server"></i>
      </div>
      <h5>No agents yet</h5>
      <p class="text-muted">Create your first agent to start monitoring your network.</p>
      <router-link :to="`/workspaces/${state.workspace.id}/agents/new`" class="btn btn-primary">
        <i class="bi bi-plus-lg"></i>&nbsp;Create First Agent
      </router-link>
    </div>

    <!-- No Results State -->
    <div v-else-if="filteredAgents.length === 0" class="empty-state card">
      <div class="empty-state-icon">
        <i class="bi bi-search"></i>
      </div>
      <h5>No agents found</h5>
      <p class="text-muted">Try adjusting your search criteria.</p>
      <button @click="state.searchQuery = ''" class="btn btn-outline-primary">
        Clear Search
      </button>
    </div>

    <!-- Agents Grid -->
    <div class="agents-grid" v-else>
      <div v-for="agent in state.agents" :key="agent.id" class="agent-card-wrapper">
        <AgentCard
          :title="agent.name"
          :subtitle="(agent.version?' ':'') + agent.location"
          :icon="getOnlineStatus(agent)?'bi bi-check-circle-fill text-success':'bi bi-x-circle-fill text-danger'"
          class="h-100"
        >

          <template #secondary>
            <div class="last-seen-badge">
              {{ getLastSeenText(agent) }}
            </div>
          </template>
          
          <div class="agent-card-content">
          <!--      todo add section panel on agent creation for pin      -->
            
            <div class="agent-stats">
              <div class="mini-stat" v-if="(state.netInfoByAgent[agent.id] as NetInfoPayload).internet_provider">
                <i class="bi bi-signpost"></i>
                <span>{{ (state.netInfoByAgent[agent.id] as NetInfoPayload).internet_provider}} </span>
              </div>

              <div class="mini-stat">
                <i class="bi bi-clock"></i>
                <span>{{ getLastSeenText(agent) }}</span>
              </div>
              <div class="mini-stat" v-if="agent.location">
                <i class="bi bi-geo-alt-fill"></i>
                <span>{{ agent.location }}</span>
              </div>
              <div class="mini-stat" v-if="agent.version">
                <i class="bi bi-hammer"></i>
                <span>{{agent.version}}</span>
              </div>
            </div>
          </div>
          
          <div class="agent-actions">
            <router-link 
              v-if="agent.initialized" 
              :to="`/workspaces/${agent.workspace_id}/agents/${agent.id}/deactivate`"
              class="btn btn-sm btn-outline-warning"
              title="Deactivate agent"
            >
              <i class="bi bi-moon-stars"></i>
              <span class="d-none d-lg-inline">&nbsp;Deactivate</span>
            </router-link>
            <router-link 
              :to="`/workspaces/${agent.workspace_id}/agents/${agent.id}/edit`"
              class="btn btn-sm btn-outline-success"
              title="Edit agent"
            >
              <i class="bi bi-pencil"></i>
              <span class="d-none d-lg-inline">&nbsp;Edit</span>
            </router-link>
            <router-link 
              :to="`/workspaces/${agent.workspace_id}/agents/${agent.id}`"
              class="btn btn-sm btn-primary"
              title="View agent details"
            >
              View&nbsp;<i class="bi bi-chevron-right"></i>
            </router-link>
          </div>
        </AgentCard>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Stats Container */
.stats-container {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 1rem;
}

.stat-card {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 1.5rem;
  display: flex;
  align-items: center;
  gap: 1rem;
  transition: all 0.2s;
}

.stat-card:hover {
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
  transform: translateY(-2px);
}

.stat-card.success {
  border-color: #10b981;
  background: #f0fdf4;
}

.stat-card.success .stat-icon {
  color: #10b981;
}

.stat-card.danger {
  border-color: #ef4444;
  background: #fef2f2;
}

.stat-card.danger .stat-icon {
  color: #ef4444;
}

.stat-icon {
  font-size: 2rem;
  color: #6b7280;
}

.stat-content {
  flex: 1;
}

.stat-value {
  font-size: 1.875rem;
  font-weight: 700;
  color: #1f2937;
  line-height: 1;
}

.stat-label {
  font-size: 0.875rem;
  color: #6b7280;
  margin-top: 0.25rem;
}

/* Filter Bar */
.filter-bar {
  display: flex;
  gap: 1rem;
  flex-wrap: wrap;
}

.search-box {
  flex: 1;
  min-width: 250px;
  position: relative;
}

.search-icon {
  position: absolute;
  left: 1rem;
  top: 50%;
  transform: translateY(-50%);
  color: #6b7280;
  pointer-events: none;
}

.search-box input {
  padding-left: 2.75rem;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
}

.search-box input:focus {
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.sort-dropdown {
  display: flex;
  align-items: center;
  min-width: 200px;
}

.sort-dropdown .form-select {
  border-radius: 8px;
  border: 1px solid #e5e7eb;
}

/* Empty States */
.empty-state {
  text-align: center;
  padding: 4rem 2rem;
  background: white;
  border-radius: 8px;
}

.empty-state-icon {
  font-size: 4rem;
  color: #e5e7eb;
  margin-bottom: 1.5rem;
}

.empty-state h5 {
  margin-bottom: 0.75rem;
  color: #1f2937;
}

.empty-state p {
  margin-bottom: 1.5rem;
}

/* Agents Grid */
.agents-grid {
  display: grid;
  gap: 1rem;
  grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
}

@media (max-width: 767px) {
  .agents-grid {
    grid-template-columns: 1fr;
  }
}

.agent-card-wrapper {
  display: flex;
}

.agent-card-wrapper :deep(.card) {
  width: 100%;
  transition: all 0.2s;
  border: 1px solid #e5e7eb;
}

.agent-card-wrapper :deep(.card):hover {
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
  transform: translateY(-2px);
}

/* Agent Card Content */
.last-seen-badge {
  font-size: 0.75rem;
  color: #6b7280;
  background: #f3f4f6;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
}

.agent-card-content {
  padding: 1rem;
  flex: 1;
}

.credentials-section {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.agent-stats {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.mini-stat {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
  color: #6b7280;
}

.mini-stat i {
  width: 1rem;
  text-align: center;
}

.agent-actions {
  padding: 1rem;
  background: #f9fafb;
  border-top: 1px solid #e5e7eb;
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
  flex-wrap: wrap;
}

/* Button Improvements */
.btn {
  transition: all 0.2s;
}

.btn-sm {
  padding: 0.375rem 0.75rem;
  font-size: 0.875rem;
}

/* Responsive Adjustments */
@media (max-width: 576px) {
  .stats-container {
    grid-template-columns: 1fr;
  }
  
  .filter-bar {
    flex-direction: column;
  }
  
  .search-box,
  .sort-dropdown {
    width: 100%;
  }
  
  .agent-actions {
    justify-content: stretch;
  }
  
  .agent-actions .btn {
    flex: 1;
    text-align: center;
  }
}

@media (max-width: 768px) {
  .agent-actions .btn-sm {
    padding: 0.5rem;
  }
}

/* Loading Animation */
@keyframes pulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.5;
  }
}

.loading {
  animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}
</style>