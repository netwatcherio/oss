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

const form = reactive<AlertRuleInput>({
  name: '',
  description: '',
  metric: 'packet_loss',
  operator: 'gt',
  threshold: 1,
  severity: 'warning',
  enabled: true,
  notify_panel: true,
  notify_email: false,
  notify_webhook: false,
  webhook_url: '',
});

const metrics = [
  { value: 'packet_loss', label: 'Packet Loss (%)', unit: '%' },
  { value: 'latency', label: 'Latency (ms)', unit: 'ms' },
  { value: 'jitter', label: 'Jitter (ms)', unit: 'ms' },
  { value: 'offline', label: 'Agent Offline (minutes)', unit: 'min' },
];

const operators = [
  { value: 'gt', label: '>' },
  { value: 'gte', label: '≥' },
  { value: 'lt', label: '<' },
  { value: 'lte', label: '≤' },
  { value: 'eq', label: '=' },
];

const isFormValid = computed(() => {
  return form.name.trim().length > 0 && form.threshold >= 0;
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
  Object.assign(form, {
    name: rule.name,
    description: rule.description || '',
    metric: rule.metric,
    operator: rule.operator,
    threshold: rule.threshold,
    severity: rule.severity,
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

onMounted(loadRules);
</script>

<template>
  <div class="alert-rules-config">
    <div class="d-flex justify-content-between align-items-center mb-3">
      <h5 class="mb-0">
        <i class="bi bi-bell me-2"></i>Alert Rules
      </h5>
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
    <div v-else-if="state.rules.length === 0 && !state.showForm" class="text-center py-4 text-muted">
      <i class="bi bi-bell-slash display-6 mb-2"></i>
      <p class="mb-2">No alert rules configured</p>
      <button class="btn btn-outline-primary btn-sm" @click="openCreateForm">
        Create your first rule
      </button>
    </div>

    <!-- Rules List -->
    <div v-else class="rules-list">
      <div v-for="rule in state.rules" :key="rule.id" class="rule-card" :class="{ disabled: !rule.enabled }">
        <div class="d-flex justify-content-between align-items-start">
          <div class="flex-grow-1">
            <div class="d-flex align-items-center gap-2 mb-1">
              <strong>{{ rule.name }}</strong>
              <span class="badge" :class="rule.severity === 'critical' ? 'bg-danger' : 'bg-warning'">
                {{ rule.severity }}
              </span>
              <span v-if="!rule.enabled" class="badge bg-secondary">Disabled</span>
            </div>
            <div class="text-muted small">
              {{ getMetricLabel(rule.metric) }} {{ getOperatorLabel(rule.operator) }} {{ rule.threshold }}
            </div>
            <div class="mt-1 small text-muted">
              <span v-if="rule.notify_panel" class="me-2"><i class="bi bi-display"></i> Panel</span>
              <span v-if="rule.notify_email" class="me-2"><i class="bi bi-envelope"></i> Email</span>
              <span v-if="rule.notify_webhook" class="me-2"><i class="bi bi-webhook"></i> Webhook</span>
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
    <div v-if="state.showForm" class="rule-form-overlay" @click.self="closeForm">
      <div class="rule-form-modal">
        <div class="modal-header">
          <h6 class="modal-title">{{ state.editingRule ? 'Edit' : 'Create' }} Alert Rule</h6>
          <button class="btn-close" @click="closeForm"></button>
        </div>
        <div class="modal-body">
          <!-- Name -->
          <div class="mb-3">
            <label class="form-label">Rule Name <span class="text-danger">*</span></label>
            <input v-model="form.name" type="text" class="form-control" placeholder="e.g., High Packet Loss Alert">
          </div>

          <!-- Condition -->
          <div class="row g-2 mb-3">
            <div class="col-5">
              <label class="form-label">Metric</label>
              <select v-model="form.metric" class="form-select">
                <option v-for="m in metrics" :key="m.value" :value="m.value">{{ m.label }}</option>
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
              <input v-model.number="form.threshold" type="number" class="form-control" step="0.1" min="0">
            </div>
          </div>

          <!-- Severity -->
          <div class="mb-3">
            <label class="form-label">Severity</label>
            <div class="btn-group w-100">
              <button type="button" class="btn" :class="form.severity === 'warning' ? 'btn-warning' : 'btn-outline-secondary'" @click="form.severity = 'warning'">
                Warning
              </button>
              <button type="button" class="btn" :class="form.severity === 'critical' ? 'btn-danger' : 'btn-outline-secondary'" @click="form.severity = 'critical'">
                Critical
              </button>
            </div>
          </div>

          <!-- Notification Channels -->
          <div class="mb-3">
            <label class="form-label">Notification Channels</label>
            <div class="form-check">
              <input type="checkbox" class="form-check-input" v-model="form.notify_panel" id="notifyPanel" disabled checked>
              <label class="form-check-label" for="notifyPanel">
                <i class="bi bi-display me-1"></i>Panel (always enabled)
              </label>
            </div>
            <div class="form-check">
              <input type="checkbox" class="form-check-input" v-model="form.notify_email" id="notifyEmail">
              <label class="form-check-label" for="notifyEmail">
                <i class="bi bi-envelope me-1"></i>Email workspace members
              </label>
            </div>
            <div class="form-check">
              <input type="checkbox" class="form-check-input" v-model="form.notify_webhook" id="notifyWebhook">
              <label class="form-check-label" for="notifyWebhook">
                <i class="bi bi-webhook me-1"></i>Webhook
              </label>
            </div>
          </div>

          <!-- Webhook URL (conditional) -->
          <div v-if="form.notify_webhook" class="mb-3">
            <label class="form-label">Webhook URL</label>
            <input v-model="form.webhook_url" type="url" class="form-control" placeholder="https://example.com/webhook">
          </div>

          <!-- Description -->
          <div class="mb-0">
            <label class="form-label">Description (optional)</label>
            <textarea v-model="form.description" class="form-control" rows="2" placeholder="Optional description..."></textarea>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeForm" :disabled="state.saving">Cancel</button>
          <button class="btn btn-primary" @click="saveRule" :disabled="!isFormValid || state.saving">
            <span v-if="state.saving" class="spinner-border spinner-border-sm me-1"></span>
            {{ state.editingRule ? 'Update' : 'Create' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.rules-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.rule-card {
  background: var(--bg-card, white);
  border: 1px solid var(--border-color, #e9ecef);
  border-radius: 0.5rem;
  padding: 0.75rem 1rem;
  transition: all 0.2s;
}

.rule-card:hover {
  border-color: var(--bs-primary);
}

.rule-card.disabled {
  opacity: 0.6;
}

.rule-form-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1050;
}

.rule-form-modal {
  background: var(--bg-card, white);
  border-radius: 0.75rem;
  width: 100%;
  max-width: 500px;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: 0 10px 25px rgba(0, 0, 0, 0.2);
}

.modal-header, .modal-footer {
  padding: 1rem;
  border-bottom: 1px solid var(--border-color, #e9ecef);
}

.modal-footer {
  border-top: 1px solid var(--border-color, #e9ecef);
  border-bottom: none;
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.modal-body {
  padding: 1rem;
}

/* Dark mode */
:global([data-theme="dark"]) .rule-card {
  background: #1f2937;
  border-color: #374151;
}

:global([data-theme="dark"]) .rule-form-modal {
  background: #1f2937;
}
</style>
