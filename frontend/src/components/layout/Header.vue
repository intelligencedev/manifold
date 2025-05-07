<template>
  <header class="header">
    <div class="logo">
      <img src="/assets/manifold.svg" alt="Manifold Logo" height="32" />
      <span class="title">Manifold</span>
    </div>
    <div class="controls">
      <input type="file" ref="fileInput" style="display:none" @change="onFileSelected" accept=".json" />
      <button class="control-btn" @click="openFile">
        <i class="fa fa-folder-open"></i>
        Open
      </button>
      <button class="control-btn" @click="saveFlow">
        <i class="fa fa-save"></i>
        Save
      </button>
      <div class="user-menu">
        <button class="control-btn user-btn" @click="toggleUserMenu">
          <i class="fa fa-user-circle"></i>
          <span>{{ username }}</span>
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
  </header>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import UserSettings from '@/components/UserSettings.vue';

const fileInput = ref<HTMLElement | null>(null);
const emit = defineEmits(['save', 'restore', 'logout']);
const showUserMenu = ref(false);
const username = ref('User');

// Get the username on component mount
onMounted(() => {
  // Try to get username from localStorage or session
  const token = localStorage.getItem('jwt_token');
  if (token) {
    // Fetch user info from the backend
    fetch('/api/restricted/user', {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    })
    .then(response => {
      if (response.ok) {
        return response.json();
      }
      throw new Error('Failed to get user info');
    })
    .then(data => {
      username.value = data.username;
    })
    .catch(error => {
      console.error('Error fetching user info:', error);
    });
  }
});

function saveFlow() {
  emit('save');
}

function openFile() {
  if (fileInput.value) {
    (fileInput.value as HTMLInputElement).click();
  }
}

function onFileSelected(event: Event) {
  const input = event.target as HTMLInputElement;
  if (input.files && input.files[0]) {
    const file = input.files[0];
    const reader = new FileReader();

    reader.onload = (e) => {
      if (e.target && typeof e.target.result === 'string') {
        try {
          const parsedFlow = JSON.parse(e.target.result);
          emit('restore', parsedFlow);
        } catch (error) {
          console.error('Failed to parse file:', error);
          alert('Failed to parse file. Please select a valid Manifold flow file.');
        }
      }
    };

    reader.readAsText(file);
    // Reset the file input so the same file can be selected again
    input.value = '';
  }
}

function toggleUserMenu() {
  showUserMenu.value = !showUserMenu.value;
}

function logout() {
  emit('logout');
}
</script>

<style scoped>
.header {
  height: 60px;
  background-color: #222;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 20px;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.2);
  color: white;
}

.logo {
  display: flex;
  align-items: center;
}

.title {
  font-size: 1.5rem;
  font-weight: bold;
  margin-left: 10px;
}

.controls {
  display: flex;
  gap: 10px;
  align-items: center;
}

.control-btn {
  background-color: #444;
  color: white;
  border: none;
  border-radius: 4px;
  padding: 8px 16px;
  font-size: 0.9rem;
  cursor: pointer;
  transition: background-color 0.2s;
  display: flex;
  align-items: center;
  gap: 6px;
}

.control-btn:hover {
  background-color: #555;
}

.control-btn i {
  font-size: 1rem;
}

.user-menu {
  position: relative;
}

.user-btn {
  display: flex;
  align-items: center;
  gap: 8px;
}

.user-btn i:last-child {
  font-size: 0.8rem;
  margin-left: 2px;
}
</style>