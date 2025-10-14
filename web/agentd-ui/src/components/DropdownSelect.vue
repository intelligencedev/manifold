<template>
  <div class="dropdown-select-wrapper" :class="wrapperClass">
    <label v-if="$slots.label || ariaLabel" class="sr-only" :for="id">
      <slot name="label">{{ ariaLabel }}</slot>
    </label>
    <select
      :id="id"
      :value="modelValue"
      :disabled="disabled"
      :required="required"
      :aria-label="ariaLabel"
      :title="title"
      :class="[baseClasses, sizeClasses, stateClasses, $attrs.class]"
      @change="handleChange"
    >
      <option v-if="placeholder && !modelValue" value="" disabled>
        {{ placeholder }}
      </option>
      <option
        v-for="option in options"
        :key="option.id"
        :value="optionValue(option)"
        :disabled="option.disabled"
      >
        {{ option.label }}
      </option>
    </select>
    <span
      class="dropdown-select-icon"
      :class="iconClasses"
      aria-hidden="true"
    >
      ▼
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed, useAttrs } from 'vue'
import type { DropdownOption, DropdownSize } from '@/types/dropdown'

interface Props {
  /** Array of options to display */
  options: DropdownOption[]
  /** Currently selected value */
  modelValue?: any
  /** Placeholder text when no option is selected */
  placeholder?: string
  /** Size variant */
  size?: DropdownSize
  /** Whether the dropdown is disabled */
  disabled?: boolean
  /** Whether the dropdown is required */
  required?: boolean
  /** HTML id attribute */
  id?: string
  /** ARIA label for accessibility */
  ariaLabel?: string
  /** Title attribute for tooltip */
  title?: string
}

interface Emits {
  (e: 'update:modelValue', value: any): void
}

const props = withDefaults(defineProps<Props>(), {
  size: 'md',
  disabled: false,
  required: false,
})

const emit = defineEmits<Emits>()
const attrs = useAttrs()

// Compute wrapper classes
const wrapperClass = computed(() => 'relative inline-flex items-center')

// Base classes that apply to all dropdowns (use theme tokens for consistency)
const baseClasses = computed(() =>
  [
    'dropdown-select',
    'appearance-none',
    'border-border',
    'bg-surface-muted/60',
    'text-foreground',
    'font-semibold',
    'transition-colors',
    'focus:border-ring',
    'focus:outline-none',
    'focus:ring-2',
    'focus:ring-ring/60',
  ].join(' ')
)

// Size-specific classes — keep border radius and color consistent; sizes affect spacing only
const sizeClasses = computed(() => {
  switch (props.size) {
    case 'xs':
      return 'pl-2 pr-10 py-1 text-xs rounded-4'
    case 'sm':
      return 'pl-2 pr-10 py-1 text-xs rounded-4'
    case 'md':
      return 'pl-3 pr-10 py-2 text-sm rounded-4'
    case 'lg':
      return 'pl-4 pr-10 py-3 text-base rounded-4'
    default:
      return 'pl-3 pr-10 py-2 text-sm rounded-4'
  }
})

// State-dependent classes
const stateClasses = computed(() => {
  const classes: string[] = []
  if (props.disabled) {
    classes.push('opacity-60', 'cursor-not-allowed', 'bg-surface-muted/30', 'border-border')
  } else {
    classes.push('border-border', 'hover:border-accent/80', 'bg-surface')
  }
  return classes.join(' ')
})

// Icon classes
const iconClasses = computed(() => {
  const classes = [
    'pointer-events-none',
    'absolute',
    'top-1/2',
    '-translate-y-1/2',
    'text-subtle-foreground',
  ]
  
  // Keep a consistent icon offset so the visual feels the same everywhere
  classes.push('right-5', 'text-[10px]')
  
  return classes.join(' ')
})

// Get the value to use for an option
function optionValue(option: DropdownOption): any {
  return option.value !== undefined ? option.value : option.id
}

// Handle selection change
function handleChange(event: Event) {
  const target = event.target as HTMLSelectElement
  const value = target.value
  
  // Find the selected option to get its proper value
  const selectedOption = props.options.find(opt => 
    String(optionValue(opt)) === String(value)
  )
  
  if (selectedOption) {
    emit('update:modelValue', optionValue(selectedOption))
  } else if (value === '') {
    // Handle placeholder/empty selection
    emit('update:modelValue', undefined)
  }
}
</script>

<style scoped>
/* Additional theme-aware styles if needed */
.dropdown-select {
  /* Ensure consistent styling across browsers */
  background-image: none;
}

/* Remove default dropdown arrow in IE/Edge */
.dropdown-select::-ms-expand {
  display: none;
}

/* Firefox-specific styles */
@-moz-document url-prefix() {
  .dropdown-select {
    text-indent: 0.01px;
    text-overflow: '';
  }
}
</style>