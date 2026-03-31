<script lang="ts" setup>
import { ref, onMounted, computed } from 'vue'
import type { DNSResult, DNSGroup, DNSDashboardData, DNSGroupEntry } from '@/types'
import { ProbeDataService } from '@/services/apiService'
import Element from '@/components/Element.vue'
import { since } from '@/time'

const props = defineProps<{
  workspaceId: string | number
  agentId: string | number
}>()

const loading = ref(true)
const error = ref<string | null>(null)
const dnsData = ref<DNSDashboardData | null>(null)
const expandedRaw = ref<Record<string, boolean>>({})
const expandedServers = ref<Record<string, boolean>>({})
const lookback = ref(60)

// Derived: for each target, sub-group entries by dns_server
interface ServerSubGroup {
  server: string
  recordType: string
  protocol: string
  latest: DNSGroupEntry
  history: DNSGroupEntry[]  // All entries for this server (chronological, newest first)
}

interface HostGroup {
  target: string
  totalChecks: number
  servers: ServerSubGroup[]
  overallStatus: 'healthy' | 'warning' | 'critical' | 'unknown'
}

const hostGroups = computed<HostGroup[]>(() => {
  if (!dnsData.value?.groups?.length) return []

  return dnsData.value.groups.map(group => {
    // Sub-group by dns_server
    const serverMap = new Map<string, DNSGroupEntry[]>()
    const serverOrder: string[] = []

    for (const entry of group.entries) {
      const server = entry.payload?.dns_server || 'unknown'
      if (!serverMap.has(server)) {
        serverMap.set(server, [])
        serverOrder.push(server)
      }
      serverMap.get(server)!.push(entry)
    }

    const servers: ServerSubGroup[] = serverOrder.map(server => {
      const entries = serverMap.get(server)!
      const latest = entries[0]!  // guaranteed non-empty: we only add servers when we find entries
      return {
        server,
        recordType: latest.payload?.record_type || 'A',
        protocol: latest.payload?.protocol || 'udp',
        latest,
        history: entries
      }
    })

    // Overall status: worst of all servers
    let hasError = false
    let hasWarn = false
    for (const s of servers) {
      const r = s.latest?.payload
      if (r?.error || r?.response_code === 'SERVFAIL' || r?.response_code === 'REFUSED') {
        hasError = true
      } else if (r?.response_code === 'NXDOMAIN') {
        hasWarn = true
      }
    }

    return {
      target: group.target,
      totalChecks: group.count,
      servers,
      overallStatus: hasError ? 'critical' : hasWarn ? 'warning' : servers.length > 0 ? 'healthy' : 'unknown'
    }
  })
})

// Summary counts
const healthSummary = computed(() => {
  let totalServers = 0
  let passing = 0
  let failing = 0

  for (const group of hostGroups.value) {
    for (const s of group.servers) {
      totalServers++
      const r = s.latest?.payload
      if (r && r.response_code === 'NOERROR' && !r.error) {
        passing++
      } else {
        failing++
      }
    }
  }

  const status = failing === 0 ? 'healthy' : failing > passing ? 'critical' : 'warning'
  return { hosts: hostGroups.value.length, totalServers, passing, failing, status }
})

function getStatusColor(code: string, err?: string): string {
  if (err) return 'dns-status-error'
  switch (code) {
    case 'NOERROR': return 'dns-status-ok'
    case 'NXDOMAIN': return 'dns-status-warn'
    case 'SERVFAIL': return 'dns-status-error'
    case 'REFUSED': return 'dns-status-error'
    default: return 'dns-status-warn'
  }
}

function getStatusIcon(code: string, err?: string): string {
  if (err) return 'bi-x-circle-fill'
  switch (code) {
    case 'NOERROR': return 'bi-check-circle-fill'
    case 'NXDOMAIN': return 'bi-exclamation-triangle-fill'
    default: return 'bi-dash-circle-fill'
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

function toggleRaw(key: string) {
  expandedRaw.value[key] = !expandedRaw.value[key]
}

function toggleServerHistory(key: string) {
  expandedServers.value[key] = !expandedServers.value[key]
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
  if (ms < 50) return 'latency-good'
  if (ms < 200) return 'latency-warn'
  return 'latency-bad'
}

function answersPreview(answers: { value: string; type: string }[]): string {
  if (!answers?.length) return '—'
  return answers.slice(0, 3).map(a => a.value).join(', ') + (answers.length > 3 ? ` +${answers.length - 3}` : '')
}

async function fetchData() {
  loading.value = true
  error.value = null
  try {
    dnsData.value = await ProbeDataService.dnsDashboard(
      props.workspaceId,
      props.agentId,
      { lookback: lookback.value, limit: 500 }
    )
  } catch (err: any) {
    error.value = err?.message || 'Failed to load DNS data'
  } finally {
    loading.value = false
  }
}

onMounted(fetchData)
</script>

<template>
  <div class="dns-dashboard">
    <!-- Summary Header -->
    <div class="dns-summary">
      <div class="dns-summary-stat" :class="healthSummary.status">
        <i class="bi" :class="getOverallStatusIcon(healthSummary.status)"></i>
        <div class="stat-text">
          <span class="stat-value">{{ healthSummary.hosts }}</span>
          <span class="stat-label">Hosts</span>
        </div>
      </div>
      <div class="dns-summary-stat" :class="healthSummary.status">
        <i class="bi bi-hdd-network"></i>
        <div class="stat-text">
          <span class="stat-value">{{ healthSummary.totalServers }}</span>
          <span class="stat-label">Resolvers</span>
        </div>
      </div>
      <div class="dns-summary-stat healthy">
        <i class="bi bi-check-circle"></i>
        <div class="stat-text">
          <span class="stat-value">{{ healthSummary.passing }}</span>
          <span class="stat-label">Passing</span>
        </div>
      </div>
      <div class="dns-summary-stat" :class="healthSummary.failing > 0 ? 'critical' : 'healthy'">
        <i class="bi bi-x-circle"></i>
        <div class="stat-text">
          <span class="stat-value">{{ healthSummary.failing }}</span>
          <span class="stat-label">Failing</span>
        </div>
      </div>
      <div class="dns-summary-actions">
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

    <!-- Loading State -->
    <div v-if="loading && !dnsData" class="dns-loading">
      <div class="spinner"></div>
      <span>Loading DNS data...</span>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="dns-error">
      <i class="bi bi-exclamation-triangle"></i>
      <span>{{ error }}</span>
      <button @click="fetchData" class="retry-btn">Retry</button>
    </div>

    <!-- Empty State -->
    <div v-else-if="!hostGroups.length" class="dns-empty">
      <i class="bi bi-globe2"></i>
      <h4>No DNS Data</h4>
      <p>No DNS probes are configured for this agent, or no data has been collected yet.</p>
    </div>

    <!-- Host Groups -->
    <div v-else class="dns-groups">
      <Element
        v-for="host in hostGroups"
        :key="host.target"
        :title="host.target"
        icon="bi bi-globe2"
        :subtitle="`${host.servers.length} resolver${host.servers.length !== 1 ? 's' : ''} · ${host.totalChecks} checks`"
      >
        <template #secondary>
          <div class="host-status" :class="host.overallStatus">
            <i class="bi" :class="getOverallStatusIcon(host.overallStatus)"></i>
          </div>
        </template>

        <!-- Server Comparison Table (latest results) -->
        <div class="server-table-wrap">
          <table class="server-table">
            <thead>
              <tr>
                <th>DNS Server</th>
                <th>Type</th>
                <th>Status</th>
                <th>Latency</th>
                <th>Answer</th>
                <th>Last Check</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <template v-for="(srv, sIdx) in host.servers" :key="srv.server">
                <!-- Latest result row -->
                <tr class="server-row">
                  <td class="mono server-cell">
                    <i class="bi bi-hdd-network"></i>
                    {{ srv.server }}
                  </td>
                  <td>
                    <span class="badge-type">{{ srv.recordType }}</span>
                  </td>
                  <td>
                    <span class="status-badge" :class="getStatusColor(srv.latest.payload?.response_code || '', srv.latest.payload?.error)">
                      <i class="bi" :class="getStatusIcon(srv.latest.payload?.response_code || '', srv.latest.payload?.error)"></i>
                      {{ srv.latest.payload?.error || srv.latest.payload?.response_code }}
                    </span>
                  </td>
                  <td>
                    <span class="mono" :class="latencyClass(srv.latest.payload?.query_time_ms || 0)">
                      {{ formatLatency(srv.latest.payload?.query_time_ms || 0) }}
                    </span>
                  </td>
                  <td class="answer-cell mono">
                    {{ answersPreview(srv.latest.payload?.answers || []) }}
                  </td>
                  <td class="time-cell">
                    {{ since(srv.latest.created_at, true) }}
                  </td>
                  <td class="action-cell">
                    <button
                      v-if="srv.history.length > 1"
                      @click="toggleServerHistory(`${host.target}::${srv.server}`)"
                      class="history-btn"
                      :title="`${srv.history.length} historical results`"
                    >
                      <i class="bi" :class="expandedServers[`${host.target}::${srv.server}`] ? 'bi-chevron-up' : 'bi-chevron-down'"></i>
                      {{ srv.history.length }}
                    </button>
                    <button
                      @click="toggleRaw(`${host.target}::${srv.server}::latest`)"
                      class="raw-toggle-btn"
                      title="View raw DNS response"
                    >
                      <i class="bi bi-code-slash"></i>
                    </button>
                  </td>
                </tr>

                <!-- Raw response for latest (inline) -->
                <tr v-if="expandedRaw[`${host.target}::${srv.server}::latest`]">
                  <td colspan="7" class="raw-cell">
                    <pre>{{ srv.latest.payload?.raw_response }}</pre>
                  </td>
                </tr>

                <!-- Historical results table -->
                <tr v-if="expandedServers[`${host.target}::${srv.server}`]">
                  <td colspan="7" class="history-cell">
                    <div class="history-panel">
                      <div class="history-title">
                        <i class="bi bi-clock-history"></i>
                        History — {{ srv.server }}
                        <span class="history-count">({{ srv.history.length }} results)</span>
                      </div>
                      <table class="history-table">
                        <thead>
                          <tr>
                            <th>Time</th>
                            <th>Status</th>
                            <th>Latency</th>
                            <th>Answer</th>
                            <th></th>
                          </tr>
                        </thead>
                        <tbody>
                          <template v-for="(h, hIdx) in srv.history" :key="hIdx">
                            <tr>
                              <td class="time-cell">{{ formatTime(h.created_at) }}</td>
                              <td>
                                <span class="status-badge sm" :class="getStatusColor(h.payload?.response_code || '', h.payload?.error)">
                                  <i class="bi" :class="getStatusIcon(h.payload?.response_code || '', h.payload?.error)"></i>
                                  {{ h.payload?.error || h.payload?.response_code }}
                                </span>
                              </td>
                              <td>
                                <span class="mono" :class="latencyClass(h.payload?.query_time_ms || 0)">
                                  {{ formatLatency(h.payload?.query_time_ms || 0) }}
                                </span>
                              </td>
                              <td class="answer-cell mono">
                                {{ answersPreview(h.payload?.answers || []) }}
                              </td>
                              <td class="action-cell">
                                <button
                                  @click="toggleRaw(`${host.target}::${srv.server}::${hIdx}`)"
                                  class="raw-toggle-btn sm"
                                  title="Raw"
                                >
                                  <i class="bi bi-code-slash"></i>
                                </button>
                              </td>
                            </tr>
                            <!-- Inline raw response immediately after the history row -->
                            <tr v-if="expandedRaw[`${host.target}::${srv.server}::${hIdx}`]" class="raw-inline-row">
                              <td colspan="5" class="raw-cell">
                                <div class="raw-header">{{ srv.server }} · {{ formatTime(h.created_at) }}</div>
                                <pre>{{ h.payload?.raw_response }}</pre>
                              </td>
                            </tr>
                          </template>
                        </tbody>
                      </table>
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
.dns-dashboard {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

/* Summary Header */
.dns-summary {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.75rem 1rem;
  background: var(--card-bg, #fff);
  border: 1px solid var(--border-color, #e0e0e0);
  border-radius: 8px;
  flex-wrap: wrap;
}

.dns-summary-stat {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.4rem 0.75rem;
  border-radius: 6px;
  background: var(--bg-subtle, #f8f9fa);
}
.dns-summary-stat.healthy { color: var(--success, #10b981); }
.dns-summary-stat.warning { color: var(--warning, #f59e0b); }
.dns-summary-stat.critical { color: var(--danger, #ef4444); }
.dns-summary-stat.unknown { color: var(--muted, #6b7280); }

.stat-text { display: flex; flex-direction: column; line-height: 1.2; }
.stat-value { font-weight: 700; font-size: 1.1rem; }
.stat-label { font-size: 0.72rem; opacity: 0.7; text-transform: uppercase; letter-spacing: 0.03em; }

.dns-summary-actions {
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

/* States */
.dns-loading, .dns-error, .dns-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 3rem 1rem;
  gap: 0.75rem;
  color: var(--muted, #6b7280);
}
.dns-error { color: var(--danger, #ef4444); }
.dns-empty i { font-size: 2.5rem; opacity: 0.3; }
.dns-empty h4 { margin: 0; font-weight: 600; }
.dns-empty p { margin: 0; opacity: 0.6; font-size: 0.9rem; }
.retry-btn { padding: 0.3rem 0.75rem; border-radius: 6px; border: 1px solid currentColor; background: transparent; cursor: pointer; color: inherit; }
.spinner { width: 24px; height: 24px; border: 2px solid var(--border-color, #d1d5db); border-top-color: var(--primary, #3b82f6); border-radius: 50%; animation: spin 0.6s linear infinite; }

/* Groups */
.dns-groups { display: flex; flex-direction: column; gap: 0.75rem; }

.host-status {
  display: flex;
  align-items: center;
  font-size: 1rem;
}
.host-status.healthy { color: var(--success, #10b981); }
.host-status.warning { color: var(--warning, #f59e0b); }
.host-status.critical { color: var(--danger, #ef4444); }
.host-status.unknown { color: var(--muted, #6b7280); }

/* Server Comparison Table */
.server-table-wrap {
  overflow-x: auto;
}

.server-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.82rem;
}

.server-table thead th {
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

.server-table tbody td {
  padding: 0.6rem 0.75rem;
  border-bottom: 1px solid var(--border-color, #f1f5f9);
  vertical-align: middle;
}

.server-row:hover td {
  background: var(--bg-subtle, #f8f9fa);
}

.server-cell {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  white-space: nowrap;
}

.server-cell i {
  opacity: 0.4;
  font-size: 0.75rem;
}

.mono {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 0.78rem;
}

.badge-type {
  display: inline-block;
  padding: 0.1rem 0.4rem;
  background: var(--bg-subtle, #f1f5f9);
  border-radius: 3px;
  font-size: 0.7rem;
  font-weight: 600;
  letter-spacing: 0.03em;
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
.status-badge.sm { font-size: 0.7rem; padding: 0.1rem 0.4rem; }

.dns-status-ok { color: var(--success, #10b981); background: rgba(16, 185, 129, 0.1); }
.dns-status-warn { color: var(--warning, #f59e0b); background: rgba(245, 158, 11, 0.1); }
.dns-status-error { color: var(--danger, #ef4444); background: rgba(239, 68, 68, 0.1); }

.latency-good { color: var(--success, #10b981); }
.latency-warn { color: var(--warning, #f59e0b); }
.latency-bad { color: var(--danger, #ef4444); }

.answer-cell {
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.time-cell { white-space: nowrap; opacity: 0.7; font-size: 0.78rem; }

.action-cell {
  white-space: nowrap;
  display: flex;
  gap: 0.25rem;
  align-items: center;
}

.history-btn, .raw-toggle-btn {
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
.history-btn:hover, .raw-toggle-btn:hover {
  background: var(--bg-subtle, #f1f5f9);
  color: var(--text, #111);
}
.raw-toggle-btn.sm { padding: 0.1rem 0.3rem; }

/* Raw response inline */
.raw-cell {
  padding: 0 !important;
}
.raw-cell pre {
  margin: 0;
  padding: 0.75rem 1rem;
  font-size: 0.68rem;
  line-height: 1.4;
  background: var(--bg-code, #1e293b);
  color: var(--text-code, #e2e8f0);
  overflow-x: auto;
  max-height: 250px;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
}

/* History Panel */
.history-cell {
  padding: 0 !important;
}

.history-panel {
  background: var(--bg-subtle, #f8f9fa);
  border-top: 2px solid var(--border-color, #e5e7eb);
}

.history-title {
  padding: 0.5rem 0.75rem;
  font-size: 0.72rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.6;
  display: flex;
  align-items: center;
  gap: 0.4rem;
  border-bottom: 1px solid var(--border-color, #e5e7eb);
}

.history-count {
  font-weight: 400;
  opacity: 0.6;
}

.history-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.78rem;
}

.history-table thead th {
  padding: 0.35rem 0.75rem;
  text-align: left;
  font-size: 0.65rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.4;
  font-weight: 600;
}

.history-table tbody td {
  padding: 0.35rem 0.75rem;
  border-bottom: 1px solid var(--border-color, #eef0f2);
  vertical-align: middle;
}

.history-table tbody tr:hover td {
  background: rgba(0, 0, 0, 0.02);
}

/* Inline raw response row styles */
.raw-inline-row {
  background: transparent;
}

.raw-inline-row td {
  padding: 0 !important;
}

.raw-inline-row .raw-header {
  padding: 0.3rem 0.75rem;
  font-size: 0.65rem;
  opacity: 0.5;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  border-top: 1px solid var(--border-color, #e5e7eb);
  background: var(--bg-subtle, #f8f9fa);
}

.raw-inline-row pre {
  margin: 0;
  padding: 0.5rem 0.75rem;
  font-size: 0.65rem;
  line-height: 1.3;
  background: var(--bg-code, #1e293b);
  color: var(--text-code, #e2e8f0);
  overflow-x: auto;
  max-height: 200px;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
}

/* Responsive */
@media (max-width: 768px) {
  .dns-summary {
    flex-direction: column;
    align-items: stretch;
  }
  .dns-summary-actions {
    margin-left: 0;
    justify-content: flex-end;
  }
  .answer-cell { max-width: 140px; }
}

/* ==================== Dark Mode ==================== */
[data-theme="dark"] .dns-summary {
  background: #1a1f2e;
  border-color: #2a3042;
}

[data-theme="dark"] .dns-summary-stat {
  background: #232838;
}

[data-theme="dark"] .lookback-select,
[data-theme="dark"] .btn-refresh {
  background: #232838;
  border-color: #2a3042;
  color: #c8cdd8;
}

[data-theme="dark"] .lookback-select:hover,
[data-theme="dark"] .btn-refresh:hover {
  background: #2a3042;
}

[data-theme="dark"] .server-table thead th {
  border-color: #2a3042;
  color: #8890a0;
}

[data-theme="dark"] .server-table tbody td {
  border-color: #1e2333;
}

[data-theme="dark"] .server-row:hover td {
  background: #1e2333;
}

[data-theme="dark"] .badge-type {
  background: #232838;
  color: #c8cdd8;
}

[data-theme="dark"] .history-btn,
[data-theme="dark"] .raw-toggle-btn {
  border-color: #2a3042;
  color: #8890a0;
}

[data-theme="dark"] .history-btn:hover,
[data-theme="dark"] .raw-toggle-btn:hover {
  background: #232838;
  color: #e0e4ec;
}

[data-theme="dark"] .history-panel {
  background: #161a26;
  border-color: #2a3042;
}

[data-theme="dark"] .history-title {
  border-color: #2a3042;
  color: #8890a0;
}

[data-theme="dark"] .history-table thead th {
  color: #6b7280;
}

[data-theme="dark"] .history-table tbody td {
  border-color: #1e2333;
}

[data-theme="dark"] .history-table tbody tr:hover td {
  background: rgba(255, 255, 255, 0.03);
}

[data-theme="dark"] .raw-cell pre,
[data-theme="dark"] .raw-inline-row pre {
  background: #0f1219;
  color: #a5b4c8;
}

[data-theme="dark"] .raw-inline-row .raw-header {
  border-color: #2a3042;
  color: #6b7280;
  background: #161a26;
}

[data-theme="dark"] .dns-status-ok {
  color: #34d399;
  background: rgba(52, 211, 153, 0.12);
}

[data-theme="dark"] .dns-status-warn {
  color: #fbbf24;
  background: rgba(251, 191, 36, 0.12);
}

[data-theme="dark"] .dns-status-error {
  color: #f87171;
  background: rgba(248, 113, 113, 0.12);
}

[data-theme="dark"] .dns-loading,
[data-theme="dark"] .dns-empty {
  color: #6b7280;
}

[data-theme="dark"] .dns-empty i {
  opacity: 0.2;
}

[data-theme="dark"] .spinner {
  border-color: #2a3042;
  border-top-color: #60a5fa;
}

[data-theme="dark"] .retry-btn {
  color: #f87171;
  border-color: #f87171;
}

[data-theme="dark"] .host-status.healthy { color: #34d399; }
[data-theme="dark"] .host-status.warning { color: #fbbf24; }
[data-theme="dark"] .host-status.critical { color: #f87171; }
[data-theme="dark"] .host-status.unknown { color: #6b7280; }

[data-theme="dark"] .latency-good { color: #34d399; }
[data-theme="dark"] .latency-warn { color: #fbbf24; }
[data-theme="dark"] .latency-bad { color: #f87171; }

[data-theme="dark"] .time-cell { color: #6b7280; }
[data-theme="dark"] .mono { color: #c8cdd8; }
</style>

