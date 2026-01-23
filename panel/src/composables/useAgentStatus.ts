/**
 * useAgentStatus Composable
 * 
 * Shared composable for consistent agent online/offline status logic
 * across the agent grid (Workspace view) and agent dashboard.
 */

import { ref, computed, onMounted, onUnmounted, type Ref, type ComputedRef } from 'vue';
import type { Agent } from '@/types';

export type AgentStatusTier = 'online' | 'stale' | 'offline';

export interface AgentStatusConfig {
    /** Threshold in minutes for "online" status (default: 2) */
    onlineThresholdMinutes?: number;
    /** Threshold in minutes for "stale" status (default: 5) */
    staleThresholdMinutes?: number;
    /** Interval in ms for auto-updating 'now' ref (default: 15000) */
    updateIntervalMs?: number;
}

export interface AgentStatusResult {
    /** Reactive current timestamp that auto-updates */
    now: Ref<Date>;
    /** Get the status tier for an agent */
    getAgentStatus: (agent: Agent) => AgentStatusTier;
    /** Get human-readable "last seen" text */
    getLastSeenText: (agent: Agent) => string;
    /** Get CSS class for status color */
    getStatusColor: (status: AgentStatusTier) => string;
    /** Get Bootstrap icon class for status */
    getStatusIcon: (status: AgentStatusTier) => string;
    /** Get status display label */
    getStatusLabel: (status: AgentStatusTier) => string;
    /** Check if agent is online (convenience method) */
    isOnline: (agent: Agent) => boolean;
    /** Check if agent is receiving live data (for pulse animation) */
    isLive: ComputedRef<boolean>;
    /** Set live status (call when receiving WebSocket data) */
    setLive: (value: boolean) => void;
}

const DEFAULT_CONFIG: Required<AgentStatusConfig> = {
    onlineThresholdMinutes: 2,
    staleThresholdMinutes: 5,
    updateIntervalMs: 15000,
};

/**
 * Composable for managing agent online/offline status with live updates.
 * 
 * @example
 * ```vue
 * <script setup>
 * import { useAgentStatus } from '@/composables/useAgentStatus';
 * 
 * const { now, getAgentStatus, getLastSeenText, isLive } = useAgentStatus();
 * </script>
 * 
 * <template>
 *   <div v-for="agent in agents" :key="agent.id">
 *     <span :class="getStatusColor(getAgentStatus(agent))">
 *       {{ getStatusLabel(getAgentStatus(agent)) }}
 *     </span>
 *     <span>{{ getLastSeenText(agent) }}</span>
 *   </div>
 * </template>
 * ```
 */
export function useAgentStatus(config: AgentStatusConfig = {}): AgentStatusResult {
    const mergedConfig = { ...DEFAULT_CONFIG, ...config };

    // Reactive timestamp that auto-updates
    const now = ref(new Date());
    let updateInterval: ReturnType<typeof setInterval> | null = null;

    // Live status tracking (for pulse animation)
    const liveStatus = ref(false);
    let liveTimeout: ReturnType<typeof setTimeout> | null = null;

    const isLive = computed(() => liveStatus.value);

    /**
     * Set live status with auto-reset after 2 seconds
     */
    function setLive(value: boolean) {
        liveStatus.value = value;
        if (value) {
            // Auto-reset after 2 seconds
            if (liveTimeout) clearTimeout(liveTimeout);
            liveTimeout = setTimeout(() => {
                liveStatus.value = false;
            }, 2000);
        }
    }

    /**
     * Calculate time difference in minutes between now and agent's last update
     */
    function getTimeDiffMinutes(agent: Agent): number {
        if (!agent.updated_at) return Infinity;
        const lastSeen = new Date(agent.updated_at);
        const diffMs = now.value.getTime() - lastSeen.getTime();
        return diffMs / 60000;
    }

    /**
     * Get the status tier for an agent based on last seen time
     */
    function getAgentStatus(agent: Agent): AgentStatusTier {
        const diffMinutes = getTimeDiffMinutes(agent);

        if (diffMinutes <= mergedConfig.onlineThresholdMinutes) {
            return 'online';
        } else if (diffMinutes <= mergedConfig.staleThresholdMinutes) {
            return 'stale';
        } else {
            return 'offline';
        }
    }

    /**
     * Get human-readable "last seen" text
     */
    function getLastSeenText(agent: Agent): string {
        if (!agent.updated_at) return 'Never';

        const lastSeen = new Date(agent.updated_at);
        const diffMs = now.value.getTime() - lastSeen.getTime();
        const diffMins = Math.floor(diffMs / 60000);
        const diffHours = Math.floor(diffMs / 3600000);
        const diffDays = Math.floor(diffMs / 86400000);

        if (diffMins < 1) return 'Just now';
        if (diffMins < 60) return `${diffMins}m ago`;
        if (diffHours < 24) return `${diffHours}h ago`;
        return `${diffDays}d ago`;
    }

    /**
     * Get CSS class for status color
     */
    function getStatusColor(status: AgentStatusTier): string {
        switch (status) {
            case 'online':
                return 'text-success';
            case 'stale':
                return 'text-warning';
            case 'offline':
                return 'text-danger';
        }
    }

    /**
     * Get Bootstrap icon class for status
     */
    function getStatusIcon(status: AgentStatusTier): string {
        switch (status) {
            case 'online':
                return 'bi bi-check-circle-fill text-success';
            case 'stale':
                return 'bi bi-exclamation-circle-fill text-warning';
            case 'offline':
                return 'bi bi-x-circle-fill text-danger';
        }
    }

    /**
     * Get status display label
     */
    function getStatusLabel(status: AgentStatusTier): string {
        switch (status) {
            case 'online':
                return 'Online';
            case 'stale':
                return 'Stale';
            case 'offline':
                return 'Offline';
        }
    }

    /**
     * Convenience method to check if agent is online
     */
    function isOnline(agent: Agent): boolean {
        return getAgentStatus(agent) === 'online';
    }

    onMounted(() => {
        // Set up interval to auto-update the 'now' ref
        updateInterval = setInterval(() => {
            now.value = new Date();
        }, mergedConfig.updateIntervalMs);
    });

    onUnmounted(() => {
        if (updateInterval) {
            clearInterval(updateInterval);
            updateInterval = null;
        }
        if (liveTimeout) {
            clearTimeout(liveTimeout);
            liveTimeout = null;
        }
    });

    return {
        now,
        getAgentStatus,
        getLastSeenText,
        getStatusColor,
        getStatusIcon,
        getStatusLabel,
        isOnline,
        isLive,
        setLive,
    };
}

/**
 * Standalone utility functions for use outside of Vue components
 */
export function getAgentStatusStandalone(
    agent: Agent,
    now: Date = new Date(),
    config: AgentStatusConfig = {}
): AgentStatusTier {
    const mergedConfig = { ...DEFAULT_CONFIG, ...config };

    if (!agent.updated_at) return 'offline';

    const lastSeen = new Date(agent.updated_at);
    const diffMinutes = (now.getTime() - lastSeen.getTime()) / 60000;

    if (diffMinutes <= mergedConfig.onlineThresholdMinutes) {
        return 'online';
    } else if (diffMinutes <= mergedConfig.staleThresholdMinutes) {
        return 'stale';
    } else {
        return 'offline';
    }
}
