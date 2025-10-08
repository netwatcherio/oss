<script lang="ts" setup>

import {onMounted, reactive} from "vue";
import type {Workspace} from "@/types";
import core from "@/core";
import {Agent} from "@/types";
import Title from "@/components/Title.vue";
import {AgentService, WorkspaceService} from "@/services/apiService";

const state = reactive({
  workspace: {} as Workspace,
  ready: false,
  agent: {} as Agent
})

onMounted(() => {
  let id = router.currentRoute.value.params["aID"] as string
  if (!id) return

  let wID = router.currentRoute.value.params["wID"] as string
  if (!id) return

  WorkspaceService.get(wID).then(res => {
    state.workspace = res as Workspace
    state.ready = true
  })

  AgentService.get(wID, id).then(res => {
    state.agent = res as Agent
    console.log(state.agent)
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
  /*AgentService.deleteAgent(state.agent.id).then((res) => {
    router.push(`/workspace/${state.workspace.id}`)
    console.log(res)
  }).catch(err => {
    console.log(err)
  })*/
}

function cancel() {
  router.push(`/workspaces/${state.workspace.id}`)
}

</script>

<template>
  <div class="container-fluid" v-if="state.ready">
    <Title :title="`delete agent`"
           :history="[{ title: 'workspace', link: '/workspaces' }, { title: state.workspace.name, link: `/workspaces/${state.workspace.id}` },{ title: `edit agent`, link: `/workspaces/${state.workspace.id}/agents/${state.agent.id}/edit` }]">
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