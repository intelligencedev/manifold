<template>
  <div class="change-password-container">
    <div class="change-password-box">
      <h2>Change Password</h2>
      <div class="form-group">
        <label for="currentPassword">Current Password</label>
        <input 
          type="password" 
          id="currentPassword" 
          v-model="currentPassword" 
          placeholder="Enter current password"
        >
      </div>
      <div class="form-group">
        <label for="newPassword">New Password</label>
        <input 
          type="password" 
          id="newPassword" 
          v-model="newPassword" 
          placeholder="Enter new password"
          @input="validatePassword"
        >
      </div>
      <div class="form-group">
        <label for="confirmPassword">Confirm New Password</label>
        <input 
          type="password" 
          id="confirmPassword" 
          v-model="confirmPassword" 
          placeholder="Confirm new password"
          @input="validateConfirmation"
        >
      </div>
      <div v-if="passwordError" class="error-message">{{ passwordError }}</div>
      <div v-if="confirmError" class="error-message">{{ confirmError }}</div>
      <div v-if="apiError" class="error-message">{{ apiError }}</div>
      <div v-if="successMessage" class="success-message">{{ successMessage }}</div>
      <button 
        @click="changePassword" 
        :disabled="isLoading || !isValid" 
        class="change-button"
      >
        {{ isLoading ? 'Processing...' : 'Change Password' }}
      </button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue';

const currentPassword = ref('');
const newPassword = ref('');
const confirmPassword = ref('');
const passwordError = ref('');
const confirmError = ref('');
const apiError = ref('');
const successMessage = ref('');
const isLoading = ref(false);

const emit = defineEmits(['password-changed']);

const validatePassword = () => {
  if (newPassword.value.length < 8) {
    passwordError.value = 'Password must be at least 8 characters';
  } else {
    passwordError.value = '';
  }
};

const validateConfirmation = () => {
  if (newPassword.value !== confirmPassword.value) {
    confirmError.value = 'Passwords do not match';
  } else {
    confirmError.value = '';
  }
};

const isValid = computed(() => {
  return currentPassword.value && 
         newPassword.value && 
         confirmPassword.value && 
         newPassword.value === confirmPassword.value && 
         newPassword.value.length >= 8;
});

const changePassword = async () => {
  // Clear previous messages
  apiError.value = '';
  successMessage.value = '';
  
  if (!isValid.value) {
    return;
  }

  isLoading.value = true;
  
  try {
    // Get token from localStorage
    const token = localStorage.getItem('jwt_token');
    if (!token) {
      apiError.value = 'You must be logged in to change your password';
      isLoading.value = false;
      return;
    }

    const response = await fetch('/api/restricted/change-password', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`
      },
      body: JSON.stringify({
        currentPassword: currentPassword.value,
        newPassword: newPassword.value
      })
    });

    const data = await response.json();

    if (!response.ok) {
      throw new Error(data.error || 'Failed to change password');
    }

    // Clear form on success
    currentPassword.value = '';
    newPassword.value = '';
    confirmPassword.value = '';
    
    // Show success message
    successMessage.value = 'Password changed successfully!';
    
    // Emit event for parent components
    emit('password-changed');
  } catch (error) {
    console.error('Password change error:', error);
    apiError.value = error.message;
  } finally {
    isLoading.value = false;
  }
};
</script>

<style scoped>
.change-password-container {
  max-width: 400px;
  margin: 0 auto;
  padding: 20px;
}

.change-password-box {
  background-color: #424242;
  border-radius: 8px;
  padding: 20px;
  box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
}

h2 {
  color: #f0f0f0;
  margin-bottom: 20px;
  text-align: center;
}

.form-group {
  margin-bottom: 15px;
}

label {
  display: block;
  margin-bottom: 5px;
  color: #f0f0f0;
}

input {
  width: 100%;
  padding: 10px;
  border: 1px solid #555;
  border-radius: 4px;
  background-color: #333;
  color: #f0f0f0;
  font-size: 14px;
}

input:focus {
  outline: none;
  border-color: #007bff;
  box-shadow: 0 0 0 2px rgba(0, 123, 255, 0.25);
}

.change-button {
  width: 100%;
  padding: 10px;
  background-color: #007bff;
  color: white;
  border: none;
  border-radius: 4px;
  font-size: 16px;
  cursor: pointer;
  margin-top: 10px;
}

.change-button:hover:not([disabled]) {
  background-color: #0069d9;
}

.change-button:disabled {
  background-color: #6c757d;
  cursor: not-allowed;
}

.error-message {
  color: #ff6b6b;
  margin-top: 10px;
  text-align: center;
}

.success-message {
  color: #4caf50;
  margin-top: 10px;
  text-align: center;
  font-weight: bold;
}
</style>