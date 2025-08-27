import request from "@/services/request";

export default {
    async getProfile(): Promise<any> {
        return request.get("/profile")
    },
}
