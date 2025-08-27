<script lang="ts" setup>

import type {AgentGroup, Site} from "@/types";
import {onMounted, reactive} from "vue";
import siteService from "@/services/siteService";
import Title from "@/components/Title.vue";
import core from "@/core";
import {Agent} from "@/types";

let state = reactive({
  groups: [] as AgentGroup[],
  site: {} as Site,
  ready: false
})

onMounted(() => {
  let id = router.currentRoute.value.params["idParam"] as string
  if (!id) return

  siteService.getSite(id).then(res => {
    state.site = res.data as Site
  })

  siteService.getAgentGroups(id).then(res => {
    if(res.data.length > 0) {
      state.groups = res.data as AgentGroup[]
      state.ready = true
    }
  }).catch(res => {
    //alert(res)
  })
})
const router = core.router()

</script>

<template>
  <div class="container-fluid">
    <Title title="Agent Groups" subtitle="agent groups associated with current site" :history="[{title: 'workspaces', link: '/sites'}, {title: state.site.name, link: `/sites/${state.site.id}`}]">
      <router-link :to="`/workspace/${state.site.id}/groups/new`" active-class="active" class="btn btn-primary"><i class="fa-solid fa-plus"></i>&nbsp;Create</router-link>

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
                <th class="px-0" scope="col">description</th>
                <th class="px-0 text-end" scope="col">edit</th>
              </tr>
              </thead>
              <tbody>
              <tr v-for="group in state.groups">
                <td class="px-0">
                  <router-link :to="`/sites/${group.id}`" class="">
                    {{group.name}}
                  </router-link>

                </td>
                <td class="px-0">
                  {{ group.description }}
                </td>
                <!--                  <td class="px-0">
                                    <span class="badge bg-dark">{{ site. }}</span>
                                  </td>-->
                <td class="px-0 text-end px-3">
                  <router-link :to="`/sites/${group.site}/groups`" class="">
                    <i class="fa-solid fa-up-right-from-square"></i> edit
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
          <h1 class="error-title text-danger">no agent groups</h1>
          <h3 class="text-error-subtitle">please create a new group</h3>
          <!-- <p class="text-muted m-t-30 m-b-30">YOU SEEM TO BE TRYING TO FIND HIS WAY HOME</p>
           <a href="/" class="btn btn-danger btn-rounded waves-effect waves-light m-b-40 text-white">Back to home</a>-->
        </div>
      </div>
    </div>

  </div>
</template>

<style>

</style>