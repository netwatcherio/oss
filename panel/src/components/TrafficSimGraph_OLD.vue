<template>
  <div ref="trafficGraph"></div>
</template>

<script lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue';
import * as d3 from 'd3';
import type { TrafficSimResult } from '@/types'; // Import your TrafficSimResult type

export default {
  name: 'TrafficGraph_old',
  props: {
    trafficResults: Array as () => TrafficSimResult[],
  },
  setup(props: { trafficResults: TrafficSimResult[]; }) {
    const trafficGraph = ref(null);

    const drawGraph = () => {
      if (!trafficGraph.value || !props.trafficResults || props.trafficResults.length === 0) {
        return;
      }
      createTrafficGraph(props.trafficResults, trafficGraph.value);
    };

    const resizeListener = () => {
      drawGraph();
    };

    onMounted(() => {
      drawGraph();
      window.addEventListener('resize', resizeListener);
    });

    onUnmounted(() => {
      window.removeEventListener('resize', resizeListener);
    });

    watch(() => props.trafficResults, drawGraph, { immediate: true });

    return { trafficGraph };
  },
};

// Define a threshold for the maximum allowed gap (in milliseconds)
const maxAllowedGap = 1000 * 90; // Example: 90 seconds

function isGapAcceptable(current: TrafficSimResult, previous: TrafficSimResult) {
  if (!previous) return true; // Always accept the first point
  return (current.lastReportTime.getTime() - previous.lastReportTime.getTime()) <= maxAllowedGap;
}

function segmentData(data: TrafficSimResult[]) {
  // First, sort the data by lastReportTime
  data.sort((a, b) => new Date(a.lastReportTime).getTime() - new Date(b.lastReportTime).getTime());

  const segments = [];
  let segment = [];

  for (let i = 0; i < data.length; i++) {
    const current = data[i];
    const next = data[i + 1];

    segment.push(current);

    if (next) {
      const currentReportTime = new Date(current.lastReportTime).getTime();
      const nextReportTime = new Date(next.lastReportTime).getTime();

      if (nextReportTime - currentReportTime > maxAllowedGap) {
        segments.push(segment);
        segment = [];
      }
    }
  }

  // Add the last segment if it has data
  if (segment.length) {
    segments.push(segment);
  }

  return segments;
}

function createTrafficGraph(data: TrafficSimResult[], graphElement: HTMLElement) {
  const margin = { top: 20, right: 20, bottom: 30, left: 50 };
  const width = graphElement.clientWidth - margin.left - margin.right;
  const height = 400 - margin.top - margin.bottom;

  d3.select(graphElement).selectAll('*').remove();

  const packetLossColorScale = d3.scaleLinear<string>()
      .domain([1, 50, 100])
      .range(['yellow', 'orange', 'red'] as any[]);
  const outOfSequenceColorScale = d3.scaleLinear<string>()
      .domain([1, 5, 20])
      .range(['green', 'blue', 'black'] as any[]);

  // Create SVG element
  const svg = d3.select(graphElement)
      .append('svg')
      .attr('width', width + margin.left + margin.right)
      .attr('height', height + margin.top + margin.bottom)
      .append('g')
      .attr('transform', `translate(${margin.left},${margin.top})`);

  let xScale = d3.scaleTime()
      .domain(d3.extent(data, (d: TrafficSimResult) => d.lastReportTime))
      .range([0, width]);

  const yScale = d3.scaleLinear()
      .domain([0, d3.max(data, (d: TrafficSimResult) => d.maxRTT > 500 ? 100 : d.maxRTT < 100 ? d.maxRTT * 1.5 : 100)])
      .range([height, 0]);

  svg.append("defs").append("clipPath")
      .attr("id", "clip")
      .append("rect")
      .attr("width", width)
      .attr("height", height);

  // Define the brush
  let brush = d3.brushX()
      .extent([[0, 0], [width, height]])
      .on("end", brushed);

  function updateLines() {
    svg.selectAll(".line-avg").attr("d", avgLine);
    svg.selectAll(".line-max").attr("d", maxLine);
    svg.selectAll(".line-min").attr("d", minLine);
    svg.selectAll(".line-loss").attr("d", lossLine);
  }

  function brushed(event) {
    const selection = event.selection;
    if (selection) {
      const [x0, x1] = selection.map(xScale.invert);
      xScale.domain([x0, x1]);
      svg.select(".x-axis").call(d3.axisBottom(xScale));
      updateLines();
      svg.select(".brush").call(brush.move, null); // Clear the brush selection
    }
  }

  svg.on("dblclick", function () {
    xScale.domain(d3.extent(data, d => d.lastReportTime));
    svg.select(".x-axis").call(d3.axisBottom(xScale));
    updateLines();
  });

  // Add brushing
  svg.append("g")
      .attr("class", "brush")
      .call(brush);

  // Add X axis
  svg.append('g')
      .attr('class', 'x-axis')
      .attr('transform', `translate(0,${height})`)
      .call(d3.axisBottom(xScale));

  // Add Y axis
  svg.append('g')
      .call(d3.axisLeft(yScale));

  const dataSegments = segmentData(data);

  data.forEach((d) => {
    const packetLoss = (d.lostPackets / d.sentPackets) * 100;
    if (packetLoss > 0) {
      const packetLossWidth = 5;
      svg.append('rect')
          .attr('x', xScale(new Date(d.lastReportTime)) - packetLossWidth / 2)
          .attr('y', 0)
          .attr('width', packetLossWidth)
          .attr('height', height)
          .attr('fill', packetLossColorScale(packetLoss))
          .attr('opacity', 0.2);
    }
  });

  data.forEach((d) => {
    const packetLoss = (d.outOfSequence / d.sentPackets) * 100;
    if (packetLoss > 0) {
      const packetLossWidth = 5;
      svg.append('rect')
          .attr('x', xScale(new Date(d.lastReportTime)) - packetLossWidth / 2)
          .attr('y', 0)
          .attr('width', packetLossWidth)
          .attr('height', height)
          .attr('fill', outOfSequenceColorScale(packetLoss))
          .attr('opacity', 0.2);
    }
  });

  for (let i = 0; i < data.length - 1; i++) {
    const currentReportTime = new Date(data[i].lastReportTime).getTime();
    const nextReportTime = new Date(data[i + 1].lastReportTime).getTime();

    if (nextReportTime - currentReportTime > maxAllowedGap) {
      svg.append('rect')
          .attr('x', xScale(currentReportTime))
          .attr('y', 0)
          .attr('width', xScale(nextReportTime) - xScale(currentReportTime))
          .attr('height', height)
          .attr('fill', '#ddd')
          .attr('opacity', 0.2);
    }
  }

  const maxLine = d3.line<TrafficSimResult>()
      .x((d) => xScale(d.lastReportTime))
      .y((d) => yScale(d.maxRTT));

  const minLine = d3.line<TrafficSimResult>()
      .x((d) => xScale(d.lastReportTime))
      .y((d) => yScale(d.minRTT));

  const avgLine = d3.line<TrafficSimResult>()
      .x((d) => xScale(d.lastReportTime))
      .y((d) => yScale(d.averageRTT));

  const lossLine = d3.line<TrafficSimResult>()
      .x((d) => xScale(d.lastReportTime))
      .y((d) => yScale((d.lostPackets / d.sentPackets) * 100));

  dataSegments.forEach(segment => {
    appendPath(segment, 'line-avg', avgLine, 'green');
    appendPath(segment, 'line-max', maxLine, 'darkblue');
    appendPath(segment, 'line-min', minLine, 'lightblue');
    appendPath(segment, 'line-loss', lossLine, 'red');
  });

  function appendPath(segment: TrafficSimResult[], className: string, lineFunction: d3.Line<TrafficSimResult>, color: string) {
    svg.append('path')
        .datum(segment)
        .attr('class', className)
        .attr('fill', 'none')
        .attr('stroke', color)
        .attr('stroke-width', 1.5)
        .attr('d', lineFunction)
        .attr("clip-path", "url(#clip)");
  }

  // Adding legend
  const legend = svg.append("g")
      .attr("class", "legend")
      .attr("transform", `translate(${width - 120},${20})`);

  addLegendItem(legend, 0, "green", "Average RTT");
  addLegendItem(legend, 20, "darkblue", "Max RTT");
  addLegendItem(legend, 40, "red", "Packet Loss %");
  addLegendItem(legend, 60, "lightblue", "Min RTT");
}

function addLegendItem(legend: d3.Selection<SVGGElement, unknown, null, undefined>, y: number, color: string, text: string) {
  legend.append("rect")
      .attr("x", 0)
      .attr("y", y)
      .attr("width", 10)
      .attr("height", 10)
      .style("fill", color);

  legend.append("text")
      .attr("x", 20)
      .attr("y", y + 10)
      .text(text)
      .style("font-size", "12px")
      .attr("alignment-baseline", "middle");
}
</script>