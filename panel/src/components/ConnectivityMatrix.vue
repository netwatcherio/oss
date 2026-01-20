<script lang="ts" setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import type { ConnectivityMatrix, ConnectivityMatrixEntry, ProbeStatusSummary, TargetLabel, AgentSummary } from '@/types'
import MatrixCell from './MatrixCell.vue'

const props = defineProps<{
  workspaceId: number
}>()

const loading = ref(true)
const error = ref<string | null>(null)
const matrixData = ref<ConnectivityMatrix | null>(null)
const selectedLookback = ref(15)
const selectedCell = ref<{ entry: ConnectivityMatrixEntry; rect: DOMRect } | null>(null)
let refreshInterval: ReturnType<typeof setInterval> | null = null

// Fetch matrix data from backend
async function fetchMatrix() {
  try {
    loading.value = true
    error.value = null
    const response = await fetch(`/api/workspaces/${props.workspaceId}/connectivity-matrix?lookback=${selectedLookback.value}`)
    if (!response.ok) throw new Error('Failed to fetch connectivity matrix')
    matrixData.value = await response.json()
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
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  selectedCell.value = { entry, rect }
}

function closePopover() {
  selectedCell.value = null
}

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
        <div class="matrix-corner"></div>
        <div 
          v-for="target in sortedTargets" 
          :key="target.id" 
          class="matrix-header-cell"
          :class="{ 'agent-target': target.type === 'agent' }"
        >
          <div class="header-content">
            <i :class="target.type === 'agent' ? 'bi bi-server' : 'bi bi-geo-alt'"></i>
            <span class="header-label">{{ target.name }}</span>
          </div>
        </div>

        <!-- Data Rows: Source agent + cells -->
        <template v-for="source in matrixData.source_agents" :key="source.id">
          <div class="matrix-row-header" :class="{ 'offline': !source.is_online }">
            <i class="bi bi-server"></i>
            <span>{{ source.name }}</span>
            <span v-if="!source.is_online" class="offline-badge">offline</span>
          </div>
          <div 
            v-for="target in sortedTargets" 
            :key="`${source.id}-${target.id}`"
            class="matrix-data-cell"
            :class="{ 'self-cell': `agent:${source.id}` === target.id }"
            @click="handleCellClick(getEntry(source.id, target.id), $event)"
          >
            <MatrixCell 
              v-if="`agent:${source.id}` !== target.id"
              :entry="getEntry(source.id, target.id)"
            />
            <span v-else class="self-indicator">—</span>
          </div>
        </template>
      </div>

      <!-- Popover for selected cell -->
      <div 
        v-if="selectedCell" 
        class="matrix-popover"
        :style="{
          top: `${selectedCell.rect.bottom + 8}px`,
          left: `${selectedCell.rect.left}px`
        }"
      >
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
      <div v-if="selectedCell" class="popover-overlay" @click="closePopover"></div>
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
    </div>
  </div>
</template>

<style scoped>
.connectivity-matrix-container {
  background: var(--bs-body-bg, #fff);
  border: 1px solid var(--bs-border-color, #dee2e6);
  border-radius: 8px;
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
}

.controls {
  display: flex;
  gap: 0.5rem;
  align-items: center;
}

.control-btn {
  background: var(--bs-secondary-bg, #e9ecef);
  border: 1px solid var(--bs-border-color, #dee2e6);
  border-radius: 6px;
  padding: 0.5rem 1rem;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
  transition: all 0.2s;
}

.control-btn:hover:not(:disabled) {
  background: var(--bs-tertiary-bg, #f8f9fa);
}

.control-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.control-select {
  background: var(--bs-body-bg, #fff);
  border: 1px solid var(--bs-border-color, #dee2e6);
  border-radius: 6px;
  padding: 0.5rem 0.75rem;
  font-size: 0.875rem;
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
  color: var(--bs-secondary-color, #6c757d);
}

.loading-state .spinner {
  width: 40px;
  height: 40px;
  border: 3px solid var(--bs-border-color, #dee2e6);
  border-top-color: var(--bs-primary, #0d6efd);
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin: 0 auto 1rem;
}

.error-state i, .empty-state i {
  font-size: 3rem;
  margin-bottom: 1rem;
  display: block;
}

.error-state i { color: var(--bs-danger, #dc3545); }
.empty-state i { color: var(--bs-secondary, #6c757d); }

/* Matrix Grid */
.matrix-wrapper {
  overflow-x: auto;
  position: relative;
}

.matrix-grid {
  display: grid;
  grid-template-columns: 180px repeat(var(--num-cols, 1) - 1, minmax(80px, 1fr));
  gap: 1px;
  background: var(--bs-border-color, #dee2e6);
  border-radius: 8px;
  overflow: hidden;
  min-width: fit-content;
}

.matrix-corner {
  background: var(--bs-tertiary-bg, #f8f9fa);
  padding: 0.75rem;
  position: sticky;
  left: 0;
  z-index: 2;
}

.matrix-header-cell {
  background: var(--bs-tertiary-bg, #f8f9fa);
  padding: 0.5rem;
  text-align: center;
  font-weight: 500;
  font-size: 0.75rem;
  min-height: 80px;
  display: flex;
  align-items: flex-end;
  justify-content: center;
}

.header-content {
  transform: rotate(-45deg);
  transform-origin: center;
  white-space: nowrap;
  display: flex;
  align-items: center;
  gap: 0.25rem;
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.header-label {
  overflow: hidden;
  text-overflow: ellipsis;
}

.agent-target .header-content {
  color: var(--bs-primary, #0d6efd);
}

.matrix-row-header {
  background: var(--bs-tertiary-bg, #f8f9fa);
  padding: 0.75rem;
  font-weight: 500;
  font-size: 0.8rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  position: sticky;
  left: 0;
  z-index: 1;
}

.matrix-row-header.offline {
  opacity: 0.6;
}

.offline-badge {
  font-size: 0.65rem;
  background: var(--bs-danger, #dc3545);
  color: white;
  padding: 0.125rem 0.375rem;
  border-radius: 4px;
  margin-left: auto;
}

.matrix-data-cell {
  background: var(--bs-body-bg, #fff);
  padding: 0.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background-color 0.2s;
  min-height: 50px;
}

.matrix-data-cell:hover {
  background: var(--bs-secondary-bg, #e9ecef);
}

.self-cell {
  background: var(--bs-tertiary-bg, #f8f9fa);
  cursor: default;
}

.self-cell:hover {
  background: var(--bs-tertiary-bg, #f8f9fa);
}

.self-indicator {
  color: var(--bs-secondary, #6c757d);
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
  background: var(--bs-body-bg, #fff);
  border: 1px solid var(--bs-border-color, #dee2e6);
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0,0,0,0.15);
  min-width: 280px;
  max-width: 350px;
}

.popover-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--bs-border-color, #dee2e6);
  background: var(--bs-tertiary-bg, #f8f9fa);
  border-radius: 8px 8px 0 0;
}

.route-info {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 500;
}

.route-info .source { color: var(--bs-primary, #0d6efd); }
.route-info .target { color: var(--bs-success, #198754); }

.close-btn {
  background: none;
  border: none;
  cursor: pointer;
  padding: 0.25rem;
  font-size: 1.25rem;
  line-height: 1;
  color: var(--bs-secondary, #6c757d);
}

.popover-body {
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.probe-stats {
  background: var(--bs-secondary-bg, #e9ecef);
  padding: 0.75rem;
  border-radius: 6px;
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

.probe-type.type-mtr { background: #e0e7ff; color: #3730a3; }
.probe-type.type-ping { background: #d1fae5; color: #065f46; }
.probe-type.type-trafficsim { background: #fef3c7; color: #92400e; }

.probe-status {
  font-size: 0.7rem;
  text-transform: uppercase;
  font-weight: 600;
  padding: 0.125rem 0.375rem;
  border-radius: 4px;
}

.probe-status.status-healthy { background: #d1fae5; color: #065f46; }
.probe-status.status-degraded { background: #fef3c7; color: #92400e; }
.probe-status.status-critical { background: #fee2e2; color: #991b1b; }
.probe-status.status-unknown { background: #e5e7eb; color: #4b5563; }

.metrics {
  display: flex;
  gap: 1rem;
  flex-wrap: wrap;
}

.metric {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  font-size: 0.8rem;
  color: var(--bs-body-color, #212529);
}

.metric.warning { color: var(--bs-warning, #ffc107); }
.metric.critical { color: var(--bs-danger, #dc3545); }

.popover-footer {
  padding: 0.75rem 1rem;
  border-top: 1px solid var(--bs-border-color, #dee2e6);
  display: flex;
  justify-content: flex-end;
}

/* Legend */
.matrix-legend {
  display: flex;
  gap: 1.5rem;
  margin-top: 1rem;
  padding-top: 1rem;
  border-top: 1px solid var(--bs-border-color, #dee2e6);
  flex-wrap: wrap;
  font-size: 0.8rem;
  color: var(--bs-secondary-color, #6c757d);
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

.legend-bubble.status-healthy { background: #10b981; }
.legend-bubble.status-degraded { background: #f59e0b; }
.legend-bubble.status-critical { background: #ef4444; }
.legend-bubble.status-unknown { background: #9ca3af; }

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
</style>
