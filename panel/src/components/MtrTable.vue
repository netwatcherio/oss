<template>
  <div class="mtr-table-container">
    <div class="mtr-header">
      <span class="mtr-title">{{ title }}</span>
      <button v-if="showCopy" @click="copyToClipboard" class="copy-btn" :class="{ copied: justCopied }">
        <i :class="justCopied ? 'bi bi-check-lg' : 'bi bi-clipboard'"></i>
        {{ justCopied ? 'Copied!' : 'Copy' }}
      </button>
    </div>
    <div class="mtr-table-wrapper">
      <table class="mtr-data-table">
        <thead>
          <tr>
            <th class="col-hop">Hop</th>
            <th class="col-host">Host</th>
            <th class="col-metric">Loss%</th>
            <th class="col-metric">Snt</th>
            <th class="col-metric">Recv</th>
            <th class="col-metric">Avg</th>
            <th class="col-metric">Best</th>
            <th class="col-metric">Worst</th>
            <th class="col-metric">StDev</th>
          </tr>
        </thead>
        <tbody>
          <template v-for="(hop, hopIndex) in mtrPayload?.report?.hops" :key="hopIndex">
            <tr v-if="!hop.hosts || hop.hosts.length === 0" class="unknown-hop">
              <td class="col-hop">{{ hopIndex + 1 }}</td>
              <td class="col-host"><span class="unknown-marker">*</span></td>
              <td class="col-metric">*</td>
              <td class="col-metric">*</td>
              <td class="col-metric">*</td>
              <td class="col-metric">*</td>
              <td class="col-metric">*</td>
              <td class="col-metric">*</td>
              <td class="col-metric">*</td>
            </tr>
            <tr v-else v-for="(host, hostIndex) in hop.hosts" :key="`${hopIndex}-${hostIndex}`">
              <td class="col-hop">{{ hostIndex === 0 ? hopIndex + 1 : '' }}</td>
              <td class="col-host">
                <span class="host-name">{{ host.hostname || host.ip }}</span>
                <span class="host-ip">({{ host.ip }})</span>
              </td>
              <td class="col-metric" :class="getLossClass(parseFloat(hop.loss_pct || '0'))">
                {{ hop.loss_pct || '0.00' }}
              </td>
              <td class="col-metric">{{ hop.sent || '-' }}</td>
              <td class="col-metric">{{ hop.recv || '-' }}</td>
              <td class="col-metric" :class="getLatencyClass(parseFloat(hop.avg || '0'))">
                {{ hop.avg || '-' }}
              </td>
              <td class="col-metric" :class="getLatencyClass(parseFloat(hop.best || '0'))">
                {{ hop.best || '-' }}
              </td>
              <td class="col-metric" :class="getLatencyClass(parseFloat(hop.worst || '0'))">
                {{ hop.worst || '-' }}
              </td>
              <td class="col-metric">{{ hop.stddev || '-' }}</td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue';
import type { ProbeData, MtrResult } from '@/types';
import { AsciiTable3 } from '@/lib/ascii-table3/ascii-table3';

const props = withDefaults(defineProps<{
  probeData: ProbeData;
  showCopy?: boolean;
}>(), {
  showCopy: true
});

const justCopied = ref(false);

// Extract the MTR payload
const mtrPayload = computed(() => props.probeData.payload as MtrResult);

// Build title string
const title = computed(() => {
  const payload = mtrPayload.value;
  if (!payload?.report?.info?.target) return 'MTR Result';
  const target = payload.report.info.target;
  const timestamp = payload.stop_timestamp || props.probeData.created_at;
  return `${target.hostname || target.ip} (${target.ip}) - ${new Date(timestamp).toLocaleString()}`;
});

// Color thresholds
const getLossClass = (lossPct: number): string => {
  if (lossPct === 0) return 'metric-excellent';
  if (lossPct <= 5) return 'metric-good';
  if (lossPct <= 20) return 'metric-warning';
  return 'metric-critical';
};

const getLatencyClass = (latencyMs: number): string => {
  if (latencyMs < 30) return 'metric-excellent';
  if (latencyMs < 80) return 'metric-good';
  if (latencyMs < 150) return 'metric-warning';
  return 'metric-critical';
};

// Generate raw ASCII table (for copy)
const rawTable = computed(() => {
  const payload = mtrPayload.value;
  if (!payload?.report?.hops) return 'No MTR data available';

  const table = new AsciiTable3(title.value);
  table.setHeading('Hop', 'Host', 'Loss%', 'Snt', 'Recv', 'Avg', 'Best', 'Worst', 'StDev');

  payload.report.hops.forEach((hop: any, hopIndex: number) => {
    if (!hop.hosts || hop.hosts.length === 0) {
      table.addRow((hopIndex + 1).toString(), '*', '*', '*', '*', '*', '*', '*', '*');
    } else {
      hop.hosts.forEach((host: any, hostIndex: number) => {
        const hostDisplay = `${host.hostname || host.ip} (${host.ip})`;
        let hopDisplay = (hopIndex + 1).toString();
        if (hostIndex !== 0) hopDisplay = '    ' + hopDisplay;

        table.addRow(
          hopDisplay,
          hostDisplay,
          hop.loss_pct || '0',
          hop.sent?.toString?.() ?? '',
          hop.recv?.toString?.() ?? '',
          hop.avg || '-',
          hop.best || '-',
          hop.worst || '-',
          hop.stddev || '-'
        );
      });
    }
  });

  table.setStyle('unicode-single');
  return table.toString();
});

const copyToClipboard = async () => {
  try {
    await navigator.clipboard.writeText(rawTable.value);
    justCopied.value = true;
    setTimeout(() => {
      justCopied.value = false;
    }, 2000);
  } catch (err) {
    console.error('Failed to copy:', err);
  }
};
</script>

<style scoped>
.mtr-table-container {
  background: #1a1b26;
  border-radius: 10px;
  overflow: hidden;
  margin: 0.5rem 0;
  border: 1px solid #2a2b3d;
}

.mtr-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.875rem 1.25rem;
  background: linear-gradient(135deg, #1e1f2e 0%, #252636 100%);
  border-bottom: 1px solid #2a2b3d;
}

.mtr-title {
  color: #a9b1d6;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, monospace;
  font-size: 0.85rem;
  font-weight: 500;
}

.copy-btn {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.45rem 0.9rem;
  background: #3d59a1;
  color: #c0caf5;
  border: none;
  border-radius: 6px;
  font-size: 0.8rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}

.copy-btn:hover {
  background: #5a7dcf;
  transform: translateY(-1px);
}

.copy-btn.copied {
  background: #9ece6a;
  color: #1a1b26;
}

.mtr-table-wrapper {
  overflow-x: auto;
  padding: 0.5rem;
}

.mtr-data-table {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, monospace;
  font-size: 0.8rem;
}

.mtr-data-table th {
  background: #24253a;
  color: #565f89;
  font-weight: 600;
  text-transform: uppercase;
  font-size: 0.7rem;
  letter-spacing: 0.05em;
  padding: 0.65rem 0.75rem;
  text-align: left;
  border-bottom: 2px solid #414868;
}

.mtr-data-table th:first-child {
  border-radius: 6px 0 0 0;
}

.mtr-data-table th:last-child {
  border-radius: 0 6px 0 0;
}

.mtr-data-table td {
  padding: 0.55rem 0.75rem;
  border-bottom: 1px solid #2a2b3d;
  color: #a9b1d6;
}

.mtr-data-table tbody tr:hover {
  background: rgba(61, 89, 161, 0.1);
}

.mtr-data-table tbody tr:last-child td {
  border-bottom: none;
}

.col-hop {
  width: 50px;
  text-align: center;
  font-weight: 600;
  color: #7aa2f7 !important;
}

.col-host {
  min-width: 200px;
}

.host-name {
  color: #7dcfff;
  font-weight: 500;
}

.host-ip {
  color: #565f89;
  margin-left: 0.35rem;
  font-size: 0.75rem;
}

.col-metric {
  text-align: right;
  min-width: 60px;
  font-variant-numeric: tabular-nums;
}

/* Unknown hop styling */
.unknown-hop td {
  color: #565f89;
  font-style: italic;
}

.unknown-marker {
  color: #565f89;
}

/* Metric color classes */
.metric-excellent {
  color: #9ece6a !important;
  font-weight: 600;
}

.metric-good {
  color: #e0af68 !important;
}

.metric-warning {
  color: #ff9e64 !important;
}

.metric-critical {
  color: #f7768e !important;
  font-weight: 600;
}

/* ==================== LIGHT THEME ==================== */
[data-theme="light"] .mtr-table-container {
  background: #ffffff;
  border-color: #e5e7eb;
}

[data-theme="light"] .mtr-header {
  background: linear-gradient(135deg, #f9fafb 0%, #f3f4f6 100%);
  border-bottom-color: #e5e7eb;
}

[data-theme="light"] .mtr-title {
  color: #4b5563;
}

[data-theme="light"] .copy-btn {
  background: #3b82f6;
  color: #ffffff;
}

[data-theme="light"] .copy-btn:hover {
  background: #2563eb;
}

[data-theme="light"] .copy-btn.copied {
  background: #16a34a;
  color: #ffffff;
}

[data-theme="light"] .mtr-data-table th {
  background: #f3f4f6;
  color: #6b7280;
  border-bottom-color: #d1d5db;
}

[data-theme="light"] .mtr-data-table td {
  border-bottom-color: #e5e7eb;
  color: #374151;
}

[data-theme="light"] .mtr-data-table tbody tr:hover {
  background: rgba(59, 130, 246, 0.04);
}

[data-theme="light"] .col-hop {
  color: #2563eb !important;
}

[data-theme="light"] .host-name {
  color: #0369a1;
}

[data-theme="light"] .host-ip {
  color: #9ca3af;
}

[data-theme="light"] .unknown-hop td {
  color: #d1d5db;
}

[data-theme="light"] .unknown-marker {
  color: #d1d5db;
}

[data-theme="light"] .metric-excellent {
  color: #16a34a !important;
}

[data-theme="light"] .metric-good {
  color: #ca8a04 !important;
}

[data-theme="light"] .metric-warning {
  color: #ea580c !important;
}

[data-theme="light"] .metric-critical {
  color: #dc2626 !important;
}

/* ==================== MOBILE RESPONSIVE ==================== */
@media (max-width: 640px) {
  .mtr-header {
    padding: 0.625rem 0.875rem;
  }

  .mtr-title {
    font-size: 0.75rem;
  }

  .copy-btn {
    padding: 0.35rem 0.625rem;
    font-size: 0.75rem;
  }

  .mtr-data-table {
    font-size: 0.7rem;
  }

  .mtr-data-table th {
    padding: 0.45rem 0.5rem;
    font-size: 0.6rem;
  }

  .mtr-data-table td {
    padding: 0.4rem 0.5rem;
  }

  .col-host {
    min-width: 140px;
  }

  .col-metric {
    min-width: 45px;
  }
}
</style>
