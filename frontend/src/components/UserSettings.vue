<template>
  <div class="user-settings-dropdown" v-show="showMenu" @click.stop>
    <div class="dropdown-content">
      <div class="dropdown-header">
        <h3>User Settings</h3>
        <button @click="$emit('close-menu')" class="close-btn">
          <i class="fa fa-times"></i>
        </button>
      </div>
      <div class="dropdown-item" @click="showPasswordModal = true">
        <i class="fa fa-key"></i>
        <span>Change Password</span>
      </div>
      <div class="dropdown-item" @click="logout">
        <i class="fa fa-sign-out"></i>
        <span>Logout</span>
      </div>
    </div>
    
    <!-- Password Change Modal -->
    <div class="modal-overlay" v-if="showPasswordModal" @click.self="showPasswordModal = false">
      <div class="modal-content">
        <div class="modal-header">
          <h3>Change Password</h3>
          <button @click="showPasswordModal = false" class="close-btn">
            <i class="fa fa-times"></i>
          </button>
        </div>
        <div class="modal-body">
          <form @submit.prevent="changePassword">
            <div class="form-group">
              <label for="currentPassword">Current Password</label>
              <div class="password-input">
                <input 
                  :type="showCurrentPassword ? 'text' : 'password'" 
                  id="currentPassword" 
                  v-model="passwordData.currentPassword"
                  required
                />
                <button type="button" class="toggle-password" @click="showCurrentPassword = !showCurrentPassword">
                  <i :class="showCurrentPassword ? 'fa fa-eye-slash' : 'fa fa-eye'"></i>
                </button>
              </div>
            </div>
            <div class="form-group">
              <label for="newPassword">New Password</label>
              <div class="password-input">
                <input 
                  :type="showNewPassword ? 'text' : 'password'" 
                  id="newPassword" 
                  v-model="passwordData.newPassword"
                  required
                />
                <button type="button" class="toggle-password" @click="showNewPassword = !showNewPassword">
                  <i :class="showNewPassword ? 'fa fa-eye-slash' : 'fa fa-eye'"></i>
                </button>
              </div>
              <span v-if="passwordError" class="error-message">{{ passwordError }}</span>
            </div>
            <div class="form-group">
              <label for="confirmPassword">Confirm New Password</label>
              <div class="password-input">
                <input 
                  :type="showConfirmPassword ? 'text' : 'password'" 
                  id="confirmPassword" 
                  v-model="passwordData.confirmPassword"
                  required
                />
                <button type="button" class="toggle-password" @click="showConfirmPassword = !showConfirmPassword">
                  <i :class="showConfirmPassword ? 'fa fa-eye-slash' : 'fa fa-eye'"></i>
                </button>
              </div>
              <span v-if="confirmError" class="error-message">{{ confirmError }}</span>
            </div>
            <div v-if="formError" class="form-error">{{ formError }}</div>
            <div v-if="formSuccess" class="form-success">{{ formSuccess }}</div>
            <div class="modal-footer">
              <button type="button" @click="showPasswordModal = false" class="cancel-btn">Cancel</button>
              <button type="submit" class="submit-btn" :disabled="isSubmitting">
                <span v-if="isSubmitting">
                  <i class="fa fa-spinner fa-spin"></i>
                </span>
                <span v-else>Change Password</span>
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, defineProps, defineEmits } from 'vue';

const props = defineProps({
  showMenu: {
    type: Boolean,
    required: true
  }
});

const emit = defineEmits(['close-menu', 'logout']);

// Password change modal
const showPasswordModal = ref(false);
const isSubmitting = ref(false);
const formError = ref('');
const formSuccess = ref('');

// Password visibility toggles
const showCurrentPassword = ref(false);
const showNewPassword = ref(false);
const showConfirmPassword = ref(false);

// Password validation
const passwordData = ref({
  currentPassword: '',
  newPassword: '',
  confirmPassword: ''
});

const passwordError = ref('');
const confirmError = ref('');

// Validate passwords
watch(() => passwordData.value.newPassword, (newValue) => {
  if (newValue.length > 0 && newValue.length < 8) {
    passwordError.value = 'Password must be at least 8 characters';
  } else {
    passwordError.value = '';
  }
});

watch(() => passwordData.value.confirmPassword, (newValue) => {
  if (newValue && newValue !== passwordData.value.newPassword) {
    confirmError.value = 'Passwords do not match';
  } else {
    confirmError.value = '';
  }
});

// Close dropdown when clicking outside
function handleClickOutside(event) {
  if (props.showMenu) {
    emit('close-menu');
  }
}

// Logout function
function logout() {
  emit('logout');
  emit('close-menu');
}

// Change password function
async function changePassword() {
  // Reset form status
  formError.value = '';
  formSuccess.value = '';
  
  // Validate password
  if (passwordData.value.newPassword.length < 8) {
    formError.value = 'New password must be at least 8 characters long';
    return;
  }
  
  // Validate confirmation
  if (passwordData.value.newPassword !== passwordData.value.confirmPassword) {
    formError.value = 'Passwords do not match';
    return;
  }
  
  // Submit form
  isSubmitting.value = true;
  
  try {
    // Get token from localStorage
    const token = localStorage.getItem('jwt_token');
    if (!token) {
      throw new Error('Not authenticated');
    }
    
    const response = await fetch('/api/restricted/change-password', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`
      },
      body: JSON.stringify({
        currentPassword: passwordData.value.currentPassword,
        newPassword: passwordData.value.newPassword
      })
    });
    
    if (response.ok) {
      formSuccess.value = 'Password changed successfully!';
      // Reset form
      passwordData.value = {
        currentPassword: '',
        newPassword: '',
        confirmPassword: ''
      };
      
      // Close modal after 2 seconds
      setTimeout(() => {
        showPasswordModal.value = false;
        formSuccess.value = '';
      }, 2000);
    } else {
      const data = await response.json();
      formError.value = data.message || 'Failed to change password';
    }
  } catch (error) {
    console.error('Error changing password:', error);
    formError.value = 'An error occurred. Please try again.';
  } finally {
    isSubmitting.value = false;
  }
}
</script>

<style scoped>
.user-settings-dropdown {
  position: absolute;
  top: 100%;
  right: 0;
  z-index: 1000;
  width: 240px;
  margin-top: 5px;
}

.dropdown-content {
  background-color: #333;
  border-radius: 6px;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.2);
  overflow: hidden;
}

.dropdown-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 15px;
  border-bottom: 1px solid #444;
}

.dropdown-header h3 {
  margin: 0;
  font-size: 16px;
  color: white;
}

.dropdown-item {
  padding: 12px 15px;
  display: flex;
  align-items: center;
  color: white;
  cursor: pointer;
  transition: background-color 0.2s;
}

.dropdown-item:hover {
  background-color: #444;
}

.dropdown-item i {
  width: 20px;
  margin-right: 10px;
}

.close-btn {
  background: none;
  border: none;
  color: #999;
  cursor: pointer;
  font-size: 16px;
}

.close-btn:hover {
  color: white;
}

/* Modal styles */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: rgba(0, 0, 0, 0.7);
  display: flex;
  justify-content: center;
  align-items: center;
  z-index: 1100;
}

.modal-content {
  background-color: #333;
  width: 400px;
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.3);
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
  font-size: 18px;
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
}

.password-input {
  position: relative;
  display: flex;
  align-items: center;
}

.password-input input {
  flex-grow: 1;
  padding: 10px;
  border: 1px solid #555;
  border-radius: 4px;
  background-color: #444;
  color: white;
  font-size: 14px;
}

.toggle-password {
  position: absolute;
  right: 10px;
  background: none;
  border: none;
  color: #999;
  cursor: pointer;
}

input:focus {
  outline: none;
  border-color: #2563eb;
}

.error-message {
  color: #ff6b6b;
  font-size: 12px;
  margin-top: 5px;
  display: block;
}

.form-error {
  background-color: rgba(255, 107, 107, 0.2);
  color: #ff6b6b;
  padding: 10px;
  border-radius: 4px;
  margin-bottom: 15px;
  font-size: 14px;
}

.form-success {
  background-color: rgba(46, 213, 115, 0.2);
  color: #2ed573;
  padding: 10px;
  border-radius: 4px;
  margin-bottom: 15px;
  font-size: 14px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 20px;
}

.cancel-btn {
  background-color: #555;
  color: white;
  border: none;
  border-radius: 4px;
  padding: 10px 16px;
  cursor: pointer;
  font-size: 14px;
  transition: background-color 0.2s;
}

.submit-btn {
  background-color: #2563eb;
  color: white;
  border: none;
  border-radius: 4px;
  padding: 10px 16px;
  cursor: pointer;
  font-size: 14px;
  transition: background-color 0.2s;
}

.cancel-btn:hover {
  background-color: #666;
}

.submit-btn:hover {
  background-color: #1d4ed8;
}

.submit-btn:disabled {
  background-color: #3b82f6;
  opacity: 0.7;
  cursor: not-allowed;
}
</style>