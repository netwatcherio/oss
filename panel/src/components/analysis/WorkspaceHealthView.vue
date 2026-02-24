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
      <div class="status-banner" :class="analysis.status?.status || 'unknown'">
        <div class="status-banner-content">
          <div class="status-main">
            <i :class="['bi', statusColor(analysis.status?.status || 'unknown').icon, 'status-icon']"></i>
            <div class="status-text">
              <div class="status-title">
                {{ (analysis.status?.status || 'unknown').charAt(0).toUpperCase() + (analysis.status?.status || 'unknown').slice(1) }}
              </div>
              <div class="status-message">{{ analysis.status?.message || 'Status unavailable' }}</div>
            </div>
          </div>
          <div class="status-stats">
            <div class="stat-item">
              <span class="stat-num">{{ analysis.total_agents }}</span>
              <span class="stat-label">agents</span>
            </div>
            <div class="stat-item">
              <span class="stat-num">{{ analysis.total_probes }}</span>
              <span class="stat-label">probes</span>
            </div>
            <div class="stat-item" v-if="analysis.status?.active_issues">
              <span class="stat-num text-warning">{{ analysis.status.active_issues }}</span>
              <span class="stat-label">issue{{ analysis.status.active_issues !== 1 ? 's' : '' }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Detected Incidents (the core of the AI analysis) -->
      <div v-if="analysis.incidents?.length" class="incidents-section">
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
            <div class="incident-info">
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
      <div v-else class="no-issues">
        <i class="bi bi-shield-check no-issues-icon"></i>
        <p class="mb-0">No issues detected — all paths performing within acceptable parameters</p>
      </div>

      <!-- Agent Health Overview (compact) -->
      <div class="agents-overview">
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

      <div class="text-muted small mt-3 text-end timestamp">
        <i class="bi bi-clock me-1"></i>
        {{ new Date(analysis.generated_at).toLocaleTimeString() }} · Auto-refreshes every 60s
      </div>
    </div>
  </div>
</template>

<style scoped>
.ai-status { 
  padding: 0; 
}

/* Status Banner - Theme Aware */
.status-banner {
  border: 1px solid var(--bs-border-color);
  border-radius: 12px;
  padding: 16px 20px;
  background: var(--bs-body-bg);
}

.status-banner.healthy {
  background: rgba(16, 185, 129, 0.1);
  border-color: rgba(16, 185, 129, 0.3);
}

.status-banner.degraded {
  background: rgba(245, 158, 11, 0.1);
  border-color: rgba(245, 158, 11, 0.3);
}

.status-banner.critical {
  background: rgba(239, 68, 68, 0.1);
  border-color: rgba(239, 68, 68, 0.3);
}

.status-banner.unknown {
  background: var(--bs-tertiary-bg);
  border-color: var(--bs-border-color);
}

.status-banner-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  flex-wrap: wrap;
}

.status-main {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 1;
  min-width: 0;
}

.status-icon { 
  font-size: 28px; 
  flex-shrink: 0;
}

.status-banner.healthy .status-icon { color: #10b981; }
.status-banner.degraded .status-icon { color: #f59e0b; }
.status-banner.critical .status-icon { color: #ef4444; }
.status-banner.unknown .status-icon { color: var(--bs-secondary-color); }

.status-text {
  min-width: 0;
}

.status-title {
  font-size: 18px;
  font-weight: 700;
  text-transform: capitalize;
  color: var(--bs-body-color);
}

.status-message {
  font-size: 13px;
  color: var(--bs-secondary-color);
  margin-top: 2px;
}

.status-stats { 
  display: flex;
  gap: 1.5rem;
  flex-shrink: 0;
}

.stat-item {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
}

.stat-num { 
  font-weight: 700; 
  font-size: 16px;
  color: var(--bs-body-color);
  line-height: 1;
}

.stat-label {
  font-size: 11px;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

/* Section Title */
.section-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin: 1.5rem 0 10px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.badge-count {
  font-size: 10px;
  font-weight: 700;
  background: var(--bs-primary);
  color: white;
  padding: 1px 6px;
  border-radius: 8px;
}

/* Incident Cards - Theme Aware */
.incident-card {
  background: var(--bs-body-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  padding: 12px 16px;
  margin-bottom: 8px;
  cursor: pointer;
  transition: transform 0.15s, box-shadow 0.15s;
}

.incident-card:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(0,0,0,0.1);
}

.incident-card.critical { 
  border-left: 4px solid #ef4444; 
}

.incident-card.warning { 
  border-left: 4px solid #f59e0b; 
}

.incident-card.info { 
  border-left: 4px solid #3b82f6; 
}

.incident-header {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.incident-severity-icon { 
  font-size: 18px; 
  margin-top: 2px; 
  flex-shrink: 0;
}

.incident-card.critical .incident-severity-icon { color: #ef4444; }
.incident-card.warning .incident-severity-icon  { color: #f59e0b; }
.incident-card.info .incident-severity-icon     { color: #3b82f6; }

.incident-info {
  flex: 1;
  min-width: 0;
}

.incident-title {
  font-weight: 600;
  font-size: 14px;
  color: var(--bs-body-color);
}

.incident-scope {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 2px;
  flex-wrap: wrap;
}

.scope-badge {
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  background: var(--bs-tertiary-bg);
  color: var(--bs-secondary-color);
  padding: 1px 6px;
  border-radius: 3px;
  border: 1px solid var(--bs-border-color);
}

.affected-list {
  font-size: 12px;
  color: var(--bs-secondary-color);
}

.expand-icon {
  color: var(--bs-secondary-color);
  margin-top: 4px;
  flex-shrink: 0;
}

/* AI Suggested Cause */
.suggested-cause {
  margin-top: 8px;
  padding: 8px 12px;
  background: rgba(59, 130, 246, 0.08);
  border: 1px solid rgba(59, 130, 246, 0.2);
  border-radius: 6px;
  font-size: 12px;
  color: var(--bs-body-color);
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
  border-top: 1px solid var(--bs-border-color);
}

.detail-section { 
  margin-bottom: 10px; 
}

.detail-label {
  font-size: 11px;
  font-weight: 600;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  margin-bottom: 4px;
}

.evidence-item {
  font-size: 12px;
  color: var(--bs-secondary-color);
  padding: 1px 0;
}

.target-chips { 
  display: flex; 
  gap: 6px; 
  flex-wrap: wrap; 
}

.target-chip {
  font-size: 11px;
  font-family: 'JetBrains Mono', monospace;
  background: var(--bs-tertiary-bg);
  color: var(--bs-body-color);
  padding: 2px 8px;
  border-radius: 4px;
  border: 1px solid var(--bs-border-color);
}

.recommendations-list {
  font-size: 12px;
  color: var(--bs-secondary-color);
  padding-left: 18px;
  margin-bottom: 0;
}

.recommendations-list li { 
  padding: 2px 0; 
}

/* No Issues */
.no-issues {
  text-align: center;
  padding: 24px;
  background: rgba(16, 185, 129, 0.08);
  border: 1px solid rgba(16, 185, 129, 0.2);
  border-radius: 10px;
  color: #10b981;
  margin-top: 1rem;
}

.no-issues-icon {
  font-size: 32px;
  display: block;
  margin-bottom: 8px;
}

.no-issues p { 
  font-size: 13px; 
}

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
  background: var(--bs-body-bg);
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

.agent-dot.online  { 
  background: #10b981; 
}

.agent-dot.offline { 
  background: #6b7280; 
}

.agent-chip-name { 
  color: var(--bs-body-color); 
}

.agent-chip-score { 
  font-weight: 700; 
}

.timestamp {
  padding-top: 1rem;
  border-top: 1px solid var(--bs-border-color);
}

/* Mobile Responsive */
@media (max-width: 768px) {
  .status-banner {
    padding: 12px 16px;
  }
  
  .status-banner-content {
    flex-direction: column;
    align-items: flex-start;
    gap: 12px;
  }
  
  .status-stats {
    width: 100%;
    justify-content: flex-start;
    gap: 1rem;
  }
  
  .stat-item {
    flex-direction: row;
    align-items: center;
    gap: 4px;
  }
  
  .status-icon {
    font-size: 24px;
  }
  
  .status-title {
    font-size: 16px;
  }
  
  .incident-card {
    padding: 10px 12px;
  }
  
  .incident-header {
    gap: 8px;
  }
  
  .incident-title {
    font-size: 13px;
  }
  
  .suggested-cause {
    font-size: 11px;
    padding: 6px 10px;
  }
  
  .agent-chip {
    padding: 3px 10px;
    font-size: 11px;
  }
}

/* Reduced Motion */
@media (prefers-reduced-motion: reduce) {
  .incident-card:hover {
    transform: none;
  }
}
</style>