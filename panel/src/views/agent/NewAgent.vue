<script lang="ts" setup>

import {onMounted, reactive} from "vue";
import siteService from "@/services/workspaceService";
import type {Site} from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import {Agent} from "@/types";
import agentService from "@/services/agentService";

const state = reactive({
  site: {} as Site,
  ready: false,
  agent: {} as Agent
})

onMounted(() => {
  let id = router.currentRoute.value.params["idParam"] as string
  if (!id) return

  siteService.getSite(id).then(res => {
    state.site = res.data as Site
    state.agent.site = state.site.id
    state.ready = true
  })
})
const router = core.router()

function onCreate(response: any) {
  router.push("/sites")
}

function onError(response: any) {
  alert(response)
}

function submit() {
  agentService.createAgent(state.agent).then((res) => {
    router.push(`/workspace/${state.site.id}`)
    console.log(res)
  }).catch(err => {
    console.log(err)
  })
}

</script>

<template>
  <div class="container-fluid" v-if="state.ready">
    <Title title="Add Agent" subtitle="create a new agent" :history="[{title: 'workspaces', link: '/workspaces'}, {title: state.site.name, link: `/workspace/${state.site.id}`}]"></Title>
    <div class="row">
      <div class="col-12">
        <div class="card">
          <div class="form-horizontal r-separator border-top">
            <div class="card-body">
              <div class="form-group row align-items-center mb-0">
                <label class="col-3 text-end control-label col-form-label" for="agentName">agent name</label>
                <div class="col-9 border-start pb-2 pt-2">
                  <input id="agentName" class="form-control" name="name" v-model="state.agent.name" placeholder="name" type="text">
                  <br>
                  <input id="agentLocation" class="form-control" name="name" v-model="state.agent.location" placeholder="location" type="text">
                </div>
              </div>
            </div>
            <div class="p-3 border-top">
              <div class="form-group mb-0 text-end">
                <button class="
                          btn btn-primary px-4" type="submit" @click="submit">
                  Create Agent
                </button>

              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style>

</style>