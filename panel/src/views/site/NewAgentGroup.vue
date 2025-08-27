<script lang="ts" setup>

import {onMounted, reactive} from "vue";
import siteService from "@/services/siteService";
import agentService from "@/services/agentService"
import type {Agent, AgentGroup, SelectOption, Site} from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";

let state = reactive({
  name: "",
  site: {} as Site,
  agents: [] as Agent[],
  selected: [] as string[], // Array reference
  options: [] as SelectOption[],
  ready: false,
  groupName: "",
  description: ""
})

const router = core.router()

onMounted(() => {
  let id = router.currentRoute.value.params["siteId"] as string
  if (!id) return

  siteService.getSite(id).then(res => {
    state.site = res.data as Site
  })
  agentService.getSiteAgents(id).then(res => {
    state.agents = res.data as Agent[]
  }).then(function (){
    for(let agent of state.agents){
    let newOption = {value: agent.id, text: agent.name + " (" + agent.location + ")"} as SelectOption
    console.log(newOption)
    state.options.push(newOption)
  }
  state.ready = true
  })
})

function onCreate(response: any) {
  router.push("/sites")
}

function onError(response: any) {
  alert(response)
}

function submit() {
  siteService.createAgentGroup(state.site.id, {
    site: state.site.id,
    agents: state.selected,
    name: state.groupName,
    description: state.description
  } as AgentGroup).then(onCreate).catch(onError)
}

</script>

<template>
  <div class="container-fluid">
    <Title title="Create Agent Group" :subtitle="`create a group of agents for the site '${state.site.name}'`" :history="[{title: 'workspaces', link: '/sites'}, {title: state.site.name, link: `/sites/${state.site.id}`}]"></Title>
    <div class="row">
      <div class="col-12">
        <div class="card">
          <div class="form-horizontal">
            <div class="card-body">
              <div v-if="state.ready" class="row">
                <div class="mb-3 col-lg-8 col-12">
                  <label for="agentOptions" class="form-label">Available Agents</label>
                  <select v-model="state.selected" class="form-select" multiple>
                    <option v-for="option in state.options" :value="option.value">
                      {{ option.text }}
                    </option>
                  </select>
                    <div class="mt-3">Selected: <strong>{{ state.selected }}</strong></div>
                  </div>
                <div class="mb-3 col-lg-4 col-12">
                  <label for="groupName" class="form-label">Group Name</label>
                  <input type="text" v-model="state.groupName" class="form-control" id="groupName" aria-describedby="groupName" placeholder="My Group Name">
                  <div id="groupName" class="form-text">The name of the agent group.</div>
                  <label for="groupDescription" class="form-label">Group Description</label>
                  <input type="text" v-model="state.description" class="form-control" id="groupDescription" aria-describedby="groupDescription" placeholder="My Group Description">
                  <div id="groupDescription" class="form-text">The name of the agent group.</div>
                </div>
              </div>
            </div>
            <div class="p-3">
              <div class="form-group mb-0 text-end">
                <button class="
                         btn btn-primary px-4" type="submit" @click="submit">
                  Create Group
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