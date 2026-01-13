import request from "./request";
import type { Agent, Probe, ProbeCreateInput, ProbeData, Workspace, Member, Role, ListResponse } from "@/types";

/** ===== Auth ===== */
export const AuthService = {
    async login(email: string, password: string) {
        const { data } = await request.post<{ token: string; data?: any }>("/auth/login", { email, password });
        return data;
    },
    async register(body: { email: string; password: string; name?: string }) {
        const { data } = await request.post("/auth/register", body);
        return data;
    },
    async health() {
        const { data } = await request.get("/healthz");
        return data;
    },
};

function qs(params?: Record<string, string | number | undefined>) {
    const u = new URLSearchParams();
    if (!params) return '';
    for (const [k, v] of Object.entries(params)) {
        if (v === undefined || v === null) continue;
        u.set(k, String(v));
    }
    const s = u.toString();
    return s ? `?${s}` : '';
}

export const WorkspaceService = {
    // ---- Workspaces ----
    async list(params?: { q?: string; limit?: number; offset?: number }) {
        const { data } = await request.get<Workspace[]>(`/workspaces${qs(params)}`);
        return data;
    },

    async create(body: { name: string; description?: string; }) {
        const { data } = await request.post<Workspace>('/workspaces', body);
        return data;
    },

    async get(id: number | string) {
        const { data } = await request.get<Workspace>(`/workspaces/${id}`);
        return data;
    },

    // PATCH expects { name?, description?, settings? }
    async update(
        id: number | string,
        body: { name?: string; description?: string; settings?: Record<string, any> }
    ) {
        const { data } = await request.patch<Workspace>(`/workspaces/${id}`, body);
        return data;
    },

    async remove(id: number | string) {
        const { data } = await request.delete<{ ok: boolean }>(`/workspaces/${id}`);
        return data;
    },

    // ---- Members ----
    async listMembers(workspaceId: number | string) {
        const { data } = await request.get<ListResponse<Member>>(`/workspaces/${workspaceId}/members`);
        return data.data;
    },

    async addMember(
        workspaceId: number | string,
        body: { userId?: number; email?: string; role: Role; meta?: Record<string, any> }
    ) {
        const { data } = await request.post<Member>(`/workspaces/${workspaceId}/members`, body);
        return data;
    },

    async updateMemberRole(
        workspaceId: number | string,
        memberId: number | string,
        role: Exclude<Role, 'OWNER'>
    ) {
        const { data } = await request.patch<Member>(
            `/workspaces/${workspaceId}/members/${memberId}`,
            { role }
        );
        return data;
    },

    async removeMember(workspaceId: number | string, memberId: number | string) {
        const { data } = await request.delete<{ ok: boolean }>(
            `/workspaces/${workspaceId}/members/${memberId}`
        );
        return data;
    },

    // ---- Invitations ----
    async acceptInvite(workspaceId: number | string, email: string) {
        const { data } = await request.post<Member>(`/workspaces/${workspaceId}/accept-invite`, {
            email,
        });
        return data;
    },

    // ---- Ownership ----
    async transferOwnership(workspaceId: number | string, newOwnerUserId: number) {
        const { data } = await request.post<{ ok: boolean }>(
            `/workspaces/${workspaceId}/transfer-ownership`,
            { newOwnerUserId }
        );
        return data;
    },
};
/** ===== Agents (scoped to workspace) ===== */
export const AgentService = {
    async list(workspaceId: number | string, params?: { limit?: number; offset?: number }) {
        const qs = new URLSearchParams();
        if (params?.limit) qs.set("limit", String(params.limit));
        if (params?.offset) qs.set("offset", String(params.offset));
        const { data } = await request.get<{ data: Agent[]; total: number; limit: number; offset: number }>(
            `/workspaces/${workspaceId}/agents${qs.toString() ? `?${qs}` : ""}`
        );
        return data;
    },
    async create(workspaceId: number | string, body: Partial<Agent> & { pinLength?: number; pinTTLSeconds?: number }) {
        const { data } = await request.post(`/workspaces/${workspaceId}/agents`, body);
        // server returns agent + bootstrap PIN bundle
        return data as { agent: Agent; pin?: string; expiresAt?: string } | Agent;
    },
    async get(workspaceId: number | string, agentId: number | string) {
        const { data } = await request.get<Agent>(`/workspaces/${workspaceId}/agents/${agentId}`);
        return data;
    },
    async update(workspaceId: number | string, agentId: number | string, body: Partial<Agent>) {
        const { data } = await request.patch<Agent>(`/workspaces/${workspaceId}/agents/${agentId}`, body);
        return data;
    },
    async remove(workspaceId: number | string, agentId: number | string) {
        const { data } = await request.delete<{ ok: boolean }>(`/workspaces/${workspaceId}/agents/${agentId}`);
        return data;
    },
    async heartbeat(workspaceId: number | string, agentId: number | string) {
        const { data } = await request.post<{ ok: boolean; ts: string }>(
            `/workspaces/${workspaceId}/agents/${agentId}/heartbeat`
        );
        return data;
    },
    async issuePin(workspaceId: number | string, agentId: number | string, body?: { pinLength?: number; ttlSeconds?: number }) {
        const { data } = await request.post(`/workspaces/${workspaceId}/agents/${agentId}/issue-pin`, body ?? {});
        return data as { pin: string; expiresAt?: string;[k: string]: any };
    },
};

/** ===== Utils (local) ===== */
const toRFC3339 = (v?: string | number | Date): string | undefined => {
    if (v == null || v === "") return undefined;
    if (v instanceof Date) return v.toISOString();
    if (typeof v === "number") return new Date(v * (v < 2_000_000_000 ? 1000 : 1)).toISOString(); // secs or ms
    return v; // assume already RFC3339 string
};
const setIf = (qs: URLSearchParams, key: string, val: any) => {
    if (val === undefined || val === null || val === "") return;
    qs.set(key, String(val));
};

/** ===== Probe Data (ClickHouse-backed) ===== */
export const ProbeDataService = {
    /**
     * Flexible finder across ClickHouse.
     * Mirrors backend /workspaces/{id}/probe-data/find
     */
    async find(
        workspaceId: number | string,
        params?: {
            type?: string;
            probeId?: number | string;
            agentId?: number | string;         // reporting agent
            probeAgentId?: number | string;    // owner of probe_id
            targetAgent?: number | string;     // reverse target agent
            targetPrefix?: string;
            triggered?: boolean;
            from?: string | number | Date;
            to?: string | number | Date;
            limit?: number;
            asc?: boolean;
        }
    ) {
        const qs = new URLSearchParams();
        if (params) {
            setIf(qs, "type", params.type);
            setIf(qs, "probeId", params.probeId);
            setIf(qs, "agentId", params.agentId);
            setIf(qs, "probeAgentId", params.probeAgentId);
            setIf(qs, "targetAgent", params.targetAgent);
            setIf(qs, "targetPrefix", params.targetPrefix);
            if (params.triggered !== undefined) setIf(qs, "triggered", params.triggered ? "true" : "false");
            setIf(qs, "from", toRFC3339(params.from));
            setIf(qs, "to", toRFC3339(params.to));
            setIf(qs, "limit", params.limit);
            if (params.asc !== undefined) setIf(qs, "asc", params.asc ? "true" : "false");
        }
        const { data } = await request.get<ListResponse<ProbeData>>(
            `/workspaces/${workspaceId}/probe-data/find${qs.toString() ? `?${qs}` : ""}`
        );
        return data?.data || [];
    },

    /**
     * Timeseries for a specific probe.
     * GET /workspaces/{id}/probe-data/probes/{probeID}/data
     * 
     * @param aggregate - Aggregation bucket size in seconds (e.g., 60 = 1 min buckets)
     * @param type - "PING" or "TRAFFICSIM" - required when using aggregate
     */
    async byProbe(
        workspaceId: number | string,
        probeId: number | string,
        params?: {
            from?: string | number | Date;
            to?: string | number | Date;
            limit?: number;
            asc?: boolean;
            aggregate?: number;  // Seconds for time-bucket aggregation
            type?: string;       // "PING" or "TRAFFICSIM"
        }
    ) {
        const qs = new URLSearchParams();
        if (params) {
            setIf(qs, "from", toRFC3339(params.from));
            setIf(qs, "to", toRFC3339(params.to));
            setIf(qs, "limit", params.limit);
            if (params.asc !== undefined) setIf(qs, "asc", params.asc ? "true" : "false");
            setIf(qs, "aggregate", params.aggregate);
            setIf(qs, "type", params.type);
        }
        const { data } = await request.get<ListResponse<ProbeData>>(
            `/workspaces/${workspaceId}/probe-data/probes/${probeId}/data${qs.toString() ? `?${qs}` : ""}`
        );
        return data?.data || [];
    },

    /**
     * Latest datapoint by type + reporting agent (optional probeId).
     * GET /workspaces/{id}/probe-data/latest?type=...&agentId=...&probeId=...
     */
    async latest(
        workspaceId: number | string,
        params: { type: string; agentId: number | string; probeId?: number | string }
    ) {
        const qs = new URLSearchParams();
        setIf(qs, "type", params.type);
        setIf(qs, "agentId", params.agentId);
        setIf(qs, "probeId", params.probeId);
        const { data } = await request.get<ProbeData>(
            `/workspaces/${workspaceId}/probe-data/latest?${qs.toString()}`
        );
        return data; // 404 -> request throws; catch upstream if you want null
    },

    /**
     * Timeseries (or latestOnly) for all probes that hit a literal target.
     * GET /workspaces/{id}/probe-data/by-target/data
     */
    async byTargetData(
        workspaceId: number | string,
        params: {
            target: string;                     // required
            type?: string;                      // optional filter
            latestOnly?: boolean;               // default false
            from?: string | number | Date;
            to?: string | number | Date;
            limit?: number;
        }
    ) {
        const qs = new URLSearchParams();
        setIf(qs, "target", params.target);
        setIf(qs, "type", params.type);
        if (params.latestOnly !== undefined) setIf(qs, "latestOnly", params.latestOnly ? "true" : "false");
        setIf(qs, "from", toRFC3339(params.from));
        setIf(qs, "to", toRFC3339(params.to));
        setIf(qs, "limit", params.limit);

        type Bundle =
            | { probe_id: number; Latest?: ProbeData; Rows?: undefined }
            | { probe_id: number; Latest?: undefined; Rows?: ProbeData[] };

        const { data } = await request.get<{
            target: string;
            probeIds: number[];
            bundles: Bundle[];
        }>(`/workspaces/${workspaceId}/probe-data/by-target/data?${qs.toString()}`);

        return data;
    },

    /**
     * Discover similar probes (same literal target and/or same target agent).
     * GET /workspaces/{id}/probe-data/probes/{probeId}/similar
     */
    async similar(
        workspaceId: number | string,
        probeId: number | string,
        params?: { sameType?: boolean; includeSelf?: boolean; latest?: boolean }
    ) {
        const qs = new URLSearchParams();
        if (params?.sameType !== undefined) setIf(qs, "sameType", params.sameType ? "true" : "false");
        if (params?.includeSelf !== undefined) setIf(qs, "includeSelf", params.includeSelf ? "true" : "false");
        if (params?.latest !== undefined) setIf(qs, "latest", params.latest ? "true" : "false");

        const { data } = await request.get<{
            probe: Probe;
            similar_by_target: Probe[];
            similar_by_agent_id: Probe[];
            latest?: { probe_id: number; latest: ProbeData | null }[];
        }>(
            `/workspaces/${workspaceId}/probe-data/probes/${probeId}/similar${qs.toString() ? `?${qs}` : ""}`
        );
        return data;
    },

    /** Convenience wrappers */
    async latestNetInfo(workspaceId: number | string, agentId: number | string, probeId?: number | string) {
        return this.latest(workspaceId, { type: "NETINFO", agentId, probeId });
    },
    async latestSysInfo(workspaceId: number | string, agentId: number | string, probeId?: number | string) {
        return this.latest(workspaceId, { type: "SYSINFO", agentId, probeId });
    },
    /**
     * Speedtest results for an agent (queries by agent_id + type, NOT probe_id).
     * This works around historical data having incorrect probe_id values.
     * GET /workspaces/{id}/probe-data/agents/{agentID}/speedtests
     */
    async speedtestsByAgent(workspaceId: number | string, agentId: number | string, limit?: number) {
        const qs = new URLSearchParams();
        if (limit) qs.set("limit", String(limit));
        const { data } = await request.get<ListResponse<ProbeData>>(
            `/workspaces/${workspaceId}/probe-data/agents/${agentId}/speedtests${qs.toString() ? `?${qs}` : ""}`
        );
        return data?.data || [];
    },
};

/** ===== Probes (scoped to workspace + agent) ===== */
export const ProbeService = {
    async list(workspaceId: number | string, agentId: number | string) {
        const { data } = await request.get<ListResponse<Probe>>(`/workspaces/${workspaceId}/agents/${agentId}/probes`);
        return data.data;
    },
    async create(workspaceId: number | string, agentId: number | string, body: Partial<ProbeCreateInput>) {
        const { data } = await request.post<Probe>(`/workspaces/${workspaceId}/agents/${agentId}/probes`, body);
        return data;
    },
    async get(workspaceId: number | string, agentId: number | string, probeId: number | string) {
        const { data } = await request.get<Probe>(`/workspaces/${workspaceId}/agents/${agentId}/probes/${probeId}`);
        return data;
    },
    async update(
        workspaceId: number | string,
        agentId: number | string,
        probeId: number | string,
        body: Partial<Probe> & { replaceTargets?: string[]; replaceAgentTargets?: number[] }
    ) {
        const { data } = await request.patch<Probe>(
            `/workspaces/${workspaceId}/agents/${agentId}/probes/${probeId}`,
            body
        );
        return data;
    },
    async remove(workspaceId: number | string, agentId: number | string, probeId: number | string) {
        const { data } = await request.delete<{ ok: boolean }>(
            `/workspaces/${workspaceId}/agents/${agentId}/probes/${probeId}`
        );
        return data;
    },
    // network information getter
    async netInfo(workspaceId: number | string, agentId: number | string) {
        const { data } = await request.get<ProbeData>(`/workspaces/${workspaceId}/agents/${agentId}/netinfo`);
        return data;
    },

    async sysInfo(workspaceId: number | string, agentId: number | string) {
        const { data } = await request.get<ProbeData>(`/workspaces/${workspaceId}/agents/${agentId}/sysinfo`);
        return data;
    },
};

/** ===== Public agent bootstrap/auth (no JWT) ===== */
export const AgentBootstrap = {
    // PSK or PIN flow for the agent binary
    async authenticate(body:
        | { workspaceId: number; agentId: number; psk: string }
        | { workspaceId: number; agentId: number; pin: string }
    ) {
        // Use a raw request without JWT header; request.ts will include JWT if present,
        // but the /agent route ignores it. If you need it truly header-free, create a raw axios call.
        const { data } = await request.post(`/agent`, body);
        return data as
            | { status: "ok"; agent: Agent }
            | { status: "bootstrapped"; psk: string; agent: Agent }
            | { error: string };
    },
};

/** ===== Speedtest Queue (scoped to workspace + agent) ===== */

export interface SpeedtestQueueItem {
    id: number;
    workspace_id: number;
    agent_id: number;
    server_id: string;
    server_name: string;
    status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled' | 'expired';
    requested_by: number;
    requested_at: string;
    expires_at: string;
    started_at?: string;
    completed_at?: string;
    error?: string;
    created_at: string;
    updated_at: string;
}

export interface SpeedtestServer {
    id: number;
    agent_id: number;
    server_id: string;
    name: string;
    sponsor: string;
    host: string;
    url: string;
    country: string;
    lat: string;
    lon: string;
    distance: number;
    last_seen_at: string;
}

export const SpeedtestService = {
    // ---- Queue Management ----
    async listQueue(workspaceId: number | string, agentId: number | string, status?: string) {
        const qs = new URLSearchParams();
        if (status) qs.set("status", status);
        const { data } = await request.get<{ data: SpeedtestQueueItem[]; total: number }>(
            `/workspaces/${workspaceId}/agents/${agentId}/speedtest-queue${qs.toString() ? `?${qs}` : ""}`
        );
        return data;
    },

    async queueTest(
        workspaceId: number | string,
        agentId: number | string,
        body: { server_id?: string; server_name?: string }
    ) {
        const { data } = await request.post<SpeedtestQueueItem>(
            `/workspaces/${workspaceId}/agents/${agentId}/speedtest-queue`,
            body
        );
        return data;
    },

    async getQueueItem(workspaceId: number | string, agentId: number | string, queueId: number | string) {
        const { data } = await request.get<SpeedtestQueueItem>(
            `/workspaces/${workspaceId}/agents/${agentId}/speedtest-queue/${queueId}`
        );
        return data;
    },

    async cancelQueueItem(workspaceId: number | string, agentId: number | string, queueId: number | string) {
        const { data } = await request.delete<{ ok: boolean }>(
            `/workspaces/${workspaceId}/agents/${agentId}/speedtest-queue/${queueId}`
        );
        return data;
    },

    // ---- Cached Servers ----
    async listServers(workspaceId: number | string, agentId: number | string) {
        const { data } = await request.get<{ data: SpeedtestServer[]; total: number }>(
            `/workspaces/${workspaceId}/agents/${agentId}/speedtest-servers`
        );
        return data;
    },
};

/** ===== Alerts ===== */
export interface Alert {
    id: number;
    created_at: string;
    updated_at: string;
    alert_rule_id: number;
    workspace_id: number;
    probe_id?: number;
    agent_id?: number;
    metric: string;
    value: number;
    threshold: number;
    severity: 'warning' | 'critical';
    status: 'active' | 'acknowledged' | 'resolved';
    message?: string;
    triggered_at: string;
    resolved_at?: string;
    acknowledged_at?: string;
    acknowledged_by?: number;
}

export interface AlertRule {
    id: number;
    created_at: string;
    updated_at: string;
    workspace_id: number;
    probe_id?: number;
    agent_id?: number;
    name: string;
    description?: string;
    metric: 'packet_loss' | 'latency' | 'jitter' | 'offline';
    operator: 'gt' | 'lt' | 'gte' | 'lte' | 'eq';
    threshold: number;
    severity: 'warning' | 'critical';
    enabled: boolean;
    notify_panel: boolean;
    notify_email: boolean;
    notify_webhook: boolean;
    webhook_url?: string;
    webhook_secret?: string;
}

export interface AlertRuleInput {
    name: string;
    description?: string;
    probe_id?: number;
    agent_id?: number;
    metric: 'packet_loss' | 'latency' | 'jitter' | 'offline';
    operator: 'gt' | 'lt' | 'gte' | 'lte' | 'eq';
    threshold: number;
    severity?: 'warning' | 'critical';
    enabled?: boolean;
    notify_panel?: boolean;
    notify_email?: boolean;
    notify_webhook?: boolean;
    webhook_url?: string;
    webhook_secret?: string;
}

export const AlertService = {
    async list(params?: { status?: string; limit?: number }) {
        const qs = new URLSearchParams();
        if (params?.status) qs.set("status", params.status);
        if (params?.limit) qs.set("limit", String(params.limit));
        const { data } = await request.get<ListResponse<Alert>>(
            `/alerts${qs.toString() ? `?${qs}` : ""}`
        );
        return data.data || [];
    },

    async getCount() {
        const { data } = await request.get<{ count: number }>("/alerts/count");
        return data.count;
    },

    async get(id: number | string) {
        const { data } = await request.get<Alert>(`/alerts/${id}`);
        return data;
    },

    async acknowledge(id: number | string) {
        const { data } = await request.patch<{ ok: boolean }>(`/alerts/${id}/acknowledge`);
        return data;
    },

    async resolve(id: number | string) {
        const { data } = await request.patch<{ ok: boolean }>(`/alerts/${id}/resolve`);
        return data;
    },
};

export const AlertRuleService = {
    async list(workspaceId: number | string) {
        const { data } = await request.get<ListResponse<AlertRule>>(
            `/workspaces/${workspaceId}/alert-rules`
        );
        return data.data || [];
    },

    async get(workspaceId: number | string, ruleId: number | string) {
        const { data } = await request.get<AlertRule>(
            `/workspaces/${workspaceId}/alert-rules/${ruleId}`
        );
        return data;
    },

    async create(workspaceId: number | string, body: AlertRuleInput) {
        const { data } = await request.post<AlertRule>(
            `/workspaces/${workspaceId}/alert-rules`,
            body
        );
        return data;
    },

    async update(workspaceId: number | string, ruleId: number | string, body: Partial<AlertRuleInput>) {
        const { data } = await request.patch<AlertRule>(
            `/workspaces/${workspaceId}/alert-rules/${ruleId}`,
            body
        );
        return data;
    },

    async remove(workspaceId: number | string, ruleId: number | string) {
        const { data } = await request.delete<{ ok: boolean }>(
            `/workspaces/${workspaceId}/alert-rules/${ruleId}`
        );
        return data;
    },
};