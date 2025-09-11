import request from "./request";
import type {Agent, Probe, ProbeCreateInput, ProbeData, Workspace, Member, Role} from "@/types";

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

    async create(body: { name: string; description?: string;}) {
        const { data } = await request.post<Workspace>('/workspaces', body);
        return data;
    },

    async get(id: number | string) {
        const { data } = await request.get<Workspace>(`/workspaces/${id}`);
        return data;
    },

    // PATCH expects { displayName?, settings? }
    async update(
        id: number | string,
        body: { displayName?: string; settings?: Record<string, any> }
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
        const { data } = await request.get<Member[]>(`/workspaces/${workspaceId}/members`);
        return data;
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
        return data as { pin: string; expiresAt?: string; [k: string]: any };
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
        const { data } = await request.get<ProbeData[]>(
            `/workspaces/${workspaceId}/probe-data/find${qs.toString() ? `?${qs}` : ""}`
        );
        return data;
    },

    /**
     * Timeseries for a specific probe.
     * GET /workspaces/{id}/probe-data/probes/{probeID}/data
     */
    async byProbe(
        workspaceId: number | string,
        probeId: number | string,
        params?: { from?: string | number | Date; to?: string | number | Date; limit?: number; asc?: boolean }
    ) {
        const qs = new URLSearchParams();
        if (params) {
            setIf(qs, "from", toRFC3339(params.from));
            setIf(qs, "to", toRFC3339(params.to));
            setIf(qs, "limit", params.limit);
            if (params.asc !== undefined) setIf(qs, "asc", params.asc ? "true" : "false");
        }
        const { data } = await request.get<ProbeData[]>(
            `/workspaces/${workspaceId}/probe-data/probes/${probeId}/data${qs.toString() ? `?${qs}` : ""}`
        );
        return data;
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
};

/** ===== Probes (scoped to workspace + agent) ===== */
export const ProbeService = {
    async list(workspaceId: number | string, agentId: number | string) {
        const { data } = await request.get<Probe[]>(`/workspaces/${workspaceId}/agents/${agentId}/probes`);
        return data;
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