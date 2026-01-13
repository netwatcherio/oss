// src/router/index.ts
import type { RouteRecordRaw } from 'vue-router'
import { createRouter, createWebHistory } from 'vue-router'
import { defineComponent, h } from 'vue'

import RootView from '@/views/Root.vue'

// route modules (may export a single route or an array)
import auth from '@/views/auth'

// workspace views
import Workspaces from '@/views/Workspaces.vue'
import NewWorkspace from '@/views/workspace/NewWorkspace.vue'
import Workspace from '@/views/workspace/Workspace.vue'
import EditWorkspace from '@/views/workspace/EditWorkspace.vue'
import Members from '@/views/workspace/Members.vue'
import InviteMember from '@/views/workspace/InviteMember.vue'
import RemoveMember from '@/views/workspace/RemoveMember.vue'
import EditMember from '@/views/workspace/EditMember.vue'

// agent views
import Agent from '@/views/agent/Agent.vue'
import NewAgent from '@/views/agent/NewAgent.vue'
import EditAgent from '@/views/agent/EditAgent.vue'
import DeactivateAgent from '@/views/agent/DeactivateAgent.vue'
import ProbesEdit from '@/views/agent/ProbesEdit.vue'
import DeleteAgent from '@/views/agent/DeleteAgent.vue'
import Speedtests from '@/views/agent/Speedtests.vue'
import NewSpeedtest from '@/views/agent/NewSpeedtest.vue'

// profile and probes
import BasicView from '@/views/BasicView.vue'
import Profile from '@/views/profile/Profile.vue'
import Probe from '@/views/probes/Probe.vue'
import NewProbe from '@/views/probes/NewProbe.vue'
import DeleteProbe from '@/views/probes/DeleteProbe.vue'
import Alerts from '@/views/Alerts.vue'

// admin views
import AdminDashboard from '@/views/admin/AdminDashboard.vue'
import AdminUsers from '@/views/admin/AdminUsers.vue'
import AdminWorkspaces from '@/views/admin/AdminWorkspaces.vue'
import AdminWorkspaceDetail from '@/views/admin/AdminWorkspaceDetail.vue'
import AdminAgents from '@/views/admin/AdminAgents.vue'

// Permission utilities
import { hasMinimumRole } from '@/composables/usePermissions'
import { WorkspaceService } from '@/services/apiService'

// Helper: normalize module export to array
const asArray = (r: unknown) => (Array.isArray(r) ? r : r ? [r] : [])

// Minimal shells that just render a <router-view />
const Shell = defineComponent({
    name: 'Shell',
    setup() {
        return () => h('div', [h('router-view')])
    },
})

const NotFound = defineComponent({
    name: 'NotFound',
    setup() {
        return () =>
            h('div', { style: 'padding:2rem' }, [
                h('h2', '404 - Not Found'),
                h('p', 'No route matched this URL.'),
            ])
    },
})

// 403 Forbidden page
const Forbidden = defineComponent({
    name: 'Forbidden',
    setup() {
        return () =>
            h('div', { class: 'container-fluid py-5 text-center' }, [
                h('i', { class: 'bi bi-shield-exclamation text-danger', style: 'font-size: 4rem' }),
                h('h2', { class: 'mt-3' }, 'Access Denied'),
                h('p', { class: 'text-muted' }, 'You do not have permission to access this page.'),
                h('a', { href: '/workspaces', class: 'btn btn-primary mt-3' }, 'Back to Workspaces'),
            ])
    },
})

// Route meta type extension
declare module 'vue-router' {
    interface RouteMeta {
        /** Minimum role required to access this route */
        requiresRole?: 'VIEWER' | 'USER' | 'ADMIN' | 'OWNER' | 'READ_ONLY' | 'READ_WRITE'
        /** Requires site-wide admin access */
        requiresSiteAdmin?: boolean
    }
}

const routes: RouteRecordRaw[] = [
    // External auth routes (whatever your module exports)
    ...asArray(auth),

    // App shell
    {
        path: '/',
        component: RootView,
        children: [
            // Redirect root -> workspaces
            { path: '', redirect: { name: 'workspaces' } },

            // ----- /workspaces (list + create) -----
            { path: 'workspaces', name: 'workspaces', component: Workspaces },
            { path: 'workspaces/alerts', name: 'workspaceAlerts', component: Alerts },
            { path: 'workspaces/new', name: 'workspaceNew', component: NewWorkspace },

            // ----- /workspaces/:wID (shell with children) -----
            {
                path: 'workspaces/:wID(\\d+)',
                component: BasicView,
                props: true,
                children: [
                    // Dashboard at /workspaces/:wID (any member can view)
                    { path: '', name: 'workspace', component: Workspace, props: true },

                    // Edit: /workspaces/:wID/edit (ADMIN+)
                    {
                        path: 'edit',
                        name: 'workspaceEdit',
                        component: EditWorkspace,
                        props: true,
                        meta: { requiresRole: 'ADMIN' }
                    },

                    // ----- Members: /workspaces/:wID/members[...] -----
                    {
                        path: 'members',
                        component: BasicView,
                        props: true,
                        children: [
                            { path: '', name: 'workspaceMembers', component: Members, props: true },
                            {
                                path: 'invite',
                                name: 'workspaceInvite',
                                component: InviteMember,
                                props: true,
                                meta: { requiresRole: 'ADMIN' }
                            },
                            {
                                path: 'remove/:userId(\\d+)',
                                name: 'workspaceMemberRemove',
                                component: RemoveMember,
                                props: true,
                                meta: { requiresRole: 'ADMIN' }
                            },
                            {
                                path: 'edit/:userId(\\d+)',
                                name: 'workspaceMemberEdit',
                                component: EditMember,
                                props: true,
                                meta: { requiresRole: 'ADMIN' }
                            },
                        ],
                    },

                    // ----- Agents: /workspaces/:wID/agents[...] -----
                    {
                        path: 'agents',
                        component: BasicView,
                        props: true,
                        children: [
                            // /workspaces/:wID/agents/new (USER+)
                            {
                                path: 'new',
                                name: 'agentNew',
                                component: NewAgent,
                                props: true,
                                meta: { requiresRole: 'USER' }
                            },

                            // /workspaces/:wID/agents/:aID
                            {
                                path: ':aID(\\d+)',
                                component: BasicView,
                                props: true,
                                children: [
                                    // agent overview (any member)
                                    { path: '', name: 'agent', component: Agent, props: true },

                                    // edit agent (USER+)
                                    {
                                        path: 'edit',
                                        name: 'agentEdit',
                                        component: EditAgent,
                                        props: true,
                                        meta: { requiresRole: 'USER' }
                                    },
                                    // deactivate agent (USER+)
                                    {
                                        path: 'deactivate',
                                        name: 'agentDeactivate',
                                        component: DeactivateAgent,
                                        props: true,
                                        meta: { requiresRole: 'USER' }
                                    },
                                    // delete agent (ADMIN+)
                                    {
                                        path: 'delete',
                                        name: 'agentDelete',
                                        component: DeleteAgent,
                                        props: true,
                                        meta: { requiresRole: 'ADMIN' }
                                    },

                                    // probes - new (USER+)
                                    {
                                        path: 'probes/new',
                                        name: 'probeNew',
                                        component: NewProbe,
                                        props: true,
                                        meta: { requiresRole: 'USER' }
                                    },
                                    // probes - view (any member)
                                    { path: 'probes/:pID(\\d+)', name: 'agentProbe', component: Probe, props: true },
                                    // probes - edit (USER+)
                                    {
                                        path: 'probes/edit',
                                        name: 'agentProbesEdit',
                                        component: ProbesEdit,
                                        props: true,
                                        meta: { requiresRole: 'USER' }
                                    },
                                    // probes - delete (ADMIN+)
                                    {
                                        path: 'probes/:pID(\\d+)/delete',
                                        name: 'probeDelete',
                                        component: DeleteProbe,
                                        props: true,
                                        meta: { requiresRole: 'ADMIN' }
                                    },

                                    // speedtests
                                    { path: 'speedtests', name: 'agentSpeedtests', component: Speedtests, props: true },
                                    {
                                        path: 'speedtests/new',
                                        name: 'agentSpeedtestNew',
                                        component: NewSpeedtest,
                                        props: true,
                                        meta: { requiresRole: 'USER' }
                                    },
                                ],
                            },
                        ],
                    },
                ],
            },

            // Profile at /profile
            { path: 'profile', name: 'profile', component: Profile },

            // IP Lookup at /lookup
            {
                path: 'lookup',
                name: 'ipLookup',
                component: () => import('@/views/IpLookup.vue'),
            },

            // Admin routes (requires SITE_ADMIN role)
            {
                path: 'admin',
                component: BasicView,
                meta: { requiresSiteAdmin: true },
                children: [
                    { path: '', name: 'adminDashboard', component: AdminDashboard },
                    { path: 'users', name: 'adminUsers', component: AdminUsers },
                    { path: 'workspaces', name: 'adminWorkspaces', component: AdminWorkspaces },
                    { path: 'workspaces/:wID', name: 'adminWorkspaceDetail', component: AdminWorkspaceDetail },
                    { path: 'agents', name: 'adminAgents', component: AdminAgents },
                ],
            },

            // 403 Forbidden
            { path: '403', name: 'forbidden', component: Forbidden },

            // 404 (must be last among siblings)
            { path: ':pathMatch(.*)*', name: 'not-found', component: NotFound },
        ],
    },
]

const router = createRouter({
    history: createWebHistory(import.meta.env.BASE_URL),
    routes,
})

// Cache for workspace roles to avoid repeated API calls
const workspaceRoleCache = new Map<string, { role: string; timestamp: number }>()
const CACHE_TTL_MS = 60_000 // 1 minute

/**
 * Get user's role for a workspace, with caching
 */
async function getUserRoleForWorkspace(workspaceId: string): Promise<string | null> {
    const cached = workspaceRoleCache.get(workspaceId)
    if (cached && Date.now() - cached.timestamp < CACHE_TTL_MS) {
        return cached.role
    }

    try {
        const workspace = await WorkspaceService.get(workspaceId)
        const role = (workspace as { my_role?: string }).my_role || null
        if (role) {
            workspaceRoleCache.set(workspaceId, { role, timestamp: Date.now() })
        }
        return role
    } catch {
        return null
    }
}

// Navigation guard for permission checking
router.beforeEach(async (to, _from, next) => {
    // Check for site admin requirement first
    if (to.matched.some(r => r.meta.requiresSiteAdmin)) {
        try {
            const session = JSON.parse(localStorage.getItem('session') || '{}')
            const token = session?.token || localStorage.getItem('token')
            if (token) {
                // Use same base URL as rest of app
                const baseUrl = (window as any).CONTROLLER_ENDPOINT || import.meta.env.VITE_CONTROLLER_ENDPOINT || ''
                const response = await fetch(`${baseUrl}/auth/me`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                })
                if (response.ok) {
                    const user = await response.json()
                    if (user.role !== 'SITE_ADMIN') {
                        return next({ name: 'forbidden' })
                    }
                } else {
                    console.warn('[Admin Route] Auth check failed:', response.status)
                    return next({ name: 'login' })
                }
            } else {
                return next({ name: 'login' })
            }
        } catch (err) {
            console.error('[Admin Route] Error checking admin access:', err)
            return next({ name: 'login' })
        }
    }

    const requiredRole = to.meta.requiresRole
    if (!requiredRole) {
        return next()
    }

    // Extract workspace ID from route params
    const workspaceId = to.params.wID as string
    if (!workspaceId) {
        console.warn('Route requires role but no workspace ID found:', to.path)
        return next({ name: 'forbidden' })
    }

    // Get user's role for this workspace
    const userRole = await getUserRoleForWorkspace(workspaceId)
    if (!userRole) {
        console.warn('Could not determine user role for workspace:', workspaceId)
        return next({ name: 'forbidden' })
    }

    // Check if user has minimum required role
    if (!hasMinimumRole(userRole, requiredRole)) {
        console.warn(`Access denied: requires ${requiredRole}, user has ${userRole}`)
        return next({ name: 'forbidden' })
    }

    next()
})

export default router