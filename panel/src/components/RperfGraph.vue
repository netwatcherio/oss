<template>
  <div ref="rperfGraph"></div>
</template>

<script lang="ts">
import { ref, onMounted, watch, onUnmounted } from 'vue';
import * as d3 from 'd3';
import type {PingResult, ProbeData, RPerfResults} from '@/types'; // Import your PingResult type

// todo
/*
latency graph is recalculated when zoomed and just reupdates the time instead??

 */


export default {
  name: 'RperfGraph',
  props: {
    rperfResults: Array as () => RPerfResults[],
  },
  setup(props: { rperfResults: RPerfResults[]; }) {
    const rperfGraph = ref(null);

    const drawGraph = () => {
      if (!rperfGraph.value || !props.rperfResults || props.rperfResults.length === 0) {
        return;
      }
      createGraph(props.rperfResults, rperfGraph.value);
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

    watch(() => props.rperfResults, drawGraph, { immediate: true });

    return { rperfGraph };
  },
};


// Define a threshold for the maximum allowed gap (in milliseconds)
const maxAllowedGap = 1000 * 90; // Example: 5 minutes

function isGapAcceptable(current: RPerfResults, previous: RPerfResults) {
  if (!previous) return true; // Always accept the first point
  return (current.stopTimestamp.getTime() - previous.stopTimestamp.getTime()) <= maxAllowedGap;
}

function segmentData(data: RPerfResults[]) {
  // First, sort the data by stopTimestamp
  data.sort((a, b) => new Date(a.stopTimestamp) - new Date(b.stopTimestamp));

  const segments = [];
  let segment = [];

  for (let i = 0; i < data.length; i++) {
    const current = data[i];
    const next = data[i + 1];

    segment.push(current);

    if (next) {
      const currentStopTime = new Date(current.stopTimestamp).getTime();
      const nextStopTime = new Date(next.stopTimestamp).getTime();

      if (nextStopTime - currentStopTime > maxAllowedGap) {
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


// Repeat for maxLine, avgLine, and lossLine

function createGraph(data: RPerfResults[], graphElement: HTMLElement) {
  const margin = {top: 20, right: 20, bottom: 30, left: 50};
  const width = graphElement.clientWidth - margin.left - margin.right;
  const height = 400 - margin.top - margin.bottom;

  d3.select(graphElement).selectAll('*').remove();

  const packetLossColorScale = d3.scaleLinear<string>()
      .domain([5, 10, 20]) // Assuming packet loss is given as a percentage
      .range(['yellow', 'orange', 'red'] as any[]); // Cast the color range as any[] to satisfy TypeScript

  // Draw packet loss areas

  // Create SVG element
  const svg = d3.select(graphElement)
      .append('svg')
      .attr('width', width + margin.left + margin.right)
      .attr('height', height + margin.top + margin.bottom)
      .append('g')
      .attr('transform', `translate(${margin.left},${margin.top})`);

  const xScaleOrig = d3.scaleTime()
      .domain(d3.extent(data, (d: { stopTimestamp: any; }) => d.stopTimestamp))
      .range([0, width]);

  // Define scales
  let xScale = d3.scaleTime()
      .domain(d3.extent(data, (d: { stopTimestamp: any; }) => d.stopTimestamp))
      .range([0, width]);

  const yScale = d3.scaleLinear()
      .domain([0, 20]/*d3.max(data, (d: { summary: {
          bytesReceived: number;
          bytesSent: number;
          durationReceive: number;
          durationSend: number;
          framedPacketSize: number;
          jitterAverage: number;
          jitterPacketsConsecutive: number;
          packetsDuplicated: number;
          packetsLost: number;
          packetsOutOfOrder: number;
          packetsReceived: number;
          packetsSent: number;
        }}) => d.summary.packetsLost)]*/)
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
    svg.selectAll(".line-loss").attr("d", lossLine);
    svg.selectAll(".line-ooo").attr("d", outOfOrder);
    svg.selectAll(".line-packetsduplicate").attr("d", packetsDuplicated);
    // Handle any other lines or elements that need to be updated
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
    xScale.domain(d3.extent(data, d => d.stopTimestamp));
    svg.select(".x-axis").call(d3.axisBottom(xScale));
    updateLines();
  });


  // Add brushing
  svg.append("g")
      .attr("class", "brush")
      .call(brush);

  // Add X axis
  svg.append('g')
      .attr('transform', `translate(0,${height})`)
      .call(d3.axisBottom(xScale));

  // Add Y axis
  svg.append('g')
      .call(d3.axisLeft(yScale));

  const dataSegments = segmentData(data);

  data.forEach((d) => {
    if (d.summary.packetsLost > 0) { // Assuming packetLoss is a property of the data
      const packetLossWidth = 5; // Fixed width for packet loss indicators, adjust as needed
      svg.append('rect')
          .attr('x', xScale(new Date(d.stopTimestamp)) - packetLossWidth / 2)
          .attr('y', 0)
          .attr('width', packetLossWidth)
          .attr('height', height)
          .attr('fill', packetLossColorScale(d.summary.packetsLost))
          .attr('opacity', 0.25); // Semi-translucent
    }
  });


  for (let i = 0; i < data.length - 1; i++) {
    const currentStopTime = new Date(data[i].stopTimestamp).getTime();
    const nextStopTime = new Date(data[i + 1].stopTimestamp).getTime();

    if (nextStopTime - currentStopTime > maxAllowedGap) {
      svg.append('rect')
          .attr('x', xScale(currentStopTime))
          .attr('y', 0)
          .attr('width', xScale(nextStopTime) - xScale(currentStopTime))
          .attr('height', height)
          .attr('fill', '#ddd') // Light grey color for gaps
          .attr('opacity', 0.2); // Semi-translucent
    }
  }

  console.log(data)

  // Line generator for avgRtt
  const lossLineRperf = d3.line<RPerfResults>()
      .x((d: { stopTimestamp: any; }) => xScale(d.stopTimestamp))
      .y((d: { summary: { packetsLost: number; } }) => yScale(d.summary.packetsLost));
  const outOfOrderRperf = d3.line<RPerfResults>()
      .x((d: { stopTimestamp: any; }) => xScale(d.stopTimestamp))
      .y((d: { summary: { packetsOutOfOrder: number; } }) => yScale(d.summary.packetsOutOfOrder));
  const packetsDuplicatedRperf = d3.line<RPerfResults>()
      .x((d: { stopTimestamp: any; }) => xScale(d.stopTimestamp))
      .y((d: { summary: { packetsDuplicated: number; } }) => yScale(d.summary.packetsDuplicated));
  // Repeat for maxLine, avgLine, and lossLine

  // Draw each segment separately
  // Draw each segment separately
  dataSegments.forEach(segment => {
    // Append paths for each line (average, maximum, standard deviation, loss)
    appendPath(segment, 'line-loss', lossLineRperf, 'red');
    appendPath(segment, 'line-ooo', outOfOrderRperf, 'blue');
    appendPath(segment, 'line-packetsduplicate', packetsDuplicatedRperf, 'purple');
  });

  function appendPath(segment: RPerfResults[], className: string | number | boolean | readonly (string | number)[] | d3.ValueFn<SVGPathElement, any, string | number | boolean | readonly (string | number)[] | null> | null, lineFunction: string | number | boolean | d3.Line<PingResult> | readonly (string | number)[] | d3.ValueFn<SVGPathElement, any, string | number | boolean | readonly (string | number)[] | null> | null, color: string | number | boolean | readonly (string | number)[] | d3.ValueFn<SVGPathElement, any, string | number | boolean | readonly (string | number)[] | null> | null) {
    svg.append('path')
        .datum(segment)
        .attr('class', className)
        .attr('fill', 'none')
        .attr('stroke', color)
        .attr('stroke-width', 1.5)
        .attr('d', lineFunction)
        .attr("clip-path", "url(#clip)");
  }

  // Adding brushing and zoom out on double click
  svg.append("g")
      .attr("class", "brush")
      .call(brush);

  svg.on("dblclick", function () {
    xScale.domain(d3.extent(data, d => d.stopTimestamp));
    svg.select(".x-axis").call(d3.axisBottom(xScale));
    updateLines();
  });

  // Adding legend
  const legend = svg.append("g")
      .attr("class", "legend")
      .attr("transform", `translate(${width - 120},${20})`); // Adjust legend position

  // Legend for avgRtt
  legend.append("rect")
      .attr("x", 0)
      .attr("y", 0)
      .attr("width", 10)
      .attr("height", 10)
      .style("fill", "purple");

  legend.append("text")
      .attr("x", 20)
      .attr("y", 10)
      .text("Duplicate")
      .style("font-size", "12px")
      .attr("alignment-baseline", "middle");

  // Legend for maxRtt
  legend.append("rect")
      .attr("x", 0)
      .attr("y", 20)
      .attr("width", 10)
      .attr("height", 10)
      .style("fill", "blue");

  legend.append("text")
      .attr("x", 20)
      .attr("y", 30)
      .text("Out of Order")
      .style("font-size", "12px")
      .attr("alignment-baseline", "middle");

  // Legend for packetLoss
  legend.append("rect")
      .attr("x", 0)
      .attr("y", 40)
      .attr("width", 10)
      .attr("height", 10)
      .style("fill", "red");

  legend.append("text")
      .attr("x", 20)
      .attr("y", 50)
      .text("Packet Loss")
      .style("font-size", "12px")
      .attr("alignment-baseline", "middle");

  /*  legend.append("rect")
      .attr("x", 0)
      .attr("y", 60)
      .attr("width", 10)
      .attr("height", 10)
      .style("fill", "lightblue");

  legend.append("text")
      .attr("x", 20)
      .attr("y", 70)
      .text("Standard Deviation")
      .style("font-size", "12px")
      .attr("alignment-baseline","middle");
}*/
}
</script>
