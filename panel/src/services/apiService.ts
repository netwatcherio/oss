import request from "./request";

/** ===== Types kept minimal & flexible ===== */
export interface Workspace {
    id: number;
    name: string;
    description?: string;
    settings?: Record<string, any>;
    createdAt?: string;
    updatedAt?: string;
}

export interface Agent {
    id: number;
    workspaceId: number;
    name: string;
    description?: string;
    location?: string;
    public_ip_override?: string;
    version?: string;
    labels?: Record<string, any>;
    metadata?: Record<string, any>;
    createdAt?: string;
    updatedAt?: string;
}

export type ProbeType = "PING" | "MTR" | "RPERF" | "TRAFFICSIM" | string;

export interface Probe {
    id: number;
    workspaceId: number;
    agentId: number;
    type: ProbeType;
    enabled?: boolean;
    intervalSec?: number;
    timeoutSec?: number;
    labels?: Record<string, any>;
    metadata?: Record<string, any>;
    targets?: string[];
    agentTargets?: number[];
    createdAt?: string;
    updatedAt?: string;
}

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

/** ===== Workspaces ===== */
export const WorkspaceService = {
    async list(params?: { q?: string; limit?: number; offset?: number }) {
        const qs = new URLSearchParams();
        if (params?.q) qs.set("q", params.q);
        if (params?.limit) qs.set("limit", String(params.limit));
        if (params?.offset) qs.set("offset", String(params.offset));
        const { data } = await request.get<Workspace[]>(`/workspaces${qs.toString() ? `?${qs}` : ""}`);
        return data;
    },
    async create(body: { name: string; displayName?: string; settings?: Record<string, any> }) {
        const { data } = await request.post<Workspace>("/workspaces", body);
        return data;
    },
    async get(id: number | string) {
        const { data } = await request.get<Workspace>(`/workspaces/${id}`);
        return data;
    },
    async update(id: number | string, body: Partial<Pick<Workspace, "name" | "description">>) {
        const { data } = await request.patch<Workspace>(`/workspaces/${id}`, body);
        return data;
    },
    async remove(id: number | string) {
        const { data } = await request.delete<{ ok: boolean }>(`/workspaces/${id}`);
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

/** ===== Probes (scoped to workspace + agent) ===== */
export const ProbeService = {
    async list(workspaceId: number | string, agentId: number | string) {
        const { data } = await request.get<Probe[]>(`/workspaces/${workspaceId}/agents/${agentId}/probes`);
        return data;
    },
    async create(workspaceId: number | string, agentId: number | string, body: Partial<Probe>) {
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