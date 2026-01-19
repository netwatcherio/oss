<template>
  <div class="network-map-container">
    <div class="controls">
      <button @click="resetView" class="control-btn">Reset View</button>
      <button @click="toggleLayout" class="control-btn">
        {{ layoutMode === 'force' ? 'Hierarchical' : 'Force' }} Layout
      </button>
      <select v-model="colorMode" class="control-select">
        <option value="combined">Combined Metrics</option>
        <option value="latency">Latency Only</option>
        <option value="packetLoss">Packet Loss Only</option>
      </select>
    </div>
    <div ref="containerRef" class="network-map"></div>
  </div>
</template>

<script lang="ts">
import { ref, onMounted, watch, onUnmounted } from 'vue';
import * as d3 from 'd3';
import type { MtrResult } from '@/types';

export default {
  name: 'NetworkMap',
  props: {
    mtrResults: {
      type: Array as () => MtrResult[],
      required: true,
    },
  },
  emits: ['node-select'],
  setup(props, { emit }) {
    const containerRef = ref<HTMLElement | null>(null);
    const colorMode = ref<'combined' | 'latency' | 'packetLoss'>('combined');
    const layoutMode = ref<'force' | 'hierarchical'>('hierarchical');
    
    const onNodeClick = (node: any) => {
      emit('node-select', {
        id: node.id,
        hostname: node.hostname,
        ip: node.ip,
        hopNumber: node.hopNumber,
        latency: node.latency,
        packetLoss: node.packetLoss,
      });
    };
    
    let visualization: NetworkVisualization | null = null;

    // console.log(props.mtrResults)

    const resetView = () => {
      visualization?.resetZoom();
    };

    const toggleLayout = () => {
      layoutMode.value = layoutMode.value === 'force' ? 'hierarchical' : 'force';
      if (visualization) {
        visualization.setLayout(layoutMode.value);
      }
    };

    const createVisualization = () => {
      if (!containerRef.value || !props.mtrResults.length) return;
      
      if (visualization) {
        visualization.destroy();
      }
      
      visualization = new NetworkVisualization(
        containerRef.value,
        props.mtrResults,
        colorMode.value,
        layoutMode.value,
        onNodeClick
      );
    };

    onMounted(() => {
      createVisualization();
    });

    onUnmounted(() => {
      visualization?.destroy();
    });

    watch([() => props.mtrResults, colorMode], () => {
      createVisualization();
    }, { deep: true });

    return {
      containerRef,
      colorMode,
      layoutMode,
      resetView,
      toggleLayout,
    };
  },
};

// Separate the D3 logic into a class for better organization
class NetworkVisualization {
  private container: HTMLElement;
  private svg!: d3.Selection<SVGSVGElement, unknown, null, undefined>;
  private g!: d3.Selection<SVGGElement, unknown, null, undefined>;
  private simulation!: d3.Simulation<Node, Link>;
  private zoom!: d3.ZoomBehavior<Element, unknown>;
  private tooltip!: d3.Selection<HTMLDivElement, unknown, null, undefined>;
  private nodes: Node[] = [];
  private links: Link[] = [];
  private width: number;
  private height: number;
  private nodeRadius = 22;
  private margin = { top: 60, right: 20, bottom: 100, left: 20 };
  private onNodeClick?: (node: Node) => void;

  constructor(
    container: HTMLElement,
    mtrResults: MtrResult[],
    private colorMode: 'combined' | 'latency' | 'packetLoss',
    private layoutMode: 'force' | 'hierarchical',
    onNodeClick?: (node: Node) => void
  ) {
    this.container = container;
    this.onNodeClick = onNodeClick;
    this.width = container.clientWidth - this.margin.left - this.margin.right;
    this.height = 600 - this.margin.top - this.margin.bottom;
    
    this.processData(mtrResults);
    this.initializeSVG();
    this.createVisualization();
  }

  private processData(mtrResults: MtrResult[]) {
    const nodeMap = new Map<string, Node>();
    const linkMap = new Map<string, Link>();

    mtrResults.forEach((result, pathIndex) => {
      // Guard: skip results with missing or malformed data
      if (!result?.report?.hops) return;
      
      let prevNode: Node | null = null;

      result.report.hops.forEach((hop, hopIndex) => {
        const hopNum = hopIndex + 1;
        let nodeId: string;
        let hostname: string | undefined;
        let ip: string | undefined;
        let isUnknown = false;

        if (hop.hosts && hop.hosts.length > 0) {
          hostname = hop.hosts[0].hostname;
          ip = hop.hosts[0].ip;
          nodeId = ip || `hop-${hopNum}`;
        } else {
          // Use same ID for all unknown hosts at the same hop number
          nodeId = `unknown-hop-${hopNum}`;
          isUnknown = true;
        }

        let node = nodeMap.get(nodeId);
        if (!node) {
          node = {
            id: nodeId,
            label: isUnknown ? '?' : `${hopNum}`,
            hopNumber: hopNum,
            paths: new Set([pathIndex]),
            packetLoss: parseFloat(hop.loss_pct || '0'),
            latency: parseFloat(hop.avg || '0'),
            hostname,
            ip,
            isUnknown,
          };
          nodeMap.set(nodeId, node);
        } else {
          // Update metrics for combined nodes (especially important for unknown hosts)
          const oldSize = node.paths.size;
          node.paths.add(pathIndex);
          
          // Average the metrics across all paths
          node.packetLoss = (node.packetLoss * oldSize + parseFloat(hop.loss_pct || '0')) / node.paths.size;
          node.latency = (node.latency * oldSize + parseFloat(hop.avg || '0')) / node.paths.size;
        }

        if (prevNode) {
          const linkId = `${prevNode.id}->${node.id}`;
          let link = linkMap.get(linkId);
          if (!link) {
            link = {
              id: linkId,
              source: prevNode.id,
              target: node.id,
              paths: new Set([pathIndex]),
            };
            linkMap.set(linkId, link);
          } else {
            link.paths.add(pathIndex);
          }
        }

        prevNode = node;
      });
    });

    this.nodes = Array.from(nodeMap.values());
    this.links = Array.from(linkMap.values());
  }

  private initializeSVG() {
    // Clear container
    d3.select(this.container).selectAll('*').remove();

    // Create SVG
    this.svg = d3.select(this.container)
      .append('svg')
      .attr('width', this.width + this.margin.left + this.margin.right)
      .attr('height', this.height + this.margin.top + this.margin.bottom);

    // Create main group
    this.g = this.svg.append('g')
      .attr('transform', `translate(${this.margin.left},${this.margin.top})`);

    // Create tooltip
    this.tooltip = d3.select(this.container)
      .append('div')
      .attr('class', 'network-tooltip')
      .style('position', 'absolute')
      .style('visibility', 'hidden')
      .style('background', 'rgba(0, 0, 0, 0.9)')
      .style('color', 'white')
      .style('padding', '10px')
      .style('border-radius', '5px')
      .style('font-size', '14px')
      .style('pointer-events', 'none')
      .style('z-index', '1000');

    // Setup zoom
    this.zoom = d3.zoom()
      .scaleExtent([0.3, 5])
      .on('zoom', (event) => {
        this.g.attr('transform', event.transform);
      });

    this.svg.call(this.zoom);

    // Add title
    this.svg.append('text')
      .attr('x', (this.width + this.margin.left + this.margin.right) / 2)
      .attr('y', 30)
      .attr('text-anchor', 'middle')
      .attr('class', 'network-title')
      .style('font-size', '20px')
      .style('font-weight', 'bold')
      .style('fill', '#c0caf5')
      .text('Network Topology');
  }

  private createVisualization() {
    // Create force simulation
    this.simulation = d3.forceSimulation<Node>(this.nodes)
      .force('link', d3.forceLink<Node, Link>(this.links)
        .id(d => d.id)
        .distance(100))
      .force('charge', d3.forceManyBody().strength(-300))
      .force('collision', d3.forceCollide(this.nodeRadius + 5));

    if (this.layoutMode === 'hierarchical') {
      this.applyHierarchicalLayout();
    } else {
      this.simulation.force('center', d3.forceCenter(this.width / 2, this.height / 2));
    }

    // Create links
    const linkSelection = this.g.append('g')
      .attr('class', 'links')
      .selectAll('line')
      .data(this.links)
      .enter()
      .append('line')
      .attr('stroke', '#999')
      .attr('stroke-opacity', 0.6)
      .attr('stroke-width', d => Math.sqrt(d.paths.size) * 2);

    // Create nodes
    const nodeSelection = this.g.append('g')
      .attr('class', 'nodes')
      .selectAll('g')
      .data(this.nodes)
      .enter()
      .append('g')
      .attr('class', 'node')
      .call(this.createDragBehavior());

    // Add circles
    nodeSelection.append('circle')
      .attr('r', this.nodeRadius)
      .attr('fill', d => this.getNodeColor(d))
      .attr('stroke', '#fff')
      .attr('stroke-width', 2)
      .style('cursor', 'pointer');

    // Add labels
    nodeSelection.append('text')
      .attr('dy', 5)
      .attr('text-anchor', 'middle')
      .attr('fill', 'white')
      .style('font-weight', 'bold')
      .style('pointer-events', 'none')
      .text(d => d.label);

    // Add hover and click interactions
    nodeSelection
      .on('mouseenter', (event, d) => this.showTooltip(event, d))
      .on('mousemove', (event) => this.moveTooltip(event))
      .on('mouseleave', () => this.hideTooltip())
      .on('click', (event, d) => {
        event.stopPropagation();
        if (this.onNodeClick) this.onNodeClick(d);
      });

    // Update positions on tick
    this.simulation.on('tick', () => {
      linkSelection
        .attr('x1', d => (d.source as Node).x!)
        .attr('y1', d => (d.source as Node).y!)
        .attr('x2', d => (d.target as Node).x!)
        .attr('y2', d => (d.target as Node).y!);

      nodeSelection
        .attr('transform', d => `translate(${d.x},${d.y})`);
    });

    // Add legend
    this.createLegend();
  }

  private applyHierarchicalLayout() {
    const xScale = d3.scaleLinear()
      .domain([1, d3.max(this.nodes, d => d.hopNumber)!])
      .range([50, this.width - 50]);

    const nodesByHop = d3.group(this.nodes, d => d.hopNumber);
    
    nodesByHop.forEach((nodes, hop) => {
      const x = xScale(hop);
      const ySpacing = this.height / (nodes.length + 1);
      
      nodes.forEach((node, i) => {
        node.fx = x;
        node.fy = ySpacing * (i + 1);
      });
    });

    this.simulation
      .force('x', d3.forceX<Node>(d => d.fx!).strength(1))
      .force('y', d3.forceY<Node>(d => d.fy!).strength(1));
  }

  private createDragBehavior() {
    return d3.drag<SVGGElement, Node>()
      .on('start', (event, d) => {
        if (!event.active) this.simulation.alphaTarget(0.3).restart();
        d.fx = d.x;
        d.fy = d.y;
      })
      .on('drag', (event, d) => {
        d.fx = event.x;
        d.fy = event.y;
      })
      .on('end', (event, d) => {
        if (!event.active) this.simulation.alphaTarget(0);
        if (this.layoutMode === 'force') {
          d.fx = null;
          d.fy = null;
        }
      });
  }

  private showTooltip(event: MouseEvent, d: Node) {
    let title = d.hostname || 'Unknown Host';
    if (d.isUnknown) {
      title = `Unknown Host (Hop ${d.hopNumber})`;
    }
    
    const html = `
      <strong>${title}</strong><br/>
      ${d.ip ? `IP: ${d.ip}<br/>` : ''}
      ${!d.isUnknown ? `Hop: ${d.hopNumber}<br/>` : ''}
      Latency: ${d.latency.toFixed(2)} ms${d.paths.size > 1 ? ' (avg)' : ''}<br/>
      Packet Loss: ${d.packetLoss.toFixed(1)}%${d.paths.size > 1 ? ' (avg)' : ''}<br/>
      <!--Paths: ${Array.from(d.paths).join(', ')} (${d.paths.size} total)-->
    `;
    
    this.tooltip
      .html(html)
      .style('visibility', 'visible');
    
    this.moveTooltip(event);
  }

  private moveTooltip(event: MouseEvent) {
    const tooltipNode = this.tooltip.node();
    if (!tooltipNode) return;
    
    const rect = this.container.getBoundingClientRect();
    const x = event.clientX - rect.left + 10;
    const y = event.clientY - rect.top - 10;
    
    this.tooltip
      .style('left', `${x}px`)
      .style('top', `${y}px`);
  }

  private hideTooltip() {
    this.tooltip.style('visibility', 'hidden');
  }

  private getNodeColor(node: Node): string {
    if (node.isUnknown) return '#999';
    
    const packetLossColors = ['#22c55e', '#84cc16', '#eab308', '#f97316', '#ef4444'];
    const latencyColors = ['#22c55e', '#84cc16', '#eab308', '#f97316', '#ef4444'];
    
    const plIndex = Math.min(Math.floor(node.packetLoss / 20), 4);
    const latIndex = Math.min(Math.floor(node.latency / 40), 4);
    
    switch (this.colorMode) {
      case 'packetLoss':
        return packetLossColors[plIndex];
      case 'latency':
        return latencyColors[latIndex];
      default:
        const plColor = packetLossColors[plIndex];
        const latColor = latencyColors[latIndex];
        return d3.interpolateRgb(plColor, latColor)(0.5);
    }
  }

  private createLegend() {
    const legendData = [
      { color: '#22c55e', label: 'Excellent' },
      { color: '#84cc16', label: 'Good' },
      { color: '#eab308', label: 'Fair' },
      { color: '#f97316', label: 'Poor' },
      { color: '#ef4444', label: 'Critical' },
      { color: '#999', label: 'Unknown' },
    ];

    const legend = this.svg.append('g')
      .attr('transform', `translate(${this.margin.left},${this.height + this.margin.top + 40})`);

    const items = legend.selectAll('.legend-item')
      .data(legendData)
      .enter()
      .append('g')
      .attr('transform', (d, i) => `translate(${i * 100},0)`);

    items.append('rect')
      .attr('width', 18)
      .attr('height', 18)
      .attr('fill', d => d.color)
      .attr('stroke', '#fff')
      .attr('stroke-width', 1);

    items.append('text')
      .attr('x', 24)
      .attr('y', 14)
      .attr('class', 'legend-text')
      .text(d => d.label)
      .style('font-size', '12px')
      .style('fill', '#c0caf5');
  }

  public setLayout(mode: 'force' | 'hierarchical') {
    this.layoutMode = mode;
    
    if (mode === 'hierarchical') {
      this.applyHierarchicalLayout();
    } else {
      // Reset fixed positions
      this.nodes.forEach(node => {
        node.fx = null;
        node.fy = null;
      });
      this.simulation.force('center', d3.forceCenter(this.width / 2, this.height / 2));
    }
    
    this.simulation.alpha(1).restart();
  }

  public resetZoom() {
    this.svg.transition()
      .duration(750)
      .call(this.zoom.transform, d3.zoomIdentity);
  }

  public destroy() {
    this.simulation.stop();
    d3.select(this.container).selectAll('*').remove();
  }
}

// Type definitions
interface Node extends d3.SimulationNodeDatum {
  id: string;
  label: string;
  hopNumber: number;
  paths: Set<number>;
  packetLoss: number;
  latency: number;
  hostname?: string;
  ip?: string;
  isUnknown: boolean;
}

interface Link extends d3.SimulationLinkDatum<Node> {
  id: string;
  paths: Set<number>;
}
</script>

<style scoped>
.network-map-container {
  position: relative;
  width: 100%;
  min-height: 700px;
  background: #f8f9fa;  /* Light mode default */
  border-radius: 8px;
  overflow: hidden;
}

/* Dark mode container */
:global([data-theme="dark"]) .network-map-container {
  background: #0f172a;
}

.controls {
  position: absolute;
  top: 10px;
  right: 10px;
  z-index: 100;
  background: white;  /* Light mode default */
  padding: 12px;
  border-radius: 6px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  display: flex;
  gap: 10px;
  align-items: center;
  border: 1px solid #e5e7eb;
}

:global([data-theme="dark"]) .controls {
  background: #1e293b;
  border-color: #374151;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
}

.control-btn {
  padding: 6px 12px;
  background: #3b82f6;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-size: 14px;
  transition: background 0.2s;
}

.control-btn:hover {
  background: #2563eb;
}

.control-select {
  padding: 6px 12px;
  border: 1px solid #e5e7eb;
  border-radius: 4px;
  font-size: 14px;
  background: white;  /* Light mode default */
  color: #374151;
  cursor: pointer;
}

:global([data-theme="dark"]) .control-select {
  background: #1e293b;
  border-color: #374151;
  color: #e2e8f0;
}

.control-select option {
  background: white;
  color: #374151;
}

:global([data-theme="dark"]) .control-select option {
  background: #1e293b;
  color: #e2e8f0;
}

.network-map {
  width: 100%;
  height: 700px;
  position: relative;
}

/* Global styles for D3 elements */
:global(.network-tooltip) {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
  background: #1e293b !important;
  border: 1px solid #374151;
}

:global(.node circle) {
  transition: transform 0.2s;
}

:global(.node:hover circle) {
  transform: scale(1.1);
}

/* Light mode overrides */
:global([data-theme="light"]) .network-map-container {
  background: #f8f9fa;
}

:global([data-theme="light"]) .controls {
  background: white;
  border-color: #e5e7eb;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
}

:global([data-theme="light"]) .control-select {
  background: white;
  border-color: #e5e7eb;
  color: #374151;
}

:global([data-theme="light"]) .control-select option {
  background: white;
  color: #374151;
}

:global([data-theme="light"]) .network-tooltip {
  background: white !important;
  color: #374151 !important;
  border-color: #e5e7eb !important;
}

:global([data-theme="light"]) .network-title {
  fill: #1f2937 !important;
}

:global([data-theme="light"]) .legend-text {
  fill: #374151 !important;
}
</style>