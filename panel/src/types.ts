export interface User {
    id: string; // You can use string for ObjectID in TypeScript
    email: string;
    firstName: string;
    lastName: string;
    company: string;
    admin: boolean;
    password: string;
    verified: boolean;
    phoneNumber: string;
    role: string;
    createdAt: Date;
    updatedAt: Date;
}

export interface Site {
    id: string; // You can use string for ObjectID in TypeScript
    name: string;
    description: string;
    location: string;
    members: SiteMember[];
    createdAt: Date;
    updatedAt: Date;
}

export interface SelectOption {
    value: string
    text: string
    disabled: boolean
}

export type SiteMemberRole = "READ_ONLY" | "READ_WRITE" | "ADMIN" | "OWNER";

export interface MemberInfo {
    email: string;
    firstName: string;
    lastName: string;
    role: SiteMemberRole;
    id: string; // MongoDB ObjectID is typically represented as a string in TypeScript
}


export interface SiteMember {
    user: string; // You can use string for ObjectID in TypeScript
    role: SiteMemberRole;
    // roles: "READ_ONLY" | "READ_WRITE" | "ADMIN" | "OWNER";
    // ADMINS can regenerate agent pins
}

export interface SiteMemberUser {
    user: string; // You can use string for ObjectID in TypeScript
    role: SiteMemberRole;
    email: string;
    name: string;
}

export interface Agent {
    id: string; // You can use string for ObjectID in TypeScript
    name: string;
    site: string; // You can use string for ObjectID in TypeScript
    pin: string;
    initialized: boolean;
    location: string; // Assuming location is a numeric value
    createdAt: Date;
    updatedAt: Date;
    public_ip_override: String;
    version: String;
}

export interface Probe {
    type: ProbeType;
    id: string; // You can use string for ObjectID in TypeScript
    agent: string; // You can use string for ObjectID in TypeScript
    pending: Date; // Assuming 'pending' is a timestamp represented as a Date
    createdAt: Date;
    updatedAt: Date;
    notifications: boolean;
    config: ProbeConfig;
}

export interface ProbeTarget {
    target: string;
    agent: string;
    group: string;
}

export // ProbeConfig
interface ProbeConfig {
    target: ProbeTarget[];
    duration: number;
    count: number;
    interval: number;
    server: boolean;
}

export interface OUIEntry {
    Registry: string;
    Assignment: string;
    "Organization Name": string;
    "Organization Address": string;
}

export interface AgentGroup {
    id: string;
    site: string;
    agents: string[]; // these are the IDs of agents in the group
    name: string;
    description: string;
}

// ProbeType
export type ProbeType = "RPERF" | "MTR" | "PING" | "SPEEDTEST" | "NETINFO" | "TRAFFICSIM" | "SPEEDTEST_SERVERS";

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
    latency?: number; // TypeScript doesn't have a built-in Duration type, so we use number
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
    ping?: number; // Using number instead of Duration
    download?: number;
    upload?: number;
    total?: number;
}

export interface SpeedTestPLoss {
    sent: number;
    dup: number;
    max: number;
}

// ProbeData
export interface ProbeData {
    id: string; // You can use string for ObjectID in TypeScript
    probe: string; // You can use string for ObjectID in TypeScript
    triggered: boolean;
    createdAt: Date;
    updatedAt: Date;
    target: ProbeTarget
    data?: any; // Use an appropriate type for data if possible, otherwise 'any'
}


export interface Preferences {
    dark: boolean,
    token: string
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


export interface NetResult {
    localAddress: string;
    defaultGateway: string;
    publicAddress: string;
    internetProvider: string;
    lat: string;
    long: string;
    timestamp: Date;
}

export interface TrafficSimResult {
    averageRTT: number;
    duplicatePackets: number;
    lostPackets: number;
    maxRTT: number;
    minRTT: number;
    outOfSequence: number;
    stdDevRTT: number;
    totalPackets: number;
    reportTime: Date;
}

export interface RPerfResults {
    startTimestamp: Date;
    stopTimestamp: Date;
    config: {
        additional: {
            ipVersion: number;
            omitSeconds: number;
            reverse: boolean;
        };
        common: {
            family: string;
            length: number;
            streams: number;
        };
        download: {};
        upload: {
            bandwidth: number;
            duration: number;
            sendInterval: number;
        };
    };
    streams: Array<{
        abandoned: boolean;
        failed: boolean;
        intervals: {
            receive: Array<{
                bytesReceived: number;
                duration: number;
                jitterSeconds: number;
                packetsDuplicated: number;
                packetsLost: number;
                packetsOutOfOrder: number;
                packetsReceived: number;
                timestamp: number;
                unbrokenSequence: number;
            }>;
            send: Array<{
                bytesSent: number;
                duration: number;
                packetsSent: number;
                sendsBlocked: number;
                timestamp: number;
            }>;
            summary: {
                bytesReceived: number;
                bytesSent: number;
                durationReceive: number;
                durationSend: number;
                framedPacketSize: number;
                jitterAverage: number;
                jitterPacketsConsecutive: number;
                packetsDuplicated: number;
                packetsLost: number;
                packetsOutOfOrder: number;
                packetsReceived: number;
                packetsSent: number;
            };
        };
    }>;
    success: boolean;
    summary: {
        bytesReceived: number;
        bytesSent: number;
        durationReceive: number;
        durationSend: number;
        framedPacketSize: number;
        jitterAverage: number;
        jitterPacketsConsecutive: number;
        packetsDuplicated: number;
        packetsLost: number;
        packetsOutOfOrder: number;
        packetsReceived: number;
        packetsSent: number;
    };
}

export interface PingResult {
    startTimestamp: Date;
    stopTimestamp: Date;
    packetsRecv: number;
    packetsSent: number;
    packetsRecvDuplicates: number;
    packetLoss: number;
    addr: string;
    minRtt: number; // Same as latency in SpeedTestResult
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
    user: number; // Assuming milliseconds or choose an appropriate unit
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
    metrics?: Record<string, number>; // Other memory related metrics
}

export interface MeanOpinionScore {
    mosValue: number;
    jitter: number;
    latency: number;
    packetLoss: number;
}

function calculateMOS(jitter: number, latency: number, packetLoss: number): MeanOpinionScore {
    // Placeholder for MOS calculation logic
    // The actual calculation would depend on the specific formula you want to use
    let mosValue = 5 /*- (jitter * factor1 + latency * factor2 + packetLoss * factor3)*/;
    mosValue = Math.max(1, Math.min(mosValue, 5)); // MOS is typically between 1 and 5

    return {
        mosValue,
        jitter,
        latency,
        packetLoss
    };
}



export {}
