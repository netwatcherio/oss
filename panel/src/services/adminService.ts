// Admin API Service
// Provides access to site admin endpoints

import request from './request';

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

// Stats
export async function getStats(): Promise<AdminStats> {
    const res = await request.get<AdminStats>('/admin/stats');
    return res.data;
}

export async function getWorkspaceStats(): Promise<ListResponse<WorkspaceStats>> {
    const res = await request.get<ListResponse<WorkspaceStats>>('/admin/workspace-stats');
    return res.data;
}

// Users
export async function listUsers(limit = 50, offset = 0, query = ''): Promise<ListResponse<AdminUser>> {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    if (query) params.set('q', query);
    const res = await request.get<ListResponse<AdminUser>>(`/admin/users?${params}`);
    return res.data;
}

export async function getUser(id: number): Promise<AdminUser> {
    const res = await request.get<AdminUser>(`/admin/users/${id}`);
    return res.data;
}

export async function updateUser(id: number, data: { email?: string; name?: string; verified?: boolean }): Promise<AdminUser> {
    const res = await request.patch<AdminUser>(`/admin/users/${id}`, data);
    return res.data;
}

export async function deleteUser(id: number): Promise<void> {
    await request.delete(`/admin/users/${id}`);
}

export async function setUserRole(id: number, role: 'USER' | 'SITE_ADMIN'): Promise<AdminUser> {
    const res = await request.patch<AdminUser>(`/admin/users/${id}/role`, { role });
    return res.data;
}

// Workspaces
export async function listWorkspaces(limit = 50, offset = 0, query = ''): Promise<ListResponse<AdminWorkspace>> {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    if (query) params.set('q', query);
    const res = await request.get<ListResponse<AdminWorkspace>>(`/admin/workspaces?${params}`);
    return res.data;
}

export async function getWorkspace(id: number): Promise<{ workspace: AdminWorkspace; members: unknown[]; agents: unknown[] }> {
    const res = await request.get<{ workspace: AdminWorkspace; members: unknown[]; agents: unknown[] }>(`/admin/workspaces/${id}`);
    return res.data;
}

export async function updateWorkspace(id: number, data: { name?: string; description?: string }): Promise<AdminWorkspace> {
    const res = await request.patch<AdminWorkspace>(`/admin/workspaces/${id}`, data);
    return res.data;
}

export async function deleteWorkspace(id: number): Promise<void> {
    await request.delete(`/admin/workspaces/${id}`);
}

// Workspace Members
export async function listMembers(workspaceId: number): Promise<ListResponse<unknown>> {
    const res = await request.get<ListResponse<unknown>>(`/admin/workspaces/${workspaceId}/members`);
    return res.data;
}

export async function addMember(workspaceId: number, data: { user_id?: number; email?: string; role: string }): Promise<unknown> {
    const res = await request.post(`/admin/workspaces/${workspaceId}/members`, data);
    return res.data;
}

export async function updateMember(workspaceId: number, memberId: number, role: string): Promise<unknown> {
    const res = await request.patch(`/admin/workspaces/${workspaceId}/members/${memberId}`, { role });
    return res.data;
}

export async function removeMember(workspaceId: number, memberId: number): Promise<void> {
    await request.delete(`/admin/workspaces/${workspaceId}/members/${memberId}`);
}

// Agents
export async function listAgents(limit = 50, offset = 0): Promise<ListResponse<AdminAgent>> {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    const res = await request.get<ListResponse<AdminAgent>>(`/admin/agents?${params}`);
    return res.data;
}

export async function getAgentStats(): Promise<ListResponse<WorkspaceStats>> {
    const res = await request.get<ListResponse<WorkspaceStats>>('/admin/agents/stats');
    return res.data;
}

// Debug / System Status
interface AgentConnection {
    agent_id: number;
    agent_name: string;
    workspace_id: number;
    workspace_name: string;
    conn_id: string;
    client_ip: string;
    connected_at: string;
}

interface WorkspaceGroup {
    workspace_id: number;
    workspace_name: string;
    agent_count: number;
    connections: AgentConnection[];
}

interface DebugConnectionsResponse {
    connected_count: number;
    workspace_count: number;
    connections: AgentConnection[];
    by_workspace: WorkspaceGroup[];
}

export async function getDebugConnections(): Promise<DebugConnectionsResponse> {
    const res = await request.get<DebugConnectionsResponse>('/admin/debug/connections');
    return res.data;
}

export type { AdminStats, WorkspaceStats, AdminUser, AdminWorkspace, AdminAgent, ListResponse, AgentConnection, WorkspaceGroup, DebugConnectionsResponse };



