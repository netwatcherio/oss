// panel/src/components/voice-report/types.ts
//
// TypeScript mirror of the controller's `probe.VoiceQualitySummary`,
// `probe.VoicePairSummary`, `probe.VoicePathMetrics`, and friends —
// the JSON shape the live voice report view consumes.
//
// The backend builds these structs in `controller/internal/probe/analysis.go`
// (and the report-data builder we'll add in wave 3). The frontend
// renders them with the same field names so the template's `REPORT_DATA`
// shape stays drop-in compatible.

export type MosGrade =
  | 'excellent'
  | 'good'
  | 'fair'
  | 'poor'
  | 'critical'
  | 'unknown';

export type VoicePathDirection = 'forward' | 'return';

export type CongestionLevel = 'none' | 'mild' | 'moderate' | 'severe';

export interface VoicePathMetrics {
  direction: VoicePathDirection;
  target_agent_id: number;
  target_agent_name: string;
  source_agent_id: number;
  source_agent_name: string;
  probe_id: number;
  probe_type: string;
  mos_score: number;
  avg_latency_ms: number;
  p95_latency_ms: number;
  median_latency_ms: number;
  jitter_avg_ms: number;
  jitter_median_ms?: number;
  jitter_p95_ms?: number;
  packet_loss_pct: number;
  out_of_sequence_pct: number;
  duplicate_pct: number;
  sample_count: number;
  mos_contributing_factors?: string[];
  congestion_level: CongestionLevel;
  max_consecutive_loss?: number;
  total_bursts?: number;
}

export interface VoiceBucket {
  timestamp: string;
  forward: number;
  return: number;
  latency_ms: number;
  jitter_ms: number;
  loss_pct: number;
}

export interface VoiceTrends {
  bucket_minutes: number;
  forward: VoiceBucket[];
  return: VoiceBucket[];
  combined: VoiceBucket[];
  issue_buckets?: string[];
}

export interface BaselineDelta {
  from: string;
  to: string;
  mos_delta: number;
  latency_delta_ms: number;
  jitter_delta_ms: number;
  loss_delta_pct: number;
  sample_count: number;
  baseline_samples: number;
  trend: 'improving' | 'stable' | 'worsening' | 'unknown';
  percent_change?: number;
}

export interface VoiceThresholds {
  warning_jitter_ms: number;
  critical_jitter_ms: number;
  jitter_spike_multiplier: number;
  warning_loss_pct: number;
  critical_loss_pct: number;
  new_loss_baseline_max_pct: number;
  new_loss_current_min_pct: number;
  asymmetry_mos_ratio_min: number;
  asymmetry_min_forward_mos: number;
  latency_only_min_ms: number;
  latency_only_max_loss_pct: number;
  latency_only_max_mos: number;
  out_of_sequence_pct: number;
  excellent_mos: number;
  good_mos: number;
  fair_mos: number;
  poor_mos: number;
  congestion_jitter_ms: number;
  congestion_loss_pct: number;
  congestion_latency_ms: number;
  codec: string;
}

export interface RouteSignal {
  probe_id: number;
  probe_type: string;
  type: string; // route_change, hop_count_change, isp_change, ip_change
  severity: string;
  title: string;
  evidence: string;
  detected_at: string;
}

export interface VoiceQualityIssue {
  id: string;
  severity: 'info' | 'warning' | 'critical';
  title: string;
  category:
    | 'jitter_spike'
    | 'packet_loss'
    | 'latency_degradation'
    | 'asymmetry'
    | 'out_of_order'
    | 'burst_loss'
    | string;
  affected_path: VoicePathDirection;
  target_agent_name: string;
  suspected_cause: string;
  evidence: string[];
  time_pattern: string;
  first_detected?: string;
  last_detected?: string;
  mos_degradation: number;
  mos_before: number;
  mos_after: number;
  recommendations: string[];
  // Heuristic enrichment fields populated by the new detectors.
  loss_pattern?: string;
  likely_hop?: number;
  hop_evidence?: string;
  duration_buckets?: number;
  total_buckets?: number;
}

export interface AgentRef {
  id: number;
  name: string;
  ip?: string;
  location?: string;
}

export interface TargetRef {
  name: string;
  host?: string;
  ip?: string;
  agent_id?: number;
  agent_name?: string;
}

export interface VoicePairSummary {
  id: string;
  agent: AgentRef;
  target: TargetRef;
  forward?: VoicePathMetrics;
  reverse?: VoicePathMetrics;
  issues: VoiceQualityIssue[];
  baseline?: BaselineDelta;
  trends?: VoiceTrends;
  route_signals?: RouteSignal[];
  thresholds: VoiceThresholds;
  overall_mos: number;
  overall_grade: MosGrade;
  recommendation?: string;
  time_pattern?: string;
}

// ---- Top-level report shapes (per the HTML templates) ----

export interface VoiceReportMeta {
  report_id: string;
  generated_at: string;
  view_mode: 'probe' | 'agent' | 'workspace';
  agent?: AgentRef;
  target?: TargetRef;
  workspace?: {
    id: number;
    name: string;
  };
  test?: {
    type: string;
    codec: string;
    duration: string;
    interval: string;
    packets_sent: number;
    dscp: string;
    payload_size: string;
  };
  window?: string;
}

export interface VoiceReportSummary {
  mos: number;
  r_factor: number;
  grade: string;
  verdict_title: string;
  verdict_text: string;
}

export interface VoiceReportMetrics {
  latency: { min: number; avg: number; max: number; stddev: number; unit: string };
  jitter: { min: number; avg: number; max: number; stddev: number; unit: string };
  one_way_up?: { min: number; avg: number; max: number; stddev: number; unit: string };
  one_way_down?: { min: number; avg: number; max: number; stddev: number; unit: string };
  packets: {
    sent: number;
    received: number;
    lost: number;
    loss_pct: number;
    duplicates: number;
    dup_pct: number;
    out_of_order: number;
    ooo_pct: number;
    discarded_jitter_buffer: number;
    discard_pct: number;
  };
}

export interface VoiceReportQualityRow {
  component: string;
  value: string;
  note: string;
}

export interface VoiceReportTracerouteHop {
  hop: number;
  host: string;
  ip: string;
  asn?: string;
  loss: number;
  sent: number;
  last: number;
  avg: number;
  best: number;
  worst: number;
  stdev: number;
}

export interface VoiceReportData {
  meta: VoiceReportMeta;
  summary: VoiceReportSummary;
  thresholds: VoiceThresholds;
  metrics: VoiceReportMetrics;
  quality?: VoiceReportQualityRow[];
  pairs?: VoicePairSummary[];
  timeseries?: {
    rtt?: number[];
    jitter?: number[];
    loss_per_interval?: number[];
    forward_mos?: VoiceBucket[];
    reverse_mos?: VoiceBucket[];
  };
  traceroute?: {
    protocol: string;
    hops: VoiceReportTracerouteHop[];
    note?: string;
  };
  // Workspace-level rollup fields (only set on per-workspace reports):
  top_issues?: VoiceQualityIssue[];
  heatmap?: Array<{
    agent_id: number;
    agent_name: string;
    forward_mos?: number;
    reverse_mos?: number;
    forward_grade?: MosGrade;
    reverse_grade?: MosGrade;
  }>;
  issues?: VoiceQualityIssue[];
}