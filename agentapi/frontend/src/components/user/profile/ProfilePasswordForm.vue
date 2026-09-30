<template>
  <div :class="embedded ? 'space-y-4' : 'card'">
    <div
      v-if="!embedded"
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        修改密码
      </h2>
    </div>
    <div :class="embedded ? '' : 'px-6 py-6'">
      <form novalidate @submit.prevent="$emit('submit')" class="space-y-4">
        <div v-if="embedded">
          <p class="text-sm font-semibold text-gray-900 dark:text-white">
            修改密码
          </p>
        </div>
        <div>
          <label for="old_password" class="input-label">
            当前密码
          </label>
          <AgentInput
            id="old_password"
            v-model="oldPassword" name="old_password" aria-label="当前密码" :disabled="loading"
            type="password"
            required
            autocomplete="current-password"
          />
        </div>

        <div>
          <label for="new_password" class="input-label">
            新密码
          </label>
          <AgentInput
            id="new_password"
            v-model="newPassword" name="new_password" aria-label="新密码" :disabled="loading" minlength="8"
            type="password"
            required
            autocomplete="new-password"
            hint="至少 8 个字符。"
          />
        </div>

        <div>
          <label for="confirm_password" class="input-label">
            确认新密码
          </label>
          <AgentInput
            id="confirm_password"
            v-model="confirmPassword" name="confirm_password" aria-label="确认新密码" :disabled="loading" minlength="8"
            type="password"
            required
            autocomplete="new-password"
          />
        </div>

        <div class="flex justify-end pt-4">
          <button type="submit" :disabled="loading" class="btn btn-primary">
            {{ loading ? '正在修改…' : '修改密码' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import AgentInput from '@/components/common/AgentInput.vue'

// Template copied from Sub2API ProfilePasswordForm; requests stay in AgentProfileView.
withDefaults(defineProps<{ embedded?: boolean; loading?: boolean }>(), { embedded: false, loading: false })
defineEmits<{ submit: [] }>()
const oldPassword = defineModel<string>('oldPassword', { required: true })
const newPassword = defineModel<string>('newPassword', { required: true })
const confirmPassword = defineModel<string>('confirmPassword', { required: true })
</script>
