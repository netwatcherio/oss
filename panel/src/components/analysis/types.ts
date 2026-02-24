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
    avg_latency: number   // ms
    p95_latency: number   // ms
    packet_loss: number   // percentage
    jitter: number        // ms
    sample_count: number
}

export interface AnalysisSignal {
    type: string       // icmp_artifact, route_change, high_loss, high_latency, jitter_anomaly
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
    rate_limited_hops: number[]
    timeout_segments: string[]
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
