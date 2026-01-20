<script lang="ts" setup>
import type { ConnectivityMatrixEntry, ProbeStatusSummary } from '@/types'

defineProps<{
  entry?: ConnectivityMatrixEntry
}>()

// Get status class for a probe
function getStatusClass(status: string): string {
  return `bubble-${status}`
}

// Get probe type abbreviation for tooltip
function getProbeAbbrev(type: string): string {
  switch (type) {
    case 'MTR': return 'M'
    case 'PING': return 'P'
    case 'TRAFFICSIM': return 'T'
    default: return type.charAt(0)
  }
}
</script>

<template>
  <div class="matrix-cell-content" v-if="entry && entry.probe_status.length > 0">
    <div class="status-bubbles">
      <span 
        v-for="probe in entry.probe_status" 
        :key="probe.type"
        class="status-bubble"
        :class="getStatusClass(probe.status)"
        :title="`${probe.type}: ${probe.avg_latency?.toFixed(1) || 0}ms, ${probe.packet_loss?.toFixed(1) || 0}% loss`"
      >
        {{ getProbeAbbrev(probe.type) }}
      </span>
    </div>
  </div>
  <div class="matrix-cell-empty" v-else>
    <span class="no-data">—</span>
  </div>
</template>

<style scoped>
.matrix-cell-content {
  display: flex;
  align-items: center;
  justify-content: center;
}

.status-bubbles {
  display: flex;
  gap: 4px;
}

.status-bubble {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.65rem;
  font-weight: 600;
  color: white;
  cursor: pointer;
  transition: transform 0.15s, box-shadow 0.15s;
}

.status-bubble:hover {
  transform: scale(1.15);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
}

.bubble-healthy {
  background: linear-gradient(135deg, #10b981, #059669);
}

.bubble-degraded {
  background: linear-gradient(135deg, #f59e0b, #d97706);
}

.bubble-critical {
  background: linear-gradient(135deg, #ef4444, #dc2626);
}

.bubble-unknown {
  background: linear-gradient(135deg, #9ca3af, #6b7280);
}

.matrix-cell-empty {
  display: flex;
  align-items: center;
  justify-content: center;
}

.no-data {
  color: var(--bs-secondary-color, #9ca3af);
  font-size: 1rem;
}
</style>
