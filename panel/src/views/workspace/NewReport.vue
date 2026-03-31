<script lang="ts" setup>
import { onMounted, reactive } from "vue";
import { useRouter } from "vue-router";
import Title from "@/components/Title.vue";
import { ReportService, WorkspaceService } from "@/services/apiService";
import type { Workspace } from "@/types";

const router = useRouter();

interface ReportState {
  workspace: Workspace | null;
  loading: boolean;
  saving: boolean;
  error: string[];
}

const state = reactive<ReportState>({
  workspace: null,
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

function getReportTypeDescription(type_: string): string {
  const found = reportTypes.find(r => r.value === type_);
  return found ? found.desc : "";
}

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

async function submit() {
  state.error = [];

  if (!form.name.trim()) {
    state.error.push("Report name is required");
    return;
  }

  state.saving = true;

  try {
    const payload: any = {
      name: form.name,
      description: form.description,
      report_type: form.report_type,
      email_enabled: form.email_enabled,
      email_recipients: form.email_recipients,
    };

    if (form.schedule) {
      payload.schedule = form.schedule;
    }

    if (form.description) {
      payload.description = JSON.stringify({
        time_range_days: form.time_range_days,
        include_alerts: form.include_alerts,
      });
    }

    await ReportService.create(state.workspace!.id, payload);
    await router.push(`/workspaces/${state.workspace!.id}/reports`);
  } catch (e: any) {
    state.error.push(e?.message || "Failed to create report");
  } finally {
    state.saving = false;
  }
}

onMounted(async () => {
  const workspaceId = router.currentRoute.value.params["wID"] as string;
  if (!workspaceId) {
    state.error.push("Missing workspace ID");
    state.loading = false;
    return;
  }

  try {
    state.workspace = await WorkspaceService.get(workspaceId) as Workspace;
  } catch (e) {
    state.error.push("Failed to load workspace");
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
        title="New Report"
        subtitle="Configure a scheduled or on-demand report"
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
          <div class="card-header">
            <h5 class="mb-0"><i class="bi bi-file-earmark-pdf me-2"></i>Report Configuration</h5>
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
                <span v-if="state.saving"><i class="bi bi-arrow-repeat spin me-1"></i>Creating...</span>
                <span v-else><i class="bi bi-plus-circle me-1"></i>Create Report</span>
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
