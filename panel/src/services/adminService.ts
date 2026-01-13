// Admin API Service
// Provides access to site admin endpoints

const API_BASE = import.meta.env.VITE_API_URL || '';

interface AdminStats {
    total_users: number;
    total_workspaces: number;
    total_agents: number;
    active_agents: number;
    total_probes: number;
    generated_at: string;
}

interface WorkspaceStats {
    workspace_id: number;
    workspace_name: string;
    member_count: number;
    agent_count: number;
    active_agents: number;
    probe_count: number;
}

interface AdminUser {
    id: number;
    email: string;
    name: string;
    role: string;
    verified: boolean;
    labels: Record<string, unknown>;
    metadata: Record<string, unknown>;
    last_login_at: string | null;
    created_at: string;
    updated_at: string;
}

interface AdminWorkspace {
    id: number;
    name: string;
    owner_id: number;
    description: string;
    settings: Record<string, unknown>;
    created_at: string;
    updated_at: string;
}

interface AdminAgent {
    id: number;
    workspace_id: number;
    workspace_name: string;
    name: string;
    description: string;
    version: string;
    location: string;
    last_seen_at: string;
    initialized: boolean;
    created_at: string;
    is_online: boolean;
}

interface ListResponse<T> {
    data: T[];
    total?: number;
    limit?: number;
    offset?: number;
}

function getAuthHeaders(): HeadersInit {
    const token = localStorage.getItem('token');
    return {
        'Content-Type': 'application/json',
        ...(token ? { 'Authorization': `Bearer ${token}` } : {})
    };
}

async function fetchJSON<T>(url: string, options: RequestInit = {}): Promise<T> {
    const response = await fetch(url, {
        ...options,
        headers: {
            ...getAuthHeaders(),
            ...options.headers
        }
    });

    if (!response.ok) {
        const error = await response.json().catch(() => ({ error: response.statusText }));
        throw new Error(error.error || `HTTP ${response.status}`);
    }

    return response.json();
}

// Stats
export async function getStats(): Promise<AdminStats> {
    return fetchJSON<AdminStats>(`${API_BASE}/admin/stats`);
}

export async function getWorkspaceStats(): Promise<ListResponse<WorkspaceStats>> {
    return fetchJSON<ListResponse<WorkspaceStats>>(`${API_BASE}/admin/workspace-stats`);
}

// Users
export async function listUsers(limit = 50, offset = 0, query = ''): Promise<ListResponse<AdminUser>> {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    if (query) params.set('q', query);
    return fetchJSON<ListResponse<AdminUser>>(`${API_BASE}/admin/users?${params}`);
}

export async function getUser(id: number): Promise<AdminUser> {
    return fetchJSON<AdminUser>(`${API_BASE}/admin/users/${id}`);
}

export async function updateUser(id: number, data: { email?: string; name?: string; verified?: boolean }): Promise<AdminUser> {
    return fetchJSON<AdminUser>(`${API_BASE}/admin/users/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data)
    });
}

export async function deleteUser(id: number): Promise<void> {
    await fetchJSON<{ ok: boolean }>(`${API_BASE}/admin/users/${id}`, { method: 'DELETE' });
}

export async function setUserRole(id: number, role: 'USER' | 'SITE_ADMIN'): Promise<AdminUser> {
    return fetchJSON<AdminUser>(`${API_BASE}/admin/users/${id}/role`, {
        method: 'PUT',
        body: JSON.stringify({ role })
    });
}

// Workspaces
export async function listWorkspaces(limit = 50, offset = 0, query = ''): Promise<ListResponse<AdminWorkspace>> {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    if (query) params.set('q', query);
    return fetchJSON<ListResponse<AdminWorkspace>>(`${API_BASE}/admin/workspaces?${params}`);
}

export async function getWorkspace(id: number): Promise<{ workspace: AdminWorkspace; members: unknown[]; agents: unknown[] }> {
    return fetchJSON(`${API_BASE}/admin/workspaces/${id}`);
}

export async function updateWorkspace(id: number, data: { name?: string; description?: string }): Promise<AdminWorkspace> {
    return fetchJSON<AdminWorkspace>(`${API_BASE}/admin/workspaces/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data)
    });
}

export async function deleteWorkspace(id: number): Promise<void> {
    await fetchJSON<{ ok: boolean }>(`${API_BASE}/admin/workspaces/${id}`, { method: 'DELETE' });
}

// Workspace Members
export async function listMembers(workspaceId: number): Promise<ListResponse<unknown>> {
    return fetchJSON(`${API_BASE}/admin/workspaces/${workspaceId}/members`);
}

export async function addMember(workspaceId: number, data: { user_id?: number; email?: string; role: string }): Promise<unknown> {
    return fetchJSON(`${API_BASE}/admin/workspaces/${workspaceId}/members`, {
        method: 'POST',
        body: JSON.stringify(data)
    });
}

export async function updateMember(workspaceId: number, memberId: number, role: string): Promise<unknown> {
    return fetchJSON(`${API_BASE}/admin/workspaces/${workspaceId}/members/${memberId}`, {
        method: 'PUT',
        body: JSON.stringify({ role })
    });
}

export async function removeMember(workspaceId: number, memberId: number): Promise<void> {
    await fetchJSON<{ ok: boolean }>(`${API_BASE}/admin/workspaces/${workspaceId}/members/${memberId}`, {
        method: 'DELETE'
    });
}

// Agents
export async function listAgents(limit = 50, offset = 0): Promise<ListResponse<AdminAgent>> {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    return fetchJSON<ListResponse<AdminAgent>>(`${API_BASE}/admin/agents?${params}`);
}

export async function getAgentStats(): Promise<ListResponse<WorkspaceStats>> {
    return fetchJSON<ListResponse<WorkspaceStats>>(`${API_BASE}/admin/agents/stats`);
}

export type { AdminStats, WorkspaceStats, AdminUser, AdminWorkspace, AdminAgent, ListResponse };
