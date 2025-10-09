<script lang="ts" setup>
import { onMounted, reactive } from "vue";
import type { Workspace } from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";
import {WorkspaceService} from "@/services/apiService";

const router = core.router();
const state = reactive({
  workspace: {} as Workspace,
  ready: false
});

// Extract the siteId from the route parameters

onMounted(() => {
  let wID = router.currentRoute.value.params["wID"] as string
  if (!wID) return

  WorkspaceService.get(wID).then(res => {
    state.workspace = res as Workspace
    console.log(state.workspace)
    state.ready = true
  })
});

function onError(error: any) {
  alert(error);
}

function submit() {
  if (state.workspace.id) {
    // Call the updateSite method from the siteService
    WorkspaceService.update(state.workspace.id, state.workspace).then(() => {
      router.push(`/workspaces/${state.workspace.id}`);
    }).catch(onError);
  }
}
</script>

<template>
  <div class="container-fluid" v-if="state.ready">
    <Title title="edit workspace"
           subtitle="update site details"
           :history="[{ title: 'workspace', link: '/workspaces' }, { title: state.workspace.name, link: `/workspaces/${state.workspace.id}` }]">
    </Title>
    <div class="row">
      <div class="col-12">
        <div class="card">
          <div class="form-horizontal r-separator border-top">
            <div class="card-body">
              <div class="form-group row align-items-center mb-0">
                <label class="col-3 text-end control-label col-form-label" for="siteName">workspace</label>
                <div class="col-9 border-start pb-2 pt-2">
                  <input id="siteName" class="form-control" name="name" v-model="state.workspace.name" placeholder="workspace name" type="text">
                  <br>
                  <input id="siteDesc" class="form-control" name="desc" v-model="state.workspace.description" placeholder="workspace description" type="text">
                  <br>
                  <input id="siteLocation" class="form-control" name="location" v-model="state.workspace.location" placeholder="workspace location" type="text">
                </div>
              </div>
            </div>
            <div class="p-3 border-top">
              <div class="form-group mb-0 text-end">
                <button class="btn btn-primary px-4" type="button" @click="submit">
                  Update Site
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
