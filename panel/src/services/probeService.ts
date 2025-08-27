import request from "@/services/request";
import type {Probe, ProbeConfig, ProbeDataRequest, ProbeTarget} from "@/types";

export default {
    async createProbe(id: string, probe: Probe): Promise<any> {
        return await request.post(`/probes/new/${id}`, probe)
    },
    async getAgentProbes(id: string): Promise<any> {
        return await request.get(`/probes/agent/${id}`)
    },
    async updateFirstProbeTarget(id: string, target: string): Promise<any> {
        let config = {target: [{target: target, agent: "", group: ""}] as ProbeTarget[]} as ProbeConfig
        let probe = {id: id, config} as Probe

        console.log(probe)

        return await request.post(`/probe/first_target_update/${id}`, probe)
    },
    async deleteProbe(id: string): Promise<any> {
        return await request.get(`/probe/delete/${id}`)
    },
    async getSimilarProbes(id: string): Promise<any> {
        return await request.get(`/probes/similar/${id}`)
    },
    async getProbeData(id: string, req: ProbeDataRequest): Promise<any> {
        return await request.post(`/probes/data/${id}`, req)
    },
    async getProbe(id: string): Promise<any> {
        return await request.get(`/probe/${id}`)
    },
    async getNetworkInfo(id: string): Promise<any> {
        return await request.get(`/netinfo/${id}`)
    },
    async getSystemInfo(id: string): Promise<any> {
        return await request.get(`/sysinfo/${id}`)
    },
}
