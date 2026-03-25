<script lang="ts" setup>
import { ref, onMounted, onUnmounted, computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { PublicShareService } from '@/services/apiService';
import { since } from '@/time';
import { themeService, type Theme } from '@/services/themeService';
import PageContainer from '@/components/PageContainer.vue';

const route = useRoute();
const router = useRouter();
const token = computed(() => route.params.token as string);

// Session storage key for password
const getSessionKey = () => `share_password_${token.value}`;

// State
const loading = ref(true);
const error = ref<string | null>(null);
const requiresPassword = ref(false);
const passwordInput = ref('');
const passwordError = ref<string | null>(null);
const authenticatedPassword = ref<string | null>(null);

// DNS data
const dnsData = ref<any>(null);
const expandedRaw = ref<Record<string, boolean>>({});
const expandedServers = ref<Record<string, boolean>>({});
const lookback = ref(60);

// Derived: host groups with server sub-groups
interface ServerSubGroup {
  server: string;
  recordType: string;
  protocol: string;
  latest: any;
  history: any[];
}

interface HostGroup {
  target: string;
  totalChecks: number;
  servers: ServerSubGroup[];
  overallStatus: 'healthy' | 'warning' | 'critical' | 'unknown';
}

const hostGroups = computed<HostGroup[]>(() => {
  if (!dnsData.value?.groups?.length) return [];

  return dnsData.value.groups.map((group: any) => {
    const serverMap = new Map<string, any[]>();
    const serverOrder: string[] = [];

    for (const entry of group.entries) {
      const server = entry.payload?.dns_server || 'unknown';
      if (!serverMap.has(server)) {
        serverMap.set(server, []);
        serverOrder.push(server);
      }
      serverMap.get(server)!.push(entry);
    }

    const servers: ServerSubGroup[] = serverOrder.map(server => {
      const entries = serverMap.get(server)!;
      const latest = entries[0]!;
      return {
        server,
        recordType: latest.payload?.record_type || 'A',
        protocol: latest.payload?.protocol || 'udp',
        latest,
        history: entries,
      };
    });

    let hasError = false;
    let hasWarn = false;
    for (const s of servers) {
      const r = s.latest?.payload;
      if (r?.error || r?.response_code === 'SERVFAIL' || r?.response_code === 'REFUSED') {
        hasError = true;
      } else if (r?.response_code === 'NXDOMAIN') {
        hasWarn = true;
      }
    }

    return {
      target: group.target,
      totalChecks: group.count,
      servers,
      overallStatus: hasError ? 'critical' : hasWarn ? 'warning' : servers.length > 0 ? 'healthy' : 'unknown',
    };
  });
});

// Summary counts
const healthSummary = computed(() => {
  let totalServers = 0;
  let passing = 0;
  let failing = 0;

  for (const group of hostGroups.value) {
    for (const s of group.servers) {
      totalServers++;
      const r = s.latest?.payload;
      if (r && r.response_code === 'NOERROR' && !r.error) {
        passing++;
      } else {
        failing++;
      }
    }
  }

  const status = failing === 0 ? 'healthy' : failing > passing ? 'critical' : 'warning';
  return { hosts: hostGroups.value.length, totalServers, passing, failing, status };
});

function getStatusColor(code: string, err?: string): string {
  if (err) return 'dns-status-error';
  switch (code) {
    case 'NOERROR': return 'dns-status-ok';
    case 'NXDOMAIN': return 'dns-status-warn';
    case 'SERVFAIL': return 'dns-status-error';
    case 'REFUSED': return 'dns-status-error';
    default: return 'dns-status-warn';
  }
}

function getStatusIcon(code: string, err?: string): string {
  if (err) return 'bi-x-circle-fill';
  switch (code) {
    case 'NOERROR': return 'bi-check-circle-fill';
    case 'NXDOMAIN': return 'bi-exclamation-triangle-fill';
    default: return 'bi-dash-circle-fill';
  }
}

function getOverallStatusIcon(status: string): string {
  switch (status) {
    case 'healthy': return 'bi-check-circle-fill';
    case 'warning': return 'bi-exclamation-triangle-fill';
    case 'critical': return 'bi-x-circle-fill';
    default: return 'bi-question-circle';
  }
}

function toggleRaw(key: string) {
  expandedRaw.value[key] = !expandedRaw.value[key];
}

function toggleServerHistory(key: string) {
  expandedServers.value[key] = !expandedServers.value[key];
}

function formatLatency(ms: number): string {
  if (ms < 1) return `${(ms * 1000).toFixed(0)}µs`;
  if (ms < 100) return `${ms.toFixed(2)}ms`;
  return `${ms.toFixed(0)}ms`;
}

function formatTime(t: string): string {
  try {
    const d = new Date(t);
    return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  } catch {
    return t;
  }
}

function latencyClass(ms: number): string {
  if (ms < 50) return 'latency-good';
  if (ms < 200) return 'latency-warn';
  return 'latency-bad';
}

function answersPreview(answers: { value: string; type: string }[]): string {
  if (!answers?.length) return '—';
  return answers.slice(0, 3).map(a => a.value).join(', ') + (answers.length > 3 ? ` +${answers.length - 3}` : '');
}

// Load DNS data
async function fetchData() {
  loading.value = true;
  error.value = null;
  try {
    dnsData.value = await PublicShareService.getDnsDashboard(token.value, {
      lookback: lookback.value,
      limit: 500,
      password: authenticatedPassword.value || undefined,
    });
  } catch (err: any) {
    error.value = err?.message || 'Failed to load DNS data';
  } finally {
    loading.value = false;
  }
}

// Initial load: check auth first
async function loadInitial() {
  loading.value = true;
  try {
    const info = await PublicShareService.getInfo(token.value);

    if (info.expired) {
      error.value = 'This share link has expired.';
      loading.value = false;
      return;
    }

    if (info.has_password) {
      const cached = sessionStorage.getItem(getSessionKey());
      if (cached) {
        try {
          authenticatedPassword.value = cached;
          await fetchData();
          return;
        } catch {
          sessionStorage.removeItem(getSessionKey());
          authenticatedPassword.value = null;
        }
      }
      requiresPassword.value = true;
      loading.value = false;
      return;
    }

    await fetchData();
  } catch (err: any) {
    if (err.message === 'PASSWORD_REQUIRED') {
      requiresPassword.value = true;
    } else {
      error.value = err.message || 'Failed to load DNS data';
    }
    loading.value = false;
  }
}

// Password submission
async function submitPassword() {
  passwordError.value = null;
  try {
    authenticatedPassword.value = passwordInput.value;
    sessionStorage.setItem(getSessionKey(), passwordInput.value);
    requiresPassword.value = false;
    await fetchData();
  } catch (err: any) {
    if (err.message === 'INVALID_PASSWORD') {
      passwordError.value = 'Incorrect password. Please try again.';
      authenticatedPassword.value = null;
      sessionStorage.removeItem(getSessionKey());
    } else {
      error.value = err.message || 'Failed to access shared agent';
    }
  }
}

function goBack() {
  router.push({ name: 'sharedAgent', params: { token: token.value } });
}

// Theme
const currentTheme = ref<Theme>(themeService.getTheme());
let themeUnsubscribe: (() => void) | null = null;
function toggleTheme() {
  themeService.toggle();
}

onMounted(() => {
  themeUnsubscribe = themeService.onThemeChange((theme) => {
    currentTheme.value = theme;
  });
  loadInitial();
});

onUnmounted(() => {
  if (themeUnsubscribe) {
    themeUnsubscribe();
    themeUnsubscribe = null;
  }
});
</script>

<template>
    <div class="shared-dns-page" :data-theme="currentTheme">
        <!-- Header -->
        <header class="shared-header">
            <PageContainer size="full">
                <div class="header-content">
                    <div class="brand">
                        <i class="bi bi-eye"></i>
                        <span class="brand-text">NetWatcher</span>
                        <span class="badge">Shared View</span>
                    </div>
                    <div class="header-actions">
                        <button class="theme-toggle-btn" @click="toggleTheme" :title="currentTheme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'">
                            <i :class="currentTheme === 'dark' ? 'bi bi-sun' : 'bi bi-moon'"></i>
                        </button>
                    </div>
                </div>
            </PageContainer>
        </header>
        
        <main class="shared-main">
            <PageContainer size="default">
                <!-- Back button -->
                <button class="back-btn" @click="goBack">
                    <i class="bi bi-arrow-left"></i>
                    Back to Agent
                </button>

                <!-- Loading State -->
                <div v-if="loading && !dnsData" class="loading-state">
                    <i class="bi bi-arrow-repeat spin"></i>
                    <p>Loading DNS data...</p>
                </div>
                
                <!-- Error State -->
                <div v-else-if="error" class="error-state">
                    <i class="bi bi-exclamation-triangle"></i>
                    <h2>Unable to Access</h2>
                    <p>{{ error }}</p>
                </div>
                
                <!-- Password Required -->
                <div v-else-if="requiresPassword" class="password-state">
                    <div class="password-card">
                        <i class="bi bi-lock"></i>
                        <h2>Password Protected</h2>
                        <p>This share link requires a password to access.</p>
                        <form @submit.prevent="submitPassword" class="password-form">
                            <div v-if="passwordError" class="password-error">
                                <i class="bi bi-exclamation-circle"></i>
                                {{ passwordError }}
                            </div>
                            <input type="password" v-model="passwordInput" placeholder="Enter password" class="password-input" autofocus />
                            <button type="submit" class="password-btn" :disabled="!passwordInput">
                                <i class="bi bi-unlock"></i>
                                Access DNS Data
                            </button>
                        </form>
                    </div>
                </div>
                
                <!-- DNS Content -->
                <div v-else class="dns-content">
                    <div class="dns-page-header">
                        <h1><i class="bi bi-globe2"></i> DNS Monitoring</h1>
                    </div>

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

                    <!-- Empty State -->
                    <div v-if="!hostGroups.length && !loading" class="dns-empty">
                        <i class="bi bi-globe2"></i>
                        <h4>No DNS Data</h4>
                        <p>No DNS probes are configured for this agent, or no data has been collected yet.</p>
                    </div>

                    <!-- Host Groups -->
                    <div v-else class="dns-groups">
                        <div v-for="host in hostGroups" :key="host.target" class="host-card">
                            <div class="host-header">
                                <div class="host-info">
                                    <i class="bi bi-globe2"></i>
                                    <span class="host-target">{{ host.target }}</span>
                                    <span class="host-meta">{{ host.servers.length }} resolver{{ host.servers.length !== 1 ? 's' : '' }} · {{ host.totalChecks }} checks</span>
                                </div>
                                <div class="host-status" :class="host.overallStatus">
                                    <i class="bi" :class="getOverallStatusIcon(host.overallStatus)"></i>
                                </div>
                            </div>

                            <!-- Server Comparison Table -->
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
                                            <tr class="server-row">
                                                <td class="mono server-cell">
                                                    <i class="bi bi-hdd-network"></i>
                                                    {{ srv.server }}
                                                </td>
                                                <td><span class="badge-type">{{ srv.recordType }}</span></td>
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
                                                    <button v-if="srv.history.length > 1" @click="toggleServerHistory(`${host.target}::${srv.server}`)" class="history-btn" :title="`${srv.history.length} historical results`">
                                                        <i class="bi" :class="expandedServers[`${host.target}::${srv.server}`] ? 'bi-chevron-up' : 'bi-chevron-down'"></i>
                                                        {{ srv.history.length }}
                                                    </button>
                                                    <button @click="toggleRaw(`${host.target}::${srv.server}::latest`)" class="raw-toggle-btn" title="View raw DNS response">
                                                        <i class="bi bi-code-slash"></i>
                                                    </button>
                                                </td>
                                            </tr>

                                            <!-- Raw response for latest -->
                                            <tr v-if="expandedRaw[`${host.target}::${srv.server}::latest`]">
                                                <td colspan="7" class="raw-cell">
                                                    <pre>{{ srv.latest.payload?.raw_response }}</pre>
                                                </td>
                                            </tr>

                                            <!-- Historical results -->
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
                                                                <tr v-for="(h, hIdx) in srv.history" :key="hIdx">
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
                                                                        <button @click="toggleRaw(`${host.target}::${srv.server}::${hIdx}`)" class="raw-toggle-btn sm" title="Raw">
                                                                            <i class="bi bi-code-slash"></i>
                                                                        </button>
                                                                    </td>
                                                                </tr>
                                                            </tbody>
                                                        </table>
                                                        <template v-for="(h, hIdx) in srv.history" :key="`raw-${hIdx}`">
                                                            <div v-if="expandedRaw[`${host.target}::${srv.server}::${hIdx}`]" class="history-raw">
                                                                <div class="raw-header">{{ srv.server }} · {{ formatTime(h.created_at) }}</div>
                                                                <pre>{{ h.payload?.raw_response }}</pre>
                                                            </div>
                                                        </template>
                                                    </div>
                                                </td>
                                            </tr>
                                        </template>
                                    </tbody>
                                </table>
                            </div>
                        </div>
                    </div>

                    <!-- Footer Notice -->
                    <div class="shared-footer">
                        <p>
                            <i class="bi bi-info-circle"></i>
                            This is a read-only view. Data updates may be delayed.
                        </p>
                    </div>
                </div>
            </PageContainer>
        </main>
    </div>
</template>

<style scoped>
.shared-dns-page {
    min-height: 100vh;
    background: var(--bs-body-bg);
    color: var(--bs-body-color);
}

.shared-header {
    background: var(--bs-tertiary-bg);
    border-bottom: 1px solid var(--bs-border-color);
    padding: 1rem 0;
}

.header-content {
    display: flex;
    align-items: center;
    justify-content: space-between;
}

.header-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

.theme-toggle-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 44px;
    height: 44px;
    min-width: 44px;
    min-height: 44px;
    border: none;
    background: var(--bs-body-bg);
    color: var(--bs-body-color);
    border-radius: 8px;
    cursor: pointer;
    transition: all 0.2s;
}
.theme-toggle-btn:hover { background: var(--bs-border-color); }
.theme-toggle-btn i { font-size: 1.125rem; }

.brand {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 1.25rem;
    font-weight: 600;
}
.brand i { color: var(--bs-primary); }

.brand .badge {
    background: linear-gradient(135deg, #3b82f6, #10b981);
    color: white;
    padding: 0.25rem 0.625rem;
    border-radius: 4px;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
}

.shared-main { padding: 2rem 0; }

/* Back button */
.back-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 1rem;
    background: var(--bs-tertiary-bg);
    border: 1px solid var(--bs-border-color);
    border-radius: 8px;
    color: var(--bs-body-color);
    font-size: 0.875rem;
    cursor: pointer;
    margin-bottom: 1.5rem;
    transition: all 0.2s;
}
.back-btn:hover { background: var(--bs-border-color); }

/* Loading State */
.loading-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 4rem 2rem;
    text-align: center;
    color: var(--bs-secondary-color);
}
.loading-state i { font-size: 2.5rem; margin-bottom: 1rem; }

/* Error State */
.error-state {
    text-align: center;
    padding: 4rem 2rem;
}
.error-state i { font-size: 4rem; margin-bottom: 1.5rem; color: #ef4444; }
.error-state h2 { font-size: 1.5rem; margin-bottom: 0.75rem; }
.error-state p { color: var(--bs-secondary-color); max-width: 400px; margin: 0 auto; }

/* Password State */
.password-state {
    display: flex;
    justify-content: center;
    padding: 3rem 1rem;
}
.password-card {
    background: var(--bs-body-bg);
    border: 1px solid var(--bs-border-color);
    border-radius: 12px;
    padding: 2.5rem;
    text-align: center;
    max-width: 400px;
    width: 100%;
    box-shadow: var(--bs-box-shadow);
}
.password-card i { font-size: 3rem; color: var(--bs-primary); margin-bottom: 1rem; }
.password-card h2 { font-size: 1.25rem; margin-bottom: 0.5rem; }
.password-card p { color: var(--bs-secondary-color); font-size: 0.875rem; margin-bottom: 1.5rem; }
.password-form { display: flex; flex-direction: column; gap: 1rem; }
.password-error {
    background: rgba(239, 68, 68, 0.15);
    color: #ef4444;
    padding: 0.75rem;
    border-radius: 8px;
    font-size: 0.875rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
}
.password-input {
    background: var(--bs-body-bg);
    border: 1px solid var(--bs-border-color);
    border-radius: 8px;
    padding: 0.875rem 1rem;
    color: var(--bs-body-color);
    font-size: 1rem;
}
.password-input:focus {
    outline: none;
    border-color: var(--bs-primary);
    box-shadow: 0 0 0 3px rgba(var(--bs-primary-rgb), 0.25);
}
.password-btn {
    background: var(--bs-primary);
    color: white;
    border: none;
    border-radius: 8px;
    padding: 0.875rem;
    font-weight: 500;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    transition: all 0.2s;
}
.password-btn:hover:not(:disabled) { opacity: 0.9; transform: translateY(-1px); }
.password-btn:disabled { opacity: 0.6; cursor: not-allowed; }

/* DNS Page Header */
.dns-page-header {
    margin-bottom: 1.5rem;
}
.dns-page-header h1 {
    font-size: 1.75rem;
    font-weight: 700;
    display: flex;
    align-items: center;
    gap: 0.75rem;
}
.dns-page-header h1 i {
    color: var(--bs-primary);
}

/* DNS Summary */
.dns-summary {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 0.75rem 1rem;
    background: var(--bs-body-bg);
    border: 1px solid var(--bs-border-color);
    border-radius: 8px;
    flex-wrap: wrap;
    margin-bottom: 1rem;
}
.dns-summary-stat {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.75rem;
    border-radius: 6px;
    background: var(--bs-tertiary-bg);
}
.dns-summary-stat.healthy { color: #10b981; }
.dns-summary-stat.warning { color: #f59e0b; }
.dns-summary-stat.critical { color: #ef4444; }
.dns-summary-stat.unknown { color: #6b7280; }

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
    border: 1px solid var(--bs-border-color);
    background: var(--bs-tertiary-bg);
    font-size: 0.8rem;
    cursor: pointer;
    color: inherit;
}

.btn-refresh {
    padding: 0.35rem 0.5rem;
    border-radius: 6px;
    border: 1px solid var(--bs-border-color);
    background: var(--bs-tertiary-bg);
    cursor: pointer;
    font-size: 0.85rem;
    color: inherit;
    transition: background 0.15s;
}
.btn-refresh:hover { background: var(--bs-border-color); }
.btn-refresh:disabled { opacity: 0.5; cursor: not-allowed; }

.spin { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }

/* Empty State */
.dns-empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 3rem 1rem;
    gap: 0.75rem;
    color: var(--bs-secondary-color);
}
.dns-empty i { font-size: 2.5rem; opacity: 0.3; }
.dns-empty h4 { margin: 0; font-weight: 600; }
.dns-empty p { margin: 0; opacity: 0.6; font-size: 0.9rem; }

/* Host Groups */
.dns-groups { display: flex; flex-direction: column; gap: 0.75rem; }

.host-card {
    background: var(--bs-body-bg);
    border: 1px solid var(--bs-border-color);
    border-radius: 10px;
    overflow: hidden;
}

.host-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.875rem 1rem;
    border-bottom: 1px solid var(--bs-border-color);
}

.host-info {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
}
.host-info > i { color: var(--bs-primary); opacity: 0.7; }
.host-target { font-weight: 600; font-size: 0.95rem; }
.host-meta { font-size: 0.75rem; color: var(--bs-secondary-color); }

.host-status {
    display: flex;
    align-items: center;
    font-size: 1rem;
}
.host-status.healthy { color: #10b981; }
.host-status.warning { color: #f59e0b; }
.host-status.critical { color: #ef4444; }
.host-status.unknown { color: #6b7280; }

/* Server Table */
.server-table-wrap { overflow-x: auto; }

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
    border-bottom: 1px solid var(--bs-border-color);
    white-space: nowrap;
}
.server-table tbody td {
    padding: 0.6rem 0.75rem;
    border-bottom: 1px solid var(--bs-border-color);
    vertical-align: middle;
}
.server-row:hover td { background: var(--bs-tertiary-bg); }

.server-cell {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    white-space: nowrap;
}
.server-cell i { opacity: 0.4; font-size: 0.75rem; }

.mono {
    font-family: 'JetBrains Mono', 'Fira Code', monospace;
    font-size: 0.78rem;
}

.badge-type {
    display: inline-block;
    padding: 0.1rem 0.4rem;
    background: var(--bs-tertiary-bg);
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

.dns-status-ok { color: #10b981; background: rgba(16, 185, 129, 0.1); }
.dns-status-warn { color: #f59e0b; background: rgba(245, 158, 11, 0.1); }
.dns-status-error { color: #ef4444; background: rgba(239, 68, 68, 0.1); }

.latency-good { color: #10b981; }
.latency-warn { color: #f59e0b; }
.latency-bad { color: #ef4444; }

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
    border: 1px solid var(--bs-border-color);
    border-radius: 4px;
    padding: 0.15rem 0.4rem;
    font-size: 0.7rem;
    cursor: pointer;
    color: var(--bs-secondary-color);
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
    transition: all 0.15s;
}
.history-btn:hover, .raw-toggle-btn:hover {
    background: var(--bs-tertiary-bg);
    color: var(--bs-body-color);
}
.raw-toggle-btn.sm { padding: 0.1rem 0.3rem; }

/* Raw response */
.raw-cell { padding: 0 !important; }
.raw-cell pre {
    margin: 0;
    padding: 0.75rem 1rem;
    font-size: 0.68rem;
    line-height: 1.4;
    background: #1e293b;
    color: #e2e8f0;
    overflow-x: auto;
    max-height: 250px;
    white-space: pre-wrap;
    word-break: break-all;
    font-family: 'JetBrains Mono', 'Fira Code', monospace;
}

/* History Panel */
.history-cell { padding: 0 !important; }
.history-panel {
    background: var(--bs-tertiary-bg);
    border-top: 2px solid var(--bs-border-color);
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
    border-bottom: 1px solid var(--bs-border-color);
}
.history-count { font-weight: 400; opacity: 0.6; }

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
    border-bottom: 1px solid var(--bs-border-color);
    vertical-align: middle;
}
.history-table tbody tr:hover td { background: rgba(0, 0, 0, 0.02); }

.history-raw { margin: 0; }
.history-raw .raw-header {
    padding: 0.3rem 0.75rem;
    font-size: 0.65rem;
    opacity: 0.5;
    font-family: 'JetBrains Mono', 'Fira Code', monospace;
    border-top: 1px solid var(--bs-border-color);
}
.history-raw pre {
    margin: 0;
    padding: 0.5rem 0.75rem;
    font-size: 0.65rem;
    line-height: 1.3;
    background: #1e293b;
    color: #e2e8f0;
    overflow-x: auto;
    max-height: 200px;
    white-space: pre-wrap;
    word-break: break-all;
    font-family: 'JetBrains Mono', 'Fira Code', monospace;
}

/* Footer */
.shared-footer {
    margin-top: 2rem;
    padding-top: 1.5rem;
    border-top: 1px solid var(--bs-border-color);
    text-align: center;
}
.shared-footer p {
    color: var(--bs-secondary-color);
    font-size: 0.8rem;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.375rem;
}

/* Responsive */
@media (max-width: 768px) {
    .dns-summary { flex-direction: column; align-items: stretch; }
    .dns-summary-actions { margin-left: 0; justify-content: flex-end; }
    .answer-cell { max-width: 140px; }
}

/* ==================== Dark Mode ==================== */
[data-theme="dark"] .dns-summary {
    background: #1a1f2e;
    border-color: #2a3042;
}
[data-theme="dark"] .dns-summary-stat { background: #232838; }
[data-theme="dark"] .lookback-select,
[data-theme="dark"] .btn-refresh {
    background: #232838;
    border-color: #2a3042;
    color: #c8cdd8;
}
[data-theme="dark"] .lookback-select:hover,
[data-theme="dark"] .btn-refresh:hover { background: #2a3042; }

[data-theme="dark"] .host-card {
    background: #1a1f2e;
    border-color: #2a3042;
}
[data-theme="dark"] .host-header { border-color: #2a3042; }

[data-theme="dark"] .server-table thead th { border-color: #2a3042; color: #8890a0; }
[data-theme="dark"] .server-table tbody td { border-color: #1e2333; }
[data-theme="dark"] .server-row:hover td { background: #1e2333; }

[data-theme="dark"] .badge-type { background: #232838; color: #c8cdd8; }

[data-theme="dark"] .history-btn,
[data-theme="dark"] .raw-toggle-btn { border-color: #2a3042; color: #8890a0; }
[data-theme="dark"] .history-btn:hover,
[data-theme="dark"] .raw-toggle-btn:hover { background: #232838; color: #e0e4ec; }

[data-theme="dark"] .history-panel { background: #161a26; border-color: #2a3042; }
[data-theme="dark"] .history-title { border-color: #2a3042; color: #8890a0; }
[data-theme="dark"] .history-table thead th { color: #6b7280; }
[data-theme="dark"] .history-table tbody td { border-color: #1e2333; }
[data-theme="dark"] .history-table tbody tr:hover td { background: rgba(255, 255, 255, 0.03); }

[data-theme="dark"] .raw-cell pre,
[data-theme="dark"] .history-raw pre { background: #0f1219; color: #a5b4c8; }
[data-theme="dark"] .history-raw .raw-header { border-color: #2a3042; color: #6b7280; }

[data-theme="dark"] .dns-status-ok { color: #34d399; background: rgba(52, 211, 153, 0.12); }
[data-theme="dark"] .dns-status-warn { color: #fbbf24; background: rgba(251, 191, 36, 0.12); }
[data-theme="dark"] .dns-status-error { color: #f87171; background: rgba(248, 113, 113, 0.12); }

[data-theme="dark"] .dns-empty { color: #6b7280; }
[data-theme="dark"] .dns-empty i { opacity: 0.2; }

[data-theme="dark"] .host-status.healthy { color: #34d399; }
[data-theme="dark"] .host-status.warning { color: #fbbf24; }
[data-theme="dark"] .host-status.critical { color: #f87171; }
[data-theme="dark"] .host-status.unknown { color: #6b7280; }

[data-theme="dark"] .latency-good { color: #34d399; }
[data-theme="dark"] .latency-warn { color: #fbbf24; }
[data-theme="dark"] .latency-bad { color: #f87171; }
[data-theme="dark"] .time-cell { color: #6b7280; }
[data-theme="dark"] .mono { color: #c8cdd8; }

[data-theme="dark"] .dns-summary-stat.healthy { color: #34d399; }
[data-theme="dark"] .dns-summary-stat.warning { color: #fbbf24; }
[data-theme="dark"] .dns-summary-stat.critical { color: #f87171; }

[data-theme="dark"] .back-btn {
    background: #1a1f2e;
    border-color: #2a3042;
    color: #c8cdd8;
}
[data-theme="dark"] .back-btn:hover { background: #232838; }
</style>
