/**
 * useSharedWebSocket - Vue composable for share-token WebSocket connections
 * 
 * Provides reactive connection state and probe data subscriptions for shared views.
 * Handles connection lifecycle tied to component mount/unmount.
 */

import { ref, onMounted, onUnmounted, computed, type Ref } from 'vue';
import { SharedWebSocketService } from '@/services/sharedWebSocketService';

export interface ProbeDataPayload {
    workspace_id: number;
    probe_id: number;
    agent_id: number;
    probe_agent_id?: number;
    target_agent?: number;
    type: string;
    payload: any;
    created_at: string;
    target?: string;
    triggered?: boolean;
}

export interface UseSharedWebSocketOptions {
    token: Ref<string> | string;
    password?: Ref<string | null> | string | null;
    probeId?: Ref<number> | number;  // 0 for all probes
    onData?: (data: ProbeDataPayload) => void;
    autoConnect?: boolean;
}

export function useSharedWebSocket(options: UseSharedWebSocketOptions) {
    const connected = ref(false);
    const error = ref<string | null>(null);
    const lastData = ref<ProbeDataPayload | null>(null);

    let unsubscribe: (() => void) | null = null;

    const tokenValue = computed(() =>
        typeof options.token === 'string' ? options.token : options.token.value
    );

    const passwordValue = computed(() => {
        if (options.password === undefined || options.password === null) return null;
        return typeof options.password === 'string' ? options.password : options.password.value;
    });

    const probeIdValue = computed(() => {
        if (options.probeId === undefined) return 0;
        return typeof options.probeId === 'number' ? options.probeId : options.probeId.value;
    });

    async function connect() {
        if (!tokenValue.value) {
            error.value = 'No token provided';
            return;
        }

        try {
            error.value = null;
            await SharedWebSocketService.connect(tokenValue.value, passwordValue.value);
            connected.value = true;

            // Subscribe to probe data
            unsubscribe = SharedWebSocketService.subscribe(probeIdValue.value, (data) => {
                lastData.value = data;
                if (options.onData) {
                    options.onData(data);
                }
            });
        } catch (err: any) {
            error.value = err.message || 'Failed to connect';
            connected.value = false;
        }
    }

    function disconnect() {
        if (unsubscribe) {
            unsubscribe();
            unsubscribe = null;
        }
        SharedWebSocketService.disconnect();
        connected.value = false;
    }

    onMounted(() => {
        if (options.autoConnect !== false) {
            connect();
        }
    });

    onUnmounted(() => {
        disconnect();
    });

    return {
        connected,
        error,
        lastData,
        connect,
        disconnect,
    };
}
