<template>
  <div class="first-login-container">
    <div class="first-login-box">
      <div class="logo-section">
        <h1>Manifold</h1>
        <div class="welcome-message">
          <h2>Welcome, {{ username }}!</h2>
          <p>For security reasons, you must set a new password before you can continue.</p>
        </div>
      </div>
      
      <form @submit.prevent="changePassword">
        <div class="form-group">
          <label for="currentPassword">Current Password</label>
          <input 
            type="password" 
            id="currentPassword" 
            v-model="currentPassword" 
            placeholder="Enter your current password"
            required
          >
        </div>
        
        <div class="form-group">
          <label for="newPassword">New Password</label>
          <input 
            type="password" 
            id="newPassword" 
            v-model="newPassword" 
            placeholder="Enter your new password (minimum 8 characters)"
            required
            @input="validatePassword"
          >
          <div v-if="passwordError" class="error-message">{{ passwordError }}</div>
        </div>
        
        <div class="form-group">
          <label for="confirmPassword">Confirm New Password</label>
          <input 
            type="password" 
            id="confirmPassword" 
            v-model="confirmPassword" 
            placeholder="Confirm your new password"
            required
            @input="validateConfirmation"
          >
          <div v-if="confirmError" class="error-message">{{ confirmError }}</div>
        </div>
        
        <div v-if="apiError" class="error-message error-box">{{ apiError }}</div>
        
        <button 
          type="submit"
          :disabled="isLoading || !isValid" 
          class="change-password-btn"
        >
          {{ isLoading ? 'Setting Password...' : 'Set New Password' }}
        </button>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue';

const props = defineProps({
  username: {
    type: String,
    required: true
  }
});

const emit = defineEmits(['password-changed']);

const currentPassword = ref('');
const newPassword = ref('');
const confirmPassword = ref('');
const passwordError = ref('');
const confirmError = ref('');
const apiError = ref('');
const isLoading = ref(false);

const validatePassword = () => {
  if (newPassword.value.length > 0 && newPassword.value.length < 8) {
    passwordError.value = 'Password must be at least 8 characters';
  } else {
    passwordError.value = '';
  }
};

const validateConfirmation = () => {
  if (confirmPassword.value && newPassword.value !== confirmPassword.value) {
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
  // Clear previous errors
  apiError.value = '';
  
  if (!isValid.value) {
    return;
  }

  isLoading.value = true;
  
  try {
    // Get token from localStorage
    const token = localStorage.getItem('jwt_token');
    if (!token) {
      throw new Error('Authentication required');
    }

    const response = await fetch('/api/restricted/first-time-password-change', {
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
    
    // Emit event for parent components to handle successful password change
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
.first-login-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  background-color: #303030;
  padding: 20px;
}

.first-login-box {
  background-color: #424242;
  border-radius: 8px;
  padding: 2rem;
  width: 100%;
  max-width: 450px;
  box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
}

.logo-section {
  text-align: center;
  margin-bottom: 2rem;
}

.logo-section h1 {
  color: #f0f0f0;
  margin-bottom: 1rem;
  font-size: 2rem;
  font-weight: bold;
}

.welcome-message h2 {
  color: #4caf50;
  margin-bottom: 0.5rem;
  font-size: 1.5rem;
}

.welcome-message p {
  color: #f0f0f0;
  margin-bottom: 0;
  line-height: 1.5;
  opacity: 0.9;
}

.form-group {
  margin-bottom: 1.5rem;
}

label {
  display: block;
  margin-bottom: 0.5rem;
  color: #f0f0f0;
  font-weight: 500;
}

input {
  width: 100%;
  padding: 0.75rem;
  border: 1px solid #555;
  border-radius: 4px;
  background-color: #333;
  color: #f0f0f0;
  font-size: 1rem;
  transition: border-color 0.3s ease;
}

input:focus {
  outline: none;
  border-color: #007bff;
  box-shadow: 0 0 0 2px rgba(0, 123, 255, 0.25);
}

.change-password-btn {
  width: 100%;
  padding: 0.75rem;
  background-color: #007bff;
  color: white;
  border: none;
  border-radius: 4px;
  font-size: 1rem;
  font-weight: 500;
  cursor: pointer;
  margin-top: 1rem;
  transition: background-color 0.3s ease;
}

.change-password-btn:hover:not([disabled]) {
  background-color: #0069d9;
}

.change-password-btn:disabled {
  background-color: #6c757d;
  cursor: not-allowed;
  opacity: 0.7;
}

.error-message {
  color: #ff6b6b;
  font-size: 0.875rem;
  margin-top: 0.25rem;
}

.error-box {
  background-color: rgba(255, 107, 107, 0.1);
  border: 1px solid rgba(255, 107, 107, 0.3);
  border-radius: 4px;
  padding: 0.75rem;
  margin-bottom: 1rem;
  text-align: center;
}
</style>
