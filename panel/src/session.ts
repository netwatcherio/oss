// Lightweight session storage for JWT + user
export interface SessionUser {
    id: number | string;
    email?: string;
    name?: string;
    [k: string]: any;
}

export interface Session {
    token: string;
    user?: SessionUser;
}

const STORAGE_KEY = "netwatcher.session";

export function getSession(): Session | null {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (!raw) return null;
        return JSON.parse(raw) as Session;
    } catch {
        return null;
    }
}

export function setSession(s: Session) {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(s));
    window.dispatchEvent(new CustomEvent("session:changed", { detail: s }));
}

export function clearSession() {
    localStorage.removeItem(STORAGE_KEY);
    window.dispatchEvent(new CustomEvent("session:changed", { detail: null }));
}

// Optional listener helper
export function onSessionChanged(cb: (s: Session | null) => void) {
    const handler = (e: Event) => cb((e as CustomEvent).detail);
    window.addEventListener("session:changed", handler as EventListener);
    return () => window.removeEventListener("session:changed", handler as EventListener);
}