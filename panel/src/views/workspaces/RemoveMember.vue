<script lang="ts" setup>

import {onMounted, reactive} from "vue";
import type {MemberInfo, Workspace} from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import {Agent} from "@/types";
import agentService from "@/services/agentService";
import {WorkspaceService} from "@/services/apiService";

const state = reactive({
  site: {} as Workspace,
  ready: false,
  agent: {} as Agent,
  memberInfo: {} as MemberInfo
})

onMounted(() => {
  let id = router.currentRoute.value.params["wID"] as string
  if (!id) return

  let userId = router.currentRoute.value.params["userId"] as string
  if (!userId) return

  WorkspaceService.get(id).then(res => {
      state.site = res as Workspace
      /*state.ready = true*/

    Workspace.getMemberInfos(id).then(res => {
      if(res.data.length > 0) {
        const members = res.data as MemberInfo[];
        state.ready = true

        for (let i = 0; i < members.length; i++) {
          if (members[i].id == userId) {
            state.memberInfo = members[i]
            break
          }
        }
      }
    }).catch(res => {
      alert(res)
    })
  })
})
const router = core.router()

function onCreate(response: any) {
  router.push(`/workspace/${state.site.id.toString()}`)
}

function onError(response: any) {
  alert(response)
}

function submit() {
  siteService.removeMember(state.site.id, state.memberInfo).then((res) => {
    router.push(`/workspace/${state.site.id}/members`)
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
    <Title :title="`remove member`"
           :history="[{ title: 'workspaces', link: '/workspaces' }, { title: state.site.name, link: `/workspace/${state.site.id}` },{ title: `members`, link: `/workspace/${state.site.id}/members` }]">
    </Title>
    <div class="row">
      <div class="col-12">
        <div class="card">
          <div class="form-horizontal r-separator border-top">
            <div class="card-body">
              <div class="form-group row align-items-center mb-0">
                <label class="col-3 text-end control-label col-form-label">confirm member removal</label>
                <div class="col-9 border-start pb-2 pt-2">
                  <p>are you sure you want to remove the member <strong>{{ state.memberInfo.email }}</strong>?</p>
                </div>
              </div>
            </div>
            <div class="p-3 border-top">
              <div class="form-group mb-0 text-end">
                <button class="btn btn-secondary px-4" @click="cancel">cancel</button>
                <button style="margin-left: 20px" class="btn btn-danger px-4" @click="submit">remove</button>
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