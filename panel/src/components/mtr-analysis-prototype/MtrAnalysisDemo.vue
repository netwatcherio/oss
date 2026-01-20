<template>
  <div class="demo-page">
    <div class="demo-container">
      <MtrAnalysisView :data="sampleData" />
    </div>
  </div>
</template>

<script lang="ts" setup>
import MtrAnalysisView from './MtrAnalysisView.vue';
import type { MtrAnalysisData } from './types';

// Sample data from the user's request
const sampleData: MtrAnalysisData = {
  meta: {
    source_region: "Vancouver, BC, CA",
    dest_region: "Cloudflare 1.1.1.1 anycast (likely Seattle, WA, US)",
    traffic_type: "dns",
    target_ip: "1.1.1.1",
    target_name: "one.one.one.one",
    measurement_type: "mtr",
    measurement_time: "2026-01-19T20:27:15-08:00"
  },
  path: {
    hops: [
      {
        hop: 1,
        ip: "72.142.150.3",
        rdns: "unallocated-static.datacentres.rogers.com",
        role_guess: "access_gateway_or_first_hop_router",
        region_guess: "Vancouver, BC",
        country_guess: "CA",
        loss_pct: 0.0,
        rtt_avg_ms: 0.18,
        rtt_best_ms: 0.16,
        rtt_worst_ms: 0.22,
        rtt_stdev_ms: 0.02,
        notes: [
          "Rogers-owned rDNS; likely first routed hop beyond host/VM network"
        ]
      },
      {
        hop: 2,
        ip: "174.137.183.5",
        rdns: "174.137.183.5",
        role_guess: "isp_access_or_aggregation",
        region_guess: "Vancouver, BC",
        country_guess: "CA",
        loss_pct: 0.0,
        rtt_avg_ms: 0.2,
        rtt_best_ms: 0.17,
        rtt_worst_ms: 0.25,
        rtt_stdev_ms: 0.03,
        notes: [
          "Unresolved rDNS; timing suggests still local/metro aggregation"
        ]
      },
      {
        hop: 3,
        ip: "207.245.205.89",
        rdns: "207.245.205.89",
        role_guess: "isp_to_transit_handoff_or_metro_core",
        region_guess: "Vancouver, BC",
        country_guess: "CA",
        loss_pct: 0.0,
        rtt_avg_ms: 8.24,
        rtt_best_ms: 1.16,
        rtt_worst_ms: 32.31,
        rtt_stdev_ms: 12.12,
        notes: [
          "High variance with only 5 probes; could be queueing, ICMP handling variance, or early ECMP/handoff effects"
        ]
      },
      {
        hop: 4,
        ip: "64.125.31.255",
        rdns: "ae33.mpr3.yvr3.ca.zip.zayo.com",
        role_guess: "metro_peering_router",
        region_guess: "Vancouver, BC",
        country_guess: "CA",
        loss_pct: 0.0,
        rtt_avg_ms: 12.06,
        rtt_best_ms: 12.01,
        rtt_worst_ms: 12.13,
        rtt_stdev_ms: 0.04,
        notes: [
          "Zayo naming: yvr3=Vancouver; mpr=metro/peering router; ae=aggregated ethernet"
        ]
      },
      {
        hop: 5,
        ip: "64.125.31.246",
        rdns: "ae31.cs1.sea1.us.zip.zayo.com",
        role_guess: "core_switch_or_core_edge",
        region_guess: "Seattle, WA",
        country_guess: "US",
        loss_pct: 60.0,
        rtt_avg_ms: 15.31,
        rtt_best_ms: 15.28,
        rtt_worst_ms: 15.33,
        rtt_stdev_ms: 0.03,
        notes: [
          "Zayo naming: sea1=Seattle; cs often used for core switch/cluster; loss does not propagate to destination (likely ICMP rate-limit/deprioritization)"
        ]
      },
      {
        hop: 6,
        ip: "64.125.28.193",
        rdns: "ae8.cr1.sea1.us.zip.zayo.com",
        role_guess: "core_router",
        region_guess: "Seattle, WA",
        country_guess: "US",
        loss_pct: 20.0,
        rtt_avg_ms: 15.44,
        rtt_best_ms: 15.39,
        rtt_worst_ms: 15.53,
        rtt_stdev_ms: 0.06,
        notes: [
          "Zayo naming: cr=core router; non-propagating loss strongly suggests ICMP artifact"
        ]
      },
      {
        hop: 7,
        ip: null,
        rdns: null,
        role_guess: "non_responding_router_or_filtered_icmp",
        region_guess: null,
        country_guess: null,
        loss_pct: null,
        rtt_avg_ms: null,
        rtt_best_ms: null,
        rtt_worst_ms: null,
        rtt_stdev_ms: null,
        notes: [
          "Timeout-only hop; commonly due to ICMP TTL-expired filtering or rate limiting"
        ]
      },
      {
        hop: 8,
        ip: "208.184.88.166",
        rdns: "208.184.88.166.IPYX-330228-003-ZYO.zip.zayo.com",
        role_guess: "provider_edge_to_peer_or_customer_vlan",
        region_guess: "Seattle, WA",
        country_guess: "US",
        loss_pct: 0.0,
        rtt_avg_ms: 31.04,
        rtt_best_ms: 16.45,
        rtt_worst_ms: 61.96,
        rtt_stdev_ms: 17.27,
        notes: [
          "Zayo-hosted interface label suggests a specific interconnect/VLAN; high RTT variance here does not match stable destination RTT (likely ICMP handling variance or per-probe ECMP)"
        ]
      },
      {
        hop: 9,
        ip: "108.162.243.39",
        rdns: "108.162.243.39",
        role_guess: "cloudflare_edge_router",
        region_guess: "Seattle, WA",
        country_guess: "US",
        loss_pct: 0.0,
        rtt_avg_ms: 26.78,
        rtt_best_ms: 16.39,
        rtt_worst_ms: 66.71,
        rtt_stdev_ms: 19.96,
        notes: [
          "Cloudflare-owned IP range; variance likely reflects ICMP response behavior/ECMP rather than end-to-end data-plane jitter (destination is stable)"
        ]
      },
      {
        hop: 10,
        ip: "1.1.1.1",
        rdns: "one.one.one.one",
        role_guess: "anycast_service_endpoint",
        region_guess: "Seattle, WA",
        country_guess: "US",
        loss_pct: 0.0,
        rtt_avg_ms: 16.31,
        rtt_best_ms: 16.24,
        rtt_worst_ms: 16.46,
        rtt_stdev_ms: 0.08,
        notes: [
          "Stable RTT and zero loss at destination; path is functioning cleanly end-to-end for reachability"
        ]
      }
    ],
    regions_traversed: [
      "Vancouver, BC, CA",
      "Seattle, WA, US"
    ],
    countries_traversed: [
      "CA",
      "US"
    ],
    border_crossings: [
      {
        from_country: "CA",
        to_country: "US",
        between_hops: [4, 5],
        notes: [
          "yvr3.ca -> sea1.us indicates Canada-to-US handoff via Zayo long-haul/metro-to-metro segment"
        ]
      }
    ],
    backhaul_suspected: true,
    ix_bypass_suspected: true,
    anycast_suspected: true,
    expected_vs_actual: {
      expected_path_summary: "From Vancouver, DNS anycast to 1.1.1.1 would ideally land on a Cloudflare PoP/edge reachable via local peering (e.g., a Vancouver metro edge / local IX) with minimal or no border crossing.",
      actual_path_summary: "Traffic enters Zayo in Vancouver (yvr3) and is carried to Seattle (sea1) before handing to Cloudflare anycast, implying the anycast catchment/handoff occurs in Seattle rather than Vancouver.",
      why_mismatch_matters: [
        "Cross-border backhaul increases policy and congestion exposure (more shared segments and more places for intermittent loss/queueing).",
        "Even if average RTT is acceptable, remote handoff can worsen tail latency during microbursts and amplify jitter sensitivity for real-time media.",
        "Return path may be asymmetric, increasing the risk of one-way media or intermittent RTP issues when NAT/firewalls and carrier policies interact."
      ]
    }
  },
  signals: {
    end_to_end: {
      loss_pct: 0.0,
      rtt_avg_ms: 16.31,
      jitter_indicator: "low"
    },
    icmp_artifacts: {
      rate_limit_suspected_hops: [5, 6, 7],
      non_propagating_loss_hops: [5, 6],
      timeout_only_segments: [
        {
          from_hop: 6,
          to_hop: 8,
          notes: [
            "Single-hop timeout at 7 with later replies; consistent with ICMP filtering/rate limiting"
          ]
        }
      ]
    },
    latency_anomalies: [
      {
        hop: 3,
        type: "high_variance_rtt",
        evidence: "Avg 8.24ms, Best 1.16ms, Worst 32.31ms, StDev 12.12ms on only 5 probes",
        confidence: 0.55
      },
      {
        hop: 8,
        type: "high_variance_rtt_non_propagating",
        evidence: "Avg 31.04ms, Best 16.45ms, Worst 61.96ms, StDev 17.27ms while destination remains stable at ~16.3ms",
        confidence: 0.75
      },
      {
        hop: 9,
        type: "high_variance_rtt_non_propagating",
        evidence: "Avg 26.78ms, Best 16.39ms, Worst 66.71ms, StDev 19.96ms while destination remains stable at ~16.3ms",
        confidence: 0.75
      }
    ],
    jitter_anomalies: [
      {
        hop: 3,
        evidence: "Large spread between best and worst RTT; could indicate transient queueing or ICMP processing variance",
        confidence: 0.5
      },
      {
        hop: 8,
        evidence: "High RTT variance at an interconnect-like hop; likely ICMP/ECMP artifact but worth validating with UDP/TCP-based probes",
        confidence: 0.6
      }
    ],
    path_policy_flags: [
      {
        flag: "seattle_hairpin_for_anycast_service",
        evidence: "Zayo yvr3.ca -> Zayo sea1.us -> Cloudflare IP space before reaching 1.1.1.1",
        impact: "Adds cross-border segment and reliance on Seattle hub; can increase tail latency and intermittent jitter exposure.",
        confidence: 0.85
      },
      {
        flag: "ix_bypass_or_missing_local_peering",
        evidence: "Traffic leaves Vancouver on Zayo toward Seattle rather than handing to Cloudflare locally in Vancouver metro",
        impact: "Suboptimal locality for DNS anycast; may indicate policy or absent peering causing non-local catchment.",
        confidence: 0.75
      },
      {
        flag: "icmp_rate_limiting_present",
        evidence: "60% loss at hop 5 and 20% at hop 6 with 0% loss at destination; hop 7 timeouts only",
        impact: "Can mislead operators into thinking there is real packet loss; data-plane likely fine end-to-end for this test.",
        confidence: 0.95
      }
    ]
  },
  findings: [
    {
      id: "F1",
      title: "End-to-end reachability and latency to 1.1.1.1 are clean",
      severity: "info",
      category: "performance",
      summary: "Destination shows 0% loss and stable ~16.3ms RTT with very low variance, indicating good end-to-end performance for this snapshot.",
      evidence: [
        "Hop 10: 0% loss, Avg 16.31ms, Best 16.24ms, Worst 16.46ms, StDev 0.08ms"
      ],
      why_it_matters: [
        "Confirms the path is currently usable for DNS resolution and general traffic without obvious impairment."
      ],
      recommended_next_steps: [
        "Keep this as a baseline reference; compare during incident windows with higher probe counts (e.g., 100+)."
      ],
      confidence: 0.9
    },
    {
      id: "F2",
      title: "Non-propagating loss at intermediate Zayo hops is an ICMP artifact",
      severity: "info",
      category: "measurement_artifact",
      summary: "Reported loss at hops 5 and 6 does not carry through to the destination, strongly indicating ICMP rate limiting/deprioritization rather than real forwarding loss.",
      evidence: [
        "Hop 5: 60% loss; Hop 6: 20% loss; Hop 10: 0% loss",
        "Hop 7 is timeout-only while later hops respond"
      ],
      why_it_matters: [
        "Avoids false alarms and mis-triage when interpreting MTR/traceroute output.",
        "Real-time media troubleshooting should focus on destination loss/jitter and application-layer metrics, not isolated mid-hop ICMP loss."
      ],
      recommended_next_steps: [
        "Repeat with UDP-based traceroute (high ports) and TCP-based traceroute to compare behavior against ICMP-only probing.",
        "Increase probe count to reduce small-sample noise."
      ],
      confidence: 0.95
    },
    {
      id: "F3",
      title: "DNS anycast catchment appears to be in Seattle instead of Vancouver",
      severity: "info",
      category: "routing_policy",
      summary: "Traffic enters Zayo in Vancouver (yvr3) and is transported to Seattle (sea1) before reaching Cloudflare, implying non-local anycast handoff and a Canada-to-US border crossing.",
      evidence: [
        "Hop 4: ae33.mpr3.yvr3.ca.zip.zayo.com",
        "Hop 5: ae31.cs1.sea1.us.zip.zayo.com",
        "Hop 9: 108.162.243.39 (Cloudflare space) prior to 1.1.1.1"
      ],
      why_it_matters: [
        "Cross-border backhaul can increase exposure to congestion/policy changes and can worsen tail latency during peak periods.",
        "If similar policy exists for media or SIP/RTP destinations, it can contribute to intermittent one-way audio/jitter without obvious sustained loss."
      ],
      recommended_next_steps: [
        "Determine Cloudflare colo actually serving the anycast by querying Cloudflare diagnostic endpoints from this source (see tests).",
        "Ask upstream/transit (Rogers/Zayo) whether local peering to Cloudflare exists in Vancouver and whether it can be preferred."
      ],
      confidence: 0.8
    },
    {
      id: "F4",
      title: "High variance at hops 8–9 likely reflects ICMP/ECMP behavior, not end-to-end jitter",
      severity: "info",
      category: "measurement_artifact",
      summary: "Hops 8 and 9 show high worst-case RTT and stdev, but the destination remains stable, suggesting ICMP processing variance, ECMP per-probe path differences, or reply-path differences at those routers.",
      evidence: [
        "Hop 8: Avg 31.04ms / Worst 61.96ms / StDev 17.27ms vs Hop 10 stable ~16.3ms",
        "Hop 9: Avg 26.78ms / Worst 66.71ms / StDev 19.96ms vs Hop 10 stable ~16.3ms"
      ],
      why_it_matters: [
        "Prevents mis-attributing media jitter to intermediate hop ICMP variance.",
        "If real jitter exists, it should be visible at the destination or in application-layer metrics (RTP stats, MOS, PLC, retransmits)."
      ],
      recommended_next_steps: [
        "Validate with destination-focused jitter/loss measurements (e.g., OWAMP/TWAMP if available, or UDP probes to a controlled endpoint).",
        "Run longer MTR with more packets and compare with TCP SYN-based MTR."
      ],
      confidence: 0.75
    }
  ],
  questions_for_upstream: [
    {
      question: "Is there local peering available to Cloudflare in Vancouver (metro or VANIX), and if so, why is traffic to 1.1.1.1 being carried to Seattle before handoff?",
      why: "The observed path indicates a Seattle handoff for a DNS anycast service; local peering would typically reduce policy risk and improve tail latency.",
      related_flags: [
        "seattle_hairpin_for_anycast_service",
        "ix_bypass_or_missing_local_peering"
      ]
    },
    {
      question: "Can you confirm the intended egress/handoff policy for Cloudflare prefixes from this source ASN and whether community/LP changes can prefer a Vancouver handoff?",
      why: "If this is policy-driven (localpref/communities), it may be adjustable without physical changes.",
      related_flags: [
        "ix_bypass_or_missing_local_peering"
      ]
    },
    {
      question: "Are there known ICMP rate-limit policies on Zayo sea1 core that would explain apparent mid-hop loss and timeouts?",
      why: "This would validate that intermediate hop loss is expected measurement behavior rather than real impairment.",
      related_flags: [
        "icmp_rate_limiting_present"
      ]
    },
    {
      question: "Is return traffic from Cloudflare to this source symmetric (or at least staying within the same region), and if not, what is the typical return path?",
      why: "Asymmetry can matter for RTP/WebRTC (NAT/firewall traversal, stateful devices) and can produce intermittent one-way media issues.",
      related_flags: [
        "seattle_hairpin_for_anycast_service"
      ]
    }
  ],
  recommended_tests: [
    {
      test: "Longer MTR with more probes to reduce small-sample variance",
      command_example: "mtr -rwzc 200 1.1.1.1",
      why: "5 probes is too small to reliably characterize variance; 200+ probes improves confidence in loss/jitter characterization."
    },
    {
      test: "TCP-based traceroute/MTR to compare against ICMP artifacts",
      command_example: "mtr -rwz -T -P 443 -c 200 1.1.1.1",
      why: "Some routers treat ICMP TTL-expired differently; TCP-based probing can better reflect forwarding behavior relevant to TCP services."
    },
    {
      test: "UDP traceroute toward high port to emulate RTP-like forwarding behavior (where allowed)",
      command_example: "traceroute -n -U -p 33434 1.1.1.1",
      why: "Helps identify whether path differs for UDP flows and whether any middleboxes behave differently for UDP."
    },
    {
      test: "Identify Cloudflare anycast colo actually serving this source",
      command_example: "curl -s https://1.1.1.1/cdn-cgi/trace | egrep 'colo=|ip=|loc='",
      why: "Confirms which Cloudflare PoP (colo code) is handling the anycast request; validates the Seattle catchment inference."
    },
    {
      test: "Cross-check DNS resolution performance and anycast behavior from the same source",
      command_example: "dig @1.1.1.1 whoami.cloudflare TXT +short && dig @1.1.1.1 one.one.one.one A +stats",
      why: "Correlates anycast PoP selection with resolver behavior and provides timing stats relevant to DNS performance."
    }
  ]
};
</script>

<style scoped>
.demo-page {
  min-height: 100vh;
  background: var(--bs-body-bg);
  padding: 2rem;
}

.demo-container {
  max-width: 1400px;
  margin: 0 auto;
}
</style>
