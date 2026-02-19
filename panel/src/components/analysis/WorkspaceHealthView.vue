<script lang="ts" setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { ProbeDataService } from '@/services/apiService'
import type { WorkspaceAnalysis, DetectedIncident } from './types'
import { gradeColors, statusColors, severityIcons } from './types'

const props = defineProps<{
  workspaceId: number | string
}>()

const analysis = ref<WorkspaceAnalysis | null>(null)
const loading = ref(true)
const error = ref('')
const expandedIncidents = ref<Set<string>>(new Set())
let refreshInterval: ReturnType<typeof setInterval> | null = null

async function fetchAnalysis() {
  try {
    analysis.value = await ProbeDataService.workspaceAnalysis(props.workspaceId, { lookback: 60 })
    error.value = ''
  } catch (e: any) {
    error.value = e?.message || 'Failed to fetch analysis'
  } finally {
    loading.value = false
  }
}

function toggleIncident(id: string) {
  if (expandedIncidents.value.has(id)) {
    expandedIncidents.value.delete(id)
  } else {
    expandedIncidents.value.add(id)
  }
}

function statusColor(status: string) {
  return statusColors[status] || statusColors.unknown
}

function gradeColor(grade: string) {
  return gradeColors[grade] || gradeColors.unknown
}

const onlineCount = computed(() => analysis.value?.agents?.filter(a => a.is_online).length || 0)
const offlineCount = computed(() => analysis.value?.agents?.filter(a => !a.is_online).length || 0)
const criticalIncidents = computed(() => analysis.value?.incidents?.filter(i => i.severity === 'critical') || [])
const warningIncidents = computed(() => analysis.value?.incidents?.filter(i => i.severity === 'warning') || [])
const infoIncidents = computed(() => analysis.value?.incidents?.filter(i => i.severity === 'info') || [])

onMounted(() => {
  fetchAnalysis()
  refreshInterval = setInterval(fetchAnalysis, 60000)
})

onUnmounted(() => {
  if (refreshInterval) clearInterval(refreshInterval)
})
</script>

<template>
  <div class="ai-status">
    <!-- Loading -->
    <div v-if="loading" class="text-center py-5">
      <div class="spinner-border text-primary" role="status">
        <span class="visually-hidden">Loading...</span>
      </div>
      <p class="text-muted mt-2">Analyzing infrastructure status...</p>
    </div>

    <!-- Error -->
    <div v-else-if="error" class="text-center py-5">
      <i class="bi bi-exclamation-triangle fs-1 text-warning mb-3"></i>
      <h5 class="text-muted">Analysis Unavailable</h5>
      <p class="text-muted small">{{ error }}</p>
      <button class="btn btn-sm btn-outline-primary" @click="fetchAnalysis">Retry</button>
    </div>

    <!-- AI Status Content -->
    <div v-else-if="analysis">
      <!-- Status Banner -->
      <div class="status-banner" :style="{ background: statusColor(analysis.status?.status || 'unknown').bg, borderColor: statusColor(analysis.status?.status || 'unknown').text }">
        <div class="d-flex align-items-center gap-3">
          <i :class="['bi', statusColor(analysis.status?.status || 'unknown').icon, 'status-icon']"
             :style="{ color: statusColor(analysis.status?.status || 'unknown').text }"></i>
          <div class="flex-grow-1">
            <div class="status-title" :style="{ color: statusColor(analysis.status?.status || 'unknown').text }">
              {{ (analysis.status?.status || 'unknown').charAt(0).toUpperCase() + (analysis.status?.status || 'unknown').slice(1) }}
            </div>
            <div class="status-message">{{ analysis.status?.message || 'Status unavailable' }}</div>
          </div>
          <div class="status-stats text-end">
            <div class="stat-line"><span class="stat-num">{{ analysis.total_agents }}</span> agents</div>
            <div class="stat-line"><span class="stat-num">{{ analysis.total_probes }}</span> probes</div>
            <div class="stat-line" v-if="analysis.status?.active_issues">
              <span class="stat-num text-warning">{{ analysis.status.active_issues }}</span> issue{{ analysis.status.active_issues !== 1 ? 's' : '' }}
            </div>
          </div>
        </div>
      </div>

      <!-- Detected Incidents (the core of the AI analysis) -->
      <div v-if="analysis.incidents?.length" class="incidents-section mt-3">
        <h6 class="section-title">
          <i class="bi bi-lightning-charge me-1"></i>
          Detected Issues
          <span class="badge-count">{{ analysis.incidents.length }}</span>
        </h6>

        <!-- Critical incidents first -->
        <div v-for="incident in [...criticalIncidents, ...warningIncidents, ...infoIncidents]" :key="incident.id"
          class="incident-card" :class="incident.severity"
          @click="toggleIncident(incident.id)"
        >
          <div class="incident-header">
            <i :class="['bi', severityIcons[incident.severity] || 'bi-info-circle', 'incident-severity-icon']"></i>
            <div class="incident-info flex-grow-1">
              <div class="incident-title">{{ incident.title }}</div>
              <div class="incident-scope">
                <span class="scope-badge">{{ incident.scope }}</span>
                <span v-if="incident.affected_agents?.length" class="affected-list">
                  {{ incident.affected_agents.join(', ') }}
                </span>
              </div>
            </div>
            <i :class="['bi', expandedIncidents.has(incident.id) ? 'bi-chevron-up' : 'bi-chevron-down', 'expand-icon']"></i>
          </div>

          <!-- AI Suggested Cause -->
          <div class="suggested-cause">
            <i class="bi bi-cpu me-1"></i>
            <span>{{ incident.suggested_cause }}</span>
          </div>

          <!-- Expanded Details -->
          <div v-if="expandedIncidents.has(incident.id)" class="incident-details" @click.stop>
            <div v-if="incident.evidence?.length" class="detail-section">
              <div class="detail-label">Evidence</div>
              <div v-for="(e, i) in incident.evidence" :key="i" class="evidence-item">
                <i class="bi bi-dot"></i>{{ e }}
              </div>
            </div>

            <div v-if="incident.affected_targets?.length" class="detail-section">
              <div class="detail-label">Affected Targets</div>
              <div class="target-chips">
                <span v-for="t in incident.affected_targets" :key="t" class="target-chip">{{ t }}</span>
              </div>
            </div>

            <div v-if="incident.recommendations?.length" class="detail-section">
              <div class="detail-label">Recommended Actions</div>
              <ol class="recommendations-list">
                <li v-for="(r, i) in incident.recommendations" :key="i">{{ r }}</li>
              </ol>
            </div>
          </div>
        </div>
      </div>

      <!-- No Issues State -->
      <div v-else class="no-issues mt-3">
        <i class="bi bi-shield-check no-issues-icon"></i>
        <p class="mb-0">No issues detected — all paths performing within acceptable parameters</p>
      </div>

      <!-- Agent Health Overview (compact) -->
      <div class="agents-overview mt-3">
        <h6 class="section-title">
          <i class="bi bi-hdd-network me-1"></i>
          Agent Summary
        </h6>
        <div class="agent-chips">
          <div v-for="agent in analysis.agents" :key="agent.agent_id" class="agent-chip"
            :style="{ borderColor: gradeColor(agent.health.grade).border }"
          >
            <span class="agent-dot" :class="agent.is_online ? 'online' : 'offline'"></span>
            <span class="agent-chip-name">{{ agent.agent_name }}</span>
            <span class="agent-chip-score" :style="{ color: gradeColor(agent.health.grade).text }">
              {{ Math.round(agent.health.overall_health) }}
            </span>
          </div>
        </div>
      </div>

      <div class="text-muted small mt-3 text-end">
        <i class="bi bi-clock me-1"></i>
        {{ new Date(analysis.generated_at).toLocaleTimeString() }} · Auto-refreshes every 60s
      </div>
    </div>
  </div>
</template>

<style scoped>
.ai-status { padding: 0; }

/* Status Banner */
.status-banner {
  border: 1px solid;
  border-radius: 12px;
  padding: 16px 20px;
}
.status-icon { font-size: 28px; }
.status-title {
  font-size: 18px;
  font-weight: 700;
  text-transform: capitalize;
}
.status-message {
  font-size: 13px;
  color: var(--text-muted, #aaa);
  margin-top: 2px;
}
.status-stats { font-size: 12px; color: var(--text-muted, #888); }
.stat-num { font-weight: 700; color: var(--text-color, #fff); }

/* Section Title */
.section-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-muted, #888);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 10px;
  display: flex;
  align-items: center;
  gap: 6px;
}
.badge-count {
  font-size: 10px;
  font-weight: 700;
  background: var(--primary, #3b82f6);
  color: #fff;
  padding: 1px 6px;
  border-radius: 8px;
}

/* Incident Cards */
.incident-card {
  background: var(--card-bg, #1e1e2e);
  border: 1px solid var(--border-color, #333);
  border-radius: 10px;
  padding: 12px 16px;
  margin-bottom: 8px;
  cursor: pointer;
  transition: transform 0.15s, box-shadow 0.15s;
}
.incident-card:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(0,0,0,0.15);
}
.incident-card.critical { border-left: 4px solid #ef4444; }
.incident-card.warning  { border-left: 4px solid #f59e0b; }
.incident-card.info     { border-left: 4px solid #3b82f6; }

.incident-header {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}
.incident-severity-icon { font-size: 18px; margin-top: 2px; }
.incident-card.critical .incident-severity-icon { color: #ef4444; }
.incident-card.warning .incident-severity-icon  { color: #f59e0b; }
.incident-card.info .incident-severity-icon     { color: #3b82f6; }

.incident-title {
  font-weight: 600;
  font-size: 14px;
  color: var(--text-color, #fff);
}
.incident-scope {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 2px;
}
.scope-badge {
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  background: var(--border-color, #333);
  color: var(--text-muted, #888);
  padding: 1px 6px;
  border-radius: 3px;
}
.affected-list {
  font-size: 12px;
  color: var(--text-muted, #888);
}
.expand-icon {
  color: var(--text-muted, #666);
  margin-top: 4px;
}

/* AI Suggested Cause */
.suggested-cause {
  margin-top: 8px;
  padding: 8px 12px;
  background: rgba(59, 130, 246, 0.08);
  border: 1px solid rgba(59, 130, 246, 0.2);
  border-radius: 6px;
  font-size: 12px;
  color: var(--text-color, #ddd);
  line-height: 1.5;
}
.suggested-cause i {
  color: #3b82f6;
  font-size: 12px;
}

/* Expanded Details */
.incident-details {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid var(--border-color, #333);
}
.detail-section { margin-bottom: 10px; }
.detail-label {
  font-size: 11px;
  font-weight: 600;
  color: var(--text-muted, #888);
  text-transform: uppercase;
  margin-bottom: 4px;
}
.evidence-item {
  font-size: 12px;
  color: var(--text-muted, #aaa);
  padding: 1px 0;
}
.target-chips { display: flex; gap: 6px; flex-wrap: wrap; }
.target-chip {
  font-size: 11px;
  font-family: 'SF Mono', 'Fira Code', monospace;
  background: var(--border-color, #333);
  color: var(--text-color, #fff);
  padding: 2px 8px;
  border-radius: 4px;
}
.recommendations-list {
  font-size: 12px;
  color: var(--text-muted, #aaa);
  padding-left: 18px;
  margin-bottom: 0;
}
.recommendations-list li { padding: 2px 0; }

/* No Issues */
.no-issues {
  text-align: center;
  padding: 24px;
  background: rgba(16, 185, 129, 0.08);
  border: 1px solid rgba(16, 185, 129, 0.2);
  border-radius: 10px;
  color: #10b981;
}
.no-issues-icon {
  font-size: 32px;
  display: block;
  margin-bottom: 8px;
}
.no-issues p { font-size: 13px; }

/* Agent Chips */
.agent-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.agent-chip {
  display: flex;
  align-items: center;
  gap: 6px;
  background: var(--card-bg, #1e1e2e);
  border: 1px solid;
  border-radius: 20px;
  padding: 4px 12px;
  font-size: 12px;
}
.agent-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}
.agent-dot.online  { background: #10b981; }
.agent-dot.offline { background: #6b7280; }
.agent-chip-name { color: var(--text-color, #fff); }
.agent-chip-score { font-weight: 700; }
</style>
