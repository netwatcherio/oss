<script lang="ts" setup>
import {nextTick, onMounted, onUnmounted, reactive, ref, watch} from "vue";
import core from "@/core";
import type {Agent, MtrResult, PingResult, Probe, ProbeData, ProbeType, TrafficSimResult, Workspace,} from "@/types";
import Title from "@/components/Title.vue";
import {AsciiTable3} from "@/lib/ascii-table3/ascii-table3";
import LatencyGraph from "@/components/PingGraph.vue";
import TrafficSimGraph from "@/components/TrafficSimGraph.vue";
import NetworkMap from "@/components/NetworkMap.vue";
import MtrTable from "@/components/MtrTable.vue";
import MtrDetailModal from "@/components/MtrDetailModal.vue";
import VueDatePicker from '@vuepic/vue-datepicker';
import '@vuepic/vue-datepicker/dist/main.css';
import { themeService } from '@/services/themeService';

// NEW: API services wired to your new endpoints
import {AgentService, ProbeDataService, ProbeService, WorkspaceService} from "@/services/apiService";
import {findMatchingProbesByProbeId, findProbesByInitialTarget} from "@/utils/probeGrouping";

// WebSocket for real-time updates
import { useProbeSubscription, type ProbeDataEvent } from "@/composables/useWebSocket";

// Ref for active tab to trigger NetworkMap updates
const activeTabIndex = ref(0);

// Theme detection for date picker
const isDark = ref(themeService.getTheme() === 'dark');

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
  state.selectedMtrData = [];  // Clear so modal falls back to state.mtrData for non-AGENT
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
  timeRange: [new Date(Date.now() - 3*60*60*1000), new Date()] as [Date, Date],
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
    direction: 'forward' | 'reverse',
    probeId: number,
    sourceAgentId: number,
    targetAgentId: number,
    sourceAgentName: string,
    targetAgentName: string,
    pingData: ProbeData[],
    mtrData: ProbeData[],
    trafficSimData: ProbeData[],
    rperfData: ProbeData[]
  }>,
  isAgentProbe: false,
  reciprocalProbe: null as Probe | null,  // The reverse AGENT probe if it exists
  selectedDirection: 0 as number,  // Index of selected direction tab
  selectedMtrData: [] as ProbeData[],  // MTR data for modal display (used by AGENT probe View All)
  rawGroups: {} as any,
});

// Pagination state for MTR results
const mtrPage = ref(1);
const mtrPageSize = 10;
const agentMtrPages = reactive<Record<number, number>>({});  // Per-agent-pair pagination

const getPaginatedMtrResults = (mtrData: ProbeData[], page: number) => {
  const notable = getNotableMtrResults(mtrData);
  const start = (page - 1) * mtrPageSize;
  const end = page * mtrPageSize;
  return {
    items: notable.slice(start, end),  // Show only current page
    total: notable.length,
    currentPage: page,
    totalPages: Math.ceil(notable.length / mtrPageSize),
    hasNext: end < notable.length,
    hasPrev: page > 1
  };
};

const goToMtrPage = (page: number, pairIndex?: number) => {
  if (pairIndex !== undefined) {
    agentMtrPages[pairIndex] = page;
  } else {
    mtrPage.value = page;
  }
};

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

// TRAFFICSIM: normalize series to match TrafficSimGraph component expectations
function transformToTrafficSimResult(rows: ProbeData[]): TrafficSimResult[] {
  return rows.map((r) => {
    const p = r.payload as any;
    return {
      averageRTT: p?.averageRTT ?? 0,
      minRTT: p?.minRTT ?? 0,
      maxRTT: p?.maxRTT ?? 0,
      lostPackets: p?.lostPackets ?? 0,
      totalPackets: p?.totalPackets ?? 0,
      outOfSequence: p?.outOfOrder ?? 0,  // Agent uses outOfOrder, graph expects outOfSequence
      duplicates: p?.duplicates ?? 0,  // Duplicate packets
      reportTime: p?.timestamp ?? r.created_at,
    };
  }).sort((a, b) => new Date(a.reportTime).getTime() - new Date(b.reportTime).getTime());
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
        direction: 'forward' as const,
        probeId: 0,  // Will be populated from actual probe if needed
        sourceAgentId: Number(sourceAgentId), 
        targetAgentId: Number(targetAgentId),
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
    
    // 4) Get first target early for multiple uses
    const firstTarget = (state.probe?.targets?.[0] ?? {}) as any;
    
    // 3a) Detect if this is an AGENT probe type for bidirectional display
    state.isAgentProbe = state.probe?.type === 'AGENT';
    state.reciprocalProbe = null;
    
    // 3b) If AGENT probe, look for reciprocal probe (B→A when we have A→B)
    if (state.isAgentProbe && firstTarget?.agent_id) {
      const targetAgentId = firstTarget.agent_id as number;
      try {
        // Get probes from the target agent
        const targetAgentProbes = (await ProbeService.list(workspaceID, String(targetAgentId))) as Probe[];
        // Find AGENT probe that targets our agent
        const reciprocal = targetAgentProbes.find(p => 
          p.type === 'AGENT' && 
          p.targets?.some(t => t.agent_id === Number(agentID))
        );
        if (reciprocal) {
          state.reciprocalProbe = reciprocal;
          console.log("Found reciprocal AGENT probe:", reciprocal.id);
        }
      } catch (e) {
        console.log("No reciprocal probe found:", e);
      }
    }

    // 5) Title from first target (agent ref vs literal)
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

    // 6) For AGENT probes, build agentPairData from collected data
    if (state.isAgentProbe && firstTarget?.agent_id != null) {
      const targetAgentId = firstTarget.agent_id as number;
      const sourceAgentId = Number(agentID);
      
      // Get agent names
      let sourceAgentName = state.agent?.name || `Agent ${sourceAgentId}`;
      let targetAgentName = state.probeAgent?.name || `Agent ${targetAgentId}`;
      
      // Build the agent pair data from loaded data
      state.agentPairData = [{
        direction: 'forward' as const,
        probeId: state.probe.id,
        sourceAgentId: sourceAgentId,
        targetAgentId: targetAgentId,
        sourceAgentName: sourceAgentName,
        targetAgentName: targetAgentName,
        pingData: state.pingData,
        mtrData: state.mtrData,
        trafficSimData: state.trafficSimData,
        rperfData: state.rperfData
      }];
      
      // If reciprocal probe exists, load its data too for in-page direction switching
      if (state.reciprocalProbe) {
        try {
          const recipProbe = state.reciprocalProbe;
          const [fromTime, toTime] = state.timeRange;
          
          // Calculate aggregation (same logic as loadProbeData - target 500 points)
          const rangeMs = new Date(toTime).getTime() - new Date(fromTime).getTime();
          const rangeSec = rangeMs / 1000;
          const targetPoints = 500;
          let recipAggregateSec = 0;
          
          if (rangeSec > 60) {
            const idealBucket = Math.ceil(rangeSec / targetPoints);
            if (idealBucket <= 10) recipAggregateSec = 10;
            else if (idealBucket <= 30) recipAggregateSec = 30;
            else if (idealBucket <= 60) recipAggregateSec = 60;
            else if (idealBucket <= 120) recipAggregateSec = 120;
            else if (idealBucket <= 300) recipAggregateSec = 300;
            else if (idealBucket <= 600) recipAggregateSec = 600;
            else if (idealBucket <= 1800) recipAggregateSec = 1800;
            else if (idealBucket <= 3600) recipAggregateSec = 3600;
            else recipAggregateSec = Math.ceil(idealBucket / 3600) * 3600;
          }
          
          const recipType = recipProbe.type as string;
          const useRecipAgg = recipAggregateSec > 0 && (recipType === 'PING' || recipType === 'TRAFFICSIM');
          
          const recipData = await ProbeDataService.byProbe(
            workspaceID, 
            recipProbe.id, 
            { 
              from: toRFC3339(fromTime), 
              to: toRFC3339(toTime), 
              // When aggregated, don't limit - bucket size controls volume
              limit: useRecipAgg ? undefined : 300,
              aggregate: useRecipAgg ? recipAggregateSec : undefined,
              type: useRecipAgg ? recipType : undefined
            }
          ) as ProbeData[];
          
          // Sort reciprocal data by type
          const recipPing = recipData.filter(d => d.type === 'PING');
          const recipMtr = recipData.filter(d => d.type === 'MTR');
          const recipTraffic = recipData.filter(d => d.type === 'TRAFFICSIM');
          const recipRperf = recipData.filter(d => d.type === 'RPERF');
          
          // Add reverse direction to agentPairData
          state.agentPairData.push({
            direction: 'reverse' as const,
            probeId: recipProbe.id,
            sourceAgentId: targetAgentId,
            targetAgentId: sourceAgentId,
            sourceAgentName: targetAgentName,
            targetAgentName: sourceAgentName,
            pingData: recipPing,
            mtrData: recipMtr,
            trafficSimData: recipTraffic,
            rperfData: recipRperf
          });
          
          console.log("Loaded reciprocal probe data:", recipPing.length, "ping,", recipMtr.length, "mtr,", recipTraffic.length, "trafficsim");
        } catch (e) {
          console.error("Failed to load reciprocal probe data:", e);
        }
      }
      
      console.log("Built agentPairData:", state.agentPairData.length, "pairs with", 
        state.pingData.length, "ping,", state.mtrData.length, "mtr,", 
        state.trafficSimData.length, "trafficsim");
    }

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
  
  // Calculate aggregation bucket size based on time range
  // Goal: aim for ~500 data points regardless of time range
  const fromDate = new Date(from);
  const toDate = new Date(to);
  const rangeMs = toDate.getTime() - fromDate.getTime();
  const rangeSec = rangeMs / 1000;
  const rangeHours = rangeSec / 3600;
  
  // Calculate bucket size to get ~500 points
  // bucketSec = rangeInSeconds / targetPoints
  const targetPoints = 500;
  let aggregateSec = 0;
  
  if (rangeSec > 60) { // Only aggregate if range > 1 minute
    // Calculate ideal bucket size
    let idealBucket = Math.ceil(rangeSec / targetPoints);
    
    // Round to nice intervals for cleaner data
    if (idealBucket <= 10) {
      aggregateSec = 10;        // 10 second buckets
    } else if (idealBucket <= 30) {
      aggregateSec = 30;        // 30 second buckets
    } else if (idealBucket <= 60) {
      aggregateSec = 60;        // 1 minute buckets
    } else if (idealBucket <= 120) {
      aggregateSec = 120;       // 2 minute buckets
    } else if (idealBucket <= 300) {
      aggregateSec = 300;       // 5 minute buckets
    } else if (idealBucket <= 600) {
      aggregateSec = 600;       // 10 minute buckets
    } else if (idealBucket <= 1800) {
      aggregateSec = 1800;      // 30 minute buckets
    } else if (idealBucket <= 3600) {
      aggregateSec = 3600;      // 1 hour buckets
    } else {
      aggregateSec = Math.ceil(idealBucket / 3600) * 3600; // Round to nearest hour
    }
  }

  console.log(`[Probe] Loading data: range=${rangeHours.toFixed(1)}h, idealBucket=${Math.ceil(rangeSec/targetPoints)}s, aggregate=${aggregateSec}s`);

  const tasks = state.probes.map(async (p) => {
    try {
      const probeType = p.type as string;
      const useAggregation = aggregateSec > 0 && (probeType === 'PING' || probeType === 'TRAFFICSIM');
      
      let rows;
      try {
        rows = await ProbeDataService.byProbe(
            workspaceID,
            p.id,
            { 
              from, 
              to, 
              // When aggregated, don't limit - bucket size controls data volume
              // When not aggregated (raw data), limit to avoid huge transfers
              limit: useAggregation ? undefined : 300,
              asc: false,
              aggregate: useAggregation ? aggregateSec : undefined,
              type: useAggregation ? probeType : undefined
            }
        );
        console.log(`[Probe ${p.id}] Fetched ${rows.length} ${useAggregation ? 'aggregated' : 'raw'} rows (type=${probeType}, bucket=${aggregateSec}s)`);
      } catch (aggErr) {
        // If aggregation fails (e.g., backend not updated), fallback to raw data
        if (useAggregation) {
          console.warn(`Aggregated fetch failed for probe ${p.id}, falling back to raw:`, aggErr);
          rows = await ProbeDataService.byProbe(
              workspaceID,
              p.id,
              { from, to, limit: 300, asc: false }
          );
        } else {
          throw aggErr;
        }
      }

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

  console.log("loaded probe data");

  // run them all in parallel, but don't throw if one fails
  await Promise.allSettled(tasks);
}

function onCreate(_: any) { router.push("/workspace"); }
function onError(response: any) { alert(response); }
function submit() {}

// Toggle to reciprocal direction (in-page, no navigation)
function switchToReciprocal() {
  if (!state.reciprocalProbe) return;
  state.selectedDirection = state.selectedDirection === 0 ? 1 : 0;
}

// Theme subscription for date picker
let themeUnsubscribe: (() => void) | null = null;

// Handler for explicit time range updates from date picker
const onTimeRangeUpdate = (newRange: [Date, Date] | null) => {
  if (!newRange || newRange.length !== 2 || !newRange[0] || !newRange[1]) {
    console.warn('[Probe] Invalid time range update:', newRange);
    return;
  }
  console.log('[Probe] Time range updated:', newRange[0].toISOString(), 'to', newRange[1].toISOString());
  // Force a new array reference to ensure reactivity
  state.timeRange = [new Date(newRange[0]), new Date(newRange[1])];
};

onMounted(() => {
  console.log('[Probe] Mounted with initial timeRange:', state.timeRange[0]?.toISOString(), 'to', state.timeRange[1]?.toISOString());
  
  // Subscribe to theme changes for date picker
  themeUnsubscribe = themeService.onThemeChange((theme) => {
    isDark.value = theme === 'dark';
  });
  
  // Load initial data (timeRange is already set in reactive state)
  reloadData();
});

onUnmounted(() => {
  if (themeUnsubscribe) {
    themeUnsubscribe();
    themeUnsubscribe = null;
  }
});

// WebSocket subscription for real-time updates
const workspaceIdRef = ref<number | undefined>(Number(workspaceID) || undefined);
const probeIdRef = ref<number | undefined>(Number(probeID) || undefined);

// Handler for incoming live probe data
const handleLiveProbeData = (data: ProbeDataEvent) => {
  console.log('[Probe] Live data received:', data.type, data.probe_id);
  
  // Convert WebSocket event to ProbeData format
  const probeData: ProbeData = {
    id: 0, // Will be assigned by addProbeDataUnique
    probe_id: data.probe_id,
    probe_agent_id: data.agent_id, // Same as agent_id for live data
    agent_id: data.agent_id,
    type: data.type as ProbeType,
    payload: data.payload,
    created_at: data.created_at,
    received_at: new Date().toISOString(),
    target: data.target || '',
    triggered: data.triggered || false,
    triggered_reason: '',
  };

  // Add to common bucket
  addProbeDataUnique(state.probeData, probeData);

  // Add to type-specific buckets
  if (data.type === 'PING') addProbeDataUnique(state.pingData, probeData);
  if (data.type === 'MTR') addProbeDataUnique(state.mtrData, probeData);
  if (data.type === 'TRAFFICSIM') addProbeDataUnique(state.trafficSimData, probeData);
  if (data.type === 'RPERF' && !(state.probe as any).server) addProbeDataUnique(state.rperfData, probeData);
};

// Set up WebSocket subscription for this probe
const { connected: wsConnected } = useProbeSubscription(
  workspaceIdRef,
  probeIdRef,
  handleLiveProbeData
);

// Watch for timeRange changes - skip if called during initial mount
let initialLoad = true;
watch(
  () => state.timeRange,
  (newRange, oldRange) => {
    // Skip the initial watch trigger since onMounted already calls reloadData
    if (initialLoad) {
      initialLoad = false;
      return;
    }
    // Validate time range
    if (!newRange || newRange.length !== 2 || !newRange[0] || !newRange[1]) {
      console.warn('[Probe] Watch: Invalid time range, skipping reload');
      return;
    }
    console.log('[Probe] Watch: Time range changed, reloading data...');
    console.log('[Probe] Old range:', oldRange?.[0]?.toISOString?.(), 'to', oldRange?.[1]?.toISOString?.());
    console.log('[Probe] New range:', newRange[0].toISOString(), 'to', newRange[1].toISOString());
    reloadData();
  },
  { deep: true }
);
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
      <div v-if="state.ready" class="d-flex gap-2 align-items-center date-picker-wrapper">
        <VueDatePicker 
          v-model="state.timeRange"
          @update:model-value="onTimeRangeUpdate"
          :partial-range="false" 
          range
          :dark="isDark"
          :enable-time-picker="true"
          :preset-dates="[
            { label: 'Last Hour', value: [new Date(Date.now() - 60*60*1000), new Date()] },
            { label: 'Last 3 Hours', value: [new Date(Date.now() - 3*60*60*1000), new Date()] },
            { label: 'Last 6 Hours', value: [new Date(Date.now() - 6*60*60*1000), new Date()] },
            { label: 'Last 24 Hours', value: [new Date(Date.now() - 24*60*60*1000), new Date()] },
            { label: 'Last 7 Days', value: [new Date(Date.now() - 7*24*60*60*1000), new Date()] },
            { label: 'Last 30 Days', value: [new Date(Date.now() - 30*24*60*60*1000), new Date()] }
          ]"
          :timezone="Intl.DateTimeFormat().resolvedOptions().timeZone"
          format="MMM dd, yyyy HH:mm"
          preview-format="MMM dd, yyyy HH:mm"
          input-class-name="date-picker-input"
          menu-class-name="date-picker-menu"
          calendar-class-name="date-picker-calendar"
        />
      </div>
    </Title>
    
    <!-- Direction Selector for AGENT probes with reciprocal -->
    <div v-if="state.ready && state.isAgentProbe && state.reciprocalProbe" class="direction-selector-wrapper mb-3">
      <div class="direction-selector">
        <div class="direction-label">
          <i class="bi bi-arrow-left-right"></i>
          <span>Direction</span>
        </div>
        <div class="direction-buttons" role="group" aria-label="probe direction">
          <button 
            type="button" 
            class="direction-btn"
            :class="{ active: state.selectedDirection === 0 }"
            @click="state.selectedDirection = 0">
            <i class="bi bi-arrow-right"></i>
            <span class="agent-name">{{ state.agent.name }}</span>
            <span class="direction-arrow">→</span>
            <span class="agent-name">{{ state.probeAgent.name || 'Target' }}</span>
          </button>
          <button 
            type="button" 
            class="direction-btn"
            :class="{ active: state.selectedDirection === 1 }"
            @click="switchToReciprocal">
            <i class="bi bi-arrow-left"></i>
            <span class="agent-name">{{ state.probeAgent.name || 'Target' }}</span>
            <span class="direction-arrow">→</span>
            <span class="agent-name">{{ state.agent.name }}</span>
          </button>
        </div>
      </div>
    </div>
    
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
        <!-- Direction content - use v-if for proper D3/NetworkMap rendering -->
        <template v-for="(pair, index) in state.agentPairData" :key="`content-${index}`">
          <div v-if="index === state.selectedDirection">
          
            <div class="row">
            <!-- Ping/Latency Data -->
            <div class="col-lg-12 mb-3">
              <div class="card h-100">
                <div class="card-header">
                  <h6 class="mb-0">
                    <i class="bi bi-speedometer2 me-2"></i>
                    Latency ({{ pair.sourceAgentName }} → {{ pair.targetAgentName }})
                  </h6>
                </div>
                <div class="card-body">
                  <div v-if="state.loading && pair.pingData.length === 0" class="text-center py-4">
                    <div class="spinner-border spinner-border-sm text-primary me-2" role="status"></div>
                    <span class="text-muted">Loading latency data...</span>
                  </div>
                  <div v-else-if="pair.pingData.length === 0" class="text-center py-4 text-muted">
                    <i class="bi bi-graph-down fs-1 mb-2 d-block"></i>
                    <p class="mb-0">No latency data available for this direction</p>
                  </div>
                  <LatencyGraph v-else :pingResults="transformPingDataMulti(pair.pingData)" :intervalSec="state.probe?.interval_sec || 60" />
                </div>
              </div>
            </div>
          </div>
          
          <div class="row">
            <!-- Traffic Sim Data -->
            <div class="col-lg-12 mb-3">
              <div class="card h-100">
                <div class="card-header">
                  <h6 class="mb-0">
                    <i class="bi bi-broadcast me-2"></i>
                    Simulated Traffic ({{ pair.sourceAgentName }} → {{ pair.targetAgentName }})
                  </h6>
                </div>
                <div class="card-body">
                  <div v-if="state.loading && pair.trafficSimData.length === 0" class="text-center py-4">
                    <div class="spinner-border spinner-border-sm text-primary me-2" role="status"></div>
                    <span class="text-muted">Loading traffic simulation data...</span>
                  </div>
                  <div v-else-if="pair.trafficSimData.length === 0" class="text-center py-4 text-muted">
                    <i class="bi bi-broadcast fs-1 mb-2 d-block"></i>
                    <p class="mb-0">No traffic simulation data available for this direction</p>
                  </div>
                  <TrafficSimGraph v-else :traffic-results="transformToTrafficSimResult(pair.trafficSimData)" :intervalSec="state.probe?.interval_sec || 60" />
                </div>
              </div>
            </div>
            
            <!-- MTR Data -->
            <div class="col-12 mb-3">
              <div class="card">
                <div class="card-header">
                  <h6 class="mb-0">
                    <i class="bi bi-diagram-3 me-2"></i>
                    Traceroutes ({{ pair.sourceAgentName }} → {{ pair.targetAgentName }})
                  </h6>
                </div>
                <div class="card-body">
                  <div v-if="state.loading && pair.mtrData.length === 0" class="text-center py-4">
                    <div class="spinner-border spinner-border-sm text-primary me-2" role="status"></div>
                    <span class="text-muted">Loading traceroute data...</span>
                  </div>
                  <div v-else-if="pair.mtrData.length === 0" class="text-center py-4 text-muted">
                    <i class="bi bi-diagram-3 fs-1 mb-2 d-block"></i>
                    <p class="mb-0">No traceroute data available for this direction</p>
                  </div>
                  <template v-else>
                    <!-- Key to force re-render on tab change -->
                    <NetworkMap 
                      :key="`mtr-map-${index}-${activeTabIndex}`"
                      :mtrResults="transformMtrDataMulti(pair.mtrData)"
                      @nodeSelect="onNodeSelect"
                    />
                    <div class="mtr-help-text">
                      <i class="bi bi-info-circle"></i> Click on any node in the map to view detailed traceroute data
                    </div>
                    
                    <!-- Notable Traces section for AGENT probes -->
                    <div class="notable-traces mt-3">
                      <div class="d-flex justify-content-between align-items-center mb-2">
                        <h6 class="mb-0">
                          <i class="bi bi-exclamation-triangle-fill text-warning me-2"></i>
                          Notable Traces
                          <span class="badge bg-secondary ms-2">{{ getNotableMtrResults(pair.mtrData).length }}</span>
                        </h6>
                        <button class="btn btn-sm btn-outline-primary" @click="showMtrModal = true; state.selectedMtrData = pair.mtrData">
                          <i class="bi bi-list-ul"></i> View All ({{ pair.mtrData.length }})
                        </button>
                      </div>
                      
                      <div v-if="getNotableMtrResults(pair.mtrData).length === 0" class="text-muted text-center py-3">
                        <i class="bi bi-check-circle text-success me-2"></i>
                        No issues detected in the selected time range
                      </div>
                      
                      <div v-else :id="`agent-notableAccordion-${index}`" class="accordion">
                        <div v-for="(item, notableIdx) in getPaginatedMtrResults(pair.mtrData, agentMtrPages[index] || 1).items" :key="`notable-${index}-${item.data.id}-${notableIdx}`">
                          <div class="accordion-item">
                            <h2 :id="`agent-notable-heading-${index}-${notableIdx}`" class="accordion-header">
                              <button :aria-controls="`agent-notable-collapse-${index}-${notableIdx}`" :aria-expanded="false"
                                      :data-bs-target="`#agent-notable-collapse-${index}-${notableIdx}`"
                                      class="accordion-button collapsed" data-bs-toggle="collapse" type="button">
                                {{ formatMtrTimestamp(item.data) }}
                                <span v-if="item.reason.includes('triggered')" class="badge bg-warning text-dark ms-2">TRIGGERED</span>
                                <span v-if="item.reason.includes('packet-loss')" class="badge bg-danger ms-2">PACKET LOSS</span>
                                <span v-if="item.reason.includes('high-latency')" class="badge bg-orange ms-2">HIGH LATENCY</span>
                                <span v-if="item.reason.includes('route-change')" class="badge bg-info ms-2">ROUTE CHANGE</span>
                              </button>
                            </h2>
                            <div :id="`agent-notable-collapse-${index}-${notableIdx}`" :aria-labelledby="`agent-notable-heading-${index}-${notableIdx}`"
                                 class="accordion-collapse collapse"
                                 :data-bs-parent="`#agent-notableAccordion-${index}`">
                              <div class="accordion-body p-0">
                                <MtrTable :probe-data="item.data" />
                              </div>
                            </div>
                          </div>
                        </div>
                        <!-- Pagination Controls per pair -->
                        <nav v-if="getPaginatedMtrResults(pair.mtrData, agentMtrPages[index] || 1).totalPages > 1" class="mt-3">
                          <ul class="pagination pagination-sm justify-content-center mb-0">
                            <li class="page-item" :class="{ disabled: !getPaginatedMtrResults(pair.mtrData, agentMtrPages[index] || 1).hasPrev }">
                              <button class="page-link" @click="goToMtrPage((agentMtrPages[index] || 1) - 1, index)">
                                <i class="bi bi-chevron-left"></i>
                              </button>
                            </li>
                            <li v-for="p in getPaginatedMtrResults(pair.mtrData, agentMtrPages[index] || 1).totalPages" :key="p" 
                                class="page-item" :class="{ active: p === (agentMtrPages[index] || 1) }">
                              <button class="page-link" @click="goToMtrPage(p, index)">{{ p }}</button>
                            </li>
                            <li class="page-item" :class="{ disabled: !getPaginatedMtrResults(pair.mtrData, agentMtrPages[index] || 1).hasNext }">
                              <button class="page-link" @click="goToMtrPage((agentMtrPages[index] || 1) + 1, index)">
                                <i class="bi bi-chevron-right"></i>
                              </button>
                            </li>
                          </ul>
                        </nav>
                      </div>
                    </div>
                  </template>
                </div>
              </div>
            </div>
          </div>
        </div>
        </template>
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
                  <div v-for="(item, index) in getPaginatedMtrResults(state.mtrData, mtrPage).items" :key="`${item.data.id}-${index}`">
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
                  <!-- Pagination Controls -->
                  <nav v-if="getPaginatedMtrResults(state.mtrData, mtrPage).totalPages > 1" class="mt-3">
                    <ul class="pagination pagination-sm justify-content-center mb-0">
                      <li class="page-item" :class="{ disabled: !getPaginatedMtrResults(state.mtrData, mtrPage).hasPrev }">
                        <button class="page-link" @click="goToMtrPage(mtrPage - 1)">
                          <i class="bi bi-chevron-left"></i>
                        </button>
                      </li>
                      <li v-for="p in getPaginatedMtrResults(state.mtrData, mtrPage).totalPages" :key="p" 
                          class="page-item" :class="{ active: p === mtrPage }">
                        <button class="page-link" @click="goToMtrPage(p)">{{ p }}</button>
                      </li>
                      <li class="page-item" :class="{ disabled: !getPaginatedMtrResults(state.mtrData, mtrPage).hasNext }">
                        <button class="page-link" @click="goToMtrPage(mtrPage + 1)">
                          <i class="bi bi-chevron-right"></i>
                        </button>
                      </li>
                    </ul>
                  </nav>
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
    :mtr-results="state.selectedMtrData.length > 0 ? state.selectedMtrData : state.mtrData" 
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

/* Direction Selector */
.direction-selector-wrapper {
  padding: 0 0.5rem;
}

.direction-selector {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.75rem 1rem;
  background: var(--bs-body-bg, #fff);
  border: 1px solid var(--bs-border-color, #dee2e6);
  border-radius: 8px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
}

.direction-label {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: var(--bs-secondary-color, #6c757d);
  font-size: 0.875rem;
  font-weight: 500;
  white-space: nowrap;
}

.direction-label i {
  font-size: 1rem;
}

.direction-buttons {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.direction-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 1rem;
  background: var(--bs-tertiary-bg, #f8f9fa);
  border: 2px solid transparent;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--bs-body-color, #212529);
  transition: all 0.2s ease;
}

.direction-btn:hover {
  background: rgba(59, 130, 246, 0.1);
  border-color: rgba(59, 130, 246, 0.3);
}

.direction-btn.active {
  background: #3b82f6;
  border-color: #3b82f6;
  color: white;
}

.direction-btn.active .direction-arrow {
  color: rgba(255, 255, 255, 0.8);
}

.direction-btn .agent-name {
  font-weight: 600;
}

.direction-btn .direction-arrow {
  color: var(--bs-secondary-color, #6c757d);
  font-weight: 400;
}

.direction-btn i {
  font-size: 0.75rem;
  opacity: 0.7;
}

/* Dark mode support */
[data-theme="dark"] .direction-selector {
  background: #1e293b;
  border-color: #334155;
}

[data-theme="dark"] .direction-btn {
  background: #334155;
  color: #e2e8f0;
}

[data-theme="dark"] .direction-btn:hover {
  background: rgba(59, 130, 246, 0.2);
  border-color: rgba(59, 130, 246, 0.5);
}

/* Date Picker Styling */
.date-picker-wrapper {
  min-width: 280px;
}

:global(.date-picker-input) {
  padding: 8px 12px !important;
  border-radius: 8px !important;
  border: 1px solid #e5e7eb !important;
  font-size: 14px !important;
  min-width: 260px;
  background: white !important;
  color: #374151 !important;
  transition: all 0.2s ease !important;
}

:global(.date-picker-input:hover) {
  border-color: #3b82f6 !important;
}

:global(.date-picker-input:focus) {
  border-color: #3b82f6 !important;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1) !important;
  outline: none !important;
}

/* Dark mode date picker input */
:global([data-theme="dark"] .date-picker-input) {
  background: #1e293b !important;
  border-color: #475569 !important;
  color: #e2e8f0 !important;
}

:global([data-theme="dark"] .date-picker-input:hover) {
  border-color: #3b82f6 !important;
}

/* Date picker menu styling */
:global(.dp__menu) {
  border-radius: 12px !important;
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.15) !important;
  border: 1px solid #e5e7eb !important;
}

:global([data-theme="dark"] .dp__menu) {
  border-color: #475569 !important;
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.4) !important;
}

/* Preset dates sidebar */
:global(.dp__preset_ranges) {
  border-right: 1px solid #e5e7eb !important;
  padding: 8px !important;
}

:global([data-theme="dark"] .dp__preset_ranges) {
  border-right-color: #475569 !important;
}

:global(.dp__preset_range) {
  padding: 8px 12px !important;
  border-radius: 6px !important;
  margin-bottom: 4px !important;
  font-size: 13px !important;
  transition: all 0.15s ease !important;
}

:global(.dp__preset_range:hover) {
  background: rgba(59, 130, 246, 0.1) !important;
  color: #3b82f6 !important;
}

:global([data-theme="dark"] .dp__preset_range:hover) {
  background: rgba(59, 130, 246, 0.2) !important;
}

/* Calendar cells */
:global(.dp__cell_inner) {
  border-radius: 6px !important;
  transition: all 0.15s ease !important;
}

:global(.dp__today) {
  border-color: #3b82f6 !important;
}

:global(.dp__active_date),
:global(.dp__range_start),
:global(.dp__range_end) {
  background: #3b82f6 !important;
}

:global(.dp__range_between) {
  background: rgba(59, 130, 246, 0.15) !important;
}
</style>