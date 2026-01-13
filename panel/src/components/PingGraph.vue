<template>
  <div class="latency-graph-container">
    <!-- Statistics Summary -->
    <div class="stats-row" v-if="statistics">
      <div class="stat-card" :class="'status-' + getLatencyClass(statistics.current)">
        <div class="stat-icon">⚡</div>
        <div class="stat-content">
          <div class="stat-label">Current</div>
          <div class="stat-value" :class="getLatencyClass(statistics.current)">
            {{ statistics.current.toFixed(1) }} ms
          </div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">📊</div>
        <div class="stat-content">
          <div class="stat-label">Average</div>
          <div class="stat-value">{{ statistics.average.toFixed(1) }} ms</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">↕️</div>
        <div class="stat-content">
          <div class="stat-label">Range</div>
          <div class="stat-value">{{ statistics.min.toFixed(1) }} – {{ statistics.max.toFixed(1) }} ms</div>
        </div>
      </div>
      <div class="stat-card" :class="'status-' + getLatencyClass(statistics.p95)">
        <div class="stat-icon">📈</div>
        <div class="stat-content">
          <div class="stat-label">P95</div>
          <div class="stat-value" :class="getLatencyClass(statistics.p95)">
            {{ statistics.p95.toFixed(1) }} ms
          </div>
        </div>
      </div>
      <div class="stat-card" :class="'status-' + getPacketLossClass(statistics.avgPacketLoss)">
        <div class="stat-icon">📦</div>
        <div class="stat-content">
          <div class="stat-label">Packet Loss</div>
          <div class="stat-value" :class="getPacketLossClass(statistics.avgPacketLoss)">
            {{ statistics.avgPacketLoss.toFixed(1) }}%
          </div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">〰️</div>
        <div class="stat-content">
          <div class="stat-label">Jitter</div>
          <div class="stat-value">{{ statistics.jitter.toFixed(1) }} ms</div>
        </div>
      </div>
    </div>

    <!-- Chart Container -->
    <div id="latencyGraph" ref="latencyGraph"></div>

    <!-- Controls Row -->
    <div class="controls-row">
      <!-- Time Range Selector -->
<!--      <div class="time-range-selector">
        <button
          v-for="range in timeRanges"
          :key="range.value"
          :class="['time-btn', { active: selectedRange === range.value }]"
          @click="setTimeRange(range.value)">
          {{ range.label }}
        </button>tt
      </div>-->

      <!-- Annotation Toggle -->
      <div class="annotation-toggle">
        <label class="toggle-label">
          <input
            type="checkbox"
            v-model="showAnnotations"
            @change="toggleAnnotations"
            class="toggle-input"
          />
          <span class="toggle-switch"></span>
          <span class="toggle-text">Show All Annotations</span>
        </label>
      </div>
    </div>
  </div>
</template>

<script lang="ts">
import { onMounted, onUnmounted, ref, watch, computed, defineComponent } from 'vue';
import ApexCharts from 'apexcharts'
import type { PropType } from 'vue';
import type { PingResult } from '@/types';
import { themeService } from '@/services/themeService';

const NS_TO_MS = 1e-6;

function toMs(ns: number): number {
  return ns * NS_TO_MS;
}

function ts(d: PingResult): number {
  return new Date(d.stop_timestamp).getTime();
}

export default defineComponent({
  name: 'LatencyGraph',
  props: {
    pingResults: {
      type: Array as PropType<PingResult[]>,
      required: true
    },
    intervalSec: {
      type: Number,
      default: 60 // Default to 60 seconds if not provided
    }
  },
  setup(props) {
    const latencyGraph = ref<HTMLElement | null>(null);
    const chart = ref<ApexCharts | null>(null);
    const selectedRange = ref<'all' | '1h' | '6h' | '24h' | '7d'>('all');
    const showAnnotations = ref(true);
    const isDark = ref(themeService.getTheme() === 'dark');
    let themeUnsubscribe: (() => void) | null = null;

    // Calculate the maximum allowed gap dynamically based on probe interval
    // Use 3x the interval to avoid breaking lines with sparse data
    const maxAllowedGap = computed(() => {
      const intervalMs = (props.intervalSec || 60) * 1000;
      return Math.max(intervalMs * 3, 180000); // At least 3 minutes minimum
    });

    const timeRanges = [
      { label: '1H', value: '1h' as const },
      { label: '6H', value: '6h' as const },
      { label: '24H', value: '24h' as const },
      { label: '7D', value: '7d' as const },
      { label: 'All', value: 'all' as const }
    ];

    // Calculate statistics (from payload.*)
    const statistics = computed(() => {
      const rows = props.pingResults ?? [];
      if (rows.length === 0) return null;

      // Find the most recent entry by timestamp first
      const sortedByTime = [...rows].sort((a, b) => 
        new Date(b.stop_timestamp).getTime() - new Date(a.stop_timestamp).getTime()
      );
      const mostRecent = sortedByTime[0];
      const currentLatency = mostRecent ? toMs(mostRecent.avg_rtt) : 0;

      const avgRtts = rows.map(d => toMs(d.avg_rtt)).sort((a, b) => a - b);
      const minRtts = rows.map(d => toMs(d.min_rtt));
      const maxRtts = rows.map(d => toMs(d.max_rtt));
      const packetLosses = rows.map(d => Number(d.packet_loss) || 0);
      const jitter = rows.reduce((sum, d) => sum + toMs(d.std_dev_rtt), 0) / rows.length;
      
      // Calculate percentiles for better insights
      const p50Idx = Math.floor(avgRtts.length * 0.5);
      const p95Idx = Math.floor(avgRtts.length * 0.95);
      const p99Idx = Math.floor(avgRtts.length * 0.99);
      
      // Total packets for context
      const totalSent = rows.reduce((sum, d) => sum + (d.packets_sent || 0), 0);
      const totalRecv = rows.reduce((sum, d) => sum + (d.packets_recv || 0), 0);

      return {
        current: currentLatency,
        average: avgRtts.reduce((a, b) => a + b, 0) / avgRtts.length,
        min: Math.min(...minRtts),
        max: Math.max(...maxRtts),
        p50: avgRtts[p50Idx] || 0,
        p95: avgRtts[p95Idx] || 0,
        p99: avgRtts[p99Idx] || 0,
        avgPacketLoss: packetLosses.reduce((a, b) => a + b, 0) / packetLosses.length,
        jitter,
        totalSent,
        totalRecv,
        samples: rows.length
      };
    });

    const getLatencyClass = (latency: number) => {
      if (latency < 50) return 'good';
      if (latency < 150) return 'fair';
      if (latency < 300) return 'poor';
      return 'critical';
    };

    const getPacketLossClass = (loss: number) => {
      if (loss < 1) return 'good';
      if (loss < 5) return 'fair';
      if (loss < 10) return 'poor';
      return 'critical';
    };

    const setTimeRange = (range: 'all' | '1h' | '6h' | '24h' | '7d') => {
      selectedRange.value = range;
      drawGraph();
    };

    const toggleAnnotations = () => {
      if (chart.value) drawGraph();
    };

    const filterDataByTimeRange = (data: PingResult[]) => {
      if (selectedRange.value === 'all') return data;

      const now = Date.now();
      const ranges: Record<typeof selectedRange.value, number> = {
        '1h': 60 * 60 * 1000,
        '6h': 6 * 60 * 60 * 1000,
        '24h': 24 * 60 * 60 * 1000,
        '7d': 7 * 24 * 60 * 60 * 1000,
        'all': Number.POSITIVE_INFINITY
      };

      const cutoff = now - ranges[selectedRange.value];
      return data.filter(d => ts(d) > cutoff);
    };

    function createChartOptions(data: PingResult[], showAll: boolean, darkMode: boolean = false): ApexCharts.ApexOptions {
      // Theme-aware color definitions
      const colors = darkMode ? {
        foreColor: '#e5e7eb',
        labelColor: '#9ca3af',
        gridColor: '#374151',
        axisBorder: '#4b5563',
        tooltipTheme: 'dark' as const
      } : {
        foreColor: '#374151',
        labelColor: '#6b7280',
        gridColor: '#e5e7eb',
        axisBorder: '#e5e7eb',
        tooltipTheme: 'light' as const
      };
      const sortedData = [...data].sort((a, b) => ts(a) - ts(b));

      // Decimate if too many points
      const maxPoints = 500;
      const step = Math.max(1, Math.ceil(sortedData.length / maxPoints));
      const decimated = sortedData.filter((_, i) => i % step === 0);

      // Build series with gap detection - insert nulls to break lines at gaps
      const buildSeriesData = (getValue: (d: PingResult) => number) => {
        const result: { x: number; y: number | null }[] = [];
        decimated.forEach((d, i) => {
          if (i > 0) {
            const gap = ts(d) - ts(decimated[i - 1]);
            if (gap > maxAllowedGap.value) {
              // Insert null to break the line
              result.push({ x: ts(decimated[i - 1]) + 1, y: null });
            }
          }
          result.push({ x: ts(d), y: getValue(d) });
        });
        return result;
      };

      // Build series
      const series: ApexCharts.ApexOptions['series'] = [
        {
          name: 'Min RTT',
          type: 'line',
          data: buildSeriesData(d => toMs(d.min_rtt))
        },
        {
          name: 'Avg RTT',
          type: 'line',
          data: buildSeriesData(d => toMs(d.avg_rtt))
        },
        {
          name: 'Max RTT',
          type: 'line',
          data: buildSeriesData(d => toMs(d.max_rtt))
        },
        {
          name: 'Packet Loss',
          type: 'area',
          data: buildSeriesData(d => Number(d.packet_loss) || 0),
        } as any
      ];

      // yAxisIndex for packet loss
      (series![3] as any).yAxisIndex = 1;

      // Annotations (only for anomalies, gaps are handled by null values in series data)
      const annotations: ApexCharts.ApexAnnotations = {
        xaxis: [],
        yaxis: [],
        points: []
      };

      // Y-axis scaling (based on 90th percentile of Avg RTT)
      const avgVals = decimated.map(d => toMs(d.avg_rtt)).sort((a, b) => a - b);
      const p90 = avgVals.length ? avgVals[Math.floor(avgVals.length * 0.9)] : 0;
      const yMax = Math.min(Math.ceil((p90 * 1.5) / 50) * 50 || 100, 500);

      if (showAll) {
        const anomalyThreshold = 300; // ms
        // Group consecutive anomalies into regions instead of individual markers
        let currentRegion: { start: number; end: number } | null = null;
        
        decimated.forEach((d, i) => {
          const avgMs = toMs(d.avg_rtt);
          const maxMs = toMs(d.max_rtt);
          const isAnomaly = avgMs > anomalyThreshold || maxMs > anomalyThreshold;
          
          if (isAnomaly) {
            const timestamp = ts(d);
            if (!currentRegion) {
              currentRegion = { start: timestamp, end: timestamp };
            } else {
              currentRegion.end = timestamp;
            }
          } else if (currentRegion) {
            // End of anomaly region - add subtle shaded annotation
            annotations.xaxis!.push({
              x: currentRegion.start,
              x2: currentRegion.end,
              fillColor: '#fecaca',
              borderColor: '#ef4444',
              opacity: 0.15,
              strokeDashArray: 0
            } as any);
            currentRegion = null;
          }
        });
        
        // Handle final region if data ends with anomaly
        if (currentRegion) {
          annotations.xaxis!.push({
            x: currentRegion.start,
            x2: currentRegion.end,
            fillColor: '#fecaca',
            borderColor: '#ef4444',
            opacity: 0.15,
            strokeDashArray: 0
          } as any);
        }
      }

      return {
        series,
        chart: {
          height: 380,
          type: 'line',
          background: 'transparent',
          foreColor: colors.foreColor,
          fontFamily: 'Inter, system-ui, sans-serif',
          animations: {
            enabled: true,
            easing: 'easeinout',
            speed: 400,
            animateGradually: { enabled: true, delay: 100 }
          },
          zoom: { type: 'x', enabled: true, autoScaleYaxis: false },
          toolbar: {
            show: true,
            offsetX: -5,
            offsetY: -5,
            tools: { download: true, selection: true, zoom: true, zoomin: true, zoomout: true, pan: true, reset: true },
            autoSelected: 'zoom'
          },
          dropShadow: { enabled: false }
        },
        colors: ['#10b981', '#3b82f6', '#f97316', '#fbbf24'],
        stroke: { width: [2, 3, 2, 2], curve: 'smooth', dashArray: [0, 0, 0, 0] },
        fill: {
          type: ['solid', 'solid', 'solid', 'gradient'],
          gradient: { shadeIntensity: 0.8, opacityFrom: 0.35, opacityTo: 0.05, stops: [0, 95, 100] }
        },
        markers: { size: [0, 4, 0, 0], strokeWidth: 2, strokeColors: '#fff', hover: { sizeOffset: 3 } },
        xaxis: {
          type: 'datetime',
          labels: { style: { colors: colors.labelColor, fontSize: '12px' }, datetimeUTC: false },
          axisBorder: { color: colors.axisBorder },
          axisTicks: { color: colors.axisBorder }
        },
        yaxis: [
          {
            title: { text: 'Round Trip Time (ms)', style: { color: colors.foreColor, fontSize: '14px', fontWeight: 600 } },
            min: 0,
            max: yMax,
            tickAmount: 8,
            labels: { style: { colors: colors.labelColor, fontSize: '12px' }, formatter: (v: number) => v.toFixed(0) }
          },
          {
            opposite: true,
            title: { text: 'Packet Loss (%)', style: { color: colors.foreColor, fontSize: '14px', fontWeight: 600} },
            min: 0,
            max: 100,
            tickAmount: 5,
            labels: { style: { colors: colors.labelColor, fontSize: '12px' }, formatter: (v: number) => `${v.toFixed(0)}%` }
          }
        ],
        tooltip: {
          shared: true,
          intersect: false,
          theme: colors.tooltipTheme,
          fixed: {
            enabled: true,
            position: 'topLeft',
            offsetX: 10,
            offsetY: 10
          },
          x: { format: 'dd MMM HH:mm:ss' },
          y: {
            formatter: (y: number, { seriesIndex }: { seriesIndex: number }) =>
                typeof y !== 'undefined' ? (seriesIndex <= 2 ? `${y.toFixed(1)} ms` : `${y.toFixed(1)}%`) : y
          },
          custom: function({ series, seriesIndex, dataPointIndex, w }: any) {
            const timestamp = w.config.series[0].data[dataPointIndex]?.x;
            const date = timestamp ? new Date(timestamp) : new Date();
            let html = '<div class="custom-tooltip">';
            html += `<div class="tooltip-title">${date.toLocaleString()}</div>`;
            html += '<div class="tooltip-body">';
            w.config.series.forEach((s: any, idx: number) => {
              const value = series[idx][dataPointIndex];
              if (value === null || value === undefined) return;
              const color = w.config.colors[idx];
              const name = s.name;
              const formattedValue = idx <= 2 ? `${Number(value).toFixed(1)} ms` : `${Number(value).toFixed(1)}%`;
              html += `
                <div class="tooltip-series">
                  <span class="tooltip-marker" style="background-color: ${color}"></span>
                  <span class="tooltip-label">${name}:</span>
                  <span class="tooltip-value">${formattedValue}</span>
                </div>`;
            });
            html += '</div></div>';
            return html;
          }
        },
        legend: {
          show: true,
          position: 'top',
          horizontalAlign: 'right',
          floating: true,
          offsetY: -25,
          offsetX: -5,
          markers: { width: 12, height: 12, radius: 12 },
          itemMargin: { horizontal: 10 }
        },
        grid: {
          borderColor: colors.gridColor,
          strokeDashArray: 0,
          xaxis: { lines: { show: true } },
          yaxis: { lines: { show: true } },
          padding: { top: 0, right: 0, bottom: 0, left: 0 }
        },
        annotations
      };
    }

    const drawGraph = () => {
      if (!latencyGraph.value || !props.pingResults?.length) return;

      const filtered = filterDataByTimeRange(props.pingResults);
      const options = createChartOptions(filtered, showAnnotations.value, isDark.value);

      if (chart.value) {
        chart.value.updateOptions(options as ApexCharts.ApexOptions, false, true);
      } else {
        chart.value = new ApexCharts(latencyGraph.value, options);
        chart.value.render();
      }
    };

    const resizeListener = () => {
      if (chart.value) {
        chart.value.updateOptions({}, false, true);
      }
    };

    onMounted(() => {
      drawGraph();
      window.addEventListener('resize', resizeListener);
      // Subscribe to theme changes
      themeUnsubscribe = themeService.onThemeChange((theme) => {
        isDark.value = theme === 'dark';
        drawGraph();
      });
    });

    onUnmounted(() => {
      window.removeEventListener('resize', resizeListener);
      if (themeUnsubscribe) themeUnsubscribe();
      if (chart.value) {
        chart.value.destroy();
        chart.value = null;
      }
    });

    watch(() => props.pingResults, () => drawGraph(), { deep: true });

    return {
      latencyGraph,
      statistics,
      getLatencyClass,
      getPacketLossClass,
      timeRanges,
      selectedRange,
      setTimeRange,
      showAnnotations,
      toggleAnnotations
    };
  },
});
</script>

<style scoped>
.latency-graph-container {
  background: white;
  border-radius: 12px;
  padding: 1.25rem;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
}

.stats-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  gap: 0.75rem;
  margin-bottom: 1.25rem;
}

.stat-card {
  background: linear-gradient(135deg, #f9fafb 0%, #f3f4f6 100%);
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: 0.875rem;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}

.stat-card:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
}

.stat-icon {
  font-size: 1.25rem;
  flex-shrink: 0;
}

.stat-content {
  flex: 1;
  min-width: 0;
}

.stat-label {
  font-size: 0.7rem;
  color: #6b7280;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 0.125rem;
}

.stat-value {
  font-size: 1.1rem;
  font-weight: 600;
  color: #1f2937;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Status-based card backgrounds */
.stat-card.status-good {
  background: linear-gradient(135deg, #ecfdf5 0%, #d1fae5 100%);
  border-color: #a7f3d0;
}

.stat-card.status-fair {
  background: linear-gradient(135deg, #eff6ff 0%, #dbeafe 100%);
  border-color: #bfdbfe;
}

.stat-card.status-poor {
  background: linear-gradient(135deg, #fffbeb 0%, #fef3c7 100%);
  border-color: #fde68a;
}

.stat-card.status-critical {
  background: linear-gradient(135deg, #fef2f2 0%, #fee2e2 100%);
  border-color: #fecaca;
}

.stat-value.good {
  color: #059669;
}

.stat-value.fair {
  color: #2563eb;
}

.stat-value.poor {
  color: #d97706;
}

.stat-value.critical {
  color: #dc2626;
}

.controls-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 1rem;
  gap: 1rem;
  flex-wrap: wrap;
}

.time-range-selector {
  display: flex;
  gap: 0.5rem;
}

.time-btn {
  padding: 0.375rem 0.75rem;
  border: 1px solid #e5e7eb;
  background: white;
  color: #6b7280;
  border-radius: 4px;
  font-size: 0.875rem;
  cursor: pointer;
  transition: all 0.2s;
}

.time-btn:hover {
  background: #f9fafb;
  color: #374151;
}

.time-btn.active {
  background: #3b82f6;
  color: white;
  border-color: #3b82f6;
}

/* Annotation Toggle Styles */
.annotation-toggle {
  display: flex;
  align-items: center;
}

.toggle-label {
  display: flex;
  align-items: center;
  cursor: pointer;
  user-select: none;
}

.toggle-input {
  position: absolute;
  opacity: 0;
  cursor: pointer;
  height: 0;
  width: 0;
}

.toggle-switch {
  position: relative;
  display: inline-block;
  width: 44px;
  height: 24px;
  background-color: #e5e7eb;
  border-radius: 12px;
  margin-right: 0.5rem;
  transition: background-color 0.2s;
}

.toggle-switch::after {
  content: '';
  position: absolute;
  top: 2px;
  left: 2px;
  width: 20px;
  height: 20px;
  background-color: white;
  border-radius: 50%;
  transition: transform 0.2s;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.2);
}

.toggle-input:checked + .toggle-switch {
  background-color: #3b82f6;
}

.toggle-input:checked + .toggle-switch::after {
  transform: translateX(20px);
}

.toggle-text {
  font-size: 0.875rem;
  color: #374151;
  font-weight: 500;
}

/* Custom tooltip styles */
:deep(.custom-tooltip) {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  padding: 0.75rem;
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
}

:deep(.tooltip-title) {
  font-size: 0.75rem;
  color: #6b7280;
  margin-bottom: 0.5rem;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid #f3f4f6;
}

:deep(.tooltip-series) {
  display: flex;
  align-items: center;
  margin-bottom: 0.25rem;
  font-size: 0.875rem;
}

:deep(.tooltip-marker) {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  margin-right: 0.5rem;
}

:deep(.tooltip-label) {
  color: #6b7280;
  margin-right: 0.5rem;
}

:deep(.tooltip-value) {
  color: #1f2937;
  font-weight: 600;
  margin-left: auto;
}

/* Responsive adjustments */
@media (max-width: 640px) {
  .controls-row {
    flex-direction: column;
    align-items: stretch;
  }

  .time-range-selector {
    justify-content: center;
  }

  .annotation-toggle {
    justify-content: center;
  }
}

/* ========================================
   Dark Mode Styles
   ======================================== */
:global([data-theme="dark"]) .latency-graph-container {
  background: #1e293b;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
}

:global([data-theme="dark"]) .stat-card {
  background: linear-gradient(135deg, #334155 0%, #1e293b 100%);
  border-color: #475569;
}

:global([data-theme="dark"]) .stat-card:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
}

:global([data-theme="dark"]) .stat-label {
  color: #9ca3af;
}

:global([data-theme="dark"]) .stat-value {
  color: #f9fafb;
}

/* Dark mode status cards */
:global([data-theme="dark"]) .stat-card.status-good {
  background: linear-gradient(135deg, #064e3b 0%, #065f46 100%);
  border-color: #059669;
}

:global([data-theme="dark"]) .stat-card.status-fair {
  background: linear-gradient(135deg, #1e3a5f 0%, #1e40af 100%);
  border-color: #3b82f6;
}

:global([data-theme="dark"]) .stat-card.status-poor {
  background: linear-gradient(135deg, #78350f 0%, #92400e 100%);
  border-color: #d97706;
}

:global([data-theme="dark"]) .stat-card.status-critical {
  background: linear-gradient(135deg, #7f1d1d 0%, #991b1b 100%);
  border-color: #dc2626;
}

:global([data-theme="dark"]) .time-btn {
  background: #1e293b;
  border-color: #475569;
  color: #9ca3af;
}

:global([data-theme="dark"]) .time-btn:hover {
  background: #334155;
  color: #f9fafb;
}

:global([data-theme="dark"]) .toggle-switch {
  background-color: #475569;
}

:global([data-theme="dark"]) .toggle-text {
  color: #e5e7eb;
}

/* Dark mode tooltips */
:global([data-theme="dark"]) .custom-tooltip {
  background: #1e293b !important;
  border-color: #475569 !important;
}

:global([data-theme="dark"]) .tooltip-title {
  color: #9ca3af;
  border-bottom-color: #475569;
}

:global([data-theme="dark"]) .tooltip-label {
  color: #9ca3af;
}

:global([data-theme="dark"]) .tooltip-value {
  color: #f9fafb;
}
</style>