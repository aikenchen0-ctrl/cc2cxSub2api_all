<template>
  <div :class="embedded ? 'space-y-4' : 'card'">
    <div
      v-if="!embedded"
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        编辑资料
      </h2>
    </div>
    <div :class="embedded ? '' : 'px-6 py-6'">
      <form novalidate @submit.prevent="$emit('submit')" class="space-y-4">
        <div v-if="embedded">
          <p class="text-sm font-semibold text-gray-900 dark:text-white">
            编辑资料
          </p>
        </div>
        <div>
          <label for="username" class="input-label">
            用户名
          </label>
          <AgentInput
            id="username"
            v-model="username" name="username" maxlength="128" autocomplete="nickname" required :disabled="disabled || loading"
            type="text"
            placeholder="请输入用户名"
          />
        </div>

        <div class="flex justify-end pt-4">
          <button type="submit" :disabled="disabled || loading || !username.trim() || username.trim() === initialUsername" class="btn btn-primary">
            {{ loading ? '正在保存…' : '保存资料' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import AgentInput from '@/components/common/AgentInput.vue'

// Template copied from Sub2API ProfileEditForm; the parent enforces session permissions.
withDefaults(defineProps<{ initialUsername: string; embedded?: boolean; loading?: boolean; disabled?: boolean }>(), { embedded: false, loading: false, disabled: false })
defineEmits<{ submit: [] }>()
const username = defineModel<string>({ required: true })
</script>
