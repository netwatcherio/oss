<template>
  <div class="latency-graph-container">
    <!-- Statistics Summary -->
    <div class="stats-row" v-if="statistics">
      <div class="stat-card">
        <div class="stat-label">Current Latency</div>
        <div class="stat-value" :class="getLatencyClass(statistics.current)">
          {{ statistics.current.toFixed(1) }} ms
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Average</div>
        <div class="stat-value">{{ statistics.average.toFixed(1) }} ms</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Min / Max</div>
        <div class="stat-value">{{ statistics.min.toFixed(1) }} / {{ statistics.max.toFixed(1) }} ms</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">P95 Latency</div>
        <div class="stat-value" :class="getLatencyClass(statistics.p95)">
          {{ statistics.p95.toFixed(1) }} ms
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Packet Loss</div>
        <div class="stat-value" :class="getPacketLossClass(statistics.avgPacketLoss)">
          {{ statistics.avgPacketLoss.toFixed(1) }}%
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Jitter</div>
        <div class="stat-value">{{ statistics.jitter.toFixed(1) }} ms</div>
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

    // Calculate the maximum allowed gap dynamically based on probe interval
    // Use 1.5x the interval + 30s buffer to allow for timing variations
    const maxAllowedGap = computed(() => {
      const intervalMs = (props.intervalSec || 60) * 1000;
      return Math.max(intervalMs * 1.5 + 30000, 90000); // At least 90 seconds minimum
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
        current: avgRtts[avgRtts.length - 1] || 0,
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

    function createChartOptions(data: PingResult[], showAll: boolean): ApexCharts.ApexOptions {
      const sortedData = [...data].sort((a, b) => ts(a) - ts(b));

      // Decimate if too many points
      const maxPoints = 500;
      const step = Math.max(1, Math.ceil(sortedData.length / maxPoints));
      const decimated = sortedData.filter((_, i) => i % step === 0);

      // Build series
      const series: ApexCharts.ApexOptions['series'] = [
        {
          name: 'Min RTT',
          type: 'line',
          data: decimated.map(d => ({ x: ts(d), y: toMs(d.min_rtt) }))
        },
        {
          name: 'Avg RTT',
          type: 'line',
          data: decimated.map(d => ({ x: ts(d), y: toMs(d.avg_rtt) }))
        },
        {
          name: 'Max RTT',
          type: 'line',
          data: decimated.map(d => ({ x: ts(d), y: toMs(d.max_rtt) }))
        },
        {
          name: 'Packet Loss',
          type: 'area',
          // IMPORTANT: map to yAxisIndex 1 so it uses the right axis
          data: decimated.map(d => ({ x: ts(d), y: Number(d.packet_loss) || 0 })),
        } as any
      ];

      // yAxisIndex for packet loss
      // (Type cast due to ApexCharts TS generics; safe in practice.)
      (series![3] as any).yAxisIndex = 1;

      // Annotations
      const annotations: ApexCharts.ApexAnnotations = {
        xaxis: [],
        yaxis: [],
        points: []
      };

      // Gap annotations (on original data set, not decimated)
      sortedData.forEach((cur, i, arr) => {
        if (i === 0) return;
        const prev = arr[i - 1];
        const gap = ts(cur) - ts(prev);
        if (gap > maxAllowedGap.value) {
          annotations.xaxis!.push({
            x: ts(prev),
            x2: ts(cur),
            strokeDashArray: 8,
            fillColor: '#64748b',
            opacity: 0.1,
            label: {
              style: {
                fontSize: '11px',
                color: '#64748b',
                background: 'transparent',
              },
              text: 'Data Gap',
              position: 'top',
              orientation: 'horizontal'
            }
          });
        }
      });

      // Y-axis scaling (based on 90th percentile of Avg RTT)
      const avgVals = decimated.map(d => toMs(d.avg_rtt)).sort((a, b) => a - b);
      const p90 = avgVals.length ? avgVals[Math.floor(avgVals.length * 0.9)] : 0;
      const yMax = Math.min(Math.ceil((p90 * 1.5) / 50) * 50 || 100, 500);

      if (showAll) {
        const anomalyThreshold = 300; // ms
        decimated.forEach(d => {
          const avgMs = toMs(d.avg_rtt);
          const maxMs = toMs(d.max_rtt);
          if (avgMs > anomalyThreshold || maxMs > anomalyThreshold) {
            annotations.points!.push({
              x: ts(d),
              y: Math.min(avgMs, yMax),
              seriesIndex: 1,
              marker: {
                size: 8,
                fillColor: '#ef4444',
                strokeColor: '#fff',
                strokeWidth: 2,
                radius: 2
              },
              label: {
                borderColor: '#ef4444',
                style: { color: '#fff', background: '#ef4444', fontSize: '12px', fontWeight: 'bold' },
                text: `Anomaly: ${avgMs.toFixed(0)}ms`,
                offsetY: -10
              }
            } as any);

            annotations.xaxis!.push({
              x: ts(d),
              strokeDashArray: 0,
              borderColor: '#ef4444',
              opacity: 0.3,
              label: {
                borderColor: '#ef4444',
                style: { color: '#fff', background: '#ef4444' },
                text: 'High Latency',
                position: 'top',
                orientation: 'horizontal'
              }
            });
          }
        });
      }

      return {
        series,
        chart: {
          height: 400,
          type: 'line',
          background: '#ffffff',
          foreColor: '#374151',
          animations: {
            enabled: true,
            easing: 'easeinout',
            speed: 800,
            animateGradually: { enabled: true, delay: 150 }
          },
          zoom: { type: 'x', enabled: true, autoScaleYaxis: false },
          toolbar: {
            show: true,
            tools: { download: true, selection: true, zoom: true, zoomin: true, zoomout: true, pan: true, reset: true }
          }
        },
        colors: ['#10b981', '#3b82f6', '#ef4444', '#f59e0b'],
        stroke: { width: [2, 3, 2, 0], curve: 'smooth', dashArray: [5, 0, 5, 0] },
        fill: {
          type: ['solid', 'solid', 'solid', 'gradient'],
          gradient: { shadeIntensity: 1, opacityFrom: 0.5, opacityTo: 0.2, stops: [0, 90, 100] }
        },
        markers: { size: 0, hover: { sizeOffset: 6 } },
        xaxis: {
          type: 'datetime',
          labels: { style: { colors: '#6b7280', fontSize: '12px' }, datetimeUTC: false },
          axisBorder: { color: '#e5e7eb' },
          axisTicks: { color: '#e5e7eb' }
        },
        yaxis: [
          {
            title: { text: 'Round Trip Time (ms)', style: { color: '#374151', fontSize: '14px', fontWeight: 600 } },
            min: 0,
            max: yMax,
            tickAmount: 8,
            labels: { style: { colors: '#6b7280', fontSize: '12px' }, formatter: (v: number) => v.toFixed(0) }
          },
          {
            opposite: true,
            title: { text: 'Packet Loss (%)', style: { color: '#374151', fontSize: '14px', fontWeight: 600} },
            min: 0,
            max: 100,
            tickAmount: 5,
            labels: { style: { colors: '#6b7280', fontSize: '12px' }, formatter: (v: number) => `${v.toFixed(0)}%` }
          }
        ],
        tooltip: {
          shared: true,
          intersect: false,
          theme: 'light',
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
          borderColor: '#e5e7eb',
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
      const options = createChartOptions(filtered, showAnnotations.value);

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
    });

    onUnmounted(() => {
      window.removeEventListener('resize', resizeListener);
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
  border-radius: 8px;
  padding: 1rem;
}

.stats-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 1rem;
  margin-bottom: 1.5rem;
}

.stat-card {
  background: #f9fafb;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  padding: 0.75rem;
  text-align: center;
}

.stat-label {
  font-size: 0.75rem;
  color: #6b7280;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 0.25rem;
}

.stat-value {
  font-size: 1.25rem;
  font-weight: 600;
  color: #1f2937;
}

.stat-value.good {
  color: #10b981;
}

.stat-value.fair {
  color: #3b82f6;
}

.stat-value.poor {
  color: #f59e0b;
}

.stat-value.critical {
  color: #ef4444;
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
</style>