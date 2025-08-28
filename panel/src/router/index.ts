// src/router/index.ts
import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { h, defineComponent } from 'vue'

import RootView from "@/views/Root.vue";

// route modules (may export a single route or an array)
import auth from '@/views/auth'

// workspace views
import Workspaces from '@/views/workspaces/Workspaces.vue'
import NewWorkspace from '@/views/workspaces/NewWorkspace.vue'
import WorkspaceDashboard from '@/views/workspaces/WorkspaceDashboard.vue'
import EditWorkspace from '@/views/workspaces/EditWorkspace.vue'
import Members from '@/views/workspaces/Members.vue'
import InviteMember from '@/views/workspaces/InviteMember.vue'
import RemoveMember from '@/views/workspaces/RemoveMember.vue'
import EditMember from '@/views/workspaces/EditMember.vue'
import AgentGroups from '@/views/workspaces/AgentGroups.vue'
import NewAgentGroup from '@/views/workspaces/NewAgentGroup.vue'

// agent views
import Agent from '@/views/agent/Agent.vue'
import NewAgent from '@/views/agent/NewAgent.vue'
import EditAgent from '@/views/agent/EditAgent.vue'
import DeactivateAgent from '@/views/agent/DeactivateAgent.vue'
import ProbesEdit from '@/views/agent/ProbesEdit.vue'
import DeleteAgent from '@/views/agent/DeleteAgent.vue'
import Speedtests from '@/views/agent/Speedtests.vue'
import NewSpeedtest from '@/views/agent/NewSpeedtest.vue'
import AgentView from "@/views/agent/AgentView.vue";
import BasicView from "@/views/BasicView.vue";
import Profile from "@/views/profile/Profile.vue"

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
    ...asArray(auth),

    // Home -> Workspaces list
    { path: '/', name: 'root', redirect: '/workspaces', component: RootView, children: [

    // ----- /workspaces (list + create) -----
    { path: '/workspaces', name: 'workspaces', component: Workspaces },
    { path: '/workspaces/new', name: 'workspaceNew', component: NewWorkspace },

    // ----- /workspaces/:wID (shell with children) -----
    {
        path: '/workspace/:wID(\\d+)',
        props: true,
        children: [
            // Dashboard at /workspaces/:wID
            {
                path: '',
                name: 'workspace',
                component: WorkspaceDashboard,
                props: true,
            },

            // Edit at /workspaces/:wID/edit
            {
                path: 'edit',
                name: 'workspaceEdit',
                component: EditWorkspace,
                props: true,
            },

            // ----- Members: /workspaces/:wID/members[...] -----
            {
                path: 'members',
                props: true,
                component: BasicView,
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

            // ----- Groups: /workspaces/:wID/groups[...] -----
            /*{
                path: 'groups',
                component: Shell,
                props: true,
                children: [
                    { path: '', name: 'workspaceGroups', component: AgentGroups, props: true },
                    { path: 'new', name: 'workspaceGroupsNew', component: NewAgentGroup, props: true },
                ],
            },*/
            // ----- Agents: /workspaces/:wID/agents[...] -----
            {
                path: 'agents',
                props: true,
                component: BasicView,
                children: [
                    // /workspaces/:wID/agents/new
                    { path: 'new', name: 'agentNew', component: NewAgent, props: true },
                    // /workspaces/:wID/agents/:aID (detail)
                    { path: ':aID(\\d+)', name: 'agent', component: Agent, props: true },

                    // /workspaces/:wID/agents/:aID/edit
                    { path: ':aID(\\d+)/edit', name: 'agentEdit', component: EditAgent, props: true },

                    // /workspaces/:wID/agents/:aID/deactivate
                    {
                        path: ':aID(\\d+)/deactivate',
                        name: 'agentDeactivate',
                        component: DeactivateAgent,
                        props: true,
                    },

                    // /workspaces/:wID/agents/:aID/probes
                    {
                        path: ':aID(\\d+)/probes',
                        name: 'agentProbesEdit',
                        component: ProbesEdit,
                        props: true,
                    },

                    // /workspaces/:wID/agents/:aID/delete
                    {
                        path: ':aID(\\d+)/delete',
                        name: 'agentDelete',
                        component: DeleteAgent,
                        props: true,
                    },

                    // /workspaces/:wID/agents/:aID/speedtests[...]
                    {
                        path: ':aID(\\d+)/speedtests',
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

            /*{
                path: '/profile',
                name: 'profile',
                component: Profile,
            },*/


            // 404
    { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFound }
        ]},
]

const router = createRouter({
    history: createWebHistory(import.meta.env.BASE_URL),
    routes,
})

export default router