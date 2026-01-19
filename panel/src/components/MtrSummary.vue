<template>
  <div class="mtr-summary">
    <!-- Summary Header -->
    <div class="summary-header">
      <div class="summary-stats">
        <div class="stat-item">
          <span class="stat-value">{{ totalTraces }}</span>
          <span class="stat-label">Traces</span>
        </div>
        <div class="stat-item">
          <span class="stat-value">{{ uniqueRoutes }}</span>
          <span class="stat-label">Unique Routes</span>
        </div>
        <div class="stat-item" v-if="issueCount > 0" :class="{ warning: true }">
          <span class="stat-value">{{ issueCount }}</span>
          <span class="stat-label">Issues</span>
        </div>
        <div class="stat-item" v-if="routeChangeCount > 0" :class="{ info: true }">
          <span class="stat-value">{{ routeChangeCount }}</span>
          <span class="stat-label">Route Changes</span>
        </div>
      </div>
      
      <!-- View All Button -->
      <button v-if="props.mtrData.length > 0" class="btn btn-sm btn-outline-primary" @click="emit('show-all-traces')">
        <i class="bi bi-list-ul me-1"></i>View All Traces
      </button>
    </div>

    <!-- No Data State -->
    <div v-if="!props.mtrData || props.mtrData.length === 0" class="no-data">
      <i class="bi bi-diagram-3"></i>
      <p>No traceroute data available</p>
    </div>

    <!-- Route Summary Cards - Grouped by Route -->
    <div v-else class="route-list">
      <div 
        v-for="(group, index) in paginatedGroups" 
        :key="group.signature"
        class="route-row"
        :class="{ 
          'has-issues': group.hasIssues,
          'is-primary': index === 0 && !group.hasIssues
        }"
      >
        <div class="route-main" @click="toggleGroup(group.signature)">
          <!-- Badge -->
          <div class="route-badge" :class="getBadgeClass(group)">
            <i :class="getBadgeIcon(group)"></i>
            <span>{{ group.count }} trace{{ group.count !== 1 ? 's' : '' }}</span>
          </div>
          
          <!-- Route Info -->
          <div class="route-info">
            <span class="hop-count">{{ group.hopCount }} hops</span>
            <span class="route-path">
              {{ group.sourceIp }} <i class="bi bi-arrow-right-short"></i> {{ group.destIp }}
            </span>
          </div>
          
          <!-- Metrics -->
          <div class="route-metrics">
            <div class="metric" :class="getLatencyClass(group.avgLatency)">
              <span class="metric-value">{{ group.avgLatency.toFixed(1) }}ms</span>
              <span class="metric-label">Avg Latency</span>
            </div>
            <div class="metric" :class="getLossClass(group.maxLoss)">
              <span class="metric-value">{{ group.maxLoss.toFixed(1) }}%</span>
              <span class="metric-label">Max Loss</span>
            </div>
            <div class="metric time">
              <span class="metric-value">{{ formatTimeRange(group) }}</span>
              <span class="metric-label">Time Range</span>
            </div>
          </div>
          
          <i :class="expandedGroups[group.signature] ? 'bi bi-chevron-up' : 'bi bi-chevron-down'" class="expand-icon"></i>
        </div>
        
        <!-- Expanded View - Show Recent Traces -->
        <div v-if="expandedGroups[group.signature]" class="route-expanded">
          <!-- Route Change Diff Display -->
          <div v-if="group.isRouteChange && group.changedFromHops" class="route-diff">
            <div class="diff-header">
              <i class="bi bi-arrow-repeat me-2"></i>
              <strong>Route Change Detected</strong>
              <span v-if="group.routeChangeAt" class="diff-time">
                <i class="bi bi-clock ms-3 me-1"></i>
                {{ formatRouteChangeTime(group.routeChangeAt) }}
              </span>
            </div>
            <div class="diff-content">
              <div class="diff-column diff-old">
                <div class="diff-label">Previous Route</div>
                <div class="diff-hops">
                  <div 
                    v-for="(ip, idx) in group.changedFromHops" 
                    :key="'old-'+idx" 
                    class="diff-hop"
                    :class="{ 'diff-removed': !group.hopIps.includes(ip), 'diff-unchanged': group.hopIps[idx] === ip }"
                  >
                    <span class="hop-num">{{ idx + 1 }}</span>
                    <span class="hop-ip">{{ ip }}</span>
                    <i v-if="!group.hopIps.includes(ip)" class="bi bi-dash-circle-fill text-danger ms-1"></i>
                  </div>
                </div>
              </div>
              <div class="diff-arrow">
                <i class="bi bi-arrow-right-circle-fill"></i>
              </div>
              <div class="diff-column diff-new">
                <div class="diff-label">Current Route</div>
                <div class="diff-hops">
                  <div 
                    v-for="(ip, idx) in group.hopIps" 
                    :key="'new-'+idx" 
                    class="diff-hop"
                    :class="{ 'diff-added': !group.changedFromHops?.includes(ip), 'diff-unchanged': group.changedFromHops?.[idx] === ip }"
                  >
                    <span class="hop-num">{{ idx + 1 }}</span>
                    <span class="hop-ip">{{ ip }}</span>
                    <i v-if="!group.changedFromHops?.includes(ip)" class="bi bi-plus-circle-fill text-success ms-1"></i>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div class="trace-list">
            <div 
              v-for="(trace, traceIdx) in getRecentTraces(group)" 
              :key="traceIdx"
              class="trace-row"
            >
              <div class="trace-timestamp">
                <i class="bi bi-clock me-1"></i>
                {{ formatTraceTimestamp(trace) }}
              </div>
              <MtrTable :probe-data="trace" :show-copy="true" />
            </div>
            <div v-if="group.traces.length > 5" class="more-traces">
              <span class="text-muted">+ {{ group.traces.length - 5 }} more traces</span>
            </div>
          </div>
        </div>
      </div>
      
      <!-- Pagination -->
      <nav v-if="totalPages > 1" class="pagination-nav">
        <ul class="pagination pagination-sm mb-0">
          <li class="page-item" :class="{ disabled: currentPage === 1 }">
            <button class="page-link" @click="currentPage = Math.max(1, currentPage - 1)">
              <i class="bi bi-chevron-left"></i>
            </button>
          </li>
          <li v-for="p in visiblePages" :key="p" class="page-item" :class="{ active: p === currentPage }">
            <button class="page-link" @click="currentPage = p">{{ p }}</button>
          </li>
          <li class="page-item" :class="{ disabled: currentPage === totalPages }">
            <button class="page-link" @click="currentPage = Math.min(totalPages, currentPage + 1)">
              <i class="bi bi-chevron-right"></i>
            </button>
          </li>
        </ul>
        <span class="page-info">{{ paginatedGroups.length }} of {{ routeGroups.length }} routes</span>
      </nav>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { computed, ref, reactive } from 'vue';
import type { ProbeData } from '@/types';
import MtrTable from '@/components/MtrTable.vue';

interface RouteGroup {
  signature: string;
  sourceIp: string;
  destIp: string;
  hopCount: number;
  hopIps: string[];          // Array of hop IPs for diff comparison
  traces: ProbeData[];
  count: number;
  firstSeen: Date;
  lastSeen: Date;
  avgLatency: number;
  maxLoss: number;
  hasIssues: boolean;
  isRouteChange: boolean;
  changedFromSignature: string | null;  // Previous route signature for diff
  changedFromHops: string[] | null;     // Previous route hops for diff
  routeChangeAt: Date | null;           // When the route change was first detected
}

const props = defineProps<{
  mtrData: ProbeData[];
}>();

const emit = defineEmits<{
  (e: 'show-all-traces'): void;
}>();

// Pagination
const currentPage = ref(1);
const pageSize = 10;

// Expansion state
const expandedGroups = reactive<Record<string, boolean>>({});

// Group traces by route signature
const routeGroups = computed<RouteGroup[]>(() => {
  if (!props.mtrData || props.mtrData.length === 0) return [];
  
  const groups = new Map<string, RouteGroup>();
  let prevSignature = '';
  let prevHopIps: string[] = [];
  
  // Sort by time ascending for route change detection
  const sortedData = [...props.mtrData].sort((a, b) => 
    new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
  );
  
  for (const trace of sortedData) {
    const payload = trace.payload as any;
    if (!payload?.report?.hops) continue;
    
    const hops = payload.report.hops;
    
    // Build signature from responding hops only
    const hopIps = hops.map((h: any) => h.hosts?.[0]?.ip || '*');
    const signature = hopIps.join('->');
    
    // Find first and last responding hops
    let sourceIp = '*';
    let destIp = '*';
    for (const hop of hops) {
      const ip = hop.hosts?.[0]?.ip;
      if (ip && ip !== '*') {
        if (sourceIp === '*') sourceIp = ip;
        destIp = ip;
      }
    }
    
    // Calculate metrics (only from responding hops)
    let finalLatency = 0;
    let maxLoss = 0;
    for (const hop of hops) {
      const ip = hop.hosts?.[0]?.ip;
      if (!ip || ip === '*') continue;
      
      const loss = parseFloat(String(hop.loss_pct || '0').replace('%', ''));
      if (!isNaN(loss)) maxLoss = Math.max(maxLoss, loss);
      
      const latency = parseFloat(hop.avg || '0');
      if (!isNaN(latency) && latency > 0) finalLatency = latency;
    }
    
    const timestamp = new Date(payload.stop_timestamp || trace.created_at);
    const isRouteChange = prevSignature !== '' && signature !== prevSignature;
    const hasIssues = trace.triggered || maxLoss > 20;
    
    if (groups.has(signature)) {
      const group = groups.get(signature)!;
      group.traces.push(trace);
      group.count++;
      group.avgLatency = ((group.avgLatency * (group.count - 1)) + finalLatency) / group.count;
      group.maxLoss = Math.max(group.maxLoss, maxLoss);
      group.firstSeen = timestamp < group.firstSeen ? timestamp : group.firstSeen;
      group.lastSeen = timestamp > group.lastSeen ? timestamp : group.lastSeen;
      group.hasIssues = group.hasIssues || hasIssues;
      // Keep track of route change status and previous route
      if (isRouteChange && !group.isRouteChange) {
        group.isRouteChange = true;
        group.changedFromSignature = prevSignature;
        group.changedFromHops = [...prevHopIps];
        group.routeChangeAt = timestamp;
      }
    } else {
      groups.set(signature, {
        signature,
        sourceIp,
        destIp,
        hopCount: hops.length,
        hopIps,
        traces: [trace],
        count: 1,
        firstSeen: timestamp,
        lastSeen: timestamp,
        avgLatency: finalLatency,
        maxLoss,
        hasIssues,
        isRouteChange,
        changedFromSignature: isRouteChange ? prevSignature : null,
        changedFromHops: isRouteChange ? [...prevHopIps] : null,
        routeChangeAt: isRouteChange ? timestamp : null,
      });
    }
    
    prevSignature = signature;
    prevHopIps = hopIps;
  }
  
  // Sort: route changes first, then issues, then by count descending
  return Array.from(groups.values()).sort((a, b) => {
    // Route changes that are also issues go first
    if (a.isRouteChange && !b.isRouteChange) return -1;
    if (!a.isRouteChange && b.isRouteChange) return 1;
    if (a.hasIssues && !b.hasIssues) return -1;
    if (!a.hasIssues && b.hasIssues) return 1;
    return b.count - a.count;
  });
});

// Pagination computed
const totalPages = computed(() => Math.ceil(routeGroups.value.length / pageSize));
const paginatedGroups = computed(() => {
  const start = (currentPage.value - 1) * pageSize;
  return routeGroups.value.slice(start, start + pageSize);
});
const visiblePages = computed(() => {
  const pages: number[] = [];
  const start = Math.max(1, currentPage.value - 2);
  const end = Math.min(totalPages.value, start + 4);
  for (let i = start; i <= end; i++) pages.push(i);
  return pages;
});

// Stats
const totalTraces = computed(() => routeGroups.value.reduce((sum, g) => sum + g.count, 0));
const uniqueRoutes = computed(() => routeGroups.value.length);
const issueCount = computed(() => routeGroups.value.filter(g => g.hasIssues).length);
const routeChangeCount = computed(() => routeGroups.value.filter(g => g.isRouteChange).length);

// Methods
const toggleGroup = (signature: string) => {
  expandedGroups[signature] = !expandedGroups[signature];
};

// Get 5 most recent traces for expanded view, sorted newest first
const getRecentTraces = (group: RouteGroup): ProbeData[] => {
  return [...group.traces]
    .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
    .slice(0, 5);
};

// Format a single trace timestamp with date and time
const formatTraceTimestamp = (trace: ProbeData): string => {
  const payload = trace.payload as any;
  const timestamp = new Date(payload?.stop_timestamp || trace.created_at);
  const today = new Date();
  const isToday = timestamp.toDateString() === today.toDateString();
  
  const formatTime = (d: Date) => d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit', second: '2-digit' });
  
  if (isToday) {
    return `Today at ${formatTime(timestamp)}`;
  }
  
  return timestamp.toLocaleDateString('en-US', { 
    month: 'short', 
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit'
  });
};

// Format time when a route change was detected
const formatRouteChangeTime = (date: Date): string => {
  const now = new Date();
  const isToday = date.toDateString() === now.toDateString();
  
  const formatTime = (d: Date) => d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
  
  if (isToday) {
    return `Today at ${formatTime(date)}`;
  }
  
  // Check if yesterday
  const yesterday = new Date(now);
  yesterday.setDate(yesterday.getDate() - 1);
  if (date.toDateString() === yesterday.toDateString()) {
    return `Yesterday at ${formatTime(date)}`;
  }
  
  return date.toLocaleDateString('en-US', { 
    month: 'short', 
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit'
  });
};

const formatTimeRange = (group: RouteGroup): string => {
  const now = new Date();
  const sameDay = group.firstSeen.toDateString() === group.lastSeen.toDateString();
  const isToday = group.lastSeen.toDateString() === now.toDateString();
  const formatTime = (d: Date) => d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
  const formatDate = (d: Date) => d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
  
  if (group.count === 1) {
    return isToday ? formatTime(group.lastSeen) : `${formatDate(group.lastSeen)} ${formatTime(group.lastSeen)}`;
  }
  if (sameDay) {
    return `${formatTime(group.firstSeen)} - ${formatTime(group.lastSeen)}`;
  }
  return `${formatDate(group.firstSeen)} - ${formatDate(group.lastSeen)}`;
};

const getBadgeClass = (group: RouteGroup): string => {
  if (group.hasIssues) return 'badge-issue';
  if (group.isRouteChange) return 'badge-change';
  return 'badge-normal';
};

const getBadgeIcon = (group: RouteGroup): string => {
  if (group.hasIssues) return 'bi bi-exclamation-triangle-fill';
  if (group.isRouteChange) return 'bi bi-arrow-repeat';
  return 'bi bi-check-circle';
};

const getLatencyClass = (latency: number): string => {
  if (latency < 50) return 'good';
  if (latency < 100) return 'warning';
  return 'critical';
};

const getLossClass = (loss: number): string => {
  if (loss === 0) return 'good';
  if (loss <= 5) return 'warning';
  return 'critical';
};
</script>

<style scoped>
.mtr-summary {
  padding: 0.5rem 0;
}

.summary-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
  flex-wrap: wrap;
  gap: 0.75rem;
}

.summary-stats {
  display: flex;
  gap: 1rem;
  flex-wrap: wrap;
}

.stat-item {
  display: flex;
  flex-direction: column;
  padding: 0.5rem 1rem;
  background: var(--bs-tertiary-bg);
  border-radius: 8px;
  border: 1px solid var(--bs-border-color);
}

.stat-item.warning {
  border-color: var(--bs-warning);
  background: rgba(var(--bs-warning-rgb), 0.1);
}

.stat-item.info {
  border-color: var(--bs-info);
  background: rgba(var(--bs-info-rgb), 0.1);
}

.stat-value {
  font-size: 1.25rem;
  font-weight: 700;
  color: var(--bs-body-color);
}

.stat-item.warning .stat-value { color: var(--bs-warning); }
.stat-item.info .stat-value { color: var(--bs-info); }

.stat-label {
  font-size: 0.7rem;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
}

.no-data {
  text-align: center;
  padding: 2rem;
  color: var(--bs-secondary-color);
}

.no-data i {
  font-size: 2rem;
  margin-bottom: 0.5rem;
  opacity: 0.5;
}

.route-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.route-row {
  background: var(--bs-tertiary-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 8px;
  overflow: hidden;
}

.route-row.has-issues {
  border-color: var(--bs-warning);
}

.route-row.is-primary {
  border-color: var(--bs-success);
}

.route-main {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.75rem 1rem;
  cursor: pointer;
  transition: background 0.15s;
}

.route-main:hover {
  background: var(--bs-secondary-bg);
}

.route-badge {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.3rem 0.7rem;
  border-radius: 6px;
  font-size: 0.8rem;
  font-weight: 500;
  white-space: nowrap;
}

.badge-normal {
  background: var(--bs-primary);
  color: white;
}

.badge-issue {
  background: rgba(var(--bs-warning-rgb), 0.2);
  color: var(--bs-warning);
}

.badge-change {
  background: rgba(var(--bs-info-rgb), 0.2);
  color: var(--bs-info);
}

.route-info {
  flex: 1;
  min-width: 0;
}

.hop-count {
  font-size: 0.75rem;
  color: var(--bs-secondary-color);
  display: block;
}

.route-path {
  font-family: var(--bs-font-monospace);
  font-size: 0.85rem;
  color: var(--bs-primary);
}

.route-path i {
  color: var(--bs-secondary-color);
}

.route-metrics {
  display: flex;
  gap: 1.25rem;
}

.metric {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  min-width: 70px;
}

.metric-value {
  font-size: 0.9rem;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.metric-label {
  font-size: 0.65rem;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
}

.metric.good .metric-value { color: var(--bs-success); }
.metric.warning .metric-value { color: var(--bs-warning); }
.metric.critical .metric-value { color: var(--bs-danger); }

.expand-icon {
  color: var(--bs-secondary-color);
}

.route-expanded {
  border-top: 1px solid var(--bs-border-color);
  padding: 1rem;
  background: var(--bs-body-bg);
}

.trace-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.trace-timestamp {
  font-size: 0.75rem;
  color: var(--bs-secondary-color);
  margin-bottom: 0.25rem;
  padding: 0.25rem 0.5rem;
  background: var(--bs-tertiary-bg);
  border-radius: 4px;
  display: inline-block;
}

.trace-timestamp i {
  opacity: 0.7;
}

.more-traces {
  text-align: center;
  padding: 0.5rem;
}

.pagination-nav {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 1rem;
  margin-top: 1rem;
  padding-top: 1rem;
  border-top: 1px solid var(--bs-border-color);
}

.page-info {
  font-size: 0.8rem;
  color: var(--bs-secondary-color);
}

/* Route Diff Styles */
.route-diff {
  margin-bottom: 1rem;
  padding: 1rem;
  background: var(--bs-tertiary-bg);
  border-radius: 8px;
  border: 1px solid var(--bs-info);
}

.diff-header {
  display: flex;
  align-items: center;
  margin-bottom: 0.75rem;
  color: var(--bs-info);
  font-size: 0.9rem;
}

.diff-time {
  font-size: 0.8rem;
  color: var(--bs-secondary-color);
  font-weight: normal;
}

.diff-content {
  display: flex;
  gap: 1rem;
  align-items: flex-start;
}

.diff-column {
  flex: 1;
  min-width: 0;
}

.diff-label {
  font-size: 0.7rem;
  text-transform: uppercase;
  color: var(--bs-secondary-color);
  margin-bottom: 0.5rem;
  font-weight: 600;
}

.diff-old .diff-label { color: var(--bs-danger); }
.diff-new .diff-label { color: var(--bs-success); }

.diff-hops {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.diff-hop {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.25rem 0.5rem;
  background: var(--bs-body-bg);
  border-radius: 4px;
  font-family: var(--bs-font-monospace);
  font-size: 0.75rem;
}

.diff-hop .hop-num {
  color: var(--bs-secondary-color);
  font-weight: 600;
  min-width: 1.5rem;
}

.diff-hop .hop-ip {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.diff-hop.diff-removed {
  background: rgba(var(--bs-danger-rgb), 0.15);
  color: var(--bs-danger);
}

.diff-hop.diff-added {
  background: rgba(var(--bs-success-rgb), 0.15);
  color: var(--bs-success);
}

.diff-hop.diff-unchanged {
  opacity: 0.6;
}

.diff-arrow {
  display: flex;
  align-items: center;
  justify-content: center;
  padding-top: 1.5rem;
  color: var(--bs-info);
  font-size: 1.25rem;
}

@media (max-width: 768px) {
  .route-main {
    flex-wrap: wrap;
  }
  
  .route-metrics {
    width: 100%;
    justify-content: space-between;
    margin-top: 0.5rem;
  }
  
  .expand-icon {
    display: none;
  }
  
  .diff-content {
    flex-direction: column;
  }
  
  .diff-arrow {
    transform: rotate(90deg);
    padding: 0.5rem 0;
  }
}
</style>
