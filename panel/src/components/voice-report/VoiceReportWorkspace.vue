<script setup lang="ts">
// panel/src/components/voice-report/VoiceReportWorkspace.vue
// Per-workspace voice report: heatmap of agents by direction, top
// issues list, recurring-failure callouts, and overall rollup.
// Built off `view_mode: "workspace"`.

import { computed } from 'vue'
import type { MosGrade, VoiceCommonFailure, VoiceReportData } from './types'
import VoiceReportHeader from './VoiceReportHeader.vue'

const props = defineProps<{
  data: VoiceReportData
}>()

const heatmap = computed(() => props.data.heatmap ?? [])
const topIssues = computed(() => props.data.top_issues ?? [])
const allIssues = computed(() => props.data.issues ?? [])
const commonFailures = computed<VoiceCommonFailure[]>(() => props.data.common_failures ?? [])

const meanMos = computed(() => {
  const hm = heatmap.value
  const moses = hm
    .map((h) => h.forward_mos ?? h.reverse_mos ?? 0)
    .filter((v) => v > 0)
  if (moses.length === 0) return 0
  return moses.reduce((a, b) => a + b, 0) / moses.length
})

const criticalCount = computed(() => allIssues.value.filter((i) => i.severity === 'critical').length)
const warningCount = computed(() => allIssues.value.filter((i) => i.severity === 'warning').length)

function gradeClass(grade?: MosGrade): string {
  switch (grade) {
    case 'excellent':
      return 'vr-row-info'
    case 'good':
      return 'vr-row-info'
    case 'fair':
      return 'vr-row-warn'
    case 'poor':
    case 'critical':
      return 'vr-row-crit'
    default:
      return ''
  }
}

function severityChipClass(severity: string): string {
  switch (severity) {
    case 'critical': return 'vr-badge crit'
    case 'warning': return 'vr-badge warn'
    case 'info': return 'vr-badge info'
    default: return 'vr-badge info'
  }
}
</script>

<template>
  <div class="vr-page" id="page1">
    <VoiceReportHeader :meta="data.meta" />

    <div class="vr-verdict">
      <div class="vr-mos-ring">
        <svg viewBox="0 0 86 86">
          <circle cx="43" cy="43" r="37" fill="none" stroke="rgba(255,255,255,.12)" stroke-width="7" />
          <circle
            cx="43"
            cy="43"
            r="37"
            fill="none"
            stroke="#22c55e"
            stroke-width="7"
            stroke-linecap="round"
            :stroke-dasharray="(2 * Math.PI * 37).toFixed(1)"
            :stroke-dashoffset="(2 * Math.PI * 37 * (1 - Math.max(0, Math.min(1, (meanMos - 1) / 4)))).toFixed(1)"
          />
        </svg>
        <div class="val">
          <b>{{ meanMos.toFixed(2) }}</b>
          <span>AVG MOS</span>
        </div>
      </div>
      <div class="vr-verdict-body">
        <h2>Workspace voice quality summary</h2>
        <p>
          {{ heatmap.length }} agents · {{ criticalCount }} critical / {{ warningCount }} warning issues across the workspace.
        </p>
      </div>
      <div class="vr-grade-chip">
        <b>{{ heatmap.length }}</b>
        <span>Agents</span>
      </div>
      <div class="vr-grade-chip">
        <b>{{ criticalCount }}</b>
        <span>Critical</span>
      </div>
    </div>

    <div class="vr-section">
      <div class="vr-section-title">Agent Voice Quality Heatmap</div>
      <table class="vr-table">
        <thead>
          <tr>
            <th>Agent</th>
            <th class="num">Forward MOS</th>
            <th>Forward Grade</th>
            <th class="num">Return MOS</th>
            <th>Return Grade</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in heatmap" :key="row.agent_id" :class="gradeClass(row.forward_grade ?? row.reverse_grade)">
            <td>{{ row.agent_name }}</td>
            <td class="num vr-mono">{{ row.forward_mos?.toFixed(2) ?? '—' }}</td>
            <td>{{ row.forward_grade ?? '—' }}</td>
            <td class="num vr-mono">{{ row.reverse_mos?.toFixed(2) ?? '—' }}</td>
            <td>{{ row.reverse_grade ?? '—' }}</td>
          </tr>
          <tr v-if="heatmap.length === 0">
            <td colspan="5" class="text-center text-muted">No voice-quality data in this window.</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="commonFailures.length > 0" class="vr-section" data-testid="common-failures">
      <div class="vr-section-title">Recurring Voice Quality Issues Across Agents</div>
      <p class="vr-section-note">
        Top patterns detected across all {{ heatmap.length }} agents in this workspace in the selected window.
        Sorted by occurrence count; affected agents listed underneath each.
      </p>
      <div v-for="cf in commonFailures" :key="cf.category" class="vr-common-failure-card">
        <div class="vr-common-failure-head">
          <div class="vr-common-failure-title">
            <span class="vr-badge" :class="cf.critical_count > 0 ? 'crit' : 'warn'">
              {{ cf.count }}
            </span>
            <strong>{{ cf.title }}</strong>
            <span class="vr-common-failure-tag">{{ cf.category }}</span>
          </div>
          <div class="vr-common-failure-counts">
            <span v-if="cf.critical_count > 0" :class="severityChipClass('critical')">
              {{ cf.critical_count }} critical
            </span>
            <span v-if="cf.warning_count > 0" :class="severityChipClass('warning')">
              {{ cf.warning_count }} warning
            </span>
          </div>
        </div>
        <div v-if="cf.affected_agents.length > 0" class="vr-common-failure-agents">
          <span class="vr-common-failure-label">Affected agents:</span>
          <span
            v-for="agent in cf.affected_agents"
            :key="agent.agent_id"
            class="vr-common-failure-agent"
            :class="severityChipClass(agent.severity)"
            :title="`${agent.agent_name}${agent.target_name ? ' → ' + agent.target_name : ''}: MOS impact ${agent.mos_impact.toFixed(2)}`"
          >
            {{ agent.agent_name }}<span v-if="agent.target_name" class="vr-target-suffix"> → {{ agent.target_name }}</span>
          </span>
        </div>
        <div v-if="cf.sample_issue" class="vr-common-failure-evidence">
          <span class="vr-section-note">Sample evidence:</span>
          {{ cf.sample_issue.suspected_cause }}<span v-if="cf.sample_issue.recommendations?.length"> · Recommended: {{ cf.sample_issue.recommendations[0] }}</span>
        </div>
      </div>
    </div>

    <div v-if="topIssues.length > 0" class="vr-section">
      <div class="vr-section-title">Top Voice Quality Issues</div>
      <table class="vr-table">
        <thead>
          <tr>
            <th>Severity</th>
            <th>Title</th>
            <th>Target</th>
            <th>Category</th>
            <th>MOS Δ</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="i in topIssues" :key="i.id" :class="i.severity === 'critical' ? 'vr-row-crit' : i.severity === 'warning' ? 'vr-row-warn' : 'vr-row-info'">
            <td>
              <span class="vr-badge" :class="i.severity">{{ i.severity }}</span>
            </td>
            <td>{{ i.title }}</td>
            <td>{{ i.target_agent_name }}</td>
            <td>{{ i.category }}</td>
            <td class="num vr-mono">{{ i.mos_degradation.toFixed(2) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <footer class="vr-page-footer">
      <span>Generated by netwatcher.io — Workspace voice quality</span>
      <span>Page 1 of 1</span>
    </footer>
  </div>
</template>