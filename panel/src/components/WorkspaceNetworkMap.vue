<template>
  <div class="workspace-network-map-container">
    <div class="map-header">
      <h3 class="map-title">
        <i class="bi bi-diagram-3"></i>
        Network Topology Map
        <span v-if="isLive" class="live-badge" :class="{ pulse: isPulsing }">
          <i class="bi bi-broadcast"></i> Live
        </span>
      </h3>
      <div class="controls">
        <button @click="refreshData" class="control-btn" :disabled="loading">
          <i class="bi bi-arrow-clockwise" :class="{ 'spin': loading }"></i>
          Refresh
        </button>
        <button @click="resetView" class="control-btn">
          <i class="bi bi-aspect-ratio"></i>
          Reset View
        </button>
        <button @click="toggleLayout" class="control-btn">
          <i class="bi bi-grid-3x3-gap"></i>
          {{ layoutMode === 'force' ? 'Hierarchical' : 'Force' }}
        </button>
        <select v-model="colorMode" class="control-select">
          <option value="combined">Combined Metrics</option>
          <option value="latency">Latency Only</option>
          <option value="packetLoss">Packet Loss Only</option>
        </select>
      </div>
    </div>
    
    <div v-if="loading && !mapData" class="loading-state">
      <div class="spinner"></div>
      <p>Loading network topology...</p>
    </div>
    
    <div v-else-if="!mapData || mapData.nodes.length === 0" class="empty-state">
      <i class="bi bi-diagram-3"></i>
      <h5>No Network Data Available</h5>
      <p>Ensure agents have active MTR or PING probes configured.</p>
    </div>
    
    <div v-else class="map-content">
      <div ref="containerRef" class="network-map"></div>
      
      <!-- Destination Summary Panel -->
      <div v-if="mapData?.destinations?.length" class="destinations-panel">
        <h5 class="panel-title">
          <i class="bi bi-geo-alt"></i>
          Destination Overview
        </h5>
        <div class="destinations-table-wrapper">
          <table class="destinations-table">
            <thead>
              <tr>
                <th>Status</th>
                <th>Target</th>
                <th>Hops</th>
                <th>Latency</th>
                <th>Loss</th>
                <th>Agents</th>
                <th>Probes</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="dest in mapData.destinations" :key="dest.target" 
                  @click="highlightDestination(dest.target)"
                  class="dest-row">
                <td>
                  <span class="status-indicator" :class="'status-' + dest.status">
                    <i :class="getStatusIcon(dest.status)"></i>
                  </span>
                </td>
                <td class="target-cell">
                  <div class="target-name">{{ dest.hostname || dest.target }}</div>
                  <div v-if="dest.hostname && dest.hostname !== dest.target" class="target-ip">{{ dest.target }}</div>
                </td>
                <td class="text-center">{{ dest.hop_count || '-' }}</td>
                <td :class="getLatencyClass(dest.avg_latency)">
                  {{ dest.avg_latency?.toFixed(1) || '0' }} ms
                </td>
                <td :class="getPacketLossClass(dest.packet_loss)">
                  {{ dest.packet_loss?.toFixed(1) || '0' }}%
                </td>
                <td class="text-center">{{ dest.agent_count }}</td>
                <td>
                  <span v-for="pt in dest.probe_types" :key="pt" 
                        class="probe-badge" :class="'probe-' + pt.toLowerCase()">
                    {{ pt }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
    
    <div v-if="selectedNode" class="node-detail-panel">
      <button class="close-btn" @click="selectedNode = null">
        <i class="bi bi-x-lg"></i>
      </button>
      <h4>{{ selectedNode.label || selectedNode.ip || 'Unknown' }}</h4>
      <div class="detail-row" v-if="selectedNode.type">
        <span class="label">Type:</span>
        <span class="value badge" :class="'badge-' + selectedNode.type">{{ selectedNode.type }}</span>
      </div>
      <div class="detail-row" v-if="selectedNode.ip">
        <span class="label">IP:</span>
        <span class="value mono">{{ selectedNode.ip }}</span>
      </div>
      <div class="detail-row" v-if="selectedNode.hostname">
        <span class="label">Hostname:</span>
        <span class="value">{{ selectedNode.hostname }}</span>
      </div>
      <div class="detail-row" v-if="selectedNode.hop_number">
        <span class="label">Hop:</span>
        <span class="value">#{{ selectedNode.hop_number }}</span>
      </div>
      <div class="detail-row">
        <span class="label">Avg Latency:</span>
        <span class="value" :class="getLatencyClass(selectedNode.avg_latency)">
          {{ selectedNode.avg_latency?.toFixed(2) || '0' }} ms
        </span>
      </div>
      <div class="detail-row">
        <span class="label">Packet Loss:</span>
        <span class="value" :class="getPacketLossClass(selectedNode.packet_loss)">
          {{ selectedNode.packet_loss?.toFixed(1) || '0' }}%
        </span>
      </div>
      <div class="detail-row" v-if="selectedNode.path_count">
        <span class="label">Paths:</span>
        <span class="value">{{ selectedNode.path_count }}</span>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, onMounted, watch, onUnmounted, computed } from 'vue';
import * as d3 from 'd3';
import type { NetworkMapNode, NetworkMapEdge, NetworkMapData } from '@/types';
import { useWebSocket } from '@/composables/useWebSocket';
import request from '@/services/request';

const props = defineProps<{
  workspaceId: number;
  initialData?: NetworkMapData;
}>();

const emit = defineEmits<{
  (e: 'node-select', node: NetworkMapNode): void;
  (e: 'refresh'): void;
}>();

const containerRef = ref<HTMLElement | null>(null);
const colorMode = ref<'combined' | 'latency' | 'packetLoss'>('combined');
const layoutMode = ref<'force' | 'hierarchical'>('hierarchical');
const loading = ref(false);
const mapData = ref<NetworkMapData | null>(props.initialData || null);
const selectedNode = ref<NetworkMapNode | null>(null);
const isPulsing = ref(false);
const isLive = ref(false);

let visualization: WorkspaceNetworkVisualization | null = null;

// WebSocket subscription for live updates
const { subscribe, connected } = useWebSocket();

const fetchMapData = async () => {
  loading.value = true;
  try {
    const response = await request.get<NetworkMapData>(
      `/workspaces/${props.workspaceId}/network-map?lookback=60`
    );
    console.log('[WorkspaceNetworkMap] Raw response:', response);
    console.log('[WorkspaceNetworkMap] Data:', response.data);
    mapData.value = response.data;
    console.log('[WorkspaceNetworkMap] mapData.value nodes:', mapData.value?.nodes?.length);
    console.log('[WorkspaceNetworkMap] containerRef:', containerRef.value);
    createVisualization();
  } catch (err) {
    console.error('[WorkspaceNetworkMap] Fetch error:', err);
  } finally {
    loading.value = false;
  }
};

const refreshData = () => {
  fetchMapData();
  emit('refresh');
};

const handleNetworkMapUpdate = (data: NetworkMapData) => {
  if (data.workspace_id === props.workspaceId) {
    mapData.value = data;
    isPulsing.value = true;
    setTimeout(() => { isPulsing.value = false; }, 500);
    createVisualization();
  }
};

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
  if (!containerRef.value || !mapData.value || mapData.value.nodes.length === 0) return;

  if (visualization) {
    visualization.destroy();
  }

  visualization = new WorkspaceNetworkVisualization(
    containerRef.value,
    mapData.value,
    colorMode.value,
    layoutMode.value,
    (node) => {
      selectedNode.value = node;
      emit('node-select', node);
    }
  );
};

const getLatencyClass = (latency: number) => {
  if (latency < 20) return 'text-success';
  if (latency < 50) return 'text-warning';
  return 'text-danger';
};

const getPacketLossClass = (loss: number) => {
  if (loss < 1) return 'text-success';
  if (loss < 5) return 'text-warning';
  return 'text-danger';
};

const getStatusIcon = (status: string) => {
  switch (status) {
    case 'healthy': return 'bi bi-check-circle-fill text-success';
    case 'degraded': return 'bi bi-exclamation-triangle-fill text-warning';
    case 'critical': return 'bi bi-x-circle-fill text-danger';
    default: return 'bi bi-question-circle text-muted';
  }
};

const highlightDestination = (target: string) => {
  // Find the destination node and select it
  const destNode = mapData.value?.nodes.find(n => n.id === target || n.ip === target);
  if (destNode) {
    selectedNode.value = destNode;
    emit('node-select', destNode);
  }
  // TODO: Implement path highlighting in visualization
};

onMounted(async () => {
  await fetchMapData();
  
  // Subscribe to workspace updates
  if (props.workspaceId) {
    subscribe(props.workspaceId, 0, (data: any) => {
      if (data.event === 'network_map_update') {
        handleNetworkMapUpdate(data.data);
      }
    });
    isLive.value = true;
  }
});

onUnmounted(() => {
  visualization?.destroy();
});

watch([colorMode, () => mapData.value], () => {
  createVisualization();
}, { deep: true });

watch(connected, (val) => {
  isLive.value = val;
});

// D3 Visualization Class
class WorkspaceNetworkVisualization {
  private container: HTMLElement;
  private svg!: d3.Selection<SVGSVGElement, unknown, null, undefined>;
  private g!: d3.Selection<SVGGElement, unknown, null, undefined>;
  private simulation!: d3.Simulation<D3Node, D3Link>;
  private zoom!: d3.ZoomBehavior<Element, unknown>;
  private tooltip!: d3.Selection<HTMLDivElement, unknown, null, undefined>;
  private nodes: D3Node[] = [];
  private links: D3Link[] = [];
  private width: number;
  private height: number;
  private nodeRadius = 24;
  private margin = { top: 40, right: 20, bottom: 60, left: 20 };
  private onNodeClick?: (node: NetworkMapNode) => void;

  constructor(
    container: HTMLElement,
    data: NetworkMapData,
    private colorMode: 'combined' | 'latency' | 'packetLoss',
    private layoutMode: 'force' | 'hierarchical',
    onNodeClick?: (node: NetworkMapNode) => void
  ) {
    this.container = container;
    this.onNodeClick = onNodeClick;
    this.width = container.clientWidth - this.margin.left - this.margin.right;
    this.height = 550 - this.margin.top - this.margin.bottom;

    this.processData(data);
    this.initializeSVG();
    this.createVisualization();
  }

  private processData(data: NetworkMapData) {
    // Convert to D3-compatible nodes
    this.nodes = data.nodes.map(n => ({
      ...n,
      id: n.id,
      x: undefined,
      y: undefined,
      fx: undefined,
      fy: undefined,
    }));

    // Convert links with source/target references
    this.links = data.edges.map(e => ({
      ...e,
      source: e.source,
      target: e.target,
    }));
  }

  private initializeSVG() {
    d3.select(this.container).selectAll('*').remove();

    this.svg = d3.select(this.container)
      .append('svg')
      .attr('width', this.width + this.margin.left + this.margin.right)
      .attr('height', this.height + this.margin.top + this.margin.bottom);

    // Add defs for gradients
    const defs = this.svg.append('defs');
    
    // Agent gradient
    const agentGrad = defs.append('linearGradient')
      .attr('id', 'agent-gradient')
      .attr('x1', '0%').attr('y1', '0%')
      .attr('x2', '0%').attr('y2', '100%');
    agentGrad.append('stop').attr('offset', '0%').attr('stop-color', '#3b82f6');
    agentGrad.append('stop').attr('offset', '100%').attr('stop-color', '#1d4ed8');

    this.g = this.svg.append('g')
      .attr('transform', `translate(${this.margin.left},${this.margin.top})`);

    // Tooltip
    this.tooltip = d3.select(this.container)
      .append('div')
      .attr('class', 'network-tooltip')
      .style('position', 'absolute')
      .style('visibility', 'hidden')
      .style('background', 'rgba(15, 23, 42, 0.95)')
      .style('color', 'white')
      .style('padding', '12px')
      .style('border-radius', '8px')
      .style('font-size', '13px')
      .style('pointer-events', 'none')
      .style('z-index', '1000')
      .style('box-shadow', '0 4px 12px rgba(0,0,0,0.3)');

    // Zoom
    this.zoom = d3.zoom()
      .scaleExtent([0.3, 4])
      .on('zoom', (event) => {
        this.g.attr('transform', event.transform);
      });

    this.svg.call(this.zoom);
  }

  private createVisualization() {
    // Create simulation
    this.simulation = d3.forceSimulation<D3Node>(this.nodes)
      .force('link', d3.forceLink<D3Node, D3Link>(this.links)
        .id(d => d.id)
        .distance(80))
      .force('charge', d3.forceManyBody().strength(-200))
      .force('collision', d3.forceCollide(this.nodeRadius + 8));

    if (this.layoutMode === 'hierarchical') {
      this.applyHierarchicalLayout();
    } else {
      this.simulation.force('center', d3.forceCenter(this.width / 2, this.height / 2));
    }

    // Links
    const linkSelection = this.g.append('g')
      .attr('class', 'links')
      .selectAll('line')
      .data(this.links)
      .enter()
      .append('line')
      .attr('stroke', d => this.getEdgeColor(d))
      .attr('stroke-opacity', 0.7)
      .attr('stroke-width', d => Math.max(2, Math.sqrt(d.path_count || 1) * 1.5));

    // Nodes
    const nodeSelection = this.g.append('g')
      .attr('class', 'nodes')
      .selectAll('g')
      .data(this.nodes)
      .enter()
      .append('g')
      .attr('class', 'node')
      .call(this.createDragBehavior());

    // Node circles
    nodeSelection.append('circle')
      .attr('r', d => d.type === 'agent' ? this.nodeRadius + 4 : this.nodeRadius)
      .attr('fill', d => this.getNodeColor(d))
      .attr('stroke', d => d.type === 'agent' ? '#1e40af' : '#475569')
      .attr('stroke-width', d => d.type === 'agent' ? 3 : 2)
      .style('cursor', 'pointer');

    // Node icons/labels
    nodeSelection.append('text')
      .attr('dy', 5)
      .attr('text-anchor', 'middle')
      .attr('fill', 'white')
      .style('font-weight', 'bold')
      .style('font-size', d => d.type === 'agent' ? '10px' : '12px')
      .style('pointer-events', 'none')
      .text(d => this.getNodeLabel(d));

    // Hover labels below
    nodeSelection.append('text')
      .attr('dy', this.nodeRadius + 16)
      .attr('text-anchor', 'middle')
      .attr('fill', '#94a3b8')
      .style('font-size', '10px')
      .style('pointer-events', 'none')
      .text(d => d.type === 'agent' ? d.label : (d.hostname || d.ip || '').slice(0, 15));

    // Interactions
    nodeSelection
      .on('mouseenter', (event, d) => this.showTooltip(event, d))
      .on('mousemove', (event) => this.moveTooltip(event))
      .on('mouseleave', () => this.hideTooltip())
      .on('click', (event, d) => {
        event.stopPropagation();
        if (this.onNodeClick) this.onNodeClick(d as NetworkMapNode);
      });

    // Tick
    this.simulation.on('tick', () => {
      linkSelection
        .attr('x1', d => (d.source as D3Node).x!)
        .attr('y1', d => (d.source as D3Node).y!)
        .attr('x2', d => (d.target as D3Node).x!)
        .attr('y2', d => (d.target as D3Node).y!);

      nodeSelection
        .attr('transform', d => `translate(${d.x},${d.y})`);
    });

    // Legend
    this.createLegend();
  }

  private applyHierarchicalLayout() {
    // Group nodes by type: agents left, hops middle, destinations right
    const agentNodes = this.nodes.filter(n => n.type === 'agent');
    const hopNodes = this.nodes.filter(n => n.type === 'hop');
    const destNodes = this.nodes.filter(n => n.type === 'destination');

    // Position agents on left
    agentNodes.forEach((node, i) => {
      node.fx = 50;
      node.fy = (this.height / (agentNodes.length + 1)) * (i + 1);
    });

    // Position destinations on right
    destNodes.forEach((node, i) => {
      node.fx = this.width - 50;
      node.fy = (this.height / (destNodes.length + 1)) * (i + 1);
    });

    // Hops spread by hop_number
    const maxHop = Math.max(...hopNodes.map(n => n.hop_number || 1));
    hopNodes.forEach((node) => {
      const hopNum = node.hop_number || 1;
      node.fx = 100 + ((this.width - 200) * (hopNum / (maxHop + 1)));
    });

    this.simulation
      .force('x', d3.forceX<D3Node>(d => d.fx || this.width / 2).strength(0.8))
      .force('y', d3.forceY<D3Node>(d => d.fy || this.height / 2).strength(0.3));
  }

  private createDragBehavior() {
    return d3.drag<SVGGElement, D3Node>()
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

  private getNodeLabel(node: D3Node): string {
    if (node.type === 'agent') {
      return node.is_online ? '●' : '○';
    }
    if (node.type === 'destination') {
      return '◎';
    }
    if (!node.ip) return '?';
    return `${node.hop_number || ''}`;
  }

  private getNodeColor(node: D3Node): string {
    if (node.type === 'agent') {
      return node.is_online ? '#3b82f6' : '#64748b';
    }
    if (node.type === 'destination') {
      return '#8b5cf6';
    }

    const colors = ['#22c55e', '#84cc16', '#eab308', '#f97316', '#ef4444'];
    let index = 0;

    switch (this.colorMode) {
      case 'packetLoss':
        index = Math.min(Math.floor((node.packet_loss || 0) / 20), 4);
        break;
      case 'latency':
        index = Math.min(Math.floor((node.avg_latency || 0) / 40), 4);
        break;
      default:
        const plIdx = Math.min(Math.floor((node.packet_loss || 0) / 20), 4);
        const latIdx = Math.min(Math.floor((node.avg_latency || 0) / 40), 4);
        index = Math.round((plIdx + latIdx) / 2);
    }

    return colors[index];
  }

  private getEdgeColor(edge: D3Link): string {
    const colors = ['#22c55e', '#84cc16', '#eab308', '#f97316', '#ef4444'];
    const latIdx = Math.min(Math.floor((edge.avg_latency || 0) / 40), 4);
    return colors[latIdx];
  }

  private showTooltip(event: MouseEvent, d: D3Node) {
    const typeLabels: Record<string, string> = {
      agent: '📡 Agent',
      hop: '🔗 Network Hop',
      destination: '🎯 Destination',
    };

    const html = `
      <strong>${typeLabels[d.type || 'hop']}</strong><br/>
      ${d.label ? `<span style="color:#94a3b8">Label:</span> ${d.label}<br/>` : ''}
      ${d.ip ? `<span style="color:#94a3b8">IP:</span> ${d.ip}<br/>` : ''}
      ${d.hostname ? `<span style="color:#94a3b8">Host:</span> ${d.hostname}<br/>` : ''}
      ${d.hop_number ? `<span style="color:#94a3b8">Hop:</span> #${d.hop_number}<br/>` : ''}
      <span style="color:#94a3b8">Latency:</span> ${(d.avg_latency || 0).toFixed(2)} ms<br/>
      <span style="color:#94a3b8">Packet Loss:</span> ${(d.packet_loss || 0).toFixed(1)}%<br/>
      ${d.path_count ? `<span style="color:#94a3b8">Paths:</span> ${d.path_count}` : ''}
    `;

    this.tooltip.html(html).style('visibility', 'visible');
    this.moveTooltip(event);
  }

  private moveTooltip(event: MouseEvent) {
    const rect = this.container.getBoundingClientRect();
    this.tooltip
      .style('left', `${event.clientX - rect.left + 15}px`)
      .style('top', `${event.clientY - rect.top - 10}px`);
  }

  private hideTooltip() {
    this.tooltip.style('visibility', 'hidden');
  }

  private createLegend() {
    const legendData = [
      { color: '#3b82f6', label: 'Agent (Online)', shape: 'circle' },
      { color: '#8b5cf6', label: 'Destination', shape: 'circle' },
      { color: '#22c55e', label: 'Excellent', shape: 'rect' },
      { color: '#eab308', label: 'Fair', shape: 'rect' },
      { color: '#ef4444', label: 'Critical', shape: 'rect' },
    ];

    const legend = this.svg.append('g')
      .attr('transform', `translate(${this.margin.left},${this.height + this.margin.top + 25})`);

    const items = legend.selectAll('.legend-item')
      .data(legendData)
      .enter()
      .append('g')
      .attr('transform', (_, i) => `translate(${i * 110},0)`);

    items.append('rect')
      .attr('width', 16)
      .attr('height', 16)
      .attr('rx', d => d.shape === 'circle' ? 8 : 2)
      .attr('fill', d => d.color);

    items.append('text')
      .attr('x', 22)
      .attr('y', 12)
      .style('font-size', '11px')
      .style('fill', '#94a3b8')
      .text(d => d.label);
  }

  public setLayout(mode: 'force' | 'hierarchical') {
    this.layoutMode = mode;
    if (mode === 'hierarchical') {
      this.applyHierarchicalLayout();
    } else {
      this.nodes.forEach(node => {
        if (node.type !== 'agent' && node.type !== 'destination') {
          node.fx = null;
          node.fy = null;
        }
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

// D3-specific interfaces
interface D3Node extends d3.SimulationNodeDatum {
  id: string;
  type: 'agent' | 'hop' | 'destination' | 'collapsed';
  label: string;
  agent_id?: number;
  ip?: string;
  hostname?: string;
  hop_number?: number;
  avg_latency: number;
  packet_loss: number;
  path_count: number;
  is_online?: boolean;
  layer?: number;
  collapsed_hops?: number;
  status?: 'healthy' | 'degraded' | 'critical' | 'unknown';
  fx?: number | null;
  fy?: number | null;
}

interface D3Link extends d3.SimulationLinkDatum<D3Node> {
  id: string;
  source: string | D3Node;
  target: string | D3Node;
  avg_latency: number;
  packet_loss: number;
  path_count: number;
}

</script>

<style scoped>
/* Theme variables - defaults to dark mode */
.workspace-network-map-container {
  --map-bg: linear-gradient(135deg, #0f172a 0%, #1e293b 100%);
  --map-border: #334155;
  --map-header-bg: rgba(15, 23, 42, 0.8);
  --map-title-color: #e2e8f0;
  --map-text-muted: #94a3b8;
  --map-control-bg: #1e293b;
  --map-control-text: #e2e8f0;
  --map-control-border: #475569;
  --map-control-hover-bg: #334155;
  --map-panel-bg: rgba(15, 23, 42, 0.95);
  --map-value-color: #e2e8f0;
  
  position: relative;
  background: var(--map-bg);
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid var(--map-border);
  margin-bottom: 2rem;
}

/* Light mode overrides - use :root to check document theme */
:root[data-theme="light"] .workspace-network-map-container {
  --map-bg: linear-gradient(135deg, #f8fafc 0%, #e2e8f0 100%);
  --map-border: #cbd5e1;
  --map-header-bg: rgba(248, 250, 252, 0.9);
  --map-title-color: #1e293b;
  --map-text-muted: #64748b;
  --map-control-bg: #ffffff;
  --map-control-text: #334155;
  --map-control-border: #cbd5e1;
  --map-control-hover-bg: #f1f5f9;
  --map-panel-bg: rgba(255, 255, 255, 0.98);
  --map-value-color: #1e293b;
}

.map-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid var(--map-border);
  background: var(--map-header-bg);
  flex-wrap: wrap;
  gap: 12px;
}

.map-title {
  margin: 0;
  font-size: 1.1rem;
  font-weight: 600;
  color: var(--map-title-color);
  display: flex;
  align-items: center;
  gap: 10px;
}

.map-title i {
  color: #3b82f6;
}

.live-badge {
  font-size: 0.7rem;
  background: #22c55e;
  color: white;
  padding: 3px 8px;
  border-radius: 12px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.live-badge.pulse {
  animation: badge-pulse 0.5s ease-out;
}

@keyframes badge-pulse {
  0% { transform: scale(1); box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.7); }
  50% { transform: scale(1.1); box-shadow: 0 0 0 6px rgba(34, 197, 94, 0); }
  100% { transform: scale(1); }
}

.controls {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}

.control-btn {
  padding: 8px 14px;
  background: var(--map-control-bg);
  color: var(--map-control-text);
  border: 1px solid var(--map-control-border);
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  transition: all 0.2s;
  display: flex;
  align-items: center;
  gap: 6px;
}

.control-btn:hover:not(:disabled) {
  background: var(--map-control-hover-bg);
  border-color: var(--map-control-border);
}

.control-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.control-btn .spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.control-select {
  padding: 8px 12px;
  background: var(--map-control-bg);
  color: var(--map-control-text);
  border: 1px solid var(--map-control-border);
  border-radius: 6px;
  font-size: 13px;
  cursor: pointer;
}

.control-select option {
  background: var(--map-control-bg);
  color: var(--map-control-text);
}

.network-map {
  width: 100%;
  height: 550px;
  position: relative;
}

.loading-state, .empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 300px;
  color: var(--map-text-muted);
}

.empty-state i {
  font-size: 3rem;
  margin-bottom: 16px;
  color: var(--map-text-muted);
}

.empty-state h5 {
  color: var(--map-title-color);
}

.spinner {
  width: 40px;
  height: 40px;
  border: 3px solid var(--map-border);
  border-top-color: #3b82f6;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin-bottom: 16px;
}

.node-detail-panel {
  position: absolute;
  top: 80px;
  right: 20px;
  background: var(--map-panel-bg);
  border: 1px solid var(--map-control-border);
  border-radius: 10px;
  padding: 16px;
  min-width: 220px;
  box-shadow: 0 8px 24px rgba(0,0,0,0.2);
  z-index: 100;
}

.node-detail-panel h4 {
  margin: 0 0 12px 0;
  color: var(--map-title-color);
  font-size: 1rem;
  padding-right: 24px;
}

.close-btn {
  position: absolute;
  top: 12px;
  right: 12px;
  background: none;
  border: none;
  color: var(--map-text-muted);
  cursor: pointer;
  padding: 4px;
}

.close-btn:hover {
  color: var(--map-title-color);
}

.detail-row {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
  font-size: 13px;
}

.detail-row .label {
  color: var(--map-text-muted);
}

.detail-row .value {
  color: var(--map-value-color);
  font-weight: 500;
}

.detail-row .value.mono {
  font-family: monospace;
}

.badge {
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  text-transform: uppercase;
}

.badge-agent { background: #3b82f6; }
.badge-hop { background: #64748b; }
.badge-destination { background: #8b5cf6; }

.text-success { color: #22c55e !important; }
.text-warning { color: #eab308 !important; }
.text-danger { color: #ef4444 !important; }
.text-muted { color: #64748b !important; }

/* Map Content Container */
.map-content {
  display: flex;
  flex-direction: column;
}

/* Destinations Panel */
.destinations-panel {
  border-top: 1px solid var(--map-border);
  padding: 16px 20px;
  background: var(--map-header-bg);
}

.destinations-panel .panel-title {
  margin: 0 0 12px 0;
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--map-title-color);
  display: flex;
  align-items: center;
  gap: 8px;
}

.destinations-table-wrapper {
  overflow-x: auto;
}

.destinations-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.destinations-table th,
.destinations-table td {
  padding: 10px 12px;
  text-align: left;
  border-bottom: 1px solid var(--map-border);
}

.destinations-table th {
  color: var(--map-text-muted);
  font-weight: 500;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.destinations-table tbody tr {
  transition: background 0.2s;
  cursor: pointer;
}

.destinations-table tbody tr:hover {
  background: var(--map-control-hover-bg);
}

.dest-row .target-cell {
  max-width: 250px;
}

.dest-row .target-name {
  font-weight: 500;
  color: var(--map-title-color);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.dest-row .target-ip {
  font-family: monospace;
  font-size: 11px;
  color: var(--map-text-muted);
}

.status-indicator {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
}

.status-indicator.status-healthy i { color: #22c55e; }
.status-indicator.status-degraded i { color: #eab308; }
.status-indicator.status-critical i { color: #ef4444; }

.probe-badge {
  display: inline-block;
  padding: 2px 6px;
  margin-right: 4px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
}

.probe-mtr { background: rgba(59, 130, 246, 0.2); color: #3b82f6; }
.probe-ping { background: rgba(34, 197, 94, 0.2); color: #22c55e; }
.probe-trafficsim { background: rgba(139, 92, 246, 0.2); color: #8b5cf6; }

.text-center { text-align: center; }
</style>
