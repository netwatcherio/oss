<script lang="ts" setup>
import { ref, onMounted, reactive, computed } from "vue";
import { AlertRuleService, type AlertRule, type AlertRuleInput } from "@/services/apiService";

const props = defineProps<{
  workspaceId: number | string;
}>();

const emit = defineEmits<{
  (e: 'updated'): void;
}>();

const state = reactive({
  rules: [] as AlertRule[],
  loading: true,
  saving: false,
  error: null as string | null,
  showForm: false,
  editingRule: null as AlertRule | null,
});

const form = reactive<AlertRuleInput & { hasSecondCondition?: boolean }>({
  name: '',
  description: '',
  metric: 'packet_loss',
  operator: 'gt',
  threshold: 1,
  severity: 'warning',
  // Compound condition fields
  metric2: undefined,
  operator2: undefined,
  threshold2: undefined,
  logical_op: 'OR',
  hasSecondCondition: false,
  enabled: true,
  notify_panel: true,
  notify_email: false,
  notify_webhook: false,
  webhook_url: '',
});

const metrics = [
  { value: 'packet_loss', label: 'Packet Loss (%)', unit: '%', category: 'general', icon: 'bi-reception-4' },
  { value: 'latency', label: 'Latency (ms)', unit: 'ms', category: 'general', icon: 'bi-clock-history' },
  { value: 'jitter', label: 'Jitter (ms)', unit: 'ms', category: 'general', icon: 'bi-activity' },
  { value: 'offline', label: 'Agent Offline (minutes)', unit: 'min', category: 'general', icon: 'bi-wifi-off' },
  // MTR-specific metrics
  { value: 'end_hop_loss', label: 'End Hop Loss (%)', unit: '%', category: 'mtr', icon: 'bi-signpost' },
  { value: 'end_hop_latency', label: 'End Hop Latency (ms)', unit: 'ms', category: 'mtr', icon: 'bi-signpost-2' },
  { value: 'worst_hop_loss', label: 'Worst Hop Loss (%)', unit: '%', category: 'mtr', icon: 'bi-exclamation-diamond' },
  { value: 'route_change', label: 'Route Change', unit: '', category: 'mtr', icon: 'bi-shuffle' },
  // SYSINFO metrics
  { value: 'cpu_usage', label: 'CPU Usage (%)', unit: '%', category: 'sysinfo', icon: 'bi-cpu' },
  { value: 'memory_usage', label: 'Memory Usage (%)', unit: '%', category: 'sysinfo', icon: 'bi-memory' },
  // AI Analysis metrics (workspace-level)
  { value: 'health_score', label: 'Health Score (0-100)', unit: '', category: 'analysis', icon: 'bi-heart-pulse' },
  { value: 'latency_baseline', label: 'Latency Regression', unit: '', category: 'analysis', icon: 'bi-graph-up-arrow' },
  { value: 'loss_baseline', label: 'Loss Regression', unit: '', category: 'analysis', icon: 'bi-graph-down-arrow' },
  { value: 'ip_change', label: 'IP Address Change', unit: '', category: 'analysis', icon: 'bi-globe2' },
  { value: 'isp_change', label: 'ISP Provider Change', unit: '', category: 'analysis', icon: 'bi-building' },
  { value: 'incident_count', label: 'Active Incidents', unit: '', category: 'analysis', icon: 'bi-lightning-charge' },
];

const operators = [
  { value: 'gt', label: '>' },
  { value: 'gte', label: '≥' },
  { value: 'lt', label: '<' },
  { value: 'lte', label: '≤' },
  { value: 'eq', label: '=' },
];

const logicalOperators = [
  { value: 'OR', label: 'OR' },
  { value: 'AND', label: 'AND' },
];

const isFormValid = computed(() => {
  return form.name.trim().length > 0 && form.threshold >= 0;
});

const selectedMetricUnit = computed(() => {
  return metrics.find(m => m.value === form.metric)?.unit || '';
});

async function loadRules() {
  try {
    state.loading = true;
    state.rules = await AlertRuleService.list(props.workspaceId);
  } catch (e: any) {
    state.error = e.message || 'Failed to load alert rules';
  } finally {
    state.loading = false;
  }
}

function openCreateForm() {
  state.editingRule = null;
  Object.assign(form, {
    name: '',
    description: '',
    metric: 'packet_loss',
    operator: 'gt',
    threshold: 1,
    severity: 'warning',
    metric2: undefined,
    operator2: undefined,
    threshold2: undefined,
    logical_op: 'OR',
    hasSecondCondition: false,
    enabled: true,
    notify_panel: true,
    notify_email: false,
    notify_webhook: false,
    webhook_url: '',
  });
  state.showForm = true;
}

function openEditForm(rule: AlertRule) {
  state.editingRule = rule;
  const hasCompound = !!rule.metric2;
  Object.assign(form, {
    name: rule.name,
    description: rule.description || '',
    metric: rule.metric,
    operator: rule.operator,
    threshold: rule.threshold,
    severity: rule.severity,
    metric2: rule.metric2,
    operator2: rule.operator2,
    threshold2: rule.threshold2,
    logical_op: rule.logical_op || 'OR',
    hasSecondCondition: hasCompound,
    enabled: rule.enabled,
    notify_panel: rule.notify_panel,
    notify_email: rule.notify_email,
    notify_webhook: rule.notify_webhook,
    webhook_url: rule.webhook_url || '',
  });
  state.showForm = true;
}

function closeForm() {
  state.showForm = false;
  state.editingRule = null;
}

async function saveRule() {
  if (!isFormValid.value) return;
  
  try {
    state.saving = true;
    if (state.editingRule) {
      await AlertRuleService.update(props.workspaceId, state.editingRule.id, form);
    } else {
      await AlertRuleService.create(props.workspaceId, form);
    }
    await loadRules();
    closeForm();
    emit('updated');
  } catch (e: any) {
    state.error = e.message || 'Failed to save rule';
  } finally {
    state.saving = false;
  }
}

async function deleteRule(rule: AlertRule) {
  if (!confirm(`Delete rule "${rule.name}"?`)) return;
  
  try {
    await AlertRuleService.remove(props.workspaceId, rule.id);
    await loadRules();
    emit('updated');
  } catch (e: any) {
    state.error = e.message || 'Failed to delete rule';
  }
}

async function toggleEnabled(rule: AlertRule) {
  try {
    await AlertRuleService.update(props.workspaceId, rule.id, { enabled: !rule.enabled });
    rule.enabled = !rule.enabled;
  } catch (e: any) {
    state.error = e.message || 'Failed to update rule';
  }
}

function getMetricLabel(metric: string) {
  return metrics.find(m => m.value === metric)?.label || metric;
}

function getOperatorLabel(op: string) {
  return operators.find(o => o.value === op)?.label || op;
}

function getMetricIcon(metric: string) {
  return metrics.find(m => m.value === metric)?.icon || 'bi-graph-up';
}

onMounted(loadRules);
</script>

<template>
  <div class="alert-rules-config">
    <div class="d-flex justify-content-end mb-3">
      <button class="btn btn-primary btn-sm" @click="openCreateForm">
        <i class="bi bi-plus-lg me-1"></i>Add Rule
      </button>
    </div>

    <!-- Loading -->
    <div v-if="state.loading" class="text-center py-4">
      <div class="spinner-border spinner-border-sm text-primary"></div>
    </div>

    <!-- Error -->
    <div v-else-if="state.error" class="alert alert-danger">
      {{ state.error }}
      <button class="btn-close float-end" @click="state.error = null"></button>
    </div>

    <!-- Empty State -->
    <div v-else-if="state.rules.length === 0 && !state.showForm" class="empty-state">
      <div class="empty-state-icon">
        <i class="bi bi-bell-slash"></i>
      </div>
      <h6 class="empty-state-title">No alert rules configured</h6>
      <p class="empty-state-text">
        Get notified when your agents experience issues like high packet loss, latency spikes, or go offline.
      </p>
      <button class="btn btn-primary" @click="openCreateForm">
        <i class="bi bi-plus-lg me-1"></i>Create your first rule
      </button>
      <div class="empty-state-suggestions">
        <span class="suggestion-label">Popular alerts:</span>
        <span class="suggestion-tag">Packet Loss > 5%</span>
        <span class="suggestion-tag">Latency > 100ms</span>
        <span class="suggestion-tag">Agent Offline</span>
      </div>
    </div>

    <!-- Rules List -->
    <div v-else class="rules-list">
      <div 
        v-for="rule in state.rules" 
        :key="rule.id" 
        class="rule-card" 
        :class="{ 
          disabled: !rule.enabled,
          'severity-warning': rule.severity === 'warning',
          'severity-critical': rule.severity === 'critical'
        }"
      >
        <div class="d-flex justify-content-between align-items-start">
          <div class="flex-grow-1">
            <div class="d-flex align-items-center gap-2 mb-1">
              <i :class="getMetricIcon(rule.metric)" class="rule-metric-icon"></i>
              <strong>{{ rule.name }}</strong>
              <span class="badge" :class="rule.severity === 'critical' ? 'bg-danger' : 'bg-warning text-dark'">
                <i :class="rule.severity === 'critical' ? 'bi bi-exclamation-octagon-fill' : 'bi bi-exclamation-triangle-fill'" class="me-1"></i>
                {{ rule.severity }}
              </span>
              <span v-if="!rule.enabled" class="badge bg-secondary">
                <i class="bi bi-pause-fill me-1"></i>Disabled
              </span>
              <span v-if="rule.metric2" class="badge bg-info">
                <i class="bi bi-layers me-1"></i>Compound
              </span>
            </div>
            <div class="rule-condition">
              <code>{{ getMetricLabel(rule.metric) }} {{ getOperatorLabel(rule.operator) }} {{ rule.threshold }}</code>
              <template v-if="rule.metric2">
                <span class="logical-badge" :class="rule.logical_op === 'AND' ? 'and' : 'or'">{{ rule.logical_op }}</span>
                <code>{{ getMetricLabel(rule.metric2) }} {{ getOperatorLabel(rule.operator2 || '') }} {{ rule.threshold2 }}</code>
              </template>
            </div>
            <div class="mt-2 rule-channels">
              <span v-if="rule.notify_panel" class="channel-badge panel">
                <i class="bi bi-display"></i> Panel
              </span>
              <span v-if="rule.notify_email" class="channel-badge email">
                <i class="bi bi-envelope"></i> Email
              </span>
              <span v-if="rule.notify_webhook" class="channel-badge webhook">
                <i class="bi bi-webhook"></i> Webhook
              </span>
            </div>
          </div>
          <div class="btn-group btn-group-sm">
            <button class="btn btn-outline-secondary" @click="toggleEnabled(rule)" :title="rule.enabled ? 'Disable' : 'Enable'">
              <i :class="rule.enabled ? 'bi bi-pause' : 'bi bi-play'"></i>
            </button>
            <button class="btn btn-outline-secondary" @click="openEditForm(rule)" title="Edit">
              <i class="bi bi-pencil"></i>
            </button>
            <button class="btn btn-outline-danger" @click="deleteRule(rule)" title="Delete">
              <i class="bi bi-trash"></i>
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Form Modal -->
    <Transition name="modal">
      <div v-if="state.showForm" class="rule-form-overlay" @click.self="closeForm">
        <div class="rule-form-modal">
          <div class="modal-header">
            <h6 class="modal-title">
              <i :class="state.editingRule ? 'bi bi-pencil-square' : 'bi bi-plus-circle'" class="me-2"></i>
              {{ state.editingRule ? 'Edit' : 'Create' }} Alert Rule
            </h6>
            <button class="btn-close btn-close-white" @click="closeForm"></button>
          </div>
          <div class="modal-body">
            <!-- Name -->
            <div class="form-section">
              <div class="form-section-header">
                <i class="bi bi-tag"></i>
                <span>Rule Identity</span>
              </div>
              <div class="mb-3">
                <label class="form-label">Rule Name <span class="text-danger">*</span></label>
                <input v-model="form.name" type="text" class="form-control" placeholder="e.g., High Packet Loss Alert">
                <div class="form-text">Give your rule a descriptive name for easy identification.</div>
              </div>
            </div>

            <!-- Condition Section -->
            <div class="form-section">
              <div class="form-section-header">
                <i class="bi bi-sliders"></i>
                <span>Trigger Condition</span>
              </div>
              
              <div class="condition-box primary">
                <div class="row g-2">
                  <div class="col-5">
                    <label class="form-label">Metric</label>
                    <select v-model="form.metric" class="form-select">
                      <optgroup label="📊 General">
                        <option v-for="m in metrics.filter(m => m.category === 'general')" :key="m.value" :value="m.value">{{ m.label }}</option>
                      </optgroup>
                      <optgroup label="🛤️ MTR (Traceroute)">
                        <option v-for="m in metrics.filter(m => m.category === 'mtr')" :key="m.value" :value="m.value">{{ m.label }}</option>
                      </optgroup>
                      <optgroup label="💻 System Resources">
                        <option v-for="m in metrics.filter(m => m.category === 'sysinfo')" :key="m.value" :value="m.value">{{ m.label }}</option>
                      </optgroup>
                      <optgroup label="🤖 AI Analysis">
                        <option v-for="m in metrics.filter(m => m.category === 'analysis')" :key="m.value" :value="m.value">{{ m.label }}</option>
                      </optgroup>
                    </select>
                  </div>
                  <div class="col-3">
                    <label class="form-label">Operator</label>
                    <select v-model="form.operator" class="form-select">
                      <option v-for="o in operators" :key="o.value" :value="o.value">{{ o.label }}</option>
                    </select>
                  </div>
                  <div class="col-4">
                    <label class="form-label">Threshold</label>
                    <div class="input-group">
                      <input v-model.number="form.threshold" type="number" class="form-control" step="0.1" min="0">
                      <span v-if="selectedMetricUnit" class="input-group-text">{{ selectedMetricUnit }}</span>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Compound Condition Toggle -->
              <div class="compound-toggle">
                <button 
                  type="button" 
                  class="btn btn-sm" 
                  :class="form.hasSecondCondition ? 'btn-outline-primary active' : 'btn-outline-secondary'"
                  @click="form.hasSecondCondition = !form.hasSecondCondition; if (!form.hasSecondCondition) { form.metric2 = undefined; form.operator2 = undefined; form.threshold2 = undefined; }"
                >
                  <i class="bi bi-plus-circle me-1"></i>
                  {{ form.hasSecondCondition ? 'Remove second condition' : 'Add second condition' }}
                </button>
              </div>

              <!-- Secondary Condition -->
              <Transition name="slide">
                <div v-if="form.hasSecondCondition" class="compound-section">
                  <div class="logical-op-toggle">
                    <button 
                      type="button" 
                      class="btn btn-sm" 
                      :class="form.logical_op === 'AND' ? 'btn-primary' : 'btn-outline-secondary'"
                      @click="form.logical_op = 'AND'"
                    >
                      AND
                    </button>
                    <button 
                      type="button" 
                      class="btn btn-sm" 
                      :class="form.logical_op === 'OR' ? 'btn-primary' : 'btn-outline-secondary'"
                      @click="form.logical_op = 'OR'"
                    >
                      OR
                    </button>
                  </div>
                  <div class="condition-box secondary">
                    <div class="row g-2">
                      <div class="col-5">
                        <label class="form-label small text-muted">Second Metric</label>
                        <select v-model="form.metric2" class="form-select form-select-sm">
                          <optgroup label="📊 General">
                            <option v-for="m in metrics.filter(m => m.category === 'general')" :key="m.value" :value="m.value">{{ m.label }}</option>
                          </optgroup>
                          <optgroup label="🛤️ MTR (Traceroute)">
                            <option v-for="m in metrics.filter(m => m.category === 'mtr')" :key="m.value" :value="m.value">{{ m.label }}</option>
                          </optgroup>
                          <optgroup label="💻 System Resources">
                            <option v-for="m in metrics.filter(m => m.category === 'sysinfo')" :key="m.value" :value="m.value">{{ m.label }}</option>
                          </optgroup>
                          <optgroup label="🤖 AI Analysis">
                            <option v-for="m in metrics.filter(m => m.category === 'analysis')" :key="m.value" :value="m.value">{{ m.label }}</option>
                          </optgroup>
                        </select>
                      </div>
                      <div class="col-3">
                        <label class="form-label small text-muted">Operator</label>
                        <select v-model="form.operator2" class="form-select form-select-sm">
                          <option v-for="o in operators" :key="o.value" :value="o.value">{{ o.label }}</option>
                        </select>
                      </div>
                      <div class="col-4">
                        <label class="form-label small text-muted">Threshold</label>
                        <input v-model.number="form.threshold2" type="number" class="form-control form-control-sm" step="0.1" min="0">
                      </div>
                    </div>
                  </div>
                  <div class="compound-hint">
                    <i class="bi bi-info-circle"></i>
                    {{ form.logical_op === 'OR' ? 'Alert triggers if EITHER condition is met' : 'Alert triggers if BOTH conditions are met' }}
                  </div>
                </div>
              </Transition>
            </div>

            <!-- Severity Section -->
            <div class="form-section">
              <div class="form-section-header">
                <i class="bi bi-shield-exclamation"></i>
                <span>Severity Level</span>
              </div>
              <div class="severity-cards">
                <div 
                  class="severity-card warning" 
                  :class="{ active: form.severity === 'warning' }"
                  @click="form.severity = 'warning'"
                >
                  <div class="severity-icon">
                    <i class="bi bi-exclamation-triangle-fill"></i>
                  </div>
                  <div class="severity-content">
                    <div class="severity-label">Warning</div>
                    <div class="severity-desc">Non-critical issue that needs attention</div>
                  </div>
                  <div class="severity-check" v-if="form.severity === 'warning'">
                    <i class="bi bi-check-circle-fill"></i>
                  </div>
                </div>
                <div 
                  class="severity-card critical" 
                  :class="{ active: form.severity === 'critical' }"
                  @click="form.severity = 'critical'"
                >
                  <div class="severity-icon">
                    <i class="bi bi-exclamation-octagon-fill"></i>
                  </div>
                  <div class="severity-content">
                    <div class="severity-label">Critical</div>
                    <div class="severity-desc">Urgent issue requiring immediate action</div>
                  </div>
                  <div class="severity-check" v-if="form.severity === 'critical'">
                    <i class="bi bi-check-circle-fill"></i>
                  </div>
                </div>
              </div>
            </div>

            <!-- Notification Channels -->
            <div class="form-section">
              <div class="form-section-header">
                <i class="bi bi-megaphone"></i>
                <span>Notification Channels</span>
              </div>
              <div class="notification-channels">
                <div class="channel-card" :class="{ active: true, disabled: true }">
                  <div class="channel-icon panel">
                    <i class="bi bi-display"></i>
                  </div>
                  <div class="channel-content">
                    <div class="channel-label">Panel</div>
                    <div class="channel-desc">Always enabled</div>
                  </div>
                  <div class="channel-toggle">
                    <i class="bi bi-check-circle-fill text-success"></i>
                  </div>
                </div>
                <div 
                  class="channel-card" 
                  :class="{ active: form.notify_email }"
                  @click="form.notify_email = !form.notify_email"
                >
                  <div class="channel-icon email">
                    <i class="bi bi-envelope"></i>
                  </div>
                  <div class="channel-content">
                    <div class="channel-label">Email</div>
                    <div class="channel-desc">Notify workspace members</div>
                  </div>
                  <div class="channel-toggle">
                    <i :class="form.notify_email ? 'bi bi-check-circle-fill text-success' : 'bi bi-circle text-muted'"></i>
                  </div>
                </div>
                <div 
                  class="channel-card" 
                  :class="{ active: form.notify_webhook }"
                  @click="form.notify_webhook = !form.notify_webhook"
                >
                  <div class="channel-icon webhook">
                    <i class="bi bi-webhook"></i>
                  </div>
                  <div class="channel-content">
                    <div class="channel-label">Webhook</div>
                    <div class="channel-desc">Send to external service</div>
                  </div>
                  <div class="channel-toggle">
                    <i :class="form.notify_webhook ? 'bi bi-check-circle-fill text-success' : 'bi bi-circle text-muted'"></i>
                  </div>
                </div>
              </div>

              <!-- Webhook URL (conditional) -->
              <Transition name="slide">
                <div v-if="form.notify_webhook" class="webhook-url-input">
                  <label class="form-label">Webhook URL</label>
                  <input v-model="form.webhook_url" type="url" class="form-control" placeholder="https://example.com/webhook">
                </div>
              </Transition>
            </div>

            <!-- Description -->
            <div class="form-section last">
              <div class="form-section-header">
                <i class="bi bi-text-paragraph"></i>
                <span>Description (optional)</span>
              </div>
              <textarea v-model="form.description" class="form-control" rows="2" placeholder="Add notes or context for this alert rule..."></textarea>
            </div>
          </div>
          <div class="modal-footer">
            <button class="btn btn-secondary" @click="closeForm" :disabled="state.saving">
              Cancel
            </button>
            <button class="btn btn-primary" @click="saveRule" :disabled="!isFormValid || state.saving">
              <span v-if="state.saving" class="spinner-border spinner-border-sm me-1"></span>
              <i v-else :class="state.editingRule ? 'bi bi-check-lg' : 'bi bi-plus-lg'" class="me-1"></i>
              {{ state.editingRule ? 'Update Rule' : 'Create Rule' }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
/* Base Layout */
.rules-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

/* Empty State */
.empty-state {
  text-align: center;
  padding: 2.5rem 1rem;
  background: linear-gradient(135deg, var(--bg-secondary, #f8f9fa) 0%, var(--bg-card, #fff) 100%);
  border-radius: 1rem;
  border: 2px dashed var(--border-color, #e9ecef);
}

.empty-state-icon {
  font-size: 3rem;
  color: var(--bs-primary);
  opacity: 0.6;
  margin-bottom: 1rem;
}

.empty-state-title {
  color: var(--text-primary, #212529);
  margin-bottom: 0.5rem;
}

.empty-state-text {
  color: var(--text-muted, #6c757d);
  max-width: 320px;
  margin: 0 auto 1.5rem;
  font-size: 0.9rem;
}

.empty-state-suggestions {
  margin-top: 1.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.suggestion-label {
  font-size: 0.75rem;
  color: var(--text-muted, #6c757d);
}

.suggestion-tag {
  font-size: 0.7rem;
  padding: 0.25rem 0.5rem;
  background: var(--bg-card, #fff);
  border: 1px solid var(--border-color, #e9ecef);
  border-radius: 1rem;
  color: var(--text-muted, #6c757d);
}

/* Rule Cards */
.rule-card {
  background: var(--bg-card, white);
  border: 1px solid var(--border-color, #e9ecef);
  border-radius: 0.75rem;
  padding: 1rem 1.25rem;
  transition: all 0.2s ease;
  position: relative;
  overflow: hidden;
}

.rule-card::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 4px;
  background: var(--border-color, #e9ecef);
  transition: background 0.2s ease;
}

.rule-card.severity-warning::before {
  background: var(--bs-warning);
}

.rule-card.severity-critical::before {
  background: var(--bs-danger);
}

.rule-card:hover {
  border-color: var(--bs-primary);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
  transform: translateY(-1px);
}

.rule-card.disabled {
  opacity: 0.6;
}

.rule-metric-icon {
  color: var(--bs-primary);
  font-size: 1.1rem;
}

.rule-condition {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
  margin-top: 0.25rem;
}

.rule-condition code {
  font-size: 0.8rem;
  padding: 0.2rem 0.5rem;
  background: var(--bg-secondary, #f8f9fa);
  border-radius: 0.25rem;
  color: var(--text-primary, #212529);
}

.logical-badge {
  font-size: 0.65rem;
  font-weight: 700;
  padding: 0.15rem 0.4rem;
  border-radius: 0.25rem;
  text-transform: uppercase;
}

.logical-badge.and {
  background: var(--bs-primary);
  color: white;
}

.logical-badge.or {
  background: var(--bs-info);
  color: white;
}

.rule-channels {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.channel-badge {
  font-size: 0.7rem;
  padding: 0.2rem 0.5rem;
  border-radius: 0.25rem;
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.channel-badge.panel {
  background: rgba(var(--bs-primary-rgb), 0.1);
  color: var(--bs-primary);
}

.channel-badge.email {
  background: rgba(var(--bs-success-rgb), 0.1);
  color: var(--bs-success);
}

.channel-badge.webhook {
  background: rgba(var(--bs-warning-rgb), 0.1);
  color: var(--bs-warning);
}

/* Modal Styles */
.rule-form-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1050;
  padding: 1rem;
}

.rule-form-modal {
  background: var(--bg-card, white);
  border-radius: 1rem;
  width: 100%;
  max-width: 540px;
  max-height: 90vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.3);
}

.modal-header {
  padding: 1.25rem 1.5rem;
  background: linear-gradient(135deg, var(--bs-primary) 0%, #4a9eff 100%);
  color: white;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.modal-title {
  margin: 0;
  font-weight: 600;
  display: flex;
  align-items: center;
}

.modal-body {
  padding: 0;
  overflow-y: auto;
  flex: 1;
}

.modal-footer {
  padding: 1rem 1.5rem;
  border-top: 1px solid var(--border-color, #e9ecef);
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
  background: var(--bg-secondary, #f8f9fa);
}

/* Form Sections */
.form-section {
  padding: 1.25rem 1.5rem;
  border-bottom: 1px solid var(--border-color, #e9ecef);
}

.form-section.last {
  border-bottom: none;
}

.form-section-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 1rem;
  font-weight: 600;
  color: var(--text-primary, #212529);
  font-size: 0.9rem;
}

.form-section-header i {
  color: var(--bs-primary);
}

/* Condition Boxes */
.condition-box {
  background: var(--bg-secondary, #f8f9fa);
  border-radius: 0.75rem;
  padding: 1rem;
  border: 1px solid var(--border-color, #e9ecef);
}

.condition-box.secondary {
  border-left: 3px solid var(--bs-primary);
}

.compound-toggle {
  margin-top: 0.75rem;
  text-align: center;
}

.compound-section {
  margin-top: 0.75rem;
}

.logical-op-toggle {
  display: flex;
  justify-content: center;
  gap: 0.25rem;
  margin-bottom: 0.75rem;
}

.logical-op-toggle .btn {
  min-width: 60px;
  font-weight: 600;
}

.compound-hint {
  margin-top: 0.75rem;
  font-size: 0.8rem;
  color: var(--text-muted, #6c757d);
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  background: rgba(var(--bs-info-rgb), 0.1);
  border-radius: 0.5rem;
}

/* Severity Cards */
.severity-cards {
  display: flex;
  gap: 0.75rem;
}

.severity-card {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.875rem 1rem;
  border-radius: 0.75rem;
  border: 2px solid var(--border-color, #e9ecef);
  cursor: pointer;
  transition: all 0.2s ease;
  position: relative;
}

.severity-card:hover {
  border-color: var(--border-color, #dee2e6);
}

.severity-card.warning.active {
  border-color: var(--bs-warning);
  background: rgba(var(--bs-warning-rgb), 0.08);
}

.severity-card.critical.active {
  border-color: var(--bs-danger);
  background: rgba(var(--bs-danger-rgb), 0.08);
}

.severity-icon {
  font-size: 1.5rem;
}

.severity-card.warning .severity-icon {
  color: var(--bs-warning);
}

.severity-card.critical .severity-icon {
  color: var(--bs-danger);
}

.severity-content {
  flex: 1;
}

.severity-label {
  font-weight: 600;
  font-size: 0.9rem;
}

.severity-desc {
  font-size: 0.75rem;
  color: var(--text-muted, #6c757d);
}

.severity-check {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  font-size: 1rem;
}

.severity-card.warning .severity-check {
  color: var(--bs-warning);
}

.severity-card.critical .severity-check {
  color: var(--bs-danger);
}

/* Notification Channels */
.notification-channels {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.channel-card {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem 1rem;
  border-radius: 0.5rem;
  border: 1px solid var(--border-color, #e9ecef);
  cursor: pointer;
  transition: all 0.2s ease;
}

.channel-card:not(.disabled):hover {
  border-color: var(--bs-primary);
  background: rgba(var(--bs-primary-rgb), 0.03);
}

.channel-card.active:not(.disabled) {
  border-color: var(--bs-primary);
  background: rgba(var(--bs-primary-rgb), 0.05);
}

.channel-card.disabled {
  cursor: default;
  opacity: 0.7;
}

.channel-icon {
  width: 36px;
  height: 36px;
  border-radius: 0.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1rem;
}

.channel-icon.panel {
  background: rgba(var(--bs-primary-rgb), 0.1);
  color: var(--bs-primary);
}

.channel-icon.email {
  background: rgba(var(--bs-success-rgb), 0.1);
  color: var(--bs-success);
}

.channel-icon.webhook {
  background: rgba(var(--bs-warning-rgb), 0.1);
  color: var(--bs-warning);
}

.channel-content {
  flex: 1;
}

.channel-label {
  font-weight: 500;
  font-size: 0.9rem;
}

.channel-desc {
  font-size: 0.75rem;
  color: var(--text-muted, #6c757d);
}

.webhook-url-input {
  margin-top: 0.75rem;
  padding: 0.75rem;
  background: var(--bg-secondary, #f8f9fa);
  border-radius: 0.5rem;
}

/* Transitions */
.modal-enter-active {
  animation: modalIn 0.25s ease-out;
}

.modal-leave-active {
  animation: modalOut 0.2s ease-in;
}

@keyframes modalIn {
  from {
    opacity: 0;
  }
  from .rule-form-modal {
    transform: scale(0.95) translateY(10px);
    opacity: 0;
  }
  to {
    opacity: 1;
  }
  to .rule-form-modal {
    transform: scale(1) translateY(0);
    opacity: 1;
  }
}

@keyframes modalOut {
  from {
    opacity: 1;
  }
  to {
    opacity: 0;
  }
}

.slide-enter-active,
.slide-leave-active {
  transition: all 0.25s ease;
}

.slide-enter-from,
.slide-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}

/* Dark mode */
:global([data-theme="dark"]) .rule-card {
  background: #1f2937;
  border-color: #374151;
}

:global([data-theme="dark"]) .rule-form-modal {
  background: #1f2937;
}

:global([data-theme="dark"]) .modal-footer {
  background: #111827;
  border-color: #374151;
}

:global([data-theme="dark"]) .form-section {
  border-color: #374151;
}

:global([data-theme="dark"]) .form-section-header {
  color: #f1f5f9;
}

:global([data-theme="dark"]) .form-section-header i {
  color: #60a5fa;
}

:global([data-theme="dark"]) .form-label {
  color: #e5e7eb;
}

:global([data-theme="dark"]) .form-text {
  color: #9ca3af !important;
}

:global([data-theme="dark"]) .form-control,
:global([data-theme="dark"]) .form-select {
  background: #374151;
  border-color: #4b5563;
  color: #f1f5f9;
}

:global([data-theme="dark"]) .form-control::placeholder {
  color: #9ca3af;
}

:global([data-theme="dark"]) .form-control:focus,
:global([data-theme="dark"]) .form-select:focus {
  background: #374151;
  border-color: #3b82f6;
  color: #f1f5f9;
}

:global([data-theme="dark"]) .input-group-text {
  background: #4b5563;
  border-color: #4b5563;
  color: #e5e7eb;
}

:global([data-theme="dark"]) .condition-box {
  background: #111827;
  border-color: #374151;
}

:global([data-theme="dark"]) .condition-box.secondary {
  border-left-color: #3b82f6;
}

:global([data-theme="dark"]) .compound-toggle .btn-outline-secondary {
  border-color: #4b5563;
  color: #9ca3af;
}

:global([data-theme="dark"]) .compound-toggle .btn-outline-secondary:hover,
:global([data-theme="dark"]) .compound-toggle .btn-outline-primary.active {
  background: #3b82f6;
  border-color: #3b82f6;
  color: white;
}

:global([data-theme="dark"]) .compound-hint {
  background: rgba(59, 130, 246, 0.15);
  color: #93c5fd;
}

:global([data-theme="dark"]) .logical-op-toggle .btn-outline-secondary {
  border-color: #4b5563;
  color: #9ca3af;
}

/* Severity Cards - Dark Mode */
:global([data-theme="dark"]) .severity-card {
  border-color: #374151;
  background: #111827;
}

:global([data-theme="dark"]) .severity-card:hover {
  border-color: #4b5563;
}

:global([data-theme="dark"]) .severity-card.warning.active {
  border-color: #f59e0b;
  background: rgba(245, 158, 11, 0.1);
}

:global([data-theme="dark"]) .severity-card.critical.active {
  border-color: #ef4444;
  background: rgba(239, 68, 68, 0.1);
}

:global([data-theme="dark"]) .severity-label {
  color: #f1f5f9;
}

:global([data-theme="dark"]) .severity-desc {
  color: #9ca3af;
}

/* Notification Channels - Dark Mode */
:global([data-theme="dark"]) .channel-card {
  border-color: #374151;
  background: #111827;
}

:global([data-theme="dark"]) .channel-card:not(.disabled):hover {
  border-color: #4b5563;
  background: #1f2937;
}

:global([data-theme="dark"]) .channel-card.active:not(.disabled) {
  border-color: #3b82f6;
  background: rgba(59, 130, 246, 0.1);
}

:global([data-theme="dark"]) .channel-label {
  color: #f1f5f9;
}

:global([data-theme="dark"]) .channel-desc {
  color: #9ca3af;
}

:global([data-theme="dark"]) .webhook-url-input {
  background: #111827;
}

/* Empty State - Dark Mode */
:global([data-theme="dark"]) .empty-state {
  background: linear-gradient(135deg, #1f2937 0%, #111827 100%);
  border-color: #374151;
}

:global([data-theme="dark"]) .empty-state-title {
  color: #f1f5f9;
}

:global([data-theme="dark"]) .empty-state-text {
  color: #9ca3af;
}

:global([data-theme="dark"]) .suggestion-tag {
  background: #374151;
  border-color: #4b5563;
  color: #d1d5db;
}

:global([data-theme="dark"]) .suggestion-label {
  color: #9ca3af;
}

/* Rule Cards - Dark Mode */
:global([data-theme="dark"]) .rule-condition code {
  background: #374151;
  color: #e5e7eb;
}

:global([data-theme="dark"]) .rule-metric-icon {
  color: #60a5fa;
}

:global([data-theme="dark"]) .channel-badge.panel {
  background: rgba(59, 130, 246, 0.2);
}

:global([data-theme="dark"]) .channel-badge.email {
  background: rgba(34, 197, 94, 0.2);
}

:global([data-theme="dark"]) .channel-badge.webhook {
  background: rgba(234, 179, 8, 0.2);
}
</style>

<!-- Unscoped dark mode styles for modal form elements -->
<style>
[data-theme="dark"] .rule-form-modal .condition-box {
  background: #111827 !important;
  border-color: #374151 !important;
}

[data-theme="dark"] .rule-form-modal .condition-box.primary {
  background: #111827 !important;
}

[data-theme="dark"] .rule-form-modal .form-control,
[data-theme="dark"] .rule-form-modal .form-select {
  background: #374151 !important;
  border-color: #4b5563 !important;
  color: #f1f5f9 !important;
}

[data-theme="dark"] .rule-form-modal .form-control::placeholder {
  color: #9ca3af !important;
}

[data-theme="dark"] .rule-form-modal .form-label {
  color: #d1d5db !important;
}

[data-theme="dark"] .rule-form-modal .input-group-text {
  background: #4b5563 !important;
  border-color: #4b5563 !important;
  color: #e5e7eb !important;
}

[data-theme="dark"] .rule-form-modal .severity-card {
  background: #111827 !important;
  border-color: #374151 !important;
}

[data-theme="dark"] .rule-form-modal .severity-card.warning.active {
  background: rgba(245, 158, 11, 0.1) !important;
  border-color: #f59e0b !important;
}

[data-theme="dark"] .rule-form-modal .severity-card.critical.active {
  background: rgba(239, 68, 68, 0.1) !important;
  border-color: #ef4444 !important;
}

[data-theme="dark"] .rule-form-modal .severity-label {
  color: #f1f5f9 !important;
}

[data-theme="dark"] .rule-form-modal .severity-desc {
  color: #9ca3af !important;
}

[data-theme="dark"] .rule-form-modal .channel-card {
  background: #111827 !important;
  border-color: #374151 !important;
}

[data-theme="dark"] .rule-form-modal .channel-card.active:not(.disabled) {
  background: rgba(59, 130, 246, 0.1) !important;
  border-color: #3b82f6 !important;
}

[data-theme="dark"] .rule-form-modal .channel-label {
  color: #f1f5f9 !important;
}

[data-theme="dark"] .rule-form-modal .channel-desc {
  color: #9ca3af !important;
}

[data-theme="dark"] .rule-form-modal .compound-toggle .btn {
  background: transparent;
  border-color: #4b5563;
  color: #9ca3af;
}

[data-theme="dark"] .rule-form-modal .compound-toggle .btn:hover,
[data-theme="dark"] .rule-form-modal .compound-toggle .btn.active {
  background: #3b82f6;
  border-color: #3b82f6;
  color: white;
}

[data-theme="dark"] .rule-form-modal .logical-op-toggle .btn {
  border-color: #4b5563;
  color: #9ca3af;
}

[data-theme="dark"] .rule-form-modal .logical-op-toggle .btn.btn-primary {
  background: #3b82f6;
  border-color: #3b82f6;
  color: white;
}

[data-theme="dark"] .rule-form-modal .compound-hint {
  background: rgba(59, 130, 246, 0.15);
  color: #93c5fd;
}

[data-theme="dark"] .rule-form-modal .webhook-url-input {
  background: #111827;
}
</style>
