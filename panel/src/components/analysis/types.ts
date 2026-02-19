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

// Status color mapping
export const statusColors: Record<string, { bg: string; text: string; icon: string }> = {
    healthy: { bg: 'rgba(16, 185, 129, 0.15)', text: '#10b981', icon: 'bi-check-circle-fill' },
    degraded: { bg: 'rgba(245, 158, 11, 0.15)', text: '#f59e0b', icon: 'bi-exclamation-triangle-fill' },
    outage: { bg: 'rgba(239, 68, 68, 0.15)', text: '#ef4444', icon: 'bi-x-octagon-fill' },
    unknown: { bg: 'rgba(107, 114, 128, 0.15)', text: '#6b7280', icon: 'bi-question-circle-fill' },
}

// Grade color mapping for consistent UI theming
export const gradeColors: Record<string, { bg: string; text: string; border: string }> = {
    excellent: { bg: 'rgba(16, 185, 129, 0.15)', text: '#10b981', border: '#10b981' },
    good: { bg: 'rgba(59, 130, 246, 0.15)', text: '#3b82f6', border: '#3b82f6' },
    fair: { bg: 'rgba(245, 158, 11, 0.15)', text: '#f59e0b', border: '#f59e0b' },
    poor: { bg: 'rgba(249, 115, 22, 0.15)', text: '#f97316', border: '#f97316' },
    critical: { bg: 'rgba(239, 68, 68, 0.15)', text: '#ef4444', border: '#ef4444' },
    unknown: { bg: 'rgba(107, 114, 128, 0.15)', text: '#6b7280', border: '#6b7280' },
}

// Severity icon mapping
export const severityIcons: Record<string, string> = {
    info: 'bi-info-circle',
    warning: 'bi-exclamation-triangle',
    critical: 'bi-x-octagon',
}
