/**
 * SharedWebSocketService - WebSocket service for share-token authenticated connections
 * 
 * Unlike the regular websocketService which uses JWT auth, this service authenticates
 * using share tokens for public shared views. Authentication happens once at connection
 * time, then all subsequent data streams without re-validation.
 */

type ProbeDataEvent = {
    event: 'probe_data';
    data: {
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
    };
};

type SubscribeOkEvent = {
    event: 'subscribe_ok';
    data: { agent_id: number; probe_id: number };
};

type PongEvent = {
    event: 'pong';
};

type WsEvent = ProbeDataEvent | SubscribeOkEvent | PongEvent;

type ProbeDataHandler = (data: ProbeDataEvent['data']) => void;

interface SharedWebSocketState {
    ws: WebSocket | null;
    connected: boolean;
    token: string;
    password: string | null;
    agentId: number | null;
    handlers: Map<string, Set<ProbeDataHandler>>;  // probeId -> handlers (0 = all probes)
    reconnectAttempts: number;
    reconnectTimer: ReturnType<typeof setTimeout> | null;
    pingTimer: ReturnType<typeof setInterval> | null;
}

const state: SharedWebSocketState = {
    ws: null,
    connected: false,
    token: '',
    password: null,
    agentId: null,
    handlers: new Map(),
    reconnectAttempts: 0,
    reconnectTimer: null,
    pingTimer: null,
};

function getWsUrl(token: string, password?: string | null): string {
    const baseUrl = (window as any).CONTROLLER_ENDPOINT
        || import.meta.env.VITE_CONTROLLER_ENDPOINT
        || import.meta.env.CONTROLLER_ENDPOINT
        || 'http://localhost:8080';

    // Convert http(s) to ws(s)
    const wsBase = baseUrl.replace(/^http/, 'ws');
    const url = new URL(`${wsBase}/ws/share/raw`);
    url.searchParams.set('token', token);
    if (password) {
        url.searchParams.set('password', password);
    }
    return url.toString();
}

function connect(token: string, password?: string | null): Promise<void> {
    return new Promise((resolve, reject) => {
        if (state.ws && state.connected && state.token === token) {
            resolve();
            return;
        }

        // Clean up existing connection
        disconnect();

        state.token = token;
        state.password = password || null;

        const wsUrl = getWsUrl(token, password);
        console.log('[SharedWS] Connecting to', wsUrl.replace(/password=[^&]+/, 'password=***'));

        try {
            state.ws = new WebSocket(wsUrl);
        } catch (err) {
            console.error('[SharedWS] Failed to create WebSocket:', err);
            reject(err);
            return;
        }

        state.ws.onopen = () => {
            console.log('[SharedWS] Connected');
            state.connected = true;
            state.reconnectAttempts = 0;

            // Start ping interval
            state.pingTimer = setInterval(() => {
                if (state.ws?.readyState === WebSocket.OPEN) {
                    state.ws.send(JSON.stringify({ event: 'ping' }));
                }
            }, 30000);

            resolve();
        };

        state.ws.onmessage = (event) => {
            try {
                const msg: WsEvent = JSON.parse(event.data);

                if (msg.event === 'probe_data') {
                    const data = msg.data;
                    const probeKey = String(data.probe_id);

                    // Notify specific probe handlers
                    const probeHandlers = state.handlers.get(probeKey);
                    if (probeHandlers) {
                        for (const handler of probeHandlers) {
                            try {
                                handler(data);
                            } catch (err) {
                                console.error('[SharedWS] Handler error:', err);
                            }
                        }
                    }

                    // Notify "all probes" handlers (key "0")
                    const allHandlers = state.handlers.get('0');
                    if (allHandlers) {
                        for (const handler of allHandlers) {
                            try {
                                handler(data);
                            } catch (err) {
                                console.error('[SharedWS] Handler error:', err);
                            }
                        }
                    }
                } else if (msg.event === 'subscribe_ok') {
                    console.log('[SharedWS] Subscribed to agent', msg.data.agent_id, 'probe', msg.data.probe_id);
                    state.agentId = msg.data.agent_id;
                } else if (msg.event === 'pong') {
                    // Heartbeat response, connection is alive
                }
            } catch (err) {
                console.warn('[SharedWS] Failed to parse message:', err);
            }
        };

        state.ws.onclose = (event) => {
            console.log('[SharedWS] Disconnected:', event.code, event.reason);
            state.connected = false;

            if (state.pingTimer) {
                clearInterval(state.pingTimer);
                state.pingTimer = null;
            }

            // Attempt reconnect with exponential backoff (max 30 seconds)
            if (state.token && state.reconnectAttempts < 10) {
                const delay = Math.min(1000 * Math.pow(2, state.reconnectAttempts), 30000);
                console.log(`[SharedWS] Reconnecting in ${delay}ms (attempt ${state.reconnectAttempts + 1})`);
                state.reconnectAttempts++;
                state.reconnectTimer = setTimeout(() => {
                    connect(state.token, state.password).catch(console.error);
                }, delay);
            }
        };

        state.ws.onerror = (error) => {
            console.error('[SharedWS] Error:', error);
            reject(error);
        };
    });
}

function disconnect(): void {
    if (state.reconnectTimer) {
        clearTimeout(state.reconnectTimer);
        state.reconnectTimer = null;
    }
    if (state.pingTimer) {
        clearInterval(state.pingTimer);
        state.pingTimer = null;
    }
    if (state.ws) {
        state.ws.close();
        state.ws = null;
    }
    state.connected = false;
    state.token = '';
    state.password = null;
    state.agentId = null;
    state.handlers.clear();
}

function subscribe(probeId: number, handler: ProbeDataHandler): () => void {
    const key = String(probeId);

    if (!state.handlers.has(key)) {
        state.handlers.set(key, new Set());
    }
    state.handlers.get(key)!.add(handler);

    // Send subscribe message to server if connected
    if (state.ws?.readyState === WebSocket.OPEN) {
        state.ws.send(JSON.stringify({
            event: 'subscribe',
            data: { probe_id: probeId }
        }));
    }

    // Return unsubscribe function
    return () => {
        const handlers = state.handlers.get(key);
        if (handlers) {
            handlers.delete(handler);
            if (handlers.size === 0) {
                state.handlers.delete(key);
            }
        }
    };
}

function isConnected(): boolean {
    return state.connected && state.ws?.readyState === WebSocket.OPEN;
}

export const SharedWebSocketService = {
    connect,
    disconnect,
    subscribe,
    isConnected,
};
