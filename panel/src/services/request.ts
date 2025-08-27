import axios from "axios";
import type {Session} from "@/session";
import {getSession} from "@/session";

interface RequestHeaders {
    mode: string
    cache: string
    headers: {
        Authorization: string
    }
}

// Generates the headers for an endpoint request with the user's session token
function getHeaders(): RequestHeaders {
    let token: string = "";
    // Attempt to retrieve the user session
    let session: Session = getSession();
    // Set the authentication token to the stored token if present
    if (session.token) {
        token = session.token;
    }
    return {
        mode: 'cors',
        cache: 'no-cache',
        headers: {
            Authorization: "Bearer " + token
        }
    };
}

// returns the preferred guardian endpoint
function host(): string {
    // Attempt to leverage implementation-specific endpoints
    let envUrl: string = import.meta.env.NW_ENDPOINT;
    // Return the environment-declared url if it is present
    if (envUrl) {
        return envUrl;
    }
    // Attempt to load the globally defined default endpoint url
    let globalUrl: string = import.meta.env.NW_GLOBAL_ENDPOINT;
    // Return the globally declared url if it is present
    if (globalUrl) {
        return globalUrl;
    }
    // Return a hard coded value if no other url is found
    return 'https://api.netwatcher.io';
}

export default {
    async post(url: string, data?: {} | undefined): Promise<any> {
        return await axios.post(`${host()}${url}`, data, getHeaders());
    },
    async get(url: string, data?: {} | undefined): Promise<any> {
        return axios.get(`${host()}${url}`, getHeaders());
    }
}

