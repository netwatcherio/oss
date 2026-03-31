<script lang="ts" setup>
import { onMounted, reactive } from "vue";
import { useRouter } from "vue-router";
import Title from "@/components/Title.vue";
import { ReportService, type ReportConfig, WorkspaceService } from "@/services/apiService";
import type { Workspace } from "@/types";

const router = useRouter();

interface ReportState {
  reports: ReportConfig[];
  workspace: Workspace | null;
  loading: boolean;
  error: string | null;
}

const state = reactive<ReportState>({
  reports: [],
  workspace: null,
  loading: true,
  error: null,
});

function formatSchedule(schedule: string): string {
  if (!schedule) return "On-demand only";
  const parts = schedule.split(" ");
  if (parts.length === 5) {
    return `Daily at ${parts[1]}:${parts[0] === "0" ? "00" : parts[0]} ${parts[4] === "1-5" ? "weekdays" : ""}`;
  }
  return schedule;
}

function formatLastRun(lastRunAt: string | null): string {
  if (!lastRunAt) return "Never";
  return new Date(lastRunAt).toLocaleString();
}

function getReportTypeLabel(type_: string): string {
  switch (type_) {
    case "workspace_summary": return "Workspace Summary";
    case "probe_detail": return "Probe Detail";
    case "sla": return "SLA Report";
    default: return type_;
  }
}

function navigateToNew() {
  router.push(`/workspaces/${state.workspace?.id}/reports/new`);
}

function navigateToEdit(report: ReportConfig) {
  router.push(`/workspaces/${state.workspace?.id}/reports/${report.id}/edit`);
}

async function runReport(report: ReportConfig) {
  window.open(
    `/api/v1/workspaces/${state.workspace?.id}/reports/${report.id}/run`,
    "_blank"
  );
}

async function previewReport() {
  window.open(
    `/api/v1/workspaces/${state.workspace?.id}/reports/preview?time_range_days=7`,
    "_blank"
  );
}

async function deleteReport(report: ReportConfig) {
  if (!confirm(`Delete report "${report.name}"? This cannot be undone.`)) return;
  try {
    await ReportService.remove(state.workspace!.id, report.id);
    state.reports = state.reports.filter(r => r.id !== report.id);
  } catch (e) {
    state.error = "Failed to delete report";
  }
}

onMounted(async () => {
  const workspaceId = router.currentRoute.value.params["wID"] as string;
  if (!workspaceId) {
    state.error = "Missing workspace ID";
    state.loading = false;
    return;
  }

  try {
    const [wsResponse, reportsResponse] = await Promise.all([
      WorkspaceService.get(workspaceId),
      ReportService.list(workspaceId),
    ]);
    state.workspace = wsResponse as Workspace;
    state.reports = (reportsResponse as any).reports || [];
  } catch (e) {
    state.error = "Failed to load reports";
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
        ]"
        title="Reports"
        :subtitle="state.reports.length > 0 ? `${state.reports.length} report(s) configured` : 'No reports configured'"
    >
      <button class="btn btn-outline-secondary btn-sm me-2" @click="previewReport">
        <i class="bi bi-file-earmark-pdf me-1"></i>Preview
      </button>
      <button class="btn btn-primary btn-sm" @click="navigateToNew">
        <i class="bi bi-plus-circle me-1"></i>New Report
      </button>
    </Title>

    <div v-if="state.loading" class="text-center py-5">
      <div class="spinner-border" role="status"></div>
    </div>

    <div v-else-if="state.error" class="alert alert-danger mt-4">{{ state.error }}</div>

    <div v-else-if="state.reports.length === 0" class="text-center py-5">
      <i class="bi bi-file-earmark-pdf" style="font-size: 4rem; color: #ccc;"></i>
      <h4 class="mt-3 text-muted">No reports yet</h4>
      <p class="text-muted">Create your first report to start monitoring your network.</p>
      <button class="btn btn-primary mt-2" @click="navigateToNew">
        <i class="bi bi-plus-circle me-1"></i>Create Report
      </button>
    </div>

    <div v-else class="row mt-4">
      <div class="col-12">
        <div class="card">
          <div class="card-header">
            <h5 class="mb-0"><i class="bi bi-file-earmark-pdf me-2"></i>Report Configurations</h5>
          </div>
          <div class="table-responsive">
            <table class="table table-hover mb-0">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Type</th>
                  <th>Schedule</th>
                  <th>Email</th>
                  <th>Last Run</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="report in state.reports" :key="report.id">
                  <td>
                    <strong>{{ report.name }}</strong>
                    <div v-if="report.description" class="small text-muted">{{ report.description }}</div>
                  </td>
                  <td>
                    <span class="badge bg-info">{{ getReportTypeLabel(report.report_type) }}</span>
                  </td>
                  <td>
                    <span v-if="report.schedule" class="text-muted small">{{ formatSchedule(report.schedule) }}</span>
                    <span v-else class="text-muted small">On-demand</span>
                  </td>
                  <td>
                    <span v-if="report.email_enabled" class="badge bg-success">
                      <i class="bi bi-envelope me-1"></i>{{ report.email_recipients?.length || 0 }}
                    </span>
                    <span v-else class="text-muted small">Disabled</span>
                  </td>
                  <td>
                    <span class="small text-muted">{{ formatLastRun(report.last_run_at) }}</span>
                    <div v-if="report.last_error" class="small text-danger">Error: {{ report.last_error }}</div>
                  </td>
                  <td>
                    <div class="btn-group btn-group-sm">
                      <button class="btn btn-outline-primary" @click="runReport(report)" title="Run now">
                        <i class="bi bi-play"></i>
                      </button>
                      <button class="btn btn-outline-secondary" @click="navigateToEdit(report)" title="Edit">
                        <i class="bi bi-pencil"></i>
                      </button>
                      <button class="btn btn-outline-danger" @click="deleteReport(report)" title="Delete">
                        <i class="bi bi-trash"></i>
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
