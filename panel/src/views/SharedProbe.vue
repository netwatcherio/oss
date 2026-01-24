<script lang="ts" setup>
import { ref, onMounted, onUnmounted, computed, reactive, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { PublicShareService } from '@/services/apiService';
import { since } from '@/time';
import LatencyGraph from '@/components/PingGraph.vue';
import TrafficSimGraph from '@/components/TrafficSimGraph.vue';
import MosGraph from '@/components/MosGraph.vue';
import NetworkMap from '@/components/NetworkMap.vue';
import MtrTable from '@/components/MtrTable.vue';
import MtrSummary from '@/components/MtrSummary.vue';
import MtrDetailModal from '@/components/MtrDetailModal.vue';
import VueDatePicker from '@vuepic/vue-datepicker';
import '@vuepic/vue-datepicker/dist/main.css';
import { themeService } from '@/services/themeService';
import type { PingResult, MtrResult, TrafficSimResult, ProbeData } from '@/types';

const route = useRoute();
const router = useRouter();

const token = computed(() => route.params.token as string);
const probeId = computed(() => Number(route.params.probeId));

// Session storage key for password
const getSessionKey = () => `share_password_${token.value}`;

// Theme detection for date picker
const isDark = ref(themeService.getTheme() === 'dark');
let themeUnsubscribe: (() => void) | null = null;

// State
const loading = ref(true);
const error = ref<string | null>(null);
const requiresPassword = ref(false);
const passwordInput = ref('');
const passwordError = ref<string | null>(null);
const authenticatedPassword = ref<string | null>(null);

// Agent/Probe data
const agent = ref<any>(null);
const probe = ref<any>(null);
const probeAgent = ref<any>(null);  // Target agent for AGENT probes
const probes = ref<any[]>([]);  // All probes for context

// Probe data
const state = reactive({
    pingData: [] as ProbeData[],
    mtrData: [] as ProbeData[],
    trafficSimData: [] as ProbeData[],
    timeRange: [new Date(Date.now() - 3*60*60*1000), new Date()] as [Date, Date],
    title: '' as string,
    ready: false,
    aggregationBucketSec: 0,  // Current aggregation bucket size (0 = no aggregation)
    // AGENT probe bidirectional support (matching Probe.vue)
    isAgentProbe: false,
    reciprocalProbe: null as any,
    selectedDirection: 0,  // 0 = forward, 1 = reverse
    agentPairData: [] as Array<{
        direction: 'forward' | 'reverse';
        probeId: number;
        sourceAgentId: number;
        targetAgentId: number;
        sourceAgentName: string;
        targetAgentName: string;
        pingData: ProbeData[];
        mtrData: ProbeData[];
        trafficSimData: ProbeData[];
    }>,
});

// Computed: get the active direction's data for AGENT probes
const activePingData = computed(() => {
    if (state.isAgentProbe && state.agentPairData.length > 0) {
        const pair = state.agentPairData[state.selectedDirection] || state.agentPairData[0];
        return pair?.pingData || [];
    }
    return state.pingData;
});

const activeMtrData = computed(() => {
    if (state.isAgentProbe && state.agentPairData.length > 0) {
        const pair = state.agentPairData[state.selectedDirection] || state.agentPairData[0];
        return pair?.mtrData || [];
    }
    return state.mtrData;
});

const activeTrafficSimData = computed(() => {
    if (state.isAgentProbe && state.agentPairData.length > 0) {
        const pair = state.agentPairData[state.selectedDirection] || state.agentPairData[0];
        return pair?.trafficSimData || [];
    }
    return state.trafficSimData;
});

// MTR Modal state (matching Probe.vue)
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

// MTR Pagination state (matching Probe.vue)
const mtrPage = ref(1);
const mtrPageSize = 10;

// Filter for notable MTR results (triggered, route changes, high loss)
const getNotableMtrResults = (mtrData: ProbeData[]): ProbeData[] => {
    // Return all for now - could filter to only notable traces
    return [...mtrData].sort((a, b) => 
        new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
    );
};

const getPaginatedMtrResults = (mtrData: ProbeData[], page: number) => {
    const notable = getNotableMtrResults(mtrData);
    const start = (page - 1) * mtrPageSize;
    const end = page * mtrPageSize;
    return {
        items: notable.slice(start, end),
        total: notable.length,
        currentPage: page,
        totalPages: Math.ceil(notable.length / mtrPageSize),
        hasNext: end < notable.length,
        hasPrev: page > 1
    };
};

const goToMtrPage = (page: number) => {
    mtrPage.value = page;
};

// Transform MTR data for NetworkMap component
function transformMtrDataMulti(rows: ProbeData[]): MtrResult[] {
    return rows.map((r) => r.payload as MtrResult).filter(Boolean);
}

// Helper: format date to RFC3339
function toRFC3339(v?: Date | string | number): string {
    return v instanceof Date ? v.toISOString() : typeof v === 'number' ? new Date(v).toISOString() : v || '';
}

// Transform PING data for graph
function transformPingDataMulti(rows: ProbeData[]): PingResult[] {
    return rows.map((r) => {
        const p = r.payload as any;
        if (!p) return null;
        
        const isAggregated = 'avgLatency' in p || 'minLatency' in p || 'latency' in p;
        
        if (isAggregated) {
            const MS_TO_NS = 1e6;
            return {
                start_timestamp: new Date(r.created_at),
                stop_timestamp: new Date(r.created_at),
                packets_recv: p.packetsRecv || 0,
                packets_sent: p.packetsSent || 0,
                packets_recv_duplicates: 0,
                packet_loss: p.packetLoss || 0,
                addr: '',
                min_rtt: (p.minLatency || 0) * MS_TO_NS,
                max_rtt: (p.maxLatency || 0) * MS_TO_NS,
                avg_rtt: (p.avgLatency || p.latency || 0) * MS_TO_NS,
                std_dev_rtt: 0
            } as PingResult;
        } else {
            return p as PingResult;
        }
    }).filter(Boolean) as PingResult[];
}

// Transform MTR data
function transformMtrData(data: ProbeData): MtrResult {
    return data.payload as MtrResult;
}

// Transform TrafficSim data
function transformToTrafficSimResult(rows: ProbeData[]): TrafficSimResult[] {
    return rows.map((r) => {
        const p = r.payload as any;
        return {
            averageRTT: p?.averageRTT ?? 0,
            minRTT: p?.minRTT ?? 0,
            maxRTT: p?.maxRTT ?? 0,
            lostPackets: p?.lostPackets ?? 0,
            totalPackets: p?.totalPackets ?? 0,
            outOfSequence: p?.outOfOrder ?? 0,
            duplicates: p?.duplicates ?? 0,
            reportTime: p?.timestamp ?? r.created_at,
        };
    }).sort((a, b) => new Date(a.reportTime).getTime() - new Date(b.reportTime).getTime());
}

// Calculate aggregation bucket size based on time range (matching Probe.vue logic)
// Goal: aim for ~500 data points regardless of time range
function calculateAggregateBucket(from: Date, to: Date): number {
    const rangeMs = to.getTime() - from.getTime();
    const rangeSec = rangeMs / 1000;
    const targetPoints = 500;
    
    if (rangeSec <= 60) return 0; // No aggregation for < 1 minute
    
    const idealBucket = Math.ceil(rangeSec / targetPoints);
    
    // Round to nice intervals
    if (idealBucket <= 10) return 10;
    if (idealBucket <= 30) return 30;
    if (idealBucket <= 60) return 60;
    if (idealBucket <= 120) return 120;
    if (idealBucket <= 300) return 300;
    if (idealBucket <= 600) return 600;
    if (idealBucket <= 1800) return 1800;
    if (idealBucket <= 3600) return 3600;
    if (idealBucket <= 7200) return 7200;
    if (idealBucket <= 14400) return 14400;
    if (idealBucket <= 21600) return 21600;
    return Math.ceil(idealBucket / 21600) * 21600;
}

// Check if probe has data of type
function containsProbeType(type: string): boolean {
    switch(type) {
        case 'PING': return state.pingData.length > 0;
        case 'MTR': return state.mtrData.length > 0;
        case 'TRAFFICSIM': return state.trafficSimData.length > 0;
        default: return false;
    }
}

// Load shared agent info first
async function loadShareInfo(): Promise<boolean> {
    try {
        const info = await PublicShareService.getInfo(token.value);
        
        if (info.expired) {
            error.value = 'This share link has expired.';
            return false;
        }
        
        if (info.has_password) {
            // Check if we have a cached password in sessionStorage
            const cachedPassword = sessionStorage.getItem(getSessionKey());
            if (cachedPassword) {
                // Try to use cached password
                try {
                    const result = await PublicShareService.getAgent(token.value, cachedPassword);
                    agent.value = result.agent;
                    probes.value = result.probes || [];
                    authenticatedPassword.value = cachedPassword;
                    return true;
                } catch {
                    // Cached password invalid, clear it
                    sessionStorage.removeItem(getSessionKey());
                }
            }
            requiresPassword.value = true;
            return false;
        }
        
        return true;
    } catch (err: any) {
        error.value = err.message || 'Failed to access shared link';
        return false;
    }
}

// Submit password
async function submitPassword() {
    passwordError.value = null;
    try {
        const result = await PublicShareService.getAgent(token.value, passwordInput.value);
        agent.value = result.agent;
        probes.value = result.probes || [];
        authenticatedPassword.value = passwordInput.value;
        requiresPassword.value = false;
        
        // Cache password in sessionStorage for this session
        sessionStorage.setItem(getSessionKey(), passwordInput.value);
        
        // Now load probe data
        await loadProbeData();
    } catch (err: any) {
        if (err.message === 'INVALID_PASSWORD') {
            passwordError.value = 'Incorrect password. Please try again.';
        } else {
            error.value = err.message || 'Failed to access shared agent';
        }
    }
}

// Load probe and its data
async function loadProbeData() {
    loading.value = true;
    state.ready = false;
    
    try {
        // Load agent and probes info if not already loaded
        if (!agent.value) {
            const result = await PublicShareService.getAgent(
                token.value, 
                authenticatedPassword.value || undefined
            );
            agent.value = result.agent;
            probes.value = result.probes || [];
        }
        
        // Find the specific probe
        probe.value = probes.value.find(p => p.id === probeId.value);
        if (!probe.value) {
            error.value = 'Probe not found';
            return;
        }
        
        // Determine title based on probe target
        const firstTarget = probe.value.targets?.[0];
        if (firstTarget?.agent_id) {
            // AGENT probe - try to get target agent name
            try {
                const targetAgent = await PublicShareService.getAgentName(
                    token.value, 
                    firstTarget.agent_id, 
                    authenticatedPassword.value || undefined
                );
                probeAgent.value = targetAgent;
                state.title = targetAgent.name || `Agent #${firstTarget.agent_id}`;
            } catch {
                state.title = `Agent #${firstTarget.agent_id}`;
            }
        } else if (firstTarget?.target) {
            state.title = String(firstTarget.target).split(':')[0] || firstTarget.target;
        } else {
            state.title = `${probe.value.type} #${probe.value.id}`;
        }
        
        // Detect AGENT probe type (matching Probe.vue logic)
        state.isAgentProbe = probe.value?.type === 'AGENT';
        state.reciprocalProbe = null;
        state.agentPairData = [];
        
        // For AGENT probes, find reciprocal probe from the loaded probes list
        if (state.isAgentProbe && firstTarget?.agent_id) {
            const targetAgentId = firstTarget.agent_id;
            // Find AGENT probe that targets the agent this link is for (reverse direction)
            const reciprocal = probes.value.find(p => 
                p.type === 'AGENT' && 
                p.id !== probe.value.id &&
                p.agent_id === targetAgentId  // Probe owned by target agent
            );
            if (reciprocal) {
                state.reciprocalProbe = reciprocal;
                console.log('[SharedProbe] Found reciprocal AGENT probe:', reciprocal.id);
            }
        }
        
        // Load probe data with aggregation based on time range
        const [from, to] = state.timeRange;
        const aggregateSec = calculateAggregateBucket(from, to);
        state.aggregationBucketSec = aggregateSec;
        console.log(`[SharedProbe] Loading data: aggregateSec=${aggregateSec}`);
        
        const baseParams = {
            from: toRFC3339(from),
            to: toRFC3339(to),
            password: authenticatedPassword.value || undefined
        };
        
        // Helper to parse ProbeData response
        const parseDataResult = (dataResult: { data: any[] }, targetProbeId: number): ProbeData[] => {
            const result: ProbeData[] = [];
            for (const item of (dataResult.data || [])) {
                const payload = typeof item.payload === 'string' ? JSON.parse(item.payload) : item.payload;
                result.push({
                    id: item.created_at,
                    probe_id: item.probe_id || targetProbeId,
                    probe_agent_id: item.probe_agent_id || 0,
                    agent_id: item.agent_id || 0,
                    triggered: item.triggered || false,
                    triggered_reason: item.triggered_reason || '',
                    type: item.type,
                    payload: payload,
                    created_at: item.created_at,
                    received_at: item.received_at || item.created_at,
                });
            }
            return result;
        };
        
        // For AGENT probes, fetch each sub-type separately (matching Probe.vue)
        if (state.isAgentProbe) {
            const pingAgg = aggregateSec > 0;
            const trafficAgg = aggregateSec > 0;
            
            // Fetch PING data with aggregation
            try {
                const pingResult = await PublicShareService.getProbeData(token.value, probeId.value, {
                    ...baseParams,
                    type: 'PING',
                    aggregate: pingAgg ? aggregateSec : undefined,
                    limit: pingAgg ? undefined : 300,
                });
                state.pingData = parseDataResult(pingResult, probeId.value);
                console.log(`[SharedProbe] PING: ${state.pingData.length} ${pingAgg ? 'aggregated' : 'raw'} rows`);
            } catch (err) { console.warn('[SharedProbe] Failed to fetch PING:', err); }
            
            // Fetch MTR data (no aggregation - need full hop data)
            try {
                const mtrResult = await PublicShareService.getProbeData(token.value, probeId.value, {
                    ...baseParams,
                    type: 'MTR',
                    limit: 300,
                });
                state.mtrData = parseDataResult(mtrResult, probeId.value);
                console.log(`[SharedProbe] MTR: ${state.mtrData.length} raw rows`);
            } catch (err) { console.warn('[SharedProbe] Failed to fetch MTR:', err); }
            
            // Fetch TRAFFICSIM data with aggregation
            try {
                const trafficResult = await PublicShareService.getProbeData(token.value, probeId.value, {
                    ...baseParams,
                    type: 'TRAFFICSIM',
                    aggregate: trafficAgg ? aggregateSec : undefined,
                    limit: trafficAgg ? undefined : 300,
                });
                state.trafficSimData = parseDataResult(trafficResult, probeId.value);
                console.log(`[SharedProbe] TRAFFICSIM: ${state.trafficSimData.length} ${trafficAgg ? 'aggregated' : 'raw'} rows`);
            } catch (err) { console.warn('[SharedProbe] Failed to fetch TRAFFICSIM:', err); }
        } else {
            // Non-AGENT probe: single request (aggregation for PING/TRAFFICSIM types)
            const probeType = probe.value.type as string;
            const useAgg = aggregateSec > 0 && (probeType === 'PING' || probeType === 'TRAFFICSIM');
            
            const forwardResult = await PublicShareService.getProbeData(token.value, probeId.value, {
                ...baseParams,
                aggregate: useAgg ? aggregateSec : undefined,
                type: useAgg ? probeType : undefined,
                limit: useAgg ? undefined : 500,
            });
            const forwardData = parseDataResult(forwardResult, probeId.value);
            
            // Separate by type
            state.pingData = forwardData.filter(d => d.type === 'PING');
            state.mtrData = forwardData.filter(d => d.type === 'MTR');
            state.trafficSimData = forwardData.filter(d => d.type === 'TRAFFICSIM');
        }
        
        // For AGENT probes, also load reverse direction and build agentPairData
        if (state.isAgentProbe && firstTarget?.agent_id) {
            const sourceAgentId = probe.value.agent_id || agent.value?.id || 0;
            const targetAgentId = firstTarget.agent_id;
            const sourceAgentName = agent.value?.name || `Agent ${sourceAgentId}`;
            const targetAgentName = probeAgent.value?.name || `Agent ${targetAgentId}`;
            
            // Forward direction (this probe: source → target)
            state.agentPairData.push({
                direction: 'forward',
                probeId: probe.value.id,
                sourceAgentId,
                targetAgentId,
                sourceAgentName,
                targetAgentName,
                pingData: state.pingData,
                mtrData: state.mtrData,
                trafficSimData: state.trafficSimData,
            });
            
            // If reciprocal probe exists, load its data (reverse: target → source)
            if (state.reciprocalProbe) {
                try {
                    const recipProbeId = state.reciprocalProbe.id;
                    let recipPing: ProbeData[] = [];
                    let recipMtr: ProbeData[] = [];
                    let recipTraffic: ProbeData[] = [];
                    
                    // Fetch PING with aggregation (matching Probe.vue logic)
                    const pingAgg = aggregateSec > 0;
                    try {
                        const pingResult = await PublicShareService.getProbeData(token.value, recipProbeId, {
                            ...baseParams,
                            type: 'PING',
                            aggregate: pingAgg ? aggregateSec : undefined,
                            limit: pingAgg ? undefined : 300,
                        });
                        recipPing = parseDataResult(pingResult, recipProbeId);
                        console.log(`[SharedProbe] Reverse PING: ${recipPing.length} ${pingAgg ? 'aggregated' : 'raw'} rows`);
                    } catch (err) { console.warn('[SharedProbe] Failed to fetch reverse PING:', err); }
                    
                    // Fetch MTR without aggregation (need full hop data)
                    try {
                        const mtrResult = await PublicShareService.getProbeData(token.value, recipProbeId, {
                            ...baseParams,
                            type: 'MTR',
                            limit: 300,
                        });
                        recipMtr = parseDataResult(mtrResult, recipProbeId);
                        console.log(`[SharedProbe] Reverse MTR: ${recipMtr.length} raw rows`);
                    } catch (err) { console.warn('[SharedProbe] Failed to fetch reverse MTR:', err); }
                    
                    // Fetch TRAFFICSIM with aggregation
                    const trafficAgg = aggregateSec > 0;
                    try {
                        const trafficResult = await PublicShareService.getProbeData(token.value, recipProbeId, {
                            ...baseParams,
                            type: 'TRAFFICSIM',
                            aggregate: trafficAgg ? aggregateSec : undefined,
                            limit: trafficAgg ? undefined : 300,
                        });
                        recipTraffic = parseDataResult(trafficResult, recipProbeId);
                        console.log(`[SharedProbe] Reverse TRAFFICSIM: ${recipTraffic.length} ${trafficAgg ? 'aggregated' : 'raw'} rows`);
                    } catch (err) { console.warn('[SharedProbe] Failed to fetch reverse TRAFFICSIM:', err); }
                    
                    state.agentPairData.push({
                        direction: 'reverse',
                        probeId: recipProbeId,
                        sourceAgentId: targetAgentId,
                        targetAgentId: sourceAgentId,
                        sourceAgentName: targetAgentName,
                        targetAgentName: sourceAgentName,
                        pingData: recipPing,
                        mtrData: recipMtr,
                        trafficSimData: recipTraffic,
                    });
                    console.log('[SharedProbe] Loaded reverse direction data');
                } catch (err) {
                    console.warn('[SharedProbe] Failed to load reciprocal probe data:', err);
                }
            }
        }
        
        state.ready = true;
    } catch (err: any) {
        if (err.message === 'PASSWORD_REQUIRED') {
            requiresPassword.value = true;
        } else {
            error.value = err.message || 'Failed to load probe data';
        }
    } finally {
        loading.value = false;
    }
}

// Navigate back to agent
function goBack() {
    router.push({ name: 'sharedAgent', params: { token: token.value } });
}

// Handler for explicit time range updates from date picker
const onTimeRangeUpdate = (newRange: [Date, Date] | null) => {
    if (!newRange || newRange.length !== 2 || !newRange[0] || !newRange[1]) {
        console.warn('[SharedProbe] Invalid time range update:', newRange);
        return;
    }
    console.log('[SharedProbe] Time range updated:', newRange[0].toISOString(), 'to', newRange[1].toISOString());
    // Force a new array reference to ensure reactivity
    state.timeRange = [new Date(newRange[0]), new Date(newRange[1])];
};

// Initial load
onMounted(async () => {
    // Subscribe to theme changes for date picker
    themeUnsubscribe = themeService.onThemeChange((theme) => {
        isDark.value = theme === 'dark';
    });
    
    const canProceed = await loadShareInfo();
    if (canProceed) {
        await loadProbeData();
    } else {
        loading.value = false;
    }
});

onUnmounted(() => {
    if (themeUnsubscribe) {
        themeUnsubscribe();
        themeUnsubscribe = null;
    }
});

// Debounced watch on timeRange to reload data when date picker changes
let timeRangeDebounceTimer: number | null = null;
watch(
    () => [state.timeRange[0]?.getTime(), state.timeRange[1]?.getTime()],
    (newVal, oldVal) => {
        // Skip if values are the same or initial mount
        if (!newVal[0] || !newVal[1]) return;
        if (oldVal && newVal[0] === oldVal[0] && newVal[1] === oldVal[1]) return;
        
        console.log('[SharedProbe] Time range changed, debouncing reload...');
        
        // Clear any pending reload
        if (timeRangeDebounceTimer) {
            clearTimeout(timeRangeDebounceTimer);
        }
        
        // Debounce reload by 500ms to avoid rapid-fire requests
        timeRangeDebounceTimer = window.setTimeout(() => {
            console.log('[SharedProbe] Debounced reload triggered');
            loadProbeData();
            timeRangeDebounceTimer = null;
        }, 500);
    },
    { deep: false }
);
</script>

<template>
    <div class="shared-probe-page">
        <!-- Header -->
        <header class="shared-header">
            <div class="header-content">
                <div class="brand">
                    <button class="back-btn" @click="goBack">
                        <i class="bi bi-arrow-left"></i>
                    </button>
                    <i class="bi bi-eye"></i>
                    <span>NetWatcher</span>
                    <span class="badge">Shared View</span>
                </div>
            </div>
        </header>
        
        <main class="shared-main">
            <!-- Loading State -->
            <div v-if="loading" class="loading-state">
                <i class="bi bi-arrow-repeat spin"></i>
                <p>Loading probe data...</p>
            </div>
            
            <!-- Error State -->
            <div v-else-if="error" class="error-state">
                <i class="bi bi-exclamation-triangle"></i>
                <p>{{ error }}</p>
                <button @click="goBack" class="back-link">
                    <i class="bi bi-arrow-left"></i> Back to Agent
                </button>
            </div>
            
            <!-- Password Required -->
            <div v-else-if="requiresPassword" class="password-state">
                <div class="password-card">
                    <div class="password-icon">
                        <i class="bi bi-lock"></i>
                    </div>
                    <h2>Password Required</h2>
                    <p>This shared page is password protected.</p>
                    
                    <form @submit.prevent="submitPassword" class="password-form">
                        <input 
                            type="password" 
                            v-model="passwordInput"
                            placeholder="Enter password"
                            class="password-input"
                            autofocus
                        />
                        <div v-if="passwordError" class="password-error">
                            <i class="bi bi-exclamation-circle"></i>
                            {{ passwordError }}
                        </div>
                        <button type="submit" class="password-submit">
                            <i class="bi bi-unlock"></i>
                            Unlock
                        </button>
                    </form>
                </div>
            </div>
            
            <!-- Probe Content -->
            <div v-else-if="state.ready" class="probe-content">
                <!-- Probe Header -->
                <div class="probe-header-section">
                    <div class="probe-title-row">
                        <div class="probe-icon" :class="probe?.type?.toLowerCase()">
                            <i :class="probe?.type === 'AGENT' ? 'bi bi-robot' : 
                                       probe?.type === 'PING' ? 'bi bi-broadcast-pin' :
                                       probe?.type === 'MTR' ? 'bi bi-diagram-2' :
                                       probe?.type === 'TRAFFICSIM' ? 'bi bi-speedometer' : 'bi bi-cpu'"></i>
                        </div>
                        <div class="probe-title-info">
                            <h1>{{ state.title }}</h1>
                            <div class="probe-meta">
                                <span class="probe-type-badge" :class="probe?.type?.toLowerCase()">
                                    {{ probe?.type }}
                                </span>
                                <span class="probe-interval" v-if="probe?.interval_sec">
                                    <i class="bi bi-clock"></i> {{ probe.interval_sec }}s interval
                                </span>
                            </div>
                        </div>
                    </div>
                    
                    <!-- Agent Info (for AGENT probes) -->
                    <div v-if="probeAgent" class="agent-context">
                        <span class="context-label">Target:</span>
                        <span class="context-value">{{ probeAgent.name }}</span>
                        <span v-if="probeAgent.location" class="context-location">
                            <i class="bi bi-geo-alt"></i> {{ probeAgent.location }}
                        </span>
                    </div>
                    
                    <!-- Date Range Picker -->
                    <div class="date-picker-wrapper">
                        <VueDatePicker 
                            v-model="state.timeRange"
                            @update:model-value="onTimeRangeUpdate"
                            :partial-range="false" 
                            range
                            :dark="isDark"
                            :enable-time-picker="true"
                            :multi-calendars="true"
                            :auto-apply="true"
                            :preset-dates="[
                                { label: 'Last Hour', value: [new Date(Date.now() - 60*60*1000), new Date()] },
                                { label: 'Last 3 Hours', value: [new Date(Date.now() - 3*60*60*1000), new Date()] },
                                { label: 'Last 6 Hours', value: [new Date(Date.now() - 6*60*60*1000), new Date()] },
                                { label: 'Last 24 Hours', value: [new Date(Date.now() - 24*60*60*1000), new Date()] },
                                { label: 'Last 7 Days', value: [new Date(Date.now() - 7*24*60*60*1000), new Date()] },
                                { label: 'Last 30 Days', value: [new Date(Date.now() - 30*24*60*60*1000), new Date()] }
                            ]"
                            format="MMM dd, yyyy HH:mm"
                            preview-format="MMM dd, yyyy HH:mm"
                            input-class-name="date-picker-input"
                            menu-class-name="date-picker-menu"
                            calendar-class-name="date-picker-calendar"
                        />
                    </div>
                </div>
                
                <!-- Direction Toggle for AGENT probes -->
                <div v-if="state.isAgentProbe && state.agentPairData.length > 1" class="direction-toggle">
                    <div class="direction-label">
                        <i class="bi bi-arrow-left-right"></i>
                        <span>Direction</span>
                    </div>
                    <div class="direction-buttons">
                        <button 
                            type="button" 
                            class="direction-btn"
                            :class="{ active: state.selectedDirection === 0 }"
                            @click="state.selectedDirection = 0">
                            <i class="bi bi-arrow-right"></i>
                            <span class="agent-name">{{ agent?.name || 'Source' }}</span>
                            <span class="direction-arrow">→</span>
                            <span class="agent-name">{{ probeAgent?.name || 'Target' }}</span>
                        </button>
                        <button 
                            type="button" 
                            class="direction-btn"
                            :class="{ active: state.selectedDirection === 1 }"
                            @click="state.selectedDirection = 1">
                            <i class="bi bi-arrow-left"></i>
                            <span class="agent-name">{{ probeAgent?.name || 'Target' }}</span>
                            <span class="direction-arrow">→</span>
                            <span class="agent-name">{{ agent?.name || 'Source' }}</span>
                        </button>
                    </div>
                </div>
                
                <!-- Data Tabs (for AGENT probes, use selected direction's data) -->
                <div class="data-tabs">
                    <!-- PING Data -->
                    <div v-if="containsProbeType('PING')" class="data-section">
                        <h2><i class="bi bi-broadcast-pin"></i> Latency</h2>
                        <div class="graph-container">
                            <LatencyGraph 
                                :pingResults="transformPingDataMulti(activePingData)" 
                                :aggregationBucketSec="state.aggregationBucketSec"
                                :currentTimeRange="state.timeRange"
                                @time-range-change="onTimeRangeUpdate"
                            />
                        </div>
                        
                        <!-- MOS Graph (Voice Quality) -->
                        <h3 class="subsection-title"><i class="bi bi-telephone"></i> Voice Quality (MOS)</h3>
                        <div class="graph-container">
                            <MosGraph 
                                :pingResults="transformPingDataMulti(activePingData)"
                                :aggregationBucketSec="state.aggregationBucketSec"
                            />
                        </div>
                        
                        <!-- PING Stats Summary -->
                        <div v-if="activePingData.length > 0" class="stats-summary">
                            <div class="stat-card">
                                <div class="stat-label">Data Points</div>
                                <div class="stat-value">{{ activePingData.length }}</div>
                            </div>
                        </div>
                    </div>
                    
                    <!-- MTR Data -->
                    <div v-if="containsProbeType('MTR')" class="data-section">
                        <h2><i class="bi bi-diagram-2"></i> Route Trace</h2>
                        
                        <!-- Network Map Visualization -->
                        <div v-if="activeMtrData.length > 0" class="network-map-container">
                            <h3 class="subsection-title"><i class="bi bi-bezier2"></i> Network Path</h3>
                            <NetworkMap 
                                :mtrResults="transformMtrDataMulti(activeMtrData)" 
                                @node-select="onNodeSelect"
                            />
                        </div>
                        
                        <!-- MTR Summary -->
                        <MtrSummary 
                            v-if="activeMtrData.length > 0" 
                            :mtrData="activeMtrData" 
                        />
                        
                        <!-- Paginated MTR Results -->
                        <div class="mtr-results">
                            <div class="mtr-results-header">
                                <h3>Recent Traces</h3>
                                <span class="mtr-count">{{ getPaginatedMtrResults(activeMtrData, mtrPage).total }} total</span>
                            </div>
                            
                            <div v-for="(mtr, idx) in getPaginatedMtrResults(activeMtrData, mtrPage).items" :key="idx" class="mtr-item">
                                <div class="mtr-header">
                                    <span class="mtr-time">
                                        {{ new Date(mtr.created_at).toLocaleString() }}
                                    </span>
                                </div>
                                <MtrTable :probeData="mtr" />
                            </div>
                            
                            <!-- Pagination Controls -->
                            <div v-if="getPaginatedMtrResults(activeMtrData, mtrPage).totalPages > 1" class="mtr-pagination">
                                <button 
                                    class="pagination-btn"
                                    :disabled="!getPaginatedMtrResults(activeMtrData, mtrPage).hasPrev"
                                    @click="goToMtrPage(mtrPage - 1)"
                                >
                                    <i class="bi bi-chevron-left"></i> Previous
                                </button>
                                <span class="pagination-info">
                                    Page {{ getPaginatedMtrResults(activeMtrData, mtrPage).currentPage }} 
                                    of {{ getPaginatedMtrResults(activeMtrData, mtrPage).totalPages }}
                                </span>
                                <button 
                                    class="pagination-btn"
                                    :disabled="!getPaginatedMtrResults(activeMtrData, mtrPage).hasNext"
                                    @click="goToMtrPage(mtrPage + 1)"
                                >
                                    Next <i class="bi bi-chevron-right"></i>
                                </button>
                            </div>
                        </div>
                    </div>
                    
                    <!-- TrafficSim Data -->
                    <div v-if="containsProbeType('TRAFFICSIM')" class="data-section">
                        <h2><i class="bi bi-speedometer"></i> Traffic Simulation</h2>
                        <div class="graph-container">
                            <TrafficSimGraph 
                                :trafficResults="transformToTrafficSimResult(activeTrafficSimData)"
                                :currentTimeRange="state.timeRange"
                                @time-range-change="onTimeRangeUpdate"
                            />
                        </div>
                    </div>
                    
                    <!-- No Data -->
                    <div v-if="!containsProbeType('PING') && !containsProbeType('MTR') && !containsProbeType('TRAFFICSIM')" class="no-data">
                        <i class="bi bi-inbox"></i>
                        <p>No data available for this probe in the selected time range.</p>
                    </div>
                </div>
            </div>
        </main>
        
        <!-- Footer -->
        <footer class="shared-footer">
            <p>
                <i class="bi bi-info-circle"></i>
                Shared via NetWatcher • Read-only view
            </p>
        </footer>
        
        <!-- MTR Detail Modal -->
        <MtrDetailModal
            :visible="showMtrModal"
            :node="selectedNode"
            :mtrResults="activeMtrData"
            @close="closeMtrModal"
        />
    </div>
</template>

<style scoped>
.shared-probe-page {
    min-height: 100vh;
    background: linear-gradient(135deg, #0f172a 0%, #1e293b 100%);
    color: #e2e8f0;
}

.shared-header {
    background: rgba(15, 23, 42, 0.9);
    border-bottom: 1px solid rgba(255, 255, 255, 0.1);
    padding: 1rem 1.5rem;
    position: sticky;
    top: 0;
    z-index: 100;
    backdrop-filter: blur(10px);
}

.header-content {
    max-width: 1400px;
    margin: 0 auto;
    display: flex;
    align-items: center;
    justify-content: space-between;
}

.brand {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    font-size: 1.25rem;
    font-weight: 600;
}

.back-btn {
    background: rgba(255, 255, 255, 0.1);
    border: none;
    color: #e2e8f0;
    padding: 0.5rem 0.75rem;
    border-radius: 6px;
    cursor: pointer;
    transition: all 0.2s;
    margin-right: 0.5rem;
}

.back-btn:hover {
    background: rgba(255, 255, 255, 0.2);
}

.badge {
    background: rgba(99, 102, 241, 0.2);
    color: #a5b4fc;
    padding: 0.25rem 0.75rem;
    border-radius: 9999px;
    font-size: 0.75rem;
    font-weight: 500;
}

.shared-main {
    max-width: 1400px;
    margin: 0 auto;
    padding: 2rem 1.5rem;
}

/* Loading State */
.loading-state {
    text-align: center;
    padding: 4rem 2rem;
}

.loading-state i {
    font-size: 2.5rem;
    margin-bottom: 1rem;
}

.spin {
    animation: spin 1s linear infinite;
}

@keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
}

/* Error State */
.error-state {
    text-align: center;
    padding: 4rem 2rem;
}

.error-state i {
    font-size: 3rem;
    color: #ef4444;
    margin-bottom: 1rem;
}

.back-link {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 1rem;
    padding: 0.75rem 1.5rem;
    background: rgba(99, 102, 241, 0.2);
    color: #a5b4fc;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    transition: all 0.2s;
}

.back-link:hover {
    background: rgba(99, 102, 241, 0.3);
}

/* Password State */
.password-state {
    display: flex;
    justify-content: center;
    padding: 4rem 1rem;
}

.password-card {
    background: rgba(255, 255, 255, 0.05);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 12px;
    padding: 2.5rem;
    text-align: center;
    max-width: 400px;
    width: 100%;
}

.password-icon {
    width: 64px;
    height: 64px;
    background: rgba(99, 102, 241, 0.2);
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    margin: 0 auto 1.5rem;
}

.password-icon i {
    font-size: 1.75rem;
    color: #a5b4fc;
}

.password-form {
    margin-top: 1.5rem;
}

.password-input {
    width: 100%;
    padding: 0.875rem 1rem;
    background: rgba(0, 0, 0, 0.3);
    border: 1px solid rgba(255, 255, 255, 0.15);
    border-radius: 8px;
    color: #fff;
    font-size: 1rem;
}

.password-error {
    color: #f87171;
    font-size: 0.875rem;
    margin-top: 0.75rem;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.375rem;
}

.password-submit {
    width: 100%;
    padding: 0.875rem;
    background: linear-gradient(135deg, #6366f1, #8b5cf6);
    border: none;
    border-radius: 8px;
    color: white;
    font-weight: 600;
    cursor: pointer;
    margin-top: 1rem;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    transition: all 0.2s;
}

.password-submit:hover {
    transform: translateY(-2px);
    box-shadow: 0 4px 12px rgba(99, 102, 241, 0.4);
}

/* Probe Content */
.probe-content {
    display: flex;
    flex-direction: column;
    gap: 2rem;
}

.probe-header-section {
    background: rgba(255, 255, 255, 0.03);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 12px;
    padding: 1.5rem;
}

.date-picker-wrapper {
    margin-top: 1rem;
    display: flex;
    gap: 0.5rem;
    align-items: center;
}

.date-picker-wrapper :deep(.dp__input) {
    background: rgba(255, 255, 255, 0.08);
    border: 1px solid rgba(255, 255, 255, 0.15);
    color: #e2e8f0;
    border-radius: 8px;
    padding: 0.5rem 1rem;
    font-size: 0.875rem;
}

.date-picker-wrapper :deep(.dp__input:hover) {
    border-color: rgba(99, 102, 241, 0.5);
}

.date-picker-wrapper :deep(.dp__input_icon) {
    color: #94a3b8;
}

.probe-title-row {
    display: flex;
    align-items: flex-start;
    gap: 1rem;
}

.probe-icon {
    width: 56px;
    height: 56px;
    background: rgba(59, 130, 246, 0.2);
    color: #93c5fd;
    border-radius: 12px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 1.5rem;
    flex-shrink: 0;
}

.probe-icon.agent { background: rgba(236, 72, 153, 0.2); color: #f9a8d4; }
.probe-icon.ping { background: rgba(34, 197, 94, 0.2); color: #86efac; }
.probe-icon.mtr { background: rgba(59, 130, 246, 0.2); color: #93c5fd; }
.probe-icon.trafficsim { background: rgba(168, 85, 247, 0.2); color: #d8b4fe; }

.probe-title-info h1 {
    margin: 0 0 0.5rem;
    font-size: 1.5rem;
    font-weight: 600;
}

.probe-meta {
    display: flex;
    align-items: center;
    gap: 1rem;
    flex-wrap: wrap;
}

.probe-type-badge {
    display: inline-block;
    padding: 0.25rem 0.75rem;
    background: rgba(243, 244, 246, 0.15);
    color: #9ca3af;
    border-radius: 4px;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
}

.probe-type-badge.ping { background: rgba(34, 197, 94, 0.2); color: #86efac; }
.probe-type-badge.mtr { background: rgba(59, 130, 246, 0.2); color: #93c5fd; }
.probe-type-badge.trafficsim { background: rgba(168, 85, 247, 0.2); color: #d8b4fe; }
.probe-type-badge.agent { background: rgba(236, 72, 153, 0.2); color: #f9a8d4; }

.probe-interval {
    font-size: 0.875rem;
    color: #9ca3af;
    display: flex;
    align-items: center;
    gap: 0.375rem;
}

.agent-context {
    margin-top: 1rem;
    padding-top: 1rem;
    border-top: 1px solid rgba(255, 255, 255, 0.1);
    display: flex;
    align-items: center;
    gap: 0.75rem;
    flex-wrap: wrap;
}

.context-label {
    color: #9ca3af;
    font-size: 0.875rem;
}

.context-value {
    font-weight: 600;
}

.context-location {
    color: #9ca3af;
    font-size: 0.875rem;
    display: flex;
    align-items: center;
    gap: 0.25rem;
}

/* Data Sections */
.data-tabs {
    display: flex;
    flex-direction: column;
    gap: 2rem;
}

.data-section {
    background: rgba(255, 255, 255, 0.03);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 12px;
    padding: 1.5rem;
}

.data-section h2 {
    font-size: 1.125rem;
    font-weight: 600;
    margin: 0 0 1rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

.graph-container {
    background: rgba(0, 0, 0, 0.2);
    border-radius: 8px;
    padding: 1rem;
    min-height: 300px;
}

.stats-summary {
    display: flex;
    gap: 1rem;
    margin-top: 1rem;
    flex-wrap: wrap;
}

.stat-card {
    background: rgba(0, 0, 0, 0.2);
    border-radius: 8px;
    padding: 1rem;
    min-width: 120px;
}

.stat-label {
    font-size: 0.75rem;
    color: #9ca3af;
    text-transform: uppercase;
    margin-bottom: 0.25rem;
}

.stat-value {
    font-size: 1.25rem;
    font-weight: 600;
}

/* MTR Results */
.mtr-results h3 {
    font-size: 1rem;
    font-weight: 500;
    margin: 1.5rem 0 1rem;
    color: #9ca3af;
}

.mtr-item {
    margin-bottom: 1.5rem;
}

.mtr-header {
    margin-bottom: 0.5rem;
}

.mtr-time {
    font-size: 0.875rem;
    color: #9ca3af;
}

/* No Data */
.no-data {
    text-align: center;
    padding: 3rem;
    color: #9ca3af;
}

.no-data i {
    font-size: 2.5rem;
    margin-bottom: 1rem;
    display: block;
}

/* Footer */
.shared-footer {
    margin-top: 3rem;
    padding: 1.5rem;
    text-align: center;
    border-top: 1px solid rgba(255, 255, 255, 0.1);
}

.shared-footer p {
    color: #666;
    font-size: 0.875rem;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
}

/* Direction Toggle for AGENT probes */
.direction-toggle {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 1rem 1.25rem;
    background: rgba(255, 255, 255, 0.02);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 12px;
    margin-bottom: 1.5rem;
}

.direction-label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    color: #9ca3af;
    font-size: 0.875rem;
    font-weight: 500;
}

.direction-buttons {
    display: flex;
    gap: 0.5rem;
    flex: 1;
}

.direction-btn {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    padding: 0.75rem 1rem;
    background: rgba(255, 255, 255, 0.05);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 8px;
    color: #9ca3af;
    font-size: 0.85rem;
    cursor: pointer;
    transition: all 0.2s;
}

.direction-btn:hover {
    background: rgba(255, 255, 255, 0.08);
    border-color: rgba(255, 255, 255, 0.2);
}

.direction-btn.active {
    background: rgba(99, 102, 241, 0.15);
    border-color: rgba(99, 102, 241, 0.4);
    color: #a5b4fc;
}

.direction-btn .agent-name {
    font-weight: 500;
    max-width: 120px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.direction-btn .direction-arrow {
    color: #6b7280;
    font-weight: 600;
}

/* Subsection titles */
.subsection-title {
    font-size: 1rem;
    font-weight: 500;
    color: #94a3b8;
    margin: 1.5rem 0 1rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

.subsection-title i {
    color: #6366f1;
}

/* Network Map Container */
.network-map-container {
    background: rgba(0, 0, 0, 0.2);
    border-radius: 12px;
    padding: 1rem;
    margin-bottom: 1.5rem;
}

/* MTR Results Header */
.mtr-results-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 1rem;
}

.mtr-results-header h3 {
    font-size: 1rem;
    font-weight: 500;
    margin: 0;
}

.mtr-count {
    font-size: 0.8rem;
    color: #6b7280;
    background: rgba(255, 255, 255, 0.05);
    padding: 0.25rem 0.75rem;
    border-radius: 999px;
}

/* MTR Pagination */
.mtr-pagination {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 1rem;
    margin-top: 1.5rem;
    padding-top: 1rem;
    border-top: 1px solid rgba(255, 255, 255, 0.1);
}

.pagination-btn {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    padding: 0.5rem 1rem;
    background: rgba(255, 255, 255, 0.05);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 6px;
    color: #94a3b8;
    font-size: 0.85rem;
    cursor: pointer;
    transition: all 0.2s;
}

.pagination-btn:hover:not(:disabled) {
    background: rgba(99, 102, 241, 0.15);
    border-color: rgba(99, 102, 241, 0.3);
    color: #a5b4fc;
}

.pagination-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
}

.pagination-info {
    font-size: 0.85rem;
    color: #6b7280;
}
</style>
