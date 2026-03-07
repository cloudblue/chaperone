<template>
	<AppLayout>
		<RouterView />
	</AppLayout>
</template>

<script setup>
import { RouterView } from "vue-router";
import AppLayout from "./layouts/AppLayout.vue";
</script>
