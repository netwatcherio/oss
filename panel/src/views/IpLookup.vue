<template>
  <div class="lookup-page">
    <div class="page-header">
      <h1><i class="bi bi-search"></i> IP Lookup</h1>
      <p class="subtitle">Search IP addresses for geolocation and WHOIS information</p>
    </div>
    
    <IpLookupPanel 
      ref="lookupPanel"
      :initial-ip="initialIp"
      @lookup="onLookup"
    />
  </div>
</template>

<script lang="ts" setup>
import { ref, onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import IpLookupPanel from '@/components/IpLookupPanel.vue';

const route = useRoute();
const router = useRouter();
const lookupPanel = ref<InstanceType<typeof IpLookupPanel>>();

// Get initial IP from URL query param
const initialIp = ref((route.query.ip as string) || '');

function onLookup(ip: string) {
  // Update URL with searched IP
  router.replace({ query: { ip } });
}

// Watch for route changes (e.g., navigating with different IP)
onMounted(() => {
  if (route.query.ip && lookupPanel.value) {
    lookupPanel.value.searchQuery = route.query.ip as string;
  }
});
</script>

<style scoped>
.lookup-page {
  max-width: 900px;
  margin: 0 auto;
  padding: 2rem;
}

.page-header {
  margin-bottom: 1.5rem;
}

.page-header h1 {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  color: #c0caf5;
  font-size: 1.75rem;
  font-weight: 600;
  margin: 0 0 0.5rem 0;
}

.page-header h1 i {
  color: #7aa2f7;
}

.subtitle {
  color: #565f89;
  font-size: 0.95rem;
  margin: 0;
}
</style>
