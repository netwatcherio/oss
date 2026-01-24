<script lang="ts" setup>
import { ref, onMounted, computed } from 'vue';
import { useRoute } from 'vue-router';
import { PublicShareService } from '@/services/apiService';
import { since } from '@/time';

const route = useRoute();
const token = computed(() => route.params.token as string);

// State
const loading = ref(true);
const error = ref<string | null>(null);
const requiresPassword = ref(false);
const passwordInput = ref('');
const passwordError = ref<string | null>(null);
const expired = ref(false);
const expiresAt = ref<string | null>(null);
const allowSpeedtest = ref(false);

// Agent data
const agent = ref<any>(null);
const probes = ref<any[]>([]);

// Password submission
async function submitPassword() {
    passwordError.value = null;
    try {
        const result = await PublicShareService.getAgent(token.value, passwordInput.value);
        agent.value = result.agent;
        probes.value = result.probes;
        expiresAt.value = result.expires_at;
        allowSpeedtest.value = result.allow_speedtest;
        requiresPassword.value = false;
    } catch (err: any) {
        if (err.message === 'INVALID_PASSWORD') {
            passwordError.value = 'Incorrect password. Please try again.';
        } else {
            error.value = err.message || 'Failed to access shared agent';
        }
    }
}

// Format date
function formatDate(dateStr: string): string {
    return new Date(dateStr).toLocaleString();
}

// Get status based on last update
function getAgentStatus(agent: any): 'online' | 'stale' | 'offline' {
    if (!agent?.updated_at) return 'offline';
    const updated = new Date(agent.updated_at);
    const now = new Date();
    const diffMinutes = (now.getTime() - updated.getTime()) / (1000 * 60);
    if (diffMinutes < 5) return 'online';
    if (diffMinutes < 30) return 'stale';
    return 'offline';
}

// Get status color
function getStatusColor(status: string): string {
    switch (status) {
        case 'online': return '#22c55e';
        case 'stale': return '#f59e0b';
        default: return '#ef4444';
    }
}

// Get status label
function getStatusLabel(status: string): string {
    switch (status) {
        case 'online': return 'Online';
        case 'stale': return 'Stale';
        default: return 'Offline';
    }
}

// Load shared agent data
async function loadAgent() {
    loading.value = true;
    error.value = null;
    
    try {
        // First check if password is required
        const info = await PublicShareService.getInfo(token.value);
        
        if (info.expired) {
            expired.value = true;
            expiresAt.value = info.expires_at;
            loading.value = false;
            return;
        }
        
        if (info.has_password) {
            requiresPassword.value = true;
            expiresAt.value = info.expires_at;
            loading.value = false;
            return;
        }
        
        // No password required, load directly
        const result = await PublicShareService.getAgent(token.value);
        agent.value = result.agent;
        probes.value = result.probes;
        expiresAt.value = result.expires_at;
        allowSpeedtest.value = result.allow_speedtest;
    } catch (err: any) {
        if (err.message === 'PASSWORD_REQUIRED') {
            requiresPassword.value = true;
        } else if (err.message === 'LINK_EXPIRED') {
            expired.value = true;
        } else if (err.message === 'LINK_NOT_FOUND') {
            error.value = 'This share link does not exist or has been revoked.';
        } else {
            error.value = err.message || 'Failed to load shared agent';
        }
    } finally {
        loading.value = false;
    }
}

onMounted(() => {
    loadAgent();
});
</script>

<template>
    <div class="shared-agent-page">
        <!-- Header -->
        <header class="shared-header">
            <div class="header-content">
                <div class="brand">
                    <i class="bi bi-eye"></i>
                    <span>NetWatcher</span>
                    <span class="badge">Shared View</span>
                </div>
            </div>
        </header>
        
        <main class="shared-main">
            <!-- Loading State -->
            <div v-if="loading" class="loading-state">
                <i class="bi bi-arrow-repeat spin"></i>
                <p>Loading shared agent...</p>
            </div>
            
            <!-- Error State -->
            <div v-else-if="error" class="error-state">
                <i class="bi bi-exclamation-triangle"></i>
                <h2>Unable to Access</h2>
                <p>{{ error }}</p>
            </div>
            
            <!-- Expired State -->
            <div v-else-if="expired" class="expired-state">
                <i class="bi bi-clock-history"></i>
                <h2>Link Expired</h2>
                <p>This share link expired on {{ expiresAt ? formatDate(expiresAt) : 'an unknown date' }}.</p>
                <p class="subtext">Contact the link owner to request a new share link.</p>
            </div>
            
            <!-- Password Required -->
            <div v-else-if="requiresPassword" class="password-state">
                <div class="password-card">
                    <i class="bi bi-lock"></i>
                    <h2>Password Protected</h2>
                    <p>This share link requires a password to access.</p>
                    
                    <form @submit.prevent="submitPassword" class="password-form">
                        <div v-if="passwordError" class="password-error">
                            <i class="bi bi-exclamation-circle"></i>
                            {{ passwordError }}
                        </div>
                        <input 
                            type="password" 
                            v-model="passwordInput"
                            placeholder="Enter password"
                            class="password-input"
                            autofocus
                        />
                        <button type="submit" class="password-btn" :disabled="!passwordInput">
                            <i class="bi bi-unlock"></i>
                            Access Agent
                        </button>
                    </form>
                </div>
            </div>
            
            <!-- Agent Data -->
            <div v-else-if="agent" class="agent-content">
                <!-- Expiry Warning -->
                <div v-if="expiresAt" class="expiry-notice">
                    <i class="bi bi-clock"></i>
                    This link expires {{ since(expiresAt, true) }}
                </div>
                
                <!-- Speedtest Capability Notice -->
                <div v-if="allowSpeedtest" class="speedtest-notice">
                    <i class="bi bi-speedometer2"></i>
                    Short-term share: Speedtest capability enabled
                </div>
                
                <!-- Agent Header -->
                <div class="agent-header">
                    <div class="agent-info">
                        <h1>{{ agent.name }}</h1>
                        <p v-if="agent.description">{{ agent.description }}</p>
                        <p v-if="agent.location" class="location">
                            <i class="bi bi-geo-alt"></i>
                            {{ agent.location }}
                        </p>
                    </div>
                    <div class="agent-status" :style="{ '--status-color': getStatusColor(getAgentStatus(agent)) }">
                        <span class="status-dot"></span>
                        {{ getStatusLabel(getAgentStatus(agent)) }}
                    </div>
                </div>
                
                <!-- Agent Details Card -->
                <div class="info-grid">
                    <div class="info-card">
                        <div class="info-label">Public IP</div>
                        <div class="info-value">{{ agent.public_ip || 'N/A' }}</div>
                    </div>
                    <div class="info-card">
                        <div class="info-label">Version</div>
                        <div class="info-value">{{ agent.version || 'N/A' }}</div>
                    </div>
                    <div class="info-card">
                        <div class="info-label">Last Seen</div>
                        <div class="info-value">{{ agent.last_seen_at ? since(agent.last_seen_at, true) : 'Never' }}</div>
                    </div>
                    <div class="info-card">
                        <div class="info-label">Status</div>
                        <div class="info-value">{{ agent.initialized ? 'Initialized' : 'Pending Setup' }}</div>
                    </div>
                </div>
                
                <!-- Probes Section -->
                <div class="probes-section">
                    <h2>
                        <i class="bi bi-diagram-3"></i>
                        Active Probes
                        <span class="probe-count">{{ probes.length }}</span>
                    </h2>
                    
                    <div v-if="probes.length === 0" class="no-probes">
                        <i class="bi bi-inbox"></i>
                        <p>No probes configured for this agent.</p>
                    </div>
                    
                    <div v-else class="probes-grid">
                        <div v-for="probe in probes" :key="probe.id" class="probe-card">
                            <div class="probe-header">
                                <span class="probe-type" :class="probe.type.toLowerCase()">
                                    {{ probe.type }}
                                </span>
                                <span v-if="!probe.enabled" class="probe-disabled">
                                    Disabled
                                </span>
                            </div>
                            <div class="probe-name">{{ probe.name || `Probe #${probe.id}` }}</div>
                            <div v-if="probe.targets && probe.targets.length > 0" class="probe-targets">
                                <span v-for="(target, idx) in probe.targets.slice(0, 3)" :key="idx" class="target-badge">
                                    {{ target.target || target.agent_id ? `Agent #${target.agent_id}` : 'N/A' }}
                                </span>
                                <span v-if="probe.targets.length > 3" class="more-targets">
                                    +{{ probe.targets.length - 3 }} more
                                </span>
                            </div>
                            <div class="probe-meta">
                                <span><i class="bi bi-clock"></i> {{ probe.interval }}s interval</span>
                            </div>
                        </div>
                    </div>
                </div>
                
                <!-- Footer Notice -->
                <div class="shared-footer">
                    <p>
                        <i class="bi bi-info-circle"></i>
                        This is a read-only view. Data updates may be delayed.
                    </p>
                </div>
            </div>
        </main>
    </div>
</template>

<style scoped>
.shared-agent-page {
    min-height: 100vh;
    background: linear-gradient(135deg, #0f0f1a 0%, #1a1a2e 100%);
    color: #fff;
}

.shared-header {
    background: rgba(0, 0, 0, 0.3);
    border-bottom: 1px solid rgba(255, 255, 255, 0.1);
    padding: 1rem 1.5rem;
}

.header-content {
    max-width: 1200px;
    margin: 0 auto;
}

.brand {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 1.25rem;
    font-weight: 600;
}

.brand i {
    color: #6366f1;
}

.brand .badge {
    background: rgba(99, 102, 241, 0.2);
    color: #a5b4fc;
    padding: 0.25rem 0.625rem;
    border-radius: 4px;
    font-size: 0.7rem;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.5px;
}

.shared-main {
    max-width: 1200px;
    margin: 0 auto;
    padding: 2rem 1.5rem;
}

/* Loading State */
.loading-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 4rem 2rem;
    text-align: center;
    color: #888;
}

.loading-state i {
    font-size: 2.5rem;
    margin-bottom: 1rem;
}

/* Error / Expired States */
.error-state, .expired-state {
    text-align: center;
    padding: 4rem 2rem;
}

.error-state i, .expired-state i {
    font-size: 4rem;
    margin-bottom: 1.5rem;
    color: #ef4444;
}

.expired-state i {
    color: #f59e0b;
}

.error-state h2, .expired-state h2 {
    font-size: 1.5rem;
    margin-bottom: 0.75rem;
}

.error-state p, .expired-state p {
    color: #888;
    max-width: 400px;
    margin: 0 auto;
}

.subtext {
    margin-top: 1rem !important;
    font-size: 0.875rem;
}

/* Password State */
.password-state {
    display: flex;
    justify-content: center;
    padding: 3rem 1rem;
}

.password-card {
    background: rgba(255, 255, 255, 0.05);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 12px;
    padding: 2.5rem;
    text-align: center;
    max-width: 400px;
    width: 100%;
}

.password-card i {
    font-size: 3rem;
    color: #6366f1;
    margin-bottom: 1rem;
}

.password-card h2 {
    font-size: 1.25rem;
    margin-bottom: 0.5rem;
}

.password-card p {
    color: #888;
    font-size: 0.875rem;
    margin-bottom: 1.5rem;
}

.password-form {
    display: flex;
    flex-direction: column;
    gap: 1rem;
}

.password-error {
    background: rgba(239, 68, 68, 0.15);
    color: #ef4444;
    padding: 0.75rem;
    border-radius: 8px;
    font-size: 0.875rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

.password-input {
    background: rgba(0, 0, 0, 0.3);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 8px;
    padding: 0.875rem 1rem;
    color: #fff;
    font-size: 1rem;
}

.password-input:focus {
    outline: none;
    border-color: #6366f1;
}

.password-btn {
    background: linear-gradient(135deg, #6366f1, #8b5cf6);
    color: white;
    border: none;
    border-radius: 8px;
    padding: 0.875rem;
    font-weight: 500;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    transition: all 0.2s;
}

.password-btn:hover:not(:disabled) {
    transform: translateY(-1px);
    box-shadow: 0 4px 12px rgba(99, 102, 241, 0.4);
}

.password-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
}

/* Agent Content */
.expiry-notice {
    background: rgba(245, 158, 11, 0.15);
    border: 1px solid rgba(245, 158, 11, 0.3);
    color: #fbbf24;
    padding: 0.75rem 1rem;
    border-radius: 8px;
    margin-bottom: 1rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.875rem;
}

.speedtest-notice {
    background: rgba(34, 197, 94, 0.15);
    border: 1px solid rgba(34, 197, 94, 0.3);
    color: #86efac;
    padding: 0.75rem 1rem;
    border-radius: 8px;
    margin-bottom: 1.5rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.875rem;
}

.agent-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1.5rem;
    margin-bottom: 2rem;
    flex-wrap: wrap;
}

.agent-info h1 {
    font-size: 2rem;
    font-weight: 700;
    margin-bottom: 0.5rem;
}

.agent-info p {
    color: #9ca3af;
    margin-bottom: 0.25rem;
}

.agent-info .location {
    display: flex;
    align-items: center;
    gap: 0.375rem;
}

.agent-status {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    background: rgba(255, 255, 255, 0.05);
    padding: 0.5rem 1rem;
    border-radius: 999px;
    font-weight: 500;
}

.status-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: var(--status-color);
    box-shadow: 0 0 8px var(--status-color);
}

/* Info Grid */
.info-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1rem;
    margin-bottom: 2rem;
}

.info-card {
    background: rgba(255, 255, 255, 0.05);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 10px;
    padding: 1rem 1.25rem;
}

.info-label {
    font-size: 0.75rem;
    color: #888;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    margin-bottom: 0.375rem;
}

.info-value {
    font-size: 1rem;
    font-weight: 500;
}

/* Probes Section */
.probes-section h2 {
    font-size: 1.25rem;
    font-weight: 600;
    margin-bottom: 1.25rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

.probe-count {
    background: rgba(99, 102, 241, 0.2);
    color: #a5b4fc;
    padding: 0.125rem 0.5rem;
    border-radius: 4px;
    font-size: 0.8rem;
    font-weight: 500;
}

.no-probes {
    text-align: center;
    padding: 3rem;
    color: #666;
}

.no-probes i {
    font-size: 2.5rem;
    margin-bottom: 0.75rem;
}

.probes-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
    gap: 1rem;
}

.probe-card {
    background: rgba(255, 255, 255, 0.03);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 10px;
    padding: 1rem 1.25rem;
    transition: all 0.2s;
}

.probe-card:hover {
    border-color: rgba(99, 102, 241, 0.3);
    background: rgba(255, 255, 255, 0.05);
}

.probe-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.5rem;
}

.probe-type {
    padding: 0.25rem 0.625rem;
    border-radius: 4px;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    background: rgba(99, 102, 241, 0.2);
    color: #a5b4fc;
}

.probe-type.ping { background: rgba(34, 197, 94, 0.2); color: #86efac; }
.probe-type.mtr { background: rgba(59, 130, 246, 0.2); color: #93c5fd; }
.probe-type.trafficsim { background: rgba(168, 85, 247, 0.2); color: #d8b4fe; }

.probe-disabled {
    background: rgba(239, 68, 68, 0.15);
    color: #fca5a5;
    padding: 0.125rem 0.5rem;
    border-radius: 4px;
    font-size: 0.65rem;
    font-weight: 500;
}

.probe-name {
    font-weight: 500;
    margin-bottom: 0.5rem;
}

.probe-targets {
    display: flex;
    flex-wrap: wrap;
    gap: 0.375rem;
    margin-bottom: 0.5rem;
}

.target-badge {
    background: rgba(0, 0, 0, 0.3);
    padding: 0.25rem 0.5rem;
    border-radius: 4px;
    font-size: 0.75rem;
    color: #9ca3af;
}

.more-targets {
    color: #666;
    font-size: 0.75rem;
    padding: 0.25rem;
}

.probe-meta {
    font-size: 0.75rem;
    color: #666;
    display: flex;
    align-items: center;
    gap: 0.25rem;
}

/* Footer */
.shared-footer {
    margin-top: 3rem;
    padding-top: 1.5rem;
    border-top: 1px solid rgba(255, 255, 255, 0.1);
    text-align: center;
}

.shared-footer p {
    color: #666;
    font-size: 0.875rem;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
}

/* Spin Animation */
@keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
}

.spin {
    animation: spin 1s linear infinite;
}

/* Light Theme Support */
[data-theme="light"] .shared-agent-page {
    background: linear-gradient(135deg, #f8fafc 0%, #e2e8f0 100%);
    color: #1f2937;
}

[data-theme="light"] .shared-header {
    background: rgba(255, 255, 255, 0.8);
    border-bottom-color: #e5e7eb;
}

[data-theme="light"] .password-card,
[data-theme="light"] .info-card,
[data-theme="light"] .probe-card {
    background: rgba(255, 255, 255, 0.8);
    border-color: #e5e7eb;
}

[data-theme="light"] .password-input {
    background: white;
    border-color: #d1d5db;
    color: #1f2937;
}
</style>
