# MTR Analysis Prototype

This folder contains prototype components for displaying enriched MTR path analysis data in the NetWatcher probe view.

## Components

| Component | Description |
|-----------|-------------|
| `MtrAnalysisView.vue` | Main container with header, E2E stats, and tab navigation |
| `PathVisualization.vue` | Interactive hops table, border crossings, and expected vs actual analysis |
| `FindingsList.vue` | Severity-sorted findings with expandable evidence sections |
| `SignalsPanel.vue` | ICMP artifacts, latency/jitter anomalies, and policy flags |
| `RecommendationsPanel.vue` | Suggested tests with copyable commands and upstream questions |
| `MtrAnalysisDemo.vue` | Demo page with sample data for testing |

## Data Structure

The components expect data conforming to the `MtrAnalysisData` interface defined in `types.ts`. Key sections include:

- **meta**: Source/destination regions, traffic type, target info, timestamp
- **path**: Hops array with role guesses, metrics, notes, border crossings
- **signals**: ICMP artifacts, latency/jitter anomalies, path policy flags
- **findings**: Categorized analysis results with severity, evidence, and next steps
- **questions_for_upstream**: Suggested provider inquiries
- **recommended_tests**: CLI commands with explanations

## Usage

```vue
<template>
  <MtrAnalysisView :data="analysisData" />
</template>

<script setup>
import { MtrAnalysisView } from '@/components/mtr-analysis-prototype';

const analysisData = {
  // ... MtrAnalysisData object
};
</script>
```

## Demo

To preview the component with sample data, import the demo component:

```vue
<template>
  <MtrAnalysisDemo />
</template>

<script setup>
import { MtrAnalysisDemo } from '@/components/mtr-analysis-prototype';
</script>
```

## Features

### Header Section
- Source → Destination route visualization
- E2E performance stats (loss, latency, jitter)
- Traffic type and timestamp badges

### Path & Hops Tab
- **Path Summary Banner**: Anycast, backhaul, IX bypass detection flags
- **Border Crossings Panel**: Country transitions with hop ranges
- **Interactive Hops Table**: 
  - Role badge color-coding (destination, edge, core, peering, access)
  - Loss/latency/jitter metrics with severity coloring
  - RDNS resolution display
- **Expandable Hop Notes**: Per-hop analysis notes
- **Expected vs Actual**: Path comparison with mismatch impact

### Findings Tab
- Severity-sorted cards (critical → warning → info)
- Confidence percentage indicators
- Collapsible sections for evidence, impact, and next steps

### Signals Tab
- **ICMP Artifacts**: Rate-limited hops, non-propagating loss, timeout segments
- **Latency Anomalies**: High variance detection with confidence
- **Jitter Anomalies**: Queueing/ECMP artifact identification
- **Path Policy Flags**: Routing policy concerns with impact assessment

### Recommendations Tab
- **Suggested Tests**: Copyable command examples with rationale
- **Upstream Questions**: Provider inquiries tied to detected policy flags

## Styling

Components use:
- Bootstrap 5 variables for consistent theming
- JetBrains Mono font for technical content
- Dark/light mode support via CSS variables
- Responsive layouts for smaller screens
