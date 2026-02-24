<script lang="ts" setup>
import { ref, computed, onMounted } from 'vue';
import { ShareLinkService, type ShareLink } from '@/services/apiService';

const props = defineProps<{
    workspaceId: number | string;
    agentId: number | string;
    agentName: string;
}>();

const emit = defineEmits<{
    (e: 'close'): void;
}>();

// State
const loading = ref(false);
const creating = ref(false);
const shareLinks = ref<ShareLink[]>([]);
const error = ref<string | null>(null);
const success = ref<string | null>(null);

// Form state
const expiryOption = ref('24h');
const customExpiryHours = ref(72);
const usePassword = ref(false);
const password = ref('');
const generatedLink = ref<string | null>(null);
const copiedLink = ref(false);

// Expiry options
const expiryOptions = [
    { label: '1 hour', value: '1h', seconds: 3600 },
    { label: '24 hours', value: '24h', seconds: 86400 },
    { label: '7 days', value: '7d', seconds: 604800 },
    { label: '30 days', value: '30d', seconds: 2592000 },
    { label: 'Custom', value: 'custom', seconds: 0 },
];

const expirySeconds = computed(() => {
    if (expiryOption.value === 'custom') {
        return customExpiryHours.value * 3600;
    }
    const option = expiryOptions.find(o => o.value === expiryOption.value);
    return option?.seconds || 86400;
});

// Load existing share links
async function loadShareLinks() {
    loading.value = true;
    error.value = null;
    try {
        shareLinks.value = await ShareLinkService.list(props.workspaceId, props.agentId);
    } catch (err: any) {
        error.value = err.message || 'Failed to load share links';
    } finally {
        loading.value = false;
    }
}

// Create new share link
async function createShareLink() {
    creating.value = true;
    error.value = null;
    success.value = null;
    try {
        const result = await ShareLinkService.create(props.workspaceId, props.agentId, {
            expires_in_seconds: expirySeconds.value,
            password: usePassword.value ? password.value : undefined,
        });
        
        // Generate the share URL
        const baseUrl = window.location.origin;
        generatedLink.value = `${baseUrl}/shared/${result.token}`;
        
        // Reload list
        await loadShareLinks();
        
        success.value = 'Share link created successfully!';
        
        // Reset form
        password.value = '';
        usePassword.value = false;
    } catch (err: any) {
        error.value = err.message || 'Failed to create share link';
    } finally {
        creating.value = false;
    }
}

// Revoke share link
async function revokeLink(link: ShareLink) {
    if (!confirm('Are you sure you want to revoke this share link? Anyone with this link will no longer be able to access the agent.')) {
        return;
    }
    
    try {
        await ShareLinkService.remove(props.workspaceId, props.agentId, link.id);
        await loadShareLinks();
        success.value = 'Share link revoked successfully';
        
        // Clear generated link if it was the revoked one
        if (generatedLink.value?.includes(link.token)) {
            generatedLink.value = null;
        }
    } catch (err: any) {
        error.value = err.message || 'Failed to revoke share link';
    }
}

// Copy link to clipboard
async function copyLink() {
    if (!generatedLink.value) return;
    
    try {
        await navigator.clipboard.writeText(generatedLink.value);
        copiedLink.value = true;
        setTimeout(() => { copiedLink.value = false; }, 2000);
    } catch {
        // Fallback
        const input = document.createElement('input');
        input.value = generatedLink.value;
        document.body.appendChild(input);
        input.select();
        document.execCommand('copy');
        document.body.removeChild(input);
        copiedLink.value = true;
        setTimeout(() => { copiedLink.value = false; }, 2000);
    }
}

// Format expiry time
function formatExpiry(expiresAt: string): string {
    const date = new Date(expiresAt);
    const now = new Date();
    const diffMs = date.getTime() - now.getTime();
    
    if (diffMs < 0) return 'Expired';
    
    const hours = Math.floor(diffMs / (1000 * 60 * 60));
    const days = Math.floor(hours / 24);
    
    if (days > 0) return `${days}d ${hours % 24}h remaining`;
    if (hours > 0) return `${hours}h remaining`;
    return 'Less than 1 hour';
}

// Format date
function formatDate(dateStr: string): string {
    return new Date(dateStr).toLocaleString();
}

// Check if link is expired
function isExpired(expiresAt: string): boolean {
    return new Date(expiresAt) < new Date();
}

onMounted(() => {
    loadShareLinks();
});
</script>

<template>
    <div class="modal-backdrop" @click.self="emit('close')">
        <div class="modal-container">
            <div class="modal-header">
                <h3><i class="bi bi-link-45deg"></i> Share Agent</h3>
                <button class="close-btn" @click="emit('close')">
                    <i class="bi bi-x-lg"></i>
                </button>
            </div>
            
            <div class="modal-body">
                <!-- Alerts -->
                <div v-if="error" class="alert alert-danger">
                    <i class="bi bi-exclamation-triangle"></i> {{ error }}
                </div>
                <div v-if="success" class="alert alert-success">
                    <i class="bi bi-check-circle"></i> {{ success }}
                </div>
                
                <!-- Generated Link -->
                <div v-if="generatedLink" class="generated-link-section">
                    <label>Share Link</label>
                    <div class="link-input-group">
                        <input type="text" readonly :value="generatedLink" class="link-input" />
                        <button class="copy-btn" @click="copyLink" :class="{ copied: copiedLink }">
                            <i :class="copiedLink ? 'bi bi-check' : 'bi bi-clipboard'"></i>
                            {{ copiedLink ? 'Copied!' : 'Copy' }}
                        </button>
                    </div>
                    <p class="link-note">
                        <i class="bi bi-info-circle"></i>
                        Anyone with this link can view this agent's status and probe data (read-only).
                    </p>
                </div>
                
                <!-- Create New Link Form -->
                <div class="create-section">
                    <h4>Create New Share Link</h4>
                    
                    <div class="form-group">
                        <label>Expires In</label>
                        <div class="expiry-options">
                            <button 
                                v-for="opt in expiryOptions" 
                                :key="opt.value"
                                type="button"
                                class="expiry-btn"
                                :class="{ active: expiryOption === opt.value }"
                                @click.stop="expiryOption = opt.value"
                            >
                                {{ opt.label }}
                            </button>
                        </div>
                        
                        <div v-if="expiryOption === 'custom'" class="custom-expiry">
                            <input 
                                type="number" 
                                v-model.number="customExpiryHours" 
                                min="1" 
                                max="720"
                                class="form-control"
                            />
                            <span class="expiry-unit">hours</span>
                        </div>
                    </div>
                    
                    <div class="form-group">
                        <label class="checkbox-label">
                            <input type="checkbox" v-model="usePassword" />
                            <span>Password protect this link</span>
                        </label>
                        
                        <input 
                            v-if="usePassword"
                            type="password"
                            v-model="password"
                            placeholder="Enter password"
                            class="form-control password-input"
                        />
                    </div>
                    
                    <button 
                        class="btn btn-primary create-btn" 
                        @click="createShareLink"
                        :disabled="creating || (usePassword && !password)"
                    >
                        <i v-if="creating" class="bi bi-arrow-repeat spin"></i>
                        <i v-else class="bi bi-plus-circle"></i>
                        {{ creating ? 'Creating...' : 'Create Share Link' }}
                    </button>
                </div>
                
                <!-- Existing Links -->
                <div class="existing-links-section" v-if="shareLinks.length > 0">
                    <h4>Active Share Links</h4>
                    
                    <div class="share-links-list">
                        <div 
                            v-for="link in shareLinks" 
                            :key="link.id" 
                            class="share-link-item"
                            :class="{ expired: isExpired(link.expires_at) }"
                        >
                            <div class="link-info">
                                <div class="link-token">
                                    <i class="bi bi-link"></i>
                                    <code>{{ link.token.substring(0, 12) }}...</code>
                                    <span v-if="link.has_password" class="badge badge-password">
                                        <i class="bi bi-lock"></i> Protected
                                    </span>
                                </div>
                                <div class="link-meta">
                                    <span class="link-expiry" :class="{ 'text-danger': isExpired(link.expires_at) }">
                                        <i class="bi bi-clock"></i>
                                        {{ formatExpiry(link.expires_at) }}
                                    </span>
                                    <span class="link-views">
                                        <i class="bi bi-eye"></i>
                                        {{ link.access_count }} views
                                    </span>
                                    <span class="link-created">
                                        Created {{ formatDate(link.created_at) }}
                                    </span>
                                </div>
                            </div>
                            <button 
                                class="revoke-btn" 
                                @click="revokeLink(link)"
                                title="Revoke this link"
                            >
                                <i class="bi bi-trash"></i>
                            </button>
                        </div>
                    </div>
                </div>
                
                <div v-else-if="!loading" class="no-links">
                    <i class="bi bi-link-45deg"></i>
                    <p>No active share links for this agent.</p>
                </div>
                
                <div v-if="loading" class="loading-state">
                    <i class="bi bi-arrow-repeat spin"></i>
                    Loading share links...
                </div>
            </div>
        </div>
    </div>
</template>

<style scoped>
.modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(var(--bs-dark-rgb), 0.6);
    backdrop-filter: blur(4px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 9999;
    padding: 1rem;
}

.modal-container {
    background: var(--bs-body-bg);
    border-radius: 16px;
    width: 100%;
    max-width: 560px;
    max-height: 90vh;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    box-shadow: 0 24px 64px rgba(var(--bs-dark-rgb), 0.3);
    border: 1px solid var(--bs-border-color);
}

.modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1.25rem 1.5rem;
    border-bottom: 1px solid var(--bs-border-color);
    background: var(--bs-tertiary-bg);
}

.modal-header h3 {
    margin: 0;
    font-size: 1.15rem;
    font-weight: 600;
    color: var(--bs-body-color);
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

.close-btn {
    background: transparent;
    border: none;
    color: var(--bs-secondary-color);
    padding: 0.5rem;
    cursor: pointer;
    border-radius: 8px;
    transition: all 0.2s;
    width: 36px;
    height: 36px;
    display: flex;
    align-items: center;
    justify-content: center;
}

.close-btn:hover {
    color: var(--bs-body-color);
    background: var(--bs-secondary-bg);
}

.modal-body {
    padding: 1.25rem;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
}

/* Alerts */
.alert {
    padding: 0.875rem 1rem;
    border-radius: 8px;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.875rem;
}

.alert-danger {
    background: rgba(var(--bs-danger-rgb), 0.1);
    color: var(--bs-danger);
    border: 1px solid rgba(var(--bs-danger-rgb), 0.3);
}

.alert-success {
    background: rgba(var(--bs-success-rgb), 0.1);
    color: var(--bs-success);
    border: 1px solid rgba(var(--bs-success-rgb), 0.3);
}

/* Generated Link */
.generated-link-section {
    background: rgba(var(--bs-primary-rgb), 0.08);
    border: 1px solid rgba(var(--bs-primary-rgb), 0.3);
    border-radius: 12px;
    padding: 1.25rem;
}

.generated-link-section label {
    display: block;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    color: var(--bs-primary);
    margin-bottom: 0.5rem;
}

.link-input-group {
    display: flex;
    gap: 0.5rem;
}

.link-input {
    flex: 1;
    background: var(--bs-body-bg);
    border: 1px solid var(--bs-border-color);
    border-radius: 8px;
    padding: 0.625rem 0.75rem;
    color: var(--bs-body-color);
    font-family: monospace;
    font-size: 0.8rem;
}

.copy-btn {
    background: var(--bs-primary);
    color: var(--bs-white);
    border: none;
    border-radius: 8px;
    padding: 0.625rem 1rem;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 0.375rem;
    font-weight: 500;
    transition: all 0.2s;
}

.copy-btn:hover {
    background: color-mix(in srgb, var(--bs-primary) 85%, black);
}

.copy-btn.copied {
    background: var(--bs-success);
}

.link-note {
    margin: 0.75rem 0 0;
    font-size: 0.8rem;
    color: var(--bs-secondary-color);
    display: flex;
    align-items: flex-start;
    gap: 0.375rem;
}

/* Create Section */
.create-section {
    background: var(--bs-secondary-bg);
    border-radius: 12px;
    padding: 1.25rem;
}

.create-section h4 {
    margin: 0 0 1rem;
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--bs-body-color);
}

.form-group {
    margin-bottom: 1rem;
}

.form-group label {
    display: block;
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--bs-secondary-color);
    margin-bottom: 0.5rem;
}

.expiry-options {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
}

.expiry-btn {
    background: var(--bs-body-bg);
    border: 1px solid var(--bs-border-color);
    color: var(--bs-secondary-color);
    padding: 0.5rem 0.875rem;
    border-radius: 8px;
    font-size: 0.8rem;
    cursor: pointer;
    transition: all 0.2s;
}

.expiry-btn:hover {
    border-color: var(--bs-primary);
    color: var(--bs-primary);
    background: var(--bs-primary-bg-subtle);
}

.expiry-btn.active {
    background: var(--bs-primary);
    border-color: var(--bs-primary);
    color: var(--bs-white);
    box-shadow: 0 0 0 3px rgba(var(--bs-primary-rgb), 0.3);
    font-weight: 600;
    transform: translateY(-1px);
}

.custom-expiry {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.5rem;
}

.custom-expiry .form-control {
    width: 100px;
}

.expiry-unit {
    color: var(--bs-secondary-color);
    font-size: 0.875rem;
}

.checkbox-label {
    display: flex !important;
    align-items: center;
    gap: 0.5rem;
    cursor: pointer;
}

.checkbox-label input[type="checkbox"] {
    width: 16px;
    height: 16px;
    accent-color: var(--bs-primary);
}

.checkbox-label span {
    color: var(--bs-body-color);
}

.password-input {
    margin-top: 0.5rem;
}

.form-control {
    background: var(--bs-body-bg);
    border: 1px solid var(--bs-border-color);
    border-radius: 8px;
    padding: 0.625rem 0.75rem;
    color: var(--bs-body-color);
    width: 100%;
}

.form-control:focus {
    outline: none;
    border-color: var(--bs-primary);
    box-shadow: 0 0 0 2px rgba(var(--bs-primary-rgb), 0.15);
}

.create-btn {
    width: 100%;
    padding: 0.875rem;
    font-weight: 500;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
}

.btn-primary {
    background: var(--bs-primary);
    color: var(--bs-white);
    border: none;
    border-radius: 10px;
    cursor: pointer;
    transition: all 0.2s;
}

.btn-primary:hover:not(:disabled) {
    transform: translateY(-1px);
    box-shadow: 0 4px 12px rgba(var(--bs-primary-rgb), 0.4);
}

.btn-primary:disabled {
    opacity: 0.6;
    cursor: not-allowed;
}

/* Existing Links */
.existing-links-section h4 {
    margin: 0 0 0.75rem;
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--bs-body-color);
}

.share-links-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
}

.share-link-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    background: var(--bs-secondary-bg);
    border: 1px solid var(--bs-border-color);
    border-radius: 10px;
    padding: 0.875rem;
    transition: all 0.15s;
}

.share-link-item:hover {
    border-color: var(--bs-border-color-translucent);
}

.share-link-item.expired {
    opacity: 0.6;
}

.link-info {
    flex: 1;
    min-width: 0;
}

.link-token {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.25rem;
}

.link-token code {
    font-size: 0.8rem;
    color: var(--bs-body-color);
}

.badge-password {
    background: rgba(var(--bs-warning-rgb), 0.15);
    color: var(--bs-warning);
    padding: 0.125rem 0.5rem;
    border-radius: 4px;
    font-size: 0.7rem;
    display: flex;
    align-items: center;
    gap: 0.25rem;
}

.link-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
    font-size: 0.75rem;
    color: var(--bs-secondary-color);
}

.link-expiry, .link-views, .link-created {
    display: flex;
    align-items: center;
    gap: 0.25rem;
}

.text-danger {
    color: var(--bs-danger) !important;
}

.revoke-btn {
    background: transparent;
    border: 1px solid rgba(var(--bs-danger-rgb), 0.3);
    color: var(--bs-danger);
    padding: 0.5rem;
    border-radius: 8px;
    cursor: pointer;
    transition: all 0.2s;
}

.revoke-btn:hover {
    background: rgba(var(--bs-danger-rgb), 0.1);
}

/* No Links State */
.no-links {
    text-align: center;
    padding: 2rem;
    color: var(--bs-secondary-color);
}

.no-links i {
    font-size: 2rem;
    margin-bottom: 0.5rem;
    color: var(--bs-secondary-color);
    opacity: 0.5;
}

.no-links p {
    margin: 0;
}

/* Loading State */
.loading-state {
    text-align: center;
    padding: 1.5rem;
    color: var(--bs-secondary-color);
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

[data-theme="light"] .modal-header {
    border-bottom-color: #e5e7eb;
}

[data-theme="light"] .modal-header h3 {
    color: #1f2937;
}

[data-theme="light"] .close-btn {
    color: #9ca3af;
}

[data-theme="light"] .close-btn:hover {
    color: #1f2937;
    background: rgba(0, 0, 0, 0.05);
}

[data-theme="light"] .generated-link-section {
    background: rgba(99, 102, 241, 0.06);
    border-color: rgba(99, 102, 241, 0.2);
}

[data-theme="light"] .link-input {
    background: #f9fafb;
    border-color: #d1d5db;
    color: #1f2937;
}

[data-theme="light"] .link-note {
    color: #6b7280;
}

[data-theme="light"] .create-section {
    background: #f9fafb;
}

[data-theme="light"] .create-section h4 {
    color: #1f2937;
}

[data-theme="light"] .form-group label {
    color: #6b7280;
}

[data-theme="light"] .expiry-btn {
    background: #ffffff;
    border-color: #d1d5db;
    color: #6b7280;
}

[data-theme="light"] .expiry-btn:hover {
    border-color: #6366f1;
    color: #1f2937;
}

[data-theme="light"] .expiry-unit {
    color: #6b7280;
}

[data-theme="light"] .checkbox-label span {
    color: #1f2937;
}

[data-theme="light"] .form-control {
    background: #ffffff;
    border-color: #d1d5db;
    color: #1f2937;
}

[data-theme="light"] .existing-links-section h4 {
    color: #1f2937;
}

[data-theme="light"] .share-link-item {
    background: #f9fafb;
    border-color: #e5e7eb;
}

[data-theme="light"] .link-token code {
    color: #1f2937;
}

[data-theme="light"] .link-meta {
    color: #6b7280;
}

[data-theme="light"] .revoke-btn {
    border-color: rgba(239, 68, 68, 0.2);
}

[data-theme="light"] .revoke-btn:hover {
    background: rgba(239, 68, 68, 0.08);
}

[data-theme="light"] .no-links {
    color: #9ca3af;
}

[data-theme="light"] .loading-state {
    color: #9ca3af;
}

/* ==================== MOBILE RESPONSIVE ==================== */
@media (max-width: 640px) {
    .modal-backdrop {
        padding: 0.5rem;
        align-items: flex-end;
    }

    .modal-container {
        max-width: 100%;
        max-height: 85vh;
        border-radius: 12px 12px 0 0;
    }

    .modal-body {
        padding: 1rem;
    }

    .link-input-group {
        flex-direction: column;
    }

    .copy-btn {
        justify-content: center;
    }

    .expiry-options {
        gap: 0.375rem;
    }

    .expiry-btn {
        padding: 0.4rem 0.625rem;
        font-size: 0.75rem;
    }

    .link-meta {
        flex-direction: column;
        gap: 0.35rem;
    }
}
</style>
