<script lang="ts" setup>

import type {AgentGroup, MemberInfo, Workspace, WorkspaceMember} from "@/types";
import {onMounted, reactive} from "vue";
import siteService from "@/services/workspaceService";
import Title from "@/components/Title.vue";
import core from "@/core";
import {Agent} from "@/types";

let state = reactive({
  members: [] as MemberInfo[],
  site: {} as Workspace,
  ready: false
})

onMounted(() => {
  let id = router.currentRoute.value.params["siteId"] as string
  if (!id) return

  siteService.getSite(id).then(res => {
    state.site = res.data as Workspace
  })

  siteService.getMemberInfos(id).then(res => {
    if(res.data.length > 0) {
      state.members = res.data as MemberInfo[]
      state.ready = true
    }

  }).catch(res => {
    alert(res)
  })
})
const router = core.router()

</script>

<template>
  <div class="container-fluid">
    <Title title="members" subtitle="agent groups associated with current site" :history="[{title: 'workspaces', link: '/workspaces'}, {title: state.site.name, link: `/workspace/${state.site.id}`}]">
      <router-link :to="`/workspace/${state.site.id}/invite`" active-class="active" class="btn btn-primary"><i class="fa-solid fa-plus"></i>&nbsp;invite</router-link>

      <!--      <div class="d-flex gap-1">
          <router-link to="/sites/alerts" active-class="active" class="btn btn-outline-primary"><i class="fa-solid fa-plus"></i>&nbsp;View Alerts</router-link>
          <router-link to="/sites/new" active-class="active" class="btn btn-primary"><i class="fa-solid fa-plus"></i>&nbsp;Create</router-link>
        </div>-->
    </Title>
    <div v-if="state.ready" class="row">
      <!-- column -->
      <div class="col-12">
        <div class="d-md-flex px-2">
          <span class="card-subtitle text-muted"></span>
        </div>
        <div class="card px-3 py-1">
          <!-- title -->
          <div class="table-responsive">
            <table class="table">
              <thead>
              <tr>
                <th class="px-0" scope="col">name</th>
                <th class="px-0" scope="col">email</th>
                <th class="px-0" scope="col">role</th>
                <th class="px-0" scope="col"> </th>
                <th class="px-0 text-end" scope="col"> </th>
              </tr>
              </thead>
              <tbody>
              <tr v-for="group in state.members">
                <td class="px-0">
                  {{group.firstName + " " + group.lastName}}
                </td>
                <td class="px-0">
                  {{ group.email }}
                </td>
                <td class="px-0">
                  {{ group.role }}
                </td>

                <td class="px-0 text-end px-3">
                  <router-link :to="`/workspace/${state.site.id}/members/edit/${group.id}`" class="">
                    <i class="fa-solid fa-up-right-from-square"></i> edit
                  </router-link>
                </td>
                <td class="px-0 text-end px-3">
                  <router-link :to="`/workspace/${state.site.id}/members/remove/${group.id}`" class="">
                    <i class="fa-solid fa-up-right-from-square"></i> remove
                  </router-link>
                </td>
              </tr>
              </tbody>
            </table>
          </div>

        </div>
      </div>
    </div>
    <div v-else class="row">
      <div class="col-lg-12">
        <div class="error-body text-center">
          <h1 class="error-title text-danger">loading...</h1>
          <h3 class="text-error-subtitle">please wait...</h3>
          <!-- <p class="text-muted m-t-30 m-b-30">YOU SEEM TO BE TRYING TO FIND HIS WAY HOME</p>
           <a href="/" class="btn btn-danger btn-rounded waves-effect waves-light m-b-40 text-white">Back to home</a>-->
        </div>
      </div>
    </div>

  </div>
</template>

<style>

</style>