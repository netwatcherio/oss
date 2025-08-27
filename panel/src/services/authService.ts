import request from "@/services/request";
import type {User} from "@/types"
import type {AxiosResponse} from "axios";

export default {
    async authLogin(user: User): Promise<any> {
        return await request.post("/auth/login", user)
    },
    async authRegister(user: User): Promise<any> {
        return await request.post("/auth/register", user)
    },
}
