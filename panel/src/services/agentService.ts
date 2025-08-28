import request from "@/services/request";
import type {Agent, Workspace} from "@/types";

export default {
    async createAgent(agent: Agent): Promise<any> {
        return await request.post(`/workspaces/${agent.workspaceId}/agents`, agent)
    },
    async updateAgent(agent: Agent): Promise<any> {
        return await request.post(`/workspaces/${agent.workspaceId}/agents/${agent.id}/update`, agent)
    },
    async deactivateAgent(workspaceId: string, id: string): Promise<any> {
        return await request.post(`/workspaces/${workspaceId}/agents/${id}/deactivate`)
    },
    async getWorkspaceAgents(id: string): Promise<any> {
        return await request.get(`/workspaces/${id}/agents`)
    },
    async getAgent(workspaceId: string, id: string): Promise<any> {
        return await request.get(`/workspaces/${workspaceId}/agents/${id}`)
    },
    async deleteAgent(workspaceId: string, id: string): Promise<any> {
        return await request.post(`/workspaces/${workspaceId}/agents/${id}/delete`)
    },
}
