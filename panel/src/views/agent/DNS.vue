<script lang="ts" setup>
import { ref, onMounted, computed } from 'vue'
import type { DNSResult, DNSGroup, DNSDashboardData } from '@/types'
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
const lookback = ref(60)

// Overall DNS health
const healthSummary = computed(() => {
  if (!dnsData.value?.groups?.length) return { total: 0, healthy: 0, errors: 0, status: 'unknown' as const }
  
  let healthy = 0
  let errors = 0
  
  for (const group of dnsData.value.groups) {
    if (group.entries.length > 0) {
      const latest = group.entries[0]
      const r = latest?.payload as DNSResult | undefined
      if (r && r.response_code === 'NOERROR' && !r.error) {
        healthy++
      } else {
        errors++
      }
    }
  }
  
  const total = healthy + errors
  const status = errors === 0 ? 'healthy' : errors > healthy ? 'critical' : 'warning'
  return { total, healthy, errors, status }
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

function toggleRaw(key: string) {
  expandedRaw.value[key] = !expandedRaw.value[key]
}

function formatLatency(ms: number): string {
  if (ms < 1) return `${(ms * 1000).toFixed(0)}µs`
  if (ms < 100) return `${ms.toFixed(2)}ms`
  return `${ms.toFixed(0)}ms`
}

async function fetchData() {
  loading.value = true
  error.value = null
  try {
    dnsData.value = await ProbeDataService.dnsDashboard(
      props.workspaceId,
      props.agentId,
      { lookback: lookback.value, limit: 100 }
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
        <i class="bi" :class="{
          'bi-check-circle-fill': healthSummary.status === 'healthy',
          'bi-exclamation-triangle-fill': healthSummary.status === 'warning',
          'bi-x-circle-fill': healthSummary.status === 'critical',
          'bi-question-circle': healthSummary.status === 'unknown'
        }"></i>
        <div class="stat-text">
          <span class="stat-value">{{ healthSummary.total }}</span>
          <span class="stat-label">DNS Checks</span>
        </div>
      </div>
      <div class="dns-summary-stat healthy">
        <i class="bi bi-check-circle"></i>
        <div class="stat-text">
          <span class="stat-value">{{ healthSummary.healthy }}</span>
          <span class="stat-label">Passing</span>
        </div>
      </div>
      <div class="dns-summary-stat" :class="healthSummary.errors > 0 ? 'critical' : 'healthy'">
        <i class="bi bi-x-circle"></i>
        <div class="stat-text">
          <span class="stat-value">{{ healthSummary.errors }}</span>
          <span class="stat-label">Failing</span>
        </div>
      </div>
      <div class="dns-summary-actions">
        <select v-model.number="lookback" @change="fetchData" class="lookback-select">
          <option :value="15">Last 15m</option>
          <option :value="60">Last 1h</option>
          <option :value="360">Last 6h</option>
          <option :value="1440">Last 24h</option>
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
    <div v-else-if="!dnsData?.groups?.length" class="dns-empty">
      <i class="bi bi-globe2"></i>
      <h4>No DNS Data</h4>
      <p>No DNS probes are configured for this agent, or no data has been collected yet.</p>
    </div>

    <!-- DNS Groups -->
    <div v-else class="dns-groups">
      <Element
        v-for="group in dnsData.groups"
        :key="group.target"
        :title="group.target"
        icon="bi bi-globe2"
        :subtitle="`${group.count} check${group.count !== 1 ? 's' : ''}`"
      >
        <div class="dns-entries">
          <div
            v-for="(entry, idx) in group.entries"
            :key="`${group.target}-${idx}`"
            class="dns-entry"
          >
            <div class="entry-header">
              <!-- Status indicator -->
              <div class="entry-status" :class="getStatusColor(entry.payload.response_code, entry.payload.error)">
                <i class="bi" :class="getStatusIcon(entry.payload.response_code, entry.payload.error)"></i>
                <span class="rcode">{{ entry.payload.error || entry.payload.response_code }}</span>
              </div>

              <!-- Key details -->
              <div class="entry-details">
                <div class="detail-item">
                  <span class="detail-label">Server</span>
                  <span class="detail-value mono">{{ entry.payload.dns_server }}</span>
                </div>
                <div class="detail-item">
                  <span class="detail-label">Type</span>
                  <span class="detail-value badge-type">{{ entry.payload.record_type }}</span>
                </div>
                <div class="detail-item">
                  <span class="detail-label">Latency</span>
                  <span class="detail-value" :class="{
                    'text-success': entry.payload.query_time_ms < 50,
                    'text-warning': entry.payload.query_time_ms >= 50 && entry.payload.query_time_ms < 200,
                    'text-danger': entry.payload.query_time_ms >= 200
                  }">{{ formatLatency(entry.payload.query_time_ms) }}</span>
                </div>
                <div class="detail-item">
                  <span class="detail-label">Proto</span>
                  <span class="detail-value">{{ entry.payload.protocol.toUpperCase() }}</span>
                </div>
                <div class="detail-item">
                  <span class="detail-label">Time</span>
                  <span class="detail-value">{{ since(entry.created_at, true) }}</span>
                </div>
              </div>
            </div>

            <!-- Answers -->
            <div v-if="entry.payload.answers?.length" class="entry-answers">
              <div class="answers-label">Answers ({{ entry.payload.answers.length }})</div>
              <div class="answers-list">
                <div v-for="(ans, aidx) in entry.payload.answers" :key="aidx" class="answer-row">
                  <span class="answer-type">{{ ans.type }}</span>
                  <span class="answer-value mono">{{ ans.value }}</span>
                  <span class="answer-ttl">TTL {{ ans.ttl }}s</span>
                </div>
              </div>
            </div>

            <!-- Raw response toggle -->
            <div class="entry-raw-toggle">
              <button @click="toggleRaw(`${group.target}-${idx}`)" class="raw-btn">
                <i class="bi" :class="expandedRaw[`${group.target}-${idx}`] ? 'bi-chevron-up' : 'bi-chevron-down'"></i>
                Raw Response
              </button>
            </div>
            <div v-if="expandedRaw[`${group.target}-${idx}`]" class="entry-raw">
              <pre>{{ entry.payload.raw_response }}</pre>
            </div>
          </div>
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

.stat-text {
  display: flex;
  flex-direction: column;
  line-height: 1.2;
}

.stat-value {
  font-weight: 700;
  font-size: 1.1rem;
}

.stat-label {
  font-size: 0.72rem;
  opacity: 0.7;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}

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

/* Loading / Error / Empty */
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

.retry-btn {
  padding: 0.3rem 0.75rem;
  border-radius: 6px;
  border: 1px solid currentColor;
  background: transparent;
  cursor: pointer;
  color: inherit;
}

.spinner {
  width: 24px;
  height: 24px;
  border: 2px solid var(--border-color, #d1d5db);
  border-top-color: var(--primary, #3b82f6);
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
}

/* DNS Groups */
.dns-groups {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.dns-entries {
  display: flex;
  flex-direction: column;
}

.dns-entry {
  padding: 0.75rem 1rem;
  border-top: 1px solid var(--border-color, #e5e7eb);
}

.entry-header {
  display: flex;
  align-items: flex-start;
  gap: 1rem;
}

.entry-status {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.2rem 0.6rem;
  border-radius: 4px;
  font-size: 0.78rem;
  font-weight: 600;
  white-space: nowrap;
  flex-shrink: 0;
}

.dns-status-ok {
  color: var(--success, #10b981);
  background: rgba(16, 185, 129, 0.1);
}
.dns-status-warn {
  color: var(--warning, #f59e0b);
  background: rgba(245, 158, 11, 0.1);
}
.dns-status-error {
  color: var(--danger, #ef4444);
  background: rgba(239, 68, 68, 0.1);
}

.entry-details {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
  flex: 1;
}

.detail-item {
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
}

.detail-label {
  font-size: 0.65rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.5;
  font-weight: 500;
}

.detail-value {
  font-size: 0.82rem;
  font-weight: 500;
}

.mono {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 0.78rem;
}

.badge-type {
  display: inline-block;
  padding: 0.05rem 0.4rem;
  background: var(--bg-subtle, #f1f5f9);
  border-radius: 3px;
  font-size: 0.72rem;
  font-weight: 600;
  letter-spacing: 0.03em;
}

/* Answers */
.entry-answers {
  margin-top: 0.5rem;
  padding: 0.5rem 0.75rem;
  background: var(--bg-subtle, #f8f9fa);
  border-radius: 6px;
}

.answers-label {
  font-size: 0.7rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.5;
  margin-bottom: 0.35rem;
  font-weight: 600;
}

.answers-list {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.answer-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  font-size: 0.8rem;
}

.answer-type {
  font-weight: 600;
  min-width: 40px;
  color: var(--primary, #3b82f6);
  font-size: 0.72rem;
}

.answer-value {
  flex: 1;
  word-break: break-all;
}

.answer-ttl {
  font-size: 0.7rem;
  opacity: 0.5;
  white-space: nowrap;
}

/* Raw Response */
.entry-raw-toggle {
  margin-top: 0.4rem;
}

.raw-btn {
  background: none;
  border: none;
  color: var(--muted, #6b7280);
  font-size: 0.72rem;
  cursor: pointer;
  padding: 0.2rem 0;
  display: flex;
  align-items: center;
  gap: 0.3rem;
  opacity: 0.6;
  transition: opacity 0.15s;
}

.raw-btn:hover { opacity: 1; }

.entry-raw {
  margin-top: 0.3rem;
}

.entry-raw pre {
  margin: 0;
  padding: 0.75rem;
  font-size: 0.7rem;
  line-height: 1.4;
  background: var(--bg-code, #1e293b);
  color: var(--text-code, #e2e8f0);
  border-radius: 6px;
  overflow-x: auto;
  max-height: 300px;
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
  .entry-header {
    flex-direction: column;
  }
  .entry-details {
    gap: 0.5rem;
  }
}

.text-success { color: var(--success, #10b981) !important; }
.text-warning { color: var(--warning, #f59e0b) !important; }
.text-danger { color: var(--danger, #ef4444) !important; }
</style>
