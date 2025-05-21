<template>
  <!-- HEADER --------------------------------------------------------------->
  <div class="bg-zinc-900 text-white flex-none h-16 flex items-center px-5 relative select-none">
    <!-- invisible spacer keeps centre alignment -->
    <div class="flex-1"></div>

    <!-- centred logo -------------------------------------------------------->
    <div
      class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 flex items-center space-x-2 pointer-events-none"
    >
      <!-- logo mark -->
      <ManifoldLogo class="h-6 w-auto" />
      <!-- word-mark -->
      <span class="text-xl font-bold tracking-wide">Manifold</span>
    </div>

    <!-- DESKTOP actions ----------------------------------------------------->
    <div class="flex items-center space-x-2 flex-1 justify-end lg:flex">
      <!-- FILE INPUT (hidden) -->
      <input
        ref="fileInput"
        type="file"
        class="hidden"
        accept=".json"
        @change="onFileSelected"
      />

      <!-- templates dropdown -->
      <div class="relative">
        <button
          @click="toggleTemplatesMenu"
          class="bg-gray-700 hover:bg-gray-600 rounded px-3 py-1 text-sm flex items-center space-x-1"
        >
          <i class="fa fa-file-text"></i> <span>Templates</span>
          <i class="fa fa-caret-down"></i>
        </button>

        <div
          v-if="showTemplatesMenu"
          class="absolute right-0 mt-2 w-52 bg-gray-800 dark:bg-neutral-900 rounded shadow-lg flex flex-col divide-y divide-gray-700 overflow-hidden z-20"
        >
          <button
            v-for="t in templates"
            :key="t.template"
            @click="loadTemplate(t)"
            class="px-4 py-2 hover:bg-gray-700 text-left"
          >
            {{ t.name }}
          </button>
          <div
            v-if="templates.length === 0"
            class="px-4 py-2 text-gray-400 text-center"
          >
            No templates available
          </div>
        </div>
      </div>

      <!-- open / save -->
      <button
        class="bg-gray-700 hover:bg-gray-600 rounded px-3 py-1 text-sm flex items-center space-x-1"
        @click="openFile"
      >
        <i class="fa fa-folder-open"></i> <span>Open</span>
      </button>
      <button
        class="bg-gray-700 hover:bg-gray-600 rounded px-3 py-1 text-sm flex items-center space-x-1"
        @click="saveFlow"
      >
        <i class="fa fa-save"></i> <span>Save</span>
      </button>

      <!-- user settings dropdown -->
      <div class="relative">
        <button
          @click="toggleUserMenu"
          class="bg-gray-700 hover:bg-gray-600 rounded px-3 py-1 text-sm flex items-center space-x-1"
        >
          <i class="fa fa-user-circle"></i> <span>{{ username }}</span>
          <i class="fa fa-caret-down"></i>
        </button>
        <UserSettings
          v-if="showUserMenu"
          :showMenu="showUserMenu"
          @close-menu="showUserMenu = false"
          @logout="logout"
        />
      </div>
    </div>

    <!-- MOBILE hamburger ---------------------------------------------------->
    <div class="flex-1 flex justify-end lg:hidden">
      <div class="relative">
        <button
          @click="mobileMenuOpen = !mobileMenuOpen"
          class="p-2 rounded hover:bg-gray-800"
        >
          <i class="fa fa-bars text-xl"></i>
        </button>

        <!-- mobile dropdown -->
        <div
          v-if="mobileMenuOpen"
          class="absolute right-0 mt-2 w-52 bg-gray-800 dark:bg-neutral-900 rounded shadow-lg flex flex-col divide-y divide-gray-700 overflow-hidden z-20"
        >
          <button @click="toggleTemplatesMenu" class="px-4 py-2 hover:bg-gray-700 text-left">
            Templates
          </button>
          <button @click="openFile" class="px-4 py-2 hover:bg-gray-700 text-left">
            Open
          </button>
          <button @click="saveFlow" class="px-4 py-2 hover:bg-gray-700 text-left">
            Save
          </button>
          <button @click="toggleUserMenu" class="px-4 py-2 hover:bg-gray-700 text-left">
            User&nbsp;Settings
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue';
import UserSettings from '@/components/UserSettings.vue';
import ManifoldLogo from '@/components/icons/ManifoldLogo.vue';

/* ───────────────────────── refs/state ───────────────────────── */
const fileInput = ref<HTMLInputElement | null>(null);
const emit = defineEmits(['save', 'restore', 'logout', 'load-template']);

const showUserMenu = ref(false);
const showTemplatesMenu = ref(false);
const mobileMenuOpen = ref(false);
const username = ref('User');

const templates = ref<Array<{ name: string; template: string }>>([]);

/* ─────────────────── helpers / api calls ────────────────────── */
onMounted(() => {
  fetchUser();
  fetchTemplates();
});

function fetchUser() {
  const token = localStorage.getItem('jwt_token');
  if (!token) return;

  fetch('/api/restricted/user', { headers: { Authorization: `Bearer ${token}` } })
    .then((r) => (r.ok ? r.json() : Promise.reject()))
    .then((d) => (username.value = d.username))
    .catch(() => {});
}

function fetchTemplates() {
  fetch('/api/workflows/templates')
    .then((r) => (r.ok ? r.json() : Promise.reject()))
    .then((arr) => {
      templates.value = arr.map((t: any) => ({
        name: formatTemplateName(t.name),
        template: t.id,
      }));
    })
    .catch(() => {});
}

function formatTemplateName(fn: string) {
  return fn
    .replace(/^\d+_/, '')
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (l) => l.toUpperCase());
}

/* ─────────────────── button actions ─────────────────────────── */
function saveFlow() {
  emit('save');
}
function openFile() {
  fileInput.value?.click();
}
function onFileSelected(e: Event) {
  const input = e.target as HTMLInputElement;
  if (!(input.files && input.files[0])) return;

  const reader = new FileReader();
  reader.onload = (ev) => {
    if (typeof ev.target?.result !== 'string') return;
    try {
      emit('restore', JSON.parse(ev.target.result));
    } catch {
      alert('Invalid Manifold flow file.');
    }
  };
  reader.readAsText(input.files[0]);
  input.value = '';
}

/* dropdown logic */
function toggleUserMenu() {
  showUserMenu.value = !showUserMenu.value;
  if (showUserMenu.value) {
    showTemplatesMenu.value = false;
    mobileMenuOpen.value = false;
  }
}
function toggleTemplatesMenu() {
  showTemplatesMenu.value = !showTemplatesMenu.value;
  if (showTemplatesMenu.value) {
    showUserMenu.value = false;
    mobileMenuOpen.value = false;
  }
}
function loadTemplate(t: { name: string; template: string }) {
  emit('load-template', t.template);
  showTemplatesMenu.value = false;
  mobileMenuOpen.value = false;
}
function logout() {
  emit('logout');
  showUserMenu.value = showTemplatesMenu.value = mobileMenuOpen.value = false;
}

/* close open dropdowns when window resizes ≥ lg ----------------*/
watch(
  () => window.innerWidth,
  () => {
    if (window.innerWidth >= 1024) mobileMenuOpen.value = false;
  },
  { immediate: true },
);
</script>

<style scoped>
.fa-bars {
  min-width: 1.25rem; /* extra touch target for hamburger */
}
</style>
