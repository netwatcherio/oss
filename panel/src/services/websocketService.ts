/**
 * WebSocket Service for NetWatcher Panel
 * 
 * Connects to the controller's WebSocket server using the neffos protocol
 * for real-time probe data updates.
 */

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

interface NeffosMessage {
    Namespace: string;
    Event: string;
    Body?: string;
    isConnect?: boolean;
    isDisconnect?: boolean;
}

class WebSocketService {
    private ws: WebSocket | null = null;
    private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    private reconnectAttempts = 0;
    private maxReconnectAttempts = 10;
    private baseReconnectDelay = 1000; // 1 second

    private probeDataHandlers: Map<string, Set<ProbeDataHandler>> = new Map();
    private onConnectHandlers: Set<ConnectionHandler> = new Set();
    private onDisconnectHandlers: Set<ConnectionHandler> = new Set();
    private onErrorHandlers: Set<ErrorHandler> = new Set();

    private pendingSubscriptions: Array<{ workspaceId: number; probeId: number }> = [];
    private activeSubscriptions: Set<string> = new Set();
    private namespaceConnected = false;

    /**
     * Get the WebSocket URL based on the controller endpoint
     */
    private getWebSocketUrl(token: string): string {
        const anyWindow = window as any;
        let baseUrl = anyWindow?.CONTROLLER_ENDPOINT
            || import.meta.env?.CONTROLLER_ENDPOINT
            || "http://localhost:8080";

        // Convert HTTP URL to WebSocket URL
        baseUrl = baseUrl.replace(/^http/, 'ws');

        // Use /ws/panel endpoint with token as query param (browsers can't send headers)
        return `${baseUrl}/ws/panel?token=${encodeURIComponent(token)}`;
    }

    /**
     * Connect to the WebSocket server
     */
    connect(): void {
        if (this.ws?.readyState === WebSocket.OPEN || this.ws?.readyState === WebSocket.CONNECTING) {
            return;
        }

        const session = getSession();
        if (!session?.token) {
            console.warn('[WebSocket] No session token, cannot connect');
            return;
        }

        const url = this.getWebSocketUrl(session.token);
        console.log('[WebSocket] Connecting to', url.replace(/token=[^&]+/, 'token=***'));

        try {
            // Connect with token in query param (browsers can't send Authorization header)
            this.ws = new WebSocket(url);

            this.ws.onopen = () => {
                console.log('[WebSocket] Connected');
                this.reconnectAttempts = 0;

                // Send neffos connection handshake with JWT
                // neffos expects: {"Namespace":"panel","Event":"_OnNamespaceConnected"}
                this.sendNeffosConnect(session.token);
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
                this.namespaceConnected = false;
                this.onDisconnectHandlers.forEach(h => h());
                this.scheduleReconnect();
            };
        } catch (error) {
            console.error('[WebSocket] Connection error:', error);
            this.scheduleReconnect();
        }
    }

    /**
     * Send neffos protocol connect message with JWT auth
     */
    private sendNeffosConnect(token: string): void {
        // neffos protocol: first message is the ack message
        // Format: 4{} where 4 is the ACK message type
        if (this.ws?.readyState === WebSocket.OPEN) {
            // neffos expects the client to send ack first
            this.ws.send('4{}');

            // Then join the panel namespace with auth in header simulation
            // We'll include auth info in a special connect payload
            const connectMsg: NeffosMessage = {
                Namespace: "panel",
                Event: "_OnNamespaceConnect",
                Body: JSON.stringify({ token })
            };

            // neffos message format: namespace;event;body
            this.sendNeffosMessage("panel", "_OnNamespaceConnect", "");
        }
    }

    /**
     * Send a neffos-formatted message
     */
    private sendNeffosMessage(namespace: string, event: string, body: string): void {
        if (this.ws?.readyState !== WebSocket.OPEN) {
            console.warn('[WebSocket] Cannot send message, not connected');
            return;
        }

        // neffos message format: namespace;event;body (with ; separator)
        // Using message type 0 (default) 
        const msg = `${namespace};${event};${body}`;
        console.log('[WebSocket] Sending:', msg);
        this.ws.send(msg);
    }

    /**
     * Handle incoming WebSocket messages
     */
    private handleMessage(data: string): void {
        console.log('[WebSocket] Received:', data);

        // neffos protocol messages
        if (data === '4{}') {
            // ACK response - namespace connection in progress
            console.log('[WebSocket] ACK received');
            return;
        }

        // Parse neffos message format: namespace;event;body
        const parts = data.split(';');
        if (parts.length < 2) {
            console.warn('[WebSocket] Invalid message format:', data);
            return;
        }

        const namespace = parts[0];
        const event = parts[1];
        const body = parts.slice(2).join(';'); // Body may contain semicolons

        if (namespace !== 'panel') return;

        switch (event) {
            case '_OnNamespaceConnected':
                console.log('[WebSocket] Panel namespace connected');
                this.namespaceConnected = true;
                this.onConnectHandlers.forEach(h => h());
                // Restore pending subscriptions
                this.restoreSubscriptions();
                break;

            case 'subscribe_ok':
                console.log('[WebSocket] Subscription confirmed');
                break;

            case 'probe_data':
                try {
                    const probeData: ProbeDataEvent = JSON.parse(body);
                    this.notifyProbeDataHandlers(probeData);
                } catch (e) {
                    console.error('[WebSocket] Failed to parse probe_data:', e);
                }
                break;

            case '_OnNamespaceDisconnect':
                console.log('[WebSocket] Panel namespace disconnected');
                this.namespaceConnected = false;
                break;

            default:
                console.log('[WebSocket] Unhandled event:', event);
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
        const body = JSON.stringify({ workspace_id: workspaceId, probe_id: probeId });
        this.sendNeffosMessage('panel', 'subscribe', body);
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
                if (this.namespaceConnected) {
                    const body = JSON.stringify({ workspace_id: workspaceId, probe_id: probeId });
                    this.sendNeffosMessage('panel', 'unsubscribe', body);
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

        if (this.ws) {
            this.ws.close();
            this.ws = null;
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
