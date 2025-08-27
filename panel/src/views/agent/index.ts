import NewAgent from "@/views/agent/NewAgent.vue";
// @ts-ignore
import Agent from "@/views/agent/Agent.vue";
// @ts-ignore
import AgentView from "@/views/agent/AgentView.vue";
import DeactivateAgent from "@/views/agent/DeactivateAgent.vue";
import EditAgent from "@/views/agent/EditAgent.vue";
import ProbesEdit from "@/views/agent/ProbesEdit.vue";
import DeleteAgent from "@/views/agent/DeleteAgent.vue";
import Speedtests from "@/views/agent/Speedtests.vue";
import NewSpeedtest from "@/views/agent/NewSpeedtest.vue";

export default {
    path: '/agent_view',
    name: 'agentView',
    component: AgentView,
    children: [
        {
            path: '/agent/:idParam',
            name: 'agent',
            component: Agent,
        },
        {
            path: '/agent/:idParam/new',
            name: 'agentNew',
            component: NewAgent,
        },
        {
            path: '/agent/:idParam/delete',
            name: 'deleteAgent',
            component: DeleteAgent,
        },
        {
            path: '/agent/:idParam/edit',
            name: 'agentEdit',
            component: EditAgent,
        },
        {
            path: '/agent/:idParam/probes',
            name: 'editProbes',
            component: ProbesEdit,
        },
        {
            path: '/agent/:idParam/deactivate',
            name: 'deactivateAgent',
            component: DeactivateAgent,
        },
        {
            path: '/agent/:idParam/speedtests',
            name: 'speedTests',
            component: Speedtests,
        },
        {
            path: '/agent/:idParam/speedtest/new',
            name: 'newSpeedTest',
            component: NewSpeedtest,
        },
    ]
}