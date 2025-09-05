import type {Router} from "vue-router";
import {useRouter} from "vue-router";
import {inject} from "vue";
import {usePersistent} from "@/persistent";
import type {Preferences, User} from "@/types";
import type {Remote} from "@/remote";
import {getSession, type Session} from "@/session";

export interface Core {
    router: () => Router,
    remote: () => Remote,
    persistent: () => Preferences
    session: () => Session
}

export default {
    router: () => useRouter(),
    remote: () => inject("remote") as Remote,
    persistent: () => usePersistent() as Preferences,
    session: () => getSession() as Session
} as Core
