<script lang="ts" setup>
import { ref, computed } from 'vue';

const props = defineProps<{
    agentId: number | string;
    agentName: string;
}>();

const emit = defineEmits<{
    (e: 'close'): void;
    (e: 'generate', timeRange: number | { from: Date; to: Date }): void;
}>();

const rangeMode = ref<'preset' | 'custom'>('preset');
const selectedPreset = ref(7);
const customFrom = ref('');
const customTo = ref('');

const presetOptions = [
    { label: '24 hours', value: 1, icon: 'bi-clock' },
    { label: '48 hours', value: 2, icon: 'bi-clock' },
    { label: '72 hours', value: 3, icon: 'bi-clock' },
    { label: '7 days', value: 7, icon: 'bi-calendar-week' },
];

const customRangeValid = computed(() => {
    if (!customFrom.value || !customTo.value) return false;
    const from = new Date(customFrom.value);
    const to = new Date(customTo.value);
    return from < to && from > new Date(0);
});

const selectedRange = computed<number | { from: Date; to: Date }>(() => {
    if (rangeMode.value === 'custom' && customRangeValid.value) {
        return {
            from: new Date(customFrom.value),
            to: new Date(customTo.value)
        };
    }
    return selectedPreset.value;
});

function handleGenerate() {
    emit('generate', selectedRange.value);
}

function formatDateForInput(date: Date): string {
    return date.toISOString().split('T')[0];
}

function setQuickRange(days: number) {
    const to = new Date();
    const from = new Date(Date.now() - days * 24 * 60 * 60 * 1000);
    customFrom.value = formatDateForInput(from);
    customTo.value = formatDateForInput(to);
}

const showCustomDateHint = computed(() => {
    if (rangeMode.value === 'custom' && customRangeValid.value) {
        const from = new Date(customFrom.value);
        const to = new Date(customTo.value);
        const days = Math.round((to.getTime() - from.getTime()) / (24 * 60 * 60 * 1000));
        return `≈ ${days} day${days !== 1 ? 's' : ''} range`;
    }
    return null;
});
</script>

<template>
    <div class="modal-backdrop" @click.self="emit('close')">
        <div class="modal-container">
            <div class="modal-header">
                <h3><i class="bi bi-file-earmark-pdf"></i> Voice Quality Report</h3>
                <button class="close-btn" @click="emit('close')">
                    <i class="bi bi-x-lg"></i>
                </button>
            </div>
            
            <div class="modal-body">
                <div class="agent-info">
                    <span class="info-label">Agent:</span>
                    <span class="info-value">{{ agentName }}</span>
                </div>

                <div class="range-section">
                    <label class="section-label">Time Range</label>
                    
                    <div class="range-mode-toggle">
                        <button 
                            class="toggle-btn"
                            :class="{ active: rangeMode === 'preset' }"
                            @click="rangeMode = 'preset'"
                        >
                            Quick Select
                        </button>
                        <button 
                            class="toggle-btn"
                            :class="{ active: rangeMode === 'custom' }"
                            @click="rangeMode = 'custom'"
                        >
                            Custom Range
                        </button>
                    </div>

                    <div v-if="rangeMode === 'preset'" class="preset-options">
                        <button
                            v-for="opt in presetOptions"
                            :key="opt.value"
                            class="preset-btn"
                            :class="{ active: selectedPreset === opt.value }"
                            @click="selectedPreset = opt.value"
                        >
                            <i :class="opt.icon"></i>
                            <span>{{ opt.label }}</span>
                        </button>
                    </div>

                    <div v-else class="custom-range">
                        <div class="date-inputs">
                            <div class="date-field">
                                <label>From</label>
                                <input 
                                    type="date" 
                                    v-model="customFrom"
                                    class="form-control"
                                    :max="customTo || undefined"
                                />
                            </div>
                            <div class="date-separator">
                                <i class="bi bi-arrow-right"></i>
                            </div>
                            <div class="date-field">
                                <label>To</label>
                                <input 
                                    type="date" 
                                    v-model="customTo"
                                    class="form-control"
                                    :min="customFrom || undefined"
                                />
                            </div>
                        </div>
                        
                        <div class="quick-ranges">
                            <span class="quick-label">Quick set:</span>
                            <button @click="setQuickRange(14)" class="quick-btn">14d</button>
                            <button @click="setQuickRange(30)" class="quick-btn">30d</button>
                            <button @click="setQuickRange(90)" class="quick-btn">90d</button>
                        </div>

                        <div v-if="showCustomDateHint" class="range-hint">
                            <i class="bi bi-info-circle"></i>
                            {{ showCustomDateHint }}
                        </div>

                        <div v-if="rangeMode === 'custom' && !customRangeValid.value && (customFrom.value || customTo.value)" class="validation-error">
                            <i class="bi bi-exclamation-triangle"></i>
                            Please select valid start and end dates
                        </div>
                    </div>
                </div>

                <div class="report-contents">
                    <label class="section-label">Report Will Include</label>
                    <ul class="content-list">
                        <li><i class="bi bi-check-circle"></i> Overall MOS score and grade</li>
                        <li><i class="bi bi-check-circle"></i> Latency, jitter, and packet loss scores</li>
                        <li><i class="bi bi-check-circle"></i> Forward and return path metrics</li>
                        <li><i class="bi bi-check-circle"></i> Per-probe health details</li>
                        <li><i class="bi bi-check-circle"></i> Detected voice quality issues</li>
                        <li><i class="bi bi-check-circle"></i> Recommendations</li>
                    </ul>
                </div>
            </div>

            <div class="modal-footer">
                <button class="btn btn-outline-secondary" @click="emit('close')">
                    Cancel
                </button>
                <button 
                    class="btn btn-primary generate-btn"
                    @click="handleGenerate"
                    :disabled="rangeMode === 'custom' && !customRangeValid"
                >
                    <i class="bi bi-download"></i>
                    Generate Report
                </button>
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
    max-width: 480px;
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

.modal-footer {
    display: flex;
    justify-content: flex-end;
    gap: 0.75rem;
    padding: 1rem 1.25rem;
    border-top: 1px solid var(--bs-border-color);
    background: var(--bs-tertiary-bg);
}

.agent-info {
    background: var(--bs-secondary-bg);
    border-radius: 10px;
    padding: 0.875rem 1rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.9rem;
}

.info-label {
    color: var(--bs-secondary-color);
    font-weight: 500;
}

.info-value {
    color: var(--bs-body-color);
    font-weight: 600;
}

.section-label {
    display: block;
    font-size: 0.8rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    color: var(--bs-secondary-color);
    margin-bottom: 0.75rem;
}

.range-mode-toggle {
    display: flex;
    background: var(--bs-secondary-bg);
    border-radius: 10px;
    padding: 4px;
    margin-bottom: 1rem;
}

.toggle-btn {
    flex: 1;
    padding: 0.625rem 0.75rem;
    border: none;
    background: transparent;
    color: var(--bs-secondary-color);
    font-size: 0.85rem;
    font-weight: 500;
    border-radius: 8px;
    cursor: pointer;
    transition: all 0.2s;
}

.toggle-btn.active {
    background: var(--bs-body-bg);
    color: var(--bs-body-color);
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

.preset-options {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 0.5rem;
}

.preset-btn {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.375rem;
    padding: 1rem;
    border: 1px solid var(--bs-border-color);
    background: var(--bs-body-bg);
    color: var(--bs-secondary-color);
    border-radius: 10px;
    cursor: pointer;
    transition: all 0.2s;
    font-size: 0.85rem;
}

.preset-btn i {
    font-size: 1.25rem;
}

.preset-btn:hover {
    border-color: var(--bs-primary);
    color: var(--bs-primary);
    background: rgba(var(--bs-primary-rgb), 0.05);
}

.preset-btn.active {
    border-color: var(--bs-primary);
    background: var(--bs-primary);
    color: var(--bs-white);
    box-shadow: 0 0 0 3px rgba(var(--bs-primary-rgb), 0.2);
}

.custom-range {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
}

.date-inputs {
    display: flex;
    align-items: flex-end;
    gap: 0.75rem;
}

.date-field {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
}

.date-field label {
    font-size: 0.8rem;
    font-weight: 500;
    color: var(--bs-secondary-color);
}

.date-separator {
    padding-bottom: 0.625rem;
    color: var(--bs-secondary-color);
}

.form-control {
    background: var(--bs-body-bg);
    border: 1px solid var(--bs-border-color);
    border-radius: 8px;
    padding: 0.625rem 0.75rem;
    color: var(--bs-body-color);
    width: 100%;
    font-size: 0.9rem;
}

.form-control:focus {
    outline: none;
    border-color: var(--bs-primary);
    box-shadow: 0 0 0 2px rgba(var(--bs-primary-rgb), 0.15);
}

.quick-ranges {
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

.quick-label {
    font-size: 0.8rem;
    color: var(--bs-secondary-color);
}

.quick-btn {
    background: var(--bs-secondary-bg);
    border: 1px solid var(--bs-border-color);
    color: var(--bs-secondary-color);
    padding: 0.25rem 0.625rem;
    border-radius: 6px;
    font-size: 0.75rem;
    cursor: pointer;
    transition: all 0.15s;
}

.quick-btn:hover {
    border-color: var(--bs-primary);
    color: var(--bs-primary);
}

.range-hint {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    font-size: 0.8rem;
    color: var(--bs-primary);
    background: rgba(var(--bs-primary-rgb), 0.1);
    padding: 0.5rem 0.75rem;
    border-radius: 6px;
}

.validation-error {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    font-size: 0.8rem;
    color: var(--bs-danger);
    background: rgba(var(--bs-danger-rgb), 0.1);
    padding: 0.5rem 0.75rem;
    border-radius: 6px;
}

.report-contents {
    background: var(--bs-secondary-bg);
    border-radius: 10px;
    padding: 1rem;
}

.content-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 0.5rem;
}

.content-list li {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.85rem;
    color: var(--bs-body-color);
}

.content-list li i {
    color: var(--bs-success);
    font-size: 0.9rem;
}

.btn {
    padding: 0.625rem 1.25rem;
    border-radius: 10px;
    font-weight: 500;
    font-size: 0.9rem;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    transition: all 0.2s;
}

.btn-outline-secondary {
    background: transparent;
    border: 1px solid var(--bs-border-color);
    color: var(--bs-secondary-color);
}

.btn-outline-secondary:hover {
    background: var(--bs-secondary-bg);
    border-color: var(--bs-secondary-color);
}

.btn-primary {
    background: var(--bs-primary);
    border: none;
    color: var(--bs-white);
}

.btn-primary:hover:not(:disabled) {
    transform: translateY(-1px);
    box-shadow: 0 4px 12px rgba(var(--bs-primary-rgb), 0.4);
}

.btn-primary:disabled {
    opacity: 0.6;
    cursor: not-allowed;
}

@keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
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

[data-theme="light"] .toggle-btn.active {
    background: #ffffff;
    color: #1f2937;
}

[data-theme="light"] .preset-btn {
    background: #ffffff;
}

@media (max-width: 480px) {
    .modal-backdrop {
        padding: 0.5rem;
        align-items: flex-end;
    }

    .modal-container {
        max-width: 100%;
        max-height: 85vh;
        border-radius: 12px 12px 0 0;
    }

    .date-inputs {
        flex-direction: column;
        align-items: stretch;
    }

    .date-separator {
        padding: 0.5rem 0;
        text-align: center;
    }

    .date-separator i {
        transform: rotate(90deg);
    }

    .content-list {
        grid-template-columns: 1fr;
    }
}
</style>