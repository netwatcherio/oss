<template>
  <div class="traffic-graph-container">
    <!-- Chart Container -->
    <div id="trafficGraph" ref="trafficGraph"></div>
  </div>
</template>

<script lang="ts">
import { onMounted, onUnmounted, ref, watch, computed } from 'vue';
import ApexCharts from 'apexcharts'
import type { TrafficSimResult } from '@/types';
import { themeService } from '@/services/themeService';
import { calculateMOS, getMosQuality, getMosQualityLabel } from '@/utils/mos';

// Loss %, guarded against rows/buckets with totalPackets=0 (plain division yields
// NaN, which poisons every average it touches).
function lossPercent(d: { lostPackets: number; totalPackets: number }): number {
  return d.totalPackets > 0 ? (d.lostPackets / d.totalPackets) * 100 : 0;
}

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
    const isDark = ref(themeService.getTheme() === 'dark');
    let themeUnsubscribe: (() => void) | null = null;

    // Calculate the maximum allowed gap dynamically based on probe interval
    // Use 3x the interval to avoid breaking lines with sparse data
    const maxAllowedGap = computed(() => {
      const intervalMs = (props.intervalSec || 60) * 1000;
      return Math.max(intervalMs * 3, 180000); // At least 3 minutes minimum
    });

    // Calculate statistics
    const statistics = computed(() => {
      if (!props.trafficResults || props.trafficResults.length === 0) return null;
      
      const avgRtts = props.trafficResults.map(d => d.averageRTT);
      const minRtts = props.trafficResults.map(d => d.minRTT);
      const maxRtts = props.trafficResults.map(d => d.maxRTT);
      const packetLosses = props.trafficResults.map(lossPercent);
      const outOfSequence = props.trafficResults.map(d => d.outOfSequence);
      const duplicates = props.trafficResults.map(d => d.duplicates ?? 0);
      
      const currentRtt = avgRtts[avgRtts.length - 1] || 0;
      const avgRtt = avgRtts.reduce((a, b) => a + b, 0) / avgRtts.length;
      const avgPacketLoss = packetLosses.reduce((a, b) => a + b, 0) / packetLosses.length;
      
      // Use actual jitterAvg if available, otherwise fall back to approximation
      const jitterVals = props.trafficResults.map(d => d.jitterAvg).filter(v => v != null && v > 0);
      const jitter = jitterVals.length > 0 
        ? jitterVals.reduce((a, b) => a + b, 0) / jitterVals.length
        : (Math.max(...maxRtts) - Math.min(...minRtts)) / 4;
      
      // Collect percentile values from results
      const medianRTTs = props.trafficResults.map(d => d.medianRTT).filter(v => v != null && v > 0);
      const p95RTTs = props.trafficResults.map(d => d.p95RTT).filter(v => v != null && v > 0);
      const p99RTTs = props.trafficResults.map(d => d.p99RTT).filter(v => v != null && v > 0);
      const jitterMedians = props.trafficResults.map(d => d.jitterMedian).filter(v => v != null && v > 0);
      const jitterP95s = props.trafficResults.map(d => d.jitterP95).filter(v => v != null && v > 0);
      
      // Use pre-computed MOS from agent data if available, otherwise calculate
      const mosScores = props.trafficResults.map(d => d.mos || d.mosScore).filter(v => v != null && v > 0);
      const avgMos = mosScores.length > 0 
        ? mosScores.reduce((a, b) => a + b, 0) / mosScores.length
        : calculateMOS(avgRtt, jitter, avgPacketLoss).mos;
      
      // Use pre-computed Network Health Score if available
      const healthScores = props.trafficResults.map(d => d.networkHealthScore).filter(v => v != null && v > 0);
      const avgHealthScore = healthScores.length > 0
        ? healthScores.reduce((a, b) => a + b, 0) / healthScores.length
        : 0;
      
      // Map the numeric MOS to a quality tier; the label is derived from the tier
      const mosQuality = getMosQuality(avgMos);
      
      // Get latency quality from data if available
      const latencyQualities = props.trafficResults.map(d => d.latencyQuality).filter(v => v != null);
      const latencyQuality = latencyQualities.length > 0 ? latencyQualities[latencyQualities.length - 1] : '';
      
      return {
        currentRtt,
        avgRtt,
        medianRtt: medianRTTs.length > 0 ? medianRTTs.reduce((a, b) => a + b, 0) / medianRTTs.length : 0,
        p95Rtt: p95RTTs.length > 0 ? p95RTTs.reduce((a, b) => a + b, 0) / p95RTTs.length : 0,
        p99Rtt: p99RTTs.length > 0 ? p99RTTs.reduce((a, b) => a + b, 0) / p99RTTs.length : 0,
        minRtt: Math.min(...minRtts),
        maxRtt: Math.max(...maxRtts),
        avgPacketLoss,
        totalOutOfSequence: outOfSequence.reduce((a, b) => a + b, 0),
        totalDuplicates: duplicates.reduce((a, b) => a + b, 0),
        jitter,
        jitterMedian: jitterMedians.length > 0 ? jitterMedians.reduce((a, b) => a + b, 0) / jitterMedians.length : 0,
        jitterP95: jitterP95s.length > 0 ? jitterP95s.reduce((a, b) => a + b, 0) / jitterP95s.length : 0,
        mos: avgMos,
        mosQuality,
        mosQualityLabel: getMosQualityLabel(mosQuality),
        networkHealthScore: avgHealthScore,
        latencyQuality
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
      const avgLoss = lastHourData.reduce((sum, d) => sum + lossPercent(d), 0) / lastHourData.length;
      const minRtts = lastHourData.map(d => d.minRTT);
      const maxRtts = lastHourData.map(d => d.maxRTT);
      
      // Use actual jitterAvg if available, otherwise fall back to approximation
      const jitterVals = lastHourData.map(d => d.jitterAvg).filter(v => v != null && v > 0);
      const avgJitter = jitterVals.length > 0
        ? jitterVals.reduce((a, b) => a + b, 0) / jitterVals.length
        : (Math.max(...maxRtts) - Math.min(...minRtts)) / 4;
      
      // Prefer the agent's E-model MOS when present; fall back to the
      // client-side estimate for legacy/non-VoIP rows.
      const agentMos = lastHourData.map(d => d.mos || d.mosScore).filter(v => v != null && v > 0);
      const avgMos = agentMos.length > 0
        ? agentMos.reduce((a, b) => a + b, 0) / agentMos.length
        : calculateMOS(avgLatency, avgJitter, avgLoss).mos;
      const quality = getMosQuality(avgMos);

      return {
        avgLatency,
        avgJitter,
        avgLoss,
        avgMos,
        mosQuality: quality,
        mosQualityLabel: getMosQualityLabel(quality),
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

    const drawGraph = () => {
      if (!trafficGraph.value || !props.trafficResults || props.trafficResults.length === 0) {
        return;
      }

      // The page-level picker already filters the data we receive via
      // props.trafficResults, so no in-component time-range filter is
      // needed.
      const filteredData = props.trafficResults;

      if (chart.value) {
        // updateOptions re-applies the events block (including the new
        // emit/props closure). ApexCharts has no removeEvents /
        // addEventsListeners API; those calls were stale and threw.
        // Note: updateOptions resets the user's zoom. The suppressRedraw
        // flag in the watch below prevents this from being called on every
        // parent re-render — the parent builds a new array reference each
        // render (transformToTrafficSimResult uses .map), so a reference-
        // based watch would loop forever: chart re-renders → zoom resets
        // → zoomed event → parent updates → parent re-renders → new array
        // → watch fires → re-render → ...
        chart.value.updateOptions(createChartOptions(filteredData, 'all', maxAllowedGap.value, isDark.value, emit, props, notifyZoomFromChart));
      } else {
        chart.value = new ApexCharts(trafficGraph.value, createChartOptions(filteredData, 'all', maxAllowedGap.value, isDark.value, emit, props, notifyZoomFromChart));
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

    // Flag set by the chart's own zoom/reset handler to tell the watch
    // "the change came from me, don't redraw". The parent will get the
    // new range, update state.timeRange, and the template will pass a
    // new trafficResults array reference (transformToTrafficSimResult
    // uses .map()). Without this guard the watch would fire, drawGraph
    // would call updateOptions, and updateOptions resets the zoom —
    // feedback loop.
    let suppressRedraw = false;

    // Content-based watch: redraw when the *actual* data changes (length
    // or last reportTime), not on every reference change. The parent
    // produces a new array reference on every render, so a reference-
    // based watch would loop with the chart's own zoom events.
    watch(
      () => {
        const arr = props.trafficResults;
        if (!arr || arr.length === 0) return 0;
        const last = arr[arr.length - 1] as any;
        return arr.length + (last?.reportTime ?? last?.created_at ?? 0);
      },
      () => {
        if (suppressRedraw) {
          suppressRedraw = false;
          return;
        }
        drawGraph();
      }
    );

    // Expose the flag setter to createChartOptions so the events block
    // can mark "I caused this range change, don't redraw."
    function notifyZoomFromChart() {
      suppressRedraw = true;
    }

    return {
      trafficGraph,
      statistics,
      lastHourStats,
      getLatencyClass,
      getPacketLossClass,
    };
  },
};

const DEFAULT_MAX_GAP = 90000; // 90 seconds fallback

function avgOf(vals: number[]): number {
  return vals.length > 0 ? vals.reduce((a, b) => a + b, 0) / vals.length : 0;
}

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
    const medianRtts = bucketData.map(d => d.medianRTT).filter(v => v != null && v > 0);
    const p95Rtts = bucketData.map(d => d.p95RTT).filter(v => v != null && v > 0);
    const p99Rtts = bucketData.map(d => d.p99RTT).filter(v => v != null && v > 0);
    const jitterAvgs = bucketData.map(d => d.jitterAvg).filter(v => v != null && v > 0);
    const jitterMedians = bucketData.map(d => d.jitterMedian).filter(v => v != null && v > 0);
    const jitterP95s = bucketData.map(d => d.jitterP95).filter(v => v != null && v > 0);
    const totalPackets = bucketData.reduce((sum, d) => sum + d.totalPackets, 0);
    const lostPackets = bucketData.reduce((sum, d) => sum + d.lostPackets, 0);
    const outOfSeq = bucketData.reduce((sum, d) => sum + d.outOfSequence, 0);
    
    // Helper for percentile calculation
    const percentile = (arr: number[], p: number) => {
      if (arr.length === 0) return 0;
      const sorted = [...arr].sort((a, b) => a - b);
      const idx = Math.floor((arr.length - 1) * p / 100);
      return sorted[idx] || sorted[sorted.length - 1];
    };
    
    aggregated.push({
      ...bucketData[0], // Copy other properties
      reportTime: new Date(bucketTime + bucketSizeMs / 2).toISOString(), // Middle of bucket
      averageRTT: avgRtts.reduce((a, b) => a + b, 0) / avgRtts.length,
      medianRTT: medianRtts.length > 0 ? percentile(medianRtts, 50) : 0,
      p95RTT: p95Rtts.length > 0 ? percentile(p95Rtts, 95) : 0,
      p99RTT: p99Rtts.length > 0 ? percentile(p99Rtts, 99) : 0,
      minRTT: Math.min(...minRtts),
      maxRTT: Math.max(...maxRtts),
      jitterAvg: jitterAvgs.length > 0 ? jitterAvgs.reduce((a, b) => a + b, 0) / jitterAvgs.length : 0,
      jitterMedian: jitterMedians.length > 0 ? percentile(jitterMedians, 50) : 0,
      jitterP95: jitterP95s.length > 0 ? percentile(jitterP95s, 95) : 0,
      totalPackets: totalPackets,
      lostPackets: lostPackets,
      outOfSequence: outOfSeq,
      // Tooltip-only quality values: average across the bucket when present
      mos: avgOf(bucketData.map(d => d.mos || d.mosScore || 0).filter(v => v > 0)),
      networkHealthScore: avgOf(bucketData.map(d => d.networkHealthScore || 0).filter(v => v > 0))
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

function createChartOptions(
  data: TrafficSimResult[],
  timeRange: string,
  maxAllowedGap: number = DEFAULT_MAX_GAP,
  darkMode: boolean = false,
  emit?: (event: 'time-range-change', payload: [Date, Date]) => void,
  props?: { currentTimeRange: [Date, Date] | null },
  notifyZoomFromChart?: () => void
): ApexCharts.ApexOptions {
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
      name: 'Avg RTT',
      type: 'line',
      data: buildSeriesData(d => d.averageRTT)
    },
    {
      name: 'Median RTT',
      type: 'line',
      data: buildSeriesData(d => d.medianRTT || d.averageRTT)
    },
    {
      name: 'P95 RTT',
      type: 'line',
      data: buildSeriesData(d => d.p95RTT || d.averageRTT)
    },
    {
      name: 'P99 RTT',
      type: 'line',
      data: buildSeriesData(d => d.p99RTT || d.averageRTT)
    },
    {
      name: 'Min RTT',
      type: 'line',
      data: buildSeriesData(d => d.minRTT)
    },
    {
      name: 'Max RTT',
      type: 'line',
      data: buildSeriesData(d => d.maxRTT)
    },
    {
      name: 'Jitter Avg',
      type: 'line',
      data: buildSeriesData(d => d.jitterAvg || 0)
    },
    {
      name: 'Jitter P95',
      type: 'line',
      data: buildSeriesData(d => d.jitterP95 || 0)
    },
    {
      name: 'Packet Loss',
      type: 'area',
      data: buildSeriesData(lossPercent)
    },
    {
      name: 'Out of Sequence',
      type: 'scatter',
      data: processedData.map(d => ({ x: new Date(d.reportTime).getTime(), y: d.outOfSequence }))
    }
    // MOS / Network Health intentionally have NO lines here — they clutter the
    // troubleshooting view and MOS has its own dedicated graph. Their values
    // still appear in the hover tooltip (looked up by timestamp below).
  ];

  // Timestamp → data point map for tooltip values that have no chart series
  const pointByTime = new Map<number, TrafficSimResult>();
  processedData.forEach(d => pointByTime.set(new Date(d.reportTime).getTime(), d));

  // Color palette - ordered to match series indices above
  const seriesColors = [
    '#3b82f6',  // Avg RTT - blue
    '#8b5cf6',  // Median RTT - purple
    '#06b6d4',  // P95 RTT - cyan
    '#ec4899',  // P99 RTT - pink
    '#10b981',  // Min RTT - green
    '#f97316',  // Max RTT - orange
    '#eab308',  // Jitter Avg - yellow
    '#f59e0b',  // Jitter P95 - amber
    '#ef4444',  // Packet Loss - red (distinct from the jitter yellows)
    '#a855f7'   // Out of Sequence - purple scatter
  ];

  // Initialize empty annotations (gaps are handled by null values in series data)
  const annotations: ApexAnnotations = {
    xaxis: [],
    yaxis: [],
    points: []
  };

  // Anomaly regions are always drawn (the toggle was removed — there was no
  // legitimate reason to hide latency / loss regions in the troubleshooting
  // view).
  {
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
      const packetLoss = lossPercent(d);
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
        autoScaleYaxis: true,
        allowMouseWheelZoom: true
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
      events: {
        zoomed: (chartContext: any, { xaxis }: any) => {
          if (xaxis && xaxis.min && xaxis.max) {
            const newFrom = new Date(xaxis.min);
            const newTo = new Date(xaxis.max);
            console.log('[TrafficSimGraph] Zoomed to:', newFrom.toISOString(), '->', newTo.toISOString());
            // Mark the next trafficResults re-render as "caused by us" so
            // the watch skips drawGraph — otherwise updateOptions resets
            // the zoom right back to the data range (feedback loop).
            notifyZoomFromChart?.();
            emit?.('time-range-change', [newFrom, newTo]);
          }
        },
        beforeResetZoom: () => {
          if (props?.currentTimeRange && props.currentTimeRange.length === 2) {
            console.log('[TrafficSimGraph] Reset zoom, restoring original range');
            notifyZoomFromChart?.();
            emit?.('time-range-change', props.currentTimeRange);
          }
          return undefined;
        }
      },
      redrawOnParentResize: true,
      redrawOnWindowResize: true
    },
    colors: seriesColors,
    stroke: {
      // Min/Max RTT are thin dashed envelope lines so spikes stay visible
      // without visually dominating the averages.
      width: [3, 2, 2, 2, 1, 1, 2, 2, 0, 0],
      curve: 'smooth',
      dashArray: [0, 0, 4, 4, 2, 2, 0, 4, 0, 0]  // Dashed for percentile/envelope lines
    },
    fill: {
      type: ['solid', 'solid', 'solid', 'solid', 'solid', 'solid', 'solid', 'solid', 'gradient', 'solid'],
      gradient: {
        shadeIntensity: 0.8,
        opacityFrom: 0.35,
        opacityTo: 0.05,
        stops: [0, 95, 100]
      }
    },
    markers: {
      size: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0],
      strokeWidth: 0,
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
        // Main latency axis: scaled to the KEY metrics only. Max RTT lives on
        // its own independent axis below so a single outlier spike (e.g.
        // 15000ms) can't blow up the scale everything else is read against.
        seriesName: ['Avg RTT', 'Median RTT', 'P95 RTT', 'P99 RTT', 'Min RTT', 'Jitter Avg', 'Jitter P95'],
        title: {
          text: 'Latency / Jitter (ms)',
          style: {
            color: colors.foreColor,
            fontSize: '14px',
            fontWeight: 600
          }
        },
        min: 0,
        forceNiceScale: true,
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
        // Max RTT outlier axis: independent hidden scale. Spikes render as a
        // visible envelope line without distorting the main axis; exact values
        // are in the tooltip and >300ms regions get the anomaly shading.
        seriesName: ['Max RTT'],
        opposite: true,
        show: false,
        min: 0,
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
      },
      // MOS / Network Health have no chart series (tooltip-only) — no axes needed.
    ],
    tooltip: {
      enabled: true,
      shared: true,
      intersect: false,
      theme: colors.tooltipTheme,
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
          if (y != null) {
            // idx 0-7: RTT/Jitter in ms, idx 8: Packet Loss %, idx 9: Out of Sequence count
            if (seriesIndex === 8) {
              return y.toFixed(1) + "%";
            } else if (seriesIndex === 9) {
              return y.toFixed(0) + " pkts";
            }
            return y.toFixed(1) + " ms";
          }
          return y;
        }
      },
      custom: function({ series, seriesIndex, dataPointIndex, w }: any) {
        const timestamp = w.config.series[0].data[dataPointIndex].x;
        const date = new Date(timestamp);
        
        // Get the data point index in original processedData
        const dataIdx = dataPointIndex;
        
        let html = '<div class="custom-tooltip traffic-sim-tooltip">';
        html += `<div class="tooltip-title">${date.toLocaleString()}</div>`;
        html += '<div class="tooltip-section">';
        html += '<div class="tooltip-section-title">Latency</div>';
        html += '<div class="tooltip-body">';
        
        // RTT metrics (indices 0-5)
        const rttNames = ['Avg RTT', 'Median RTT', 'P95 RTT', 'P99 RTT', 'Min RTT', 'Max RTT'];
        const rttIndices = [0, 1, 2, 3, 4, 5];
        
        w.config.series.forEach((s: any, idx: number) => {
          if (idx > 5) return; // RTT series only (0-5); jitter/loss/quality have their own sections
          const value = series[idx][dataPointIndex];
          if (value == null) return;
          const color = w.config.colors[idx];
          const name = s.name;
          html += `
            <div class="tooltip-series">
              <span class="tooltip-marker" style="background-color: ${color}"></span>
              <span class="tooltip-label">${name}:</span>
              <span class="tooltip-value">${value.toFixed(2)} ms</span>
            </div>
          `;
        });
        
        html += '</div></div>';
        
        // Jitter section
        html += '<div class="tooltip-section">';
        html += '<div class="tooltip-section-title">Jitter</div>';
        html += '<div class="tooltip-body">';
        
        // Jitter metrics (indices 6-7)
        w.config.series.forEach((s: any, idx: number) => {
          if (idx !== 6 && idx !== 7) return; // Only jitter — loss/MOS/health have their own sections
          const value = series[idx][dataPointIndex];
          if (value == null || value === 0) return;
          const color = w.config.colors[idx];
          const name = s.name;
          html += `
            <div class="tooltip-series">
              <span class="tooltip-marker" style="background-color: ${color}"></span>
              <span class="tooltip-label">${name}:</span>
              <span class="tooltip-value">${value.toFixed(2)} ms</span>
            </div>
          `;
        });
        
        html += '</div></div>';
        
        // Loss section
        html += '<div class="tooltip-section">';
        html += '<div class="tooltip-section-title">Reliability</div>';
        html += '<div class="tooltip-body">';
        
        // Packet Loss (idx 8)
        const lossValue = series[8]?.[dataPointIndex];
        if (lossValue != null) {
          const color = w.config.colors[8];
          html += `
            <div class="tooltip-series">
              <span class="tooltip-marker" style="background-color: ${color}"></span>
              <span class="tooltip-label">Packet Loss:</span>
              <span class="tooltip-value">${lossValue.toFixed(1)}%</span>
            </div>
          `;
        }
        
        // Out of Sequence (idx 9)
        const oosValue = series[9]?.[dataPointIndex];
        if (oosValue != null) {
          const color = w.config.colors[9];
          html += `
            <div class="tooltip-series">
              <span class="tooltip-marker" style="background-color: ${color}"></span>
              <span class="tooltip-label">Out of Sequence:</span>
              <span class="tooltip-value">${oosValue.toFixed(0)}</span>
            </div>
          `;
        }
        
        html += '</div></div>';
        
        // Quality section: MOS / Network Health have no chart lines (kept off
        // the graph for clarity) — values come from the data point itself.
        const qualityPoint = pointByTime.get(timestamp);
        const mosValue = qualityPoint ? (qualityPoint.mos || qualityPoint.mosScore || 0) : 0;
        const healthValue = qualityPoint?.networkHealthScore || 0;
        if ((mosValue != null && mosValue > 0) || (healthValue != null && healthValue > 0)) {
          html += '<div class="tooltip-section">';
          html += '<div class="tooltip-section-title">Quality</div>';
          html += '<div class="tooltip-body">';
          
          if (mosValue != null && mosValue > 0) {
            const mosColor = mosValue >= 4.0 ? '#22c55e' : mosValue >= 3.5 ? '#3b82f6' : mosValue >= 3.0 ? '#eab308' : '#ef4444';
            const mosLabel = mosValue >= 4.3 ? 'Excellent' : mosValue >= 4.0 ? 'Good' : mosValue >= 3.6 ? 'Acceptable' : mosValue >= 3.0 ? 'Poor' : 'Bad';
            html += `
              <div class="tooltip-series">
                <span class="tooltip-marker" style="background-color: ${mosColor}"></span>
                <span class="tooltip-label">MOS Score:</span>
                <span class="tooltip-value">${mosValue.toFixed(2)} (${mosLabel})</span>
              </div>
            `;
          }
          
          if (healthValue != null && healthValue > 0) {
            const healthColor = healthValue >= 80 ? '#22c55e' : healthValue >= 60 ? '#3b82f6' : healthValue >= 40 ? '#eab308' : '#ef4444';
            html += `
              <div class="tooltip-series">
                <span class="tooltip-marker" style="background-color: ${healthColor}"></span>
                <span class="tooltip-label">Network Health:</span>
                <span class="tooltip-value">${healthValue.toFixed(1)}</span>
              </div>
            `;
          }
          
          html += '</div></div>';
        }
        
        html += '</div></div></div>';
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
        horizontal: 8
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

/* Time-range buttons and annotation toggle were removed — the page-level
   date picker controls the range, and anomaly regions are always on. */


/* Custom tooltip styles */
:deep(.traffic-sim-tooltip) {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 0.75rem;
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.15);
  min-width: 280px;
}

:deep(.tooltip-title) {
  font-size: 0.875rem;
  color: #1f2937;
  margin-bottom: 0.75rem;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid #e5e7eb;
  font-weight: 600;
}

:deep(.tooltip-section) {
  margin-bottom: 0.75rem;
}

:deep(.tooltip-section:last-child) {
  margin-bottom: 0;
}

:deep(.tooltip-section-title) {
  font-size: 0.7rem;
  color: #6b7280;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 0.5rem;
  font-weight: 600;
}

:deep(.tooltip-body) {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

:deep(.tooltip-series) {
  display: flex;
  align-items: center;
  margin-bottom: 0.15rem;
  font-size: 0.8rem;
}

:deep(.tooltip-marker) {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 0.5rem;
  flex-shrink: 0;
}

:deep(.tooltip-label) {
  color: #4b5563;
  margin-right: 0.5rem;
  flex-shrink: 0;
}

:deep(.tooltip-value) {
  color: #1f2937;
  font-weight: 600;
  margin-left: auto;
}

/* Responsive adjustments — controls row was removed */
@media (max-width: 640px) {
  #trafficGraph {
    height: 280px;
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

/* Dark mode tooltips */
:global([data-theme="dark"]) .traffic-sim-tooltip {
  background: #1e293b !important;
  border-color: #475569 !important;
}

:global([data-theme="dark"]) .traffic-sim-tooltip .tooltip-title {
  color: #f9fafb;
  border-bottom-color: #475569;
}

:global([data-theme="dark"]) .traffic-sim-tooltip .tooltip-section-title {
  color: #9ca3af;
}

:global([data-theme="dark"]) .traffic-sim-tooltip .tooltip-label {
  color: #d1d5db;
}

:global([data-theme="dark"]) .traffic-sim-tooltip .tooltip-value {
  color: #f9fafb;
}

:global([data-theme="dark"]) .tooltip-value {
  color: #f9fafb;
}
</style>