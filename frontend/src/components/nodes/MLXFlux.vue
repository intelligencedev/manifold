<template>
  <BaseNode
    :id="id"
    :data="data"
    :min-height="612"
    @resize="onResize"
  >
    <template #header>
      <div :style="data.labelStyle" class="node-label">
        {{ data.type }}
      </div>
    </template>

    <BaseAccordion title="Parameters" :initiallyOpen="true">
      <div class="flex flex-col gap-2">
        <BaseInput :id="`${data.id}-model`" label="Model" v-model="model" />
        <BaseTextarea :id="`${data.id}-prompt`" label="Prompt" v-model="prompt" />
        <div class="flex items-center gap-4">
          <BaseInput class="flex-1" :id="`${data.id}-steps`" label="Steps" v-model.number="steps" type="number" min="1" />
          <BaseInput class="flex-1" :id="`${data.id}-seed`" label="Seed" v-model.number="seed" type="number" />
        </div>
        <div class="flex items-center gap-2">
          <span class="input-label text-xs">Quality:</span>
          <label class="radio-label flex items-center gap-1">
            <input type="radio" :name="`${data.id}-quality`" :value="4" v-model.number="quality" /> 4bit
          </label>
          <label class="radio-label flex items-center gap-1">
            <input type="radio" :name="`${data.id}-quality`" :value="8" v-model.number="quality" /> 8bit
          </label>
        </div>
        <BaseInput :id="`${data.id}-output`" label="File Name" v-model="output" />
      </div>
    </BaseAccordion>

    <div v-if="imageSrc" class="flex-1 flex items-center justify-center mt-2">
      <img :src="imageSrc" alt="Generated Image" class="max-w-full max-h-48 rounded border border-gray-700" />
    </div>

    <Handle v-if="data.hasInputs" type="target" position="left" style="width:12px;height:12px" />
    <Handle v-if="data.hasOutputs" type="source" position="right" style="width:12px;height:12px" />
  </BaseNode>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import BaseNode from '@/components/base/BaseNode.vue'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import BaseAccordion from '@/components/base/BaseAccordion.vue'
import { useMLXNode } from '@/composables/useMLXNode'

const props = defineProps({
  id: { type: String, required: true, default: 'MLXNode_0' },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'MLXNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        model: '',
        prompt: '',
        steps: 20,
        seed: 0,
        quality: 8,
        output: ''
      },
      outputs: {},
    })
  }
})

const emit = defineEmits(['update:data', 'resize'])
const {
  model,
  prompt,
  steps,
  seed,
  quality,
  output,
  imageSrc,
  onResize
} = useMLXNode(props, emit)
</script>
