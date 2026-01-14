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
                <th>Endpoints</th>
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
                  <span v-if="isAgentTarget(dest.target)" class="agent-badge-inline">
                    <i class="bi bi-hdd-network"></i> {{ getDestinationLabel(dest.target) }}
                  </span>
                  <template v-else>
                    <div class="target-name">{{ dest.hostname || dest.target }}</div>
                    <div v-if="dest.hostname && dest.hostname !== dest.target" class="target-ip">{{ dest.target }}</div>
                  </template>
                </td>
                <td class="endpoints-cell">
                  <template v-if="dest.endpoint_ips?.length">
                    <span class="endpoint-list">
                      {{ dest.endpoint_ips.slice(0, 3).join(', ') }}
                      <span v-if="dest.endpoint_ips.length > 3" class="text-muted">
                        +{{ dest.endpoint_ips.length - 3 }} more
                      </span>
                    </span>
                  </template>
                  <span v-else class="text-muted">-</span>
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
      <div class="detail-section" v-if="aggregatedRoutes.length > 0">
        <span class="section-title"><i class="bi bi-arrow-right-circle"></i> Routes through this node</span>
        <div class="routes-list">
          <div v-for="{ route, count } in aggregatedRoutes" :key="route" class="route-item">
            <span class="route-path">{{ route }}</span>
            <span v-if="count > 1" class="route-count">×{{ count }}</span>
          </div>
          <div v-if="selectedNode.path_ids && selectedNode.path_ids.length > aggregatedRoutes.length" class="route-item text-muted">
            <i class="bi bi-three-dots"></i> {{ selectedNode.path_ids.length - aggregatedRoutes.length }} more routes
          </div>
        </div>
      </div>
      <div class="detail-section" v-if="selectedNode.shared_agents && selectedNode.shared_agents.length > 1">
        <span class="section-title"><i class="bi bi-share"></i> Shared by {{ selectedNode.shared_agents.length }} agents</span>
        <div class="shared-agents">
          <span v-for="(agentId, index) in selectedNode.shared_agents" :key="agentId" class="agent-badge">
            {{ getAgentName(agentId) }}<span v-if="index < selectedNode.shared_agents.length - 1">, </span>
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, onMounted, watch, onUnmounted, computed, nextTick } from 'vue';
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
    mapData.value = response.data;
    // Wait for Vue to update the DOM before creating visualization
    await nextTick();
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
      // Highlight paths through this node
      if (visualization) {
        visualization.highlightPath(node.id);
      }
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
    
    // Highlight the path in the visualization
    if (visualization) {
      visualization.highlightPath(target);
    }
  }
};

// Debounce function for resize
let resizeTimeout: ReturnType<typeof setTimeout> | null = null;
const handleResize = () => {
  if (resizeTimeout) clearTimeout(resizeTimeout);
  resizeTimeout = setTimeout(() => {
    if (visualization && containerRef.value) {
      visualization.resize(containerRef.value.clientWidth);
    }
  }, 200);
};

onMounted(async () => {
  await fetchMapData();
  
  // Add resize listener for responsive visualization
  window.addEventListener('resize', handleResize);
  
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
  window.removeEventListener('resize', handleResize);
  if (resizeTimeout) clearTimeout(resizeTimeout);
  visualization?.destroy();
});

watch([colorMode, () => mapData.value], () => {
  createVisualization();
}, { deep: true });

watch(connected, (val) => {
  isLive.value = val;
});

// Clear highlight when node is deselected
watch(selectedNode, (node) => {
  if (!node && visualization) {
    visualization.clearHighlight();
  }
});

// Refresh data when workspaceId changes (tab switch scenario)
watch(() => props.workspaceId, (newId, oldId) => {
  if (newId && newId !== oldId) {
    fetchMapData();
  }
}, { immediate: false });

// Helper functions for route display
const formatPathId = (pathId: string): string => {
  // Format: "agentId:target" -> "Agent Name → Target Name"
  const parts = pathId.split(':');
  if (parts.length >= 2) {
    const agentId = parseInt(parts[0]);
    const target = parts.slice(1).join(':'); // Handle IPv6 or "agent:X"
    const agentName = getAgentName(agentId);
    // Resolve target - if it's "agent:X", convert to agent name
    const targetLabel = getDestinationLabel(target);
    return `${agentName} → ${targetLabel}`;
  }
  return pathId;
};

const getAgentName = (agentId: number): string => {
  // Try to find agent name in map data
  const agentNode = mapData.value?.nodes.find(
    n => n.type === 'agent' && n.id === `agent:${agentId}`
  );
  return agentNode?.label || `Agent ${agentId}`;
};

// Get display label for destination target (resolves agent:ID to agent name)
const getDestinationLabel = (target: string): string => {
  if (target.startsWith('agent:')) {
    const agentId = parseInt(target.substring(6), 10);
    return getAgentName(agentId);
  }
  return target;
};

// Aggregate and deduplicate routes for display
const aggregatedRoutes = computed(() => {
  if (!selectedNode.value?.path_ids) return [];
  
  // Count occurrences of each formatted path
  const routeCounts = new Map<string, number>();
  for (const pathId of selectedNode.value.path_ids) {
    const formatted = formatPathId(pathId);
    routeCounts.set(formatted, (routeCounts.get(formatted) || 0) + 1);
  }
  
  // Convert to array and sort by count (descending)
  return Array.from(routeCounts.entries())
    .map(([route, count]) => ({ route, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 8); // Show top 8 unique routes
});

// Check if destination target is an agent
const isAgentTarget = (target: string): boolean => {
  return target.startsWith('agent:');
};

// Expose refresh method for parent components
defineExpose({
  refresh: fetchMapData
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
  private nodeRadius = 14;
  private margin = { top: 30, right: 20, bottom: 50, left: 20 };
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
    // Use full container width (no cap) for responsiveness
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

    const totalWidth = this.width + this.margin.left + this.margin.right;
    const totalHeight = this.height + this.margin.top + this.margin.bottom;
    
    this.svg = d3.select(this.container)
      .append('svg')
      .attr('width', '100%')  // Responsive width
      .attr('height', totalHeight)
      .attr('viewBox', `0 0 ${totalWidth} ${totalHeight}`)
      .attr('preserveAspectRatio', 'xMidYMid meet');

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
    // Create simulation with tighter parameters
    this.simulation = d3.forceSimulation<D3Node>(this.nodes)
      .force('link', d3.forceLink<D3Node, D3Link>(this.links)
        .id(d => d.id)
        .distance(50))
      .force('charge', d3.forceManyBody().strength(-120))
      .force('collision', d3.forceCollide(this.nodeRadius + 4))
      .force('center', d3.forceCenter(this.width / 2, this.height / 2));

    if (this.layoutMode === 'hierarchical') {
      this.applyHierarchicalLayout();
    }

    // Links - teal color like reference
    const linkSelection = this.g.append('g')
      .attr('class', 'links')
      .selectAll('line')
      .data(this.links)
      .enter()
      .append('line')
      .attr('stroke', '#14b8a6')
      .attr('stroke-opacity', 0.8)
      .attr('stroke-width', 2);

    // Nodes
    const nodeSelection = this.g.append('g')
      .attr('class', 'nodes')
      .selectAll('g')
      .data(this.nodes)
      .enter()
      .append('g')
      .attr('class', 'node')
      .call(this.createDragBehavior());

    // Render different shapes based on node type
    nodeSelection.each((d, i, nodes) => {
      const nodeEl = nodes[i];
      if (!nodeEl) return;
      const node = d3.select(nodeEl);
      
      if (d.type === 'agent') {
        // Agent: Small circle with thick border
        node.append('circle')
          .attr('r', 10)
          .attr('fill', '#22c55e')
          .attr('stroke', '#15803d')
          .attr('stroke-width', 2)
          .style('cursor', 'pointer');
      } else if (d.type === 'destination') {
        // Destination: Small circle, colored by status
        const color = d.status === 'critical' ? '#ef4444' : 
                      d.status === 'degraded' ? '#f59e0b' : '#22c55e';
        node.append('circle')
          .attr('r', 8)
          .attr('fill', color)
          .attr('stroke', d.status === 'critical' ? '#dc2626' : '#475569')
          .attr('stroke-width', 2)
          .style('cursor', 'pointer');
        // Add label to the right
        node.append('text')
          .attr('x', 14)
          .attr('dy', 4)
          .attr('text-anchor', 'start')
          .attr('fill', '#64748b')
          .style('font-size', '11px')
          .style('pointer-events', 'none')
          .text((d.hostname || d.label || d.ip || '').slice(0, 25));
        // IP below label
        if (d.ip && d.hostname && d.hostname !== d.ip) {
          node.append('text')
            .attr('x', 14)
            .attr('dy', 16)
            .attr('text-anchor', 'start')
            .attr('fill', '#94a3b8')
            .style('font-size', '9px')
            .style('pointer-events', 'none')
            .text(d.ip);
        }
      } else {
        // Hop: Small circle, colored by status/health
        const color = d.status === 'critical' ? '#ef4444' : 
                      d.status === 'degraded' ? '#f59e0b' : 
                      d.status === 'unknown' ? '#94a3b8' : '#22c55e';
        const isShared = (d.shared_agents?.length || 0) > 1;
        node.append('circle')
          .attr('r', isShared ? 7 : 5)
          .attr('fill', color)
          .attr('stroke', isShared ? '#3b82f6' : color)
          .attr('stroke-width', isShared ? 2 : 1)
          .style('cursor', 'pointer');
      }
    });

    // Agent labels - below the node
    nodeSelection.filter(d => d.type === 'agent').append('text')
      .attr('dy', 24)
      .attr('text-anchor', 'middle')
      .attr('fill', '#475569')
      .style('font-size', '10px')
      .style('font-weight', '500')
      .style('pointer-events', 'none')
      .text(d => d.label || 'Agent');

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
    const agentNodes = this.nodes.filter(n => n.type === 'agent');
    const hopNodes = this.nodes.filter(n => n.type === 'hop');
    const destNodes = this.nodes.filter(n => n.type === 'destination');

    // Build adjacency list for BFS depth calculation
    const adjacency = new Map<string, Set<string>>();
    this.links.forEach(link => {
      const sourceId = typeof link.source === 'string' ? link.source : (link.source as D3Node).id;
      const targetId = typeof link.target === 'string' ? link.target : (link.target as D3Node).id;
      if (!adjacency.has(sourceId)) adjacency.set(sourceId, new Set());
      if (!adjacency.has(targetId)) adjacency.set(targetId, new Set());
      adjacency.get(sourceId)!.add(targetId);
      adjacency.get(targetId)!.add(sourceId);
    });

    // BFS from agents to calculate depth (layer) for each node
    const nodeDepth = new Map<string, number>();
    const queue: { id: string; depth: number }[] = [];
    
    // Start BFS from all agents (layer 0)
    agentNodes.forEach(n => {
      nodeDepth.set(n.id, 0);
      queue.push({ id: n.id, depth: 0 });
    });

    while (queue.length > 0) {
      const { id, depth } = queue.shift()!;
      const neighbors = adjacency.get(id) || new Set();
      neighbors.forEach(neighborId => {
        if (!nodeDepth.has(neighborId)) {
          nodeDepth.set(neighborId, depth + 1);
          queue.push({ id: neighborId, depth: depth + 1 });
        }
      });
    }

    // Find max depth for scaling
    const maxDepth = Math.max(...Array.from(nodeDepth.values()), 1);
    const layerWidth = (this.width - 200) / (maxDepth + 1);

    // Group nodes by depth layer
    const nodesByLayer: Map<number, D3Node[]> = new Map();
    this.nodes.forEach(node => {
      let depth = nodeDepth.get(node.id) ?? 0;
      // Force destinations to rightmost layer
      if (node.type === 'destination') {
        depth = maxDepth;
      }
      if (!nodesByLayer.has(depth)) nodesByLayer.set(depth, []);
      nodesByLayer.get(depth)!.push(node);
    });

    // Position all nodes with fixed positions (rigid layout)
    nodesByLayer.forEach((layerNodes, depth) => {
      const xPos = 50 + layerWidth * depth;
      const ySpacing = this.height / (layerNodes.length + 1);
      
      layerNodes.forEach((node, i) => {
        node.fx = xPos;
        node.fy = ySpacing * (i + 1);
        node.x = xPos;
        node.y = ySpacing * (i + 1);
      });
    });

    // Destinations get extra right margin for labels
    destNodes.forEach((node, i) => {
      node.fx = this.width - 120;
      if (!nodesByLayer.get(maxDepth)?.includes(node)) {
        node.fy = (this.height / (destNodes.length + 1)) * (i + 1);
      }
    });

    // Minimal simulation forces - nodes are fixed, just need links to render
    this.simulation
      .force('link', d3.forceLink<D3Node, D3Link>(this.links).id(d => d.id).distance(40).strength(0.1))
      .force('charge', null)  // Disable charge in rigid mode
      .force('collision', null);  // Disable collision in rigid mode
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

  public resize(newWidth: number) {
    const adjustedWidth = newWidth - this.margin.left - this.margin.right;
    if (Math.abs(adjustedWidth - this.width) < 50) return; // Skip small changes
    
    this.width = adjustedWidth;
    const totalWidth = this.width + this.margin.left + this.margin.right;
    const totalHeight = this.height + this.margin.top + this.margin.bottom;
    
    // Update SVG viewBox
    this.svg
      .attr('viewBox', `0 0 ${totalWidth} ${totalHeight}`);
    
    // Update simulation center force
    this.simulation.force('center', d3.forceCenter(this.width / 2, this.height / 2));
    
    // Re-apply layout if hierarchical
    if (this.layoutMode === 'hierarchical') {
      this.applyHierarchicalLayout();
    }
    
    this.simulation.alpha(0.3).restart();
  }

  public highlightPath(targetId: string) {
    // Find the clicked node and get its path_ids
    const clickedNode = this.nodes.find(n => n.id === targetId);
    const relevantPathIds = new Set<string>(clickedNode?.path_ids || []);
    
    // Build set of agent node IDs for checking if target is an agent
    const agentNodeIds = new Set<string>();
    const agentIPs = new Set<string>();
    this.nodes.filter(n => n.type === 'agent').forEach(n => {
      agentNodeIds.add(n.id);
      if (n.ip) agentIPs.add(n.ip);
    });
    
    // Special handling for AGENT nodes - ONLY show agent-to-agent paths
    // path_id format is "sourceAgentId:target" (e.g., "1:google.com", "3:agent:1")
    if (clickedNode?.type === 'agent' && clickedNode.agent_id) {
      const agentId = clickedNode.agent_id;
      this.links.forEach(link => {
        (link.path_ids || []).forEach(pathId => {
          const [sourceAgentStr, ...targetParts] = pathId.split(':');
          const sourceAgentId = parseInt(sourceAgentStr, 10);
          const pathTarget = targetParts.join(':'); // rejoin in case target has ':' like "agent:3"
          
          // Check if this path's target is an agent (agent-to-agent only)
          const targetIsAgent = pathTarget.startsWith('agent:') || agentIPs.has(pathTarget);
          
          if (targetIsAgent) {
            // Include paths ORIGINATING from this agent to another agent
            if (sourceAgentId === agentId) {
              relevantPathIds.add(pathId);
            }
            // Include paths TARGETING this agent (reverse direction)
            if (pathTarget === targetId || pathTarget === `agent:${agentId}`) {
              relevantPathIds.add(pathId);
            }
          }
        });
      });
    }
    // For destination nodes - include ALL paths to this destination from any agent
    else if (clickedNode?.type === 'destination') {
      this.links.forEach(link => {
        (link.path_ids || []).forEach(pathId => {
          const [, ...targetParts] = pathId.split(':');
          const pathTarget = targetParts.join(':');
          // Match by ID, label, hostname, or IP
          if (pathTarget === targetId || 
              pathTarget === clickedNode.label ||
              pathTarget === clickedNode.hostname ||
              pathTarget === clickedNode.ip) {
            relevantPathIds.add(pathId);
          }
        });
      });
    }
    
    // If no path_ids found, fall back to showing edges connected directly to this node
    if (relevantPathIds.size === 0) {
      // Fallback: show only edges directly connected to this node
      const connectedEdgeIds = new Set<string>();
      const connectedNodeIds = new Set<string>([targetId]);
      
      this.links.forEach(link => {
        const sourceId = typeof link.source === 'string' ? link.source : (link.source as D3Node).id;
        const targetLinkId = typeof link.target === 'string' ? link.target : (link.target as D3Node).id;
        
        if (sourceId === targetId || targetLinkId === targetId) {
          connectedEdgeIds.add(link.id);
          connectedNodeIds.add(sourceId);
          connectedNodeIds.add(targetLinkId);
        }
      });
      
      this.g.selectAll('.nodes g')
        .style('opacity', (d: any) => connectedNodeIds.has(d.id) ? 1 : 0.2);
      
      this.g.selectAll('.links line')
        .style('opacity', (d: any) => connectedEdgeIds.has(d.id) ? 1 : 0.1)
        .style('stroke-width', (d: any) => connectedEdgeIds.has(d.id) ? 4 : 2);
      return;
    }
    
    // Collect nodes and edges that belong to the relevant paths
    const connectedNodeIds = new Set<string>([targetId]);
    const connectedEdgeIds = new Set<string>();
    
    // Also add source agents from the relevant path_ids
    relevantPathIds.forEach(pathId => {
      const sourceAgentId = parseInt(pathId.split(':')[0], 10);
      if (!isNaN(sourceAgentId)) {
        connectedNodeIds.add(`agent:${sourceAgentId}`);
      }
    });
    
    this.links.forEach(link => {
      // Check if this edge has ANY path_id that's in our relevant set
      const hasMatchingPath = (link.path_ids || []).some(pid => relevantPathIds.has(pid));
      if (hasMatchingPath) {
        connectedEdgeIds.add(link.id);
        
        const sourceId = typeof link.source === 'string' ? link.source : (link.source as D3Node).id;
        const targetLinkId = typeof link.target === 'string' ? link.target : (link.target as D3Node).id;
        connectedNodeIds.add(sourceId);
        connectedNodeIds.add(targetLinkId);
      }
    });

    // Dim non-connected elements
    this.g.selectAll('.nodes g')
      .style('opacity', (d: any) => connectedNodeIds.has(d.id) ? 1 : 0.2);
    
    this.g.selectAll('.links line')
      .style('opacity', (d: any) => connectedEdgeIds.has(d.id) ? 1 : 0.1)
      .style('stroke-width', (d: any) => connectedEdgeIds.has(d.id) ? 4 : 2);
  }

  public clearHighlight() {
    this.g.selectAll('.nodes g').style('opacity', 1);
    this.g.selectAll('.links line')
      .style('opacity', 0.7)
      .style('stroke-width', (d: any) => Math.max(2, Math.sqrt(d.path_count || 1) * 1.5));
  }
}

// D3-specific interfaces
interface D3Node extends d3.SimulationNodeDatum {
  id: string;
  type: 'agent' | 'hop' | 'destination';
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
  status?: 'healthy' | 'degraded' | 'critical' | 'unknown';
  shared_agents?: number[];
  path_ids?: string[];
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
  path_ids?: string[];
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

/* Detail sections */
.detail-section {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--map-border);
}

.section-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--map-text-muted);
  margin-bottom: 8px;
  text-transform: uppercase;
  letter-spacing: 0.3px;
}

.section-title i {
  font-size: 11px;
}

/* Routes list */
.routes-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.route-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 8px;
  background: rgba(59, 130, 246, 0.08);
  border-radius: 6px;
  font-size: 12px;
}

.route-path {
  color: var(--map-value-color);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
}

.route-count {
  flex-shrink: 0;
  background: rgba(59, 130, 246, 0.2);
  color: #3b82f6;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 600;
  margin-left: 8px;
}

/* Shared agents */
.shared-agents {
  font-size: 12px;
  color: var(--map-value-color);
  line-height: 1.6;
}

.agent-badge {
  display: inline;
}

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
