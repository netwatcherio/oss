# Example: Stats Bar with Circular Progress Rings

## Before (in parent)

```vue
<template>
  <div class="quick-stats">
    <!-- CPU Usage with Circular Ring -->
    <div class="stat-item" :class="{'loading': loadingState.systemInfo}">
      <div class="progress-ring-container">
        <svg class="progress-ring" width="68" height="68">
          <circle class="progress-ring-bg" r="28" cx="34" cy="34" />
          <circle
            class="progress-ring-fill"
            :class="cpuStatusLevel"
            :style="{ strokeDasharray: ringCircumference, strokeDashoffset: cpuRingOffset }"
            r="28" cx="34" cy="34"
          />
        </svg>
        <div class="ring-icon"><i class="bi bi-cpu"></i></div>
      </div>
      <div class="stat-content">
        <div class="stat-value">
          <span v-if="loadingState.systemInfo" class="skeleton-text">--</span>
          <span v-else>{{ cpuUsagePercent }}<small>%</small></span>
        </div>
        <div class="stat-label">CPU Usage</div>
      </div>
    </div>
    
    <!-- Memory, Probes, Uptime stats... -->
  </div>
</template>

<script>
// Computed properties for ring calculations
const ringRadius = 28;
const ringCircumference = 2 * Math.PI * ringRadius;

const cpuRingOffset = computed(() => {
  const value = parseFloat(cpuUsagePercent.value) || 0;
  return ringCircumference - (value / 100) * ringCircumference;
});

const cpuUsagePercent = computed(() => {
  if (loadingState.systemInfo || !systemData?.cpu) return '0.0';
  return ((systemData.cpu.user + systemData.cpu.system) * 100).toFixed(1);
});

const cpuStatusLevel = computed(() => {
  const value = parseFloat(cpuUsagePercent.value) || 0;
  if (value >= 90) return 'critical';
  if (value >= 70) return 'warning';
  return 'healthy';
});
</script>
```

## After (extracted component)

**QuickStatsBar.vue:**
```vue
<script setup lang="ts">
import { computed } from 'vue';

interface Props {
  loadingState: { systemInfo: boolean; probes: boolean };
  cpuUsage: number;
  memoryUsage: number;
  totalProbes: number;
  targetGroupsLength: number;
}

const props = defineProps<Props>();

// Ring calculations - internal to component
const ringCircumference = 2 * Math.PI * 28;

const cpuRingOffset = computed(() => {
  return ringCircumference - (props.cpuUsage / 100) * ringCircumference;
});

const cpuStatusLevel = computed(() => {
  if (props.cpuUsage >= 90) return 'critical';
  if (props.cpuUsage >= 70) return 'warning';
  return 'healthy';
});
</script>

<template>
  <div class="quick-stats">
    <div class="stat-item" :class="{'loading': loadingState.systemInfo}">
      <div class="progress-ring-container">
        <svg class="progress-ring" width="68" height="68">
          <circle class="progress-ring-bg" r="28" cx="34" cy="34" />
          <circle
            class="progress-ring-fill"
            :class="cpuStatusLevel"
            :style="{ strokeDasharray: ringCircumference, strokeDashoffset: cpuRingOffset }"
            r="28" cx="34" cy="34"
          />
        </svg>
        <div class="ring-icon"><i class="bi bi-cpu"></i></div>
      </div>
      <div class="stat-content">
        <div class="stat-value">
          <span v-if="loadingState.systemInfo" class="skeleton-text">--</span>
          <span v-else>{{ cpuUsage }}<small>%</small></span>
        </div>
        <div class="stat-label">CPU Usage</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.quick-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 1rem;
}

.stat-item {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1rem;
  background: white;
  border-radius: 0.75rem;
  border: 1px solid rgba(0, 0, 0, 0.05);
}

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

.progress-ring-fill.healthy { stroke: #198754; }
.progress-ring-fill.warning { stroke: #ffc107; }
.progress-ring-fill.critical { stroke: #dc3545; }

.ring-icon {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  font-size: 1.5rem;
  color: #6c757d;
}

.stat-value {
  font-size: 1.75rem;
  font-weight: 700;
  line-height: 1.2;
}

.skeleton-text {
  display: inline-block;
  width: 2rem;
  height: 1.5rem;
  background: #e9ecef;
  border-radius: 4px;
  animation: skeleton-shimmer 1.5s infinite;
}

@keyframes skeleton-shimmer {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(100%); }
}
</style>
```

## Key Points

1. **Computed logic moved:** Ring offset and status level calculations in component
2. **Simplified props:** Pass raw data (cpuUsage) not computed values
3. **Self-contained:** All SVG ring math in one place
4. **Reusable:** Can be used in any view with similar stats
