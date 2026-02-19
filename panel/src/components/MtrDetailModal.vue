<template>
  <Teleport to="body">
    <div v-if="visible" class="mtr-modal-overlay" @click.self="close">
      <div class="mtr-modal">
        <div class="mtr-modal-header">
          <h5 class="mtr-modal-title">
            <i class="bi bi-diagram-3"></i>
            Traceroute Details
            <span v-if="node" class="node-badge">{{ node.hostname || node.ip || `Hop ${node.hopNumber}` }}</span>
          </h5>
          <button @click="close" class="close-btn" aria-label="Close">
            <i class="bi bi-x-lg"></i>
          </button>
        </div>
        
        <div class="mtr-modal-body">
          <div v-if="filteredResults.length === 0" class="no-data">
            <i class="bi bi-inbox"></i>
            <p>No traceroute data available</p>
          </div>
          
          <template v-else>
            <!-- Summary stats -->
            <div class="summary-bar">
              <div class="summary-stat">
                <span class="stat-value">{{ filteredResults.length }}</span>
                <span class="stat-label">Traceroutes</span>
              </div>
              <div v-if="routeChanges > 0" class="summary-stat warning">
                <span class="stat-value">{{ routeChanges }}</span>
                <span class="stat-label">Route Changes</span>
              </div>
              <div class="summary-stat">
                <span class="stat-value">{{ timeRange }}</span>
                <span class="stat-label">Time Range</span>
              </div>
            </div>
            
            <!-- Route change indicator -->
            <div v-if="routeChanges > 0" class="route-change-alert">
              <i class="bi bi-exclamation-triangle"></i>
              <span>Route changes detected! Paths that differ from the previous trace are highlighted.</span>
            </div>
            
            <!-- Traceroute list -->
            <div class="traceroute-list">
              <div 
                v-for="(result, index) in paginatedResults" 
                :key="result.id || index" 
                class="traceroute-item"
                :class="{ 'route-changed': hasRouteChange((currentPage - 1) * pageSize + index) }"
              >
                <div class="traceroute-header">
                  <span class="traceroute-time">
                    <i class="bi bi-clock"></i>
                    {{ formatTimestamp(result) }}
                  </span>
                  <span v-if="hasRouteChange((currentPage - 1) * pageSize + index)" class="route-change-badge">
                    <i class="bi bi-shuffle"></i> Route Changed
                  </span>
                </div>
                
                <!-- Route Diff Display -->
                <div v-if="hasRouteChange((currentPage - 1) * pageSize + index)" class="route-diff-container">
                  <div class="route-diff-header" @click="toggleDiff((currentPage - 1) * pageSize + index)">
                    <i :class="expandedDiffs[(currentPage - 1) * pageSize + index] ? 'bi bi-chevron-down' : 'bi bi-chevron-right'"></i>
                    Show Route Difference
                  </div>
                  <div v-if="expandedDiffs[(currentPage - 1) * pageSize + index]" class="route-diff-content">
                    <div class="route-diff-columns">
                      <div class="route-column previous">
                        <div class="column-header">
                          <i class="bi bi-arrow-left-circle"></i> Previous Route
                        </div>
                        <div class="hop-list">
                          <div 
                            v-for="(hop, hopIdx) in getPreviousHops((currentPage - 1) * pageSize + index)" 
                            :key="hopIdx"
                            class="hop-item"
                            :class="getHopDiffClass((currentPage - 1) * pageSize + index, hopIdx, 'previous')"
                          >
                            <span class="hop-number">{{ hopIdx + 1 }}</span>
                            <span class="hop-ip">{{ hop }}</span>
                          </div>
                        </div>
                      </div>
                      <div class="route-column current">
                        <div class="column-header">
                          <i class="bi bi-arrow-right-circle"></i> Current Route
                        </div>
                        <div class="hop-list">
                          <div 
                            v-for="(hop, hopIdx) in getCurrentHops((currentPage - 1) * pageSize + index)" 
                            :key="hopIdx"
                            class="hop-item"
                            :class="getHopDiffClass((currentPage - 1) * pageSize + index, hopIdx, 'current')"
                          >
                            <span class="hop-number">{{ hopIdx + 1 }}</span>
                            <span class="hop-ip">{{ hop }}</span>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
                
                <MtrTable :probe-data="result" :show-copy="true" />
              </div>
            </div>
            
            <!-- Pagination Controls -->
            <nav v-if="totalPages > 1" class="mt-3">
              <ul class="pagination justify-content-center mb-0">
                <li class="page-item" :class="{ disabled: currentPage === 1 }">
                  <button class="page-link" @click="currentPage--">
                    <i class="bi bi-chevron-left"></i>
                  </button>
                </li>
                <li v-for="p in displayedPages" :key="p" 
                    class="page-item" :class="{ active: p === currentPage }">
                  <button class="page-link" @click="currentPage = p">{{ p }}</button>
                </li>
                <li class="page-item" :class="{ disabled: currentPage === totalPages }">
                  <button class="page-link" @click="currentPage++">
                    <i class="bi bi-chevron-right"></i>
                  </button>
                </li>
              </ul>
              <div class="text-center mt-2 text-muted" style="font-size: 0.85rem;">
                Showing {{ (currentPage - 1) * pageSize + 1 }}-{{ Math.min(currentPage * pageSize, filteredResults.length) }} of {{ filteredResults.length }}
              </div>
            </nav>
          </template>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script lang="ts" setup>
import { computed, watch, ref } from 'vue';
import type { ProbeData, MtrResult } from '@/types';
import MtrTable from '@/components/MtrTable.vue';

interface NodeData {
  id: string;
  hostname?: string;
  ip?: string;
  hopNumber: number;
}

const props = defineProps<{
  visible: boolean;
  node?: NodeData | null;
  mtrResults: ProbeData[];
}>();

const emit = defineEmits<{
  (e: 'close'): void;
}>();

const close = () => emit('close');

// Pagination state
const currentPage = ref(1);
const pageSize = 10;

const totalPages = computed(() => Math.ceil(filteredResults.value.length / pageSize));

const paginatedResults = computed(() => {
  const start = (currentPage.value - 1) * pageSize;
  const end = start + pageSize;
  return filteredResults.value.slice(start, end);
});

// Show limited page numbers (e.g., 1 2 3 ... 8 9 10)
const displayedPages = computed(() => {
  const total = totalPages.value;
  const current = currentPage.value;
  const pages: number[] = [];
  
  if (total <= 7) {
    for (let i = 1; i <= total; i++) pages.push(i);
  } else {
    if (current <= 3) {
      pages.push(1, 2, 3, 4, 5);
    } else if (current >= total - 2) {
      for (let i = total - 4; i <= total; i++) pages.push(i);
    } else {
      for (let i = current - 2; i <= current + 2; i++) pages.push(i);
    }
  }
  
  return pages;
});

// Reset page when modal opens or data changes
watch(() => props.visible, (visible) => {
  if (visible) currentPage.value = 1;
});

watch(() => props.mtrResults, () => {
  currentPage.value = 1;
});

// Close on escape key
watch(() => props.visible, (isVisible) => {
  if (isVisible) {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close();
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }
});

// Filter results based on selected node (if any)
const filteredResults = computed(() => {
  if (!props.mtrResults || props.mtrResults.length === 0) return [];
  
  // Sort by timestamp, newest first
  const sorted = [...props.mtrResults].sort((a, b) => {
    const timeA = new Date(a.payload?.stop_timestamp || a.created_at).getTime();
    const timeB = new Date(b.payload?.stop_timestamp || b.created_at).getTime();
    return timeB - timeA;
  });
  
  // If a node is selected, filter to those containing that node
  if (props.node && props.node.ip) {
    return sorted.filter(result => {
      const payload = result.payload as MtrResult;
      return payload?.report?.hops?.some(hop => 
        hop.hosts?.some(host => host.ip === props.node?.ip)
      );
    });
  }
  
  return sorted;
});

// Compute route signature for comparison
const getRouteSignature = (result: ProbeData): string => {
  const payload = result.payload as MtrResult;
  if (!payload?.report?.hops) return '';
  
  return payload.report.hops
    .map(hop => hop.hosts?.[0]?.ip || '*')
    .join(' -> ');
};

// Check if route changed from previous
const hasRouteChange = (index: number): boolean => {
  if (index === filteredResults.value.length - 1) return false; // First (oldest) trace has no previous
  
  const currentResult = filteredResults.value[index];
  const previousResult = filteredResults.value[index + 1];
  if (!currentResult || !previousResult) return false;
  
  const current = getRouteSignature(currentResult);
  const previous = getRouteSignature(previousResult);
  
  return current !== previous;
};

// Count route changes
const routeChanges = computed(() => {
  let changes = 0;
  for (let i = 0; i < filteredResults.value.length - 1; i++) {
    if (hasRouteChange(i)) changes++;
  }
  return changes;
});

// Track which diffs are expanded
const expandedDiffs = ref<Record<number, boolean>>({});

const toggleDiff = (index: number) => {
  expandedDiffs.value[index] = !expandedDiffs.value[index];
};

// Get hop IPs as array
const getHopsArray = (result: ProbeData): string[] => {
  const payload = result.payload as MtrResult;
  if (!payload?.report?.hops) return [];
  return payload.report.hops.map(hop => hop.hosts?.[0]?.ip || '*');
};

// Get previous route hops
const getPreviousHops = (index: number): string[] => {
  const previousResult = filteredResults.value[index + 1];
  return previousResult ? getHopsArray(previousResult) : [];
};

// Get current route hops
const getCurrentHops = (index: number): string[] => {
  const currentResult = filteredResults.value[index];
  return currentResult ? getHopsArray(currentResult) : [];
};

// Get CSS class for hop diff highlighting
const getHopDiffClass = (index: number, hopIdx: number, side: 'previous' | 'current'): string => {
  const previousHops = getPreviousHops(index);
  const currentHops = getCurrentHops(index);
  
  if (side === 'previous') {
    const hop = previousHops[hopIdx];
    if (!hop) return '';
    // If this hop doesn't exist in current route at same position, mark as removed
    if (currentHops[hopIdx] !== hop) {
      if (!currentHops.includes(hop)) {
        return 'hop-removed';
      }
      return 'hop-changed';
    }
  } else {
    const hop = currentHops[hopIdx];
    if (!hop) return '';
    // If this hop doesn't exist in previous route at same position, mark as added
    if (previousHops[hopIdx] !== hop) {
      if (!previousHops.includes(hop)) {
        return 'hop-added';
      }
      return 'hop-changed';
    }
  }
  return '';
};

// Time range display - show actual date range
const timeRange = computed(() => {
  if (filteredResults.value.length === 0) return '-';
  
  const oldest = filteredResults.value[filteredResults.value.length - 1];
  const newest = filteredResults.value[0];
  
  if (!oldest || !newest) return '-';
  
  const oldestTime = new Date(oldest.payload?.stop_timestamp || oldest.created_at);
  const newestTime = new Date(newest.payload?.stop_timestamp || newest.created_at);
  
  // Format as "Jan 5 - Jan 8" or "Jan 5, 10:30 - 14:45" for same day
  const formatDate = (d: Date) => d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
  const formatTime = (d: Date) => d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
  
  const sameDay = oldestTime.toDateString() === newestTime.toDateString();
  
  if (sameDay) {
    return `${formatDate(oldestTime)}, ${formatTime(oldestTime)} - ${formatTime(newestTime)}`;
  } else {
    return `${formatDate(oldestTime)} - ${formatDate(newestTime)}`;
  }
});

const formatTimestamp = (result: ProbeData): string => {
  const timestamp = result.payload?.stop_timestamp || result.created_at;
  return new Date(timestamp).toLocaleString();
};
</script>

<style scoped>
.mtr-modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.75);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1050;
  padding: 2rem;
}

.mtr-modal {
  background: linear-gradient(180deg, #1a1b26 0%, #16161e 100%);
  border-radius: 16px;
  width: 100%;
  max-width: 1100px;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 25px 80px -12px rgba(0, 0, 0, 0.6), 0 0 0 1px rgba(122, 162, 247, 0.1);
  border: 1px solid #2a2b3d;
}

.mtr-modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1.25rem 1.5rem;
  background: linear-gradient(135deg, #1e1f2e 0%, #252636 100%);
  border-bottom: 1px solid #2a2b3d;
  border-radius: 16px 16px 0 0;
}

.mtr-modal-title {
  margin: 0;
  color: #c0caf5;
  font-size: 1.15rem;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.mtr-modal-title i {
  color: #7aa2f7;
}

.node-badge {
  background: linear-gradient(135deg, #3d59a1 0%, #2a3f73 100%);
  padding: 0.35rem 0.75rem;
  border-radius: 6px;
  font-size: 0.85rem;
  font-weight: 500;
  color: #c0caf5;
}

.close-btn {
  background: transparent;
  border: 1px solid transparent;
  color: #565f89;
  font-size: 1.25rem;
  cursor: pointer;
  padding: 0.5rem;
  border-radius: 8px;
  transition: all 0.2s;
  display: flex;
  align-items: center;
  justify-content: center;
}

.close-btn:hover {
  background: rgba(247, 118, 142, 0.1);
  border-color: rgba(247, 118, 142, 0.2);
  color: #f7768e;
}

.mtr-modal-body {
  flex: 1;
  overflow-y: auto;
  padding: 1.25rem 1.5rem;
}

.no-data {
  text-align: center;
  padding: 4rem 2rem;
  color: #565f89;
}

.no-data i {
  font-size: 3.5rem;
  margin-bottom: 1rem;
  opacity: 0.5;
}

.no-data p {
  font-size: 1rem;
  margin: 0;
}

.summary-bar {
  display: flex;
  gap: 2rem;
  margin-bottom: 1.25rem;
  padding: 1.25rem 1.5rem;
  background: linear-gradient(135deg, #1e1f2e 0%, #252636 100%);
  border-radius: 12px;
  border: 1px solid #2a2b3d;
}

.summary-stat {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.summary-stat.warning .stat-value {
  color: #ff9e64;
}

.stat-value {
  font-size: 1.5rem;
  font-weight: 700;
  color: #c0caf5;
  font-variant-numeric: tabular-nums;
}

.stat-label {
  font-size: 0.7rem;
  color: #565f89;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  font-weight: 500;
}

.route-change-alert {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.875rem 1.25rem;
  background: rgba(255, 158, 100, 0.08);
  border: 1px solid rgba(255, 158, 100, 0.2);
  border-radius: 10px;
  color: #ff9e64;
  margin-bottom: 1.25rem;
  font-size: 0.9rem;
}

.route-change-alert i {
  font-size: 1.1rem;
}

.traceroute-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.traceroute-item {
  border: 1px solid #2a2b3d;
  border-radius: 12px;
  overflow: hidden;
  transition: all 0.2s;
}

.traceroute-item:hover {
  border-color: #3d59a1;
}

.traceroute-item.route-changed {
  border-color: rgba(255, 158, 100, 0.4);
  box-shadow: 0 0 0 1px rgba(255, 158, 100, 0.1), inset 0 0 20px rgba(255, 158, 100, 0.02);
}

.traceroute-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.65rem 1.25rem;
  background: linear-gradient(135deg, #1e1f2e 0%, #252636 100%);
  border-bottom: 1px solid #2a2b3d;
}

.traceroute-time {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: #7aa2f7;
  font-size: 0.85rem;
  font-weight: 500;
}

.traceroute-time i {
  opacity: 0.7;
}

.route-change-badge {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.3rem 0.65rem;
  background: rgba(255, 158, 100, 0.15);
  color: #ff9e64;
  border-radius: 6px;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}

/* Route Diff Display */
.route-diff-container {
  margin: 0.75rem 1rem;
  border: 1px solid #3d3e50;
  border-radius: 8px;
  overflow: hidden;
}

.route-diff-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.6rem 1rem;
  background: #252636;
  color: #7aa2f7;
  cursor: pointer;
  font-size: 0.85rem;
  font-weight: 500;
  transition: background 0.2s;
}

.route-diff-header:hover {
  background: #2d2e40;
}

.route-diff-content {
  padding: 1rem;
  background: #1a1b26;
}

.route-diff-columns {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1rem;
}

.route-column {
  background: #1e1f2e;
  border-radius: 6px;
  overflow: hidden;
}

.column-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.6rem 0.8rem;
  font-size: 0.8rem;
  font-weight: 600;
  text-transform: uppercase;
}

.route-column.previous .column-header {
  background: rgba(247, 118, 142, 0.1);
  color: #f7768e;
}

.route-column.current .column-header {
  background: rgba(158, 206, 106, 0.1);
  color: #9ece6a;
}

.hop-list {
  padding: 0.5rem;
}

.hop-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.4rem 0.6rem;
  border-radius: 4px;
  font-size: 0.8rem;
  font-family: 'Monaco', 'Menlo', monospace;
}

.hop-item:hover {
  background: rgba(255, 255, 255, 0.05);
}

.hop-number {
  min-width: 20px;
  color: #565f89;
  font-weight: 600;
}

.hop-ip {
  color: #c0caf5;
}

/* Diff highlighting */
.hop-added {
  background: rgba(158, 206, 106, 0.15);
  border-left: 3px solid #9ece6a;
}

.hop-added .hop-ip {
  color: #9ece6a;
}

.hop-removed {
  background: rgba(247, 118, 142, 0.15);
  border-left: 3px solid #f7768e;
}

.hop-removed .hop-ip {
  color: #f7768e;
}

.hop-changed {
  background: rgba(224, 175, 104, 0.15);
  border-left: 3px solid #e0af68;
}

.hop-changed .hop-ip {
  color: #e0af68;
}

/* ==================== LIGHT THEME ==================== */
[data-theme="light"] .mtr-modal {
  background: linear-gradient(180deg, #ffffff 0%, #f9fafb 100%);
  border-color: #e5e7eb;
  box-shadow: 0 25px 80px -12px rgba(0, 0, 0, 0.15), 0 0 0 1px rgba(0, 0, 0, 0.05);
}

[data-theme="light"] .mtr-modal-header {
  background: linear-gradient(135deg, #f9fafb 0%, #f3f4f6 100%);
  border-bottom-color: #e5e7eb;
}

[data-theme="light"] .mtr-modal-title {
  color: #1f2937;
}

[data-theme="light"] .mtr-modal-title i {
  color: #3b82f6;
}

[data-theme="light"] .node-badge {
  background: linear-gradient(135deg, #dbeafe 0%, #bfdbfe 100%);
  color: #1e40af;
}

[data-theme="light"] .close-btn {
  color: #9ca3af;
}

[data-theme="light"] .close-btn:hover {
  background: rgba(239, 68, 68, 0.06);
  border-color: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

[data-theme="light"] .no-data {
  color: #9ca3af;
}

[data-theme="light"] .summary-bar {
  background: linear-gradient(135deg, #f9fafb 0%, #f3f4f6 100%);
  border-color: #e5e7eb;
}

[data-theme="light"] .stat-value {
  color: #1f2937;
}

[data-theme="light"] .stat-label {
  color: #6b7280;
}

[data-theme="light"] .route-change-alert {
  background: rgba(245, 158, 11, 0.08);
  border-color: rgba(245, 158, 11, 0.2);
  color: #b45309;
}

[data-theme="light"] .traceroute-item {
  border-color: #e5e7eb;
}

[data-theme="light"] .traceroute-item:hover {
  border-color: #93c5fd;
}

[data-theme="light"] .traceroute-item.route-changed {
  border-color: rgba(245, 158, 11, 0.3);
  box-shadow: 0 0 0 1px rgba(245, 158, 11, 0.08);
}

[data-theme="light"] .traceroute-header {
  background: linear-gradient(135deg, #f9fafb 0%, #f3f4f6 100%);
  border-bottom-color: #e5e7eb;
}

[data-theme="light"] .traceroute-time {
  color: #2563eb;
}

[data-theme="light"] .route-change-badge {
  background: rgba(245, 158, 11, 0.1);
  color: #b45309;
}

[data-theme="light"] .route-diff-container {
  border-color: #d1d5db;
}

[data-theme="light"] .route-diff-header {
  background: #f3f4f6;
  color: #2563eb;
}

[data-theme="light"] .route-diff-header:hover {
  background: #e5e7eb;
}

[data-theme="light"] .route-diff-content {
  background: #ffffff;
}

[data-theme="light"] .route-column {
  background: #f9fafb;
}

[data-theme="light"] .route-column.previous .column-header {
  background: rgba(239, 68, 68, 0.06);
  color: #dc2626;
}

[data-theme="light"] .route-column.current .column-header {
  background: rgba(34, 197, 94, 0.06);
  color: #16a34a;
}

[data-theme="light"] .hop-item:hover {
  background: rgba(0, 0, 0, 0.03);
}

[data-theme="light"] .hop-number {
  color: #9ca3af;
}

[data-theme="light"] .hop-ip {
  color: #1f2937;
}

[data-theme="light"] .hop-added {
  background: rgba(34, 197, 94, 0.08);
  border-left-color: #16a34a;
}

[data-theme="light"] .hop-added .hop-ip {
  color: #16a34a;
}

[data-theme="light"] .hop-removed {
  background: rgba(239, 68, 68, 0.08);
  border-left-color: #dc2626;
}

[data-theme="light"] .hop-removed .hop-ip {
  color: #dc2626;
}

[data-theme="light"] .hop-changed {
  background: rgba(245, 158, 11, 0.08);
  border-left-color: #d97706;
}

[data-theme="light"] .hop-changed .hop-ip {
  color: #d97706;
}

/* ==================== MOBILE RESPONSIVE ==================== */
@media (max-width: 768px) {
  .mtr-modal-overlay {
    padding: 0.5rem;
    align-items: flex-end;
  }

  .mtr-modal {
    max-width: 100%;
    max-height: 90vh;
    border-radius: 12px 12px 0 0;
  }

  .mtr-modal-header {
    padding: 1rem;
  }

  .mtr-modal-title {
    font-size: 1rem;
  }

  .mtr-modal-body {
    padding: 0.75rem 1rem;
  }

  .summary-bar {
    gap: 1rem;
    padding: 0.875rem 1rem;
    flex-wrap: wrap;
  }

  .stat-value {
    font-size: 1.15rem;
  }

  .route-diff-columns {
    grid-template-columns: 1fr;
  }

  .traceroute-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 0.35rem;
    padding: 0.5rem 0.75rem;
  }

  .pagination .page-link {
    padding: 0.5rem 0.75rem;
    min-width: 2.5rem;
    text-align: center;
  }
}
</style>

