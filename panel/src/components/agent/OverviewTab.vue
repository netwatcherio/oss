<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Agent, NetInfoPayload, SysInfoPayload, RouteEntry, InterfaceInfo } from '@/types';
import { since } from '@/time';

interface Props {
  agent: Agent;
  workspaceId: string | number;
  loadingState: {
    systemInfo: boolean;
    networkInfo: boolean;
  };
  systemInfo: SysInfoPayload;
  networkInfo: NetInfoPayload;
  isOnline: boolean;
  ouiCache: Record<string, string>;
}

const props = defineProps<Props>();

const copiedField = ref<string | null>(null);

const hasSystemData = computed(() => {
  return props.systemInfo && props.systemInfo.hostInfo && !props.loadingState.systemInfo;
});

const hasNetworkData = computed(() => {
  return props.networkInfo && props.networkInfo.public_address && !props.loadingState.networkInfo;
});

const hasP11Interfaces = computed(() => {
  return props.networkInfo?.interfaces && props.networkInfo.interfaces.length > 0;
});

const hasP11Routes = computed(() => {
  return props.networkInfo?.routes && props.networkInfo.routes.length > 0;
});

function getVendorSync(macAddress: string): string {
  if (!macAddress) return 'Unknown';
  const normalizedMac = macAddress.replace(/[:-]/g, '').toUpperCase();
  return props.ouiCache[normalizedMac] || 'Loading...';
}

function getInterfaceType(ifaceName: string): string {
  const lower = ifaceName.toLowerCase();
  if (lower.includes('wifi') || lower.includes('wlan') || lower.includes('wlp')) return 'wifi';
  if (lower.includes('eth') || lower.includes('enp') || lower.includes('eno')) return 'ethernet';
  if (lower.includes('lo') || lower === 'loopback') return 'loopback';
  if (lower.includes('docker') || lower.includes('br-') || lower.includes('veth')) return 'virtual';
  if (lower.includes('tun') || lower.includes('tap') || lower.includes('vpn')) return 'vpn';
  return 'other';
}

function getInterfaceIcon(ifaceName: string): string {
  const type = getInterfaceType(ifaceName);
  switch (type) {
    case 'wifi': return 'bi bi-wifi';
    case 'ethernet': return 'bi bi-ethernet';
    case 'loopback': return 'bi bi-arrow-repeat';
    case 'virtual': return 'bi bi-box';
    case 'vpn': return 'bi bi-shield-lock';
    default: return 'bi bi-hdd-network';
  }
}

async function copyToClipboard(text: string, fieldName: string) {
  try {
    await navigator.clipboard.writeText(text);
    copiedField.value = fieldName;
    setTimeout(() => {
      copiedField.value = null;
    }, 2000);
  } catch (err) {
    console.error('Failed to copy:', err);
  }
}

function getLocalAddresses(addresses: string[]): string[] {
  let ipv4s = addresses.filter(f => f.split(".").length == 4);
  let nonLocal = ipv4s.filter(i => !i.includes("127.0.0.1"));
  return nonLocal.map(l => l.split('/')[0]);
}
</script>

<template>
  <div class="tab-panel">
    <!-- Network Information Grid -->
    <div class="info-grid mt-4">
      <!-- Network Information Card -->
      <div class="info-card enhanced" :class="{'loading': loadingState.networkInfo}">
        <div class="card-header">
          <h5 class="card-title"><i class="bi bi-globe2"></i> Network Information</h5>
          <div class="connection-status" v-if="!loadingState.networkInfo">
            <span class="status-dot" :class="isOnline ? 'online' : 'offline'"></span>
            <span class="status-text">{{ isOnline ? 'Connected' : 'Offline' }}</span>
          </div>
        </div>
        
        <div class="card-content">
          <div class="info-row" v-if="hasNetworkData">
            <span class="info-label"><i class="bi bi-clock-history"></i> Last updated</span>
            <span class="info-value"><span>{{ since(networkInfo.timestamp, true) }}</span></span>
          </div>
          
          <div class="info-row">
            <span class="info-label"><i class="bi bi-pc-display"></i> Hostname</span>
            <span class="info-value">
              <span v-if="loadingState.systemInfo" class="skeleton-text">--------------------</span>
              <span v-else class="hostname-value">{{ systemInfo.hostInfo?.name || 'Unknown' }}</span>
            </span>
          </div>
          
          <div class="info-row copyable">
            <span class="info-label"><i class="bi bi-cloud"></i> Public IP</span>
            <div class="info-value-with-copy">
              <span v-if="loadingState.networkInfo" class="skeleton-text">---------------</span>
              <template v-else>
                <span class="ip-value">{{ networkInfo.public_address || 'Unknown' }}</span>
                <button 
                  v-if="networkInfo.public_address" 
                  class="copy-btn" 
                  @click.stop="copyToClipboard(networkInfo.public_address, 'publicIp')" 
                  :class="{ copied: copiedField === 'publicIp' }" 
                  :title="copiedField === 'publicIp' ? 'Copied!' : 'Copy to clipboard'"
                >
                  <i :class="copiedField === 'publicIp' ? 'bi bi-check-lg' : 'bi bi-clipboard'"></i>
                </button>
              </template>
            </div>
          </div>
          
          <div class="info-row">
            <span class="info-label"><i class="bi bi-building"></i> ISP</span>
            <span class="info-value">
              <span v-if="loadingState.networkInfo" class="skeleton-text">-------------------------</span>
              <span v-else class="isp-value">{{ networkInfo.internet_provider || 'Unknown' }}</span>
            </span>
          </div>
          
          <div class="info-row copyable">
            <span class="info-label"><i class="bi bi-router"></i> Gateway</span>
            <div class="info-value-with-copy">
              <span v-if="loadingState.networkInfo" class="skeleton-text">---------------</span>
              <template v-else>
                <span class="ip-value">{{ networkInfo.default_gateway || 'Unknown' }}</span>
                <button 
                  v-if="networkInfo.default_gateway" 
                  class="copy-btn" 
                  @click.stop="copyToClipboard(networkInfo.default_gateway, 'gateway')" 
                  :class="{ copied: copiedField === 'gateway' }" 
                  :title="copiedField === 'gateway' ? 'Copied!' : 'Copy to clipboard'"
                >
                  <i :class="copiedField === 'gateway' ? 'bi bi-check-lg' : 'bi bi-clipboard'"></i>
                </button>
              </template>
            </div>
          </div>
          
          <div class="info-row local-ips-section">
            <span class="info-label"><i class="bi bi-hdd-network"></i> Local IPs</span>
            <div class="info-value local-ips-list">
              <div v-if="loadingState.systemInfo" class="skeleton-text">---------------</div>
              <div v-else-if="hasSystemData && systemInfo.hostInfo?.ip" class="ip-chips">
                <span 
                  v-for="ip in getLocalAddresses(systemInfo.hostInfo.ip)" 
                  :key="ip" 
                  class="ip-chip" 
                  @click="copyToClipboard(ip, `localIp-${ip}`)" 
                  :class="{ copied: copiedField === `localIp-${ip}` }" 
                  :title="copiedField === `localIp-${ip}` ? 'Copied!' : 'Click to copy'"
                >
                  {{ ip }} <i :class="copiedField === `localIp-${ip}` ? 'bi bi-check-lg' : 'bi bi-clipboard'"></i>
                </span>
              </div>
              <div v-else class="text-muted">No IPs found</div>
            </div>
          </div>
          
          <div class="info-row" v-if="hasNetworkData && networkInfo.lat && networkInfo.long">
            <span class="info-label"><i class="bi bi-geo-alt"></i> Location</span>
            <span class="info-value location-value">
              <a :href="`https://maps.google.com/?q=${networkInfo.lat},${networkInfo.long}`" target="_blank" class="location-link" title="View on Google Maps">
                <i class="bi bi-pin-map"></i> {{ parseFloat(String(networkInfo.lat)).toFixed(4) }}, {{ parseFloat(String(networkInfo.long)).toFixed(4) }}
              </a>
            </span>
          </div>
        </div>
        
        <div class="card-footer" v-if="agent.id">
          <router-link :to="`/workspaces/${workspaceId}/agents/${agent.id}/speedtests`" class="btn btn-sm btn-outline-primary">
            <i class="bi bi-speedometer2"></i> Run Speedtest
          </router-link>
        </div>
      </div>

      <!-- Network Interfaces Card -->
      <div class="info-card enhanced" :class="{'loading': loadingState.networkInfo && loadingState.systemInfo}">
        <div class="card-header">
          <h5 class="card-title"><i class="bi bi-ethernet"></i> Network Interfaces</h5>
          <span v-if="hasP11Interfaces" class="badge bg-success">{{ networkInfo.interfaces.length }} interfaces</span>
          <span v-else-if="!loadingState.systemInfo && hasSystemData && systemInfo?.hostInfo?.mac" class="badge bg-primary">{{ Object.keys(systemInfo.hostInfo.mac).length }} detected</span>
        </div>
        
        <div class="card-content interfaces-content">
          <div v-if="loadingState.networkInfo && loadingState.systemInfo" class="interfaces-loading">
            <div v-for="i in 2" :key="`iface-skeleton-${i}`" class="interface-item skeleton">
              <div class="interface-icon skeleton-box"></div>
              <div class="interface-info">
                <div class="skeleton-text" style="width: 80px; height: 16px;"></div>
                <div class="skeleton-text" style="width: 140px; height: 14px;"></div>
              </div>
            </div>
          </div>
          
          <div v-else-if="hasP11Interfaces" class="interfaces-list p11-interfaces">
            <div 
              v-for="iface in networkInfo.interfaces" 
              :key="iface.name" 
              class="interface-item p11" 
              :class="{ 'is-default': iface.is_default }"
            >
              <div class="interface-icon" :class="iface.type || 'unknown'">
                <i :class="getInterfaceIcon(iface.name)"></i>
              </div>
              <div class="interface-details">
                <div class="interface-header">
                  <span class="interface-name">{{ iface.name }}</span>
                  <span v-if="iface.is_default" class="badge bg-primary ms-1">Default</span>
                  <span v-if="iface.type" class="badge bg-secondary ms-1">{{ iface.type }}</span>
                </div>
                <div v-if="iface.mac" class="interface-mac">
                  <code>{{ iface.mac }}</code>
                  <span class="vendor-name">{{ getVendorSync(iface.mac) }}</span>
                </div>
                <div v-if="iface.ipv4?.length" class="interface-ips">
                  <span v-for="ip in iface.ipv4" :key="ip" class="ip-badge">{{ ip }}</span>
                </div>
                <div v-if="iface.gateway" class="interface-gateway">
                  <i class="bi bi-signpost-2"></i> Gateway: {{ iface.gateway }}
                </div>
              </div>
            </div>
          </div>
          
          <div v-else-if="hasSystemData && systemInfo?.hostInfo?.mac" class="interfaces-list">
            <div 
              v-for="(mac, iface) in systemInfo.hostInfo.mac" 
              :key="iface" 
              class="interface-item" 
              @click="copyToClipboard(mac, `mac-${iface}`)" 
              :class="{ copied: copiedField === `mac-${iface}` }" 
              :title="copiedField === `mac-${iface}` ? 'Copied!' : 'Click to copy MAC address'"
            >
              <div class="interface-icon" :class="getInterfaceType(String(iface))">
                <i :class="getInterfaceIcon(String(iface))"></i>
              </div>
              <div class="interface-info">
                <div class="interface-name">{{ iface }}</div>
                <div class="interface-mac">
                  <code>{{ mac }}</code>
                  <span class="vendor-name">{{ getVendorSync(mac) }}</span>
                </div>
              </div>
              <div class="copy-indicator">
                <i :class="copiedField === `mac-${iface}` ? 'bi bi-check-lg' : 'bi bi-clipboard'"></i>
              </div>
            </div>
          </div>
          
          <div v-else class="empty-interfaces">
            <i class="bi bi-ethernet"></i>
            <span>No network interfaces detected</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Routing Table -->
    <div v-if="hasP11Routes" class="card glass mt-4">
      <div class="card-header">
        <h5><i class="bi bi-signpost-split"></i> Routing Table</h5>
        <span class="badge bg-info">{{ networkInfo.routes.length }} routes</span>
      </div>
      <div class="card-content routes-content">
        <table class="routes-table">
          <thead>
            <tr>
              <th>Destination</th>
              <th>Gateway</th>
              <th>Interface</th>
              <th>Metric</th>
            </tr>
          </thead>
          <tbody>
            <tr 
              v-for="(route, idx) in networkInfo.routes.slice(0, 10)" 
              :key="idx" 
              :class="{ 'default-route': route.destination === '0.0.0.0/0' }"
            >
              <td><code>{{ route.destination }}</code></td>
              <td>{{ route.gateway || 'on-link' }}</td>
              <td>{{ route.interface }}</td>
              <td>{{ route.metric }}</td>
            </tr>
          </tbody>
        </table>
        <div v-if="networkInfo.routes.length > 10" class="routes-more">
          +{{ networkInfo.routes.length - 10 }} more routes
        </div>
      </div>
    </div>
  </div>
</template>
