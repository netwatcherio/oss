/**
 * WebSocket Service for NetWatcher Panel
 * 
 * Connects to the controller's raw WebSocket server (not neffos)
 * for real-time probe data updates.
 */

import { getSession } from "@/session";

// Event types for probe data
export interface ProbeDataEvent {
    workspace_id: number;
    probe_id: number;
    agent_id: number;
    type: string;
    payload: any;
    created_at: string;
    target?: string;
    triggered?: boolean;
}

// Event type for speedtest queue updates
export interface SpeedtestUpdateEvent {
    queue_id: number;
    workspace_id: number;
    agent_id: number;
    status: 'completed' | 'failed' | 'running';
    error?: string;
    server_id?: string;
    server_name?: string;
}

export type ProbeDataHandler = (data: ProbeDataEvent) => void;
export type SpeedtestUpdateHandler = (data: SpeedtestUpdateEvent) => void;
export type ConnectionHandler = () => void;
export type ErrorHandler = (error: Event | Error) => void;

class WebSocketService {
    private ws: WebSocket | null = null;
    private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    private reconnectAttempts = 0;
    private maxReconnectAttempts = 10;
    private baseReconnectDelay = 1000;

    private probeDataHandlers: Map<string, Set<ProbeDataHandler>> = new Map();
    private speedtestUpdateHandlers: Map<string, Set<SpeedtestUpdateHandler>> = new Map();
    private onConnectHandlers: Set<ConnectionHandler> = new Set();
    private onDisconnectHandlers: Set<ConnectionHandler> = new Set();
    private onErrorHandlers: Set<ErrorHandler> = new Set();

    private activeSubscriptions: Set<string> = new Set();
    private connected = false;

    /**
     * Get the WebSocket URL based on the controller endpoint
     */
    private getWebSocketUrl(token: string): string {
        const anyWindow = window as any;
        let baseUrl = anyWindow?.CONTROLLER_ENDPOINT
            || import.meta.env.CONTROLLER_ENDPOINT
            || '';

        // If no endpoint configured, use current host
        if (!baseUrl) {
            const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            baseUrl = `${proto}//${window.location.host}`;
        } else {
            // Convert http(s) to ws(s)
            baseUrl = baseUrl.replace(/^http/, 'ws');
        }

        // Ensure we have ws:// or wss://
        if (!baseUrl.startsWith('ws://') && !baseUrl.startsWith('wss://')) {
            const proto = window.location.protocol === 'https:' ? 'wss://' : 'ws://';
            baseUrl = proto + baseUrl;
        }

        // Use raw WebSocket endpoint (simpler than neffos)
        return `${baseUrl}/ws/panel/raw?token=${encodeURIComponent(token)}`;
    }

    /**
     * Connect to the raw WebSocket server
     */
    async connect(): Promise<void> {
        const session = getSession();
        if (!session?.token) {
            console.log('[WebSocket] No session token, skipping connection');
            return;
        }

        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            console.log('[WebSocket] Already connected');
            return;
        }

        const url = this.getWebSocketUrl(session.token);
        console.log('[WebSocket] Connecting to', url.replace(/token=[^&]+/, 'token=***'));

        try {
            this.ws = new WebSocket(url);

            this.ws.onopen = () => {
                console.log('[WebSocket] Connected');
                this.connected = true;
                this.reconnectAttempts = 0;
                this.onConnectHandlers.forEach(h => h());
                this.restoreSubscriptions();
            };

            this.ws.onmessage = (event) => {
                this.handleMessage(event.data);
            };

            this.ws.onerror = (error) => {
                console.error('[WebSocket] Error:', error);
                this.onErrorHandlers.forEach(h => h(error));
            };

            this.ws.onclose = (event) => {
                console.log('[WebSocket] Disconnected', event.code, event.reason);
                this.connected = false;
                this.onDisconnectHandlers.forEach(h => h());
                this.scheduleReconnect();
            };

        } catch (error) {
            console.error('[WebSocket] Connection error:', error);
            this.scheduleReconnect();
        }
    }

    /**
     * Handle incoming messages (simple JSON protocol)
     */
    private handleMessage(data: string): void {
        try {
            const msg = JSON.parse(data);

            switch (msg.event) {
                case 'probe_data':
                    const probeData: ProbeDataEvent = msg.data;
                    this.notifyProbeDataHandlers(probeData);
                    break;

                case 'speedtest_update':
                    const speedtestData: SpeedtestUpdateEvent = msg.data;
                    this.notifySpeedtestUpdateHandlers(speedtestData);
                    break;

                case 'subscribe_ok':
                    console.log('[WebSocket] Subscription confirmed:', msg.data);
                    break;

                case 'pong':
                    // Heartbeat response
                    break;

                default:
                    console.log('[WebSocket] Unknown event:', msg.event);
            }
        } catch (e) {
            console.error('[WebSocket] Failed to parse message:', e);
        }
    }

    /**
     * Notify handlers for probe data
     */
    private notifyProbeDataHandlers(data: ProbeDataEvent): void {
        // Notify specific probe handlers
        const probeKey = `${data.workspace_id}:${data.probe_id}`;
        this.probeDataHandlers.get(probeKey)?.forEach(h => h(data));

        // Notify workspace-wide handlers (probeId = 0)
        const wsKey = `${data.workspace_id}:0`;
        this.probeDataHandlers.get(wsKey)?.forEach(h => h(data));
    }

    /**
     * Notify handlers for speedtest updates
     */
    private notifySpeedtestUpdateHandlers(data: SpeedtestUpdateEvent): void {
        // Notify workspace-wide handlers
        const wsKey = `${data.workspace_id}:0`;
        this.speedtestUpdateHandlers.get(wsKey)?.forEach(h => h(data));

        // Notify agent-specific handlers
        const agentKey = `${data.workspace_id}:${data.agent_id}`;
        this.speedtestUpdateHandlers.get(agentKey)?.forEach(h => h(data));
    }

    /**
     * Send a JSON message
     */
    private send(event: string, data: any): void {
        if (this.ws?.readyState !== WebSocket.OPEN) {
            console.warn('[WebSocket] Cannot send, not connected');
            return;
        }
        this.ws.send(JSON.stringify({ event, data }));
    }

    /**
     * Schedule a reconnection attempt
     */
    private scheduleReconnect(): void {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.log('[WebSocket] Max reconnect attempts reached');
            return;
        }

        const delay = this.baseReconnectDelay * Math.pow(2, this.reconnectAttempts);
        console.log(`[WebSocket] Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts + 1})`);

        this.reconnectTimer = setTimeout(() => {
            this.reconnectAttempts++;
            this.connect();
        }, delay);
    }

    /**
     * Restore subscriptions after reconnect
     */
    private restoreSubscriptions(): void {
        this.activeSubscriptions.forEach(key => {
            const parts = key.split(':');
            const wsId = parseInt(parts[0] || '0', 10);
            const probeId = parseInt(parts[1] || '0', 10);
            if (!isNaN(wsId) && !isNaN(probeId) && wsId > 0) {
                this.send('subscribe', { workspace_id: wsId, probe_id: probeId });
            }
        });
    }

    /**
     * Subscribe to probe data updates
     */
    subscribe(workspaceId: number, probeId: number, handler: ProbeDataHandler): () => void {
        const key = `${workspaceId}:${probeId}`;

        if (!this.probeDataHandlers.has(key)) {
            this.probeDataHandlers.set(key, new Set());
        }
        this.probeDataHandlers.get(key)!.add(handler);

        // Track subscription for reconnect restoration
        this.activeSubscriptions.add(key);

        // Send subscription if connected
        if (this.connected) {
            this.send('subscribe', { workspace_id: workspaceId, probe_id: probeId });
        }

        // Return unsubscribe function
        return () => {
            this.probeDataHandlers.get(key)?.delete(handler);
            if (this.probeDataHandlers.get(key)?.size === 0) {
                this.probeDataHandlers.delete(key);
                this.activeSubscriptions.delete(key);
            }
        };
    }

    /**
     * Subscribe to speedtest queue updates for a workspace/agent
     * @param workspaceId - Workspace ID
     * @param agentId - Agent ID (0 for all agents in workspace)
     * @param handler - Callback function
     * @returns Unsubscribe function
     */
    onSpeedtestUpdate(workspaceId: number, agentId: number, handler: SpeedtestUpdateHandler): () => void {
        const key = `${workspaceId}:${agentId}`;

        if (!this.speedtestUpdateHandlers.has(key)) {
            this.speedtestUpdateHandlers.set(key, new Set());
        }
        this.speedtestUpdateHandlers.get(key)!.add(handler);

        // Return unsubscribe function
        return () => {
            this.speedtestUpdateHandlers.get(key)?.delete(handler);
            if (this.speedtestUpdateHandlers.get(key)?.size === 0) {
                this.speedtestUpdateHandlers.delete(key);
            }
        };
    }

    /**
     * Add connection event handler
     */
    onConnect(handler: ConnectionHandler): () => void {
        this.onConnectHandlers.add(handler);
        return () => this.onConnectHandlers.delete(handler);
    }

    /**
     * Add disconnection event handler
     */
    onDisconnect(handler: ConnectionHandler): () => void {
        this.onDisconnectHandlers.add(handler);
        return () => this.onDisconnectHandlers.delete(handler);
    }

    /**
     * Add error event handler
     */
    onError(handler: ErrorHandler): () => void {
        this.onErrorHandlers.add(handler);
        return () => this.onErrorHandlers.delete(handler);
    }

    /**
     * Disconnect from WebSocket server
     */
    disconnect(): void {
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }

        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }

        this.connected = false;
        this.reconnectAttempts = 0;
    }

    /**
     * Check if connected
     */
    isConnected(): boolean {
        return this.connected;
    }
}

// Export singleton instance
export const websocketService = new WebSocketService();
