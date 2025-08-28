<script lang="ts" setup>

import {onMounted, reactive} from "vue";
import siteService from "@/services/workspaceService";
import type {Workspace} from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import {Agent} from "@/types";
import agentService from "@/services/agentService";

const state = reactive({
  site: {} as Workspace,
  ready: false,
  agent: {} as Agent
})

onMounted(() => {
  let id = router.currentRoute.value.params["idParam"] as string
  if (!id) return

  agentService.getAgent(id).then(res => {
    state.agent = res.data as Agent
    siteService.getSite(state.agent.site).then(res => {
      state.site = res.data as Workspace
      state.ready = true
    })
  })
})
const router = core.router()

function onCreate(response: any) {
  router.push("/workspaces")
}

function onError(response: any) {
  alert(response)
}

function submit() {
  agentService.deleteAgent(state.agent.id).then((res) => {
    router.push(`/workspace/${state.site.id}`)
    console.log(res)
  }).catch(err => {
    console.log(err)
  })
}

function cancel() {
  router.push(`/workspace/${state.site.id}`)
}

</script>

<template>
  <div class="container-fluid" v-if="state.ready">
    <Title :title="`delete agent`"
           :history="[{ title: 'workspaces', link: '/workspaces' }, { title: state.site.name, link: `/workspace/${state.site.id}` },{ title: `edit agent`, link: `/agent/${state.agent.id}/edit` }]">
    </Title>
    <div class="row">
      <div class="col-12">
        <div class="card">
          <div class="form-horizontal r-separator border-top">
            <div class="card-body">
              <div class="form-group row align-items-center mb-0">
                <label class="col-3 text-end control-label col-form-label">confirm deletion</label>
                <div class="col-9 border-start pb-2 pt-2">
                  <p>are you sure you want to delete the agent <strong>{{ state.agent.name }}</strong>?</p>
                </div>
              </div>
            </div>
            <div class="p-3 border-top">
              <div class="form-group mb-0 text-end">
                <button class="btn btn-secondary px-4" @click="cancel">cancel</button>
                <button style="margin-left: 20px" class="btn btn-danger px-4" @click="submit">delete</button>
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