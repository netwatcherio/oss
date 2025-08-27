
import SiteView from "@/views/site/Index.vue";
// @ts-ignore
import Site from "@/views/site/WorkspaceDashboard.vue";
import Sites from "@/views/site/Sites.vue";
import SiteNew from "@/views/site/NewWorkspace.vue";
import Invite from "@/views/site/InviteMember.vue";
import AgentGroups from "@/views/site/AgentGroups.vue";
import AgentGroupsNew from "@/views/site/NewAgentGroup.vue";
import EditSite from "@/views/site/EditSite.vue";
import Members from "@/views/site/Members.vue";
import RemoveMember from "@/views/site/RemoveMember.vue";
import EditMember from "@/views/site/EditMember.vue";

export default {
  path: '/workspace_view',
  name: 'siteView',
  component: SiteView,
  children: [
    {
      path: '/workspace/:siteId',
      name: 'site',
      component: Site,
    },
    {
      path: '/workspaces',
      name: 'sites',
      component: Sites,
    },
    {
      path: '/workspace/new',
      name: 'siteNew',
      component: SiteNew,
    },
    {
      path: '/workspace/:siteId/edit',
      name: 'editSite',
      component: EditSite,
    },
    {
      path: '/workspace/:siteId/invite',
      name: 'siteInvite',
      component: Invite,
    },
    {
      path: '/workspace/:siteId/members/remove/:userId',
      name: 'memberRemove',
      component: RemoveMember,
    },
    {
      path: '/workspace/:siteId/members/edit/:userId',
      name: 'memberEdit',
      component: EditMember,
    },
    {
      path: '/workspace/:siteId/groups',
      name: 'agentGroups',
      component: AgentGroups,
    },
    {
      path: '/site/:siteId/groups/new',
      name: 'agentGroupsNew',
      component: AgentGroupsNew,
    },
    {
      path: '/workspace/:siteId/members',
      name: 'members',
      component: Members,
    }
  ]
}
