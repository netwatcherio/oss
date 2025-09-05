<script lang="ts" setup>

import {reactive} from "vue";
import type {Workspace} from "@/types";
import core from "@/core";
import Title from "@/components/Title.vue";

const state = reactive({
  name: "",
  description: "",
  location: ""
})

const router = core.router()

function onCreate(response: any) {
  router.push("/workspace")
}

function onError(response: any) {
  alert(response)
}

function submit() {
  siteService.createSite({
    name: state.name,
    description: state.description,
    location: state.location
  } as Workspace).then(onCreate).catch(onError)
}

</script>

<template>
  <div class="container-fluid">
  <Title title="new workspace" subtitle="create a new site" :history="[{title: 'workspaces', link: '/workspaces'}]"></Title>
    <div class="row">
      <div class="col-12">
        <div class="card">
          <div class="form-horizontal r-separator border-top">
            <div class="card-body">
              <div class="form-group row align-items-center mb-0">
                <label class="col-3 text-end control-label col-form-label" for="siteName">new workspace</label>
                <div class="col-9 border-start pb-2 pt-2">
                  <input id="siteName" class="form-control" name="name" v-model="state.name" placeholder="name" type="text">
                  <br>
                  <input id="siteDesc" class="form-control" name="desc" v-model="state.description" placeholder="description" type="text">
                  <br>
                  <input id="siteLocation" class="form-control" name="name" v-model="state.location" placeholder="location" type="text">
                </div>
              </div>
            </div>
            <div class="p-3 border-top">
              <div class="form-group mb-0 text-end">
                <button class="
                          btn btn-primary px-4" type="submit" @click="submit">
                  create
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