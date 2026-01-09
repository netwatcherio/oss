/**
 * useWebSocket Composable
 * 
 * Vue composable for subscribing to real-time probe data updates.
 * Automatically manages WebSocket connection and subscriptions.
 */

import { ref, onMounted, onUnmounted, watch, type Ref } from 'vue';
import { websocketService, type ProbeDataEvent } from '@/services/websocketService';

// Re-export ProbeDataEvent for convenience
export type { ProbeDataEvent } from '@/services/websocketService';

export interface UseWebSocketOptions {
    /** Auto-connect on mount (default: true) */
    autoConnect?: boolean;
    /** Workspace ID to subscribe to */
    workspaceId?: number | Ref<number | undefined>;
    /** Probe ID to subscribe to (0 or undefined = all probes in workspace) */
    probeId?: number | Ref<number | undefined>;
}

export function useWebSocket(options: UseWebSocketOptions = {}) {
    const { autoConnect = true } = options;

    const connected = ref(false);
    const lastError = ref<string | null>(null);
    const lastProbeData = ref<ProbeDataEvent | null>(null);

    // Track cleanup functions
    const cleanupFns: Array<() => void> = [];

    // Get reactive values
    const getWorkspaceId = (): number | undefined => {
        if (options.workspaceId === undefined) return undefined;
        return typeof options.workspaceId === 'number'
            ? options.workspaceId
            : options.workspaceId.value;
    };

    const getProbeId = (): number => {
        if (options.probeId === undefined) return 0;
        return typeof options.probeId === 'number'
            ? options.probeId
            : (options.probeId.value ?? 0);
    };

    // Connection handlers
    const handleConnect = () => {
        connected.value = true;
        lastError.value = null;
    };

    const handleDisconnect = () => {
        connected.value = false;
    };

    const handleError = (error: Event | Error) => {
        lastError.value = error instanceof Error ? error.message : 'WebSocket error';
    };

    // Current subscription cleanup
    let unsubscribe: (() => void) | null = null;

    /**
     * Subscribe to probe data updates
     */
    const subscribe = (workspaceId: number, probeId: number = 0, handler: (data: ProbeDataEvent) => void) => {
        // Clean up previous subscription
        if (unsubscribe) {
            unsubscribe();
        }

        unsubscribe = websocketService.subscribe(workspaceId, probeId, (data) => {
            lastProbeData.value = data;
            handler(data);
        });

        return unsubscribe;
    };

    /**
     * Connect to WebSocket server
     */
    const connect = () => {
        websocketService.connect();
    };

    /**
     * Disconnect from WebSocket server
     */
    const disconnect = () => {
        websocketService.disconnect();
    };

    onMounted(() => {
        // Register connection handlers
        cleanupFns.push(websocketService.onConnect(handleConnect));
        cleanupFns.push(websocketService.onDisconnect(handleDisconnect));
        cleanupFns.push(websocketService.onError(handleError));

        // Set initial connection state
        connected.value = websocketService.isConnected();

        // Auto-connect if enabled
        if (autoConnect) {
            connect();
        }

        // Auto-subscribe if workspaceId is provided
        const wsId = getWorkspaceId();
        if (wsId !== undefined) {
            subscribe(wsId, getProbeId(), () => { });
        }
    });

    // Watch for changes in workspace/probe IDs
    if (typeof options.workspaceId === 'object') {
        watch(options.workspaceId, (newWsId) => {
            if (newWsId !== undefined) {
                subscribe(newWsId, getProbeId(), () => { });
            }
        });
    }

    if (typeof options.probeId === 'object') {
        watch(options.probeId, () => {
            const wsId = getWorkspaceId();
            if (wsId !== undefined) {
                subscribe(wsId, getProbeId(), () => { });
            }
        });
    }

    onUnmounted(() => {
        // Clean up subscriptions
        if (unsubscribe) {
            unsubscribe();
        }

        // Clean up connection handlers
        cleanupFns.forEach(fn => fn());
        cleanupFns.length = 0;
    });

    return {
        /** Whether WebSocket is connected */
        connected,
        /** Last error message */
        lastError,
        /** Last received probe data */
        lastProbeData,
        /** Connect to WebSocket server */
        connect,
        /** Disconnect from WebSocket server */
        disconnect,
        /** Subscribe to probe data updates */
        subscribe,
        /** Check if connected */
        isConnected: () => websocketService.isConnected(),
    };
}

/**
 * useProbeSubscription Composable
 * 
 * Simplified composable for subscribing to a specific probe's updates.
 */
export function useProbeSubscription(
    workspaceId: Ref<number | undefined>,
    probeId: Ref<number | undefined>,
    onData: (data: ProbeDataEvent) => void
) {
    const connected = ref(false);
    let unsubscribe: (() => void) | null = null;

    const handleConnect = () => {
        connected.value = true;
        // Resubscribe on connect
        updateSubscription();
    };

    const handleDisconnect = () => {
        connected.value = false;
    };

    const updateSubscription = () => {
        // Clean up previous subscription
        if (unsubscribe) {
            unsubscribe();
            unsubscribe = null;
        }

        const wsId = workspaceId.value;
        const pId = probeId.value ?? 0;

        if (wsId !== undefined) {
            unsubscribe = websocketService.subscribe(wsId, pId, onData);
        }
    };

    onMounted(() => {
        websocketService.onConnect(handleConnect);
        websocketService.onDisconnect(handleDisconnect);

        connected.value = websocketService.isConnected();
        websocketService.connect();
        updateSubscription();
    });

    // Watch for changes
    watch([workspaceId, probeId], () => {
        updateSubscription();
    });

    onUnmounted(() => {
        if (unsubscribe) {
            unsubscribe();
        }
    });

    return { connected };
}
