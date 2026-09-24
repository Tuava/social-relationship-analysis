<template>
  <div class="login-shell">
    <v-card class="login-card pa-8" max-width="420">
      <h1 class="text-h5 mb-6">社会关系分析</h1>
      <v-form @submit.prevent="submit">
        <v-text-field
          v-model="username"
          label="用户名"
          prepend-inner-icon="mdi-account-outline"
          autocomplete="username"
        />
        <v-text-field
          v-model="password"
          label="密码"
          type="password"
          prepend-inner-icon="mdi-lock-outline"
          autocomplete="current-password"
          class="mt-3"
        />
        <v-alert
          v-if="error"
          type="error"
          variant="tonal"
          density="compact"
          class="mt-4"
          >{{ error }}</v-alert
        >
        <v-btn
          type="submit"
          color="primary"
          block
          size="large"
          class="mt-6"
          :loading="session.loading"
          >登录</v-btn
        >
      </v-form>
    </v-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { useRouter } from "vue-router";
import { useSessionStore } from "@/stores/session";
const router = useRouter();
const session = useSessionStore();
const username = ref("admin");
const password = ref("");
const error = ref("");
async function submit() {
  error.value = "";
  try {
    await session.login(username.value, password.value);
    await router.push("/");
  } catch (e: any) {
    error.value = e.response?.data?.error || "登录失败，请检查服务状态";
  }
}
</script>
