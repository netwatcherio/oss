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
      <pre class="mtr-ascii" v-html="coloredTable"></pre>
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
  if (lossPct === 0) return 'loss-excellent';
  if (lossPct <= 5) return 'loss-good';
  if (lossPct <= 20) return 'loss-warning';
  return 'loss-critical';
};

const getLatencyClass = (latencyMs: number): string => {
  if (latencyMs < 30) return 'latency-excellent';
  if (latencyMs < 80) return 'latency-good';
  if (latencyMs < 150) return 'latency-warning';
  return 'latency-critical';
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

// Generate colored HTML table
const coloredTable = computed(() => {
  const payload = mtrPayload.value;
  if (!payload?.report?.hops) return '<span class="mtr-no-data">No MTR data available</span>';

  const lines: string[] = [];
  
  // Header
  lines.push('<span class="mtr-header-row">┌──────┬────────────────────────────────────────────────┬────────┬──────┬──────┬────────┬────────┬────────┬────────┐</span>');
  lines.push('<span class="mtr-header-row">│ Hop  │ Host                                           │ Loss%  │ Snt  │ Recv │ Avg    │ Best   │ Worst  │ StDev  │</span>');
  lines.push('<span class="mtr-header-row">├──────┼────────────────────────────────────────────────┼────────┼──────┼──────┼────────┼────────┼────────┼────────┤</span>');

  payload.report.hops.forEach((hop: any, hopIndex: number) => {
    const hopNum = (hopIndex + 1).toString().padStart(4);
    
    if (!hop.hosts || hop.hosts.length === 0) {
      // Unknown hop
      lines.push(`<span class="mtr-unknown-hop">│ ${hopNum} │ ${'*'.padEnd(46)} │ ${'*'.padStart(6)} │ ${'*'.padStart(4)} │ ${'*'.padStart(4)} │ ${'*'.padStart(6)} │ ${'*'.padStart(6)} │ ${'*'.padStart(6)} │ ${'*'.padStart(6)} │</span>`);
    } else {
      hop.hosts.forEach((host: any, hostIndex: number) => {
        const hostDisplay = `${host.hostname || host.ip} (${host.ip})`.substring(0, 46).padEnd(46);
        const lossPct = parseFloat(hop.loss_pct || '0');
        const avgLatency = parseFloat(hop.avg || '0');
        
        const lossClass = getLossClass(lossPct);
        const latencyClass = getLatencyClass(avgLatency);
        
        const lossStr = (hop.loss_pct || '0').toString().padStart(6);
        const sntStr = (hop.sent?.toString?.() || '-').padStart(4);
        const recvStr = (hop.recv?.toString?.() || '-').padStart(4);
        const avgStr = (hop.avg || '-').toString().padStart(6);
        const bestStr = (hop.best || '-').toString().padStart(6);
        const worstStr = (hop.worst || '-').toString().padStart(6);
        const stddevStr = (hop.stddev || '-').toString().padStart(6);
        
        const hopDisplay = hostIndex === 0 ? hopNum : '    ';
        
        lines.push(
          `│ ${hopDisplay} │ <span class="mtr-host">${hostDisplay}</span> │ <span class="${lossClass}">${lossStr}</span> │ ${sntStr} │ ${recvStr} │ <span class="${latencyClass}">${avgStr}</span> │ <span class="${latencyClass}">${bestStr}</span> │ <span class="${latencyClass}">${worstStr}</span> │ ${stddevStr} │`
        );
      });
    }
  });

  lines.push('<span class="mtr-header-row">└──────┴────────────────────────────────────────────────┴────────┴──────┴──────┴────────┴────────┴────────┴────────┘</span>');

  return lines.join('\n');
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
  background: #1e1e2e;
  border-radius: 8px;
  overflow: hidden;
  margin: 0.5rem 0;
}

.mtr-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.75rem 1rem;
  background: #2a2a3e;
  border-bottom: 1px solid #3a3a4e;
}

.mtr-title {
  color: #cdd6f4;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, monospace;
  font-size: 0.85rem;
  font-weight: 500;
}

.copy-btn {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.4rem 0.8rem;
  background: #45475a;
  color: #cdd6f4;
  border: none;
  border-radius: 4px;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.2s;
}

.copy-btn:hover {
  background: #585b70;
}

.copy-btn.copied {
  background: #a6e3a1;
  color: #1e1e2e;
}

.mtr-table-wrapper {
  overflow-x: auto;
  padding: 1rem;
}

.mtr-ascii {
  font-family: 'SF Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, monospace;
  font-size: 0.8rem;
  line-height: 1.4;
  color: #cdd6f4;
  margin: 0;
  white-space: pre;
}

/* Loss percentage colors */
:deep(.loss-excellent) {
  color: #a6e3a1;
  font-weight: 600;
}

:deep(.loss-good) {
  color: #f9e2af;
}

:deep(.loss-warning) {
  color: #fab387;
}

:deep(.loss-critical) {
  color: #f38ba8;
  font-weight: 600;
}

/* Latency colors */
:deep(.latency-excellent) {
  color: #a6e3a1;
}

:deep(.latency-good) {
  color: #f9e2af;
}

:deep(.latency-warning) {
  color: #fab387;
}

:deep(.latency-critical) {
  color: #f38ba8;
}

/* Other styling */
:deep(.mtr-header-row) {
  color: #6c7086;
}

:deep(.mtr-unknown-hop) {
  color: #6c7086;
  font-style: italic;
}

:deep(.mtr-host) {
  color: #89b4fa;
}

:deep(.mtr-no-data) {
  color: #6c7086;
  font-style: italic;
}
</style>
