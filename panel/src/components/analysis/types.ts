// panel/src/components/analysis/types.ts
// TypeScript interfaces for the AI Analysis health vectorization system

export interface HealthVector {
    latency_score: number     // 0-100
    packet_loss_score: number // 0-100
    route_stability: number   // 0-100
    mos_score: number         // 1.0-4.5
    overall_health: number    // 0-100
    grade: 'excellent' | 'good' | 'fair' | 'poor' | 'critical' | 'unknown'
}

export interface ProbeMetrics {
    avg_latency: number
    median_latency?: number
    p95_latency: number
    p99_latency?: number
    packet_loss: number
    jitter_avg: number
    jitter_median?: number
    jitter_p95?: number
    sample_count: number
}

export interface AnalysisSignal {
    type: string       // icmp_artifact, route_change, high_loss, high_latency, jitter_anomaly, icmp_latency_incomplete
    severity: string   // info, warning, critical
    title: string
    evidence: string
    confidence: number // 0-1.0
    hop_number?: number
}

export interface AnalysisFinding {
    id: string
    title: string
    severity: string   // info, warning, critical
    category: string   // performance, routing, measurement_artifact
    summary: string
    evidence?: string[]
    recommended_steps?: string[]
}

export interface MtrPathAnalysis {
    hop_count: number
    unique_routes: number
    route_stability_pct: number
    avg_end_hop_latency: number
    avg_end_hop_loss: number
    avg_end_hop_jitter?: number
    rate_limited_hops?: number[]
    timeout_segments?: string[]
    trace_count?: number
    latest_hops_detail?: HopDetail[]
}

export interface ProbeAnalysis {
    probe_id: number
    probe_type: string
    target: string
    agent_id: number
    agent_name: string
    health: HealthVector
    metrics: ProbeMetrics
    path_analysis?: MtrPathAnalysis
    reverse?: ProbeAnalysis
    // Worse-direction-weighted merge of forward + reverse health (bidirectional only)
    combined_health?: HealthVector
    signals: AnalysisSignal[]
    findings: AnalysisFinding[]
    generated_at: string
}

export interface ProbeHealthEntry {
    probe_id: number
    target: string
    probe_type: string
    health: HealthVector
    metrics: ProbeMetrics
}

export interface AgentHealthSummary {
    agent_id: number
    agent_name: string
    is_online: boolean
    health: HealthVector
    probe_count: number
    worst_probes: ProbeHealthEntry[]
}

export interface DetectedIncident {
    id: string
    title: string
    severity: string
    scope: string
    suggested_cause: string
    affected_agents: string[]
    affected_targets: string[]
    evidence: string[]
    recommendations: string[]
}

export interface StatusSummary {
    status: 'healthy' | 'degraded' | 'outage' | 'unknown'
    message: string
    active_issues: number
}

export interface WorkspaceAnalysis {
    workspace_id: number
    overall_health: HealthVector
    status: StatusSummary
    incidents: DetectedIncident[]
    agents: AgentHealthSummary[]
    total_probes: number
    total_agents: number
    generated_at: string
}

// ── Agent Health Mesh (chord diagram) ──
// Mirrors controller/internal/probe/analysis_mesh.go

export interface AgentMeshNode {
    agent_id: number
    agent_name: string
    location?: string
    is_online: boolean
    health: HealthVector
    link_count: number
}

export interface AgentMeshLink {
    source_agent_id: number
    source_agent_name: string
    target_agent_id: number
    target_agent_name: string
    health: HealthVector
    metrics: {
        avg_latency: number
        packet_loss: number
        jitter_avg: number
        sample_count: number
    }
    probe_types: string[]
}

export interface WorkspaceHealthMesh {
    workspace_id: number
    nodes: AgentMeshNode[]
    links: AgentMeshLink[]
    overall_health: HealthVector
    generated_at: string
}

// Status color mapping using Bootstrap CSS variables
export const statusColors: Record<string, { bg: string; text: string; icon: string }> = {
    healthy: { bg: 'rgba(var(--bs-success-rgb), 0.15)', text: 'var(--bs-success)', icon: 'bi-check-circle-fill' },
    degraded: { bg: 'rgba(var(--bs-warning-rgb), 0.15)', text: 'var(--bs-warning)', icon: 'bi-exclamation-triangle-fill' },
    outage: { bg: 'rgba(var(--bs-danger-rgb), 0.15)', text: 'var(--bs-danger)', icon: 'bi-x-octagon-fill' },
    unknown: { bg: 'rgba(var(--bs-secondary-rgb), 0.15)', text: 'var(--bs-secondary)', icon: 'bi-question-circle-fill' },
}

// Grade color mapping for consistent UI theming using Bootstrap variables
export const gradeColors: Record<string, { bg: string; text: string; border: string }> = {
    excellent: { bg: 'rgba(var(--bs-success-rgb), 0.15)', text: 'var(--bs-success)', border: 'var(--bs-success)' },
    good: { bg: 'rgba(var(--bs-primary-rgb), 0.15)', text: 'var(--bs-primary)', border: 'var(--bs-primary)' },
    fair: { bg: 'rgba(var(--bs-warning-rgb), 0.15)', text: 'var(--bs-warning)', border: 'var(--bs-warning)' },
    poor: { bg: 'rgba(var(--bs-warning-rgb), 0.25)', text: 'var(--bs-warning)', border: 'var(--bs-warning)' },
    critical: { bg: 'rgba(var(--bs-danger-rgb), 0.15)', text: 'var(--bs-danger)', border: 'var(--bs-danger)' },
    unknown: { bg: 'rgba(var(--bs-secondary-rgb), 0.15)', text: 'var(--bs-secondary)', border: 'var(--bs-secondary)' },
}

// Severity icon mapping
export const severityIcons: Record<string, string> = {
    info: 'bi-info-circle',
    warning: 'bi-exclamation-triangle',
    critical: 'bi-x-octagon',
}

// ── Route / Path Analysis Types ──

export interface HopDetail {
    ip: string
    hostname?: string
    is_agent: boolean
    agent_id?: number
    agent_name?: string
    is_final_hop: boolean
    latency?: number
    loss?: number
    is_rate_limited?: boolean
}

export interface HopMetric {
    ip: string
    loss: number
    latency: number
    hop_index: number
}

export interface ProbeRouteInfo {
    probe_id: number
    target: string
    baseline_fingerprint?: string
    baseline_hop_count?: number
    baseline_route_path?: string
    latest_signature?: string
    latest_hops?: string[]
    latest_hops_detail?: HopDetail[]
    has_route_change: boolean
    route_changed_at?: string
    trace_count?: number
    route_stability_pct?: number
    avg_end_hop_latency?: number
    avg_end_hop_loss?: number
    intermediate_hops?: HopMetric[]
}

export interface AgentRouteInfo {
    agent_id: number
    agent_name: string
    public_ip?: string
    isp?: string
    has_ip_change: boolean
    has_isp_change: boolean
    routes: ProbeRouteInfo[]
}

export interface SharedHopInfo {
    hop_ip: string
    hop_hostname?: string
    agent_ids: number[]
    agent_names: string[]
    hop_count: number
    has_issues?: boolean
    avg_loss?: number
    avg_latency?: number
}

export interface SharedDestinationInfo {
    target: string
    target_ip?: string
    agent_ids: number[]
    agent_names: string[]
    agent_count: number
    avg_end_latency_ms?: number
    avg_end_loss_pct?: number
    has_issues: boolean
}

export interface SharedAsnInfo {
    asn: number
    asn_org?: string
    hop_ips: string[]
    agent_ids: number[]
    agent_names: string[]
    agent_count: number
    has_issues: boolean
    avg_latency_ms?: number
    avg_loss_pct?: number
}

export interface CommonTargetInfo {
    target: string
    agent_ids: number[]
    agent_names: string[]
    agent_count: number
    probe_count: number
    avg_end_latency_ms?: number
    avg_end_loss_pct?: number
    has_issues: boolean
}

export interface RouteIncident {
    id: string
    type: 'ip_change' | 'isp_change' | 'route_change'
    severity: string
    agent_id: number
    agent_name: string
    probe_id?: number
    target?: string
    message: string
    evidence?: string[]
    detected_at?: string

    // Structured change data for route_change incidents. Optional on
    // other incident types. Lets the UI render a "before / after" view
    // of the actual IP paths, not just fingerprint hashes.
    baseline_fingerprint?: string
    current_fingerprint?: string
    baseline_path?: string          // Human-readable baseline hop list ("1.2.3.4 -> 5.6.7.8")
    current_path?: string           // Human-readable current hop list
    baseline_hop_count?: number
    current_hop_count?: number
    added_hops?: string[]           // IPs that appear in current but not baseline
    removed_hops?: string[]         // IPs that appear in baseline but not current
    jaccard?: number                // 0..1 similarity between baseline and current hop sets
    stability_pct?: number          // Dominant signature's share of recent traces
    trace_count?: number            // Traces considered for this change detection
}

export interface WorkspaceRouteAnalysis {
    workspace_id: number
    agents: AgentRouteInfo[]
    shared_hops: SharedHopInfo[]
    shared_destinations?: SharedDestinationInfo[]
    shared_asns?: SharedAsnInfo[]
    common_targets?: CommonTargetInfo[]
    incidents: RouteIncident[]
    total_agents: number
    total_routes: number
    generated_at: string
}
