<script lang="ts" setup>

import {useRouter} from "vue-router";
import {useSession} from "@/session";

const router = useRouter()
let session = useSession()

</script>

<template>
  <RouterView></RouterView>
</template>

<style>

</style>
