// Basic JSON type used for labels/metadata
export type JSONValue =
    | string
    | number
    | boolean
    | null
    | JSONValue[]
    | { [key: string]: JSONValue };

// ---- Users ----

export interface User {
    id: number;
    email: string;
    firstName: string;
    lastName: string;
    company: string;
    phoneNumber: string;

    // Access & security
    admin: boolean;
    role: string;
    password: string; // (hash on backend; usually not sent to client)
    verified: boolean;
    mfaEnabled: boolean;

    // State & telemetry
    status?: string; // e.g. "ACTIVE"
    lastLoginAt?: string; // ISO string
    timezone?: string;
    avatarUrl?: string;

    // Free-form data
    labels?: Record<string, JSONValue>;
    metadata?: Record<string, JSONValue>;

    createdAt: string; // ISO
    updatedAt: string; // ISO
}

// ---- Workspaces ----

export type WorkspaceMemberRole = "READ_ONLY" | "READ_WRITE" | "ADMIN" | "OWNER";

export interface WorkspaceMember {
    id: number;
    createdAt: string;
    updatedAt: string;

    workspaceId: number;
    userId: number; // 0 if invite-only (not yet linked)
    email: string;

    role: WorkspaceMemberRole;

    invitedAt?: string | null;
    acceptedAt?: string | null;
    revokedAt?: string | null;

    displayName?: string;
}

export interface Workspace {
    id: number;
    createdAt: string;
    updatedAt: string;

    name: string;
    slug: string;
    description: string;
    location: string;

    ownerUserId: number;

    labels: Record<string, JSONValue>;
    metadata: Record<string, JSONValue>;

    // optional association
    members?: WorkspaceMember[] | null;
}

// Helpful DTOs for member operations
export interface MemberInviteRequest {
    // One of (userId | email) is required
    userId?: number;
    email?: string;
    role: WorkspaceMemberRole;
}
export interface MemberRoleUpdateRequest {
    role: WorkspaceMemberRole;
}

// ---- Agents ----

export interface Agent {
    id: number;
    createdAt: string;
    updatedAt: string;

    // Ownership / scoping
    workspaceId: number;
    siteId: number; // kept for legacy compatibility if still present in payloads

    // Identity
    name: string;
    hostname: string;
    initialized: boolean;

    // Auth
    pin: string;
    publicKey: string;

    // Network
    location: string;
    public_ip_override: string; // preserving original field if still used on UI
    detectedPublicIp: string;
    privateIp: string;
    macAddress: string;

    // Runtime / versioning
    version: string;
    platform: string; // linux, darwin, windows
    arch: string; // amd64, arm64, etc.

    // Health / status
    status?: string; // e.g. "ACTIVE"
    lastSeenAt?: string; // ISO
    heartbeatIntervalSec: number;

    // Tags / labels
    labels?: Record<string, JSONValue>;
    metadata?: Record<string, JSONValue>;
}

// ---- Groups (optional, if you use them) ----

export interface AgentGroup {
    id: number;
    workspaceId: number;
    agents: number[]; // agent IDs
    name: string;
    description?: string;
}

// ---- Probes ----

export type ProbeType =
    | "RPERF"
    | "MTR"
    | "PING"
    | "SPEEDTEST"
    | "NETINFO"
    | "TRAFFICSIM"
    | "SPEEDTEST_SERVERS";

export interface Target {
    id: number;
    createdAt: string;
    updatedAt: string;

    probeId: number;
    target: string; // IP/host[:port]
    agentId?: number | null;
    groupId?: number | null;
}

export interface Probe {
    id: number;
    createdAt: string;
    updatedAt: string;

    workspaceId?: number; // optional/handy
    agentId: number;
    type: ProbeType;

    // Flags & knobs
    notifications: boolean;
    durationSec: number;
    count: number;
    intervalSec: number;
    server: boolean;
    pendingAt?: string | null;

    // Reverse/meta
    reverseOfProbeId?: number | null;
    originalAgentId?: number | null;

    // Free-form extras
    labels?: Record<string, JSONValue>;
    metadata?: Record<string, JSONValue>;

    targets: Target[];
}

// ---- Sessions ----

export interface Session {
    itemId: number; // user or agent PK (matches Go: column:item_id)
    isAgent: boolean;
    sessionId: number;
    expiry: string; // ISO
    created: string; // ISO
    wsConn?: string;
    ip?: string;
}

// ---- Legacy / existing shapes you already had ----

export interface SelectOption {
    value: string;
    text: string;
    disabled: boolean;
}

// Kept for components that expect this shape when inviting via email
export interface MemberInfo {
    email: string;
    firstName?: string;
    lastName?: string;
    role: WorkspaceMemberRole;
    id?: number; // if you use numeric IDs now
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

export interface ProbeData {
    id: string;
    probe: string;
    triggered: boolean;
    createdAt: Date;
    updatedAt: Date;
    target: ProbeTargetDTO;
    data?: any;
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
    startTimestamp: Date;
    stopTimestamp: Date;
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
    startTimestamp: Date;
    stopTimestamp: Date;
    packetsRecv: number;
    packetsSent: number;
    packetsRecvDuplicates: number;
    packetLoss: number;
    addr: string;
    minRtt: number;
    maxRtt: number;
    avgRtt: number;
    stdDevRtt: number;
}

export interface ProbeDataRequest {
    limit: number;
    startTimestamp: Date;
    endTimestamp: Date;
    recent: boolean;
}

export interface CompleteSystemInfo {
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
    bootTime: Date;
    containerized?: boolean | null;
    hostname: string;
    IPs?: string[];
    kernelVersion: string;
    MACs: string[];
    os: OSInfo;
    timezone: string;
    timezoneOffsetSec: number;
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
    totalBytes: number;
    usedBytes: number;
    availableBytes: number;
    freeBytes: number;
    virtualTotalBytes: number;
    virtualUsedBytes: number;
    virtualFreeBytes: number;
    metrics?: Record<string, number>;
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
    token: string;
}