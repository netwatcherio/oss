<template>
  <div class="ip-lookup-panel">
    <!-- Search Input -->
    <div class="lookup-search">
      <div class="search-input-wrapper">
        <i class="bi bi-search"></i>
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Enter IP address or hostname..."
          @keyup.enter="performLookup"
          :disabled="loading"
        />
        <button 
          class="lookup-btn" 
          @click="performLookup" 
          :disabled="loading || !searchQuery.trim()"
        >
          <i v-if="loading" class="bi bi-arrow-clockwise spin"></i>
          <span v-else>Lookup</span>
        </button>
      </div>
      <div v-if="error" class="error-message">
        <i class="bi bi-exclamation-circle"></i>
        {{ error }}
      </div>
    </div>

    <!-- Results -->
    <div v-if="result" class="lookup-results">
      <!-- Header -->
      <div class="result-header">
        <div class="result-ip">
          <span v-if="geoip?.country?.code" class="country-flag">{{ countryFlag }}</span>
          <span class="ip-text">{{ result.ip }}</span>
          <span v-if="result.cached" class="cached-badge">
            <i class="bi bi-database"></i> Cached
          </span>
        </div>
        <div v-if="result.cache_time" class="cache-time">
          Cached {{ formatCacheTime(result.cache_time) }}
        </div>
      </div>

      <!-- GeoIP Section -->
      <div v-if="geoip" class="result-section">
        <div class="section-header">
          <i class="bi bi-geo-alt"></i>
          <span>Geolocation</span>
        </div>
        <div class="info-grid">
          <div class="info-item" v-if="geoip.country">
            <span class="label">Country</span>
            <span class="value">
              {{ countryFlag }} {{ geoip.country.name || geoip.country.code }}
            </span>
          </div>
          <div class="info-item" v-if="geoip.city?.name">
            <span class="label">City</span>
            <span class="value">
              {{ geoip.city.name }}
              <span v-if="geoip.city.subdivision">, {{ geoip.city.subdivision }}</span>
            </span>
          </div>
          <div class="info-item" v-if="geoip.asn">
            <span class="label">ASN</span>
            <span class="value asn-value">
              <span v-if="geoip.asn.number" class="asn-number">AS{{ geoip.asn.number }}</span>
              <span v-if="geoip.asn.organization" class="asn-org">{{ geoip.asn.organization }}</span>
            </span>
          </div>
          <div class="info-item" v-if="geoip.coordinates">
            <span class="label">Coordinates</span>
            <span class="value coords">
              {{ geoip.coordinates.latitude.toFixed(4) }}, {{ geoip.coordinates.longitude.toFixed(4) }}
              <a 
                :href="`https://www.google.com/maps?q=${geoip.coordinates.latitude},${geoip.coordinates.longitude}`"
                target="_blank"
                class="map-link"
              >
                <i class="bi bi-box-arrow-up-right"></i>
              </a>
            </span>
          </div>
        </div>
      </div>

      <!-- WHOIS Section -->
      <div v-if="whoisData" class="result-section">
        <div class="section-header" @click="whoisExpanded = !whoisExpanded">
          <i class="bi bi-file-text"></i>
          <span>WHOIS</span>
          <i :class="whoisExpanded ? 'bi bi-chevron-up' : 'bi bi-chevron-down'" class="expand-icon"></i>
        </div>
        
        <div v-if="whoisExpanded" class="whois-content">
          <div class="info-grid">
            <div class="info-item" v-if="whoisData.parsed?.organization">
              <span class="label">Organization</span>
              <span class="value">{{ whoisData.parsed.organization }}</span>
            </div>
            <div class="info-item" v-if="whoisData.parsed?.netname">
              <span class="label">Network Name</span>
              <span class="value">{{ whoisData.parsed.netname }}</span>
            </div>
            <div class="info-item" v-if="whoisData.parsed?.netrange">
              <span class="label">Network Range</span>
              <span class="value mono">{{ whoisData.parsed.netrange }}</span>
            </div>
            <div class="info-item" v-if="whoisData.parsed?.country">
              <span class="label">Country</span>
              <span class="value">{{ whoisData.parsed.country }}</span>
            </div>
            <div class="info-item" v-if="whoisData.parsed?.registrar">
              <span class="label">Registrar</span>
              <span class="value">{{ whoisData.parsed.registrar }}</span>
            </div>
            <div class="info-item" v-if="whoisData.parsed?.abuse_email">
              <span class="label">Abuse Contact</span>
              <span class="value">
                <a :href="`mailto:${whoisData.parsed.abuse_email}`">{{ whoisData.parsed.abuse_email }}</a>
              </span>
            </div>
            <div class="info-item" v-if="whoisData.parsed?.created">
              <span class="label">Created</span>
              <span class="value">{{ whoisData.parsed.created }}</span>
            </div>
            <div class="info-item" v-if="whoisData.parsed?.updated">
              <span class="label">Updated</span>
              <span class="value">{{ whoisData.parsed.updated }}</span>
            </div>
          </div>

          <!-- Raw WHOIS Toggle -->
          <div class="raw-toggle" @click="showRawWhois = !showRawWhois">
            <i :class="showRawWhois ? 'bi bi-chevron-up' : 'bi bi-chevron-down'"></i>
            {{ showRawWhois ? 'Hide' : 'Show' }} Raw WHOIS Output
          </div>
          <div v-if="showRawWhois" class="raw-whois">
            <button class="copy-btn" @click="copyRawWhois">
              <i class="bi bi-clipboard"></i> Copy
            </button>
            <pre>{{ whoisData.raw_output }}</pre>
          </div>
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <div v-else-if="!loading && !error" class="empty-state">
      <i class="bi bi-globe"></i>
      <p>Enter an IP address to lookup geolocation and WHOIS information</p>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed } from 'vue';
import type { IPLookupResult, GeoIPResult, WhoisResult } from '@/types';
import { lookupCombined, countryCodeToFlag, isValidIP } from '@/services/lookupService';

const props = defineProps<{
  initialIp?: string;
}>();

const emit = defineEmits<{
  (e: 'lookup', ip: string): void;
  (e: 'result', result: IPLookupResult): void;
}>();

const searchQuery = ref(props.initialIp || '');
const result = ref<IPLookupResult | null>(null);
const loading = ref(false);
const error = ref('');
const whoisExpanded = ref(true);
const showRawWhois = ref(false);

const geoip = computed(() => result.value?.geoip as GeoIPResult | undefined);
const whoisData = computed(() => result.value?.whois as WhoisResult | undefined);

const countryFlag = computed(() => {
  if (!geoip.value?.country?.code) return '🌐';
  return countryCodeToFlag(geoip.value.country.code);
});

async function performLookup() {
  const query = searchQuery.value.trim();
  if (!query) return;

  // Basic validation
  if (!isValidIP(query)) {
    // Try DNS lookup or just proceed - backend will validate
  }

  loading.value = true;
  error.value = '';
  result.value = null;

  try {
    const lookupResult = await lookupCombined(query);
    result.value = lookupResult;
    emit('lookup', query);
    emit('result', lookupResult);
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Lookup failed';
    console.error('IP lookup error:', err);
  } finally {
    loading.value = false;
  }
}

function formatCacheTime(isoTime: string): string {
  const date = new Date(isoTime);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMins / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffMins < 1) return 'just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${diffDays}d ago`;
}

function copyRawWhois() {
  if (whoisData.value?.raw_output) {
    navigator.clipboard.writeText(whoisData.value.raw_output);
  }
}

// Perform initial lookup if IP provided
if (props.initialIp) {
  performLookup();
}

// Expose lookup function for parent components
defineExpose({ performLookup, searchQuery });
</script>

<style scoped>
.ip-lookup-panel {
  background: linear-gradient(180deg, #1a1b26 0%, #16161e 100%);
  border-radius: 12px;
  border: 1px solid #2a2b3d;
  overflow: hidden;
}

.lookup-search {
  padding: 1.25rem;
  background: linear-gradient(135deg, #1e1f2e 0%, #252636 100%);
  border-bottom: 1px solid #2a2b3d;
}

.search-input-wrapper {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  background: #1a1b26;
  border: 1px solid #3d3e50;
  border-radius: 8px;
  padding: 0.5rem 0.75rem;
  transition: border-color 0.2s;
}

.search-input-wrapper:focus-within {
  border-color: #7aa2f7;
}

.search-input-wrapper i {
  color: #565f89;
  font-size: 1rem;
}

.search-input-wrapper input {
  flex: 1;
  background: transparent;
  border: none;
  color: #c0caf5;
  font-size: 0.95rem;
  outline: none;
}

.search-input-wrapper input::placeholder {
  color: #565f89;
}

.lookup-btn {
  background: linear-gradient(135deg, #7aa2f7 0%, #5d7bd5 100%);
  color: #fff;
  border: none;
  padding: 0.5rem 1rem;
  border-radius: 6px;
  font-weight: 600;
  font-size: 0.85rem;
  cursor: pointer;
  transition: all 0.2s;
}

.lookup-btn:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(122, 162, 247, 0.3);
}

.lookup-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.error-message {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.75rem;
  padding: 0.75rem;
  background: rgba(247, 118, 142, 0.1);
  border: 1px solid rgba(247, 118, 142, 0.2);
  border-radius: 6px;
  color: #f7768e;
  font-size: 0.9rem;
}

.lookup-results {
  padding: 1.25rem;
}

.result-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1.25rem;
  padding-bottom: 1rem;
  border-bottom: 1px solid #2a2b3d;
}

.result-ip {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.country-flag {
  font-size: 1.5rem;
}

.ip-text {
  font-size: 1.25rem;
  font-weight: 600;
  color: #c0caf5;
  font-family: 'Monaco', 'Menlo', monospace;
}

.cached-badge {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.25rem 0.6rem;
  background: rgba(158, 206, 106, 0.15);
  color: #9ece6a;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 500;
}

.cache-time {
  font-size: 0.8rem;
  color: #565f89;
}

.result-section {
  margin-bottom: 1rem;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem 1rem;
  background: linear-gradient(135deg, #1e1f2e 0%, #252636 100%);
  border-radius: 8px 8px 0 0;
  color: #7aa2f7;
  font-weight: 600;
  font-size: 0.9rem;
  cursor: pointer;
  border: 1px solid #3d3e50;
  border-bottom: none;
}

.section-header .expand-icon {
  margin-left: auto;
  font-size: 0.8rem;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 0.75rem;
  padding: 1rem;
  background: #1a1b26;
  border: 1px solid #3d3e50;
  border-radius: 0 0 8px 8px;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.info-item .label {
  font-size: 0.75rem;
  color: #565f89;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.info-item .value {
  color: #c0caf5;
  font-size: 0.9rem;
}

.info-item .value.mono {
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 0.85rem;
}

.asn-value {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
}

.asn-number {
  padding: 0.15rem 0.4rem;
  background: rgba(122, 162, 247, 0.15);
  color: #7aa2f7;
  border-radius: 4px;
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 0.8rem;
}

.asn-org {
  color: #9aa5ce;
}

.coords {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 0.85rem;
}

.map-link {
  color: #7aa2f7;
  font-size: 0.8rem;
}

.map-link:hover {
  color: #9aa5ce;
}

.whois-content {
  padding: 1rem;
  background: #1a1b26;
  border: 1px solid #3d3e50;
  border-top: none;
  border-radius: 0 0 8px 8px;
}

.raw-toggle {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 1rem;
  padding: 0.6rem 0;
  border-top: 1px solid #2a2b3d;
  color: #565f89;
  font-size: 0.85rem;
  cursor: pointer;
  transition: color 0.2s;
}

.raw-toggle:hover {
  color: #7aa2f7;
}

.raw-whois {
  position: relative;
  margin-top: 0.75rem;
}

.raw-whois pre {
  background: #16161e;
  border: 1px solid #2a2b3d;
  border-radius: 6px;
  padding: 1rem;
  max-height: 300px;
  overflow: auto;
  font-size: 0.8rem;
  color: #9aa5ce;
  white-space: pre-wrap;
  word-break: break-all;
}

.copy-btn {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  display: flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.35rem 0.65rem;
  background: #252636;
  border: 1px solid #3d3e50;
  border-radius: 4px;
  color: #7aa2f7;
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.2s;
}

.copy-btn:hover {
  background: #3d3e50;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 3rem 2rem;
  color: #565f89;
}

.empty-state i {
  font-size: 3rem;
  margin-bottom: 1rem;
  opacity: 0.5;
}

.empty-state p {
  font-size: 0.95rem;
  text-align: center;
}

/* Link styling */
a {
  color: #7aa2f7;
  text-decoration: none;
}

a:hover {
  text-decoration: underline;
}
</style>
