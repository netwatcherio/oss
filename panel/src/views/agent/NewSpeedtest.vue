<script lang="ts" setup>
import { onMounted, onUnmounted, reactive, ref, watch, computed } from "vue";
import type {
  Agent,
  AgentGroup,
  Probe,
  ProbeConfig, 
  ProbeData, 
  ProbeDataRequest,
  ProbeTarget,
  ProbeType,
  SelectOption,
  Site, 
  SpeedTestPLoss,
  SpeedTestServer, 
  SpeedTestTestDuration
} from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import agentService from "@/services/agentService";
import probeService from "@/services/probeService";
import siteService from "@/services/siteService";

const state = reactive({
  site: {} as Site,
  ready: false,
  loading: true,
  agent: {} as Agent,
  selected: null as SelectOption | null,
  options: [] as SelectOption[],
  probe: {} as Probe,
  probeConfig: {} as ProbeConfig,
  probeTarget: {} as ProbeTarget,
  speedTestServers: {} as SpeedTestServer[],
  customServerEnable: false,
  customServer: '',
  speedtestProbe: {} as Probe,
  submitting: false
})

// Refs for the searchable dropdown
const searchQuery = ref('');
const filteredOptions = ref<SelectOption[]>([]);
const isDropdownOpen = ref(false);
const dropdownRef = ref<HTMLElement | null>(null);
const inputRef = ref<HTMLInputElement | null>(null);

// Computed properties
const isFormValid = computed(() => {
  if (state.customServerEnable) {
    return state.customServer.trim().length > 0;
  }
  return state.selected !== null;
});

const selectedServer = computed(() => {
  if (!state.selected) return null;
  const parts = state.selected.text.split(' - ');
  const distance = parts[0];
  const [sponsor, location] = parts[1] ? parts[1].split(' (') : ['', ''];
  return {
    distance,
    sponsor,
    location: location ? location.replace(')', '') : ''
  };
});

// Function to filter options based on search query
function filterOptions() {
  const query = searchQuery.value.toLowerCase();
  if (query === '') {
    filteredOptions.value = state.options;
  } else {
    filteredOptions.value = state.options.filter(option =>
      option.text.toLowerCase().includes(query)
    );
  }
}

// Watch for changes
watch(searchQuery, filterOptions);
watch(() => state.customServerEnable, (newVal) => {
  if (newVal) {
    state.selected = null;
    searchQuery.value = '';
  }
});

// Function to select an option
function selectOption(option: SelectOption) {
  state.selected = option;
  searchQuery.value = option.text;
  isDropdownOpen.value = false;
  
  // Emit a visual feedback
  if (inputRef.value) {
    inputRef.value.classList.add('selected');
    setTimeout(() => {
      inputRef.value?.classList.remove('selected');
    }, 300);
  }
}

// Function to handle clicking outside the dropdown
function handleClickOutside(event: MouseEvent) {
  if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) {
    isDropdownOpen.value = false;
    if (!state.selected && !state.customServerEnable) {
      searchQuery.value = '';
    }
  }
}

// Function to clear selection
function clearSelection() {
  state.selected = null;
  searchQuery.value = '';
  isDropdownOpen.value = false;
}

const router = core.router()

onMounted(async () => {
  let id = router.currentRoute.value.params["idParam"] as string
  if (!id) return

  try {
    const probeRes = await probeService.getProbe(id);
    let pps = probeRes.data as Probe[]
    state.probe = pps[0]

    const agentRes = await agentService.getAgent(state.probe.agent);
    state.agent = agentRes.data as Agent

    const siteRes = await siteService.getSite(state.agent.site);
    state.site = siteRes.data as Site

    let req = {limit: 1, recent: true} as ProbeDataRequest
    const dataRes = await probeService.getProbeData(state.probe.id, req);
    let probeData = dataRes.data as ProbeData[]

    if (probeData[0]?.data) {
      for(let item in probeData[0].data){
        let srv = convertToSpeedTestServer(probeData[0].data[item])
        let displayText = `${srv.distance}km - ${srv.sponsor} (${srv.name}, ${srv.country})`
        state.options.push({value: srv.id, text: displayText} as SelectOption)
      }
    }

    // Sort options by distance
    state.options.sort((a, b) => {
      const distA = parseInt(a.text.split('km')[0]);
      const distB = parseInt(b.text.split('km')[0]);
      return distA - distB;
    });

    const probesRes = await probeService.getAgentProbes(state.agent.id);
    let probes = probesRes.data as Probe[]
    for(let item in probes){
      if(probes[item].type == "SPEEDTEST"){
        state.speedtestProbe = probes[item]
        break
      }
    }

    state.ready = true
    state.loading = false
    filteredOptions.value = state.options;
  } catch (error) {
    console.error('Error loading data:', error);
    state.loading = false;
  }

  // Add event listener for clicking outside
  document.addEventListener('click', handleClickOutside);
})

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

function convertTestDuration(data: Array<{ Key: string; Value: number | null }>): SpeedTestTestDuration {
  const result: SpeedTestTestDuration = {};
  for (const item of data) {
    result[item.Key as keyof SpeedTestTestDuration] = item.Value !== null ? Number(item.Value) : undefined;
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

// Remove event listener on component unmount
onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside);
})

function onCreate(response: any) {
  router.push("/agent/" + state.agent.id + "/speedtests")
}

function onError(response: any) {
  state.submitting = false;
  alert('Error: ' + response)
}

async function submit() {
  if (!isFormValid.value || state.submitting) return;

  state.submitting = true;

  try {
    if (state.customServerEnable) {
      await probeService.updateFirstProbeTarget(state.speedtestProbe.id, state.customServer.trim());
    } else if (state.selected) {
      await probeService.updateFirstProbeTarget(state.speedtestProbe.id, state.selected.value);
    }
    onCreate(null);
  } catch (error) {
    onError(error);
  }
}
</script>

<template>
  <div class="container-fluid">
    <Title
        :history="[
          {title: 'workspaces', link: '/workspaces'}, 
          {title: state.site.name || 'Loading...', link: `/workspace/${state.site.id}`}, 
          {title: state.agent.name || 'Loading...', link: `/agent/${state.agent.id}`}, 
          {title: 'Speedtests', link: `/agent/${state.agent.id}/speedtests`}
        ]"
        :subtitle="`Configure and run a network speed test`"
        title="New Speedtest">
      <div class="status-indicator">
        <i class="fa-solid fa-gauge-high"></i>
        Ready to Test
      </div>
    </Title>

    <!-- Loading State -->
    <div v-if="state.loading" class="loading-container">
      <div class="spinner-border text-primary" role="status">
        <span class="visually-hidden">Loading...</span>
      </div>
      <p class="loading-text">Loading server list...</p>
    </div>

    <!-- Main Form -->
    <div v-else class="speedtest-form">
      <!-- Server Selection Section -->
      <div class="form-section" :class="{ 'disabled': state.customServerEnable }">
        <div class="section-header">
          <h5 class="section-title">
            <i class="fa-solid fa-server"></i>
            Select Test Server
          </h5>
          <span class="server-count" v-if="state.options.length > 0">
            {{ state.options.length }} servers available
          </span>
        </div>

        <div class="server-selector" ref="dropdownRef">
          <div class="search-container">
            <i class="fa-solid fa-search search-icon"></i>
            <input
                ref="inputRef"
                type="text"
                class="form-control search-input"
                v-model="searchQuery"
                @focus="isDropdownOpen = true"
                placeholder="Search servers by location, sponsor, or country..."
                :disabled="state.customServerEnable"
            >
            <button 
              v-if="searchQuery && !state.customServerEnable" 
              @click="clearSelection"
              class="clear-btn"
              type="button"
            >
              <i class="fa-solid fa-times"></i>
            </button>
          </div>

          <!-- Selected Server Display -->
          <div v-if="state.selected && !state.customServerEnable" class="selected-server">
            <div class="server-icon">
              <i class="fa-solid fa-server"></i>
            </div>
            <div class="server-details">
              <div class="server-name">{{ selectedServer?.sponsor }}</div>
              <div class="server-location">
                <i class="fa-solid fa-location-dot"></i>
                {{ selectedServer?.location }}
                <span class="server-distance">{{ selectedServer?.distance }} away</span>
              </div>
            </div>
          </div>

          <!-- Dropdown Menu -->
          <div class="dropdown-menu custom-dropdown" :class="{ show: isDropdownOpen && !state.customServerEnable }">
            <div v-if="filteredOptions.length === 0" class="no-results">
              <i class="fa-solid fa-search"></i>
              <p>No servers found matching "{{ searchQuery }}"</p>
            </div>
            <div v-else class="dropdown-list">
              <button 
                v-for="option in filteredOptions" 
                :key="option.value"
                class="dropdown-item server-option"
                :class="{ 'selected': state.selected?.value === option.value }"
                @click="selectOption(option)"
                type="button"
              >
                <div class="option-content">
                  <div class="option-main">
                    <i class="fa-solid fa-server"></i>
                    <span>{{ option.text.split(' - ')[1] || option.text }}</span>
                  </div>
                  <div class="option-distance">
                    {{ option.text.split(' - ')[0] }}
                  </div>
                </div>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Custom Server Section -->
      <div class="form-section">
        <div class="section-header">
          <h5 class="section-title">
            <i class="fa-solid fa-cog"></i>
            Custom Server Configuration
          </h5>
        </div>

        <div class="custom-server-container">
          <div class="form-check custom-toggle">
            <input 
              id="customServerEnable" 
              v-model="state.customServerEnable" 
              class="form-check-input"
              type="checkbox"
            >
            <label class="form-check-label" for="customServerEnable">
              Use Custom Server ID
              <span class="text-muted">Override automatic server selection</span>
            </label>
          </div>

          <transition name="slide">
            <div v-if="state.customServerEnable" class="custom-input-container">
              <div class="input-group">
                <span class="input-group-text">
                  <i class="fa-solid fa-hashtag"></i>
                </span>
                <input 
                  id="serverID" 
                  class="form-control" 
                  v-model="state.customServer" 
                  placeholder="Enter SpeedTest.net Server ID (e.g., 12345)" 
                  type="text"
                >
              </div>
              <small class="form-text text-muted">
                Enter a specific SpeedTest.net server ID to test against that server
              </small>
            </div>
          </transition>
        </div>
      </div>

      <!-- Action Buttons -->
      <div class="form-actions">
        <router-link 
          :to="`/agent/${state.agent.id}/speedtests`" 
          class="btn btn-outline-secondary"
        >
          <i class="fa-solid fa-times"></i>
          Cancel
        </router-link>
        <button 
          class="btn btn-primary" 
          @click="submit"
          :disabled="!isFormValid || state.submitting"
        >
          <span v-if="!state.submitting">
            <i class="fa-solid fa-play"></i>
            Run Speedtest
          </span>
          <span v-else>
            <span class="spinner-border spinner-border-sm me-2" role="status"></span>
            Starting Test...
          </span>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Loading State */
.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 400px;
  background: white;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
}

.loading-text {
  margin-top: 1rem;
  color: #6b7280;
}

/* Status Indicator */
.status-indicator {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.375rem 0.875rem;
  background: #dbeafe;
  color: #1e40af;
  border-radius: 999px;
  font-size: 0.875rem;
  font-weight: 500;
}

/* Form Container */
.speedtest-form {
  background: white;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  overflow: hidden;
}

/* Form Sections */
.form-section {
  padding: 1.5rem;
  border-bottom: 1px solid #e5e7eb;
  transition: opacity 0.3s;
}

.form-section:last-of-type {
  border-bottom: none;
}

.form-section.disabled {
  opacity: 0.5;
  pointer-events: none;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1.25rem;
}

.section-title {
  margin: 0;
  font-size: 1.125rem;
  font-weight: 600;
  color: #1f2937;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.section-title i {
  color: #6b7280;
}

.server-count {
  font-size: 0.875rem;
  color: #6b7280;
  background: #f3f4f6;
  padding: 0.25rem 0.75rem;
  border-radius: 999px;
}

/* Server Selector */
.server-selector {
  position: relative;
}

.search-container {
  position: relative;
}

.search-icon {
  position: absolute;
  left: 1rem;
  top: 50%;
  transform: translateY(-50%);
  color: #6b7280;
  pointer-events: none;
}

.search-input {
  padding-left: 2.75rem;
  padding-right: 2.75rem;
  height: 3rem;
  border-radius: 8px;
  border: 2px solid #e5e7eb;
  transition: all 0.2s;
}

.search-input:focus {
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
  outline: none;
}

.search-input.selected {
  animation: pulse 0.3s ease-out;
}

@keyframes pulse {
  0% { transform: scale(1); }
  50% { transform: scale(1.01); }
  100% { transform: scale(1); }
}

.clear-btn {
  position: absolute;
  right: 1rem;
  top: 50%;
  transform: translateY(-50%);
  background: none;
  border: none;
  color: #6b7280;
  cursor: pointer;
  padding: 0.25rem;
  transition: color 0.2s;
}

.clear-btn:hover {
  color: #374151;
}

/* Selected Server Display */
.selected-server {
  display: flex;
  align-items: center;
  gap: 1rem;
  margin-top: 1rem;
  padding: 1rem;
  background: #f0f9ff;
  border: 2px solid #3b82f6;
  border-radius: 8px;
}

.server-icon {
  width: 2.5rem;
  height: 2.5rem;
  background: #3b82f6;
  color: white;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.server-details {
  flex: 1;
}

.server-name {
  font-weight: 600;
  color: #1f2937;
  margin-bottom: 0.25rem;
}

.server-location {
  font-size: 0.875rem;
  color: #6b7280;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.server-distance {
  margin-left: 0.5rem;
  padding: 0.125rem 0.5rem;
  background: #e0e7ff;
  color: #3730a3;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 500;
}

/* Dropdown */
.custom-dropdown {
  position: absolute;
  top: 100%;
  left: 0;
  right: 0;
  margin-top: 0.5rem;
  max-height: 300px;
  overflow-y: auto;
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.1);
  z-index: 1000;
}

.dropdown-list {
  padding: 0.5rem;
}

.server-option {
  width: 100%;
  text-align: left;
  padding: 0.75rem;
  margin-bottom: 0.25rem;
  background: white;
  border: 1px solid transparent;
  border-radius: 6px;
  transition: all 0.2s;
  cursor: pointer;
}

.server-option:hover {
  background: #f3f4f6;
  border-color: #e5e7eb;
}

.server-option.selected {
  background: #eff6ff;
  border-color: #3b82f6;
}

.option-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.option-main {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  color: #1f2937;
}

.option-main i {
  color: #6b7280;
}

.option-distance {
  font-size: 0.875rem;
  color: #6b7280;
  font-weight: 500;
}

.no-results {
  text-align: center;
  padding: 2rem;
  color: #6b7280;
}

.no-results i {
  font-size: 2rem;
  margin-bottom: 0.5rem;
  opacity: 0.5;
}

/* Custom Server */
.custom-server-container {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.custom-toggle {
  padding: 1rem;
  background: #f9fafb;
  border-radius: 8px;
  margin-bottom: 0;
}

.custom-toggle .form-check-input {
  width: 1.25rem;
  height: 1.25rem;
  margin-top: 0.125rem;
}

.custom-toggle .form-check-label {
  margin-left: 0.5rem;
  font-weight: 500;
  color: #1f2937;
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.custom-toggle .text-muted {
  font-size: 0.875rem;
  font-weight: 400;
}

.custom-input-container {
  padding: 1rem;
  background: #f9fafb;
  border-radius: 8px;
}

.input-group-text {
  background: white;
  border-right: none;
}

.custom-input-container .form-control {
  border-left: none;
}

.custom-input-container .form-control:focus {
  box-shadow: none;
  border-color: #3b82f6;
}

/* Form Actions */
.form-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1.5rem;
  background: #f9fafb;
  border-top: 1px solid #e5e7eb;
}

/* Transitions */
.slide-enter-active,
.slide-leave-active {
  transition: all 0.3s ease;
}

.slide-enter-from {
  transform: translateY(-10px);
  opacity: 0;
}

.slide-leave-to {
  transform: translateY(-10px);
  opacity: 0;
}

/* Responsive */
@media (max-width: 768px) {
  .form-section {
    padding: 1.25rem;
  }
  
  .form-actions {
    flex-direction: column-reverse;
    gap: 1rem;
  }
  
  .form-actions .btn {
    width: 100%;
  }
}
</style>