<script lang="ts" setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import type { ConnectivityMatrix, ConnectivityMatrixEntry, ProbeStatusSummary, TargetLabel, AgentSummary } from '@/types'
import MatrixCell from './MatrixCell.vue'
import request from '@/services/request'

const props = defineProps<{
  workspaceId: number
}>()

const loading = ref(true)
const error = ref<string | null>(null)
const matrixData = ref<ConnectivityMatrix | null>(null)
const selectedLookback = ref(15)
const selectedCell = ref<{ entry: ConnectivityMatrixEntry; rect: DOMRect } | null>(null)
const selectedTarget = ref<{ target: TargetLabel; rect: DOMRect } | null>(null)
let refreshInterval: ReturnType<typeof setInterval> | null = null

// Fetch matrix data from backend
async function fetchMatrix() {
  try {
    loading.value = true
    error.value = null
    const response = await request.get<ConnectivityMatrix>(
      `/workspaces/${props.workspaceId}/connectivity-matrix?lookback=${selectedLookback.value}`
    )
    matrixData.value = response.data
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Unknown error'
    console.error('Failed to fetch matrix:', e)
  } finally {
    loading.value = false
  }
}

// Get entry for a specific source-target pair
function getEntry(sourceId: number, targetId: string): ConnectivityMatrixEntry | undefined {
  return matrixData.value?.entries.find(
    e => e.source_agent_id === sourceId && e.target_id === targetId
  )
}

// Handle cell click for popover
function handleCellClick(entry: ConnectivityMatrixEntry | undefined, event: MouseEvent) {
  if (!entry || entry.probe_status.length === 0) return
  selectedTarget.value = null // Close target popup if open
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  selectedCell.value = { entry, rect }
}

// Handle column header click
function handleTargetClick(target: TargetLabel, event: MouseEvent) {
  selectedCell.value = null // Close cell popup if open
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  selectedTarget.value = { target, rect }
}

// Handle source row header click
const selectedSource = ref<{ source: AgentSummary; rect: DOMRect } | null>(null)

function handleSourceClick(source: AgentSummary, event: MouseEvent) {
  selectedCell.value = null
  selectedTarget.value = null
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  selectedSource.value = { source, rect }
}

function closePopover() {
  selectedCell.value = null
  selectedTarget.value = null
  selectedSource.value = null
}

// Get all entries for a specific target (for target popup)
const targetEntries = computed(() => {
  if (!selectedTarget.value || !matrixData.value) return []
  const targetId = selectedTarget.value.target.id
  return matrixData.value.entries.filter(e => e.target_id === targetId)
})

// Get all entries for a specific source agent (for source popup)
const sourceEntries = computed(() => {
  if (!selectedSource.value || !matrixData.value) return []
  const sourceId = selectedSource.value.source.id
  return matrixData.value.entries.filter(e => e.source_agent_id === sourceId)
})

// Get agent ID from target if it's an agent type
function getAgentIdFromTarget(target: TargetLabel): number | null {
  if (target.type === 'agent' && target.id.startsWith('agent:')) {
    return parseInt(target.id.substring(6), 10)
  }
  return null
}

// Find agent by IP address (for reverse probe detection)
function findAgentByIp(ip: string): AgentSummary | null {
  if (!matrixData.value) return null
  // Check if any source agent has this IP in their name or if target matches an agent
  return matrixData.value.source_agents.find(agent => 
    agent.name.includes(ip) || ip.includes(agent.name.split('.')[0] || '')
  ) || null
}

// Check if this is an agent-to-agent connection
function isAgentToAgent(targetId: string): boolean {
  return targetId.startsWith('agent:')
}

// Get reverse path entry (Agent B -> Agent A) for bidirectional display
function getReversePath(sourceAgentId: number, targetId: string): ConnectivityMatrixEntry | undefined {
  if (!matrixData.value || !targetId.startsWith('agent:')) return undefined
  const targetAgentId = parseInt(targetId.substring(6), 10)
  // Find entry where source is the target agent and target is the source agent
  return matrixData.value.entries.find(
    e => e.source_agent_id === targetAgentId && e.target_id === `agent:${sourceAgentId}`
  )
}

// Smart popup positioning to stay within viewport
const targetPopoverStyle = computed(() => {
  if (!selectedTarget.value) return {}
  const rect = selectedTarget.value.rect
  const popupWidth = 360
  const popupMaxHeight = 400
  const padding = 16
  
  // Center horizontally relative to the header cell
  let left = rect.left + (rect.width / 2) - (popupWidth / 2)
  
  // Keep within viewport bounds
  if (left + popupWidth > globalThis.innerWidth - padding) {
    left = globalThis.innerWidth - popupWidth - padding
  }
  if (left < padding) {
    left = padding
  }
  
  // Calculate top position - prefer below, but flip if no room
  let top = rect.bottom + 12
  if (top + popupMaxHeight > globalThis.innerHeight - padding) {
    // Try above
    top = rect.top - popupMaxHeight - 12
    if (top < padding) {
      // Just pin to bottom of viewport
      top = globalThis.innerHeight - popupMaxHeight - padding
    }
  }
  
  return {
    top: `${top}px`,
    left: `${left}px`
  }
})

// Source popover positioning
const sourcePopoverStyle = computed(() => {
  if (!selectedSource.value) return {}
  const rect = selectedSource.value.rect
  const popupWidth = 360
  const popupMaxHeight = 400
  const padding = 16
  
  // Position to the right of the row header by default
  let left = rect.right + 12
  // If not enough room on right, position to the left
  if (left + popupWidth > globalThis.innerWidth - padding) {
    left = rect.left - popupWidth - 12
  }
  if (left < padding) {
    left = padding
  }
  
  // Align top with the row header, but keep in viewport
  let top = rect.top
  if (top + popupMaxHeight > globalThis.innerHeight - padding) {
    top = globalThis.innerHeight - popupMaxHeight - padding
  }
  if (top < padding) {
    top = padding
  }
  
  return {
    top: `${top}px`,
    left: `${left}px`
  }
})

// Cell popover positioning - smart positioning to stay in viewport
const cellPopoverStyle = computed(() => {
  if (!selectedCell.value) return {}
  const rect = selectedCell.value.rect
  const popupWidth = 320
  const popupHeight = 200
  const padding = 16
  
  // Center horizontally relative to the cell
  let left = rect.left + (rect.width / 2) - (popupWidth / 2)
  
  // Keep within viewport bounds
  if (left + popupWidth > globalThis.innerWidth - padding) {
    left = globalThis.innerWidth - popupWidth - padding
  }
  if (left < padding) {
    left = padding
  }
  
  // Position below the cell by default
  let top = rect.bottom + 12
  let position = 'below'
  
  // If not enough room below, flip to above
  if (top + popupHeight > globalThis.innerHeight - padding) {
    top = rect.top - popupHeight - 12
    position = 'above'
    
    // If still not enough room, pin to bottom of viewport
    if (top < padding) {
      top = globalThis.innerHeight - popupHeight - padding
      position = 'below'
    }
  }
  
  return {
    top: `${top}px`,
    left: `${left}px`,
    '--arrow-position': position
  }
})

// Check if target IP matches an agent (for external destinations that are actually agents)
const targetAgentMatch = computed(() => {
  if (!selectedTarget.value || !matrixData.value) return null
  const target = selectedTarget.value.target
  
  // If already marked as agent, skip
  if (target.type === 'agent') return null
  
  // Check if the target name/id matches any agent's hostname pattern
  const targetName = target.name.toLowerCase()
  for (const agent of matrixData.value.source_agents) {
    // Check if agent name appears in target or vice versa
    const agentName = agent.name.toLowerCase()
    if (targetName.includes(agentName) || agentName.includes(targetName)) {
      return agent
    }
  }
  return null
})

// Sort targets: agents first, then destinations
const sortedTargets = computed(() => {
  if (!matrixData.value) return []
  return [...matrixData.value.target_labels].sort((a, b) => {
    if (a.type === 'agent' && b.type !== 'agent') return -1
    if (a.type !== 'agent' && b.type === 'agent') return 1
    return a.name.localeCompare(b.name)
  })
})

onMounted(() => {
  fetchMatrix()
  // Auto-refresh every 30 seconds
  refreshInterval = setInterval(fetchMatrix, 30000)
})

onUnmounted(() => {
  if (refreshInterval) clearInterval(refreshInterval)
})
</script>

<template>
  <div class="connectivity-matrix-container">
    <div class="matrix-header">
      <h3 class="matrix-title">
        <i class="bi bi-grid-3x3"></i>
        Connectivity Matrix
      </h3>
      <div class="controls">
        <button @click="fetchMatrix" class="control-btn" :disabled="loading">
          <i class="bi bi-arrow-clockwise" :class="{ 'spin': loading }"></i>
          Refresh
        </button>
        <select v-model="selectedLookback" class="control-select" @change="fetchMatrix">
          <option :value="15">Last 15 min</option>
          <option :value="60">Last Hour</option>
          <option :value="360">Last 6 Hours</option>
          <option :value="1440">Last 24 Hours</option>
        </select>
      </div>
    </div>

    <!-- Loading State -->
    <div v-if="loading && !matrixData" class="loading-state">
      <div class="spinner"></div>
      <p>Loading connectivity matrix...</p>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="error-state">
      <i class="bi bi-exclamation-triangle"></i>
      <p>{{ error }}</p>
      <button @click="fetchMatrix" class="btn btn-outline-primary">Retry</button>
    </div>

    <!-- Empty State -->
    <div v-else-if="!matrixData || matrixData.entries.length === 0" class="empty-state">
      <i class="bi bi-grid-3x3"></i>
      <h5>No Connectivity Data</h5>
      <p>Configure MTR, PING, or TrafficSim probes to see connectivity status.</p>
    </div>

    <!-- Matrix Grid -->
    <div v-else class="matrix-wrapper">
      <div class="matrix-grid" :style="{ '--num-cols': sortedTargets.length + 1 }">
        <!-- Header Row: Empty corner + Target headers -->
        <div class="matrix-corner">
          <span class="corner-label">Source → Target</span>
        </div>
        <div 
          v-for="target in sortedTargets" 
          :key="target.id" 
          class="matrix-header-cell"
          :class="{ 'agent-target': target.type === 'agent', 'selected': selectedTarget?.target.id === target.id }"
          :title="target.name"
          @click="handleTargetClick(target, $event)"
        >
          <div class="header-content">
            <i :class="target.type === 'agent' ? 'bi bi-server' : 'bi bi-geo-alt'"></i>
            <span class="header-label">{{ target.name }}</span>
          </div>
          <div class="header-type-badge">{{ target.type === 'agent' ? 'Agent' : 'Dest' }}</div>
        </div>

        <!-- Data Rows: Source agent + cells -->
        <template v-for="source in matrixData.source_agents" :key="source.id">
          <div 
            class="matrix-row-header" 
            :class="{ 'offline': !source.is_online, 'selected': selectedSource?.source.id === source.id }"
            @click="handleSourceClick(source, $event)"
          >
            <i class="bi bi-server"></i>
            <span>{{ source.name }}</span>
            <span v-if="!source.is_online" class="offline-badge">offline</span>
          </div>
          <div 
            v-for="target in sortedTargets" 
            :key="`${source.id}-${target.id}`"
            class="matrix-data-cell"
            :class="{ 
              'self-cell': `agent:${source.id}` === target.id,
              'agent-to-agent': isAgentToAgent(target.id) && `agent:${source.id}` !== target.id,
              'has-return-path': getReversePath(source.id, target.id) !== undefined
            }"
            @click="handleCellClick(getEntry(source.id, target.id), $event)"
          >
            <MatrixCell 
              v-if="`agent:${source.id}` !== target.id"
              :entry="getEntry(source.id, target.id)"
            />
            <span v-else class="self-indicator">—</span>
            <!-- Bidirectional indicator for agent-to-agent connections with return data -->
            <span 
              v-if="isAgentToAgent(target.id) && `agent:${source.id}` !== target.id && getReversePath(source.id, target.id)"
              class="bidirectional-indicator"
              title="Bidirectional: return path data available"
            >
              <i class="bi bi-arrow-left-right"></i>
            </span>
          </div>
        </template>
      </div>

      <!-- Popover for selected cell -->
      <div 
        v-if="selectedCell" 
        class="matrix-popover"
        :class="cellPopoverStyle['--arrow-position']"
        :style="cellPopoverStyle"
      >
        <div class="popover-arrow"></div>
        <div class="popover-header">
          <div class="route-info">
            <span class="source">{{ selectedCell.entry.source_agent_name }}</span>
            <i class="bi bi-arrow-right"></i>
            <span class="target">{{ selectedCell.entry.target_name }}</span>
          </div>
          <button class="close-btn" @click="closePopover">
            <i class="bi bi-x"></i>
          </button>
        </div>
        <div class="popover-body">
          <div 
            v-for="probe in selectedCell.entry.probe_status" 
            :key="probe.type"
            class="probe-stats"
          >
            <div class="probe-header">
              <span class="probe-type" :class="'type-' + probe.type.toLowerCase()">
                {{ probe.type }}
              </span>
              <span class="probe-status" :class="'status-' + probe.status">
                {{ probe.status }}
              </span>
            </div>
            <div class="metrics">
              <div class="metric">
                <i class="bi bi-clock"></i>
                <span>{{ probe.avg_latency?.toFixed(2) || '0' }}ms</span>
              </div>
              <div class="metric" :class="{ 'warning': probe.packet_loss > 5, 'critical': probe.packet_loss > 25 }">
                <i class="bi bi-box-seam"></i>
                <span>{{ probe.packet_loss?.toFixed(1) || '0' }}%</span>
              </div>
              <div v-if="probe.jitter && probe.jitter > 0" class="metric">
                <i class="bi bi-activity"></i>
                <span>{{ probe.jitter?.toFixed(2) }}ms</span>
              </div>
            </div>
          </div>
        </div>
        <div class="popover-footer">
          <router-link 
            :to="`/workspaces/${workspaceId}/agents/${selectedCell.entry.source_agent_id}`"
            class="btn btn-sm btn-outline-secondary"
          >
            View Agent
          </router-link>
        </div>
      </div>

      <!-- Click overlay to close popover -->
      <div v-if="selectedCell || selectedTarget || selectedSource" class="popover-overlay" @click="closePopover"></div>

      <!-- Target Detail Popup -->
      <div 
        v-if="selectedTarget" 
        class="target-popover"
        :style="targetPopoverStyle"
      >
        <div class="popover-header">
          <div class="target-info">
            <i :class="selectedTarget.target.type === 'agent' || targetAgentMatch ? 'bi bi-server' : 'bi bi-geo-alt'"></i>
            <div class="target-details">
              <span class="target-name">{{ selectedTarget.target.name }}</span>
              <span v-if="targetAgentMatch" class="target-type-label agent-match">
                <i class="bi bi-link-45deg"></i> Agent: {{ targetAgentMatch.name }}
              </span>
              <span v-else class="target-type-label">{{ selectedTarget.target.type === 'agent' ? 'Agent Target' : 'External Destination' }}</span>
            </div>
          </div>
          <button class="close-btn" @click="closePopover">
            <i class="bi bi-x"></i>
          </button>
        </div>
        <div class="popover-body">
          <div class="target-summary">
            <span class="summary-label">Tested by {{ targetEntries.length }} agent(s)</span>
          </div>
          <div class="target-entries">
            <div 
              v-for="entry in targetEntries" 
              :key="entry.source_agent_id"
              class="target-entry"
            >
              <div class="entry-source">
                <i class="bi bi-server"></i>
                <span>{{ entry.source_agent_name }}</span>
              </div>
              <div class="entry-probes">
                <span 
                  v-for="probe in entry.probe_status" 
                  :key="probe.type"
                  class="mini-bubble"
                  :class="'status-' + probe.status"
                  :title="`${probe.type}: ${probe.avg_latency?.toFixed(1)}ms, ${probe.packet_loss?.toFixed(1)}% loss`"
                >
                  {{ probe.type.charAt(0) }}
                </span>
              </div>
            </div>
            <div v-if="targetEntries.length === 0" class="no-entries">
              No active probes for this target
            </div>
          </div>
        </div>
        <div class="popover-footer" v-if="getAgentIdFromTarget(selectedTarget.target)">
          <router-link 
            :to="`/workspaces/${workspaceId}/agents/${getAgentIdFromTarget(selectedTarget.target)}`"
            class="btn btn-sm btn-primary"
          >
            <i class="bi bi-arrow-right"></i> View Agent
          </router-link>
        </div>
      </div>

      <!-- Source Detail Popup -->
      <div 
        v-if="selectedSource" 
        class="target-popover source-popover"
        :style="sourcePopoverStyle"
      >
        <div class="popover-header">
          <div class="target-info">
            <i class="bi bi-server"></i>
            <div class="target-details">
              <span class="target-name">{{ selectedSource.source.name }}</span>
              <span class="target-type-label">Source Agent</span>
            </div>
          </div>
          <button class="close-btn" @click="closePopover">
            <i class="bi bi-x"></i>
          </button>
        </div>
        <div class="popover-body">
          <div class="target-summary">
            <span class="summary-label">Testing {{ sourceEntries.length }} target(s)</span>
          </div>
          <div class="target-entries">
            <div 
              v-for="entry in sourceEntries" 
              :key="entry.target_id"
              class="target-entry"
              :class="{ 'bidirectional-entry': entry.target_type === 'agent' && getReversePath(selectedSource.source.id, entry.target_id) }"
            >
              <div class="entry-source">
                <i :class="entry.target_type === 'agent' ? 'bi bi-server' : 'bi bi-geo-alt'"></i>
                <span>{{ entry.target_name }}</span>
                <span v-if="entry.target_type === 'agent'" class="agent-badge">Agent</span>
              </div>
              <div class="entry-probes">
                <span 
                  v-for="probe in entry.probe_status" 
                  :key="probe.type"
                  class="mini-bubble"
                  :class="'status-' + probe.status"
                  :title="`${probe.type}: ${probe.avg_latency?.toFixed(1)}ms, ${probe.packet_loss?.toFixed(1)}% loss`"
                >
                  {{ probe.type.charAt(0) }}
                </span>
              </div>
              <!-- Show return path stats for agent-to-agent -->
              <div v-if="entry.target_type === 'agent' && getReversePath(selectedSource.source.id, entry.target_id)" class="return-path">
                <span class="return-label"><i class="bi bi-arrow-return-left"></i> Return:</span>
                <span 
                  v-for="probe in getReversePath(selectedSource.source.id, entry.target_id)?.probe_status" 
                  :key="probe.type"
                  class="mini-bubble"
                  :class="'status-' + probe.status"
                  :title="`Return ${probe.type}: ${probe.avg_latency?.toFixed(1)}ms, ${probe.packet_loss?.toFixed(1)}% loss`"
                >
                  {{ probe.type.charAt(0) }}
                </span>
              </div>
            </div>
            <div v-if="sourceEntries.length === 0" class="no-entries">
              No active probes from this agent
            </div>
          </div>
        </div>
        <div class="popover-footer">
          <router-link 
            :to="`/workspaces/${workspaceId}/agents/${selectedSource.source.id}`"
            class="btn btn-sm btn-primary"
          >
            <i class="bi bi-arrow-right"></i> View Agent
          </router-link>
        </div>
      </div>
    </div>

    <!-- Legend -->
    <div class="matrix-legend" v-if="matrixData && matrixData.entries.length > 0">
      <div class="legend-item">
        <span class="legend-bubble status-healthy"></span>
        <span>Healthy (&lt;5% loss, &lt;100ms)</span>
      </div>
      <div class="legend-item">
        <span class="legend-bubble status-degraded"></span>
        <span>Degraded (5-25% loss or 100-200ms)</span>
      </div>
      <div class="legend-item">
        <span class="legend-bubble status-critical"></span>
        <span>Critical (&gt;25% loss or &gt;200ms)</span>
      </div>
      <div class="legend-item">
        <span class="legend-bubble status-unknown"></span>
        <span>No Data</span>
      </div>
      <div class="legend-item">
        <i class="bi bi-arrow-left-right legend-icon"></i>
        <span>Bidirectional (return path)</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.connectivity-matrix-container {
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 12px;
  padding: 1.5rem;
  position: relative;
}

.matrix-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
  gap: 1rem;
}

.matrix-title {
  font-size: 1.25rem;
  font-weight: 600;
  margin: 0;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: var(--bs-body-color);
}

.controls {
  display: flex;
  gap: 0.5rem;
  align-items: center;
}

.control-btn {
  background: var(--bs-tertiary-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 6px;
  padding: 0.5rem 1rem;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
  color: var(--bs-body-color);
  transition: all 0.2s;
  min-height: 40px;
}

.control-btn:hover:not(:disabled) {
  background: var(--bs-secondary-bg);
  border-color: var(--bs-primary);
}

.control-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.control-select {
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 6px;
  padding: 0.5rem 0.75rem;
  font-size: 0.875rem;
  color: var(--bs-body-color);
  min-height: 40px;
}

.spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* States */
.loading-state, .error-state, .empty-state {
  text-align: center;
  padding: 3rem;
  color: var(--bs-secondary-color);
}

.loading-state .spinner {
  width: 40px;
  height: 40px;
  border: 3px solid var(--bs-border-color);
  border-top-color: var(--bs-primary);
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin: 0 auto 1rem;
}

.error-state i, .empty-state i {
  font-size: 3rem;
  margin-bottom: 1rem;
  display: block;
}

.error-state i { color: var(--bs-danger); }
.empty-state i { color: var(--bs-secondary-color); }

/* Matrix Grid */
.matrix-wrapper {
  overflow-x: auto;
  position: relative;
  border-radius: 8px;
  border: 1px solid var(--bs-border-color);
}

.matrix-grid {
  display: grid;
  /* Use explicit repeat value - --num-cols includes the row header column */
  grid-template-columns: 180px repeat(calc(var(--num-cols) - 1), minmax(60px, 100px));
  gap: 0;
  background: var(--bs-body-bg);
  overflow: hidden;
  min-width: max-content;
}

.matrix-corner {
  background: var(--bs-tertiary-bg);
  padding: 0.75rem;
  position: sticky;
  left: 0;
  z-index: 2;
  border-bottom: 2px solid var(--bs-border-color);
  border-right: 2px solid var(--bs-border-color);
  min-height: 100px;
  display: flex;
  align-items: flex-end;
  justify-content: flex-end;
}

.corner-label {
  font-size: 0.65rem;
  color: var(--bs-secondary-color);
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.matrix-header-cell {
  background: var(--bs-tertiary-bg);
  padding: 0.5rem 0.25rem;
  text-align: center;
  font-weight: 500;
  font-size: 0.7rem;
  min-height: 100px;
  min-width: 70px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: flex-end;
  gap: 0.25rem;
  border-bottom: 2px solid var(--bs-border-color);
  border-right: 1px solid var(--bs-border-color);
  cursor: default;
  transition: background-color 0.15s;
}

.matrix-header-cell:hover {
  background: var(--bs-secondary-bg);
}

.header-content {
  writing-mode: vertical-rl;
  text-orientation: mixed;
  transform: rotate(180deg);
  white-space: nowrap;
  display: flex;
  align-items: center;
  gap: 0.25rem;
  max-height: 80px;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 0.75rem;
  color: var(--bs-body-color);
}

.header-content i {
  font-size: 0.8rem;
  opacity: 0.7;
}

.header-label {
  overflow: hidden;
  text-overflow: ellipsis;
}

.header-type-badge {
  font-size: 0.55rem;
  text-transform: uppercase;
  padding: 0.125rem 0.375rem;
  border-radius: 3px;
  background: var(--bs-secondary-bg);
  color: var(--bs-secondary-color);
  font-weight: 600;
  letter-spacing: 0.3px;
}

.agent-target .header-content {
  color: var(--bs-primary);
}

.agent-target .header-type-badge {
  background: rgba(var(--bs-primary-rgb), 0.1);
  color: var(--bs-primary);
}

.matrix-row-header {
  background: var(--bs-tertiary-bg);
  padding: 0.5rem 0.75rem;
  font-weight: 500;
  font-size: 0.75rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  position: sticky;
  left: 0;
  z-index: 1;
  border-bottom: 1px solid var(--bs-border-color);
  border-right: 1px solid var(--bs-border-color);
  min-height: 50px;
}

.matrix-row-header.offline {
  opacity: 0.6;
}

.offline-badge {
  font-size: 0.6rem;
  background: var(--bs-danger);
  color: var(--bs-white);
  padding: 0.125rem 0.375rem;
  border-radius: 4px;
  margin-left: auto;
}

.matrix-data-cell {
  background: var(--bs-body-bg);
  padding: 0.25rem;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background-color 0.2s;
  min-height: 50px;
  border-bottom: 1px solid var(--bs-border-color);
  border-right: 1px solid var(--bs-border-color);
}

.matrix-data-cell:hover {
  background: var(--bs-secondary-bg);
}

.self-cell {
  background: var(--bs-tertiary-bg);
  cursor: default;
}

.self-cell:hover {
  background: var(--bs-tertiary-bg);
}

.self-indicator {
  color: var(--bs-secondary);
  font-size: 1.25rem;
}

/* Popover */
.popover-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 999;
}

.matrix-popover {
  position: fixed;
  z-index: 1000;
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(var(--bs-dark-rgb), 0.2);
  min-width: 280px;
  max-width: 340px;
  animation: popover-in 0.15s ease-out;
}

@keyframes popover-in {
  from {
    opacity: 0;
    transform: translateY(-8px) scale(0.96);
  }
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

.popover-arrow {
  position: absolute;
  width: 12px;
  height: 12px;
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  transform: rotate(45deg);
  z-index: -1;
}

.matrix-popover.below .popover-arrow {
  top: -6px;
  left: 50%;
  margin-left: -6px;
  border-right: none;
  border-bottom: none;
}

.matrix-popover.above .popover-arrow {
  bottom: -6px;
  left: 50%;
  margin-left: -6px;
  border-left: none;
  border-top: none;
}

.popover-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.875rem 1rem;
  border-bottom: 1px solid var(--bs-border-color);
  background: var(--bs-tertiary-bg);
  border-radius: 12px 12px 0 0;
  gap: 0.5rem;
}

.route-info {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  font-weight: 500;
  font-size: 0.85rem;
  flex-wrap: wrap;
  line-height: 1.4;
}

.route-info .source { 
  color: var(--bs-primary); 
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.route-info .target { 
  color: var(--bs-success); 
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.route-info i {
  color: var(--bs-secondary-color);
  font-size: 0.75rem;
  flex-shrink: 0;
}

.close-btn {
  background: none;
  border: none;
  cursor: pointer;
  padding: 0.25rem;
  font-size: 1.25rem;
  line-height: 1;
  color: var(--bs-secondary-color);
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  flex-shrink: 0;
  transition: all 0.15s;
}

.close-btn:hover {
  background: var(--bs-secondary-bg);
  color: var(--bs-body-color);
}

.popover-body {
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  max-height: 280px;
  overflow-y: auto;
}

.probe-stats {
  background: var(--bs-secondary-bg);
  padding: 0.875rem;
  border-radius: 8px;
  border: 1px solid var(--bs-border-color-translucent);
}

.probe-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}

.probe-type {
  font-weight: 600;
  font-size: 0.75rem;
  padding: 0.125rem 0.5rem;
  border-radius: 4px;
}

.probe-type.type-mtr { background: var(--bs-info-bg-subtle); color: var(--bs-info-text-emphasis); }
.probe-type.type-ping { background: var(--bs-success-bg-subtle); color: var(--bs-success-text-emphasis); }
.probe-type.type-trafficsim { background: var(--bs-warning-bg-subtle); color: var(--bs-warning-text-emphasis); }

.probe-status {
  font-size: 0.7rem;
  text-transform: uppercase;
  font-weight: 600;
  padding: 0.125rem 0.375rem;
  border-radius: 4px;
}

.probe-status.status-healthy { background: var(--bs-success-bg-subtle); color: var(--bs-success-text-emphasis); }
.probe-status.status-degraded { background: var(--bs-warning-bg-subtle); color: var(--bs-warning-text-emphasis); }
.probe-status.status-critical { background: var(--bs-danger-bg-subtle); color: var(--bs-danger-text-emphasis); }
.probe-status.status-unknown { background: var(--bs-secondary-bg); color: var(--bs-secondary-color); }

.metrics {
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
}

.metric {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  font-size: 0.8rem;
  color: var(--bs-body-color);
  background: var(--bs-body-bg);
  padding: 0.375rem 0.625rem;
  border-radius: 6px;
  border: 1px solid var(--bs-border-color-translucent);
}

.metric i {
  font-size: 0.85rem;
  color: var(--bs-secondary-color);
}

.metric.warning { 
  color: var(--bs-warning);
  border-color: rgba(var(--bs-warning-rgb), 0.3);
  background: rgba(var(--bs-warning-rgb), 0.1);
}
.metric.critical { 
  color: var(--bs-danger);
  border-color: rgba(var(--bs-danger-rgb), 0.3);
  background: rgba(var(--bs-danger-rgb), 0.1);
}

.popover-footer {
  padding: 0.875rem 1rem;
  border-top: 1px solid var(--bs-border-color);
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.popover-footer .btn {
  font-size: 0.8rem;
  padding: 0.5rem 1rem;
  border-radius: 6px;
}

/* Legend */
.matrix-legend {
  display: flex;
  gap: 1.5rem;
  margin-top: 1rem;
  padding-top: 1rem;
  border-top: 1px solid var(--bs-border-color);
  flex-wrap: wrap;
  font-size: 0.8rem;
  color: var(--bs-secondary-color);
}

.legend-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.legend-bubble {
  width: 12px;
  height: 12px;
  border-radius: 50%;
}

.legend-bubble.status-healthy { background: var(--bs-success); }
.legend-bubble.status-degraded { background: var(--bs-warning); }
.legend-bubble.status-critical { background: var(--bs-danger); }
.legend-bubble.status-unknown { background: var(--bs-secondary); }

/* Responsive */
@media (max-width: 768px) {
  .matrix-header {
    flex-direction: column;
    align-items: flex-start;
  }
  
  .matrix-legend {
    flex-direction: column;
    gap: 0.5rem;
  }
}


/* Selected header state */
.matrix-header-cell.selected {
  background: var(--bs-primary-bg-subtle);
  box-shadow: inset 0 0 0 2px var(--bs-primary);
}

.matrix-header-cell {
  cursor: pointer;
}

/* Target Popover */
.target-popover {
  position: fixed;
  z-index: 1001;
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(var(--bs-dark-rgb), 0.2);
  min-width: 320px;
  max-width: 400px;
  max-height: 80vh;
  overflow: hidden;
  animation: popover-in 0.15s ease-out;
}

.target-info {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.target-info > i {
  font-size: 1.5rem;
  color: var(--bs-primary);
}

.target-details {
  display: flex;
  flex-direction: column;
}

.target-name {
  font-weight: 600;
  font-size: 0.95rem;
}

.target-type-label {
  font-size: 0.7rem;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
}

.target-type-label.agent-match {
  color: var(--bs-primary);
  background: rgba(13, 110, 253, 0.1);
  padding: 0.125rem 0.375rem;
  border-radius: 4px;
  text-transform: none;
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
}

.target-summary {
  padding: 0.5rem 0;
  border-bottom: 1px solid var(--bs-border-color);
  margin-bottom: 0.5rem;
}

.summary-label {
  font-size: 0.8rem;
  color: var(--bs-secondary-color);
}

.target-entries {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  max-height: 300px;
  overflow-y: auto;
}

.target-entry {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.5rem 0.75rem;
  background: var(--bs-secondary-bg);
  border-radius: 6px;
}

.entry-source {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.8rem;
  font-weight: 500;
}

.entry-source i {
  font-size: 0.9rem;
  opacity: 0.7;
}

.entry-probes {
  display: flex;
  gap: 0.25rem;
}

.mini-bubble {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.6rem;
  font-weight: 700;
  color: white;
  cursor: default;
}

.mini-bubble.status-healthy { background: linear-gradient(135deg, var(--bs-success), color-mix(in srgb, var(--bs-success) 70%, black)); }
.mini-bubble.status-degraded { background: linear-gradient(135deg, var(--bs-warning), color-mix(in srgb, var(--bs-warning) 70%, black)); }
.mini-bubble.status-critical { background: linear-gradient(135deg, var(--bs-danger), color-mix(in srgb, var(--bs-danger) 70%, black)); }
.mini-bubble.status-unknown { background: linear-gradient(135deg, var(--bs-secondary), color-mix(in srgb, var(--bs-secondary) 70%, black)); }

.no-entries {
  text-align: center;
  padding: 1rem;
  color: var(--bs-secondary-color);
  font-size: 0.85rem;
}

/* Row header selection and clickability */
.matrix-row-header {
  cursor: pointer;
  transition: background-color 0.15s;
}

.matrix-row-header:hover {
  background: var(--bs-secondary-bg);
}

.matrix-row-header.selected {
  background: var(--bs-primary-bg-subtle);
  box-shadow: inset 0 0 0 2px var(--bs-primary);
}

/* Agent-to-agent cell highlighting */
.matrix-data-cell.agent-to-agent {
  background: color-mix(in srgb, var(--bs-primary) 5%, var(--bs-body-bg));
}

.matrix-data-cell.agent-to-agent:hover {
  background: color-mix(in srgb, var(--bs-primary) 12%, var(--bs-body-bg));
}

.matrix-data-cell.has-return-path {
  position: relative;
}

/* Bidirectional indicator */
.bidirectional-indicator {
  position: absolute;
  top: 2px;
  right: 2px;
  font-size: 0.55rem;
  color: var(--bs-primary);
  opacity: 0.7;
}

.bidirectional-indicator i {
  font-size: 0.6rem;
}

/* Return path display in source popup */
.return-path {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  margin-top: 0.375rem;
  padding-top: 0.375rem;
  border-top: 1px dashed var(--bs-border-color);
}

.return-label {
  font-size: 0.65rem;
  color: var(--bs-secondary-color);
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.return-label i {
  font-size: 0.7rem;
}

/* Bidirectional entry styling */
.bidirectional-entry {
  border-left: 3px solid var(--bs-primary);
}

/* Agent badge */
.agent-badge {
  font-size: 0.55rem;
  text-transform: uppercase;
  padding: 0.1rem 0.3rem;
  border-radius: 3px;
  background: rgba(13, 110, 253, 0.1);
  color: var(--bs-primary);
  font-weight: 600;
  margin-left: 0.25rem;
}

/* Legend icon */
.legend-icon {
  font-size: 0.85rem;
  color: var(--bs-primary);
}

/* Source popover adjustments */
.source-popover .target-entry {
  flex-wrap: wrap;
}

.source-popover .entry-source {
  flex: 1;
  min-width: 140px;
}
</style>

<!-- Separate unscoped style block for dark mode overrides -->
<style>
/* Dark Mode Overrides - Unscoped with !important for proper specificity */
[data-theme="dark"] .connectivity-matrix-container {
  background: var(--bs-body-bg) !important;
  border-color: var(--bs-border-color) !important;
}

[data-theme="dark"] .matrix-grid {
  background: var(--bs-body-bg) !important;
  border-color: var(--bs-border-color) !important;
}

[data-theme="dark"] .matrix-corner,
[data-theme="dark"] .matrix-header-cell,
[data-theme="dark"] .matrix-row-header {
  background: var(--bs-tertiary-bg) !important;
  border-color: var(--bs-border-color) !important;
}

[data-theme="dark"] .matrix-header-cell:hover,
[data-theme="dark"] .matrix-row-header:hover {
  background: var(--bs-secondary-bg) !important;
}

[data-theme="dark"] .matrix-data-cell {
  background: var(--bs-body-bg) !important;
  border-color: var(--bs-border-color) !important;
}

[data-theme="dark"] .matrix-data-cell:hover {
  background: var(--bs-secondary-bg) !important;
}

[data-theme="dark"] .self-cell,
[data-theme="dark"] .self-cell:hover {
  background: var(--bs-tertiary-bg) !important;
}

[data-theme="dark"] .matrix-data-cell.agent-to-agent {
  background: color-mix(in srgb, var(--bs-primary) 8%, var(--bs-body-bg)) !important;
}

[data-theme="dark"] .matrix-data-cell.agent-to-agent:hover {
  background: color-mix(in srgb, var(--bs-primary) 15%, var(--bs-body-bg)) !important;
}

[data-theme="dark"] .header-type-badge {
  background: var(--bs-secondary-bg) !important;
  color: var(--bs-secondary-color) !important;
}

[data-theme="dark"] .agent-target .header-type-badge {
  background: rgba(59, 130, 246, 0.2) !important;
  color: var(--bs-primary) !important;
}

[data-theme="dark"] .control-btn {
  background: var(--bs-secondary-bg) !important;
  border-color: var(--bs-border-color) !important;
  color: var(--bs-body-color) !important;
}

[data-theme="dark"] .control-btn:hover:not(:disabled) {
  background: var(--bs-tertiary-bg) !important;
}

[data-theme="dark"] .control-select {
  background: var(--bs-body-bg) !important;
  border-color: var(--bs-border-color) !important;
  color: var(--bs-body-color) !important;
}

[data-theme="dark"] .matrix-popover,
[data-theme="dark"] .target-popover {
  background: var(--bs-body-bg) !important;
  border-color: var(--bs-border-color) !important;
}

[data-theme="dark"] .popover-header {
  background: var(--bs-tertiary-bg) !important;
  border-color: var(--bs-border-color) !important;
}

[data-theme="dark"] .probe-stats,
[data-theme="dark"] .target-entry {
  background: var(--bs-secondary-bg) !important;
}

[data-theme="dark"] .probe-type.type-mtr {
  background: rgba(99, 102, 241, 0.2) !important;
  color: var(--bs-info) !important;
}

[data-theme="dark"] .probe-type.type-ping {
  background: rgba(16, 185, 129, 0.2) !important;
  color: var(--bs-success) !important;
}

[data-theme="dark"] .probe-type.type-trafficsim {
  background: rgba(245, 158, 11, 0.2) !important;
  color: var(--bs-warning) !important;
}

[data-theme="dark"] .probe-status.status-healthy {
  background: rgba(16, 185, 129, 0.2) !important;
  color: var(--bs-success) !important;
}

[data-theme="dark"] .probe-status.status-degraded {
  background: rgba(245, 158, 11, 0.2) !important;
  color: var(--bs-warning) !important;
}

[data-theme="dark"] .probe-status.status-critical {
  background: rgba(239, 68, 68, 0.2) !important;
  color: var(--bs-danger) !important;
}

[data-theme="dark"] .probe-status.status-unknown {
  background: rgba(107, 114, 128, 0.3) !important;
  color: var(--bs-secondary) !important;
}

[data-theme="dark"] .matrix-header-cell.selected,
[data-theme="dark"] .matrix-row-header.selected {
  background: color-mix(in srgb, var(--bs-primary) 20%, var(--bs-body-bg)) !important;
}
</style>
