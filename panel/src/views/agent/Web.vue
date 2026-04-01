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
const lookback = ref(60)
const selectedCert = ref<{ subject: string; issuer: string; not_before: string; not_after: string; issuer_org?: string; cert_type?: string; fingerprint?: string; signature_algorithm?: string; public_key_algorithm?: string; serial_number?: string } | null>(null)
const showCertModal = ref(false)
const selectedHTTP = ref<WebGroupEntry | null>(null)
const showHTTPModal = ref(false)
const selectedTLSDetails = ref<WebGroupEntry | null>(null)
const showTLSDetailsModal = ref(false)

function isHTTP(entry: WebGroupEntry): boolean {
  return !!entry.payload.status_code || !!entry.payload.url
}

function isEntryFailing(entry: WebGroupEntry): boolean {
  const r = entry.payload
  if (r?.error) return true
  if (isHTTP(entry)) {
    const code = r?.status_code || 0
    if (code >= 400) return true
  } else {
    if (r?.is_expired || r?.is_expiring_soon) return true
  }
  return false
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

const httpPassingGroups = computed(() => targetGroups.value.filter(g => g.type === 'HTTP' && !isEntryFailing(g.latest)))
const httpFailingGroups = computed(() => targetGroups.value.filter(g => g.type === 'HTTP' && isEntryFailing(g.latest)))
const tlsGroups = computed(() => targetGroups.value.filter(g => g.type === 'TLS' || g.type === 'mixed'))

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

function formatLatency(ms: number | undefined): string {
  if (ms === undefined || ms === null) return '—'
  if (ms < 1) return `${(ms * 1000).toFixed(0)}µs`
  if (ms < 100) return `${ms.toFixed(2)}ms`
  return `${ms.toFixed(0)}ms`
}

function formatBodySize(bytes: number | undefined): string {
  if (bytes === undefined || bytes === null) return '—'
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  const value = bytes / Math.pow(1024, i)
  if (i === 0) return `${bytes} B`
  return `${value.toFixed(1)} ${units[i]}`
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

function viewCertificate(entry: WebGroupEntry) {
  const cert = getCertInfo(entry)
  if (cert) {
    selectedCert.value = {
      subject: cert.subject || 'Unknown',
      issuer: cert.issuer || 'Unknown',
      not_before: cert.not_before || 'Unknown',
      not_after: cert.not_after || 'Unknown',
      issuer_org: entry.payload?.certificate?.issuer_org,
      cert_type: entry.payload?.certificate?.cert_type,
      fingerprint: entry.payload?.certificate?.fingerprint,
      signature_algorithm: entry.payload?.certificate?.signature_algorithm,
      public_key_algorithm: entry.payload?.certificate?.public_key_algorithm,
      serial_number: entry.payload?.certificate?.serial_number
    }
    showCertModal.value = true
  }
}

function viewHTTPDetails(entry: WebGroupEntry) {
  selectedHTTP.value = entry
  showHTTPModal.value = true
}

function viewTLSDetails(entry: WebGroupEntry) {
  selectedTLSDetails.value = entry
  showTLSDetailsModal.value = true
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

    <div v-else class="web-sections">
      <!-- HTTP Passing Section -->
      <div v-if="httpPassingGroups.length" class="web-section">
        <div class="section-header">
          <div class="section-title">
            <i class="bi bi-check-circle-fill text-success"></i>
            <span>HTTP <span class="text-muted">/ Passing</span></span>
          </div>
          <span class="section-count">{{ httpPassingGroups.length }}</span>
        </div>
        <div class="web-groups">
          <Element
            v-for="group in httpPassingGroups"
            :key="group.target"
            :title="group.target"
            icon="bi bi-globe"
            :subtitle="`${group.totalChecks} checks`"
          >
            <template #secondary>
              <div class="target-status healthy">
                <i class="bi bi-check-circle-fill"></i>
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
                  <template v-for="(entry, idx) in group.entries.filter(e => isHTTP(e))" :key="`${group.target}-${idx}`">
                    <tr class="web-row">
                      <td class="mono url-cell">
                        <span class="url-text" :title="entry.payload?.url">{{ entry.payload?.url }}</span>
                        <span class="probe-type-badge http">HTTP</span>
                      </td>
                      <td>
                        <span class="status-badge" :class="getStatusColor(entry)">
                          <i class="bi" :class="getStatusIcon(entry)"></i>
                          {{ getStatusText(entry) }}
                        </span>
                      </td>
                      <td>
                        <span class="mono" :class="latencyClass(entry.payload?.total_ms)">
                          {{ formatLatency(entry.payload?.total_ms) }}
                        </span>
                      </td>
                      <td>
                        <span class="badge-tls">{{ entry.payload?.tls_version || '—' }}</span>
                      </td>
                      <td>
                        <span v-if="getCertInfo(entry)" class="cert-badge cert-clickable" :class="certExpiryClass(getCertInfo(entry)?.days_until_expiry)" @click="viewCertificate(entry)">
                          {{ getCertInfo(entry)?.days_until_expiry }}d <i class="bi bi-search"></i>
                        </span>
                        <span v-else class="muted">—</span>
                      </td>
                      <td class="time-cell">
                        {{ since(entry.created_at, true) }}
                      </td>
                      <td class="action-cell">
                        <button
                          @click="viewHTTPDetails(entry)"
                          class="details-btn"
                          title="View HTTP details"
                        >
                          <i class="bi bi-rulers"></i>
                        </button>
                      </td>
                    </tr>
                  </template>
                </tbody>
              </table>
            </div>
          </Element>
        </div>
      </div>

      <!-- HTTP Failing Section -->
      <div v-if="httpFailingGroups.length" class="web-section">
        <div class="section-header failing">
          <div class="section-title">
            <i class="bi bi-x-circle-fill text-danger"></i>
            <span>HTTP <span class="text-muted">/ Failing</span></span>
          </div>
          <span class="section-count">{{ httpFailingGroups.length }}</span>
        </div>
        <div class="web-groups">
          <Element
            v-for="group in httpFailingGroups"
            :key="group.target"
            :title="group.target"
            icon="bi bi-globe"
            :subtitle="`${group.totalChecks} checks`"
          >
            <template #secondary>
              <div class="target-status critical">
                <i class="bi bi-x-circle-fill"></i>
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
                  <template v-for="(entry, idx) in group.entries.filter(e => isHTTP(e))" :key="`${group.target}-${idx}`">
                    <tr class="web-row failing">
                      <td class="mono url-cell">
                        <span class="url-text" :title="entry.payload?.url">{{ entry.payload?.url }}</span>
                        <span class="probe-type-badge http">HTTP</span>
                      </td>
                      <td>
                        <span class="status-badge" :class="getStatusColor(entry)">
                          <i class="bi" :class="getStatusIcon(entry)"></i>
                          {{ getStatusText(entry) }}
                        </span>
                      </td>
                      <td>
                        <span class="mono" :class="latencyClass(entry.payload?.total_ms)">
                          {{ formatLatency(entry.payload?.total_ms) }}
                        </span>
                      </td>
                      <td>
                        <span class="badge-tls">{{ entry.payload?.tls_version || '—' }}</span>
                      </td>
                      <td>
                        <span v-if="getCertInfo(entry)" class="cert-badge cert-clickable" :class="certExpiryClass(getCertInfo(entry)?.days_until_expiry)" @click="viewCertificate(entry)">
                          {{ getCertInfo(entry)?.days_until_expiry }}d <i class="bi bi-search"></i>
                        </span>
                        <span v-else class="muted">—</span>
                      </td>
                      <td class="time-cell">
                        {{ since(entry.created_at, true) }}
                      </td>
                      <td class="action-cell">
                        <button
                          @click="viewHTTPDetails(entry)"
                          class="details-btn"
                          title="View HTTP details"
                        >
                          <i class="bi bi-rulers"></i>
                        </button>
                      </td>
                    </tr>
                  </template>
                </tbody>
              </table>
            </div>
          </Element>
        </div>
      </div>

      <!-- TLS Section -->
      <div v-if="tlsGroups.length" class="web-section">
        <div class="section-header tls">
          <div class="section-title">
            <i class="bi bi-shield-lock-fill text-purple"></i>
            <span>TLS <span class="text-muted">/ Certificates</span></span>
          </div>
          <span class="section-count">{{ tlsGroups.length }}</span>
        </div>
        <div class="web-groups">
          <Element
            v-for="group in tlsGroups"
            :key="group.target"
            :title="group.target"
            icon="bi bi-shield-lock"
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
                    <th>Host</th>
                    <th>Status</th>
                    <th>Latency</th>
                    <th>Protocol</th>
                    <th>Cert Expiry</th>
                    <th>Last Check</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  <template v-for="(entry, idx) in group.entries.filter(e => !isHTTP(e))" :key="`${group.target}-${idx}`">
                    <tr class="web-row">
                      <td class="mono url-cell">
                        <span class="url-text" :title="entry.payload?.remote_addr || entry.target">{{ entry.payload?.remote_addr || entry.target }}</span>
                        <span class="probe-type-badge tls">TLS</span>
                      </td>
                      <td>
                        <span class="status-badge" :class="getStatusColor(entry)">
                          <i class="bi" :class="getStatusIcon(entry)"></i>
                          {{ getStatusText(entry) }}
                        </span>
                      </td>
                      <td>
                        <span class="mono" :class="latencyClass(entry.payload?.total_ms)">
                          {{ formatLatency(entry.payload?.total_ms) }}
                        </span>
                      </td>
                      <td>
                        <span class="badge-tls">{{ entry.payload?.tls_version || '—' }}</span>
                      </td>
                      <td>
                        <span v-if="getCertInfo(entry)" class="cert-badge cert-clickable" :class="certExpiryClass(getCertInfo(entry)?.days_until_expiry)" @click="viewCertificate(entry)">
                          {{ getCertInfo(entry)?.days_until_expiry }}d <i class="bi bi-search"></i>
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
                          @click="viewTLSDetails(entry)"
                          class="details-btn"
                          title="View TLS details"
                        >
                          <i class="bi bi-shield-check"></i>
                        </button>
                      </td>
                    </tr>
                  </template>
                </tbody>
              </table>
            </div>
          </Element>
        </div>
      </div>
    </div>

    <!-- Certificate Modal -->
    <div v-if="showCertModal && selectedCert" class="cert-modal-overlay" @click.self="showCertModal = false">
      <div class="cert-modal">
        <div class="cert-modal-header">
          <div class="cert-modal-title">
            <i class="bi bi-certificate"></i>
            Certificate Details
          </div>
          <button class="cert-modal-close" @click="showCertModal = false">
            <i class="bi bi-x"></i>
          </button>
        </div>
        <div class="cert-modal-body">
          <div class="cert-modal-grid">
            <div class="cert-modal-item">
              <span class="cert-modal-label">Subject</span>
              <span class="cert-modal-value">{{ selectedCert.subject }}</span>
            </div>
            <div class="cert-modal-item">
              <span class="cert-modal-label">Issuer</span>
              <span class="cert-modal-value">{{ selectedCert.issuer }}</span>
            </div>
            <div class="cert-modal-item">
              <span class="cert-modal-label">Valid From</span>
              <span class="cert-modal-value">{{ selectedCert.not_before }}</span>
            </div>
            <div class="cert-modal-item">
              <span class="cert-modal-label">Valid Until</span>
              <span class="cert-modal-value">{{ selectedCert.not_after }}</span>
            </div>
            <div v-if="selectedCert.issuer_org" class="cert-modal-item">
              <span class="cert-modal-label">Issuer Organization</span>
              <span class="cert-modal-value">{{ selectedCert.issuer_org }}</span>
            </div>
            <div v-if="selectedCert.cert_type" class="cert-modal-item">
              <span class="cert-modal-label">Certificate Type</span>
              <span class="cert-modal-value">{{ selectedCert.cert_type }}</span>
            </div>
            <div v-if="selectedCert.fingerprint" class="cert-modal-item full-width">
              <span class="cert-modal-label">Fingerprint</span>
              <span class="cert-modal-value mono fingerprint">{{ selectedCert.fingerprint }}</span>
            </div>
            <div v-if="selectedCert.signature_algorithm" class="cert-modal-item">
              <span class="cert-modal-label">Signature Algorithm</span>
              <span class="cert-modal-value">{{ selectedCert.signature_algorithm }}</span>
            </div>
            <div v-if="selectedCert.public_key_algorithm" class="cert-modal-item">
              <span class="cert-modal-label">Public Key Algorithm</span>
              <span class="cert-modal-value">{{ selectedCert.public_key_algorithm }}</span>
            </div>
            <div v-if="selectedCert.serial_number" class="cert-modal-item full-width">
              <span class="cert-modal-label">Serial Number</span>
              <span class="cert-modal-value mono">{{ selectedCert.serial_number }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
    <!-- HTTP Details Modal -->
    <div v-if="showHTTPModal && selectedHTTP" class="http-modal-overlay" @click.self="showHTTPModal = false">
      <div class="http-modal">
        <div class="http-modal-header">
          <div class="http-modal-title">
            <i class="bi bi-rulers"></i>
            HTTP Details
          </div>
          <button class="http-modal-close" @click="showHTTPModal = false">
            <i class="bi bi-x"></i>
          </button>
        </div>
        <div class="http-modal-body">
          <div class="http-modal-grid">
            <div class="http-modal-item full-width">
              <span class="http-modal-label">URL</span>
              <span class="http-modal-value mono">{{ selectedHTTP.payload?.url || '—' }}</span>
            </div>
            <div class="http-modal-item">
              <span class="http-modal-label">Status</span>
              <span class="http-modal-value">
                <span class="status-badge" :class="getStatusColor(selectedHTTP)">
                  <i class="bi" :class="getStatusIcon(selectedHTTP)"></i>
                  {{ getStatusText(selectedHTTP) }}
                </span>
              </span>
            </div>
            <div class="http-modal-item">
              <span class="http-modal-label">Latency</span>
              <span class="http-modal-value mono" :class="latencyClass(selectedHTTP.payload?.total_ms)">
                {{ formatLatency(selectedHTTP.payload?.total_ms) }}
              </span>
            </div>
            <div class="http-modal-item">
              <span class="http-modal-label">TTFB</span>
              <span class="http-modal-value mono" :class="latencyClass(selectedHTTP.payload?.first_byte_ms)">
                {{ formatLatency(selectedHTTP.payload?.first_byte_ms) }}
              </span>
            </div>
            <div class="http-modal-item">
              <span class="http-modal-label">DNS Lookup</span>
              <span class="http-modal-value mono">{{ formatLatency(selectedHTTP.payload?.dns_lookup_ms) }}</span>
            </div>
            <div class="http-modal-item">
              <span class="http-modal-label">TCP Connect</span>
              <span class="http-modal-value mono">{{ formatLatency(selectedHTTP.payload?.tcp_connect_ms) }}</span>
            </div>
            <div class="http-modal-item">
              <span class="http-modal-label">TLS Handshake</span>
              <span class="http-modal-value mono">{{ formatLatency(selectedHTTP.payload?.tls_handshake_ms) }}</span>
            </div>
            <div class="http-modal-item">
              <span class="http-modal-label">Body Size</span>
              <span class="http-modal-value mono">{{ formatBodySize(selectedHTTP.payload?.body_size) }}</span>
            </div>
            <div class="http-modal-item">
              <span class="http-modal-label">Content-Type</span>
              <span class="http-modal-value">{{ selectedHTTP.payload?.content_type || '—' }}</span>
            </div>
            <div class="http-modal-item">
              <span class="http-modal-label">RemoteAddr</span>
              <span class="http-modal-value mono">{{ selectedHTTP.payload?.remote_addr || '—' }}</span>
            </div>
            <div class="http-modal-item">
              <span class="http-modal-label">TLS Version</span>
              <span class="http-modal-value">{{ selectedHTTP.payload?.tls_version || '—' }}</span>
            </div>
            <div class="http-modal-item">
              <span class="http-modal-label">Cipher Suite</span>
              <span class="http-modal-value mono">{{ selectedHTTP.payload?.tls_cipher_suite || '—' }}</span>
            </div>
            <div v-if="selectedHTTP.payload?.error" class="http-modal-item full-width">
              <span class="http-modal-label">Error</span>
              <span class="http-modal-value error">{{ selectedHTTP.payload.error }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
    <!-- TLS Details Modal -->
    <div v-if="showTLSDetailsModal && selectedTLSDetails" class="tls-modal-overlay" @click.self="showTLSDetailsModal = false">
      <div class="tls-modal">
        <div class="tls-modal-header">
          <div class="tls-modal-title">
            <i class="bi bi-shield-check"></i>
            TLS Details
          </div>
          <button class="tls-modal-close" @click="showTLSDetailsModal = false">
            <i class="bi bi-x"></i>
          </button>
        </div>
        <div class="tls-modal-body">
          <div class="tls-modal-grid">
            <div class="tls-modal-item">
              <span class="tls-modal-label">Connect</span>
              <span class="tls-modal-value mono">{{ formatLatency(selectedTLSDetails.payload?.total_ms) }}</span>
            </div>
            <div class="tls-modal-item">
              <span class="tls-modal-label">Protocol</span>
              <span class="tls-modal-value">{{ selectedTLSDetails.payload?.protocol || '—' }}</span>
            </div>
            <div class="tls-modal-item">
              <span class="tls-modal-label">Cipher Suite</span>
              <span class="tls-modal-value mono">{{ selectedTLSDetails.payload?.tls_cipher_suite || '—' }}</span>
            </div>
            <div class="tls-modal-item">
              <span class="tls-modal-label">RemoteAddr</span>
              <span class="tls-modal-value mono">{{ selectedTLSDetails.payload?.remote_addr || '—' }}</span>
            </div>
          </div>
        </div>
      </div>
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
  grid-template-columns: repeat(2, 1fr);
  gap: 0.75rem;
}
@media (max-width: 600px) {
  .detail-grid {
    grid-template-columns: 1fr;
  }
}

.detail-item {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  padding: 0.5rem 0.75rem;
  background: var(--bg-elevated, rgba(255, 255, 255, 0.03));
  border-radius: 6px;
  border: 1px solid var(--border-color, rgba(255, 255, 255, 0.05));
  min-width: 0;
}

.detail-item.wide {
  grid-column: span 2;
}

@media (max-width: 600px) {
  .detail-item.wide {
    grid-column: span 1;
  }
}

.detail-label {
  font-size: 0.65rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.6;
  font-weight: 600;
  color: var(--muted);
}

.detail-value {
  font-size: 0.9rem;
  color: var(--text);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-value.truncate {
  display: block;
  max-width: 100%;
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

/* ==================== Dark Mode ==================== */
[data-theme="dark"] .web-summary {
  background: #1a1f2e;
  border-color: #2a3042;
}

[data-theme="dark"] .web-summary-stat {
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

[data-theme="dark"] .web-table thead th {
  border-color: #2a3042;
  color: #8890a0;
}

[data-theme="dark"] .web-table tbody td {
  border-color: #1e2333;
}

[data-theme="dark"] .web-row:hover td {
  background: #1e2333;
}

[data-theme="dark"] .probe-type-badge.http {
  background: rgba(59, 130, 246, 0.15);
  color: #60a5fa;
}

[data-theme="dark"] .probe-type-badge.tls {
  background: rgba(168, 85, 247, 0.15);
  color: #c084fc;
}

[data-theme="dark"] .badge-tls {
  background: #232838;
  color: #c8cdd8;
}

.details-btn, .expand-btn {
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
.details-btn:hover, .expand-btn:hover {
  background: var(--bg-subtle, #f1f5f9);
  color: var(--text, #111);
}

.expand-btn {
  border-color: #2a3042;
  color: #8890a0;
}

[data-theme="dark"] .expand-btn:hover {
  background: #232838;
  color: #e0e4ec;
}

[data-theme="dark"] .detail-panel {
  background: #161a26;
  border-color: #2a3042;
}

[data-theme="dark"] .detail-label {
  color: #6b7280;
}

[data-theme="dark"] .detail-value {
  color: #c8cdd8;
}

[data-theme="dark"] .detail-item {
  background: rgba(35, 40, 56, 0.5);
  border-color: #2a3042;
}

[data-theme="dark"] .cert-info {
  border-color: #2a3042;
}

[data-theme="dark"] .cert-title {
  color: #6b7280;
}

[data-theme="dark"] .cert-details {
  color: #c8cdd8;
}

[data-theme="dark"] .web-loading,
[data-theme="dark"] .web-empty {
  color: #6b7280;
}

[data-theme="dark"] .web-empty i {
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

[data-theme="dark"] .target-status.healthy { color: #34d399; }
[data-theme="dark"] .target-status.warning { color: #fbbf24; }
[data-theme="dark"] .target-status.critical { color: #f87171; }
[data-theme="dark"] .target-status.unknown { color: #6b7280; }

[data-theme="dark"] .latency-good { color: #34d399; }
[data-theme="dark"] .latency-warn { color: #fbbf24; }
[data-theme="dark"] .latency-bad { color: #f87171; }

[data-theme="dark"] .time-cell { color: #6b7280; }
[data-theme="dark"] .mono { color: #c8cdd8; }

[data-theme="dark"] .web-status-ok {
  color: #34d399;
  background: rgba(52, 211, 153, 0.12);
}

[data-theme="dark"] .web-status-warn {
  color: #fbbf24;
  background: rgba(251, 191, 36, 0.12);
}

[data-theme="dark"] .web-status-error {
  color: #f87171;
  background: rgba(248, 113, 113, 0.12);
}

[data-theme="dark"] .cert-ok {
  color: #34d399;
  background: rgba(52, 211, 153, 0.12);
}

[data-theme="dark"] .cert-warn {
  color: #fbbf24;
  background: rgba(251, 191, 36, 0.12);
}

[data-theme="dark"] .cert-expired {
  color: #f87171;
  background: rgba(248, 113, 113, 0.12);
}

[data-theme="dark"] .muted {
  color: #6b7280;
}

/* Web Sections */
.web-sections {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.web-section {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.6rem 1rem;
  background: var(--card-bg, #fff);
  border: 1px solid var(--border-color, #e0e0e0);
  border-radius: 8px;
}

.section-header.failing {
  border-left: 3px solid var(--danger, #ef4444);
}

.section-header.tls {
  border-left: 3px solid #a855f7;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 600;
  font-size: 0.9rem;
}

.section-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 24px;
  height: 24px;
  padding: 0 0.5rem;
  background: var(--bg-subtle, #f8f9fa);
  border-radius: 12px;
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--muted, #6b7280);
}

.text-success { color: var(--success, #10b981); }
.text-danger { color: var(--danger, #ef4444); }
.text-purple { color: #a855f7; }
.text-muted { color: var(--muted, #6b7280); }

.web-row.failing td {
  background: rgba(239, 68, 68, 0.04);
}

/* Certificate Clickable */
.cert-clickable {
  cursor: pointer;
  transition: all 0.15s;
}

.cert-clickable:hover {
  opacity: 0.8;
}

.cert-view-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.4rem 0.75rem;
  background: var(--bg-subtle, #f8f9fa);
  border: 1px solid var(--border-color, #d1d5db);
  border-radius: 6px;
  font-size: 0.78rem;
  font-weight: 500;
  color: var(--text, #374151);
  cursor: pointer;
  transition: all 0.15s;
}

.cert-view-btn:hover {
  background: var(--border-color, #e5e7eb);
}

/* Certificate Modal */
.cert-modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 1rem;
}

.cert-modal {
  background: var(--card-bg, #fff);
  border-radius: 12px;
  width: 100%;
  max-width: 560px;
  max-height: 80vh;
  overflow: hidden;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.2);
}

.cert-modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem 1.25rem;
  border-bottom: 1px solid var(--border-color, #e0e0e0);
  background: var(--bg-subtle, #f8f9fa);
}

.cert-modal-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 600;
  font-size: 1rem;
  color: var(--text, #1f2937);
}

.cert-modal-title i {
  color: #a855f7;
  font-size: 1.25rem;
}

.cert-modal-close {
  background: none;
  border: none;
  padding: 0.25rem;
  cursor: pointer;
  color: var(--muted, #6b7280);
  font-size: 1.25rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  transition: all 0.15s;
}

.cert-modal-close:hover {
  background: var(--bg-subtle, #f8f9fa);
  color: var(--text, #1f2937);
}

.cert-modal-body {
  padding: 1.25rem;
  overflow-y: auto;
  max-height: calc(80vh - 70px);
}

.cert-modal-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 1rem;
}

.cert-modal-item {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.cert-modal-item.full-width {
  grid-column: span 2;
}

.cert-modal-label {
  font-size: 0.68rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-weight: 600;
  color: var(--muted, #6b7280);
}

.cert-modal-value {
  font-size: 0.85rem;
  color: var(--text, #1f2937);
  word-break: break-word;
}

.cert-modal-value.mono {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 0.78rem;
}

.cert-modal-value.fingerprint {
  font-size: 0.72rem;
  word-break: break-all;
}

/* Dark mode for new elements */
[data-theme="dark"] .section-header {
  background: #1a1f2e;
  border-color: #2a3042;
}

[data-theme="dark"] .section-header.failing {
  border-left-color: #f87171;
}

[data-theme="dark"] .section-header.tls {
  border-left-color: #c084fc;
}

[data-theme="dark"] .section-count {
  background: #232838;
  color: #8890a0;
}

[data-theme="dark"] .cert-view-btn {
  background: #232838;
  border-color: #2a3042;
  color: #c8cdd8;
}

[data-theme="dark"] .cert-view-btn:hover {
  background: #2a3042;
}

[data-theme="dark"] .cert-modal {
  background: #1a1f2e;
}

[data-theme="dark"] .cert-modal-header {
  background: #232838;
  border-color: #2a3042;
}

[data-theme="dark"] .cert-modal-title {
  color: #e0e4ec;
}

[data-theme="dark"] .cert-modal-close {
  color: #8890a0;
}

[data-theme="dark"] .cert-modal-close:hover {
  background: #2a3042;
  color: #e0e4ec;
}

[data-theme="dark"] .cert-modal-body {
  background: #1a1f2e;
}

[data-theme="dark"] .cert-modal-label {
  color: #6b7280;
}

[data-theme="dark"] .cert-modal-value {
  color: #c8cdd8;
}

[data-theme="dark"] .web-row.failing td {
  background: rgba(248, 113, 113, 0.08);
}

[data-theme="dark"] .cert-clickable:hover {
  opacity: 0.7;
}

/* TLS Details Modal */
.tls-modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 1rem;
}

.tls-modal {
  background: var(--card-bg, #fff);
  border-radius: 12px;
  width: 100%;
  max-width: 480px;
  max-height: 80vh;
  overflow: hidden;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.2);
}

.tls-modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem 1.25rem;
  border-bottom: 1px solid var(--border-color, #e0e0e0);
  background: var(--bg-subtle, #f8f9fa);
}

.tls-modal-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 600;
  font-size: 1rem;
  color: var(--text, #1f2937);
}

.tls-modal-title i {
  color: #a855f7;
  font-size: 1.25rem;
}

.tls-modal-close {
  background: none;
  border: none;
  padding: 0.25rem;
  cursor: pointer;
  color: var(--muted, #6b7280);
  font-size: 1.25rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  transition: all 0.15s;
}

.tls-modal-close:hover {
  background: var(--bg-subtle, #f8f9fa);
  color: var(--text, #1f2937);
}

.tls-modal-body {
  padding: 1.25rem;
  overflow-y: auto;
  max-height: calc(80vh - 70px);
}

.tls-modal-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 1rem;
}

.tls-modal-item {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.tls-modal-label {
  font-size: 0.68rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-weight: 600;
  color: var(--muted, #6b7280);
}

.tls-modal-value {
  font-size: 0.9rem;
  color: var(--text, #1f2937);
  word-break: break-word;
}

.tls-modal-value.mono {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 0.85rem;
}

/* Dark mode for TLS modal */
[data-theme="dark"] .tls-modal {
  background: #1a1f2e;
}

[data-theme="dark"] .tls-modal-header {
  background: #232838;
  border-color: #2a3042;
}

[data-theme="dark"] .tls-modal-title {
  color: #e0e4ec;
}

[data-theme="dark"] .tls-modal-close {
  color: #8890a0;
}

[data-theme="dark"] .tls-modal-close:hover {
  background: #2a3042;
  color: #e0e4ec;
}

[data-theme="dark"] .tls-modal-body {
  background: #1a1f2e;
}

[data-theme="dark"] .tls-modal-label {
  color: #6b7280;
}

[data-theme="dark"] .tls-modal-value {
  color: #c8cdd8;
}

/* HTTP Details Modal */
.http-modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 1rem;
}

.http-modal {
  background: var(--card-bg, #fff);
  border-radius: 12px;
  width: 100%;
  max-width: 520px;
  max-height: 80vh;
  overflow: hidden;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.2);
}

.http-modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem 1.25rem;
  border-bottom: 1px solid var(--border-color, #e0e0e0);
  background: var(--bg-subtle, #f8f9fa);
}

.http-modal-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 600;
  font-size: 1rem;
  color: var(--text, #1f2937);
}

.http-modal-title i {
  color: #3b82f6;
  font-size: 1.25rem;
}

.http-modal-close {
  background: none;
  border: none;
  padding: 0.25rem;
  cursor: pointer;
  color: var(--muted, #6b7280);
  font-size: 1.25rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  transition: all 0.15s;
}

.http-modal-close:hover {
  background: var(--bg-subtle, #f8f9fa);
  color: var(--text, #1f2937);
}

.http-modal-body {
  padding: 1.25rem;
  overflow-y: auto;
  max-height: calc(80vh - 70px);
}

.http-modal-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 1rem;
}

.http-modal-item {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.http-modal-item.full-width {
  grid-column: span 2;
}

.http-modal-label {
  font-size: 0.68rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-weight: 600;
  color: var(--muted, #6b7280);
}

.http-modal-value {
  font-size: 0.9rem;
  color: var(--text, #1f2937);
  word-break: break-word;
}

.http-modal-value.mono {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 0.85rem;
}

.http-modal-value.error {
  color: var(--danger, #ef4444);
}

/* Dark mode for HTTP modal */
[data-theme="dark"] .http-modal {
  background: #1a1f2e;
}

[data-theme="dark"] .http-modal-header {
  background: #232838;
  border-color: #2a3042;
}

[data-theme="dark"] .http-modal-title {
  color: #e0e4ec;
}

[data-theme="dark"] .http-modal-close {
  color: #8890a0;
}

[data-theme="dark"] .http-modal-close:hover {
  background: #2a3042;
  color: #e0e4ec;
}

[data-theme="dark"] .http-modal-body {
  background: #1a1f2e;
}

[data-theme="dark"] .http-modal-label {
  color: #6b7280;
}

[data-theme="dark"] .http-modal-value {
  color: #c8cdd8;
}

[data-theme="dark"] .http-modal-value.error {
  color: #f87171;
}
</style>
