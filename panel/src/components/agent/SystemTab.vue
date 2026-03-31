<script setup lang="ts">
import { computed } from 'vue';
import type { Agent, SysInfoPayload, NetInfoPayload } from '@/types';
import { since } from '@/time';
import ElementExpand from '@/components/ElementExpand.vue';

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
  loadingState: {
    systemInfo: boolean;
    networkInfo: boolean;
    agent: boolean;
  };
  systemInfo: SysInfoPayload;
  systemData: SystemData;
  networkInfo: NetInfoPayload;
  agent: Agent;
  isOnline: boolean;
  cpuUsagePercent: string;
  memoryUsagePercent: string;
  cpuStatusLevel: 'healthy' | 'warning' | 'critical';
  memoryStatusLevel: 'healthy' | 'warning' | 'critical';
}

const props = defineProps<Props>();

const hasSystemData = computed(() => {
  return props.systemInfo && props.systemInfo.hostInfo && !props.loadingState.systemInfo;
});

const hasNetworkData = computed(() => {
  return props.networkInfo && props.networkInfo.public_address && !props.loadingState.networkInfo;
});

function getOsIcon(osName?: string): string {
  if (!osName) return 'bi-display';
  const lower = osName.toLowerCase();
  if (lower.includes('windows')) return 'bi-windows';
  if (lower.includes('mac') || lower.includes('darwin')) return 'bi-apple';
  if (lower.includes('linux') || lower.includes('ubuntu') || lower.includes('debian') || lower.includes('centos') || lower.includes('fedora')) return 'bi-ubuntu';
  return 'bi-display';
}

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

function formatSnakeCaseToHumanCase(name: string): string {
  let words = name.split("_");
  words = words.filter(w => w != "bytes");
  words = words.map(w => w[0].toUpperCase() + w.substring(1));
  return words.join(" ");
}
</script>

<template>
  <div class="tab-panel">
    <div class="info-grid">
      <!-- System Resources - Enhanced -->
      <div class="info-card enhanced" :class="{'loading': loadingState.systemInfo}">
        <div class="card-header">
          <h5 class="card-title">
            <i class="bi bi-speedometer"></i>
            System Resources
          </h5>
          <div class="refresh-indicator" v-if="hasSystemData">
            <i class="bi bi-clock"></i>
            <span>{{ since(systemInfo.timestamp + "", true) }}</span>
          </div>
        </div>
        <div class="card-content">
          <!-- CPU Meter -->
          <div class="resource-meter enhanced" :class="cpuStatusLevel">
            <div class="resource-header">
              <div class="resource-label">
                <i class="bi bi-cpu"></i>
                <span>CPU Usage</span>
              </div>
              <div class="resource-value">
                <span v-if="loadingState.systemInfo" class="skeleton-text">--%</span>
                <span v-else :class="`status-${cpuStatusLevel}`">{{ cpuUsagePercent }}%</span>
              </div>
            </div>
            <div class="progress-bar-container">
              <div class="progress gradient">
                <div 
                  class="progress-bar" 
                  :class="cpuStatusLevel"
                  :style="{width: loadingState.systemInfo ? '0%' : cpuUsagePercent + '%'}"
                ></div>
              </div>
            </div>
            <div class="resource-breakdown" v-if="!loadingState.systemInfo && hasSystemData">
              <div class="breakdown-item">
                <span class="breakdown-label">User</span>
                <span class="breakdown-value">{{ (systemData?.cpu?.user * 100).toFixed(1) }}%</span>
              </div>
              <div class="breakdown-item">
                <span class="breakdown-label">System</span>
                <span class="breakdown-value">{{ (systemData?.cpu?.system * 100).toFixed(1) }}%</span>
              </div>
              <div class="breakdown-item">
                <span class="breakdown-label">Idle</span>
                <span class="breakdown-value">{{ (systemData?.cpu?.idle * 100).toFixed(1) }}%</span>
              </div>
            </div>
          </div>

          <!-- Memory Meter -->
          <div class="resource-meter enhanced" :class="memoryStatusLevel">
            <div class="resource-header">
              <div class="resource-label">
                <i class="bi bi-memory"></i>
                <span>Memory Usage</span>
              </div>
              <div class="resource-value">
                <span v-if="loadingState.systemInfo" class="skeleton-text">--%</span>
                <span v-else :class="`status-${memoryStatusLevel}`">{{ memoryUsagePercent }}%</span>
              </div>
            </div>
            <div class="progress-bar-container">
              <div class="progress gradient">
                <div 
                  class="progress-bar" 
                  :class="memoryStatusLevel"
                  :style="{width: loadingState.systemInfo ? '0%' : memoryUsagePercent + '%'}"
                ></div>
              </div>
            </div>
            <div class="resource-breakdown" v-if="!loadingState.systemInfo && hasSystemData">
              <div class="breakdown-item">
                <span class="breakdown-label">Used</span>
                <span class="breakdown-value">{{ bytesToString(systemInfo.memoryInfo?.used_bytes || 0) }}</span>
              </div>
              <div class="breakdown-item">
                <span class="breakdown-label">Available</span>
                <span class="breakdown-value">{{ bytesToString(systemInfo.memoryInfo?.available_bytes || 0) }}</span>
              </div>
              <div class="breakdown-item">
                <span class="breakdown-label">Total</span>
                <span class="breakdown-value">{{ bytesToString(systemInfo.memoryInfo?.total_bytes || 0) }}</span>
              </div>
            </div>
          </div>

          <ElementExpand title="Memory Details" code :disabled="loadingState.systemInfo || !hasSystemData">
            <template v-slot:expanded>
              <div class="memory-details">
                <div v-if="loadingState.systemInfo" v-for="i in 4" :key="`mem-skeleton-${i}`" class="detail-row">
                  <span class="skeleton-text">--------------</span>
                  <span class="skeleton-text">--- GB</span>
                </div>
                <div v-else-if="hasSystemData && systemInfo.memoryInfo?.raw" v-for="(value, key) in systemInfo.memoryInfo.raw" :key="key" class="detail-row">
                  <span>{{ formatSnakeCaseToHumanCase(key) }}</span>
                  <span>{{ bytesToString(value) }}</span>
                </div>
                <div v-else class="text-muted">No memory details available</div>
              </div>
            </template>
          </ElementExpand>
        </div>
      </div>

      <!-- System Information - Enhanced -->
      <div class="info-card enhanced" :class="{'loading': loadingState.systemInfo}">
        <div class="card-header">
          <h5 class="card-title">
            <i :class="hasSystemData ? getOsIcon(systemInfo.hostInfo?.os?.name) : 'bi bi-display'"></i>
            System Information
          </h5>
        </div>
        <div class="card-content">
          <!-- OS Info with Icon -->
          <div class="info-row os-info">
            <span class="info-label"><i class="bi bi-pc-display-horizontal"></i> Operating System</span>
            <div class="info-value os-value">
              <span v-if="loadingState.systemInfo" class="skeleton-text">-------------------------</span>
              <template v-else-if="hasSystemData">
                <i :class="getOsIcon(systemInfo.hostInfo?.os?.name)" class="os-icon"></i>
                <span>
                  {{ systemInfo.hostInfo?.os?.name || 'Unknown' }}
                  <small v-if="systemInfo.hostInfo?.os?.version">{{ systemInfo.hostInfo?.os?.version }}</small>
                </span>
              </template>
              <span v-else>Unknown</span>
            </div>
          </div>
          
          <div class="info-row">
            <span class="info-label"><i class="bi bi-cpu"></i> Architecture</span>
            <span class="info-value">
              <span v-if="loadingState.systemInfo" class="skeleton-text">-----------</span>
              <span v-else class="arch-badge">{{ systemInfo.hostInfo?.architecture || 'Unknown' }}</span>
            </span>
          </div>
          
          <div class="info-row">
            <span class="info-label"><i class="bi bi-box"></i> Environment</span>
            <div class="info-value">
              <span v-if="loadingState.systemInfo" class="skeleton-text">-----------</span>
              <template v-else-if="hasSystemData">
                <span class="env-badge" :class="systemInfo.hostInfo?.containerized ? 'virtual' : 'physical'">
                  <i :class="systemInfo.hostInfo?.containerized ? 'bi bi-box-seam' : 'bi bi-motherboard'"></i>
                  {{ systemInfo.hostInfo?.containerized ? 'Virtualized' : 'Physical' }}
                </span>
              </template>
              <span v-else>Unknown</span>
            </div>
          </div>
          
          <div class="info-row">
            <span class="info-label"><i class="bi bi-clock"></i> Timezone</span>
            <span class="info-value">
              <span v-if="loadingState.systemInfo" class="skeleton-text">-------------------</span>
              <span v-else>{{ systemInfo.hostInfo?.timezone || 'Unknown' }}</span>
            </span>
          </div>
          
          <div class="info-row">
            <span class="info-label"><i class="bi bi-geo-alt"></i> Location</span>
            <span class="info-value location-value">
              <span v-if="loadingState.networkInfo" class="skeleton-text">------------------</span>
              <template v-else-if="hasNetworkData && networkInfo.lat && networkInfo.long">
                <a 
                  :href="`https://maps.google.com/?q=${networkInfo.lat},${networkInfo.long}`" 
                  target="_blank" 
                  class="location-link"
                  title="View on Google Maps"
                >
                  <i class="bi bi-pin-map"></i>
                  {{ parseFloat(String(networkInfo.lat)).toFixed(4) }}, {{ parseFloat(String(networkInfo.long)).toFixed(4) }}
                </a>
              </template>
              <span v-else>Unknown</span>
            </span>
          </div>
          
          <div class="info-row">
            <span class="info-label"><i class="bi bi-eye"></i> Last Seen</span>
            <span class="info-value">
              <span v-if="loadingState.agent" class="skeleton-text">------------</span>
              <span v-else :class="isOnline ? 'text-success' : 'text-muted'">
                {{ agent.updated_at ? since(agent.updated_at, true) : 'Never' }}
              </span>
            </span>
          </div>
        </div>
        <div class="card-footer subtle" v-if="hasSystemData">
          <div class="footer-info">
            <i class="bi bi-info-circle"></i>
            <span>System data from {{ since(systemInfo.timestamp + "", true) }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
