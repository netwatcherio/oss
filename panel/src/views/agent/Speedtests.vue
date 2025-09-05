<script lang="ts" setup>
import {onMounted, reactive, computed} from "vue";
import type {
  Agent,
  Probe,
  ProbeData,
  ProbeDataRequest,
  Workspace,
  SpeedTestPLoss, 
  SpeedTestResult, 
  SpeedTestServer, 
  SpeedTestTestDuration
} from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import VueDatePicker from '@vuepic/vue-datepicker';
import '@vuepic/vue-datepicker/dist/main.css'

const state = reactive({
  site: {} as Workspace,
  ready: false,
  loading: true,
  agent: {} as Agent,
  probe: {} as Probe,
  title: "Speedtests",
  speedtestProbe: {} as Probe,
  speedtestData: [] as SpeedTestResult[],
  speedtestServerProbe: {} as Probe,
  selectedTest: null as SpeedTestResult | null,
  expandedTests: new Set<string>(),
  pendingTest: null as { target: string, time: Date } | null
})

// Computed properties
const latestTest = computed(() => {
  if (state.speedtestData.length === 0) return null;
  return state.speedtestData[0];
});

const averageDownload = computed(() => {
  if (state.speedtestData.length === 0) return 0;
  const sum = state.speedtestData.reduce((acc, test) => acc + (test.test_data[0]?.dl_speed || 0), 0);
  return ((sum / state.speedtestData.length) * 8 / 1048576).toFixed(1); // Convert MiB/s to Mb/s
});

const averageUpload = computed(() => {
  if (state.speedtestData.length === 0) return 0;
  const sum = state.speedtestData.reduce((acc, test) => acc + (test.test_data[0]?.ul_speed || 0), 0);
  return ((sum / state.speedtestData.length) * 8 / 1048576).toFixed(1); // Convert MiB/s to Mb/s
});

const averageLatency = computed(() => {
  if (state.speedtestData.length === 0) return 0;
  const sum = state.speedtestData.reduce((acc, test) => acc + (test.test_data[0]?.latency || 0), 0);
  return (sum / state.speedtestData.length / 1000000).toFixed(1);
});

function convertToSpeedTestResult(data: any): SpeedTestResult {
  const result: SpeedTestResult = {
    test_data: [],
    timestamp: new Date(data.createdAt)
  };

  const dataArray = data;
  if (!dataArray || !Array.isArray(dataArray)) {
    throw new Error("Invalid data structure");
  }

  const testDataItem = dataArray.find(item => item.Key === "testdata");
  if (!testDataItem || !Array.isArray(testDataItem.Value)) {
    throw new Error("Invalid testdata structure");
  }

  testDataItem.Value.forEach((serverData: any[]) => {
    const server = convertToSpeedTestServer(serverData);
    result.test_data.push(server);
  });

  const timestampItem = dataArray.find(item => item.Key === "timestamp");
  if (timestampItem && timestampItem.Value) {
    result.timestamp = new Date(timestampItem.Value);
  }

  return result;
}

function convertToSpeedTestServer(data: Array<{ Key: string; Value: any }>): SpeedTestServer {
  const result: Partial<SpeedTestServer> = {};

  for (const item of data) {
    switch (item.Key) {
      case 'url':
      case 'lat':
      case 'lon':
      case 'name':
      case 'country':
      case 'sponsor':
      case 'id':
      case 'host':
        result[item.Key] = item.Value as string;
        break;
      case 'distance':
      case 'latency':
      case 'max_latency':
      case 'min_latency':
      case 'jitter':
      case 'dl_speed':
      case 'ul_speed':
        result[item.Key] = Number(item.Value);
        break;
      case 'test_duration':
        result.test_duration = convertTestDuration(item.Value);
        break;
      case 'packet_loss':
        result.packet_loss = convertPacketLoss(item.Value);
        break;
    }
  }

  return result as SpeedTestServer;
}

function convertTestDuration(data: Array<{ Key: string; Value: number }>): SpeedTestTestDuration {
  const result: SpeedTestTestDuration = {};
  for (const item of data) {
    result[item.Key as keyof SpeedTestTestDuration] = Number(item.Value);
  }
  return result;
}

function convertPacketLoss(data: Array<{ Key: string; Value: number }>): SpeedTestPLoss {
  const result: SpeedTestPLoss = { sent: 0, dup: 0, max: 0 };
  for (const item of data) {
    result[item.Key as keyof SpeedTestPLoss] = Number(item.Value);
  }
  return result;
}

function formatSpeed(speed: number): string {
  // Convert from MiB/s to Mb/s (1 MiB = 8.388608 Mb)
  return (speed * 8 / 1048576).toFixed(1);
}

function formatLatency(latency: number): string {
  return (latency / 1000000).toFixed(1);
}

function formatDuration(duration: number): string {
  return (duration / 1000000000).toFixed(1);
}

function formatDate(date: Date): string {
  return new Intl.DateTimeFormat('en-US', {
    dateStyle: 'medium',
    timeStyle: 'short'
  }).format(date);
}

function getSpeedClass(speed: number): string {
  // Convert MiB/s to Mb/s for classification
  const mbps = speed * 8 / 1048576;
  if (mbps >= 100) return 'excellent';
  if (mbps >= 50) return 'good';
  if (mbps >= 25) return 'fair';
  return 'poor';
}

function getLatencyClass(latency: number): string {
  const ms = latency / 1000000;
  if (ms <= 20) return 'excellent';
  if (ms <= 50) return 'good';
  if (ms <= 100) return 'fair';
  return 'poor';
}

function toggleTest(testId: string) {
  if (state.expandedTests.has(testId)) {
    state.expandedTests.delete(testId);
  } else {
    state.expandedTests.add(testId);
  }
}

function isExpanded(testId: string): boolean {
  return state.expandedTests.has(testId);
}

function formatTimeUntil(date: Date): string {
  const now = new Date();
  const diff = date.getTime() - now.getTime();
  
  if (diff <= 0) return 'Starting soon...';
  
  const minutes = Math.floor(diff / 60000);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);
  
  if (days > 0) return `in ${days} day${days > 1 ? 's' : ''}`;
  if (hours > 0) return `in ${hours} hour${hours > 1 ? 's' : ''}`;
  if (minutes > 0) return `in ${minutes} minute${minutes > 1 ? 's' : ''}`;
  return 'in less than a minute';
}

onMounted(() => {
  let checkId = router.currentRoute.value.params["idParam"] as string
  if (!checkId) return

  agentService.getAgent(checkId).then(res => {
    state.agent = res.data as Agent

    siteService.getSite(state.agent.site).then(res => {
      state.site = res.data as Workspace
    })

    probeService.getAgentProbes(state.agent.id).then(res => {
      let probes = res.data as Probe[]
      for (let item in probes) {
        if (probes[item].type == "SPEEDTEST") {
          state.speedtestProbe = probes[item]
          
          // Check if there's a pending test
          if (state.speedtestProbe.config?.target?.[0]?.target !== "ok") {
            state.pendingTest = {
              target: state.speedtestProbe.config.target[0].target,
              time: state.speedtestProbe.config.pending ? new Date(state.speedtestProbe.config.pending) : new Date()
            };
          }
          
          let req = {limit: 25, recent: true} as ProbeDataRequest
          probeService.getProbeData(state.speedtestProbe.id, req).then(res => {
            for (let i in res.data as ProbeData[]) {
              let convD = convertToSpeedTestResult(res.data[i].data)
              state.speedtestData.push(convD)
            }
            state.ready = true
            state.loading = false
          })
        } else if (probes[item].type == "SPEEDTEST_SERVERS") {
          state.speedtestServerProbe = probes[item]
        }
      }
    })
  })
})

const router = core.router()
</script>

<template>
  <div class="container-fluid">
    <Title
        :history="[
          {title: 'workspaces', link: '/workspaces'}, 
          {title: state.site.name || 'Loading...', link: `/workspace/${state.site.id}`}, 
          {title: state.agent.name || 'Loading...', link: `/agent/${state.agent.id}`}
        ]"
        :title="state.title"
        subtitle="Network performance history">
      <div class="d-flex gap-2">
        <router-link 
          :to="`/agent/${state.speedtestServerProbe.id}/speedtest/new`" 
          class="btn btn-primary"
        >
          <i class="fa-solid fa-gauge-high"></i>&nbsp;Run Speedtest
        </router-link>
      </div>
    </Title>

    <!-- Pending Test Notification -->
    <div v-if="state.pendingTest" class="pending-test-card">
      <div class="pending-icon">
        <i class="fa-solid fa-clock"></i>
      </div>
      <div class="pending-content">
        <h6 class="pending-title">Speedtest Scheduled</h6>
        <p class="pending-description">
          A speedtest is pending for server <strong>{{ state.pendingTest.target }}</strong>
        </p>
        <div class="pending-time">
          <i class="fa-regular fa-clock"></i>
          Starting {{ formatTimeUntil(state.pendingTest.time) }}
        </div>
      </div>
      <div class="pending-actions">
        <button class="btn btn-sm btn-outline-secondary" disabled>
          <i class="fa-solid fa-hourglass-half"></i>
          Waiting...
        </button>
      </div>
    </div>

    <!-- Statistics Cards -->
    <div class="stats-grid" v-if="state.ready && state.speedtestData.length > 0">
      <div class="stat-card">
        <div class="stat-icon download">
          <i class="fa-solid fa-download"></i>
        </div>
        <div class="stat-content">
          <div class="stat-label">Average Download</div>
          <div class="stat-value">{{ averageDownload }} <span class="stat-unit">Mbps</span></div>
        </div>
      </div>
      
      <div class="stat-card">
        <div class="stat-icon upload">
          <i class="fa-solid fa-upload"></i>
        </div>
        <div class="stat-content">
          <div class="stat-label">Average Upload</div>
          <div class="stat-value">{{ averageUpload }} <span class="stat-unit">Mbps</span></div>
        </div>
      </div>
      
      <div class="stat-card">
        <div class="stat-icon latency">
          <i class="fa-solid fa-stopwatch"></i>
        </div>
        <div class="stat-content">
          <div class="stat-label">Average Latency</div>
          <div class="stat-value">{{ averageLatency }} <span class="stat-unit">ms</span></div>
        </div>
      </div>
      
      <div class="stat-card">
        <div class="stat-icon tests">
          <i class="fa-solid fa-list-check"></i>
        </div>
        <div class="stat-content">
          <div class="stat-label">Total Tests</div>
          <div class="stat-value">{{ state.speedtestData.length }}</div>
        </div>
      </div>
    </div>

    <!-- Loading State -->
    <div v-if="state.loading" class="content-card">
      <div class="loading-state">
        <div class="spinner-border text-primary" role="status">
          <span class="visually-hidden">Loading...</span>
        </div>
        <p class="loading-text">Loading speedtest data...</p>
      </div>
    </div>

    <!-- Empty State -->
    <div v-else-if="state.ready && state.speedtestData.length === 0" class="content-card">
      <div class="empty-state">
        <i class="fa-solid fa-gauge-high"></i>
        <h5>No Speedtest Results</h5>
        <p>Run your first speedtest to start tracking network performance.</p>
        <router-link 
          :to="`/agent/${state.speedtestServerProbe.id}/speedtest/new`" 
          class="btn btn-primary"
        >
          <i class="fa-solid fa-plus"></i> Run First Speedtest
        </router-link>
      </div>
    </div>

    <!-- Speedtest Results -->
    <div v-else class="speedtest-results">
      <div class="results-header">
        <h5 class="results-title">Recent Speedtests</h5>
        <span class="results-count">Showing last {{ state.speedtestData.length }} tests</span>
      </div>

      <div class="test-list">
        <div 
          v-for="(test, index) in state.speedtestData" 
          :key="state.speedtestProbe.id + test.timestamp.getTime()"
          class="test-item"
          :class="{ 'expanded': isExpanded(state.speedtestProbe.id + test.timestamp.getTime()) }"
        >
          <div 
            class="test-header"
            @click="toggleTest(state.speedtestProbe.id + test.timestamp.getTime())"
          >
            <div class="test-info">
              <div class="test-time">
                <i class="fa-regular fa-clock"></i>
                {{ formatDate(test.timestamp) }}
              </div>
              <div class="test-server">
                <i class="fa-solid fa-server"></i>
                {{ test.test_data[0].sponsor }} 
                <span class="server-location">
                  ({{ test.test_data[0].name }}, {{ test.test_data[0].country }})
                </span>
                <span class="server-distance">{{ test.test_data[0].distance }}km</span>
              </div>
            </div>
            
            <div class="test-metrics">
              <div class="metric" :class="'speed-' + getSpeedClass(test.test_data[0].dl_speed)">
                <i class="fa-solid fa-download"></i>
                <span>{{ formatSpeed(test.test_data[0].dl_speed) }} Mbps</span>
              </div>
              <div class="metric" :class="'speed-' + getSpeedClass(test.test_data[0].ul_speed)">
                <i class="fa-solid fa-upload"></i>
                <span>{{ formatSpeed(test.test_data[0].ul_speed) }} Mbps</span>
              </div>
              <div class="metric" :class="'latency-' + getLatencyClass(test.test_data[0].latency)">
                <i class="fa-solid fa-stopwatch"></i>
                <span>{{ formatLatency(test.test_data[0].latency) }} ms</span>
              </div>
            </div>
            
            <div class="test-toggle">
              <i :class="isExpanded(state.speedtestProbe.id + test.timestamp.getTime()) ? 'fa-solid fa-chevron-up' : 'fa-solid fa-chevron-down'"></i>
            </div>
          </div>
          
          <div 
            class="test-details"
            v-show="isExpanded(state.speedtestProbe.id + test.timestamp.getTime())"
          >
            <div class="details-grid">
              <div class="detail-section">
                <h6>Performance Metrics</h6>
                <div class="detail-row">
                  <span class="detail-label">Download Speed</span>
                  <span class="detail-value">{{ formatSpeed(test.test_data[0].dl_speed) }} Mbps</span>
                </div>
                <div class="detail-row">
                  <span class="detail-label">Upload Speed</span>
                  <span class="detail-value">{{ formatSpeed(test.test_data[0].ul_speed) }} Mbps</span>
                </div>
                <div class="detail-row">
                  <span class="detail-label">Latency</span>
                  <span class="detail-value">{{ formatLatency(test.test_data[0].latency) }} ms</span>
                </div>
                <div class="detail-row">
                  <span class="detail-label">Jitter</span>
                  <span class="detail-value">{{ formatLatency(test.test_data[0].jitter) }} ms</span>
                </div>
              </div>
              
              <div class="detail-section">
                <h6>Server Information</h6>
                <div class="detail-row">
                  <span class="detail-label">Server</span>
                  <span class="detail-value">{{ test.test_data[0].sponsor }}</span>
                </div>
                <div class="detail-row">
                  <span class="detail-label">Location</span>
                  <span class="detail-value">{{ test.test_data[0].name }}, {{ test.test_data[0].country }}</span>
                </div>
                <div class="detail-row">
                  <span class="detail-label">Distance</span>
                  <span class="detail-value">{{ test.test_data[0].distance }} km</span>
                </div>
                <div class="detail-row">
                  <span class="detail-label">Host</span>
                  <span class="detail-value">{{ test.test_data[0].host }}</span>
                </div>
              </div>
              
              <div class="detail-section" v-if="test.test_data[0].test_duration">
                <h6>Test Duration</h6>
                <div class="detail-row" v-if="test.test_data[0].test_duration.download">
                  <span class="detail-label">Download Test</span>
                  <span class="detail-value">{{ formatDuration(test.test_data[0].test_duration.download) }} s</span>
                </div>
                <div class="detail-row" v-if="test.test_data[0].test_duration.upload">
                  <span class="detail-label">Upload Test</span>
                  <span class="detail-value">{{ formatDuration(test.test_data[0].test_duration.upload) }} s</span>
                </div>
                <div class="detail-row" v-if="test.test_data[0].test_duration.total">
                  <span class="detail-label">Total Duration</span>
                  <span class="detail-value">{{ formatDuration(test.test_data[0].test_duration.total) }} s</span>
                </div>
              </div>
              
              <div class="detail-section" v-if="test.test_data[0].packet_loss">
                <h6>Packet Loss</h6>
                <div class="detail-row">
                  <span class="detail-label">Packets Sent</span>
                  <span class="detail-value">{{ test.test_data[0].packet_loss.sent }}</span>
                </div>
                <div class="detail-row" v-if="test.test_data[0].packet_loss.dup">
                  <span class="detail-label">Duplicates</span>
                  <span class="detail-value">{{ test.test_data[0].packet_loss.dup }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Pending Test Card */
.pending-test-card {
  background: linear-gradient(135deg, #fef3c7 0%, #fed7aa 100%);
  border: 1px solid #fbbf24;
  border-radius: 8px;
  padding: 1.25rem;
  margin-bottom: 1.5rem;
  display: flex;
  align-items: center;
  gap: 1rem;
  box-shadow: 0 2px 4px rgba(251, 191, 36, 0.1);
}

.pending-icon {
  width: 3rem;
  height: 3rem;
  background: white;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #f59e0b;
  font-size: 1.5rem;
  flex-shrink: 0;
  animation: pulse 2s infinite;
}

@keyframes pulse {
  0% {
    box-shadow: 0 0 0 0 rgba(245, 158, 11, 0.4);
  }
  70% {
    box-shadow: 0 0 0 10px rgba(245, 158, 11, 0);
  }
  100% {
    box-shadow: 0 0 0 0 rgba(245, 158, 11, 0);
  }
}

.pending-content {
  flex: 1;
}

.pending-title {
  margin: 0 0 0.25rem 0;
  font-size: 1rem;
  font-weight: 600;
  color: #92400e;
}

.pending-description {
  margin: 0 0 0.5rem 0;
  font-size: 0.875rem;
  color: #78350f;
}

.pending-description strong {
  color: #92400e;
  font-family: monospace;
}

.pending-time {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
  color: #92400e;
  font-weight: 500;
}

.pending-actions {
  flex-shrink: 0;
}

/* Statistics Grid */
.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 1rem;
  margin-bottom: 1.5rem;
}

.stat-card {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 1.25rem;
  display: flex;
  align-items: center;
  gap: 1rem;
  transition: all 0.2s;
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
}

.stat-icon {
  width: 3rem;
  height: 3rem;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.25rem;
}

.stat-icon.download {
  background: #dbeafe;
  color: #3b82f6;
}

.stat-icon.upload {
  background: #d1fae5;
  color: #10b981;
}

.stat-icon.latency {
  background: #fef3c7;
  color: #f59e0b;
}

.stat-icon.tests {
  background: #ede9fe;
  color: #8b5cf6;
}

.stat-content {
  flex: 1;
}

.stat-label {
  font-size: 0.875rem;
  color: #6b7280;
  margin-bottom: 0.25rem;
}

.stat-value {
  font-size: 1.75rem;
  font-weight: 700;
  color: #1f2937;
  line-height: 1;
}

.stat-unit {
  font-size: 1rem;
  font-weight: 500;
  color: #6b7280;
}

/* Content Card */
.content-card {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 2rem;
}

/* Loading State */
.loading-state {
  text-align: center;
  padding: 3rem 0;
}

.loading-text {
  margin-top: 1rem;
  color: #6b7280;
}

/* Empty State */
.empty-state {
  text-align: center;
  padding: 3rem 0;
}

.empty-state i {
  font-size: 3rem;
  color: #e5e7eb;
  margin-bottom: 1rem;
}

.empty-state h5 {
  color: #1f2937;
  margin-bottom: 0.5rem;
}

.empty-state p {
  color: #6b7280;
  margin-bottom: 1.5rem;
}

/* Results Section */
.speedtest-results {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  overflow: hidden;
}

.results-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1.25rem;
  border-bottom: 1px solid #e5e7eb;
  background: #f9fafb;
}

.results-title {
  margin: 0;
  font-size: 1.125rem;
  font-weight: 600;
  color: #1f2937;
}

.results-count {
  font-size: 0.875rem;
  color: #6b7280;
}

/* Test List */
.test-list {
  display: flex;
  flex-direction: column;
}

.test-item {
  border-bottom: 1px solid #e5e7eb;
  transition: all 0.2s;
}

.test-item:last-child {
  border-bottom: none;
}

.test-item:hover {
  background: #f9fafb;
}

.test-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1.25rem;
  cursor: pointer;
  gap: 1rem;
}

.test-info {
  flex: 1;
  min-width: 0;
}

.test-time {
  font-size: 0.875rem;
  color: #6b7280;
  margin-bottom: 0.25rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.test-server {
  font-weight: 600;
  color: #1f2937;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.server-location {
  font-weight: 400;
  color: #6b7280;
}

.server-distance {
  font-size: 0.875rem;
  color: #9ca3af;
  padding: 0.125rem 0.5rem;
  background: #f3f4f6;
  border-radius: 4px;
}

/* Test Metrics */
.test-metrics {
  display: flex;
  gap: 1.5rem;
  align-items: center;
}

.metric {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 600;
  padding: 0.375rem 0.75rem;
  border-radius: 6px;
  font-size: 0.875rem;
}

.metric i {
  font-size: 0.875rem;
}

/* Speed Classes */
.speed-excellent {
  background: #d1fae5;
  color: #065f46;
}

.speed-good {
  background: #dbeafe;
  color: #1e40af;
}

.speed-fair {
  background: #fef3c7;
  color: #92400e;
}

.speed-poor {
  background: #fee2e2;
  color: #991b1b;
}

/* Latency Classes */
.latency-excellent {
  background: #d1fae5;
  color: #065f46;
}

.latency-good {
  background: #dbeafe;
  color: #1e40af;
}

.latency-fair {
  background: #fef3c7;
  color: #92400e;
}

.latency-poor {
  background: #fee2e2;
  color: #991b1b;
}

.test-toggle {
  color: #9ca3af;
  transition: transform 0.2s;
}

.test-item.expanded .test-toggle {
  transform: rotate(180deg);
}

/* Test Details */
.test-details {
  padding: 0 1.25rem 1.25rem;
  background: #f9fafb;
  border-top: 1px solid #e5e7eb;
}

.details-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 1.5rem;
  margin-top: 1rem;
}

.detail-section h6 {
  font-size: 0.875rem;
  font-weight: 600;
  color: #374151;
  margin-bottom: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.detail-row {
  display: flex;
  justify-content: space-between;
  padding: 0.5rem 0;
  border-bottom: 1px solid #e5e7eb;
}

.detail-row:last-child {
  border-bottom: none;
}

.detail-label {
  font-size: 0.875rem;
  color: #6b7280;
}

.detail-value {
  font-size: 0.875rem;
  color: #1f2937;
  font-weight: 500;
  font-family: monospace;
}

/* Responsive */
@media (max-width: 768px) {
  .stats-grid {
    grid-template-columns: repeat(2, 1fr);
  }
  
  .test-header {
    flex-direction: column;
    align-items: flex-start;
  }
  
  .test-metrics {
    width: 100%;
    justify-content: space-between;
  }
  
  .metric {
    padding: 0.25rem 0.5rem;
    font-size: 0.75rem;
  }
  
  .details-grid {
    grid-template-columns: 1fr;
  }
  
  .pending-test-card {
    flex-direction: column;
    text-align: center;
  }
  
  .pending-content {
    text-align: center;
  }
  
  .pending-time {
    justify-content: center;
  }
}

@media (max-width: 576px) {
  .stats-grid {
    grid-template-columns: 1fr;
  }
  
  .test-metrics {
    flex-wrap: wrap;
    gap: 0.5rem;
  }
  
  .server-location {
    display: block;
    width: 100%;
  }
}
</style>