<template>
  <div class="traffic-graph-container">
    <!-- Statistics Summary -->
    <div class="stats-row" v-if="statistics">
      <div class="stat-card">
        <div class="stat-label">Current RTT</div>
        <div class="stat-value" :class="getLatencyClass(statistics.currentRtt)">
          {{ statistics.currentRtt.toFixed(1) }} ms
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Average RTT</div>
        <div class="stat-value">{{ statistics.avgRtt.toFixed(1) }} ms</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Min / Max RTT</div>
        <div class="stat-value">{{ statistics.minRtt.toFixed(1) }} / {{ statistics.maxRtt.toFixed(1) }} ms</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Packet Loss</div>
        <div class="stat-value" :class="getPacketLossClass(statistics.avgPacketLoss)">
          {{ statistics.avgPacketLoss.toFixed(1) }}%
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Out of Sequence</div>
        <div class="stat-value">{{ statistics.totalOutOfSequence }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Duplicates</div>
        <div class="stat-value">{{ statistics.totalDuplicates }}</div>
      </div>
      <div class="stat-card" :class="'status-' + statistics.mosQuality">
        <div class="stat-label">Voice Quality (MOS)</div>
        <div class="stat-value" :class="'mos-' + statistics.mosQuality">
          {{ statistics.mos.toFixed(2) }}
          <span class="quality-badge" :class="'badge-' + statistics.mosQuality">{{ statistics.mosQualityLabel }}</span>
        </div>
      </div>
    </div>

    <!-- Last Hour Summary -->
    <div class="last-hour-summary" v-if="lastHourStats">
      <div class="summary-header">
        <span class="summary-icon">📊</span>
        <span class="summary-title">Last Hour Summary</span>
      </div>
      <div class="summary-grid">
        <div class="summary-item">
          <span class="summary-label">Avg Latency</span>
          <span class="summary-value">{{ lastHourStats.avgLatency.toFixed(1) }} ms</span>
        </div>
        <div class="summary-item">
          <span class="summary-label">Avg Jitter</span>
          <span class="summary-value">{{ lastHourStats.avgJitter.toFixed(1) }} ms</span>
        </div>
        <div class="summary-item">
          <span class="summary-label">Packet Loss</span>
          <span class="summary-value" :class="getPacketLossClass(lastHourStats.avgLoss)">{{ lastHourStats.avgLoss.toFixed(2) }}%</span>
        </div>
        <div class="summary-item">
          <span class="summary-label">Avg MOS</span>
          <span class="summary-value" :class="'mos-' + lastHourStats.mosQuality">{{ lastHourStats.avgMos.toFixed(2) }} ({{ lastHourStats.mosQualityLabel }})</span>
        </div>
      </div>
    </div>

    <!-- Chart Container -->
    <div id="trafficGraph" ref="trafficGraph"></div>

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
import { onMounted, onUnmounted, ref, watch, computed } from 'vue';
import ApexCharts from 'apexcharts'
import type { TrafficSimResult } from '@/types';
import { themeService } from '@/services/themeService';
import { calculateMOS, getMosQualityLabel } from '@/utils/mos';

export default {
  name: 'TrafficGraph',
  props: {
    trafficResults: Array as () => TrafficSimResult[],
    intervalSec: {
      type: Number,
      default: 60 // Default to 60 seconds if not provided
    },
    // Pass current time range so we can emit changes for data reload
    currentTimeRange: {
      type: Array as () => [Date, Date] | null,
      default: null
    }
  },
  emits: ['time-range-change'],
  setup(props: { trafficResults: TrafficSimResult[]; intervalSec: number; currentTimeRange: [Date, Date] | null }, { emit }: { emit: (event: 'time-range-change', payload: [Date, Date]) => void }) {
    const trafficGraph = ref(null);
    const chart = ref<ApexCharts | null>(null);
    const selectedRange = ref('all');
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
      { label: '1H', value: '1h' },
      { label: '6H', value: '6h' },
      { label: '24H', value: '24h' },
      { label: '7D', value: '7d' },
      { label: 'All', value: 'all' }
    ];

    // Calculate statistics
    const statistics = computed(() => {
      if (!props.trafficResults || props.trafficResults.length === 0) return null;
      
      const avgRtts = props.trafficResults.map(d => d.averageRTT);
      const minRtts = props.trafficResults.map(d => d.minRTT);
      const maxRtts = props.trafficResults.map(d => d.maxRTT);
      const packetLosses = props.trafficResults.map(d => (d.lostPackets / d.totalPackets) * 100);
      const outOfSequence = props.trafficResults.map(d => d.outOfSequence);
      const duplicates = props.trafficResults.map(d => d.duplicates ?? 0);
      
      const currentRtt = avgRtts[avgRtts.length - 1] || 0;
      const avgRtt = avgRtts.reduce((a, b) => a + b, 0) / avgRtts.length;
      const avgPacketLoss = packetLosses.reduce((a, b) => a + b, 0) / packetLosses.length;
      
      // Calculate jitter as standard deviation of RTT (approximation from min/max spread)
      const jitter = (Math.max(...maxRtts) - Math.min(...minRtts)) / 4;
      
      // Calculate MOS using ITU-T G.107 E-Model
      const mosResult = calculateMOS(avgRtt, jitter, avgPacketLoss);
      
      return {
        currentRtt,
        avgRtt,
        minRtt: Math.min(...minRtts),
        maxRtt: Math.max(...maxRtts),
        avgPacketLoss,
        totalOutOfSequence: outOfSequence.reduce((a, b) => a + b, 0),
        totalDuplicates: duplicates.reduce((a, b) => a + b, 0),
        jitter,
        mos: mosResult.mos,
        mosQuality: mosResult.quality,
        mosQualityLabel: getMosQualityLabel(mosResult.quality)
      };
    });

    // Calculate last hour statistics
    const lastHourStats = computed(() => {
      if (!props.trafficResults || props.trafficResults.length === 0) return null;
      
      const oneHourAgo = Date.now() - (60 * 60 * 1000);
      const lastHourData = props.trafficResults.filter(d => 
        new Date(d.reportTime).getTime() > oneHourAgo
      );
      
      if (lastHourData.length === 0) return null;
      
      const avgLatency = lastHourData.reduce((sum, d) => sum + d.averageRTT, 0) / lastHourData.length;
      const avgLoss = lastHourData.reduce((sum, d) => sum + (d.lostPackets / d.totalPackets) * 100, 0) / lastHourData.length;
      const minRtts = lastHourData.map(d => d.minRTT);
      const maxRtts = lastHourData.map(d => d.maxRTT);
      const avgJitter = (Math.max(...maxRtts) - Math.min(...minRtts)) / 4;
      
      const mosResult = calculateMOS(avgLatency, avgJitter, avgLoss);
      
      return {
        avgLatency,
        avgJitter,
        avgLoss,
        avgMos: mosResult.mos,
        mosQuality: mosResult.quality,
        mosQualityLabel: getMosQualityLabel(mosResult.quality),
        sampleCount: lastHourData.length
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

    const setTimeRange = (range: string) => {
      selectedRange.value = range;
      drawGraph();
    };

    const toggleAnnotations = () => {
      if (chart.value) {
        // Update the chart with or without annotations
        drawGraph();
      }
    };

    const filterDataByTimeRange = (data: TrafficSimResult[]) => {
      if (selectedRange.value === 'all') return data;
      
      const now = new Date().getTime();
      const ranges: Record<string, number> = {
        '1h': 60 * 60 * 1000,
        '6h': 6 * 60 * 60 * 1000,
        '24h': 24 * 60 * 60 * 1000,
        '7d': 7 * 24 * 60 * 60 * 1000
      };
      
      const cutoff = now - ranges[selectedRange.value];
      return data.filter(d => new Date(d.reportTime).getTime() > cutoff);
    };

    const drawGraph = () => {
      if (!trafficGraph.value || !props.trafficResults || props.trafficResults.length === 0) {
        return;
      }

      const filteredData = filterDataByTimeRange(props.trafficResults);
      
      if (chart.value) {
        chart.value.updateOptions(createChartOptions(filteredData, selectedRange.value, showAnnotations.value, maxAllowedGap.value, isDark.value));
      } else {
        chart.value = new ApexCharts(trafficGraph.value, createChartOptions(filteredData, selectedRange.value, showAnnotations.value, maxAllowedGap.value, isDark.value));
        chart.value.render();
      }
    };

    const resizeListener = () => {
      if (chart.value && trafficGraph.value) {
        chart.value.updateOptions({ 
          chart: { width: (trafficGraph.value as HTMLElement).clientWidth } 
        });
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

    watch(() => props.trafficResults, drawGraph, { deep: true });

    return { 
      trafficGraph, 
      statistics, 
      lastHourStats,
      getLatencyClass, 
      getPacketLossClass,
      timeRanges,
      selectedRange,
      setTimeRange,
      showAnnotations,
      toggleAnnotations
    };
  },
};

const DEFAULT_MAX_GAP = 90000; // 90 seconds fallback

function aggregateTrafficData(data: TrafficSimResult[], bucketSizeMs: number): TrafficSimResult[] {
  if (bucketSizeMs === 0) return data; // No aggregation needed
  
  const buckets: Map<number, TrafficSimResult[]> = new Map();
  
  // Group data into time buckets
  data.forEach(point => {
    const bucketTime = Math.floor(new Date(point.reportTime).getTime() / bucketSizeMs) * bucketSizeMs;
    if (!buckets.has(bucketTime)) {
      buckets.set(bucketTime, []);
    }
    buckets.get(bucketTime)!.push(point);
  });
  
  // Aggregate each bucket
  const aggregated: TrafficSimResult[] = [];
  buckets.forEach((bucketData, bucketTime) => {
    if (bucketData.length === 0) return;
    
    // Calculate aggregated values
    const avgRtts = bucketData.map(d => d.averageRTT);
    const minRtts = bucketData.map(d => d.minRTT);
    const maxRtts = bucketData.map(d => d.maxRTT);
    const totalPackets = bucketData.reduce((sum, d) => sum + d.totalPackets, 0);
    const lostPackets = bucketData.reduce((sum, d) => sum + d.lostPackets, 0);
    const outOfSeq = bucketData.reduce((sum, d) => sum + d.outOfSequence, 0);
    
    aggregated.push({
      ...bucketData[0], // Copy other properties
      reportTime: new Date(bucketTime + bucketSizeMs / 2).toISOString(), // Middle of bucket
      averageRTT: avgRtts.reduce((a, b) => a + b, 0) / avgRtts.length,
      minRTT: Math.min(...minRtts),
      maxRTT: Math.max(...maxRtts),
      totalPackets: totalPackets,
      lostPackets: lostPackets,
      outOfSequence: outOfSeq
    });
  });
  
  return aggregated.sort((a, b) => new Date(a.reportTime).getTime() - new Date(b.reportTime).getTime());
}

function getTrafficBucketSize(data: TrafficSimResult[], timeRange: string): number {
  if (data.length === 0) return 0;
  
  // Define target points for each time range - lower counts for better performance
  const targetPoints = {
    '1h': 180,    // ~20 second buckets (reduced from 360)
    '6h': 180,    // ~2 minute buckets  (reduced from 360)
    '24h': 150,   // ~10 minute buckets (reduced from 288)
    '7d': 150,    // ~1 hour buckets (reduced from 336)
    'all': 200    // Dynamic based on data span (reduced from 500)
  };
  
  let dataSpanMs: number;
  if (timeRange === 'all') {
    const times = data.map(d => new Date(d.reportTime).getTime());
    dataSpanMs = Math.max(...times) - Math.min(...times);
  } else {
    const ranges: Record<string, number> = {
      '1h': 60 * 60 * 1000,
      '6h': 6 * 60 * 60 * 1000,
      '24h': 24 * 60 * 60 * 1000,
      '7d': 7 * 24 * 60 * 60 * 1000
    };
    dataSpanMs = ranges[timeRange];
  }
  
  const target = targetPoints[timeRange as keyof typeof targetPoints] || 200;
  const bucketSize = Math.floor(dataSpanMs / target);
  
  // Always aggregate if data exceeds target for performance
  if (data.length <= target) return 0;
  
  return bucketSize;
}

function createChartOptions(data: TrafficSimResult[], timeRange: string, showAnnotations: boolean, maxAllowedGap: number = DEFAULT_MAX_GAP, darkMode: boolean = false): ApexCharts.ApexOptions {
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
  const sortedData = data.sort((a, b) => new Date(a.reportTime).getTime() - new Date(b.reportTime).getTime());
  
  // Determine aggregation bucket size based on time range
  const bucketSize = getTrafficBucketSize(sortedData, timeRange);
  const processedData = bucketSize > 0 ? aggregateTrafficData(sortedData, bucketSize) : sortedData;

  // Calculate the effective gap threshold based on aggregation or actual data spacing
  // For aggregated data, use 3x the bucket size. Otherwise, derive from actual data gaps.
  let effectiveMaxGap = maxAllowedGap;
  if (bucketSize > 0) {
    // Aggregated data: allow 3x the bucket size + some margin
    effectiveMaxGap = Math.max(bucketSize * 3.5, maxAllowedGap);
  } else if (processedData.length >= 2) {
    // Non-aggregated: calculate median gap from actual data and use 3x that
    const gaps: number[] = [];
    for (let i = 1; i < Math.min(processedData.length, 100); i++) {
      const gap = new Date(processedData[i].reportTime).getTime() - new Date(processedData[i - 1].reportTime).getTime();
      if (gap > 0) gaps.push(gap);
    }
    if (gaps.length > 0) {
      gaps.sort((a, b) => a - b);
      const medianGap = gaps[Math.floor(gaps.length / 2)];
      effectiveMaxGap = Math.max(medianGap * 3, maxAllowedGap);
    }
  }

  // Build series with gap detection - insert nulls to break lines at gaps
  const buildSeriesData = (getValue: (d: TrafficSimResult) => number) => {
    const result: { x: number; y: number | null }[] = [];
    processedData.forEach((d, i) => {
      const currentTime = new Date(d.reportTime).getTime();
      if (i > 0) {
        const prevTime = new Date(processedData[i - 1].reportTime).getTime();
        const gap = currentTime - prevTime;
        if (gap > effectiveMaxGap) {
          // Insert null to break the line
          result.push({ x: prevTime + 1, y: null });
        }
      }
      result.push({ x: currentTime, y: getValue(d) });
    });
    return result;
  };

  const series = [
    {
      name: 'Min RTT',
      type: 'line',
      data: buildSeriesData(d => d.minRTT)
    },
    {
      name: 'Avg RTT',
      type: 'line',
      data: buildSeriesData(d => d.averageRTT)
    },
    {
      name: 'Max RTT',
      type: 'line',
      data: buildSeriesData(d => d.maxRTT)
    },
    {
      name: 'Packet Loss',
      type: 'area',
      data: buildSeriesData(d => (d.lostPackets / d.totalPackets) * 100)
    },
    {
      name: 'Out of Sequence',
      type: 'scatter',
      data: processedData.map(d => ({ x: new Date(d.reportTime).getTime(), y: d.outOfSequence }))
    }
  ];

  // Initialize empty annotations (gaps are handled by null values in series data)
  const annotations: ApexAnnotations = {
    xaxis: [],
    yaxis: [],
    points: []
  };

  // Only add other annotations if showAnnotations is true
  if (showAnnotations) {
    const anomalyThreshold = 300; // ms
    
    // Group consecutive high-latency anomalies into regions (like PingGraph)
    let latencyRegion: { start: number; end: number } | null = null;
    
    processedData.forEach((d, i) => {
      const avgRtt = d.averageRTT;
      const maxRtt = d.maxRTT;
      const isAnomaly = avgRtt > anomalyThreshold || maxRtt > anomalyThreshold;
      const timestamp = new Date(d.reportTime).getTime();
      
      if (isAnomaly) {
        if (!latencyRegion) {
          latencyRegion = { start: timestamp, end: timestamp };
        } else {
          latencyRegion.end = timestamp;
        }
      } else if (latencyRegion) {
        // End of anomaly region - add subtle shaded annotation
        annotations.xaxis!.push({
          x: latencyRegion.start,
          x2: latencyRegion.end,
          fillColor: '#fecaca',
          borderColor: '#ef4444',
          opacity: 0.15,
          strokeDashArray: 0
        } as any);
        latencyRegion = null;
      }
    });
    
    // Handle final region if data ends with anomaly
    if (latencyRegion) {
      annotations.xaxis!.push({
        x: latencyRegion.start,
        x2: latencyRegion.end,
        fillColor: '#fecaca',
        borderColor: '#ef4444',
        opacity: 0.15,
        strokeDashArray: 0
      } as any);
    }
    
    // Group consecutive packet loss periods into regions
    let lossRegion: { start: number; end: number; severity: 'low' | 'moderate' | 'high' } | null = null;
    
    processedData.forEach((d, i) => {
      const packetLoss = (d.lostPackets / d.totalPackets) * 100;
      const timestamp = new Date(d.reportTime).getTime();
      
      // Determine severity
      let severity: 'low' | 'moderate' | 'high' | null = null;
      if (packetLoss >= 10) severity = 'high';
      else if (packetLoss >= 5) severity = 'moderate';
      else if (packetLoss >= 1) severity = 'low';
      
      if (severity) {
        if (!lossRegion) {
          lossRegion = { start: timestamp, end: timestamp, severity };
        } else {
          lossRegion.end = timestamp;
          // Upgrade severity if needed
          if (severity === 'high' || (severity === 'moderate' && lossRegion.severity === 'low')) {
            lossRegion.severity = severity;
          }
        }
      } else if (lossRegion) {
        // End of loss region - add subtle shaded annotation
        const color = lossRegion.severity === 'high' ? '#fecaca' : 
                      lossRegion.severity === 'moderate' ? '#fed7aa' : '#fef3c7';
        const borderColor = lossRegion.severity === 'high' ? '#ef4444' : 
                            lossRegion.severity === 'moderate' ? '#f97316' : '#eab308';
        
        annotations.xaxis!.push({
          x: lossRegion.start,
          x2: lossRegion.end,
          fillColor: color,
          borderColor: borderColor,
          opacity: 0.15,
          strokeDashArray: 0
        } as any);
        lossRegion = null;
      }
    });
    
    // Handle final region
    if (lossRegion) {
      const color = lossRegion.severity === 'high' ? '#fecaca' : 
                    lossRegion.severity === 'moderate' ? '#fed7aa' : '#fef3c7';
      const borderColor = lossRegion.severity === 'high' ? '#ef4444' : 
                          lossRegion.severity === 'moderate' ? '#f97316' : '#eab308';
      
      annotations.xaxis!.push({
        x: lossRegion.start,
        x2: lossRegion.end,
        fillColor: color,
        borderColor: borderColor,
        opacity: 0.15,
        strokeDashArray: 0
      } as any);
    }
  }

  // Calculate Y-axis for consistent scaling
  const avgRttValues = processedData.map(d => d.averageRTT);
  const sortedAvgRtts = [...avgRttValues].sort((a, b) => a - b);
  const p90Index = Math.floor(sortedAvgRtts.length * 0.90);
  const p90Value = sortedAvgRtts[p90Index] || sortedAvgRtts[sortedAvgRtts.length - 1];
  const yMax = Math.min(Math.ceil(p90Value * 1.5 / 50) * 50, 500);

  // Performance optimization: determine if we have a large dataset
  const isLargeDataset = processedData.length > 300;

  return {
    series,
    chart: {
      height: 380,
      type: 'line',
      background: 'transparent',
      foreColor: colors.foreColor,
      fontFamily: 'Inter, system-ui, sans-serif',
      stacked: false,
      animations: {
        enabled: !isLargeDataset,  // Disable animations for large datasets
        easing: 'easeinout',
        speed: isLargeDataset ? 0 : 300,
        animateGradually: {
          enabled: !isLargeDataset,
          delay: 50
        },
        dynamicAnimation: {
          enabled: !isLargeDataset,
          speed: 200
        }
      },
      zoom: {
        type: 'x',
        enabled: true,
        autoScaleYaxis: false
      },
      toolbar: {
        show: true,
        offsetX: -5,
        offsetY: -5,
        tools: {
          download: true,
          selection: true,
          zoom: true,
          zoomin: true,
          zoomout: true,
          pan: true,
          reset: true
        },
        autoSelected: 'zoom'
      },
      // Disable drop shadows for better performance
      dropShadow: {
        enabled: false
      },
      redrawOnParentResize: true,
      redrawOnWindowResize: true
    },
    colors: ['#10b981', '#3b82f6', '#f97316', '#fbbf24', '#a855f7'],
    stroke: {
      width: [2, 3, 2, 2, 0],
      curve: 'smooth',
      dashArray: [0, 0, 0, 0, 0]
    },
    fill: {
      type: ['solid', 'solid', 'solid', 'gradient', 'solid'],
      gradient: {
        shadeIntensity: 0.8,
        opacityFrom: 0.35,
        opacityTo: 0.05,
        stops: [0, 95, 100]
      }
    },
    markers: {
      size: isLargeDataset ? [0, 0, 0, 0, 0] : [0, 4, 0, 0, 5],
      strokeWidth: 2,
      strokeColors: '#fff',
      hover: {
        size: 6,
        sizeOffset: 3
      }
    },
    xaxis: {
      type: 'datetime',
      labels: {
        style: {
          colors: colors.labelColor,
          fontSize: '12px'
        },
        datetimeUTC: false
      },
      axisBorder: {
        color: colors.axisBorder
      },
      axisTicks: {
        color: colors.axisBorder
      }
    },
    yaxis: [
      {
        seriesName: ['Min RTT', 'Avg RTT', 'Max RTT'],
        title: {
          text: 'Round Trip Time (ms)',
          style: {
            color: colors.foreColor,
            fontSize: '14px',
            fontWeight: 600
          }
        },
        min: 0,
        max: yMax,
        tickAmount: 8,
        labels: {
          style: {
            colors: colors.labelColor,
            fontSize: '12px'
          },
          formatter: (val) => val.toFixed(0)
        }
      },
      {
        seriesName: ['Packet Loss'],
        opposite: true,
        title: {
          text: 'Packet Loss (%)',
          style: {
            color: colors.foreColor,
            fontSize: '14px',
            fontWeight: 600
          }
        },
        min: 0,
        max: 100,
        tickAmount: 5,
        labels: {
          style: {
            colors: colors.labelColor,
            fontSize: '12px'
          },
          formatter: (val) => val.toFixed(0) + '%'
        }
      },
      {
        seriesName: ['Out of Sequence'],
        opposite: true,
        show: false,
        min: 0,
        labels: {
          style: {
            colors: colors.labelColor,
            fontSize: '12px'
          }
        }
      }
    ],
    tooltip: {
      enabled: true,
      shared: true,
      intersect: false,
      theme: colors.tooltipTheme,
      // Performance: use fixed tooltip position for large datasets
      fixed: isLargeDataset ? {
        enabled: true,
        position: 'topRight',
        offsetX: 0,
        offsetY: 0
      } : {
        enabled: false
      },
      x: {
        format: 'dd MMM HH:mm:ss'
      },
      y: {
        formatter: function (y, { seriesIndex }) {
          if (typeof y !== "undefined") {
            if (seriesIndex <= 2) {
              return y.toFixed(1) + " ms";
            } else if (seriesIndex === 3) {
              return y.toFixed(1) + "%";
            } else {
              return y.toFixed(0) + " packets";
            }
          }
          return y;
        }
      },
      custom: function({ series, seriesIndex, dataPointIndex, w }: any) {
        const timestamp = w.config.series[0].data[dataPointIndex].x;
        const date = new Date(timestamp);
        
        let html = '<div class="custom-tooltip">';
        html += `<div class="tooltip-title">${date.toLocaleString()}</div>`;
        html += '<div class="tooltip-body">';
        
        w.config.series.forEach((s: any, idx: number) => {
          const value = series[idx][dataPointIndex];
          const color = w.config.colors[idx];
          const name = s.name;
          let formattedValue;
          
          if (idx <= 2) {
            formattedValue = value.toFixed(1) + ' ms';
          } else if (idx === 3) {
            formattedValue = value.toFixed(1) + '%';
          } else {
            formattedValue = value.toFixed(0) + ' packets';
          }
          
          html += `
            <div class="tooltip-series">
              <span class="tooltip-marker" style="background-color: ${color}"></span>
              <span class="tooltip-label">${name}:</span>
              <span class="tooltip-value">${formattedValue}</span>
            </div>
          `;
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
      markers: {
        width: 12,
        height: 12,
        radius: 12
      },
      itemMargin: {
        horizontal: 10
      }
    },
    grid: {
      borderColor: colors.gridColor,
      strokeDashArray: 0,
      xaxis: {
        lines: {
          show: true
        }
      },
      yaxis: {
        lines: {
          show: true
        }
      },
      padding: {
        top: 0,
        right: 0,
        bottom: 0,
        left: 0
      }
    },
    annotations: annotations
  };
}
</script>

<style scoped>
.traffic-graph-container {
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

/* MOS Quality Colors */
.stat-value.mos-excellent {
  color: #10b981;
}

.stat-value.mos-good {
  color: #3b82f6;
}

.stat-value.mos-fair {
  color: #eab308;
}

.stat-value.mos-poor {
  color: #f97316;
}

.stat-value.mos-bad {
  color: #ef4444;
}

/* Status-based card backgrounds */
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

/* Quality Badge */
.quality-badge {
  font-size: 0.625rem;
  font-weight: 500;
  padding: 0.125rem 0.375rem;
  border-radius: 4px;
  margin-left: 0.5rem;
  text-transform: uppercase;
  letter-spacing: 0.025em;
}

.badge-excellent {
  background-color: #d1fae5;
  color: #065f46;
}

.badge-good {
  background-color: #dbeafe;
  color: #1e40af;
}

.badge-fair {
  background-color: #fef9c3;
  color: #854d0e;
}

.badge-poor {
  background-color: #ffedd5;
  color: #9a3412;
}

.badge-bad {
  background-color: #fee2e2;
  color: #991b1b;
}

/* Last Hour Summary */
.last-hour-summary {
  background: linear-gradient(135deg, #f0f9ff 0%, #e0f2fe 100%);
  border: 1px solid #bae6fd;
  border-radius: 8px;
  padding: 1rem;
  margin-bottom: 1rem;
}

.summary-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
}

.summary-icon {
  font-size: 1.25rem;
}

.summary-title {
  font-size: 0.875rem;
  font-weight: 600;
  color: #0369a1;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: 1rem;
}

.summary-item {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.summary-label {
  font-size: 0.7rem;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.025em;
}

.summary-value {
  font-size: 1rem;
  font-weight: 600;
  color: #1e293b;
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
:global([data-theme="dark"]) .traffic-graph-container {
  background: #1e293b;
}

:global([data-theme="dark"]) .stat-card {
  background: #334155;
  border-color: #475569;
}

:global([data-theme="dark"]) .stat-label {
  color: #9ca3af;
}

:global([data-theme="dark"]) .stat-value {
  color: #f9fafb;
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