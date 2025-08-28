<script lang="ts" setup>
import { onMounted, reactive } from "vue";
import siteService from "@/services/workspaceService";
import type { Workspace } from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";

const router = core.router();
const state = reactive({
  site: {} as Workspace,
  ready: false
});

// Extract the siteId from the route parameters
const siteId = router.currentRoute.value.params.siteId as string;

onMounted(() => {
  if (!siteId) return;

  siteService.getSite(siteId).then(res => {
    state.site = res.data as Workspace
    console.log(state.site)
    state.ready = true
  })
});

function onError(error: any) {
  alert(error);
}

function submit() {
  if (state.site.id) {
    // Call the updateSite method from the siteService
    siteService.updateSite(state.site).then(() => {
      router.push(`/workspace/${state.site.id}`);
    }).catch(onError);
  }
}
</script>

<template>
  <div class="container-fluid" v-if="state.ready">
    <Title title="edit workspace"
           subtitle="update site details"
           :history="[{ title: 'workspaces', link: '/workspaces' }, { title: state.site.name, link: `/workspace/${state.site.id}` }]">
    </Title>
    <div class="row">
      <div class="col-12">
        <div class="card">
          <div class="form-horizontal r-separator border-top">
            <div class="card-body">
              <div class="form-group row align-items-center mb-0">
                <label class="col-3 text-end control-label col-form-label" for="siteName">workspace</label>
                <div class="col-9 border-start pb-2 pt-2">
                  <input id="siteName" class="form-control" name="name" v-model="state.site.name" placeholder="workspace name" type="text">
                  <br>
                  <input id="siteDesc" class="form-control" name="desc" v-model="state.site.description" placeholder="workspace description" type="text">
                  <br>
                  <input id="siteLocation" class="form-control" name="location" v-model="state.site.location" placeholder="workspace location" type="text">
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
