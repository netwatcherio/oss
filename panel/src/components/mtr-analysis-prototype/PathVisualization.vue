<template>
  <div class="path-visualization">
    <!-- Path Summary Banner -->
    <div class="path-summary-banner">
      <div class="path-flags">
        <span v-if="path.anycast_suspected" class="flag-badge anycast">
          <i class="bi bi-broadcast-pin"></i> Anycast Detected
        </span>
        <span v-if="path.backhaul_suspected" class="flag-badge backhaul">
          <i class="bi bi-arrow-repeat"></i> Backhaul Suspected
        </span>
        <span v-if="path.ix_bypass_suspected" class="flag-badge ix-bypass">
          <i class="bi bi-sign-turn-right"></i> IX Bypass
        </span>
      </div>
      <div class="path-traverse">
        <span class="traverse-label">Route:</span>
        <span 
          v-for="(region, idx) in path.regions_traversed" 
          :key="region"
          class="region-tag"
        >
          {{ region }}
          <i v-if="idx < path.regions_traversed.length - 1" class="bi bi-chevron-right"></i>
        </span>
      </div>
    </div>

    <!-- Border Crossings Alert -->
    <div v-if="path.border_crossings.length > 0" class="border-crossings-panel">
      <div class="panel-header">
        <i class="bi bi-flag-fill"></i>
        <span>Border Crossings</span>
      </div>
      <div class="crossings-list">
        <div 
          v-for="(crossing, idx) in path.border_crossings" 
          :key="idx"
          class="crossing-item"
        >
          <div class="crossing-route">
            <span class="country-flag">{{ getCountryEmoji(crossing.from_country) }}</span>
            <span class="country-code">{{ crossing.from_country }}</span>
            <i class="bi bi-arrow-right"></i>
            <span class="country-flag">{{ getCountryEmoji(crossing.to_country) }}</span>
            <span class="country-code">{{ crossing.to_country }}</span>
          </div>
          <div class="crossing-hops">
            Between hops {{ crossing.between_hops[0] }} → {{ crossing.between_hops[1] }}
          </div>
          <div v-if="crossing.notes.length" class="crossing-notes">
            <span v-for="(note, nIdx) in crossing.notes" :key="nIdx">{{ note }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Hops Table -->
    <div class="hops-table-container">
      <table class="hops-table">
        <thead>
          <tr>
            <th class="col-hop">#</th>
            <th class="col-ip">IP / Host</th>
            <th class="col-role">Role</th>
            <th class="col-region">Region</th>
            <th class="col-loss">Loss</th>
            <th class="col-latency">Latency (ms)</th>
            <th class="col-jitter">Jitter</th>
          </tr>
        </thead>
        <tbody>
          <tr 
            v-for="hop in path.hops" 
            :key="hop.hop"
            :class="getHopRowClass(hop)"
          >
            <td class="col-hop">
              <span class="hop-number">{{ hop.hop }}</span>
            </td>
            <td class="col-ip">
              <div v-if="hop.ip" class="ip-cell">
                <span class="ip-address">{{ hop.ip }}</span>
                <span v-if="hop.rdns && hop.rdns !== hop.ip" class="rdns-name">{{ hop.rdns }}</span>
              </div>
              <div v-else class="timeout-cell">
                <i class="bi bi-asterisk"></i>
                <span>No response</span>
              </div>
            </td>
            <td class="col-role">
              <span class="role-badge" :class="getRoleClass(hop.role_guess)">
                {{ formatRole(hop.role_guess) }}
              </span>
            </td>
            <td class="col-region">
              <span v-if="hop.region_guess" class="region-cell">
                <span class="region-name">{{ hop.region_guess }}</span>
                <span class="country-indicator">{{ hop.country_guess }}</span>
              </span>
              <span v-else class="unknown-region">—</span>
            </td>
            <td class="col-loss">
              <span v-if="hop.loss_pct !== null" class="loss-value" :class="getLossClass(hop.loss_pct)">
                {{ hop.loss_pct.toFixed(1) }}%
              </span>
              <span v-else class="na-value">—</span>
            </td>
            <td class="col-latency">
              <div v-if="hop.rtt_avg_ms !== null" class="latency-cell">
                <span class="latency-avg" :class="getLatencyClass(hop.rtt_avg_ms)">
                  {{ hop.rtt_avg_ms.toFixed(2) }}
                </span>
                <span class="latency-range">
                  {{ hop.rtt_best_ms?.toFixed(1) }}–{{ hop.rtt_worst_ms?.toFixed(1) }}
                </span>
              </div>
              <span v-else class="na-value">—</span>
            </td>
            <td class="col-jitter">
              <span 
                v-if="hop.rtt_stdev_ms !== null" 
                class="jitter-value"
                :class="getJitterClass(hop.rtt_stdev_ms)"
              >
                ±{{ hop.rtt_stdev_ms.toFixed(2) }}
              </span>
              <span v-else class="na-value">—</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Hop Notes (expandable) -->
    <div class="hop-notes-section">
      <div class="notes-header" @click="toggleNotes">
        <i :class="showNotes ? 'bi bi-chevron-up' : 'bi bi-chevron-down'"></i>
        <span>Hop Analysis Notes</span>
        <span class="notes-count">{{ hopNotesCount }} notes</span>
      </div>
      <div v-if="showNotes" class="notes-grid">
        <div 
          v-for="hop in hopsWithNotes" 
          :key="hop.hop"
          class="note-card"
        >
          <div class="note-header">
            <span class="note-hop">Hop {{ hop.hop }}</span>
            <span class="note-ip">{{ hop.ip || '*' }}</span>
          </div>
          <ul class="note-list">
            <li v-for="(note, idx) in hop.notes" :key="idx">{{ note }}</li>
          </ul>
        </div>
      </div>
    </div>

    <!-- Expected vs Actual Panel -->
    <div class="expected-actual-panel">
      <div class="panel-header">
        <i class="bi bi-signpost-split-fill"></i>
        <span>Path Analysis: Expected vs Actual</span>
      </div>
      <div class="comparison-grid">
        <div class="comparison-column expected">
          <div class="comparison-label">
            <i class="bi bi-bullseye"></i> Expected
          </div>
          <p>{{ path.expected_vs_actual.expected_path_summary }}</p>
        </div>
        <div class="comparison-column actual">
          <div class="comparison-label">
            <i class="bi bi-geo-alt"></i> Actual
          </div>
          <p>{{ path.expected_vs_actual.actual_path_summary }}</p>
        </div>
      </div>
      <div v-if="path.expected_vs_actual.why_mismatch_matters.length" class="mismatch-reasons">
        <div class="reasons-header">
          <i class="bi bi-exclamation-triangle-fill"></i>
          Why This Matters
        </div>
        <ul>
          <li 
            v-for="(reason, idx) in path.expected_vs_actual.why_mismatch_matters" 
            :key="idx"
          >
            {{ reason }}
          </li>
        </ul>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed } from 'vue';
import type { MtrPath, MtrHop } from './types';

const props = defineProps<{
  path: MtrPath;
}>();

const showNotes = ref(false);

const hopsWithNotes = computed(() => 
  props.path.hops.filter(h => h.notes && h.notes.length > 0)
);

const hopNotesCount = computed(() => 
  hopsWithNotes.value.reduce((sum, h) => sum + h.notes.length, 0)
);

function toggleNotes() {
  showNotes.value = !showNotes.value;
}

function getCountryEmoji(code: string): string {
  const codePoints = code
    .toUpperCase()
    .split('')
    .map(char => 127397 + char.charCodeAt(0));
  return String.fromCodePoint(...codePoints);
}

function getHopRowClass(hop: MtrHop): string {
  const classes: string[] = [];
  if (!hop.ip) classes.push('timeout-hop');
  if (hop.loss_pct !== null && hop.loss_pct > 0) classes.push('loss-hop');
  if (hop.role_guess.includes('anycast')) classes.push('destination-hop');
  return classes.join(' ');
}

function getRoleClass(role: string): string {
  if (role.includes('anycast') || role.includes('endpoint')) return 'role-destination';
  if (role.includes('edge') || role.includes('cloudflare')) return 'role-edge';
  if (role.includes('core')) return 'role-core';
  if (role.includes('peering') || role.includes('handoff')) return 'role-peering';
  if (role.includes('access') || role.includes('gateway')) return 'role-access';
  if (role.includes('timeout') || role.includes('filtered')) return 'role-timeout';
  return 'role-default';
}

function formatRole(role: string): string {
  return role
    .replace(/_/g, ' ')
    .replace(/\b\w/g, l => l.toUpperCase());
}

function getLossClass(loss: number): string {
  if (loss === 0) return 'good';
  if (loss <= 10) return 'warning';
  return 'critical';
}

function getLatencyClass(latency: number): string {
  if (latency < 20) return 'good';
  if (latency < 50) return 'warning';
  return 'critical';
}

function getJitterClass(jitter: number): string {
  if (jitter < 1) return 'good';
  if (jitter < 5) return 'warning';
  return 'critical';
}
</script>

<style scoped>
.path-visualization {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

/* Summary Banner */
.path-summary-banner {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1rem 1.25rem;
  background: var(--bs-tertiary-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  flex-wrap: wrap;
  gap: 1rem;
}

.path-flags {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.flag-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.35rem 0.75rem;
  border-radius: 6px;
  font-size: 0.75rem;
  font-weight: 600;
}

.flag-badge.anycast {
  background: rgba(var(--bs-info-rgb), 0.15);
  color: var(--bs-info);
}

.flag-badge.backhaul {
  background: rgba(var(--bs-warning-rgb), 0.15);
  color: var(--bs-warning);
}

.flag-badge.ix-bypass {
  background: rgba(var(--bs-danger-rgb), 0.15);
  color: var(--bs-danger);
}

.path-traverse {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.85rem;
}

.traverse-label {
  color: var(--bs-secondary-color);
}

.region-tag {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  color: var(--bs-body-color);
}

.region-tag i {
  color: var(--bs-secondary-color);
  font-size: 0.7rem;
}

/* Border Crossings */
.border-crossings-panel {
  background: var(--bs-tertiary-bg);
  border: 1px solid rgba(var(--bs-warning-rgb), 0.3);
  border-radius: 10px;
  overflow: hidden;
}

.panel-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem 1rem;
  background: rgba(var(--bs-warning-rgb), 0.1);
  color: var(--bs-warning);
  font-weight: 600;
  font-size: 0.85rem;
}

.crossings-list {
  padding: 1rem;
}

.crossing-item {
  padding: 0.75rem;
  background: var(--bs-body-bg);
  border-radius: 8px;
  margin-bottom: 0.5rem;
}

.crossing-item:last-child {
  margin-bottom: 0;
}

.crossing-route {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 600;
  margin-bottom: 0.25rem;
}

.country-flag {
  font-size: 1.25rem;
}

.country-code {
  font-size: 0.9rem;
}

.crossing-route i {
  color: var(--bs-secondary-color);
}

.crossing-hops {
  font-size: 0.75rem;
  color: var(--bs-secondary-color);
  margin-bottom: 0.5rem;
}

.crossing-notes {
  font-size: 0.8rem;
  color: var(--bs-body-color);
  padding: 0.5rem;
  background: var(--bs-tertiary-bg);
  border-radius: 6px;
}

/* Hops Table */
.hops-table-container {
  overflow-x: auto;
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
}

.hops-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.85rem;
}

.hops-table th {
  padding: 0.75rem 1rem;
  background: var(--bs-tertiary-bg);
  color: var(--bs-secondary-color);
  font-weight: 600;
  font-size: 0.7rem;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  text-align: left;
  border-bottom: 1px solid var(--bs-border-color);
}

.hops-table td {
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--bs-border-color);
  vertical-align: middle;
}

.hops-table tr:last-child td {
  border-bottom: none;
}

.hops-table tr:hover {
  background: var(--bs-tertiary-bg);
}

.hops-table tr.timeout-hop {
  opacity: 0.6;
}

.hops-table tr.loss-hop {
  background: rgba(var(--bs-warning-rgb), 0.05);
}

.hops-table tr.destination-hop {
  background: rgba(var(--bs-success-rgb), 0.05);
}

.hop-number {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  background: var(--bs-primary);
  color: white;
  border-radius: 6px;
  font-weight: 600;
  font-size: 0.8rem;
}

.ip-cell {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.ip-address {
  font-weight: 600;
  color: var(--bs-body-color);
}

.rdns-name {
  font-size: 0.75rem;
  color: var(--bs-secondary-color);
  max-width: 250px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.timeout-cell {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: var(--bs-secondary-color);
  font-style: italic;
}

.role-badge {
  display: inline-block;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  font-size: 0.7rem;
  font-weight: 500;
}

.role-destination {
  background: rgba(var(--bs-success-rgb), 0.15);
  color: var(--bs-success);
}

.role-edge {
  background: rgba(var(--bs-info-rgb), 0.15);
  color: var(--bs-info);
}

.role-core {
  background: rgba(var(--bs-primary-rgb), 0.15);
  color: var(--bs-primary);
}

.role-peering {
  background: rgba(var(--bs-warning-rgb), 0.15);
  color: var(--bs-warning);
}

.role-access {
  background: var(--bs-secondary-bg);
  color: var(--bs-secondary-color);
}

.role-timeout {
  background: var(--bs-tertiary-bg);
  color: var(--bs-secondary-color);
  font-style: italic;
}

.role-default {
  background: var(--bs-secondary-bg);
  color: var(--bs-body-color);
}

.region-cell {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.region-name {
  color: var(--bs-body-color);
}

.country-indicator {
  font-size: 0.7rem;
  padding: 0.15rem 0.4rem;
  background: var(--bs-secondary-bg);
  border-radius: 3px;
  color: var(--bs-secondary-color);
}

.unknown-region, .na-value {
  color: var(--bs-secondary-color);
}

.loss-value, .latency-avg, .jitter-value {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.loss-value.good, .latency-avg.good, .jitter-value.good {
  color: var(--bs-success);
}

.loss-value.warning, .latency-avg.warning, .jitter-value.warning {
  color: var(--bs-warning);
}

.loss-value.critical, .latency-avg.critical, .jitter-value.critical {
  color: var(--bs-danger);
}

.latency-cell {
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
}

.latency-range {
  font-size: 0.7rem;
  color: var(--bs-secondary-color);
}

/* Hop Notes */
.hop-notes-section {
  background: var(--bs-tertiary-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  overflow: hidden;
}

.notes-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem 1rem;
  cursor: pointer;
  font-weight: 600;
  font-size: 0.9rem;
  color: var(--bs-body-color);
  transition: background 0.15s;
}

.notes-header:hover {
  background: var(--bs-secondary-bg);
}

.notes-count {
  margin-left: auto;
  font-size: 0.75rem;
  color: var(--bs-secondary-color);
  font-weight: normal;
}

.notes-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 1rem;
  padding: 1rem;
  border-top: 1px solid var(--bs-border-color);
}

.note-card {
  background: var(--bs-body-bg);
  border-radius: 8px;
  padding: 1rem;
}

.note-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}

.note-hop {
  font-weight: 600;
  color: var(--bs-primary);
}

.note-ip {
  font-size: 0.8rem;
  color: var(--bs-secondary-color);
  font-family: var(--bs-font-monospace);
}

.note-list {
  margin: 0;
  padding-left: 1.25rem;
  font-size: 0.8rem;
  color: var(--bs-body-color);
}

.note-list li {
  margin-bottom: 0.25rem;
}

/* Expected vs Actual */
.expected-actual-panel {
  background: var(--bs-tertiary-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  overflow: hidden;
}

.expected-actual-panel .panel-header {
  background: var(--bs-secondary-bg);
  color: var(--bs-body-color);
}

.comparison-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1px;
  background: var(--bs-border-color);
}

.comparison-column {
  padding: 1rem;
  background: var(--bs-body-bg);
}

.comparison-label {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  margin-bottom: 0.5rem;
}

.comparison-column.expected .comparison-label {
  color: var(--bs-success);
}

.comparison-column.actual .comparison-label {
  color: var(--bs-warning);
}

.comparison-column p {
  margin: 0;
  font-size: 0.85rem;
  line-height: 1.5;
  color: var(--bs-body-color);
}

.mismatch-reasons {
  padding: 1rem;
  border-top: 1px solid var(--bs-border-color);
  background: rgba(var(--bs-warning-rgb), 0.05);
}

.reasons-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 600;
  color: var(--bs-warning);
  margin-bottom: 0.75rem;
  font-size: 0.85rem;
}

.mismatch-reasons ul {
  margin: 0;
  padding-left: 1.5rem;
  font-size: 0.85rem;
  color: var(--bs-body-color);
}

.mismatch-reasons li {
  margin-bottom: 0.5rem;
}

@media (max-width: 768px) {
  .comparison-grid {
    grid-template-columns: 1fr;
  }
  
  .notes-grid {
    grid-template-columns: 1fr;
  }
}
</style>
