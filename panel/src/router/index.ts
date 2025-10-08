// src/router/index.ts
import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { h, defineComponent } from 'vue'

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
import AgentGroups from '@/views/workspace/AgentGroups.vue'
import NewAgentGroup from '@/views/workspace/NewAgentGroup.vue'

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
            { path: 'workspaces/new', name: 'workspaceNew', component: NewWorkspace },

            // ----- /workspaces/:wID (shell with children) -----
            {
                path: 'workspaces/:wID(\\d+)',
                component: BasicView,
                props: true,
                children: [
                    // Dashboard at /workspaces/:wID
                    { path: '', name: 'workspace', component: Workspace, props: true },

                    // Edit: /workspaces/:wID/edit
                    { path: 'edit', name: 'workspaceEdit', component: EditWorkspace, props: true },

                    // ----- Members: /workspaces/:wID/members[...] -----
                    {
                        path: 'members',
                        component: BasicView,
                        props: true,
                        children: [
                            { path: '', name: 'workspaceMembers', component: Members, props: true },
                            { path: 'invite', name: 'workspaceInvite', component: InviteMember, props: true },
                            {
                                path: 'remove/:userId(\\d+)',
                                name: 'workspaceMemberRemove',
                                component: RemoveMember,
                                props: true,
                            },
                            {
                                path: 'edit/:userId(\\d+)',
                                name: 'workspaceMemberEdit',
                                component: EditMember,
                                props: true,
                            },
                        ],
                    },

                    // ----- Groups (uncomment if/when used) -----
                    // {
                    //   path: 'groups',
                    //   component: Shell,
                    //   props: true,
                    //   children: [
                    //     { path: '', name: 'workspaceGroups', component: AgentGroups, props: true },
                    //     { path: 'new', name: 'workspaceGroupsNew', component: NewAgentGroup, props: true },
                    //   ],
                    // },

                    // ----- Agents: /workspaces/:wID/agents[...] -----
                    {
                        path: 'agents',
                        component: BasicView,
                        props: true,
                        children: [
                            // /workspaces/:wID/agents/new
                            { path: 'new', name: 'agentNew', component: NewAgent, props: true },

                            // /workspaces/:wID/agents/:aID
                            {
                                path: ':aID(\\d+)',
                                component: BasicView,
                                props: true,
                                children: [
                                    // agent overview (or details)
                                    { path: '', name: 'agent', component: Agent, props: true },

                                    // simple actions on the agent
                                    { path: 'edit', name: 'agentEdit', component: EditAgent, props: true },
                                    { path: 'deactivate', name: 'agentDeactivate', component: DeactivateAgent, props: true },
                                    { path: 'delete', name: 'agentDelete', component: DeleteAgent, props: true },

                                    // probes editor page (bulk edit)
                                    { path: 'probes', name: 'agentProbesEdit', component: ProbesEdit, props: true },

                                    // nested probe routes
                                    {
                                        path: 'probes',
                                        component: Shell,
                                        props: true,
                                        children: [
                                            { path: 'new', name: 'probeNew', component: NewProbe, props: true },
                                            { path: ':pID(\\d+)', name: 'probe', component: Probe, props: true },
                                            { path: ':pID(\\d+)/delete', name: 'probeDelete', component: DeleteProbe, props: true },
                                        ],
                                    },

                                    // speedtests
                                    {
                                        path: 'speedtests',
                                        component: Shell,
                                        props: true,
                                        children: [
                                            { path: '', name: 'agentSpeedtests', component: Speedtests, props: true },
                                            { path: 'new', name: 'agentSpeedtestNew', component: NewSpeedtest, props: true },
                                        ],
                                    },
                                ],
                            },
                        ],
                    },
                ],
            },

            // Profile at /profile
            { path: 'profile', name: 'profile', component: Profile },

            // 404 (must be last among siblings)
            { path: ':pathMatch(.*)*', name: 'not-found', component: NotFound },
        ],
    },
]

const router = createRouter({
    history: createWebHistory(import.meta.env.BASE_URL),
    routes,
})

export default router