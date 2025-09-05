<script lang="ts" setup>

import {useRouter} from "vue-router";
import {getSession} from "@/session";

const router = useRouter()
let session = getSession()

</script>

<template>
  <RouterView></RouterView>
</template>

<style>

</style>
