<script lang="ts" setup>

import {onMounted, reactive} from "vue";
import type {Member, Role, Workspace} from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import {WorkspaceService} from "@/services/apiService";

const state = reactive({
  name: "",
  site: {} as Workspace,
  email: "",
  role: "" as Role
})

const router = core.router()

onMounted(() => {
  let id = router.currentRoute.value.params["wID"] as string
  if (!id) return

  WorkspaceService.get(id).then(res => {
    state.site = res as Workspace
  })

})

function onCreate(response: any) {
  router.push("/workspaces/" + state.site.id + "/members")
}

function onError(response: any) {
  alert(response)
}

function submit() {

  //todo

  WorkspaceService.addMember(state.site.id, state.email, state.role).then(onCreate).catch(onError)
}

</script>

<template>
  <div class="container-fluid">
  <Title title="invite member" :subtitle="`Invite a member to the site '${state.site.name}'`" :history="[{title: 'workspaces', link: '/workspaces'}, {title: state.site.name, link: `/workspaces/${state.site.id}`}, {title: `members`, link: `/workspaces/${state.site.id}/members`}]"></Title>
    <div class="row">
      <div class="col-12">
        <div class="card">
          <div class="form-horizontal">
            <div class="card-body">
              <div class="row">
              <div class="mb-3 col-lg-8 col-12">
                <label for="memberEmail" class="form-label">Email address</label>
                <input type="email" v-model="state.email" class="form-control" id="memberEmail" aria-describedby="memberEmail" placeholder="example@netwatcher.io">
                <div id="memberEmail" class="form-text">If the email belongs to a user with a netwatcher account, they will be added to the workspace. If they do not have an account, they will be invited to create one.</div>
              </div>
              <div class="mb-3 col-lg-4 col-12">
                <label for="memberEmail" class="form-label">Member Permissions</label>
                <select class="form-select" v-model="state.role" aria-label="Default select example">
                  <option value="USER" selected>User (no delete)</option>
                  <option value="ADMIN">Admin (delete)</option>
                </select>
                <div id="memberEmail" class="form-text">Members with full access can permanently change aspects of the workspace.</div>
              </div>
              </div>
            </div>
            <div class="p-3">
              <div class="form-group mb-0 text-end">
                <button class="
                         btn btn-primary px-4" type="submit" @click="submit">
                  invite
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