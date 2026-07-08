<script lang="ts" setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { ProbeDataService } from '@/services/apiService'
import type { WorkspaceHealthMesh, AgentMeshLink } from './types'
import { gradeColors } from './types'
import HealthChordDiagram from './HealthChordDiagram.vue'

const props = defineProps<{
  workspaceId: number | string
}>()

const mesh = ref<WorkspaceHealthMesh | null>(null)
const loading = ref(true)
const error = ref('')
const sortKey = ref<'health' | 'latency' | 'loss' | 'jitter' | 'samples'>('health')
let refreshInterval: ReturnType<typeof setInterval> | null = null

async function fetchMesh() {
  try {
    mesh.value = await ProbeDataService.healthMesh(props.workspaceId, { lookback: 60 })
    error.value = ''
  } catch (e: any) {
    error.value = e?.message || 'Failed to fetch health mesh'
  } finally {
    loading.value = false
  }
}

function gradeColor(grade: string) {
  return gradeColors[grade] || gradeColors.unknown
}

const sortedLinks = computed<AgentMeshLink[]>(() => {
  const links = [...(mesh.value?.links ?? [])]
  switch (sortKey.value) {
    case 'latency':
      return links.sort((a, b) => b.metrics.avg_latency - a.metrics.avg_latency)
    case 'loss':
      return links.sort((a, b) => b.metrics.packet_loss - a.metrics.packet_loss)
    case 'jitter':
      return links.sort((a, b) => b.metrics.jitter_avg - a.metrics.jitter_avg)
    case 'samples':
      return links.sort((a, b) => b.metrics.sample_count - a.metrics.sample_count)
    default:
      // worst health first (API default order)
      return links.sort((a, b) => a.health.overall_health - b.health.overall_health)
  }
})

const gradedNodes = computed(() => (mesh.value?.nodes ?? []).filter((n) => n.link_count > 0))

onMounted(() => {
  fetchMesh()
  refreshInterval = setInterval(fetchMesh, 60000)
})

onUnmounted(() => {
  if (refreshInterval) clearInterval(refreshInterval)
})
</script>

<template>
  <div class="health-mesh-view">
    <!-- Loading -->
    <div v-if="loading" class="text-center py-5">
      <div class="spinner-border text-primary" role="status">
        <span class="visually-hidden">Loading...</span>
      </div>
      <p class="text-muted mt-2">Building agent health mesh...</p>
    </div>

    <!-- Error -->
    <div v-else-if="error" class="text-center py-5">
      <i class="bi bi-exclamation-triangle fs-1 text-warning mb-3"></i>
      <h5 class="text-muted">Health Mesh Unavailable</h5>
      <p class="text-muted small">{{ error }}</p>
      <button class="btn btn-sm btn-outline-primary" @click="fetchMesh">Retry</button>
    </div>

    <div v-else-if="mesh">
      <!-- Overall banner -->
      <div class="mesh-banner" :style="{ borderColor: gradeColor(mesh.overall_health.grade).border }">
        <div class="banner-score" :style="{ color: gradeColor(mesh.overall_health.grade).text }">
          {{ Math.round(mesh.overall_health.overall_health) }}<span class="score-denominator">/100</span>
        </div>
        <div class="banner-text">
          <div class="banner-title">Workspace mesh health — {{ mesh.overall_health.grade }}</div>
          <div class="banner-sub">
            {{ gradedNodes.length }} agents with inter-agent paths · {{ mesh.links.length }} directed paths ·
            aggregated across PING, MTR and TrafficSim probes in both directions
          </div>
        </div>
      </div>

      <div class="mesh-layout">
        <!-- Chord diagram -->
        <div class="mesh-chart">
          <HealthChordDiagram :mesh="mesh" />
        </div>

        <!-- Links table -->
        <div class="mesh-table" v-if="mesh.links.length > 0">
          <div class="table-header">
            <span class="table-title">Agent-to-Agent Paths</span>
            <div class="sort-controls">
              <span class="sort-label">sort:</span>
              <button
                v-for="key in (['health', 'latency', 'loss', 'jitter', 'samples'] as const)"
                :key="key"
                class="sort-btn"
                :class="{ active: sortKey === key }"
                @click="sortKey = key"
              >{{ key }}</button>
            </div>
          </div>
          <table class="table table-sm links-table">
            <thead>
              <tr>
                <th>Path</th>
                <th class="num">Health</th>
                <th class="num">Latency</th>
                <th class="num">Loss</th>
                <th class="num">Jitter</th>
                <th>Probes</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="l in sortedLinks" :key="`${l.source_agent_id}-${l.target_agent_id}`">
                <td class="path-cell">{{ l.source_agent_name }} <i class="bi bi-arrow-right path-arrow"></i> {{ l.target_agent_name }}</td>
                <td class="num">
                  <span class="grade-pill" :style="{ background: gradeColor(l.health.grade).bg, color: gradeColor(l.health.grade).text }">
                    {{ Math.round(l.health.overall_health) }} · {{ l.health.grade }}
                  </span>
                </td>
                <td class="num">{{ l.metrics.avg_latency.toFixed(1) }}ms</td>
                <td class="num">{{ l.metrics.packet_loss.toFixed(2) }}%</td>
                <td class="num">{{ l.metrics.jitter_avg > 0 ? l.metrics.jitter_avg.toFixed(1) + 'ms' : '—' }}</td>
                <td class="types-cell">{{ l.probe_types.join(' + ') }} <span class="samples">({{ l.metrics.sample_count }})</span></td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.mesh-banner {
  display: flex;
  align-items: center;
  gap: 1rem;
  border: 1px solid;
  border-radius: 8px;
  padding: 0.75rem 1rem;
  margin-bottom: 1rem;
}
.banner-score {
  font-size: 2rem;
  font-weight: 700;
  line-height: 1;
}
.score-denominator {
  font-size: 0.9rem;
  font-weight: 400;
  opacity: 0.6;
}
.banner-title {
  font-weight: 600;
  text-transform: capitalize;
}
.banner-sub {
  font-size: 0.8rem;
  color: var(--bs-secondary-color);
}
.mesh-layout {
  display: grid;
  grid-template-columns: minmax(320px, 560px) 1fr;
  gap: 1.5rem;
  align-items: start;
}
@media (max-width: 992px) {
  .mesh-layout {
    grid-template-columns: 1fr;
  }
}
.table-header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
}
.table-title {
  font-weight: 600;
}
.sort-controls {
  display: flex;
  gap: 0.25rem;
  align-items: baseline;
  font-size: 0.75rem;
}
.sort-label {
  color: var(--bs-secondary-color);
  margin-right: 0.25rem;
}
.sort-btn {
  border: 1px solid var(--bs-border-color);
  background: transparent;
  color: var(--bs-secondary-color);
  border-radius: 4px;
  padding: 0.1rem 0.5rem;
  font-size: 0.75rem;
}
.sort-btn.active {
  background: var(--bs-primary);
  border-color: var(--bs-primary);
  color: #fff;
}
.links-table {
  font-size: 0.85rem;
}
.links-table .num {
  text-align: right;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.path-cell {
  white-space: nowrap;
}
.path-arrow {
  color: var(--bs-secondary-color);
  font-size: 0.75rem;
}
.grade-pill {
  display: inline-block;
  border-radius: 10px;
  padding: 0.05rem 0.5rem;
  font-size: 0.75rem;
  white-space: nowrap;
  text-transform: capitalize;
}
.types-cell {
  font-size: 0.75rem;
  color: var(--bs-secondary-color);
  white-space: nowrap;
}
.samples {
  opacity: 0.7;
}
</style>
