<script lang="ts" setup>

import {onMounted, reactive} from "vue";
import siteService from "@/services/workspaceService";
import type {MemberInfo, Site} from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";

const state = reactive({
  name: "",
  site: {} as Site,
  newMember: {} as MemberInfo,
  ready: false
})

const router = core.router()

onMounted(() => {
  let id = router.currentRoute.value.params["siteId"] as string
  if (!id) return

  let mId = router.currentRoute.value.params["userId"] as string
  if (!id) return

  siteService.getSite(id).then(res => {
    state.site = res.data as Site

    siteService.getMemberInfos(id).then(res => {
      if(res.data.length > 0) {
        const members = res.data as MemberInfo[];
        state.ready = true

        for (let i = 0; i < members.length; i++) {
          if (members[i].id == mId) {
            state.newMember = members[i]
            break
          }
        }
      }
    }).catch(res => {
      alert(res)
    })
  })
})

function onCreate(response: any) {
  router.push("/workspace/" + state.site.id + "/members")
}

function onError(response: any) {
  alert(response)
}

function submit() {
  siteService.updateMember(state.site.id, state.newMember).then(onCreate).catch(onError)
}

</script>

<template>
  <div class="container-fluid">
  <Title title="edit member" :subtitle="`edit a member of the site '${state.site.name}'`" :history="[{title: 'workspaces', link: '/workspaces'}, {title: state.site.name, link: `/workspace/${state.site.id}`}, {title: `members`, link: `/workspace/${state.site.id}/members`}]"></Title>
    <div class="row">
      <div class="col-12">
        <div class="card">
          <div class="form-horizontal">
            <div class="card-body">
              <div class="row">
              <div class="mb-3 col-lg-8 col-12">
                <label for="memberEmail" class="form-label">Email address</label>
                <input type="email" disabled v-model="state.newMember.email" class="form-control" id="memberEmail" aria-describedby="memberEmail" placeholder="example@netwatcher.io">
                <div id="memberEmail" class="form-text">If the email belongs to a user with a netwatcher account, they will be added to the workspace. If they do not have an account, they will be invited to create one.</div>
              </div>
              <div class="mb-3 col-lg-4 col-12">
                <label for="memberEmail" class="form-label">Member Permissions</label>
                <select class="form-select" v-model="state.newMember.role" aria-label="Default select example" v-if="state.newMember.role != 'OWNER'">
                  <option value="READONLY" selected>Read Only</option>
                  <option value="READWRITE">Read/Write</option>
                  <option value="ADMIN">Full Access</option>
                </select>
                <select class="form-select" v-model="state.newMember.role" aria-label="Default select example" v-else>
                  <option value="OWNER" selected>Owner</option>
                </select>
                <div id="memberEmail" class="form-text">Members with full access can permanently change aspects of the workspace.</div>
              </div>
              </div>
            </div>
            <div class="p-3">
              <div class="form-group mb-0 text-end">
                <button class="
                         btn btn-primary px-4" type="submit" @click="submit">
                  update
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