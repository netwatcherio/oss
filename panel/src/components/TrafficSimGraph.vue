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

export default {
  name: 'TrafficGraph',
  props: {
    trafficResults: Array as () => TrafficSimResult[],
    intervalSec: {
      type: Number,
      default: 60 // Default to 60 seconds if not provided
    }
  },
  setup(props: { trafficResults: TrafficSimResult[]; intervalSec: number }) {
    const trafficGraph = ref(null);
    const chart = ref<ApexCharts | null>(null);
    const selectedRange = ref('all');
    const showAnnotations = ref(true);
    
    // Calculate the maximum allowed gap dynamically based on probe interval
    // Use 1.5x the interval + 30s buffer to allow for timing variations
    const maxAllowedGap = computed(() => {
      const intervalMs = (props.intervalSec || 60) * 1000;
      return Math.max(intervalMs * 1.5 + 30000, 90000); // At least 90 seconds minimum
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
      
      return {
        currentRtt: avgRtts[avgRtts.length - 1] || 0,
        avgRtt: avgRtts.reduce((a, b) => a + b, 0) / avgRtts.length,
        minRtt: Math.min(...minRtts),
        maxRtt: Math.max(...maxRtts),
        avgPacketLoss: packetLosses.reduce((a, b) => a + b, 0) / packetLosses.length,
        totalOutOfSequence: outOfSequence.reduce((a, b) => a + b, 0)
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
        chart.value.updateOptions(createChartOptions(filteredData, selectedRange.value, showAnnotations.value, maxAllowedGap.value));
      } else {
        chart.value = new ApexCharts(trafficGraph.value, createChartOptions(filteredData, selectedRange.value, showAnnotations.value, maxAllowedGap.value));
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
    });

    onUnmounted(() => {
      window.removeEventListener('resize', resizeListener);
      if (chart.value) {
        chart.value.destroy();
        chart.value = null;
      }
    });

    watch(() => props.trafficResults, drawGraph, { deep: true });

    return { 
      trafficGraph, 
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
  
  // Define target points for each time range
  const targetPoints = {
    '1h': 360,    // ~10 second buckets
    '6h': 360,    // ~1 minute buckets  
    '24h': 288,   // ~5 minute buckets
    '7d': 336,    // ~30 minute buckets
    'all': 500    // Dynamic based on data span
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
  
  const target = targetPoints[timeRange as keyof typeof targetPoints] || 500;
  const bucketSize = Math.floor(dataSpanMs / target);
  
  // Don't aggregate if we have fewer points than target
  if (data.length <= target) return 0;
  
  return bucketSize;
}

function createChartOptions(data: TrafficSimResult[], timeRange: string, showAnnotations: boolean, maxAllowedGap: number = DEFAULT_MAX_GAP): ApexCharts.ApexOptions {
  const sortedData = data.sort((a, b) => new Date(a.reportTime).getTime() - new Date(b.reportTime).getTime());
  
  // Determine aggregation bucket size based on time range
  const bucketSize = getTrafficBucketSize(sortedData, timeRange);
  const processedData = bucketSize > 0 ? aggregateTrafficData(sortedData, bucketSize) : sortedData;

  const series = [
    {
      name: 'Min RTT',
      type: 'line',
      data: processedData.map(d => ({ x: new Date(d.reportTime).getTime(), y: d.minRTT }))
    },
    {
      name: 'Avg RTT',
      type: 'line',
      data: processedData.map(d => ({ x: new Date(d.reportTime).getTime(), y: d.averageRTT }))
    },
    {
      name: 'Max RTT',
      type: 'line',
      data: processedData.map(d => ({ x: new Date(d.reportTime).getTime(), y: d.maxRTT }))
    },
    {
      name: 'Packet Loss',
      type: 'area',
      data: processedData.map(d => ({ x: new Date(d.reportTime).getTime(), y: (d.lostPackets / d.totalPackets) * 100 }))
    },
    {
      name: 'Out of Sequence',
      type: 'scatter',
      data: processedData.map(d => ({ x: new Date(d.reportTime).getTime(), y: d.outOfSequence }))
    }
  ];

  // Initialize empty annotations
  const annotations: ApexAnnotations = {
    xaxis: [],
    yaxis: [],
    points: []
  };

  // Gap annotations - always show these
  // Check gaps in original data, not decimated data
  sortedData.forEach((current, index, array) => {
    if (index > 0) {
      const prev = array[index - 1];
      const gap = new Date(current.reportTime).getTime() - new Date(prev.reportTime).getTime();
      if (gap > maxAllowedGap) {
        // Find the corresponding points in decimated data
        const prevTime = new Date(prev.reportTime).getTime();
        const currentTime = new Date(current.reportTime).getTime();
        
        // Only add annotation if both points exist in processed data
        const prevExists = processedData.some(d => Math.abs(new Date(d.reportTime).getTime() - prevTime) < bucketSize);
        const currentExists = processedData.some(d => Math.abs(new Date(d.reportTime).getTime() - currentTime) < bucketSize);
        
        if (prevExists || currentExists || bucketSize === 0) {
          annotations.xaxis.push({
            x: prevTime,
            x2: currentTime,
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
      }
    }
  });

  // Only add other annotations if showAnnotations is true
  if (showAnnotations) {

    // Calculate Y-axis with limited scale for better readability
    const avgRttValues = processedData.map(d => d.averageRTT);
    const sortedAvgRtts = [...avgRttValues].sort((a, b) => a - b);
    const p90Index = Math.floor(sortedAvgRtts.length * 0.90);
    const p90Value = sortedAvgRtts[p90Index] || sortedAvgRtts[sortedAvgRtts.length - 1];
    
    // Set Y-axis limit based on 90th percentile
    const yMax = Math.min(Math.ceil(p90Value * 1.5 / 50) * 50, 500); // Cap at 500ms

    // Add high RTT anomaly annotations (>300ms)
    const anomalyThreshold = 300;
    processedData.forEach((d, index) => {
      const avgRtt = d.averageRTT;
      const maxRtt = d.maxRTT;
      
      if (avgRtt > anomalyThreshold || maxRtt > anomalyThreshold) {
        annotations.points.push({
          x: new Date(d.reportTime).getTime(),
          y: Math.min(avgRtt, yMax),
          seriesIndex: 1, // Avg RTT series
          marker: {
            size: 8,
            fillColor: '#ef4444',
            strokeColor: '#fff',
            strokeWidth: 2,
            radius: 2
          },
          label: {
            borderColor: '#ef4444',
            style: {
              color: '#fff',
              background: '#ef4444',
              fontSize: '12px',
              fontWeight: 'bold'
            },
            text: `Anomaly: ${avgRtt.toFixed(0)}ms`,
            offsetY: -10
          }
        });
        
        // Vertical line for anomaly
        annotations.xaxis.push({
          x: new Date(d.reportTime).getTime(),
          strokeDashArray: 0,
          borderColor: '#ef4444',
          borderWidth: 2,
          opacity: 0.3,
          label: {
            borderColor: '#ef4444',
            style: {
              color: '#fff',
              background: '#ef4444'
            },
            text: 'High Latency',
            position: 'top',
            orientation: 'horizontal'
          }
        });
      }
    });

    // Enhanced packet loss annotations
    let lossRegions: Array<{start: number, end: number, severity: string, color: string}> = [];
    let currentRegion: any = null;

    processedData.forEach((d, index) => {
      const packetLoss = (d.lostPackets / d.totalPackets) * 100;
      let severity = '';
      let color = '';

      if (packetLoss >= 1 && packetLoss < 5) {
        severity = 'Low';
        color = '#fbbf24'; // Amber
      } else if (packetLoss >= 5 && packetLoss < 10) {
        severity = 'Moderate';
        color = '#fb923c'; // Orange
      } else if (packetLoss >= 10) {
        severity = 'High';
        color = '#ef4444'; // Red
      }

      if (severity) {
        if (!currentRegion || currentRegion.severity !== severity) {
          if (currentRegion) {
            lossRegions.push(currentRegion);
          }
          currentRegion = {
            start: new Date(d.reportTime).getTime(),
            end: new Date(d.reportTime).getTime(),
            severity,
            color
          };
        } else {
          currentRegion.end = new Date(d.reportTime).getTime();
        }
      } else if (currentRegion) {
        lossRegions.push(currentRegion);
        currentRegion = null;
      }
    });

    if (currentRegion) {
      lossRegions.push(currentRegion);
    }

    // Add enhanced loss region annotations
    lossRegions.forEach(region => {
      annotations.xaxis.push({
        x: region.start,
        x2: region.end,
        fillColor: region.color,
        borderColor: region.color,
        opacity: 0.2,
        label: {
          text: `${region.severity} Packet Loss`,
          style: {
            fontSize: '12px',
            fontWeight: 'bold',
            color: '#fff',
            background: region.color,
            padding: {
              left: 10,
              right: 10,
              top: 4,
              bottom: 4
            }
          },
          position: 'top'
        }
      });
    });

    // Add individual packet loss point annotations
    processedData.forEach((d, index) => {
      const packetLoss = (d.lostPackets / d.totalPackets) * 100;
      
      if (packetLoss > 0) {
        let color = '';
        let severity = '';
        
        if (packetLoss >= 10) {
          color = '#ef4444'; // Red
          severity = 'High';
        } else if (packetLoss >= 5) {
          color = '#fb923c'; // Orange
          severity = 'Moderate';
        } else if (packetLoss >= 1) {
          color = '#fbbf24'; // Amber
          severity = 'Low';
        }
        
        annotations.points.push({
          x: new Date(d.reportTime).getTime(),
          y: packetLoss,
          seriesIndex: 3, // Packet Loss series
          marker: {
            size: 8,
            fillColor: color,
            strokeColor: '#fff',
            strokeWidth: 2,
            radius: 2
          },
          label: {
            borderColor: color,
            style: {
              color: '#fff',
              background: color,
              fontSize: '12px',
              fontWeight: 'bold'
            },
            text: `${packetLoss.toFixed(1)}% Loss`,
            offsetY: -10
          }
        });
        
        // Vertical line for significant packet loss
        if (packetLoss >= 5) {
          annotations.xaxis.push({
            x: new Date(d.reportTime).getTime(),
            strokeDashArray: 0,
            borderColor: color,
            borderWidth: 2,
            opacity: 0.3,
            label: {
              borderColor: color,
              style: {
                color: '#fff',
                background: color
              },
              text: `${severity} Loss`,
              position: 'bottom',
              orientation: 'horizontal'
            }
          });
        }
      }
    });
  }

  // Calculate Y-axis for consistent scaling
  const avgRttValues = processedData.map(d => d.averageRTT);
  const sortedAvgRtts = [...avgRttValues].sort((a, b) => a - b);
  const p90Index = Math.floor(sortedAvgRtts.length * 0.90);
  const p90Value = sortedAvgRtts[p90Index] || sortedAvgRtts[sortedAvgRtts.length - 1];
  const yMax = Math.min(Math.ceil(p90Value * 1.5 / 50) * 50, 500);

  return {
    series,
    chart: {
      height: 400,
      type: 'line',
      background: '#ffffff',
      foreColor: '#374151',
      stacked: false,
      animations: {
        enabled: true,
        easing: 'easeinout',
        speed: 800,
        animateGradually: {
          enabled: true,
          delay: 150
        }
      },
      zoom: {
        type: 'x',
        enabled: true,
        autoScaleYaxis: false // Keep Y-axis fixed
      },
      toolbar: {
        show: true,
        tools: {
          download: true,
          selection: true,
          zoom: true,
          zoomin: true,
          zoomout: true,
          pan: true,
          reset: true
        }
      }
    },
    colors: ['#10b981', '#3b82f6', '#ef4444', '#f59e0b', '#8b5cf6'],
    stroke: {
      width: [2, 3, 2, 0, 0],
      curve: 'smooth',
      dashArray: [5, 0, 5, 0, 0]
    },
    fill: {
      type: ['solid', 'solid', 'solid', 'gradient', 'solid'],
      gradient: {
        shadeIntensity: 1,
        opacityFrom: 0.5,
        opacityTo: 0.2,
        stops: [0, 90, 100]
      }
    },
    markers: {
      size: [0, 0, 0, 0, 4],
      hover: {
        sizeOffset: 6
      }
    },
    xaxis: {
      type: 'datetime',
      labels: {
        style: {
          colors: '#6b7280',
          fontSize: '12px'
        },
        datetimeUTC: false
      },
      axisBorder: {
        color: '#e5e7eb'
      },
      axisTicks: {
        color: '#e5e7eb'
      }
    },
    yaxis: [
      {
        seriesName: ['Min RTT', 'Avg RTT', 'Max RTT'],
        title: {
          text: 'Round Trip Time (ms)',
          style: {
            color: '#374151',
            fontSize: '14px',
            fontWeight: 600
          }
        },
        min: 0,
        max: yMax,
        tickAmount: 8,
        labels: {
          style: {
            colors: '#6b7280',
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
            color: '#374151',
            fontSize: '14px',
            fontWeight: 600
          }
        },
        min: 0,
        max: 100,
        tickAmount: 5,
        labels: {
          style: {
            colors: '#6b7280',
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
            colors: '#6b7280',
            fontSize: '12px'
          }
        }
      }
    ],
    tooltip: {
      shared: true,
      intersect: false,
      theme: 'light',
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
      borderColor: '#e5e7eb',
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