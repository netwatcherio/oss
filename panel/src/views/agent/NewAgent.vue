<script lang="ts" setup>

import {onMounted, reactive} from "vue";
import type {Workspace} from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import {Agent} from "@/types";
import {AgentService, WorkspaceService} from "@/services/apiService";

const state = reactive({
  workspace: {} as Workspace,
  ready: false,
  agent: {} as Agent
})

onMounted(() => {
  let id = router.currentRoute.value.params["wID"] as string
  if (!id) return

  WorkspaceService.get(id).then(res => {
    state.workspace = res as Workspace
    state.agent.workspace_id = state.workspace.id
    state.ready = true
  })
})
const router = core.router()

function onCreate(response: any) {
  router.push(`/workspace/${state.workspace.id}`)
}

function onError(response: any) {
  alert(response)
}

function submit() {
  AgentService.create(state.workspace.id, state.agent).then((res) => {
    router.push(`/workspaces/${state.workspace.id}`)
    console.log(res)
  }).catch(err => {
    console.log(err)
  })
}

</script>

<template>
  <div class="container-fluid" v-if="state.ready">
    <Title title="Add Agent" subtitle="create a new agent" :history="[{title: 'workspaces', link: '/workspaces'}, {title: state.workspace.name, link: `/workspaces/${state.workspace.id}`}]"></Title>
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