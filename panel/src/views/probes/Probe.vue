<script lang="ts" setup>
import {nextTick, onMounted, reactive, ref, watch} from "vue";
import core from "@/core";
import type {Agent, MtrResult, PingResult, Probe, ProbeData, ProbeType, Workspace,} from "@/types";
import Title from "@/components/Title.vue";
import {AsciiTable3} from "@/lib/ascii-table3/ascii-table3";
import LatencyGraph from "@/components/PingGraph.vue";
import TrafficSimGraph from "@/components/TrafficSimGraph.vue";
import NetworkMap from "@/components/NetworkMap.vue";
import MtrTable from "@/components/MtrTable.vue";
import MtrDetailModal from "@/components/MtrDetailModal.vue";
import VueDatePicker from '@vuepic/vue-datepicker';
import '@vuepic/vue-datepicker/dist/main.css';

// NEW: API services wired to your new endpoints
import {AgentService, ProbeDataService, ProbeService, WorkspaceService} from "@/services/apiService";
import {findMatchingProbesByProbeId, findProbesByInitialTarget} from "@/utils/probeGrouping";

// Ref for active tab to trigger NetworkMap updates
const activeTabIndex = ref(0);

// Modal state for MTR detail view
const showMtrModal = ref(false);
const selectedNode = ref<{ id: string; hostname?: string; ip?: string; hopNumber: number } | null>(null);

const onNodeSelect = (node: any) => {
  selectedNode.value = node;
  showMtrModal.value = true;
};

const closeMtrModal = () => {
  showMtrModal.value = false;
  selectedNode.value = null;
};

// Reactive state to hold parsed groups and UI data
const state = reactive({
  probes: [] as Probe[],
  workspace: {} as Workspace,
  agent: {} as Agent,
  similarProbes: [] as Probe[],
  // Parsed ProbeData by type
  pingData: [] as ProbeData[],
  probe: {} as Probe,
  mtrData: [] as ProbeData[],
  rperfData: [] as ProbeData[],
  trafficSimData: [] as ProbeData[],
  probeData: [] as ProbeData[],
  // Additional sections from AgentProbe
  availableTargets: [] as Array<{agent:string,group:string}>,
  summary: {
    totalDataPoints: 0,
    reportingAgents: [] as string[],
    targetAgents: [] as string[],
    probeTypes: [] as string[],
    dataCountByType: {} as Record<string,number>
  },
  timeRange: [] as [Date, Date],
  title: "",
  ready: false,
  loading: true,
  probeAgent: {} as Agent,
  table: {} as string,
  pingGraph: {} as any,
  target: "" as string,
  checks: [] as Probe[],
  // New state for agent probe groupings
  agentPairData: [] as Array<{
    sourceAgentId: string,
    targetAgentId: string,
    sourceAgentName: string,
    targetAgentName: string,
    pingData: ProbeData[],
    mtrData: ProbeData[],
    trafficSimData: ProbeData[],
    rperfData: ProbeData[]
  }>,
  isAgentProbe: false,
  rawGroups: {} as any,
});

const router = core.router();

// ---------- Small utils ----------
const toRFC3339 = (v?: Date | string | number) =>
    v instanceof Date ? v.toISOString() : typeof v === "number" ? new Date(v).toISOString() : v;

function onTabChange(index: number) {
  activeTabIndex.value = index;
  nextTick(() => window.dispatchEvent(new Event('resize')));
}

function generateTable(probeData: ProbeData) {
  // NEW: payload holds the typed body
  const mtrCalculate = (probeData as any).payload;

  if (!mtrCalculate?.report?.info?.target) return "No MTR payload";
  const title =
      `${mtrCalculate.report.info.target.hostname} (${mtrCalculate.report.info.target.ip}) - ` +
      new Date(mtrCalculate.stopTimestamp || probeData.created_at || (probeData as any).createdAt).toISOString();

  const table = new AsciiTable3(title);
  table.setHeading('Hop', 'Host', 'Loss%', 'Snt', 'Recv', 'Avg', 'Best', 'Worst', 'StDev');

  const seenIPs = new Map<string, number>();

  (mtrCalculate.report.hops as any[]).forEach((hop: any, hopIndex: number) => {
    if (!hop.hosts || hop.hosts.length === 0) {
      table.addRow(hopIndex.toString(), '*','*','*','*','*','*','*','*');
    } else {
      hop.hosts.forEach((host: any, hostIndex: number) => {
        const hostDisplay = `${host.hostname} (${host.ip})`;
        let hopDisplay = hopIndex.toString();
        let prefix = '    ';

        if (seenIPs.has(host.ip)) {
          const occurrences = seenIPs.get(host.ip)!;
          prefix = '|   ';
          hopDisplay = "+-> " + hopDisplay;
          seenIPs.set(host.ip, occurrences + 1);
        } else {
          seenIPs.set(host.ip, 1);
        }

        if (hostIndex !== 0) hopDisplay = prefix + hopDisplay;

        table.addRow(
            hopDisplay,
            hostDisplay,
            hop.loss_pct,
            hop.sent?.toString?.() ?? '',
            hop.recv?.toString?.() ?? '',
            hop.avg,
            hop.best,
            hop.worst,
            hop.stddev
        );
      });
    }
  });

  table.setStyle("unicode-single");
  return table.toString();
}

// Helper function to add probe data without duplicates
// Helper: safely add probe data without duplicates or Vue reactivity issues
function addProbeDataUnique(targetArray: ProbeData[], newData: ProbeData) {
  if (!newData) return;

  // --- ensure stable unique key ---
  // Many backends reuse `id=0` or null; generate UUID if missing or falsy
  if (typeof crypto !== "undefined" && (crypto as any).randomUUID) {
    (newData as any).id = (crypto as any).randomUUID();
  } else {
    (newData as any).id = "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, c => {
      const r = (Math.random() * 16) | 0;
      const v = c === "x" ? r : (r & 0x3) | 0x8;
      return v.toString(16);
    });
  }

  // --- deduplication logic ---
  // Use a stable composite key instead of only `id` when data sources overlap
  const key = `${newData.id}-${(newData as any).timestamp ?? ""}-${(newData as any).type ?? ""}`;

  const exists = targetArray.some(
      (item) =>
          item.id === newData.id ||
          `${item.id}-${(item as any).timestamp ?? ""}-${(item as any).type ?? ""}` === key
  );

  if (!exists) {
    // Use .push() to preserve reactivity in Vue arrays
    targetArray.push(newData);
  }
}
// --------- Adapters for graphs (expecting your components’ input shapes) ---------

// PING: flatten rows -> PingResult[]
function transformPingDataMulti(rows: ProbeData[]): PingResult[] {
  return rows
      .map((r) => {
        // Normalize likely fields: ts/created_at, avg/latency, loss, min/max
        return r.payload as PingResult
      })
}

// MTR: a single MTR payload -> MtrResult (for title/accordion)
function transformMtrData(data: ProbeData): MtrResult {
  return data.payload as MtrResult;
}

// MTR: multiple rows -> MtrResult[]
function transformMtrDataMulti(rows: ProbeData[]): MtrResult[] {
  return rows.map(r => transformMtrData((r as any)));
}

// MTR: format timestamp for accordion header
function formatMtrTimestamp(mtr: ProbeData): string {
  const payload = mtr.payload as MtrResult;
  const timestamp = payload?.stop_timestamp || mtr.created_at;
  try {
    return new Date(timestamp).toLocaleString();
  } catch {
    return 'Unknown time';
  }
}

// MTR: get route signature for comparison
function getMtrRouteSignature(mtr: ProbeData): string {
  const payload = mtr.payload as MtrResult;
  if (!payload?.report?.hops) return '';
  return payload.report.hops
    .map(hop => hop.hosts?.[0]?.ip || '*')
    .join('->');
}

// MTR: check if trace has notable packet loss (>1% on any known hop)
function hasHighLoss(mtr: ProbeData): boolean {
  const payload = mtr.payload as MtrResult;
  if (!payload?.report?.hops) return false;
  return payload.report.hops.some(hop => {
    // Skip unknown hops (no hosts = "*" row)
    if (!hop.hosts || hop.hosts.length === 0) return false;
    // loss_pct can be "5.0%" or "5.0" - strip % if present
    const lossStr = String(hop.loss_pct || '0').replace('%', '').trim();
    const loss = parseFloat(lossStr);
    return !isNaN(loss) && loss > 1; // >1% to avoid noise
  });
}

// MTR: check if trace has high latency (>150ms avg on any known hop)
function hasHighLatency(mtr: ProbeData): boolean {
  const payload = mtr.payload as MtrResult;
  if (!payload?.report?.hops) return false;
  return payload.report.hops.some(hop => {
    // Skip unknown hops
    if (!hop.hosts || hop.hosts.length === 0) return false;
    // avg can be "45.2 ms" or "45.2" - extract number
    const avgStr = String(hop.avg || '0').replace(/[^\d.]/g, '');
    const avg = parseFloat(avgStr);
    return !isNaN(avg) && avg > 150; // >150ms is high
  });
}

// MTR: filter to only notable results (triggered, high loss, route changes)
function getNotableMtrResults(mtrData: ProbeData[]): { data: ProbeData; reason: string }[] {
  if (!mtrData || mtrData.length === 0) return [];
  
  // Sort by timestamp
  const sorted = [...mtrData].sort((a, b) => {
    const timeA = new Date(a.payload?.stop_timestamp || a.created_at).getTime();
    const timeB = new Date(b.payload?.stop_timestamp || b.created_at).getTime();
    return timeB - timeA;
  });
  
  const notable: { data: ProbeData; reason: string }[] = [];
  let prevSignature = '';
  
  // Process in reverse (oldest first) to detect route changes properly
  for (let i = sorted.length - 1; i >= 0; i--) {
    const mtr = sorted[i];
    const reasons: string[] = [];
    
    // Check if triggered
    if (mtr.triggered) reasons.push('triggered');
    
    // Check packet loss (any loss is notable)
    if (hasHighLoss(mtr)) reasons.push('packet-loss');
    
    // Check high latency
    if (hasHighLatency(mtr)) reasons.push('high-latency');
    
    // Check route change
    const signature = getMtrRouteSignature(mtr);
    if (prevSignature && signature !== prevSignature) reasons.push('route-change');
    prevSignature = signature;
    
    if (reasons.length > 0) {
      notable.push({ data: mtr, reason: reasons.join(',') });
    }
  }
  
  // Return in newest-first order
  return notable.reverse();
}

// TRAFFICSIM: normalize series
function transformToTrafficSimResult(rows: ProbeData[]) {
  return rows.map((r) => {
    const p = (r as any).payload;
    return {
      ts: new Date(p?.timestamp ?? r.created_at ?? (r as any).createdAt).getTime(),
      bitrate: p?.bitrate_bps ?? p?.throughput_bps ?? 0,
      loss: p?.loss ?? p?.loss_pct ?? 0,
      jitter: p?.jitter_ms ?? 0,
      probeId: r.probe_id,
      agentId: r.agent_id,
      target: r.target || p?.target,
    };
  }).sort((a,b) => a.ts - b.ts);
}

// ---------- Agent pair parsing (kept; expects grouped data if you add that later) ----------
async function parseAgentPairData(groups: any) {
  const agentPairs: typeof state.agentPairData = [];
  const nameCache: Record<string,string> = {};
  const getName = async (wid: number|string, aid: string) => {
    if (nameCache[aid]) return nameCache[aid];
    try {
      const a = await AgentService.get(wid, aid);
      nameCache[aid] = a.name || String(a.id);
      return nameCache[aid];
    } catch { return aid; }
  };
  const wid = currentWorkspaceId();

  for (const [sourceAgentId, targetAgents] of Object.entries(groups || {})) {
    for (const [targetAgentId, probeTypes] of Object.entries(targetAgents as any)) {
      const pair = {
        sourceAgentId, targetAgentId,
        sourceAgentName: await getName(wid, sourceAgentId),
        targetAgentName: await getName(wid, targetAgentId),
        pingData: [] as ProbeData[], mtrData: [] as ProbeData[], trafficSimData: [] as ProbeData[], rperfData: [] as ProbeData[]
      };
      for (const [t, arr] of Object.entries(probeTypes as any)) {
        if (t === "PING") pair.pingData = arr as ProbeData[];
        if (t === "MTR") pair.mtrData = arr as ProbeData[];
        if (t === "TRAFFICSIM") pair.trafficSimData = arr as ProbeData[];
        if (t === "RPERF") pair.rperfData = arr as ProbeData[];
      }
      agentPairs.push(pair);
    }
  }
  return agentPairs;
}

// ---------- Data loader (new services + new data model) ----------

function currentWorkspaceId(): number {
  // route param name choices: workspaceId OR wid; support both
  const p = router.currentRoute.value.params;
  return Number(p["workspaceId"] ?? p["wid"] ?? 0);
}
function currentAgentId(): number {
  const p = router.currentRoute.value.params;
  return Number(p["agentId"] ?? p["aid"] ?? 0);
}
function currentProbeId(): number {
  const p = router.currentRoute.value.params;
  return Number(p["probeId"] ?? p["idParam"] ?? 0);
}

let agentID = router.currentRoute.value.params["aID"] as string
let workspaceID = router.currentRoute.value.params["wID"] as string
let probeID = (router.currentRoute.value.params["pID"] as string)
    || (router.currentRoute.value.params["idParam"] as string) // fallback if needed
if (!agentID || !workspaceID || !probeID) {
  // early exit — nothing to load
}

async function reloadData() {
  state.loading = true;
  state.ready = false;

  // reset buckets
  state.pingData = [];
  state.mtrData = [];
  state.rperfData = [];
  state.trafficSimData = [];
  state.probeData = [];
  state.similarProbes = [];
  state.agentPairData = [];
  state.isAgentProbe = false;
  (state as any).similarByHost = [];
  (state as any).similarByAgent = [];
  (state as any).matchedGroupKeys = [];

  try {
    if (!workspaceID || !probeID) {
      state.ready = false;
      return;
    }

    // 1) Load workspace & agent metadata in parallel (non-fatal if they fail)
    const [wsRes, agRes] = await Promise.allSettled([
      WorkspaceService.get(workspaceID),
      AgentService.get(workspaceID, agentID),
    ]);
    if (wsRes.status === "fulfilled") state.workspace = wsRes.value as Workspace;
    if (agRes.status === "fulfilled") state.agent = agRes.value as Agent;

    // 2) Load the main probe
    state.probe = (await ProbeService.get(workspaceID, agentID, probeID)) as Probe;

    // 3) Load related probe list and filter
    const allProbes = (await ProbeService.list(workspaceID, agentID)) as Probe[];
    state.probes = findProbesByInitialTarget(state.probe, allProbes);
    console.log("probes:", state.probes);

    // 4) Title from first target (agent ref vs literal)
    const firstTarget = (state.probe?.targets?.[0] ?? {}) as any;
    if (firstTarget.agent_id != null) {
      try {
        const targ = (await AgentService.get(workspaceID, firstTarget.agent_id as number)) as Agent;
        state.probeAgent = targ;
        state.title = targ.name || `agent:${(targ as any).id}`;
      } catch {
        state.title = `${state.probe.type} #${state.probe.id}`;
      }
    } else if (firstTarget.target) {
      const split = String(firstTarget.target).split(":");
      state.title = split[0] || String(firstTarget.target);
    } else {
      state.title = `${state.probe.type} #${state.probe.id}`;
    }

    // 5) Pull series for each probe (await so ready/loading are correct)
    await loadProbeData();

    state.ready = true;
  } catch (e) {
    console.error(e);
    state.ready = false;
  } finally {
    state.loading = false;
    console.log("pingData len:", state.pingData.length);
  }
}

// ---------- Guards / helpers ----------
function containsProbeType(type: ProbeType): boolean {
  switch(type) {
    case 'PING': return state.pingData.length > 0;
    case 'MTR': return state.mtrData.length > 0;
    case 'RPERF': return state.rperfData.length > 0;
    case 'TRAFFICSIM': return state.trafficSimData.length > 0;
    default: return false;
  }
}

async function loadProbeData(): Promise<void> {
  const [from, to] = state.timeRange;

  console.log("loading probe data")

  const tasks = state.probes.map(async (p) => {
    try {
      const rows = await ProbeDataService.byProbe(
          workspaceID,
          p.id,
          { from, to, limit: 5000, asc: false }
      );

      for (const d of rows) {
        // common bucket
        addProbeDataUnique(state.probeData, d);

        // per-type buckets
        const t = (d as any).type as ProbeType;
        if (t === "PING") addProbeDataUnique(state.pingData, d);
        if (t === "MTR") addProbeDataUnique(state.mtrData, d);
        if (t === "RPERF" && !(state.probe as any).server) addProbeDataUnique(state.rperfData, d);
        if (t === "TRAFFICSIM") addProbeDataUnique(state.trafficSimData, d);
      }
    } catch (err) {
      console.error(`probe ${p.id} fetch failed:`, err);
    }
  });

  console.log("loaded probe data")

  // run them all in parallel, but don't throw if one fails
  await Promise.allSettled(tasks);
}

function onCreate(_: any) { router.push("/workspace"); }
function onError(response: any) { alert(response); }
function submit() {}

onMounted(() => {
  // default to last 3 hours
  state.timeRange = [new Date(Date.now() - 3*60*60*1000), new Date()];
});

// Watch for timeRange changes
watch(() => state.timeRange, () => { reloadData() }, { deep: true });
</script>

<template>
  <div class="container-fluid">
    <Title
        :history="[
        {title: 'workspaces', link: '/workspaces'},
        {title: state.workspace.name || 'Loading...', link: `/workspaces/${state.workspace.id}`},
        {title: state.agent.name || 'Loading...', link: `/workspaces/${state.workspace.id}/agents/${state.agent.id}`},
      ]"
        :title="state.title"
        subtitle="information about this target">
      <div v-if="state.ready" class="d-flex gap-1">
        <VueDatePicker v-model="state.timeRange" :partial-range="false" range/>
      </div>
    </Title>
    <div v-if="state.ready" >
    <!-- Summary Card (from AgentProbe) 
    <div class="card mb-3" v-if="state.summary.totalDataPoints > 0">
      <div class="card-body">
        <h5 class="card-title">Summary</h5>
        <p>Total Data Points: {{ state.summary.totalDataPoints }}</p>
        <p>Reporting Agents: {{ state.summary.reportingAgents.join(', ') }}</p>
        <p>Target Agents: {{ state.summary.targetAgents.join(', ') }}</p>
        <p>Probe Types: {{ state.summary.probeTypes.join(', ') }}</p>
        <p>Counts: <span v-for="(count, type) in state.summary.dataCountByType" :key="type">{{ type }}: {{ count }}, </span></p>
      </div>
    </div>-->

    <!-- Available Targets (from AgentProbe) 
    <div class="card mb-3" v-if="state.availableTargets.length > 0">
      <div class="card-body">
        <h5 class="card-title">Available Targets</h5>
        <ul>
          <li v-for="t in state.availableTargets" :key="t.agent + '-' + t.group">
            Agent: {{ t.agent }}, Group: {{ t.group }}
          </li>
        </ul>
      </div>
    </div>-->

    <div class="row">
      <!-- Agent Probe Comparison View -->
      <div v-if="state.isAgentProbe && state.agentPairData.length > 0" class="col-12">
        <div class="card mb-3">
          <div class="card-body">
            <h5 class="card-title">Agent-to-Agent Monitoring Comparison</h5>
            <p class="card-text">Bidirectional monitoring data between agent pairs</p>
            
            <!-- Tabs for different agent pairs -->
            <ul class="nav nav-tabs" role="tablist">
              <li v-for="(pair, index) in state.agentPairData" :key="`tab-${index}`" class="nav-item" role="presentation">
                <button 
                  :class="['nav-link', index === 0 ? 'active' : '']"
                  :id="`pair-tab-${index}`"
                  :data-bs-target="`#pair-content-${index}`"
                  data-bs-toggle="tab"
                  type="button"
                  role="tab"
                  @click="onTabChange(index)">
                  {{ pair.sourceAgentName }} → {{ pair.targetAgentName }}
                </button>
              </li>
            </ul>
            
            <!-- Tab content for each agent pair -->
            <div class="tab-content mt-3">
              <div v-for="(pair, index) in state.agentPairData" 
                   :key="`content-${index}`"
                   :id="`pair-content-${index}`"
                   :class="['tab-pane', 'fade', index === 0 ? 'show active' : '']"
                   role="tabpanel">
                
                <!-- Agent Pair Information -->
                <div class="alert alert-info">
                  <strong>Source Agent:</strong> {{ pair.sourceAgentName }} ({{ pair.sourceAgentId }})<br>
                  <strong>Target Agent:</strong> {{ pair.targetAgentName }} ({{ pair.targetAgentId }})
                </div>
                
                <div class="row">
                  <!-- Ping Data for this pair -->
                  <div v-if="pair.pingData.length > 0" class="col-lg-12 mb-3">
                    <div class="card h-100">
                      <div class="card-header">
                        <h6 class="mb-0">Latency ({{ pair.sourceAgentName }} → {{ pair.targetAgentName }})</h6>
                      </div>
                      <div class="card-body">
                        <LatencyGraph :pingResults="transformPingDataMulti(pair.pingData)" :intervalSec="state.probe?.interval_sec || 60" />
                      </div>
                    </div>
                  </div>
                  <div v-else-if="containsProbeType('PING')" class="col-lg-12 mb-3">
                    <div class="card h-100">
                      <div class="card-header">
                        <h6 class="mb-0">Latency ({{ pair.sourceAgentName }} → {{ pair.targetAgentName }})</h6>
                      </div>
                      <div class="card-body d-flex align-items-center justify-content-center text-muted">
                        <div class="text-center">
                          <i class="bi bi-info-circle fs-1 mb-2"></i>
                          <p>No latency data available for this direction</p>
                        </div>
                      </div>
                    </div>
                  </div>
                  </div>
                  <div class="row">
                  <!-- Traffic Sim Data for this pair -->
                  <div v-if="pair.trafficSimData.length > 0" class="col-lg-12 mb-3">
                    <div class="card h-100">
                      <div class="card-header">
                        <h6 class="mb-0">Simulated Traffic ({{ pair.sourceAgentName }} → {{ pair.targetAgentName }})</h6>
                      </div>
                      <div class="card-body">
                        <TrafficSimGraph :traffic-results="transformToTrafficSimResult(pair.trafficSimData)" :intervalSec="state.probe?.interval_sec || 60" />
                      </div>
                    </div>
                  </div>
                  <div v-else-if="containsProbeType('TRAFFICSIM')" class="col-lg-12 mb-3">
                    <div class="card h-100">
                      <div class="card-header">
                        <h6 class="mb-0">Simulated Traffic ({{ pair.sourceAgentName }} → {{ pair.targetAgentName }})</h6>
                      </div>
                      <div class="card-body d-flex align-items-center justify-content-center text-muted">
                        <div class="text-center">
                          <i class="bi bi-info-circle fs-1 mb-2"></i>
                          <p>No traffic simulation data available for this direction</p>
                        </div>
                      </div>
                    </div>
                  </div>
                  
                  <!-- MTR Data for this pair -->
                  <div v-if="pair.mtrData.length > 0" class="col-12">
                    <div class="card">
                      <div class="card-header">
                        <h6 class="mb-0">Traceroutes ({{ pair.sourceAgentName }} → {{ pair.targetAgentName }})</h6>
                      </div>
                      <div class="card-body">
                        <!-- Key to force re-render on tab change -->
                        <NetworkMap 
                          :key="`mtr-map-${index}-${activeTabIndex}`"
                          :mtrResults="transformMtrDataMulti(pair.mtrData)"
                        />
                        <div :id="`mtrAccordion-${index}`" class="accordion mt-3">
                          <div v-for="(mtr, mtrIndex) in pair.mtrData" :key="`${mtr.id}-${index}-${mtrIndex}`">
                            <div class="accordion-item">
                              <h2 :id="`heading-${index}-${mtr.id}`" class="accordion-header">
                                <button 
                                  :aria-controls="`collapse-${index}-${mtr.id}`" 
                                  :aria-expanded="false"
                                  :data-bs-target="`#collapse-${index}-${mtr.id}`"
                                  class="accordion-button collapsed" 
                                  data-bs-toggle="collapse" 
                                  type="button">
                                  {{ formatMtrTimestamp(mtr as ProbeData) }}
                                  <span v-if="(mtr as ProbeData).triggered" class="badge bg-warning text-dark ms-2">TRIGGERED</span>
                                </button>
                              </h2>
                              <div 
                                :id="`collapse-${index}-${mtr.id}`" 
                                :aria-labelledby="`heading-${index}-${mtr.id}`"
                                :data-bs-parent="`#mtrAccordion-${index}`"
                                class="accordion-collapse collapse">
                                <div class="accordion-body p-0">
                                  <MtrTable :probe-data="mtr as ProbeData" />
                                </div>
                              </div>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                  <div v-else-if="containsProbeType('MTR')" class="col-12">
                    <div class="card">
                      <div class="card-header">
                        <h6 class="mb-0">Traceroutes ({{ pair.sourceAgentName }} → {{ pair.targetAgentName }})</h6>
                      </div>
                      <div class="card-body d-flex align-items-center justify-content-center text-muted" style="min-height: 200px;">
                        <div class="text-center">
                          <i class="bi bi-info-circle fs-1 mb-2"></i>
                          <p>No traceroute data available for this direction</p>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
      
      <!-- No data state for agent probes -->
      <div v-else-if="state.isAgentProbe && !state.loading && state.agentPairData.length === 0" class="col-12">
        <div class="card mb-3">
          <div class="card-body text-center py-5">
            <i class="bi bi-inbox fs-1 text-muted mb-3"></i>
            <h5 class="text-muted">No Agent-to-Agent Data Available</h5>
            <p class="text-muted">No monitoring data found for the selected time range. Try adjusting the date range.</p>
          </div>
        </div>
      </div>

      <!-- Original probe views for non-AGENT probes -->
      <template v-if="!state.isAgentProbe">
        <!-- Ping/Latency Graph -->
      <div class="col-sm-12" v-if="containsProbeType('PING')">
        <div class="card mb-3">
          <div class="card-body">
            <h5 class="card-title">Latency</h5>
            <p class="card-text">displays the stats associated with latency</p>
            <div v-if="state.loading && state.pingData.length === 0" class="text-center py-5">
              <div class="spinner-border text-primary" role="status">
                <span class="visually-hidden">Loading...</span>
              </div>
              <h3 class="mt-3 text-muted">Loading latency data...</h3>
              <p class="text-muted">Please wait while we fetch the data</p>
            </div>
            <div v-else-if="!state.loading && state.pingData.length === 0" class="text-center py-5">
              <i class="bi bi-graph-down fs-1 text-muted mb-3"></i>
              <h5 class="text-muted">No Latency Data Available</h5>
              <p class="text-muted">No ping data found for the selected time range</p>
            </div>
            <div v-else>
              <LatencyGraph :pingResults="transformPingDataMulti(state.pingData)" :intervalSec="state.probe?.interval_sec || 60" />
            </div>
          </div>
        </div>
      </div>

      <!-- TrafficSim Graph -->
      <div class="col-sm-12" v-if="containsProbeType('TRAFFICSIM')">
        <div class="card mb-3">
          <div class="card-body">
            <h5 class="card-title">Simulated Traffic</h5>
            <p class="card-text">displays the stats for simulated traffic</p>
            <div v-if="state.loading && state.trafficSimData.length === 0" class="text-center py-5">
              <div class="spinner-border text-primary" role="status">
                <span class="visually-hidden">Loading...</span>
              </div>
              <h3 class="mt-3 text-muted">Loading traffic simulation data...</h3>
              <p class="text-muted">Please wait while we fetch the data</p>
            </div>
            <div v-else-if="!state.loading && state.trafficSimData.length === 0" class="text-center py-5">
              <i class="bi bi-broadcast fs-1 text-muted mb-3"></i>
              <h5 class="text-muted">No Traffic Simulation Data Available</h5>
              <p class="text-muted">No traffic simulation data found for the selected time range</p>
            </div>
            <div v-else>
              <TrafficSimGraph :traffic-results="transformToTrafficSimResult(state.trafficSimData)" :intervalSec="state.probe?.interval_sec || 60" />
            </div>
          </div>
        </div>
      </div>

      <!-- MTR Map and Table -->
      <div class="col-sm-12" v-if="containsProbeType('MTR')">
        <div class="card mb-3">
          <div class="card-body">
            <h5 class="card-title">Traceroutes</h5>
            <p class="card-text">view the recent trace routes for the selected period of time</p>
            <div v-if="state.loading && state.mtrData.length === 0" class="text-center py-5">
              <div class="spinner-border text-primary" role="status">
                <span class="visually-hidden">Loading...</span>
              </div>
              <h3 class="mt-3 text-muted">Loading traceroute data...</h3>
              <p class="text-muted">Please wait while we fetch the data</p>
            </div>
            <div v-else-if="!state.loading && state.mtrData.length === 0" class="text-center py-5">
              <i class="bi bi-diagram-3 fs-1 text-muted mb-3"></i>
              <h5 class="text-muted">No Traceroute Data Available</h5>
              <p class="text-muted">No MTR data found for the selected time range</p>
            </div>
            <div v-else>
              <NetworkMap :mtrResults="transformMtrDataMulti(state.mtrData)" @node-select="onNodeSelect" />
              <div class="mtr-help-text">
                <i class="bi bi-info-circle"></i> Click on any node in the map to view detailed traceroute data
              </div>
              
              <!-- Notable traces section -->
              <div class="notable-traces mt-3">
                <div class="d-flex justify-content-between align-items-center mb-2">
                  <h6 class="mb-0">
                    <i class="bi bi-exclamation-triangle-fill text-warning me-2"></i>
                    Notable Traces
                    <span class="badge bg-secondary ms-2">{{ getNotableMtrResults(state.mtrData).length }}</span>
                  </h6>
                  <button class="btn btn-sm btn-outline-primary" @click="showMtrModal = true">
                    <i class="bi bi-list-ul"></i> View All ({{ state.mtrData.length }})
                  </button>
                </div>
                
                <div v-if="getNotableMtrResults(state.mtrData).length === 0" class="text-muted text-center py-3">
                  <i class="bi bi-check-circle text-success me-2"></i>
                  No issues detected in the selected time range
                </div>
                
                <div v-else id="mtrAccordion" class="accordion">
                  <div v-for="(item, index) in getNotableMtrResults(state.mtrData)" :key="`${item.data.id}-${index}`">
                    <div class="accordion-item">
                      <h2 :id="'heading' + item.data.id" class="accordion-header">
                        <button :aria-controls="'collapse' + item.data.id" :aria-expanded="false"
                                :data-bs-target="'#collapse' + item.data.id"
                                class="accordion-button collapsed" data-bs-toggle="collapse" type="button">
                          {{ formatMtrTimestamp(item.data) }}
                          <span v-if="item.reason.includes('triggered')" class="badge bg-warning text-dark ms-2">TRIGGERED</span>
                          <span v-if="item.reason.includes('packet-loss')" class="badge bg-danger ms-2">PACKET LOSS</span>
                          <span v-if="item.reason.includes('high-latency')" class="badge bg-orange ms-2">HIGH LATENCY</span>
                          <span v-if="item.reason.includes('route-change')" class="badge bg-info ms-2">ROUTE CHANGE</span>
                        </button>
                      </h2>
                      <div :id="'collapse' + item.data.id" :aria-labelledby="'heading' + item.data.id"
                           class="accordion-collapse collapse"
                           data-bs-parent="#mtrAccordion">
                        <div class="accordion-body p-0">
                          <MtrTable :probe-data="item.data" />
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
      </template>
    </div>
  </div>
  <!-- Loading state for entire page -->
  <div v-else-if="state.loading" class="container-fluid">
    <div class="d-flex justify-content-center align-items-center" style="min-height: 80vh;">
      <div class="text-center">
        <div class="spinner-border text-primary mb-3" style="width: 3rem; height: 3rem;" role="status">
          <span class="visually-hidden">Loading...</span>
        </div>
        <h3 class="text-muted">Loading probe data...</h3>
        <p class="text-muted">Fetching monitoring information</p>
      </div>
    </div>
  </div>
  
  <!-- Error state -->

  <!-- MTR Detail Modal -->
  <MtrDetailModal 
    :visible="showMtrModal" 
    :node="selectedNode" 
    :mtr-results="state.mtrData" 
    @close="closeMtrModal" 
  />
</div>
</template>

<style lang="scss" scoped>
.container-fluid { 
  padding: 1rem; 
}
.mb-3 { 
  margin-bottom: 1rem; 
}
.check-grid {
  display: grid;
  width: 100%;
  height: 100%;
  grid-template-columns: repeat(6, 1fr);
  grid-template-rows: repeat(12, minmax(8rem, 1fr));
  grid-gap: 0.5rem;
}

/* Loading spinner animation */
.spinner-border {
  animation: spinner-border .75s linear infinite;
}

/* Bootstrap Icons support (if not already included) */
.bi::before {
  display: inline-block;
  content: "";
  vertical-align: -.125em;
  background-repeat: no-repeat;
  background-size: 1rem 1rem;
}

/* MTR help text */
.mtr-help-text {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.75rem;
  padding: 0.5rem 0.75rem;
  background: rgba(59, 130, 246, 0.1);
  border-radius: 6px;
  color: #3b82f6;
  font-size: 0.85rem;
}

/* Custom badge colors */
.badge.bg-orange {
  background-color: #f97316 !important;
  color: white;
}
</style>