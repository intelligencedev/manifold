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
      <div class="flex flex-col items-center">
        <ManifoldLogo class="h-6 w-auto" />
        <!-- word-mark -->
        <span class="text-md font-bold tracking-wide">Manifold</span>
      </div>
    </div>

    <!-- DESKTOP actions ----------------------------------------------------->
    <div class="hidden items-center space-x-2 flex-1 justify-end lg:flex">
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
        <BaseButton
          @click="toggleTemplatesMenu"
          class="bg-teal-700 hover:bg-teal-600 rounded px-3 py-1 text-sm flex items-center space-x-1"
        >
          <i class="fa fa-file-text"></i> <span>Templates</span>
          <i class="fa fa-caret-down"></i>
        </BaseButton>

        <BaseDropdownMenu :show="showTemplatesMenu">
          <div class="templates-list">
            <div 
              v-for="(t, index) in templates" 
              :key="t.template"
              class="template-item"
              @click="loadTemplate(t)"
            >
              {{ t.name }}
              <div v-if="index < templates.length - 1" class="template-divider"></div>
            </div>
            <div
              v-if="templates.length === 0"
              class="px-4 py-2 text-gray-400 text-center"
            >
              No templates available
            </div>
          </div>
        </BaseDropdownMenu>
      </div>

      <!-- open / save -->
      <BaseButton
        class="bg-teal-700 hover:bg-teal-600 rounded px-3 py-1 text-sm flex items-center space-x-1"
        @click="openFile"
      >
        <i class="fa fa-folder-open"></i> <span>Open</span>
      </BaseButton>
      <BaseButton
        class="bg-teal-700 hover:bg-teal-600 rounded px-3 py-1 text-sm flex items-center space-x-1"
        @click="saveFlow"
      >
        <i class="fa fa-save"></i> <span>Save</span>
      </BaseButton>

      <BaseButton
        class="bg-teal-700 hover:bg-teal-600 rounded px-3 py-1 text-sm"
        @click="emit('toggle-mode')"
        title="Switch Mode"
      >
        <svg v-if="props.mode === 'flow'" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" class="w-5 h-5 fill-current">
          <path d="M2 3h20v14H6l-4 4V3z" stroke="currentColor" stroke-width="2" fill="none"/></svg>
        <svg v-else xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" class="w-5 h-5 fill-current">
          <circle cx="6" cy="12" r="2"/><circle cx="12" cy="6" r="2"/><circle cx="18" cy="12" r="2"/>
          <path d="M7.5 11L11 7.5m2 0l3.5 3.5M11 7.5v9" stroke="currentColor" stroke-width="2" fill="none"/></svg>
      </BaseButton>

      <!-- user settings dropdown -->
      <div class="relative">
        <BaseButton
          @click="toggleUserMenu"
          class="bg-teal-700 hover:bg-teal-600 rounded px-3 py-1 text-sm flex items-center space-x-1"
        >
          <i class="fa fa-user-circle"></i> <span>{{ username }}</span>
          <i class="fa fa-caret-down"></i>
        </BaseButton>
        <BaseDropdownMenu :show="showUserMenu">
          <div class="templates-list user-menu-list">
            <div 
              class="template-item"
              @click="openPasswordModal"
            >
              <i class="fa fa-key mr-2"></i> Change Password
              <div class="template-divider"></div>
            </div>
            <div 
              class="template-item"
              @click="logout"
            >
              <i class="fa fa-sign-out mr-2"></i> Logout
            </div>
          </div>
        </BaseDropdownMenu>
      </div>
    </div>

    <!-- MOBILE hamburger ---------------------------------------------------->
    <div class="flex-1 flex justify-end lg:hidden">
      <div class="relative">
        <BaseButton
          @click="mobileMenuOpen = !mobileMenuOpen"
          class="p-2 rounded bg-teal-700 hover:bg-teal-600"
        >
          <i class="fa fa-bars text-xl"></i>
        </BaseButton>

        <!-- mobile dropdown -->
        <BaseDropdownMenu :show="mobileMenuOpen">
          <!-- Templates Button with inline dropdown -->
          <BaseButton @click="toggleMobileTemplatesMenu" class="px-4 py-2 bg-teal-700 hover:bg-teal-600 text-left flex justify-between items-center">
            Templates
            <i class="fa fa-caret-down ml-2"></i>
          </BaseButton>
          
          <!-- Templates Submenu -->
          <div v-if="showMobileTemplatesMenu" class="bg-gray-900">
            <div class="templates-list mobile">
              <template v-if="templates.length > 0">
                <div 
                  v-for="(t, index) in templates" 
                  :key="t.template"
                  class="template-item mobile"
                  @click="loadTemplate(t)"
                >
                  {{ t.name }}
                  <div v-if="index < templates.length - 1" class="template-divider"></div>
                </div>
              </template>
              <div
                v-else
                class="px-4 py-2 text-gray-400 text-center"
              >
                No templates available
              </div>
            </div>
          </div>
          
          <!-- Standard menu items -->
          <BaseButton @click="openFile" class="px-4 py-2 bg-teal-700 hover:bg-teal-600 text-left">
            Open
          </BaseButton>
          <BaseButton @click="saveFlow" class="px-4 py-2 bg-teal-700 hover:bg-teal-600 text-left">
            Save
          </BaseButton>

          <BaseButton @click="emit('toggle-mode')" class="px-4 py-2 bg-teal-700 hover:bg-teal-600 text-left flex items-center justify-center">
            <svg v-if="props.mode === 'flow'" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" class="w-5 h-5 fill-current">
              <path d="M2 3h20v14H6l-4 4V3z" stroke="currentColor" stroke-width="2" fill="none"/>
            </svg>
            <svg v-else xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" class="w-5 h-5 fill-current">
              <circle cx="6" cy="12" r="2"/><circle cx="12" cy="6" r="2"/><circle cx="18" cy="12" r="2"/>
              <path d="M7.5 11L11 7.5m2 0l3.5 3.5M11 7.5v9" stroke="currentColor" stroke-width="2" fill="none"/>
            </svg>
          </BaseButton>
          
          <!-- User Settings with inline dropdown -->
          <BaseButton @click="toggleMobileUserMenu" class="px-4 py-2 bg-teal-700 hover:bg-teal-600 text-left flex justify-between items-center">
            User&nbsp;Settings
            <i class="fa fa-caret-down ml-2"></i>
          </BaseButton>
          
          <!-- User Settings submenu -->
          <div v-if="showMobileUserMenu" class="bg-gray-900">
            <div class="templates-list mobile">
              <div 
                class="template-item mobile"
                @click="openPasswordModal"
              >
                <i class="fa fa-key mr-2"></i> Change Password
                <div class="template-divider"></div>
              </div>
              <div 
                class="template-item mobile"
                @click="logout"
              >
                <i class="fa fa-sign-out mr-2"></i> Logout
              </div>
            </div>
          </div>
        </BaseDropdownMenu>
      </div>
    </div>
  </div>
  
  <!-- Password Change Modal - Moved to root level -->
  <div class="modal-overlay" v-if="showPasswordModal" @click.self="closePasswordModal" @touchstart.self="closePasswordModal">
    <div class="modal-content" @click.stop>
      <div class="modal-header">
        <h3>Change Password</h3>
        <button @click="closePasswordModal" class="close-btn">
          <i class="fa fa-times"></i>
        </button>
      </div>
      <div class="modal-body">
        <form @submit.prevent="changePassword">
          <div class="form-group">
            <label>Current Password</label>
            <div class="password-input">
              <input 
                :type="showCurrentPassword ? 'text' : 'password'" 
                v-model="passwordData.currentPassword" 
                required
              />
              <button type="button" @click="showCurrentPassword = !showCurrentPassword">
                <i :class="['fa', showCurrentPassword ? 'fa-eye-slash' : 'fa-eye']"></i>
              </button>
            </div>
          </div>
          <div class="form-group">
            <label>New Password</label>
            <div class="password-input">
              <input 
                :type="showNewPassword ? 'text' : 'password'" 
                v-model="passwordData.newPassword" 
                required 
              />
              <button type="button" @click="showNewPassword = !showNewPassword">
                <i :class="['fa', showNewPassword ? 'fa-eye-slash' : 'fa-eye']"></i>
              </button>
            </div>
            <div v-if="passwordError" class="error">{{ passwordError }}</div>
          </div>
          <div class="form-group">
            <label>Confirm New Password</label>
            <div class="password-input">
              <input 
                :type="showConfirmPassword ? 'text' : 'password'" 
                v-model="passwordData.confirmPassword" 
                required
              />
              <button type="button" @click="showConfirmPassword = !showConfirmPassword">
                <i :class="['fa', showConfirmPassword ? 'fa-eye-slash' : 'fa-eye']"></i>
              </button>
            </div>
            <div v-if="confirmError" class="error">{{ confirmError }}</div>
          </div>
          <div v-if="formError" class="error form-error">{{ formError }}</div>
          <div v-if="formSuccess" class="success form-success">{{ formSuccess }}</div>
          <div class="form-actions">
            <button type="button" @click="closePasswordModal" class="cancel-btn">Cancel</button>
            <button type="submit" class="submit-btn" :disabled="isSubmitting">
              <span v-if="isSubmitting">Updating...</span>
              <span v-else>Update Password</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>

  <!-- Save Template Modal -->
  <div class="modal-overlay" v-if="showSaveModal" @click.self="closeSaveModal" @touchstart.self="closeSaveModal">
    <div class="modal-content" @click.stop>
      <div class="modal-header">
        <h3>Name Template</h3>
        <button @click="closeSaveModal" class="close-btn">
          <i class="fa fa-times"></i>
        </button>
      </div>
      <div class="modal-body">
        <div class="form-group">
          <label>Template Name</label>
          <input v-model="templateName" type="text" placeholder="Enter template name" />
        </div>
        <div v-if="saveError" class="error">{{ saveError }}</div>
        <div class="form-actions">
          <button type="button" @click="closeSaveModal" class="cancel-btn">Cancel</button>
          <button type="button" class="submit-btn" @click="confirmSaveTemplate" :disabled="isSavingTemplate">
            <span v-if="isSavingTemplate">Saving...</span>
            <span v-else>Save</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue';
import ManifoldLogo from '@/components/icons/ManifoldLogo.vue';
import BaseButton from '@/components/base/BaseButton.vue';
import BaseDropdownMenu from '@/components/base/BaseDropdownMenu.vue';

/* ───────────────────────── refs/state ───────────────────────── */
const fileInput = ref<HTMLInputElement | null>(null);
const props = defineProps({
  mode: { type: String, default: 'flow' }
})
const emit = defineEmits(['save', 'restore', 'logout', 'load-template', 'toggle-mode']);

const showUserMenu = ref(false);
const showTemplatesMenu = ref(false);
const mobileMenuOpen = ref(false);
const showMobileTemplatesMenu = ref(false);
const showMobileUserMenu = ref(false);
const showPasswordModal = ref(false);
const username = ref('User');

// Password change state
const passwordData = ref({
  currentPassword: '',
  newPassword: '',
  confirmPassword: ''
});
const showCurrentPassword = ref(false);
const showNewPassword = ref(false);
const showConfirmPassword = ref(false);
const passwordError = ref('');
const confirmError = ref('');
const formError = ref('');
const formSuccess = ref('');
const isSubmitting = ref(false);

const templates = ref<Array<{ name: string; template: string }>>([]);

const showSaveModal = ref(false);
const templateName = ref('');
const saveError = ref('');
const isSavingTemplate = ref(false);

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
  // Open modal to name and save template
  showSaveModal.value = true;
  document.body.classList.add('modal-open');
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

/* mobile dropdown logic */
function toggleMobileTemplatesMenu() {
  showMobileTemplatesMenu.value = !showMobileTemplatesMenu.value;
  if (showMobileTemplatesMenu.value) {
    showMobileUserMenu.value = false;
  }
}
function toggleMobileUserMenu() {
  showMobileUserMenu.value = !showMobileUserMenu.value;
  if (showMobileUserMenu.value) {
    showMobileTemplatesMenu.value = false;
  }
}
function loadTemplate(t: { name: string; template: string }) {
  emit('load-template', t.template);
  showTemplatesMenu.value = false;
  showMobileTemplatesMenu.value = false;
  mobileMenuOpen.value = false;
}
function logout() {
  emit('logout');
  showUserMenu.value = false;
  showTemplatesMenu.value = false;
  showMobileUserMenu.value = false;
  showMobileTemplatesMenu.value = false;
  showPasswordModal.value = false;
  mobileMenuOpen.value = false;
}

/* password modal functions */
function openPasswordModal() {
  // Close all menus when opening password modal
  showUserMenu.value = false;
  showTemplatesMenu.value = false;
  mobileMenuOpen.value = false;
  showMobileUserMenu.value = false;
  showMobileTemplatesMenu.value = false;
  
  // Add a small delay to ensure dropdown menus are closed first
  setTimeout(() => {
    // Show the password modal
    showPasswordModal.value = true;
    // Add a class to the body to prevent background scrolling
    document.body.classList.add('modal-open');
  }, 150); // Increased delay for better reliability
}

function closePasswordModal() {
  showPasswordModal.value = false;
  // Remove the modal-open class from body
  document.body.classList.remove('modal-open');
  
  // Reset form fields and errors
  passwordData.value = {
    currentPassword: '',
    newPassword: '',
    confirmPassword: ''
  };
  passwordError.value = '';
  confirmError.value = '';
  formError.value = '';
  formSuccess.value = '';
  isSubmitting.value = false;
}

function changePassword() {
  // Validate passwords
  if (passwordData.value.newPassword.length < 8) {
    passwordError.value = 'Password must be at least 8 characters';
    return;
  }
  
  if (passwordData.value.newPassword !== passwordData.value.confirmPassword) {
    confirmError.value = 'Passwords do not match';
    return;
  }
  
  isSubmitting.value = true;
  formError.value = '';
  formSuccess.value = '';
  
  // Here you would make an API call to change the password
  // For now, we'll just simulate success after a delay
  setTimeout(() => {
    isSubmitting.value = false;
    formSuccess.value = 'Password updated successfully!';
    
    // Close the modal after a short delay
    setTimeout(() => {
      closePasswordModal();
    }, 2000);
  }, 1000);
}

/* close open dropdowns when window resizes ≥ lg ----------------*/
watch(
  () => window.innerWidth,
  () => {
    if (window.innerWidth >= 1024) {
      mobileMenuOpen.value = false;
      showMobileTemplatesMenu.value = false;
      showMobileUserMenu.value = false;
    }
  },
  { immediate: true },
);

// Close mobile submenus when the main mobile menu is closed
watch(
  () => mobileMenuOpen.value,
  (isOpen) => {
    if (!isOpen) {
      showMobileTemplatesMenu.value = false;
      showMobileUserMenu.value = false;
    }
  }
);

// Handle modal visibility and body class
watch(
  () => showPasswordModal.value,
  (isVisible) => {
    if (isVisible) {
      document.body.classList.add('modal-open');
    } else {
      document.body.classList.remove('modal-open');
    }
  }
);

// Functions for save template modal
function closeSaveModal() {
  showSaveModal.value = false;
  document.body.classList.remove('modal-open');
  templateName.value = '';
  saveError.value = '';
  isSavingTemplate.value = false;
}

async function confirmSaveTemplate() {
  if (!templateName.value) {
    saveError.value = 'Name is required';
    return;
  }
  isSavingTemplate.value = true;
  saveError.value = '';
  // Emit save event with template name
  emit('save', templateName.value);
  closeSaveModal();
}

defineExpose({ fetchTemplates });
</script>

<style scoped>
.fa-bars {
  min-width: 1.25rem; /* extra touch target for hamburger */
}

.templates-list {
  background-color: #1e1e1e;
  border-radius: 0.25rem;
  overflow: hidden;
  box-shadow: 0 4px 6px rgba(0, 0, 0, 0.2);
}

.templates-list.mobile {
  background-color: rgba(30, 30, 30, 0.8); /* Similar to desktop but with transparency */
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
  padding-left: 0; /* Remove any left padding that could cause alignment issues */
}

.template-item {
  padding: 0.75rem 1rem;
  color: #fff;
  cursor: pointer;
  position: relative;
  transition: background-color 0.2s ease;
}

.template-item:hover {
  background-color: rgba(56, 178, 172, 0.5); /* teal-600 with opacity */
}

.template-item.mobile {
  padding-left: 1.5rem; /* Ensure consistent indentation */
}

.template-item.mobile:hover {
  background-color: rgba(56, 178, 172, 0.5); /* teal-600 with opacity - same as desktop */
}

.template-divider {
  position: absolute;
  bottom: 0;
  left: 0.75rem;
  right: 0.75rem;
  height: 1px;
  background-color: rgba(107, 114, 128, 0.4); /* gray-500 with opacity */
}

/* Special handling for mobile dividers */
.template-item.mobile .template-divider {
  left: 1.25rem;
  right: 0.75rem;
}

/* User menu specific styles */
.user-menu-list {
  min-width: 180px;
}

/* Modal styles */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: rgba(0, 0, 0, 0.8); /* Darkened for better visibility */
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999; /* Use highest possible z-index */
  overflow-y: auto; /* Allow scrolling on smaller screens if needed */
  padding: 1rem 0; /* Add some vertical padding */
  will-change: opacity; /* Optimize for animations */
}

/* Adding @layer to ensure these styles take precedence */
@media (max-width: 1023px) {
  .modal-overlay {
    padding: 0;
    overflow: hidden;
    display: flex !important; /* Force display even if there are display conflicts */
    align-items: center !important;
    justify-content: center !important;
  }
}

.modal-content {
  background-color: #2a2a2a;
  border-radius: 6px;
  width: 90%;
  max-width: 400px;
  box-shadow: 0 5px 15px rgba(0, 0, 0, 0.3);
  margin: 0 1rem; /* Add margin to prevent touching edges on small screens */
  position: relative; /* Add position relative for better mobile handling */
  z-index: 10000; /* Ensure it's above everything else */
}

@media (max-width: 1023px) {
  .modal-content {
    width: 85%;
    max-height: 85vh;
    overflow-y: auto;
    margin: auto; /* Center in viewport */
  }
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 15px 20px;
  border-bottom: 1px solid #444;
}

.modal-header h3 {
  margin: 0;
  color: white;
  font-size: 18px;
}

.close-btn {
  background: none;
  border: none;
  color: #999;
  font-size: 18px;
  cursor: pointer;
}

.close-btn:hover {
  color: white;
}

.modal-body {
  padding: 20px;
}

.form-group {
  margin-bottom: 15px;
}

.form-group label {
  display: block;
  margin-bottom: 5px;
  color: #ddd;
  font-size: 14px;
}

.password-input {
  display: flex;
  position: relative;
  width: 100%; /* Ensure full width on mobile */
}

.password-input input {
  width: 100%;
  padding: 8px 12px;
  border: 1px solid #555;
  border-radius: 4px;
  background-color: #333;
  color: white;
  font-size: 16px; /* Prevent zoom on input focus on iOS */
}

.password-input button {
  position: absolute;
  right: 8px;
  top: 50%;
  transform: translateY(-50%);
  background: none;
  border: none;
  color: #999;
  cursor: pointer;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 20px;
  gap: 10px;
}

.cancel-btn {
  padding: 8px 15px;
  background-color: transparent;
  border: 1px solid #666;
  color: #ddd;
  border-radius: 4px;
  cursor: pointer;
}

.submit-btn {
  padding: 8px 15px;
  background-color: #38b2ac; /* teal-600 */
  border: none;
  color: white;
  border-radius: 4px;
  cursor: pointer;
}

.submit-btn:hover {
  background-color: #319795; /* teal-700 */
}

.submit-btn:disabled {
  background-color: #4a5568;
  cursor: not-allowed;
}

.error {
  color: #f56565; /* red-500 */
  font-size: 12px;
  margin-top: 5px;
}

.form-error {
  padding: 8px;
  background-color: rgba(245, 101, 101, 0.1);
  border-left: 2px solid #f56565;
  margin-bottom: 15px;
  max-width: 100%; /* Ensure it doesn't overflow on mobile */
  word-break: break-word; /* Break words if needed */
}

.success {
  color: #48bb78; /* green-500 */
  font-size: 12px;
  margin-top: 5px;
}

.form-success {
  padding: 8px;
  background-color: rgba(72, 187, 120, 0.1);
  border-left: 2px solid #48bb78;
  margin-bottom: 15px;
  max-width: 100%; /* Ensure it doesn't overflow on mobile */
  word-break: break-word; /* Break words if needed */
}

/* Animation for modal appearance */
@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

@keyframes scaleIn {
  from { transform: scale(0.95); opacity: 0; }
  to { transform: scale(1); opacity: 1; }
}

.modal-overlay {
  animation: fadeIn 0.2s ease-out forwards;
}

.modal-content {
  animation: scaleIn 0.2s ease-out forwards;
}

/* Style for body when modal is open to prevent background scrolling */
</style>

<style>
/* Global styles that need to be applied outside the component */
body.modal-open {
  overflow: hidden !important;
  position: fixed !important;
  width: 100% !important;
  height: 100% !important;
}
</style>
