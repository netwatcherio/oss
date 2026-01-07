<script lang="ts" setup>

import core from "@/core";
import type {User} from "@/types";

const persistent = core.persistent()
const router = core.router()
const session = core.session()

function logout() {
  session.token = ""
  router.push("/auth/login")
}

</script>

<template>
  <div class="d-flex flex-column flex-shrink-0 p-3 bg-light h-100 sidebar" style="width: 280px;">
    <a class="d-flex align-items-center logo navbar-brand gap-2 px-1 mb-3" href="#">
      <img src="/assets/images/logo-icon.png" alt="Logo" class="d-inline-block align-text-top">
      netwatcher
    </a>
    <ul class="nav nav-pills flex-column gap-1">
      <li class="nav-item">
        <router-link active-class="active" class="nav-link" to="/home">
          <div class="sidebar-link waves-effect waves-dark"><i class="bi bi-house"></i>&nbsp;&nbsp;home
          </div>
        </router-link>
      </li>
      <li class="nav-item">
        <router-link active-class="active" class="nav-link" to="/workspaces">
          <div class="sidebar-link waves-effect waves-dark"><i class="bi bi-grid"></i>&nbsp;&nbsp;workspaces
          </div>
        </router-link>
      </li>
    </ul>
    <div class="flex-fill"></div>
    <ul class="nav nav-pills flex-column mb-2">
      <li class="nav-item">
        <div class="nav-link nav-link-static"  @click="logout">
          <div><i class="bi bi-box-arrow-right"></i>&nbsp;logout
          </div>
        </div>
      </li>
    </ul>
  </div>
</template>

<style>

</style>