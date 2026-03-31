<script lang="ts" setup>
import { ref, onMounted, computed } from 'vue'
import type { WebResult, WebDashboardData, WebGroupEntry } from '@/types'
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

function isHTTP(entry: WebGroupEntry): boolean {
  return !!entry.payload.status_code || !!entry.payload.url
}

interface TargetGroup {
  target: string
  totalChecks: number
  entries: WebGroupEntry[]
  latest: WebGroupEntry
  overallStatus: 'healthy' | 'warning' | 'critical' | 'unknown'
  type: 'HTTP' | 'TLS' | 'mixed'
}

const targetGroups = computed<TargetGroup[]>(() => {
  if (!webData.value?.groups?.length) return []

  return webData.value.groups.map(group => {
    const latest = group.entries[0!]
    let hasError = false
    let hasWarn = false
    let hasHTTP = false
    let hasTLS = false

    for (const entry of group.entries) {
      if (isHTTP(entry)) {
        hasHTTP = true
        const r = entry.payload
        if (r?.error) {
          hasError = true
        } else if ((r?.status_code || 0) >= 500) {
          hasError = true
        } else if ((r?.status_code || 0) >= 400) {
          hasWarn = true
        }
      } else {
        hasTLS = true
        const r = entry.payload
        if (r?.error || r?.is_expired) {
          hasError = true
        } else if (r?.is_expiring_soon) {
          hasWarn = true
        }
      }
    }

    let type: 'HTTP' | 'TLS' | 'mixed' = 'HTTP'
    if (hasHTTP && hasTLS) type = 'mixed'
    else if (hasTLS) type = 'TLS'

    return {
      target: group.target,
      totalChecks: group.count,
      entries: group.entries,
      latest,
      overallStatus: hasError ? 'critical' : hasWarn ? 'warning' : group.entries.length > 0 ? 'healthy' : 'unknown',
      type
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

    if (isHTTP(group.latest)) {
      if (r && !r.error && (r.status_code || 0) < 400) {
        passing++
      } else {
        failing++
      }
    } else {
      if (r && !r.error && !r.is_expired && !r.is_expiring_soon) {
        passing++
      } else {
        failing++
      }
    }
  }

  const status = failing === 0 ? 'healthy' : failing > passing ? 'critical' : 'warning'
  return { targets: targetGroups.value.length, passing, failing, status }
})

function getStatusColor(entry: WebGroupEntry): string {
  const r = entry.payload
  if (r?.error) return 'web-status-error'

  if (isHTTP(entry)) {
    const code = r?.status_code || 0
    if (code >= 500) return 'web-status-error'
    if (code >= 400) return 'web-status-warn'
    if (code >= 200 && code < 300) return 'web-status-ok'
    return 'web-status-warn'
  } else {
    if (r?.is_expired) return 'web-status-error'
    if (r?.is_expiring_soon) return 'web-status-warn'
    return 'web-status-ok'
  }
}

function getStatusIcon(entry: WebGroupEntry): string {
  const r = entry.payload
  if (r?.error) return 'bi-x-circle-fill'

  if (isHTTP(entry)) {
    const code = r?.status_code || 0
    if (code >= 500) return 'bi-x-circle-fill'
    if (code >= 400) return 'bi-exclamation-triangle-fill'
    if (code >= 200 && code < 300) return 'bi-check-circle-fill'
    return 'bi-dash-circle-fill'
  } else {
    if (r?.is_expired) return 'bi-x-circle-fill'
    if (r?.is_expiring_soon) return 'bi-exclamation-triangle-fill'
    return 'bi-check-circle-fill'
  }
}

function getStatusText(entry: WebGroupEntry): string {
  const r = entry.payload
  if (r?.error) return r.error

  if (isHTTP(entry)) {
    return r?.status_code ? String(r.status_code) : '—'
  } else {
    if (r?.is_expired) return 'EXPIRED'
    if (r?.is_expiring_soon) return 'EXPIRING SOON'
    if (r?.days_until_expiry !== undefined) return `${r.days_until_expiry}d valid`
    return 'OK'
  }
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

function formatLatency(ms: number | undefined): string {
  if (ms === undefined || ms === null) return '—'
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

function latencyClass(ms: number | undefined): string {
  if (ms === undefined || ms === null) return ''
  if (ms < 200) return 'latency-good'
  if (ms < 500) return 'latency-warn'
  return 'latency-bad'
}

function certExpiryClass(days: number | undefined): string {
  if (days === undefined) return ''
  if (days > 30) return 'cert-ok'
  if (days > 7) return 'cert-warn'
  return 'cert-expired'
}

function getCertInfo(entry: WebGroupEntry) {
  const r = entry.payload
  if (r?.certificate_info) return r.certificate_info
  if (r?.certificate) return r.certificate
  return null
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
        :subtitle="`${group.totalChecks} checks · ${group.type}`"
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
                <th>URL / Host</th>
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
                    <span class="url-text" :title="entry.payload?.url || entry.target">{{ entry.payload?.url || entry.target }}</span>
                    <span v-if="isHTTP(entry)" class="probe-type-badge http">HTTP</span>
                    <span v-else class="probe-type-badge tls">TLS</span>
                  </td>
                  <td>
                    <span class="status-badge" :class="getStatusColor(entry)">
                      <i class="bi" :class="getStatusIcon(entry)"></i>
                      {{ getStatusText(entry) }}
                    </span>
                  </td>
                  <td>
                    <span v-if="isHTTP(entry)" class="mono" :class="latencyClass(entry.payload?.total_ms)">
                      {{ formatLatency(entry.payload?.total_ms) }}
                    </span>
                    <span v-else class="mono" :class="latencyClass(entry.payload?.total_ms)">
                      {{ formatLatency(entry.payload?.total_ms) }}
                    </span>
                  </td>
                  <td>
                    <span class="badge-tls">{{ entry.payload?.tls_version || '—' }}</span>
                  </td>
                  <td>
                    <span v-if="getCertInfo(entry)" class="cert-badge" :class="certExpiryClass(getCertInfo(entry)?.days_until_expiry)">
                      {{ getCertInfo(entry)?.days_until_expiry }}d
                    </span>
                    <span v-else-if="entry.payload?.days_until_expiry !== undefined" class="cert-badge" :class="certExpiryClass(entry.payload.days_until_expiry)">
                      {{ entry.payload.days_until_expiry }}d
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
                      <div v-if="isHTTP(entry)" class="detail-grid">
                        <div class="detail-item">
                          <span class="detail-label">DNS Lookup</span>
                          <span class="detail-value mono">{{ formatLatency(entry.payload?.dns_lookup_ms) }}</span>
                        </div>
                        <div class="detail-item">
                          <span class="detail-label">TCP Connect</span>
                          <span class="detail-value mono">{{ formatLatency(entry.payload?.tcp_connect_ms) }}</span>
                        </div>
                        <div class="detail-item">
                          <span class="detail-label">TLS Handshake</span>
                          <span class="detail-value mono">{{ formatLatency(entry.payload?.tls_handshake_ms) }}</span>
                        </div>
                        <div class="detail-item">
                          <span class="detail-label">First Byte</span>
                          <span class="detail-value mono">{{ formatLatency(entry.payload?.first_byte_ms) }}</span>
                        </div>
                        <div class="detail-item">
                          <span class="detail-label">Total</span>
                          <span class="detail-value mono">{{ formatLatency(entry.payload?.total_ms) }}</span>
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
                      <div v-else class="detail-grid">
                        <div class="detail-item">
                          <span class="detail-label">Connect</span>
                          <span class="detail-value mono">{{ formatLatency(entry.payload?.total_ms) }}</span>
                        </div>
                        <div class="detail-item">
                          <span class="detail-label">Protocol</span>
                          <span class="detail-value">{{ entry.payload?.protocol || '—' }}</span>
                        </div>
                        <div class="detail-item">
                          <span class="detail-label">Cipher Suite</span>
                          <span class="detail-value mono">{{ entry.payload?.tls_cipher_suite || '—' }}</span>
                        </div>
                        <div class="detail-item">
                          <span class="detail-label">RemoteAddr</span>
                          <span class="detail-value mono">{{ entry.payload?.remote_addr || '—' }}</span>
                        </div>
                      </div>
                      <div v-if="getCertInfo(entry)" class="cert-info">
                        <div class="cert-title">Certificate</div>
                        <div class="cert-details">
                          <span><strong>Subject:</strong> {{ getCertInfo(entry)?.subject }}</span>
                          <span><strong>Issuer:</strong> {{ getCertInfo(entry)?.issuer }}</span>
                          <span><strong>Valid:</strong> {{ getCertInfo(entry)?.not_before }} to {{ getCertInfo(entry)?.not_after }}</span>
                          <span v-if="entry.payload?.certificate?.issuer_org"><strong>Issuer Org:</strong> {{ entry.payload.certificate.issuer_org }}</span>
                          <span v-if="entry.payload?.certificate?.cert_type"><strong>Cert Type:</strong> {{ entry.payload.certificate.cert_type }}</span>
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
  background: var(--card-bg);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  flex-wrap: wrap;
}

.web-summary-stat {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.4rem 0.75rem;
  border-radius: 6px;
  background: var(--bg-subtle);
}
.web-summary-stat.healthy { color: var(--success); }
.web-summary-stat.warning { color: var(--warning); }
.web-summary-stat.critical { color: var(--danger); }
.web-summary-stat.unknown { color: var(--muted); }

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
  border: 1px solid var(--border-color);
  background: var(--bg-subtle);
  font-size: 0.8rem;
  cursor: pointer;
  color: var(--text);
}

.btn-refresh {
  padding: 0.35rem 0.5rem;
  border-radius: 6px;
  border: 1px solid var(--border-color);
  background: var(--bg-subtle);
  cursor: pointer;
  font-size: 0.85rem;
  color: var(--text);
  transition: background 0.15s;
}
.btn-refresh:hover { background: var(--border-color); }
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
  color: var(--muted);
}
.web-error { color: var(--danger); }
.web-empty i { font-size: 2.5rem; opacity: 0.3; }
.web-empty h4 { margin: 0; font-weight: 600; color: var(--text); }
.web-empty p { margin: 0; opacity: 0.6; font-size: 0.9rem; color: var(--muted); }
.retry-btn { padding: 0.3rem 0.75rem; border-radius: 6px; border: 1px solid currentColor; background: transparent; cursor: pointer; color: inherit; }
.spinner { width: 24px; height: 24px; border: 2px solid var(--border-color); border-top-color: var(--primary); border-radius: 50%; animation: spin 0.6s linear infinite; }

.web-groups { display: flex; flex-direction: column; gap: 0.75rem; }

.target-status {
  display: flex;
  align-items: center;
  font-size: 1rem;
}
.target-status.healthy { color: var(--success); }
.target-status.warning { color: var(--warning); }
.target-status.critical { color: var(--danger); }
.target-status.unknown { color: var(--muted); }

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
  border-bottom: 1px solid var(--border-color);
  white-space: nowrap;
  color: var(--text);
}

.web-table tbody td {
  padding: 0.6rem 0.75rem;
  border-bottom: 1px solid var(--border-color);
  vertical-align: middle;
  color: var(--text);
}

.web-row:hover td {
  background: var(--bg-subtle);
}

.url-cell {
  max-width: 300px;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.url-text {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.probe-type-badge {
  display: inline-block;
  padding: 0.1rem 0.35rem;
  border-radius: 3px;
  font-size: 0.6rem;
  font-weight: 700;
  text-transform: uppercase;
  flex-shrink: 0;
}
.probe-type-badge.http {
  background: rgba(59, 130, 246, 0.15);
  color: #3b82f6;
}
.probe-type-badge.tls {
  background: rgba(168, 85, 247, 0.15);
  color: #a855f7;
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

.web-status-ok { color: var(--success); background: var(--success-bg); }
.web-status-warn { color: var(--warning); background: var(--warning-bg); }
.web-status-error { color: var(--danger); background: var(--danger-bg); }

.latency-good { color: var(--success); }
.latency-warn { color: var(--warning); }
.latency-bad { color: var(--danger); }

.badge-tls {
  display: inline-block;
  padding: 0.1rem 0.4rem;
  background: var(--bg-subtle);
  border-radius: 3px;
  font-size: 0.7rem;
  font-weight: 600;
  color: var(--text);
}

.cert-badge {
  display: inline-block;
  padding: 0.1rem 0.4rem;
  border-radius: 3px;
  font-size: 0.7rem;
  font-weight: 600;
}
.cert-ok { color: var(--success); background: var(--success-bg); }
.cert-warn { color: var(--warning); background: var(--warning-bg); }
.cert-expired { color: var(--danger); background: var(--danger-bg); }

.muted { opacity: 0.5; color: var(--muted); }

.time-cell { white-space: nowrap; opacity: 0.7; font-size: 0.78rem; color: var(--muted); }

.action-cell {
  white-space: nowrap;
  display: flex;
  gap: 0.25rem;
  align-items: center;
}

.expand-btn {
  background: none;
  border: 1px solid var(--border-color);
  border-radius: 4px;
  padding: 0.15rem 0.4rem;
  font-size: 0.7rem;
  cursor: pointer;
  color: var(--muted);
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  transition: all 0.15s;
}
.expand-btn:hover {
  background: var(--bg-subtle);
  color: var(--text);
}

.detail-row td {
  padding: 0 !important;
}

.detail-panel {
  padding: 1rem;
  background: var(--bg-subtle);
  border-top: 1px solid var(--border-color);
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
  color: var(--muted);
}

.detail-value {
  font-size: 0.82rem;
  color: var(--text);
}

.cert-info {
  margin-top: 1rem;
  padding-top: 0.75rem;
  border-top: 1px solid var(--border-color);
}

.cert-title {
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.5;
  font-weight: 600;
  margin-bottom: 0.5rem;
  color: var(--muted);
}

.cert-details {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  font-size: 0.78rem;
  color: var(--text);
}
</style>
