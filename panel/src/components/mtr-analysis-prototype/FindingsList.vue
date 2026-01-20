<template>
  <div class="findings-list">
    <!-- Summary Row -->
    <div class="findings-summary">
      <div 
        v-for="severity in ['critical', 'warning', 'info']" 
        :key="severity"
        class="severity-stat"
        :class="severity"
        v-show="getSeverityCount(severity) > 0"
      >
        <i :class="getSeverityIcon(severity)"></i>
        <span class="count">{{ getSeverityCount(severity) }}</span>
        <span class="label">{{ severity }}</span>
      </div>
    </div>

    <!-- Findings Cards -->
    <div class="findings-grid">
      <div 
        v-for="finding in sortedFindings" 
        :key="finding.id"
        class="finding-card"
        :class="finding.severity"
      >
        <div class="finding-header">
          <div class="finding-title-row">
            <span class="finding-id">{{ finding.id }}</span>
            <h4 class="finding-title">{{ finding.title }}</h4>
          </div>
          <div class="finding-badges">
            <span class="severity-badge" :class="finding.severity">
              <i :class="getSeverityIcon(finding.severity)"></i>
              {{ finding.severity }}
            </span>
            <span class="category-badge">
              <i :class="getCategoryIcon(finding.category)"></i>
              {{ formatCategory(finding.category) }}
            </span>
            <span class="confidence-badge" :title="'Confidence: ' + (finding.confidence * 100).toFixed(0) + '%'">
              <i class="bi bi-bullseye"></i>
              {{ (finding.confidence * 100).toFixed(0) }}%
            </span>
          </div>
        </div>

        <p class="finding-summary">{{ finding.summary }}</p>

        <!-- Evidence Section -->
        <div class="finding-section">
          <div class="section-header" @click="toggleSection(finding.id, 'evidence')">
            <i class="bi bi-search"></i>
            <span>Evidence</span>
            <i :class="isOpen(finding.id, 'evidence') ? 'bi bi-chevron-up' : 'bi bi-chevron-down'" class="toggle-icon"></i>
          </div>
          <div v-if="isOpen(finding.id, 'evidence')" class="section-content">
            <ul class="evidence-list">
              <li v-for="(item, idx) in finding.evidence" :key="idx">
                <code>{{ item }}</code>
              </li>
            </ul>
          </div>
        </div>

        <!-- Why It Matters -->
        <div class="finding-section">
          <div class="section-header" @click="toggleSection(finding.id, 'why')">
            <i class="bi bi-info-circle"></i>
            <span>Why It Matters</span>
            <i :class="isOpen(finding.id, 'why') ? 'bi bi-chevron-up' : 'bi bi-chevron-down'" class="toggle-icon"></i>
          </div>
          <div v-if="isOpen(finding.id, 'why')" class="section-content">
            <ul>
              <li v-for="(item, idx) in finding.why_it_matters" :key="idx">{{ item }}</li>
            </ul>
          </div>
        </div>

        <!-- Next Steps -->
        <div class="finding-section next-steps">
          <div class="section-header" @click="toggleSection(finding.id, 'steps')">
            <i class="bi bi-arrow-right-circle"></i>
            <span>Recommended Next Steps</span>
            <i :class="isOpen(finding.id, 'steps') ? 'bi bi-chevron-up' : 'bi bi-chevron-down'" class="toggle-icon"></i>
          </div>
          <div v-if="isOpen(finding.id, 'steps')" class="section-content">
            <ul>
              <li v-for="(step, idx) in finding.recommended_next_steps" :key="idx">{{ step }}</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, reactive } from 'vue';
import type { Finding } from './types';

const props = defineProps<{
  findings: Finding[];
}>();

// Open sections state
const openSections = reactive<Record<string, string[]>>({});

const sortedFindings = computed(() => {
  const severityOrder: Record<string, number> = { critical: 0, warning: 1, info: 2 };
  return [...props.findings].sort((a, b) => {
    const orderDiff = (severityOrder[a.severity] ?? 3) - (severityOrder[b.severity] ?? 3);
    if (orderDiff !== 0) return orderDiff;
    return b.confidence - a.confidence;
  });
});

function getSeverityCount(severity: string): number {
  return props.findings.filter(f => f.severity === severity).length;
}

function getSeverityIcon(severity: string): string {
  switch (severity) {
    case 'critical': return 'bi bi-exclamation-octagon-fill';
    case 'warning': return 'bi bi-exclamation-triangle-fill';
    case 'info': return 'bi bi-info-circle-fill';
    default: return 'bi bi-circle';
  }
}

function getCategoryIcon(category: string): string {
  switch (category) {
    case 'performance': return 'bi bi-speedometer2';
    case 'measurement_artifact': return 'bi bi-rulers';
    case 'routing_policy': return 'bi bi-signpost-split';
    default: return 'bi bi-tag';
  }
}

function formatCategory(category: string): string {
  return category.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase());
}

function toggleSection(findingId: string, section: string) {
  if (!openSections[findingId]) {
    openSections[findingId] = [];
  }
  const idx = openSections[findingId].indexOf(section);
  if (idx > -1) {
    openSections[findingId].splice(idx, 1);
  } else {
    openSections[findingId].push(section);
  }
}

function isOpen(findingId: string, section: string): boolean {
  return openSections[findingId]?.includes(section) ?? false;
}
</script>

<style scoped>
.findings-list {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

/* Summary Row */
.findings-summary {
  display: flex;
  gap: 1rem;
  flex-wrap: wrap;
}

.severity-stat {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 1rem;
  border-radius: 8px;
  font-size: 0.85rem;
}

.severity-stat.critical {
  background: rgba(var(--bs-danger-rgb), 0.1);
  border: 1px solid rgba(var(--bs-danger-rgb), 0.3);
  color: var(--bs-danger);
}

.severity-stat.warning {
  background: rgba(var(--bs-warning-rgb), 0.1);
  border: 1px solid rgba(var(--bs-warning-rgb), 0.3);
  color: var(--bs-warning);
}

.severity-stat.info {
  background: rgba(var(--bs-info-rgb), 0.1);
  border: 1px solid rgba(var(--bs-info-rgb), 0.3);
  color: var(--bs-info);
}

.severity-stat .count {
  font-weight: 700;
  font-size: 1.1rem;
}

.severity-stat .label {
  text-transform: capitalize;
}

/* Findings Grid */
.findings-grid {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.finding-card {
  background: var(--bs-tertiary-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 12px;
  overflow: hidden;
}

.finding-card.critical {
  border-left: 4px solid var(--bs-danger);
}

.finding-card.warning {
  border-left: 4px solid var(--bs-warning);
}

.finding-card.info {
  border-left: 4px solid var(--bs-info);
}

/* Finding Header */
.finding-header {
  padding: 1rem 1.25rem;
  background: var(--bs-secondary-bg);
  border-bottom: 1px solid var(--bs-border-color);
}

.finding-title-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 0.75rem;
}

.finding-id {
  font-size: 0.75rem;
  font-weight: 700;
  padding: 0.2rem 0.5rem;
  background: var(--bs-primary);
  color: white;
  border-radius: 4px;
}

.finding-title {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
  color: var(--bs-body-color);
}

.finding-badges {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.severity-badge, .category-badge, .confidence-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.2rem 0.6rem;
  border-radius: 4px;
  font-size: 0.7rem;
  font-weight: 600;
  text-transform: uppercase;
}

.severity-badge.critical {
  background: rgba(var(--bs-danger-rgb), 0.15);
  color: var(--bs-danger);
}

.severity-badge.warning {
  background: rgba(var(--bs-warning-rgb), 0.15);
  color: var(--bs-warning);
}

.severity-badge.info {
  background: rgba(var(--bs-info-rgb), 0.15);
  color: var(--bs-info);
}

.category-badge {
  background: var(--bs-body-bg);
  color: var(--bs-secondary-color);
}

.confidence-badge {
  background: rgba(var(--bs-success-rgb), 0.1);
  color: var(--bs-success);
}

/* Finding Summary */
.finding-summary {
  padding: 1rem 1.25rem;
  margin: 0;
  font-size: 0.9rem;
  line-height: 1.6;
  color: var(--bs-body-color);
  border-bottom: 1px solid var(--bs-border-color);
}

/* Finding Sections */
.finding-section {
  border-bottom: 1px solid var(--bs-border-color);
}

.finding-section:last-child {
  border-bottom: none;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem 1.25rem;
  cursor: pointer;
  font-size: 0.85rem;
  font-weight: 500;
  color: var(--bs-secondary-color);
  transition: all 0.15s;
}

.section-header:hover {
  background: var(--bs-secondary-bg);
  color: var(--bs-body-color);
}

.toggle-icon {
  margin-left: auto;
}

.section-content {
  padding: 0 1.25rem 1rem 1.25rem;
  background: var(--bs-body-bg);
}

.section-content ul {
  margin: 0;
  padding-left: 1.25rem;
  font-size: 0.85rem;
  color: var(--bs-body-color);
}

.section-content li {
  margin-bottom: 0.5rem;
}

.section-content li:last-child {
  margin-bottom: 0;
}

.evidence-list {
  list-style: none;
  padding: 0;
}

.evidence-list li {
  padding: 0.5rem;
  background: var(--bs-tertiary-bg);
  border-radius: 6px;
  margin-bottom: 0.5rem;
}

.evidence-list code {
  font-family: 'JetBrains Mono', var(--bs-font-monospace);
  font-size: 0.8rem;
  color: var(--bs-body-color);
  background: transparent;
}

/* Next Steps emphasis */
.finding-section.next-steps .section-header {
  color: var(--bs-primary);
}

.finding-section.next-steps .section-content li {
  color: var(--bs-primary);
}
</style>
