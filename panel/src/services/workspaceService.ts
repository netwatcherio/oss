// src/services/workspaces.ts
import request from "@/services/request";
import type { AgentGroup, MemberInfo, Site } from "@/types";

// NOTE on members:
// - GET    /workspaces/:id/members
// - POST   /workspaces/:id/members                 { userId?: number, email?: string, role: 'READ_ONLY'|'READ_WRITE'|'ADMIN' }
// - PATCH  /workspaces/:id/members/:memberId       { role: ... }
// - DELETE /workspaces/:id/members/:memberId
//
// Your previous MemberInfo may have held a userId/email instead of a memberId.
// For update/remove you now need the WorkspaceMember row id (memberId).

export default {
    // === Workspaces ===
    async getSites(): Promise<any> {
        // was: GET /workspaces
        return await request.get("/workspaces");
    },

    async createSite(site: Site): Promise<any> {
        // was: POST /workspaces
        return await request.post("/workspaces", site);
    },

    async getSite(id: string): Promise<any> {
        // was: GET /workspaces/{id}
        return await request.get(`/workspaces/${id}`);
    },

    async updateSite(site: Site & { id: string }): Promise<void> {
        // was: POST /workspaces/update/{id}
        // now: PATCH /workspaces/{id}
        // send only updatable fields (backend accepts partials)
        const { id, name, location, description } = site as any;
        await request.post(`/workspaces/${id}`, { name, location, description });
    },

    // Optional new helper (clearer name):
    // async updateWorkspace(id: string, patch: Partial<Pick<Site,'name'|'location'|'description'>>) {
    //   return await request.patch(`/workspaces/${id}`, patch);
    // },

    // === Members ===
    async getMemberInfos(id: string): Promise<any> {
        // was: /workspaces/{id}/memberinfo
        // now: /workspaces/{id}/members
        return await request.get(`/workspaces/${id}/members`);
    },

    async createNewMember(id: string, member: MemberInfo): Promise<any> {
        // was: POST /workspaces/{id}/invite
        // now: POST /workspaces/{id}/members
        // Expecting payload like { userId?: number, email?: string, role: 'READ_ONLY'|'READ_WRITE'|'ADMIN' }
        return await request.post(`/workspaces/${id}/members`, member);
    },

    async updateMember(id: string, member: MemberInfo & { memberId?: number }): Promise<any> {
        // was: POST /workspaces/{id}/update_role
        // now: PATCH /workspaces/{id}/members/{memberId}
        // Ensure you pass the WorkspaceMember row id as member.memberId (not userId).
        const memberId = (member as any).memberId ?? (member as any).id;
        return await request.post(`/workspaces/${id}/members/${memberId}`, { role: (member as any).role });
    },

    async removeMember(id: string, member: MemberInfo & { memberId?: number }): Promise<any> {
        // was: POST /workspaces/{id}/remove
        // now: DELETE /workspaces/{id}/members/{memberId}
        const memberId = (member as any).memberId ?? (member as any).id;
        return await request.post(`/workspaces/${id}/members/${memberId}`);
    },

    // Optional explicit helpers (use these if you refactor callsites):
    // async addMemberByUserId(id: string, userId: number, role: string) {
    //   return await request.post(`/workspaces/${id}/members`, { userId, role });
    // },
    // async inviteMemberByEmail(id: string, email: string, role: string) {
    //   return await request.post(`/workspaces/${id}/members`, { email, role });
    // },

    // === Groups ===
    async getAgentGroups(id: string): Promise<any> {
        // unchanged path (GET is supported): /workspaces/{id}/groups
        return await request.get(`/workspaces/${id}/groups`);
    },

    async createAgentGroup(id: string, group: AgentGroup): Promise<any> {
        // if you enable POST on backend party: /workspaces/{id}/groups
        // (your server currently exposes only GET; enable POST there to use this)
        return await request.post(`/workspaces/${id}/groups`, group);
    },
};