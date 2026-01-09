/**
 * WebSocket Service for NetWatcher Panel
 * 
 * Connects to the controller's WebSocket server using the official neffos.js client
 * for real-time probe data updates.
 */

import * as neffos from 'neffos.js';
import { getSession } from "@/session";

// Event types for the panel namespace
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

export type ProbeDataHandler = (data: ProbeDataEvent) => void;
export type ConnectionHandler = () => void;
export type ErrorHandler = (error: Event | Error) => void;

class WebSocketService {
    private conn: neffos.Conn | null = null;
    private nsConn: neffos.NSConn | null = null;
    private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    private reconnectAttempts = 0;
    private maxReconnectAttempts = 10;
    private baseReconnectDelay = 1000; // 1 second

    private probeDataHandlers: Map<string, Set<ProbeDataHandler>> = new Map();
    private onConnectHandlers: Set<ConnectionHandler> = new Set();
    private onDisconnectHandlers: Set<ConnectionHandler> = new Set();
    private onErrorHandlers: Set<ErrorHandler> = new Set();

    private activeSubscriptions: Set<string> = new Set();
    private namespaceConnected = false;

    /**
     * Get the WebSocket URL based on the controller endpoint
     */
    private getWebSocketUrl(token: string): string {
        const anyWindow = window as any;
        let baseUrl = anyWindow?.CONTROLLER_ENDPOINT
            || import.meta.env.VITE_CONTROLLER_ENDPOINT
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

        // Token passed as query param (browsers can't send custom headers)
        return `${baseUrl}/ws/panel?token=${encodeURIComponent(token)}`;
    }

    /**
     * Connect to the WebSocket server using neffos.js
     */
    async connect(): Promise<void> {
        const session = getSession();
        if (!session?.token) {
            console.log('[WebSocket] No session token, skipping connection');
            return;
        }

        if (this.conn) {
            console.log('[WebSocket] Already connected');
            return;
        }

        const url = this.getWebSocketUrl(session.token);
        console.log('[WebSocket] Connecting to', url.replace(/token=[^&]+/, 'token=***'));

        try {
            // Define namespace events
            const panelNamespace: neffos.Events = {
                _OnNamespaceConnected: (nsConn: neffos.NSConn, msg: neffos.Message) => {
                    console.log('[WebSocket] Panel namespace connected');
                    this.namespaceConnected = true;
                    this.reconnectAttempts = 0;
                    this.onConnectHandlers.forEach(h => h());
                    this.restoreSubscriptions();
                },
                _OnNamespaceDisconnect: (nsConn: neffos.NSConn, msg: neffos.Message) => {
                    console.log('[WebSocket] Panel namespace disconnected');
                    this.namespaceConnected = false;
                    this.onDisconnectHandlers.forEach(h => h());
                },
                subscribe_ok: (nsConn: neffos.NSConn, msg: neffos.Message) => {
                    console.log('[WebSocket] Subscription confirmed');
                },
                probe_data: (nsConn: neffos.NSConn, msg: neffos.Message) => {
                    try {
                        const data: ProbeDataEvent = JSON.parse(new TextDecoder().decode(msg.Body));
                        this.notifyProbeDataHandlers(data);
                    } catch (e) {
                        console.error('[WebSocket] Failed to parse probe_data:', e);
                    }
                }
            };

            // Dial with neffos
            this.conn = await neffos.dial(url, { panel: panelNamespace });
            console.log('[WebSocket] Connected to server');

            // Set up connection close handler
            this.conn.wasReconnected().then(() => {
                console.log('[WebSocket] Reconnection handler triggered');
            });

            // Connect to the panel namespace
            this.nsConn = await this.conn.connect("panel");
            console.log('[WebSocket] Joined panel namespace');

        } catch (error) {
            console.error('[WebSocket] Connection error:', error);
            this.onErrorHandlers.forEach(h => h(error as Error));
            this.scheduleReconnect();
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
            this.conn = null;
            this.nsConn = null;
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
                this.sendSubscribe(wsId, probeId);
            }
        });
    }

    /**
     * Send subscribe message
     */
    private sendSubscribe(workspaceId: number, probeId: number): void {
        if (!this.nsConn || !this.namespaceConnected) {
            console.warn('[WebSocket] Cannot send subscribe, not connected to namespace');
            return;
        }

        const body = JSON.stringify({ workspace_id: workspaceId, probe_id: probeId });
        this.nsConn.emit("subscribe", new TextEncoder().encode(body));
        console.log(`[WebSocket] Subscribed to workspace ${workspaceId} probe ${probeId}`);
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
        if (this.namespaceConnected) {
            this.sendSubscribe(workspaceId, probeId);
        }

        // Return unsubscribe function
        return () => {
            this.probeDataHandlers.get(key)?.delete(handler);
            if (this.probeDataHandlers.get(key)?.size === 0) {
                this.probeDataHandlers.delete(key);
                this.activeSubscriptions.delete(key);

                // Send unsubscribe if connected
                if (this.namespaceConnected && this.nsConn) {
                    const body = JSON.stringify({ workspace_id: workspaceId, probe_id: probeId });
                    this.nsConn.emit("unsubscribe", new TextEncoder().encode(body));
                }
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

        if (this.conn) {
            this.conn.close();
            this.conn = null;
            this.nsConn = null;
        }

        this.namespaceConnected = false;
        this.reconnectAttempts = 0;
    }

    /**
     * Check if connected
     */
    isConnected(): boolean {
        return this.namespaceConnected;
    }
}

// Export singleton instance
export const websocketService = new WebSocketService();
