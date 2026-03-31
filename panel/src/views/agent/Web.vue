<script lang="ts" setup>
import { ref, onMounted, computed } from 'vue'
import type { WebResult, WebGroup, WebDashboardData, WebGroupEntry } from '@/types'
import { ProbeDataService } from '@/services/apiService'
import Element from '@/components/Element.vue'
import { since } from '@/time'

const props = defineProps<{
  workspaceId: string | number
  agentId: string | number
}>()

const loading = ref(true)
const error = ref<string | null>(null)
const webData = ref<WebDashboardData | null>(null)
const expandedRows = ref<Record<string, boolean>>({})
const lookback = ref(60)

interface TargetGroup {
  target: string
  totalChecks: number
  entries: WebGroupEntry[]
  latest: WebGroupEntry
  overallStatus: 'healthy' | 'warning' | 'critical' | 'unknown'
}

const targetGroups = computed<TargetGroup[]>(() => {
  if (!webData.value?.groups?.length) return []

  return webData.value.groups.map(group => {
    const latest = group.entries[0!]
    let hasError = false
    let hasWarn = false

    for (const entry of group.entries) {
      const r = entry.payload
      if (r?.error) {
        hasError = true
      } else if (r?.status_code >= 500) {
        hasError = true
      } else if (r?.status_code >= 400) {
        hasWarn = true
      }
    }

    return {
      target: group.target,
      totalChecks: group.count,
      entries: group.entries,
      latest,
      overallStatus: hasError ? 'critical' : hasWarn ? 'warning' : group.entries.length > 0 ? 'healthy' : 'unknown'
    }
  })
})

const healthSummary = computed(() => {
  let totalTargets = 0
  let passing = 0
  let failing = 0

  for (const group of targetGroups.value) {
    totalTargets++
    const r = group.latest?.payload
    if (r && !r.error && r.status_code < 400) {
      passing++
    } else {
      failing++
    }
  }

  const status = failing === 0 ? 'healthy' : failing > passing ? 'critical' : 'warning'
  return { targets: targetGroups.value.length, passing, failing, status }
})

function getStatusColor(code: number, err?: string): string {
  if (err) return 'web-status-error'
  if (code >= 500) return 'web-status-error'
  if (code >= 400) return 'web-status-warn'
  if (code >= 200 && code < 300) return 'web-status-ok'
  return 'web-status-warn'
}

function getStatusIcon(code: number, err?: string): string {
  if (err) return 'bi-x-circle-fill'
  if (code >= 500) return 'bi-x-circle-fill'
  if (code >= 400) return 'bi-exclamation-triangle-fill'
  if (code >= 200 && code < 300) return 'bi-check-circle-fill'
  return 'bi-dash-circle-fill'
}

function getOverallStatusIcon(status: string): string {
  switch (status) {
    case 'healthy': return 'bi-check-circle-fill'
    case 'warning': return 'bi-exclamation-triangle-fill'
    case 'critical': return 'bi-x-circle-fill'
    default: return 'bi-question-circle'
  }
}

function toggleExpand(key: string) {
  expandedRows.value[key] = !expandedRows.value[key]
}

function formatLatency(ms: number): string {
  if (ms < 1) return `${(ms * 1000).toFixed(0)}µs`
  if (ms < 100) return `${ms.toFixed(2)}ms`
  return `${ms.toFixed(0)}ms`
}

function formatTime(t: string): string {
  try {
    const d = new Date(t)
    return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  } catch {
    return t
  }
}

function latencyClass(ms: number): string {
  if (ms < 200) return 'latency-good'
  if (ms < 500) return 'latency-warn'
  return 'latency-bad'
}

function certExpiryClass(days: number): string {
  if (days > 30) return 'cert-ok'
  if (days > 7) return 'cert-warn'
  return 'cert-expired'
}

async function fetchData() {
  loading.value = true
  error.value = null
  try {
    webData.value = await ProbeDataService.httpDashboard(
      props.workspaceId,
      props.agentId,
      { lookback: lookback.value, limit: 500 }
    )
  } catch (err: any) {
    error.value = err?.message || 'Failed to load web probe data'
  } finally {
    loading.value = false
  }
}

onMounted(fetchData)
</script>

<template>
  <div class="web-dashboard">
    <div class="web-summary">
      <div class="web-summary-stat" :class="healthSummary.status">
        <i class="bi" :class="getOverallStatusIcon(healthSummary.status)"></i>
        <div class="stat-text">
          <span class="stat-value">{{ healthSummary.targets }}</span>
          <span class="stat-label">Targets</span>
        </div>
      </div>
      <div class="web-summary-stat" :class="healthSummary.status">
        <i class="bi bi-hdd-network"></i>
        <div class="stat-text">
          <span class="stat-value">{{ webData?.total || 0 }}</span>
          <span class="stat-label">Checks</span>
        </div>
      </div>
      <div class="web-summary-stat healthy">
        <i class="bi bi-check-circle"></i>
        <div class="stat-text">
          <span class="stat-value">{{ healthSummary.passing }}</span>
          <span class="stat-label">Passing</span>
        </div>
      </div>
      <div class="web-summary-stat" :class="healthSummary.failing > 0 ? 'critical' : 'healthy'">
        <i class="bi bi-x-circle"></i>
        <div class="stat-text">
          <span class="stat-value">{{ healthSummary.failing }}</span>
          <span class="stat-label">Failing</span>
        </div>
      </div>
      <div class="web-summary-actions">
        <select v-model.number="lookback" @change="fetchData" class="lookback-select">
          <option :value="15">Last 15m</option>
          <option :value="60">Last 1h</option>
          <option :value="360">Last 6h</option>
          <option :value="1440">Last 24h</option>
          <option :value="10080">Last 7d</option>
        </select>
        <button class="btn-refresh" @click="fetchData" :disabled="loading">
          <i class="bi bi-arrow-clockwise" :class="{ 'spin': loading }"></i>
        </button>
      </div>
    </div>

    <div v-if="loading && !webData" class="web-loading">
      <div class="spinner"></div>
      <span>Loading web probe data...</span>
    </div>

    <div v-else-if="error" class="web-error">
      <i class="bi bi-exclamation-triangle"></i>
      <span>{{ error }}</span>
      <button @click="fetchData" class="retry-btn">Retry</button>
    </div>

    <div v-else-if="!targetGroups.length" class="web-empty">
      <i class="bi bi-globe"></i>
      <h4>No Web Data</h4>
      <p>No HTTP/TLS probes are configured for this agent, or no data has been collected yet.</p>
    </div>

    <div v-else class="web-groups">
      <Element
        v-for="group in targetGroups"
        :key="group.target"
        :title="group.target"
        icon="bi bi-globe"
        :subtitle="`${group.totalChecks} checks`"
      >
        <template #secondary>
          <div class="target-status" :class="group.overallStatus">
            <i class="bi" :class="getOverallStatusIcon(group.overallStatus)"></i>
          </div>
        </template>

        <div class="web-table-wrap">
          <table class="web-table">
            <thead>
              <tr>
                <th>URL</th>
                <th>Status</th>
                <th>Latency</th>
                <th>TLS</th>
                <th>Cert Expiry</th>
                <th>Last Check</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <template v-for="(entry, idx) in group.entries" :key="`${group.target}-${idx}`">
                <tr class="web-row">
                  <td class="mono url-cell">
                    <span class="url-text" :title="entry.payload?.url">{{ entry.payload?.url || entry.target }}</span>
                  </td>
                  <td>
                    <span class="status-badge" :class="getStatusColor(entry.payload?.status_code || 0, entry.payload?.error)">
                      <i class="bi" :class="getStatusIcon(entry.payload?.status_code || 0, entry.payload?.error)"></i>
                      {{ entry.payload?.error || entry.payload?.status_code }}
                    </span>
                  </td>
                  <td>
                    <span class="mono" :class="latencyClass(entry.payload?.total_ms || 0)">
                      {{ formatLatency(entry.payload?.total_ms || 0) }}
                    </span>
                  </td>
                  <td>
                    <span class="badge-tls">{{ entry.payload?.tls_version || '—' }}</span>
                  </td>
                  <td>
                    <span v-if="entry.payload?.certificate_info" class="cert-badge" :class="certExpiryClass(entry.payload.certificate_info.days_until_expiry)">
                      {{ entry.payload.certificate_info.days_until_expiry }}d
                    </span>
                    <span v-else class="muted">—</span>
                  </td>
                  <td class="time-cell">
                    {{ since(entry.created_at, true) }}
                  </td>
                  <td class="action-cell">
                    <button
                      v-if="group.entries.length > 1"
                      @click="toggleExpand(`${group.target}-${idx}`)"
                      class="expand-btn"
                      :title="`${group.entries.length} historical results`"
                    >
                      <i class="bi" :class="expandedRows[`${group.target}-${idx}`] ? 'bi-chevron-up' : 'bi-chevron-down'"></i>
                    </button>
                  </td>
                </tr>

                <tr v-if="expandedRows[`${group.target}-${idx}`]" class="detail-row">
                  <td colspan="7">
                    <div class="detail-panel">
                      <div class="detail-grid">
                        <div class="detail-item">
                          <span class="detail-label">DNS Lookup</span>
                          <span class="detail-value mono">{{ formatLatency(entry.payload?.dns_lookup_ms || 0) }}</span>
                        </div>
                        <div class="detail-item">
                          <span class="detail-label">TCP Connect</span>
                          <span class="detail-value mono">{{ formatLatency(entry.payload?.tcp_connect_ms || 0) }}</span>
                        </div>
                        <div class="detail-item">
                          <span class="detail-label">TLS Handshake</span>
                          <span class="detail-value mono">{{ formatLatency(entry.payload?.tls_handshake_ms || 0) }}</span>
                        </div>
                        <div class="detail-item">
                          <span class="detail-label">First Byte</span>
                          <span class="detail-value mono">{{ formatLatency(entry.payload?.first_byte_ms || 0) }}</span>
                        </div>
                        <div class="detail-item">
                          <span class="detail-label">Total</span>
                          <span class="detail-value mono">{{ formatLatency(entry.payload?.total_ms || 0) }}</span>
                        </div>
                        <div class="detail-item">
                          <span class="detail-label">Body Size</span>
                          <span class="detail-value mono">{{ entry.payload?.body_size || 0 }} bytes</span>
                        </div>
                        <div class="detail-item">
                          <span class="detail-label">Protocol</span>
                          <span class="detail-value">{{ entry.payload?.protocol || '—' }}</span>
                        </div>
                        <div class="detail-item">
                          <span class="detail-label">Cipher Suite</span>
                          <span class="detail-value mono">{{ entry.payload?.tls_cipher_suite || '—' }}</span>
                        </div>
                      </div>
                      <div v-if="entry.payload?.certificate_info" class="cert-info">
                        <div class="cert-title">Certificate</div>
                        <div class="cert-details">
                          <span><strong>Subject:</strong> {{ entry.payload.certificate_info.subject }}</span>
                          <span><strong>Issuer:</strong> {{ entry.payload.certificate_info.issuer }}</span>
                          <span><strong>Valid:</strong> {{ entry.payload.certificate_info.not_before }} to {{ entry.payload.certificate_info.not_after }}</span>
                        </div>
                      </div>
                    </div>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </Element>
    </div>
  </div>
</template>

<style scoped>
.web-dashboard {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.web-summary {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.75rem 1rem;
  background: var(--card-bg, #fff);
  border: 1px solid var(--border-color, #e0e0e0);
  border-radius: 8px;
  flex-wrap: wrap;
}

.web-summary-stat {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.4rem 0.75rem;
  border-radius: 6px;
  background: var(--bg-subtle, #f8f9fa);
}
.web-summary-stat.healthy { color: var(--success, #10b981); }
.web-summary-stat.warning { color: var(--warning, #f59e0b); }
.web-summary-stat.critical { color: var(--danger, #ef4444); }
.web-summary-stat.unknown { color: var(--muted, #6b7280); }

.stat-text { display: flex; flex-direction: column; line-height: 1.2; }
.stat-value { font-weight: 700; font-size: 1.1rem; }
.stat-label { font-size: 0.72rem; opacity: 0.7; text-transform: uppercase; letter-spacing: 0.03em; }

.web-summary-actions {
  margin-left: auto;
  display: flex;
  gap: 0.5rem;
  align-items: center;
}

.lookback-select {
  padding: 0.3rem 0.5rem;
  border-radius: 6px;
  border: 1px solid var(--border-color, #d1d5db);
  background: var(--bg-subtle, #f8f9fa);
  font-size: 0.8rem;
  cursor: pointer;
  color: inherit;
}

.btn-refresh {
  padding: 0.35rem 0.5rem;
  border-radius: 6px;
  border: 1px solid var(--border-color, #d1d5db);
  background: var(--bg-subtle, #f8f9fa);
  cursor: pointer;
  font-size: 0.85rem;
  color: inherit;
  transition: background 0.15s;
}
.btn-refresh:hover { background: var(--border-color, #e5e7eb); }
.btn-refresh:disabled { opacity: 0.5; cursor: not-allowed; }

.spin { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }

.web-loading, .web-error, .web-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 3rem 1rem;
  gap: 0.75rem;
  color: var(--muted, #6b7280);
}
.web-error { color: var(--danger, #ef4444); }
.web-empty i { font-size: 2.5rem; opacity: 0.3; }
.web-empty h4 { margin: 0; font-weight: 600; }
.web-empty p { margin: 0; opacity: 0.6; font-size: 0.9rem; }
.retry-btn { padding: 0.3rem 0.75rem; border-radius: 6px; border: 1px solid currentColor; background: transparent; cursor: pointer; color: inherit; }
.spinner { width: 24px; height: 24px; border: 2px solid var(--border-color, #d1d5db); border-top-color: var(--primary, #3b82f6); border-radius: 50%; animation: spin 0.6s linear infinite; }

.web-groups { display: flex; flex-direction: column; gap: 0.75rem; }

.target-status {
  display: flex;
  align-items: center;
  font-size: 1rem;
}
.target-status.healthy { color: var(--success, #10b981); }
.target-status.warning { color: var(--warning, #f59e0b); }
.target-status.critical { color: var(--danger, #ef4444); }
.target-status.unknown { color: var(--muted, #6b7280); }

.web-table-wrap {
  overflow-x: auto;
}

.web-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.82rem;
}

.web-table thead th {
  padding: 0.5rem 0.75rem;
  text-align: left;
  font-size: 0.68rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.5;
  font-weight: 600;
  border-bottom: 1px solid var(--border-color, #e5e7eb);
  white-space: nowrap;
}

.web-table tbody td {
  padding: 0.6rem 0.75rem;
  border-bottom: 1px solid var(--border-color, #f1f5f9);
  vertical-align: middle;
}

.web-row:hover td {
  background: var(--bg-subtle, #f8f9fa);
}

.url-cell {
  max-width: 300px;
}
.url-text {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mono {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 0.78rem;
}

.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.15rem 0.5rem;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 600;
  white-space: nowrap;
}

.web-status-ok { color: var(--success, #10b981); background: rgba(16, 185, 129, 0.1); }
.web-status-warn { color: var(--warning, #f59e0b); background: rgba(245, 158, 11, 0.1); }
.web-status-error { color: var(--danger, #ef4444); background: rgba(239, 68, 68, 0.1); }

.latency-good { color: var(--success, #10b981); }
.latency-warn { color: var(--warning, #f59e0b); }
.latency-bad { color: var(--danger, #ef4444); }

.badge-tls {
  display: inline-block;
  padding: 0.1rem 0.4rem;
  background: var(--bg-subtle, #f1f5f9);
  border-radius: 3px;
  font-size: 0.7rem;
  font-weight: 600;
}

.cert-badge {
  display: inline-block;
  padding: 0.1rem 0.4rem;
  border-radius: 3px;
  font-size: 0.7rem;
  font-weight: 600;
}
.cert-ok { color: var(--success, #10b981); background: rgba(16, 185, 129, 0.1); }
.cert-warn { color: var(--warning, #f59e0b); background: rgba(245, 158, 11, 0.1); }
.cert-expired { color: var(--danger, #ef4444); background: rgba(239, 68, 68, 0.1); }

.muted { opacity: 0.5; }

.time-cell { white-space: nowrap; opacity: 0.7; font-size: 0.78rem; }

.action-cell {
  white-space: nowrap;
  display: flex;
  gap: 0.25rem;
  align-items: center;
}

.expand-btn {
  background: none;
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 4px;
  padding: 0.15rem 0.4rem;
  font-size: 0.7rem;
  cursor: pointer;
  color: var(--muted, #6b7280);
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  transition: all 0.15s;
}
.expand-btn:hover {
  background: var(--bg-subtle, #f1f5f9);
  color: var(--text, #111);
}

.detail-row td {
  padding: 0 !important;
}

.detail-panel {
  padding: 1rem;
  background: var(--bg-subtle, #f8f9fa);
  border-top: 1px solid var(--border-color, #f1f5f9);
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 0.75rem;
}

.detail-item {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
}

.detail-label {
  font-size: 0.68rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.5;
  font-weight: 600;
}

.detail-value {
  font-size: 0.82rem;
}

.cert-info {
  margin-top: 1rem;
  padding-top: 0.75rem;
  border-top: 1px solid var(--border-color, #e5e7eb);
}

.cert-title {
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.5;
  font-weight: 600;
  margin-bottom: 0.5rem;
}

.cert-details {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  font-size: 0.78rem;
}
</style>
