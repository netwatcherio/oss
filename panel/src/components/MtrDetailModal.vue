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
                v-for="(result, index) in filteredResults" 
                :key="result.id || index" 
                class="traceroute-item"
                :class="{ 'route-changed': hasRouteChange(index) }"
              >
                <div class="traceroute-header">
                  <span class="traceroute-time">
                    <i class="bi bi-clock"></i>
                    {{ formatTimestamp(result) }}
                  </span>
                  <span v-if="hasRouteChange(index)" class="route-change-badge">
                    <i class="bi bi-shuffle"></i> Route Changed
                  </span>
                </div>
                <MtrTable :probe-data="result" :show-copy="true" />
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script lang="ts" setup>
import { computed, watch } from 'vue';
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
  
  const current = getRouteSignature(filteredResults.value[index]);
  const previous = getRouteSignature(filteredResults.value[index + 1]);
  
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

// Time range display
const timeRange = computed(() => {
  if (filteredResults.value.length === 0) return '-';
  
  const oldest = filteredResults.value[filteredResults.value.length - 1];
  const newest = filteredResults.value[0];
  
  const oldestTime = new Date(oldest.payload?.stop_timestamp || oldest.created_at);
  const newestTime = new Date(newest.payload?.stop_timestamp || newest.created_at);
  
  const diffMs = newestTime.getTime() - oldestTime.getTime();
  const diffHours = Math.round(diffMs / (1000 * 60 * 60));
  
  if (diffHours < 1) return '< 1 hour';
  if (diffHours < 24) return `${diffHours} hours`;
  return `${Math.round(diffHours / 24)} days`;
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
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1050;
  padding: 2rem;
}

.mtr-modal {
  background: #1e1e2e;
  border-radius: 12px;
  width: 100%;
  max-width: 1000px;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
}

.mtr-modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1rem 1.5rem;
  border-bottom: 1px solid #313244;
}

.mtr-modal-title {
  margin: 0;
  color: #cdd6f4;
  font-size: 1.1rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.node-badge {
  background: #45475a;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  font-size: 0.85rem;
  font-weight: normal;
}

.close-btn {
  background: transparent;
  border: none;
  color: #6c7086;
  font-size: 1.2rem;
  cursor: pointer;
  padding: 0.5rem;
  border-radius: 4px;
  transition: all 0.2s;
}

.close-btn:hover {
  background: #313244;
  color: #cdd6f4;
}

.mtr-modal-body {
  flex: 1;
  overflow-y: auto;
  padding: 1rem 1.5rem;
}

.no-data {
  text-align: center;
  padding: 3rem;
  color: #6c7086;
}

.no-data i {
  font-size: 3rem;
  margin-bottom: 1rem;
}

.summary-bar {
  display: flex;
  gap: 1.5rem;
  margin-bottom: 1rem;
  padding: 1rem;
  background: #2a2a3e;
  border-radius: 8px;
}

.summary-stat {
  display: flex;
  flex-direction: column;
}

.summary-stat.warning .stat-value {
  color: #fab387;
}

.stat-value {
  font-size: 1.25rem;
  font-weight: 600;
  color: #cdd6f4;
}

.stat-label {
  font-size: 0.75rem;
  color: #6c7086;
  text-transform: uppercase;
}

.route-change-alert {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem 1rem;
  background: rgba(250, 179, 135, 0.15);
  border: 1px solid rgba(250, 179, 135, 0.3);
  border-radius: 6px;
  color: #fab387;
  margin-bottom: 1rem;
  font-size: 0.9rem;
}

.traceroute-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.traceroute-item {
  border: 1px solid #313244;
  border-radius: 8px;
  overflow: hidden;
}

.traceroute-item.route-changed {
  border-color: rgba(250, 179, 135, 0.5);
  box-shadow: 0 0 0 1px rgba(250, 179, 135, 0.2);
}

.traceroute-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.5rem 1rem;
  background: #2a2a3e;
  border-bottom: 1px solid #313244;
}

.traceroute-time {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  color: #a6adc8;
  font-size: 0.85rem;
}

.route-change-badge {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.25rem 0.5rem;
  background: rgba(250, 179, 135, 0.2);
  color: #fab387;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 500;
}
</style>
