// Basic JSON type used for labels/metadata
export type JSONValue =
    | string
    | number
    | boolean
    | null
    | JSONValue[]
    | { [key: string]: JSONValue };

/** Generic JSON blob equivalent to GORM datatypes.JSON */
export type JsonObject = Record<string, unknown>;

/** Probe type placeholder (tighten to string literals when you finalize the set) */
export type ProbeType = string;

/** ===== Agents ===== */

export interface AgentCreateInput {
    workspaceId: number;
    name: string;
    description: string;
    /** default 9 */
    pinLength: number;
    location: string;
    publicIPOverride: string;
    version: string;
    labels: JsonObject;
    metadata: JsonObject;
    /** optional bootstrap PIN TTL (milliseconds recommended) */
    pinTtlMs?: number;
}

export interface Agent {
    id: number;
    created_at: string;   // ISO 8601
    updated_at: string;   // ISO 8601

    // Ownership / scoping
    workspace_id: number;

    // Identity
    name: string;
    description: string;

    // Network
    location: string;
    public_ip_override: string;

    // Runtime / versioning
    version: string;

    // Health
    last_seen_at: string;  // ISO 8601

    // Tags / labels
    labels: JsonObject;
    metadata: JsonObject;

    // Authentication: PSKHash is omitted (json:"-")
    initialized: boolean;
}

/** ===== Probes ===== */

export interface Probe {
    id: number;
    created_at: string;     // ISO 8601
    updated_at: string;     // ISO 8601

    workspace_id: number;
    agent_id: number;
    type: ProbeType;
    enabled: boolean;
    interval_sec: number;
    timeout_sec: number;
    count: number;
    duration_sec: number;
    server: boolean;
    labels: JsonObject;
    metadata: JsonObject;

    targets: Target[];
}

export interface Target {
    id: number;
    createdAt: string;   // ISO 8601
    updatedAt: string;   // ISO 8601

    probeId: number;

    /** ip/host[:port] (leave empty when agentId is set) */
    target: string;

    /** target agent (nullable in DB; may be null or omitted) */
    agentId?: number | null;

    /** optional grouping/batching (nullable) */
    groupId?: number | null;
}

export interface ProbeCreateInput {
    workspace_id: number;
    agent_id: number;
    type: ProbeType;
    enabled?: boolean;
    interval_sec: number;
    timeout_sec: number;
    count: number
    duration_sec: number
    labels: JsonObject;
    metadata: JsonObject;
    server?: boolean

    /** One of:
     *  - targets: literal endpoints
     *  - agentTargets: agent IDs (controller resolves runtime address)
     */
    targets?: string[];
    agent_targets?: number[];
}

export interface ProbeUpdateInput {
    id: number;
    enabled?: boolean;
    intervalSec?: number;
    timeoutSec?: number;
    labels?: JsonObject;
    metadata?: JsonObject;

    /** Optional full replacement of targets in one shot */
    replaceTargets?: string[];
    replaceAgentTargets?: number[];
}

/** ===== Users ===== */

export interface User {
    id: number;
    email: string;
    // passwordHash omitted (json:"-")
    name: string;
    role: string;            // "USER" by default on backend
    verified: boolean;
    labels: JsonObject;
    metadata: JsonObject;
    lastLoginAt?: string | null; // ISO 8601 or null
    createdAt: string;           // ISO 8601
    updatedAt: string;           // ISO 8601
}

/** ===== Workspaces & Members ===== */

export interface Workspace {
    id: number;
    name: string;
    ownerId: number;
    settings: JsonObject;
    createdAt: string;   // ISO 8601
    updatedAt: string;   // ISO 8601
    // DeletedAt omitted (json:"-")

    /** denormalized convenience */
    description: string;
}

export type Role = 'USER' | 'ADMIN' | 'OWNER';

export interface Member {
    id: number;
    workspace_id: number;
    /** 0 means invited by email only */
    user_id: number;
    email: string;
    role: Role;
    meta: JsonObject;

    created_at: string;   // ISO 8601
    updated_at: string;   // ISO 8601
    // DeletedAt omitted (json:"-")

    invited_at?: string | null;   // ISO 8601 or null
    accepted_at?: string | null;  // ISO 8601 or null
    revoked_at?: string | null;   // ISO 8601 or null
}



export interface SelectOption {
    value: string;
    text: string;
    disabled: boolean;
    agentAvailable: boolean;
}

export interface ProbeData {
    id: number;
    probe_id: number;
    probe_agent_id: number; // probe ID owner - used for reverse probes
    agent_id: number;
    triggered: boolean;
    triggered_reason: string;
    created_at: string;   // ISO 8601 timestamp (agent-provided)
    received_at: string;  // ISO 8601 timestamp (backend-provided)
    type: string;         // maps to probe.Type enum on backend
    payload: any;         // raw JSON payload, may vary by probe type
    target?: string;      // optional
    targetAgent?: number; // optional
}

// ProbeData (runtime telemetry payloads)

export interface ProbeTargetDTO {
    target: string;
    agent?: number | null;
    group?: number | null;
}

export interface ProbeConfig {
    target: ProbeTargetDTO[];
    duration: number;
    count: number;
    interval: number;
    server: boolean;
}

// Speedtest / MTR / Ping results (unchanged)

export interface SpeedTestResult {
    test_data: SpeedTestServer[];
    timestamp: Date;
}

export interface SpeedTestServer {
    url?: string;
    lat?: string;
    lon?: string;
    name?: string;
    country?: string;
    sponsor?: string;
    id?: string;
    host?: string;
    distance?: number;
    latency?: number;
    max_latency?: number;
    min_latency?: number;
    jitter?: number;
    dl_speed?: SpeedTestByteRate;
    ul_speed?: SpeedTestByteRate;
    test_duration?: SpeedTestTestDuration;
    packet_loss?: SpeedTestPLoss;
}

export type SpeedTestByteRate = number;

export interface SpeedTestTestDuration {
    ping?: number;
    download?: number;
    upload?: number;
    total?: number;
}

export interface SpeedTestPLoss {
    sent: number;
    dup: number;
    max: number;
}

export interface MtrResult {
    start_timestamp: Date;
    stop_timestamp: Date;
    report: {
        info: {
            target: {
                ip: string;
                hostname: string;
            };
        };
        hops: MtrHop[];
    };
}

export interface MtrHop {
    ttl: number;
    hosts: {
        ip: string;
        hostname: string;
    }[];
    extensions: string[];
    loss_pct: string;
    sent: number;
    last: string;
    recv: number;
    avg: string;
    best: string;
    worst: string;
    stddev: string;
}

export interface PingResult {
    start_timestamp: Date;
    stop_timestamp: Date;
    packets_recv: number;
    packets_sent: number;
    packets_recv_duplicates: number;
    packet_loss: number;
    addr: string;
    min_rtt: number;
    max_rtt: number;
    avg_rtt: number;
    std_dev_rtt: number;
}

export interface ProbeDataRequest {
    limit: number;
    startTimestamp: Date;
    endTimestamp: Date;
    recent: boolean;
}

export interface NetInfoPayload {
    local_address: string;
    default_gateway: string;
    public_address: string;
    internet_provider: string;
    lat: string;
    long: string;
    timestamp: string; // ISO 8601 timestamp
}

export interface OUIEntry {
    Registry: string;
    Assignment: string;
    "Organization Name": string;
    "Organization Address": string;
}

export interface SysInfoPayload {
    hostInfo: HostInfo;
    memoryInfo: HostMemoryInfo;
    CPUTimes: CPUTimes;
    timestamp: Date;
}

export interface CPUTimes {
    user: number;
    system: number;
    idle?: number;
    iowait?: number;
    irq?: number;
    nice?: number;
    softIRQ?: number;
    steal?: number;
}

export interface HostInfo {
    architecture: string;
    boot_time: Date;
    containerized?: boolean | null;
    name: string;
    ip?: string[];
    kernel_version: string;
    mac: string[];
    os: OSInfo;
    timezone: string;
    timezone_offset_sec: number;
    uniqueID?: string;
}

export interface OSInfo {
    type: string;
    family: string;
    platform: string;
    name: string;
    version: string;
    major: number;
    minor: number;
    patch: number;
    build?: string;
    codename?: string;
}

export interface HostMemoryInfo {
    total_bytes: number;
    used_bytes: number;
    available_bytes: number;
    free_Bytes: number;
    virtual_total_bytes: number;
    virtual_used_bytes: number;
    virtual_free_bytes: number;
    raw?: Record<string, number>;
}

export interface MeanOpinionScore {
    mosValue: number;
    jitter: number;
    latency: number;
    packetLoss: number;
}

function calculateMOS(
    jitter: number,
    latency: number,
    packetLoss: number
): MeanOpinionScore {
    let mosValue = 5;
    mosValue = Math.max(1, Math.min(mosValue, 5));
    return { mosValue, jitter, latency, packetLoss };
}

export interface Preferences {
    dark: boolean;
}