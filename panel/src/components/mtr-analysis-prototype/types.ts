// MTR Analysis Data Types

export interface MtrAnalysisMeta {
    source_region: string;
    dest_region: string;
    traffic_type: string;
    target_ip: string;
    target_name: string;
    measurement_type: string;
    measurement_time: string;
}

export interface MtrHop {
    hop: number;
    ip: string | null;
    rdns: string | null;
    role_guess: string;
    region_guess: string | null;
    country_guess: string | null;
    loss_pct: number | null;
    rtt_avg_ms: number | null;
    rtt_best_ms: number | null;
    rtt_worst_ms: number | null;
    rtt_stdev_ms: number | null;
    notes: string[];
}

export interface BorderCrossing {
    from_country: string;
    to_country: string;
    between_hops: [number, number];
    notes: string[];
}

export interface ExpectedVsActual {
    expected_path_summary: string;
    actual_path_summary: string;
    why_mismatch_matters: string[];
}

export interface MtrPath {
    hops: MtrHop[];
    regions_traversed: string[];
    countries_traversed: string[];
    border_crossings: BorderCrossing[];
    backhaul_suspected: boolean;
    ix_bypass_suspected: boolean;
    anycast_suspected: boolean;
    expected_vs_actual: ExpectedVsActual;
}

export interface EndToEndSignals {
    loss_pct: number;
    rtt_avg_ms: number;
    jitter_indicator: 'low' | 'medium' | 'high';
}

export interface IcmpArtifacts {
    rate_limit_suspected_hops: number[];
    non_propagating_loss_hops: number[];
    timeout_only_segments: {
        from_hop: number;
        to_hop: number;
        notes: string[];
    }[];
}

export interface LatencyAnomaly {
    hop: number;
    type: string;
    evidence: string;
    confidence: number;
}

export interface JitterAnomaly {
    hop: number;
    evidence: string;
    confidence: number;
}

export interface PathPolicyFlag {
    flag: string;
    evidence: string;
    impact: string;
    confidence: number;
}

export interface MtrSignals {
    end_to_end: EndToEndSignals;
    icmp_artifacts: IcmpArtifacts;
    latency_anomalies: LatencyAnomaly[];
    jitter_anomalies: JitterAnomaly[];
    path_policy_flags: PathPolicyFlag[];
}

export interface Finding {
    id: string;
    title: string;
    severity: 'info' | 'warning' | 'critical';
    category: string;
    summary: string;
    evidence: string[];
    why_it_matters: string[];
    recommended_next_steps: string[];
    confidence: number;
}

export interface UpstreamQuestion {
    question: string;
    why: string;
    related_flags: string[];
}

export interface RecommendedTest {
    test: string;
    command_example: string;
    why: string;
}

export interface MtrAnalysisData {
    meta: MtrAnalysisMeta;
    path: MtrPath;
    signals: MtrSignals;
    findings: Finding[];
    questions_for_upstream: UpstreamQuestion[];
    recommended_tests: RecommendedTest[];
}
