<template>
  <div class="mos-graph-container">
    <!-- Statistics Summary -->
    <div class="stats-row" v-if="statistics">
      <div class="stat-card" :class="'status-' + statistics.quality">
        <div class="stat-icon">🎙️</div>
        <div class="stat-content">
          <div class="stat-label">Current MOS</div>
          <div class="stat-value" :class="'mos-' + statistics.quality">
            {{ statistics.current.toFixed(2) }}
            <span class="quality-badge" :class="'badge-' + statistics.quality">{{ statistics.qualityLabel }}</span>
          </div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">📊</div>
        <div class="stat-content">
          <div class="stat-label">Average MOS</div>
          <div class="stat-value">{{ statistics.average.toFixed(2) }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">↕️</div>
        <div class="stat-content">
          <div class="stat-label">Range</div>
          <div class="stat-value">{{ statistics.min.toFixed(2) }} – {{ statistics.max.toFixed(2) }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">📡</div>
        <div class="stat-content">
          <div class="stat-label">Data Sources</div>
          <div class="stat-value">{{ statistics.dataSources }}</div>
        </div>
      </div>
    </div>

    <!-- Chart Container -->
    <div id="mosGraph" ref="mosGraph"></div>

    <!-- No data message -->
    <div v-if="!hasData" class="no-data-message">
      <i class="bi bi-graph-up-arrow"></i>
      <p>No MOS data available</p>
    </div>
  </div>
</template>

<script lang="ts">
import { onMounted, onUnmounted, ref, watch, computed, defineComponent } from 'vue';
import ApexCharts from 'apexcharts';
import type { PropType } from 'vue';
import type { PingResult, TrafficSimResult } from '@/types';
import { themeService } from '@/services/themeService';
import { calculateMOS, getMosQuality, getMosQualityLabel, getMosColor, type MosQuality } from '@/utils/mos';

interface MosDataPoint {
  timestamp: number;
  mos: number;
  quality: MosQuality;
  source: 'ping' | 'trafficsim' | 'combined';
}

const NS_TO_MS = 1e-6;

function toMs(ns: number): number {
  return ns * NS_TO_MS;
}

export default defineComponent({
  name: 'MosGraph',
  props: {
    pingResults: {
      type: Array as PropType<PingResult[]>,
      default: () => []
    },
    trafficSimResults: {
      type: Array as PropType<TrafficSimResult[]>,
      default: () => []
    },
    intervalSec: {
      type: Number,
      default: 60
    }
  },
  setup(props) {
    const mosGraph = ref<HTMLElement | null>(null);
    const chart = ref<ApexCharts | null>(null);
    const isDark = ref(themeService.getTheme() === 'dark');
    let themeUnsubscribe: (() => void) | null = null;

    const hasData = computed(() => {
      return (props.pingResults?.length || 0) > 0 || (props.trafficSimResults?.length || 0) > 0;
    });

    // Calculate MOS data points from both sources
    const mosDataPoints = computed((): MosDataPoint[] => {
      const points: MosDataPoint[] = [];
      
      // Process ping data
      if (props.pingResults?.length) {
        for (const ping of props.pingResults) {
          const latency = toMs(ping.avg_rtt);
          const jitter = toMs(ping.std_dev_rtt);
          const packetLoss = ping.packet_loss || 0;
          const { mos, quality } = calculateMOS(latency, jitter, packetLoss);
          points.push({
            timestamp: new Date(ping.stop_timestamp).getTime(),
            mos,
            quality,
            source: 'ping'
          });
        }
      }
      
      // Process traffic sim data
      if (props.trafficSimResults?.length) {
        for (const ts of props.trafficSimResults) {
          const latency = ts.averageRTT;
          // Estimate jitter from min/max spread
          const jitter = (ts.maxRTT - ts.minRTT) / 4;
          const packetLoss = ts.totalPackets > 0 ? (ts.lostPackets / ts.totalPackets) * 100 : 0;
          const { mos, quality } = calculateMOS(latency, jitter, packetLoss);
          points.push({
            timestamp: new Date(ts.reportTime).getTime(),
            mos,
            quality,
            source: 'trafficsim'
          });
        }
      }
      
      // Sort by timestamp
      return points.sort((a, b) => a.timestamp - b.timestamp);
    });

    // Calculate statistics
    const statistics = computed(() => {
      const points = mosDataPoints.value;
      if (points.length === 0) return null;
      
      const mosValues = points.map(p => p.mos);
      const current = mosValues[mosValues.length - 1];
      const average = mosValues.reduce((a, b) => a + b, 0) / mosValues.length;
      const min = Math.min(...mosValues);
      const max = Math.max(...mosValues);
      
      const currentQuality = getMosQuality(current);
      
      // Determine data sources
      const hasPing = points.some(p => p.source === 'ping');
      const hasTrafficSim = points.some(p => p.source === 'trafficsim');
      let dataSources = '';
      if (hasPing && hasTrafficSim) {
        dataSources = 'ICMP + TrafficSim';
      } else if (hasPing) {
        dataSources = 'ICMP Ping';
      } else {
        dataSources = 'TrafficSim';
      }
      
      return {
        current,
        average,
        min,
        max,
        quality: currentQuality,
        qualityLabel: getMosQualityLabel(currentQuality),
        dataSources,
        sampleCount: points.length
      };
    });

    function createChartOptions(data: MosDataPoint[], darkMode: boolean): ApexCharts.ApexOptions {
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

      // Calculate gap threshold dynamically from actual data spacing
      // This ensures lines connect properly across aggregated time ranges
      let maxGap = (props.intervalSec || 60) * 1000 * 3; // Default: 3x interval
      
      if (data.length >= 2) {
        // Calculate median gap from actual data to adapt to aggregation
        const gaps: number[] = [];
        for (let i = 1; i < Math.min(data.length, 50); i++) {
          const gap = data[i].timestamp - data[i - 1].timestamp;
          if (gap > 0) gaps.push(gap);
        }
        if (gaps.length > 0) {
          gaps.sort((a, b) => a - b);
          const medianGap = gaps[Math.floor(gaps.length / 2)];
          // Use 3.5x median gap to allow for variance in aggregated data
          maxGap = Math.max(medianGap * 3.5, maxGap);
        }
      }

      // Build series with gap detection
      const seriesData: { x: number; y: number | null }[] = [];
      data.forEach((d, i) => {
        if (i > 0) {
          const gap = d.timestamp - data[i - 1].timestamp;
          if (gap > maxGap) {
            seriesData.push({ x: data[i - 1].timestamp + 1, y: null });
          }
        }
        seriesData.push({ x: d.timestamp, y: d.mos });
      });

      // Quality band annotations
      const annotations: ApexAnnotations = {
        yaxis: [
          {
            y: 4.3,
            y2: 5.0,
            fillColor: '#10b981',
            opacity: 0.1,
            label: { text: 'Excellent', style: { color: '#10b981' } }
          },
          {
            y: 4.0,
            y2: 4.3,
            fillColor: '#3b82f6',
            opacity: 0.1,
            label: { text: 'Good', style: { color: '#3b82f6' } }
          },
          {
            y: 3.6,
            y2: 4.0,
            fillColor: '#eab308',
            opacity: 0.1,
            label: { text: 'Fair', style: { color: '#eab308' } }
          },
          {
            y: 3.1,
            y2: 3.6,
            fillColor: '#f97316',
            opacity: 0.1,
            label: { text: 'Poor', style: { color: '#f97316' } }
          },
          {
            y: 1.0,
            y2: 3.1,
            fillColor: '#ef4444',
            opacity: 0.1,
            label: { text: 'Bad', style: { color: '#ef4444' } }
          }
        ]
      };

      return {
        series: [{
          name: 'MOS Score',
          type: 'area',
          data: seriesData
        }],
        chart: {
          height: 300,
          type: 'area',
          background: 'transparent',
          foreColor: colors.foreColor,
          fontFamily: 'Inter, system-ui, sans-serif',
          animations: {
            enabled: true,
            easing: 'easeinout',
            speed: 400
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
        colors: ['#8b5cf6'], // Purple for MOS line
        stroke: { 
          width: 3, 
          curve: 'smooth'
        },
        fill: {
          type: 'gradient',
          gradient: {
            shadeIntensity: 0.8,
            opacityFrom: 0.4,
            opacityTo: 0.05,
            stops: [0, 95, 100]
          }
        },
        markers: { 
          size: 0, 
          strokeWidth: 0, 
          hover: { sizeOffset: 3 } 
        },
        dataLabels: {
          enabled: false
        },
        xaxis: {
          type: 'datetime',
          labels: { style: { colors: colors.labelColor, fontSize: '12px' }, datetimeUTC: false },
          axisBorder: { color: colors.axisBorder },
          axisTicks: { color: colors.axisBorder }
        },
        yaxis: {
          min: 1,
          max: 5,
          tickAmount: 8,
          title: { 
            text: 'MOS Score', 
            style: { color: colors.foreColor, fontSize: '14px', fontWeight: 600 } 
          },
          labels: { 
            style: { colors: colors.labelColor, fontSize: '12px' },
            formatter: (v: number) => v.toFixed(1)
          }
        },
        tooltip: {
          shared: false,
          theme: colors.tooltipTheme,
          x: { format: 'dd MMM HH:mm:ss' },
          y: {
            formatter: (y: number) => y != null ? y.toFixed(2) : 'N/A'
          }
        },
        legend: { show: false },
        grid: {
          borderColor: colors.gridColor,
          strokeDashArray: 0,
          xaxis: { lines: { show: true } },
          yaxis: { lines: { show: true } }
        },
        annotations
      };
    }

    const drawGraph = () => {
      if (!mosGraph.value || !hasData.value) return;

      const options = createChartOptions(mosDataPoints.value, isDark.value);

      if (chart.value) {
        chart.value.updateOptions(options, false, true);
      } else {
        chart.value = new ApexCharts(mosGraph.value, options);
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

    watch([() => props.pingResults, () => props.trafficSimResults], () => drawGraph(), { deep: true });

    return {
      mosGraph,
      statistics,
      hasData
    };
  }
});
</script>

<style scoped>
.mos-graph-container {
  background: white;
  border-radius: 12px;
  padding: 1.25rem;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
}

.stats-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
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

/* Status-based backgrounds */
.stat-card.status-excellent {
  background: linear-gradient(135deg, #ecfdf5 0%, #d1fae5 100%);
  border-color: #a7f3d0;
}

.stat-card.status-good {
  background: linear-gradient(135deg, #eff6ff 0%, #dbeafe 100%);
  border-color: #bfdbfe;
}

.stat-card.status-fair {
  background: linear-gradient(135deg, #fefce8 0%, #fef9c3 100%);
  border-color: #fde047;
}

.stat-card.status-poor {
  background: linear-gradient(135deg, #fff7ed 0%, #ffedd5 100%);
  border-color: #fed7aa;
}

.stat-card.status-bad {
  background: linear-gradient(135deg, #fef2f2 0%, #fee2e2 100%);
  border-color: #fecaca;
}

/* MOS Quality Colors */
.stat-value.mos-excellent { color: #10b981; }
.stat-value.mos-good { color: #3b82f6; }
.stat-value.mos-fair { color: #eab308; }
.stat-value.mos-poor { color: #f97316; }
.stat-value.mos-bad { color: #ef4444; }

/* Quality Badge */
.quality-badge {
  font-size: 0.6rem;
  font-weight: 500;
  padding: 0.125rem 0.375rem;
  border-radius: 4px;
  margin-left: 0.5rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.badge-excellent { background: #d1fae5; color: #059669; }
.badge-good { background: #dbeafe; color: #2563eb; }
.badge-fair { background: #fef9c3; color: #ca8a04; }
.badge-poor { background: #ffedd5; color: #ea580c; }
.badge-bad { background: #fee2e2; color: #dc2626; }

.no-data-message {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 3rem;
  color: #9ca3af;
}

.no-data-message i {
  font-size: 2.5rem;
  margin-bottom: 1rem;
}

.no-data-message p {
  margin: 0;
  font-size: 1rem;
}

/* Dark mode support */
[data-theme="dark"] .mos-graph-container {
  background: #1e293b;
}

[data-theme="dark"] .stat-card {
  background: linear-gradient(135deg, #334155 0%, #1e293b 100%);
  border-color: #475569;
}

[data-theme="dark"] .stat-label {
  color: #9ca3af;
}

[data-theme="dark"] .stat-value {
  color: #e2e8f0;
}
</style>
