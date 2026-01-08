import type { AxiosInstance, AxiosRequestConfig } from "axios";
import axios from "axios";
import { getSession } from "@/session";

function baseURL(): string {
    // Prefer a global override if present (e.g., set on index.html)
    const anyWindow = window as any;
    if (anyWindow?.CONTROLLER_ENDPOINT) return anyWindow.CONTROLLER_ENDPOINT as string;

    // Fallback env or default
    const envUrl = (import.meta as any)?.env?.CONTROLLER_ENDPOINT;
    if (envUrl) return envUrl as string;

    return "http://localhost:8080";
}

const client: AxiosInstance = axios.create({
    baseURL: baseURL(),
    withCredentials: false,
});

client.interceptors.request.use((config) => {
    const session = getSession();
    /*if (!config.headers) config.headers = {};*/
    if (session?.token) {
        (config.headers as any).Authorization = `Bearer ${session.token}`;
    }
    return config;
});

export default {
    get<T = any>(url: string, config?: AxiosRequestConfig) {
        return client.get<T>(url, config);
    },
    post<T = any>(url: string, data?: any, config?: AxiosRequestConfig) {
        return client.post<T>(url, data, config);
    },
    patch<T = any>(url: string, data?: any, config?: AxiosRequestConfig) {
        return client.patch<T>(url, data, config);
    },
    delete<T = any>(url: string, config?: AxiosRequestConfig) {
        return client.delete<T>(url, config);
    },
};