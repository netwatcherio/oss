<script setup lang="ts">
import { computed } from 'vue';
import type { SysInfoPayload } from '@/types';
import { since } from '@/time';

interface LoadingState {
  agent: boolean;
  workspace: boolean;
  probes: boolean;
  systemInfo: boolean;
  networkInfo: boolean;
}

interface SystemData {
  cpu: {
    idle: number;
    system: number;
    user: number;
  };
  ram: {
    used: number;
    free: number;
    total: number;
  };
}

interface Props {
  loadingState: LoadingState;
  systemInfo: SysInfoPayload;
  systemData: SystemData;
  totalProbes: number;
  targetGroupsLength: number;
}

const props = defineProps<Props>();

// Ring calculations
const ringRadius = 28;
const ringCircumference = 2 * Math.PI * ringRadius;

// CPU calculations
const cpuUsagePercent = computed(() => {
  if (props.loadingState.systemInfo || !props.systemData?.cpu) return '0.0';
  return ((props.systemData.cpu.user + props.systemData.cpu.system) * 100).toFixed(1);
});

const cpuStatusLevel = computed(() => {
  const value = parseFloat(cpuUsagePercent.value) || 0;
  if (value >= 90) return 'critical';
  if (value >= 70) return 'warning';
  return 'healthy';
});

const cpuRingOffset = computed(() => {
  const value = parseFloat(cpuUsagePercent.value) || 0;
  return ringCircumference - (value / 100) * ringCircumference;
});

// Memory calculations
const memoryUsagePercent = computed(() => {
  if (props.loadingState.systemInfo || !props.systemData?.ram) return '0.0';
  return (props.systemData.ram.used * 100).toFixed(1);
});

const memoryStatusLevel = computed(() => {
  const value = parseFloat(memoryUsagePercent.value) || 0;
  if (value >= 90) return 'critical';
  if (value >= 70) return 'warning';
  return 'healthy';
});

const memoryRingOffset = computed(() => {
  const value = parseFloat(memoryUsagePercent.value) || 0;
  return ringCircumference - (value / 100) * ringCircumference;
});

// Uptime
const hasSystemData = computed(() => {
  return props.systemInfo && props.systemInfo.hostInfo && !props.loadingState.systemInfo;
});

// Helper functions
function bytesToString(bytes: number, si: boolean = true, dp: number = 2): string {
  const thresh = si ? 1000 : 1024;

  if (Math.abs(bytes) < thresh) {
    return bytes + ' B';
  }

  const units = si
    ? ['kB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB']
    : ['KiB', 'MiB', 'GiB', 'TiB', 'PiB', 'EiB', 'ZiB', 'YiB'];
  let u = -1;
  const r = 10 ** dp;

  do {
    bytes /= thresh;
    ++u;
  } while (Math.round(Math.abs(bytes) * r) / r >= thresh && u < units.length - 1);

  return bytes.toFixed(dp) + ' ' + units[u];
}
</script>

<template>
  <div class="quick-stats">
    <!-- CPU Usage with Circular Ring -->
    <div 
      class="stat-item glass" 
      :class="{
        'loading': loadingState.systemInfo, 
        [`status-${cpuStatusLevel}`]: !loadingState.systemInfo
      }"
    >
      <div class="progress-ring-container">
        <svg class="progress-ring" width="68" height="68">
          <circle
            class="progress-ring-bg"
            stroke-width="6"
            fill="transparent"
            r="28"
            cx="34"
            cy="34"
          />
          <circle
            class="progress-ring-fill"
            :class="cpuStatusLevel"
            stroke-width="6"
            fill="transparent"
            r="28"
            cx="34"
            cy="34"
            :style="{ 
              strokeDasharray: ringCircumference, 
              strokeDashoffset: loadingState.systemInfo ? ringCircumference : cpuRingOffset 
            }"
          />
        </svg>
        <div class="ring-icon">
          <i class="bi bi-cpu"></i>
        </div>
      </div>
      
      <div class="stat-content">
        <div class="stat-value">
          <span v-if="loadingState.systemInfo" class="skeleton-text">--</span>
          <span v-else>{{ cpuUsagePercent }}<small>%</small></span>
        </div>
        <div class="stat-label">CPU Usage</div>
        <div class="stat-breakdown" v-if="!loadingState.systemInfo && hasSystemData">
          <span>User: {{ (systemData?.cpu?.user * 100).toFixed(0) }}%</span>
          <span>Sys: {{ (systemData?.cpu?.system * 100).toFixed(0) }}%</span>
        </div>
      </div>
    </div>

    <!-- Memory Usage with Circular Ring -->
    <div 
      class="stat-item glass" 
      :class="{
        'loading': loadingState.systemInfo, 
        [`status-${memoryStatusLevel}`]: !loadingState.systemInfo
      }"
    >
      <div class="progress-ring-container">
        <svg class="progress-ring" width="68" height="68">
          <circle
            class="progress-ring-bg"
            stroke-width="6"
            fill="transparent"
            r="28"
            cx="34"
            cy="34"
          />
          <circle
            class="progress-ring-fill"
            :class="memoryStatusLevel"
            stroke-width="6"
            fill="transparent"
            r="28"
            cx="34"
            cy="34"
            :style="{ 
              strokeDasharray: ringCircumference, 
              strokeDashoffset: loadingState.systemInfo ? ringCircumference : memoryRingOffset 
            }"
          />
        </svg>
        <div class="ring-icon">
          <i class="bi bi-memory"></i>
        </div>
      </div>
      
      <div class="stat-content">
        <div class="stat-value">
          <span v-if="loadingState.systemInfo" class="skeleton-text">--</span>
          <span v-else>{{ memoryUsagePercent }}<small>%</small></span>
        </div>
        <div class="stat-label">Memory Usage</div>
        <div class="stat-breakdown" v-if="!loadingState.systemInfo && hasSystemData">
          <span>{{ bytesToString(systemInfo?.memoryInfo?.used_bytes || 0) }}</span>
          <span>of {{ bytesToString(systemInfo?.memoryInfo?.total_bytes || 0) }}</span>
        </div>
      </div>
    </div>

    <!-- Probes -->
    <div class="stat-item glass" :class="{'loading': loadingState.probes}">
      <div class="stat-icon-large probes">
        <i class="bi bi-diagram-3"></i>
      </div>
      <div class="stat-content">
        <div class="stat-value">
          <span v-if="loadingState.probes" class="skeleton-text">-</span>
          <span v-else>{{ totalProbes }}</span>
        </div>
        <div class="stat-label">Probes</div>
        <div class="stat-breakdown" v-if="!loadingState.probes && targetGroupsLength > 0">
          <span>{{ targetGroupsLength }} targets</span>
        </div>
      </div>
    </div>

    <!-- Uptime -->
    <div class="stat-item glass" :class="{'loading': loadingState.systemInfo}">
      <div class="stat-icon-large uptime">
        <i class="bi bi-clock-history"></i>
      </div>
      <div class="stat-content">
        <div class="stat-value">
          <span v-if="loadingState.systemInfo" class="skeleton-text">--</span>
          <span v-else class="uptime-value">
            {{ hasSystemData ? since(systemInfo.hostInfo?.boot_time + "", false) : 'N/A' }}
          </span>
        </div>
        <div class="stat-label">Uptime</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.quick-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 1rem;
  margin-bottom: 1.5rem;
}

.stat-item {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1rem;
  background: white;
  border-radius: 0.75rem;
  border: 1px solid rgba(0, 0, 0, 0.05);
  transition: all 0.2s ease;
}

.stat-item.glass {
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(10px);
  border: 1px solid rgba(255, 255, 255, 0.2);
}

.stat-item:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

/* Progress Ring Styles */
.progress-ring-container {
  position: relative;
  width: 68px;
  height: 68px;
  flex-shrink: 0;
}

.progress-ring {
  transform: rotate(-90deg);
}

.progress-ring-bg {
  stroke: #e9ecef;
}

.progress-ring-fill {
  stroke-linecap: round;
  transition: stroke-dashoffset 0.5s ease;
}

.progress-ring-fill.healthy {
  stroke: #198754;
}

.progress-ring-fill.warning {
  stroke: #ffc107;
}

.progress-ring-fill.critical {
  stroke: #dc3545;
}

.ring-icon {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  font-size: 1.5rem;
  color: #6c757d;
}

/* Stat Content */
.stat-content {
  flex: 1;
  min-width: 0;
}

.stat-value {
  font-size: 1.75rem;
  font-weight: 700;
  line-height: 1.2;
  color: #212529;
}

.stat-value small {
  font-size: 0.875rem;
  font-weight: 500;
  color: #6c757d;
  margin-left: 0.125rem;
}

.stat-label {
  font-size: 0.875rem;
  color: #6c757d;
  margin-top: 0.125rem;
}

.stat-breakdown {
  display: flex;
  gap: 0.75rem;
  margin-top: 0.375rem;
  font-size: 0.75rem;
  color: #adb5bd;
}

/* Status Colors */
.stat-item.status-healthy .stat-value {
  color: #198754;
}

.stat-item.status-warning .stat-value {
  color: #997404;
}

.stat-item.status-critical .stat-value {
  color: #dc3545;
}

/* Icon Large Stats (Probes, Uptime) */
.stat-icon-large {
  width: 68px;
  height: 68px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.75rem;
  flex-shrink: 0;
}

.stat-icon-large.probes {
  background: rgba(13, 110, 253, 0.1);
  color: #0d6efd;
}

.stat-icon-large.uptime {
  background: rgba(25, 135, 84, 0.1);
  color: #198754;
}

/* Loading States */
.skeleton-text {
  display: inline-block;
  width: 2rem;
  height: 1.5rem;
  background: #e9ecef;
  border-radius: 4px;
  position: relative;
  overflow: hidden;
}

.skeleton-text::after {
  content: "";
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  left: 0;
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.4), transparent);
  animation: skeleton-shimmer 1.5s infinite;
}

@keyframes skeleton-shimmer {
  0% {
    transform: translateX(-100%);
  }
  100% {
    transform: translateX(100%);
  }
}

/* Responsive */
@media (max-width: 575px) {
  .quick-stats {
    grid-template-columns: repeat(2, 1fr);
  }

  .stat-item {
    flex-direction: column;
    text-align: center;
    padding: 0.875rem 0.5rem;
  }

  .progress-ring-container {
    width: 56px;
    height: 56px;
  }

  .progress-ring {
    width: 56px;
    height: 56px;
  }

  .progress-ring circle {
    r: 22;
    cx: 28;
    cy: 28;
  }

  .ring-icon {
    font-size: 1.25rem;
  }

  .stat-icon-large {
    width: 56px;
    height: 56px;
    font-size: 1.5rem;
  }

  .stat-value {
    font-size: 1.375rem;
  }

  .stat-breakdown {
    flex-direction: column;
    gap: 0.25rem;
  }
}
</style>
