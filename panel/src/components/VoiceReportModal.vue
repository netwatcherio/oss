<script lang="ts" setup>
import { ref, computed } from 'vue';

const props = defineProps<{
    agentId: number | string;
    agentName: string;
}>();

const emit = defineEmits<{
    (e: 'close'): void;
    (e: 'generate', timeRange: number | { from: Date; to: Date }, sections: string): void;
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

// Section toggles — backed by the same set the backend
// `ParseAgentReportSections` understands. The keys map 1:1 to the
// CSV tokens (`summary`, `timeline`, `aggregate`, `probes`,
// `issues`, `correlation`, `appendix`, `raw`). The presets in
// `sectionPresets` build the CSV for the user.
interface SectionToggle {
    key: string;
    label: string;
    description: string;
    icon: string;
}

const sectionToggles: SectionToggle[] = [
    { key: 'summary',     label: 'Executive snapshot', description: 'Grade chip, sub-scores, baseline comparison, recommendation', icon: 'bi-card-text' },
    { key: 'timeline',    label: 'MOS timeline chart', description: 'Forward + return MOS over time with issue markers',          icon: 'bi-graph-up' },
    { key: 'aggregate',   label: 'Aggregate paths',     description: 'Sample-weighted forward + return summary cards',            icon: 'bi-bar-chart' },
    { key: 'probes',      label: 'Per-probe table',     description: 'One row per probe with MOS, latency, jitter, loss',         icon: 'bi-table' },
    { key: 'issues',      label: 'Detected issues',     description: 'Grouped by category with evidence and recommendations',     icon: 'bi-exclamation-triangle' },
    { key: 'correlation', label: 'Workspace correlation', description: 'Workspace-level incidents and route/MTR signals',         icon: 'bi-diagram-3' },
    { key: 'appendix',    label: 'Methodology',         description: 'How MOS / sub-scores are computed and the active thresholds', icon: 'bi-book' },
    { key: 'raw',         label: 'Raw API reference',   description: 'Link to the JSON endpoint for offline scripting',          icon: 'bi-code' },
];

const sectionEnabled = ref<Record<string, boolean>>({
    summary:     true,
    timeline:    true,
    aggregate:   false,
    probes:      true,
    issues:      true,
    correlation: true,
    appendix:    false,
    raw:         false,
});

type SectionPreset = 'quick' | 'full' | 'minimal' | 'custom';
const sectionPreset = ref<SectionPreset>('quick');

function applySectionPreset(preset: SectionPreset) {
    sectionPreset.value = preset;
    switch (preset) {
        case 'quick':
            sectionEnabled.value = {
                summary: true, timeline: true, aggregate: false, probes: true,
                issues: true, correlation: true, appendix: false, raw: false,
            };
            break;
        case 'full':
            sectionEnabled.value = {
                summary: true, timeline: true, aggregate: true, probes: true,
                issues: true, correlation: true, appendix: true, raw: true,
            };
            break;
        case 'minimal':
            sectionEnabled.value = {
                summary: true, timeline: false, aggregate: false, probes: true,
                issues: true, correlation: false, appendix: false, raw: false,
            };
            break;
        case 'custom':
            // Leave current state; user toggles individually.
            break;
    }
}

function onSectionToggle(key: string) {
    sectionEnabled.value[key] = !sectionEnabled.value[key];
    sectionPreset.value = 'custom';
}

const sectionsCSV = computed(() => {
    const enabled = Object.entries(sectionEnabled.value)
        .filter(([, on]) => on)
        .map(([k]) => k)
        .join(',');
    return enabled || 'summary';
});

function handleGenerate() {
    emit('generate', selectedRange.value, sectionsCSV.value);
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

const enabledSectionCount = computed(() =>
    Object.values(sectionEnabled.value).filter(Boolean).length
);
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

                <div class="sections-section">
                    <div class="sections-header">
                        <label class="section-label">Report Sections</label>
                        <div class="section-presets">
                            <button
                                v-for="p in (['quick', 'full', 'minimal', 'custom'] as const)"
                                :key="p"
                                class="preset-pill"
                                :class="{ active: sectionPreset === p }"
                                @click="applySectionPreset(p)"
                            >{{ p }}</button>
                        </div>
                    </div>

                    <div class="sections-grid">
                        <label
                            v-for="s in sectionToggles"
                            :key="s.key"
                            class="section-toggle"
                            :class="{ enabled: sectionEnabled[s.key] }"
                        >
                            <input
                                type="checkbox"
                                :checked="sectionEnabled[s.key]"
                                @change="onSectionToggle(s.key)"
                            />
                            <i :class="s.icon" class="section-icon"></i>
                            <div class="section-text">
                                <div class="section-label-text">{{ s.label }}</div>
                                <div class="section-desc">{{ s.description }}</div>
                            </div>
                        </label>
                    </div>
                    <div class="sections-summary">
                        <i class="bi bi-info-circle"></i>
                        {{ enabledSectionCount }} of {{ sectionToggles.length }} sections selected
                    </div>
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

.sections-section {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
}

.sections-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0;
}

.section-presets {
    display: flex;
    gap: 0.25rem;
}

.preset-pill {
    background: var(--bs-secondary-bg);
    border: 1px solid var(--bs-border-color);
    color: var(--bs-secondary-color);
    padding: 0.2rem 0.6rem;
    border-radius: 999px;
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.4px;
    cursor: pointer;
    transition: all 0.15s;
}

.preset-pill.active {
    background: var(--bs-primary);
    color: var(--bs-white);
    border-color: var(--bs-primary);
}

.sections-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 0.5rem;
}

.section-toggle {
    display: flex;
    align-items: flex-start;
    gap: 0.625rem;
    padding: 0.625rem 0.75rem;
    border: 1px solid var(--bs-border-color);
    border-radius: 10px;
    cursor: pointer;
    background: var(--bs-body-bg);
    transition: all 0.15s;
}

.section-toggle input[type="checkbox"] {
    margin-top: 0.15rem;
    cursor: pointer;
}

.section-toggle.enabled {
    border-color: var(--bs-primary);
    background: rgba(var(--bs-primary-rgb), 0.04);
}

.section-icon {
    font-size: 1.1rem;
    color: var(--bs-secondary-color);
    margin-top: 0.05rem;
}

.section-toggle.enabled .section-icon {
    color: var(--bs-primary);
}

.section-text {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    min-width: 0;
}

.section-label-text {
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--bs-body-color);
}

.section-desc {
    font-size: 0.72rem;
    color: var(--bs-secondary-color);
    line-height: 1.3;
}

.sections-summary {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    font-size: 0.75rem;
    color: var(--bs-secondary-color);
    padding: 0.4rem 0.6rem;
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

    .sections-grid {
        grid-template-columns: 1fr;
    }
}
</style>
