<script lang="ts" setup>
import { onMounted, reactive, computed } from "vue";
import { useRouter } from "vue-router";
import Title from "@/components/Title.vue";
import { ReportService, WorkspaceService, AgentService } from "@/services/apiService";
import type { Workspace, ReportConfig } from "@/services/apiService";
import { getSession } from "@/session";

const router = useRouter();

function getControllerEndpoint(): string {
  const anyWindow = window as any;
  if (anyWindow?.CONTROLLER_ENDPOINT) return anyWindow.CONTROLLER_ENDPOINT;
  const envUrl = (import.meta as any).env?.CONTROLLER_ENDPOINT;
  if (envUrl) return envUrl;
  return "http://localhost:8080";
}

async function downloadReport(url: string, filename: string) {
  const session = getSession();
  const token = session?.token;
  if (!token) {
    alert("You must be logged in to download reports");
    return;
  }

  try {
    const response = await fetch(url, {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }

    const blob = await response.blob();
    const blobUrl = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = blobUrl;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(blobUrl);
  } catch (err) {
    console.error("Report download failed:", err);
    alert("Failed to download report. Please try again.");
  }
}

async function runNow() {
  if (!state.report) return;
  const endpoint = getControllerEndpoint();
  const url = `${endpoint}/workspaces/${state.workspace?.id}/reports/${state.report.id}/run`;
  await downloadReport(url, `${state.report.name.replace(/[^a-z0-9]/gi, "_")}.pdf`);
}

interface EditState {
  workspace: Workspace | null;
  report: ReportConfig | null;
  agents: any[];
  probes: { id: number; name: string; type: string; target: string; agentId: number }[];
  loading: boolean;
  saving: boolean;
  error: string[];
}

const state = reactive<EditState>({
  workspace: null,
  report: null,
  agents: [],
  probes: [],
  loading: true,
  saving: false,
  error: [],
});

const form = reactive({
  name: "",
  description: "",
  report_type: "workspace_summary",
  schedule: "",
  email_enabled: false,
  email_recipients: [] as string[],
  email_input: "",
  time_range_days: 7,
  include_alerts: true,
  selected_probe_ids: [] as number[],
  sla_target: 99.5,
});

const reportTypes = [
  { value: "workspace_summary", label: "Workspace Summary", desc: "Overview of all agents, probes, and incidents" },
  { value: "probe_detail", label: "Probe Detail", desc: "Detailed metrics for specific probes" },
  { value: "sla", label: "SLA Report", desc: "Uptime and latency SLO compliance" },
];

const schedulePresets = [
  { value: "", label: "On-demand only" },
  { value: "0 9 * * *", label: "Daily at 9:00 AM" },
  { value: "0 9 * * 1", label: "Weekly (Monday 9 AM)" },
  { value: "0 9 1 * *", label: "Monthly (1st day 9 AM)" },
];

const showProbeSelector = computed(() => form.report_type === "probe_detail");
const showSLASettings = computed(() => form.report_type === "sla");

function addRecipient() {
  const email = form.email_input.trim();
  if (email && !form.email_recipients.includes(email)) {
    form.email_recipients.push(email);
    form.email_input = "";
  }
}

function removeRecipient(email: string) {
  form.email_recipients = form.email_recipients.filter(e => e !== email);
}

function toggleProbe(probeId: number) {
  const idx = form.selected_probe_ids.indexOf(probeId);
  if (idx >= 0) {
    form.selected_probe_ids.splice(idx, 1);
  } else {
    form.selected_probe_ids.push(probeId);
  }
}

function isProbeSelected(probeId: number): boolean {
  return form.selected_probe_ids.includes(probeId);
}

async function submit() {
  state.error = [];

  if (!form.name.trim()) {
    state.error.push("Report name is required");
    return;
  }

  if (form.report_type === "probe_detail" && form.selected_probe_ids.length === 0) {
    state.error.push("Please select at least one probe for Probe Detail report");
    return;
  }

  state.saving = true;

  try {
    const configJSON: Record<string, any> = {
      time_range_days: form.time_range_days,
      include_alerts: form.include_alerts,
    };

    if (form.report_type === "probe_detail") {
      configJSON.probe_ids = form.selected_probe_ids;
    }

    if (form.report_type === "sla") {
      configJSON.sla_target = form.sla_target;
    }

    const payload: any = {
      name: form.name,
      description: JSON.stringify(configJSON),
      report_type: form.report_type,
      email_enabled: form.email_enabled,
      email_recipients: form.email_recipients,
    };

    if (form.schedule !== undefined) {
      payload.schedule = form.schedule;
    }

    await ReportService.update(state.workspace!.id, state.report!.id, payload);
    await router.push(`/workspaces/${state.workspace!.id}/reports`);
  } catch (e: any) {
    state.error.push(e?.message || "Failed to update report");
  } finally {
    state.saving = false;
  }
}

onMounted(async () => {
  const workspaceId = router.currentRoute.value.params["wID"] as string;
  const reportId = router.currentRoute.value.params["rID"] as string;

  if (!workspaceId || !reportId) {
    state.error.push("Missing workspace or report ID");
    state.loading = false;
    return;
  }

  try {
    const [wsResponse, reportResponse] = await Promise.all([
      WorkspaceService.get(workspaceId),
      ReportService.get(workspaceId, reportId),
    ]);
    state.workspace = wsResponse as Workspace;
    state.report = reportResponse as ReportConfig;

    form.name = state.report.name;
    form.description = state.report.description;
    form.report_type = state.report.report_type;
    form.schedule = state.report.schedule || "";
    form.email_enabled = state.report.email_enabled;
    form.email_recipients = [...(state.report.email_recipients || [])];
    form.time_range_days = state.report.config?.time_range_days || 7;
    form.include_alerts = state.report.config?.include_alerts ?? true;
    form.selected_probe_ids = state.report.config?.probe_ids || [];
    form.sla_target = state.report.config?.sla_target || 99.5;

    const agentsResponse = await AgentService.list(workspaceId);
    const agentsData = (agentsResponse as any).data || agentsResponse || [];
    state.agents = agentsData;

    const allProbes: { id: number; name: string; type: string; target: string; agentId: number }[] = [];
    for (const agent of state.agents) {
      try {
        const probesResponse = await AgentService.listAgentsProbes ? AgentService.listAgentsProbes(workspaceId, agent.id) : [];
        const probes = (probesResponse as any) || [];
        for (const p of probes) {
          allProbes.push({
            id: p.id,
            name: p.name || `Probe ${p.id}`,
            type: p.type || "UNKNOWN",
            target: p.target || "N/A",
            agentId: agent.id,
          });
        }
      } catch {
        // Skip agents without probes
      }
    }
    state.probes = allProbes;
  } catch (e) {
    state.error.push("Failed to load report");
  } finally {
    state.loading = false;
  }
});
</script>

<template>
  <div class="container-fluid">
    <Title
        v-if="state.workspace"
        :history="[
          { title: 'workspaces', link: '/workspaces' },
          { title: state.workspace.name, link: `/workspaces/${state.workspace.id}` },
          { title: 'Reports', link: `/workspaces/${state.workspace.id}/reports` },
        ]"
        :title="form.name || 'Edit Report'"
        subtitle="Update report configuration"
    />

    <div v-if="state.loading" class="text-center py-5">
      <div class="spinner-border" role="status"></div>
    </div>

    <div v-else class="row mt-4">
      <div class="col-lg-8 col-xl-6">

        <div v-if="state.error.length > 0" class="alert alert-danger">
          <div v-for="(err, i) in state.error" :key="i">{{ err }}</div>
        </div>

        <div class="card mb-4">
          <div class="card-header d-flex justify-content-between align-items-center">
            <h5 class="mb-0"><i class="bi bi-file-earmark-pdf me-2"></i>Report Configuration</h5>
            <button v-if="state.report" class="btn btn-outline-primary btn-sm" @click="runNow">
              <i class="bi bi-play me-1"></i>Run Now
            </button>
          </div>
          <div class="card-body">

            <div class="mb-3">
              <label class="form-label fw-semibold">Report Name</label>
              <input v-model="form.name" class="form-control" type="text" placeholder="Weekly Network Summary" />
            </div>

            <div class="mb-3">
              <label class="form-label fw-semibold">Description (optional)</label>
              <textarea v-model="form.description" class="form-control" rows="2" placeholder="Brief description of this report..."></textarea>
            </div>

            <div class="mb-4">
              <label class="form-label fw-semibold">Report Type</label>
              <div class="row g-3">
                <div v-for="rt in reportTypes" :key="rt.value" class="col-md-4">
                  <div
                    class="card report-type-card"
                    :class="{ selected: form.report_type === rt.value }"
                    @click="form.report_type = rt.value"
                  >
                    <div class="card-body py-2">
                      <div class="fw-semibold small">{{ rt.label }}</div>
                      <div class="text-muted small">{{ rt.desc }}</div>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <div class="mb-3">
              <label class="form-label fw-semibold">Schedule</label>
              <select v-model="form.schedule" class="form-select">
                <option v-for="preset in schedulePresets" :key="preset.value" :value="preset.value">
                  {{ preset.label }}
                </option>
              </select>
              <div class="form-text" v-if="form.schedule">
                Cron expression: <code>{{ form.schedule }}</code>
              </div>
            </div>

            <div class="mb-3">
              <label class="form-label fw-semibold">Time Range</label>
              <select v-model.number="form.time_range_days" class="form-select">
                <option :value="1">Last 24 hours</option>
                <option :value="7">Last 7 days</option>
                <option :value="14">Last 14 days</option>
                <option :value="30">Last 30 days</option>
                <option :value="90">Last 90 days</option>
              </select>
            </div>

            <div v-if="showProbeSelector" class="mb-3">
              <label class="form-label fw-semibold">Select Probes</label>
              <div class="border rounded p-2" style="max-height: 200px; overflow-y: auto;">
                <div v-if="state.probes.length === 0" class="text-muted small">No probes available</div>
                <div
                  v-for="probe in state.probes"
                  :key="probe.id"
                  class="form-check"
                >
                  <input
                    class="form-check-input"
                    type="checkbox"
                    :id="`probe-${probe.id}`"
                    :checked="isProbeSelected(probe.id)"
                    @change="toggleProbe(probe.id)"
                  />
                  <label class="form-check-label" :for="`probe-${probe.id}`">
                    <span class="fw-medium">{{ probe.name }}</span>
                    <span class="text-muted small ms-2">({{ probe.type }} → {{ probe.target }})</span>
                  </label>
                </div>
              </div>
              <div class="form-text">Select which probes to include in the report</div>
            </div>

            <div v-if="showSLASettings" class="mb-3">
              <label class="form-label fw-semibold">SLA Uptime Target (%)</label>
              <div class="input-group" style="max-width: 200px;">
                <input
                  v-model.number="form.sla_target"
                  type="number"
                  class="form-control"
                  min="90"
                  max="99.99"
                  step="0.1"
                />
                <span class="input-group-text">%</span>
              </div>
              <div class="form-text">Probes with uptime below this threshold will be flagged</div>
            </div>

            <div class="mb-3">
              <div class="form-check form-switch">
                <input id="includeAlerts" v-model="form.include_alerts" class="form-check-input" type="checkbox" />
                <label for="includeAlerts" class="form-check-label">Include alert history</label>
              </div>
            </div>

            <hr />

            <div class="mb-3">
              <div class="form-check form-switch">
                <input id="emailEnabled" v-model="form.email_enabled" class="form-check-input" type="checkbox" />
                <label for="emailEnabled" class="form-check-label fw-semibold">Email delivery</label>
              </div>
            </div>

            <div v-if="form.email_enabled" class="email-recipients-section">
              <label class="form-label">Recipients</label>
              <div class="input-group mb-2">
                <input
                  v-model="form.email_input"
                  class="form-control"
                  type="email"
                  placeholder="email@example.com"
                  @keydown.enter.prevent="addRecipient"
                />
                <button class="btn btn-outline-secondary" type="button" @click="addRecipient">Add</button>
              </div>
              <div v-if="form.email_recipients.length === 0" class="text-muted small mb-2">No recipients added</div>
              <div class="d-flex flex-wrap gap-1">
                <span
                  v-for="email in form.email_recipients"
                  :key="email"
                  class="badge bg-primary d-flex align-items-center gap-1"
                >
                  {{ email }}
                  <button type="button" class="btn-close btn-close-white small" @click="removeRecipient(email)"></button>
                </span>
              </div>
            </div>

          </div>
          <div class="card-footer">
            <div class="d-flex justify-content-between">
              <router-link :to="`/workspaces/${state.workspace?.id}/reports`" class="btn btn-outline-secondary">
                <i class="bi bi-arrow-left me-1"></i>Cancel
              </router-link>
              <button class="btn btn-primary px-4" @click="submit" :disabled="state.saving">
                <span v-if="state.saving"><i class="bi bi-arrow-repeat spin me-1"></i>Saving...</span>
                <span v-else><i class="bi bi-check-circle me-1"></i>Save Changes</span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.report-type-card {
  cursor: pointer;
  border: 2px solid #e9ecef;
  transition: all 0.2s;
}
.report-type-card:hover {
  border-color: #0d6efd;
}
.report-type-card.selected {
  border-color: #0d6efd;
  background-color: #f0f6ff;
}
</style>
