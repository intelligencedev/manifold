<template>
  <div class="admin-setup-container">
    <div class="admin-setup-box">
      <div class="logo-section">
        <h1>Manifold</h1>
        <h2>Initial Admin Setup</h2>
        <p class="setup-description">
          Welcome! Please set your admin password to complete the setup.
        </p>
      </div>
      
      <form @submit.prevent="setupPassword">
        <div class="form-group">
          <label for="newPassword">Admin Password</label>
          <input 
            type="password" 
            id="newPassword" 
            v-model="newPassword" 
            placeholder="Enter your admin password"
            required
            minlength="8"
            @input="validatePassword"
          >
          <small class="password-help">Password must be at least 8 characters long</small>
        </div>
        
        <div class="form-group">
          <label for="confirmPassword">Confirm Password</label>
          <input 
            type="password" 
            id="confirmPassword" 
            v-model="confirmPassword" 
            placeholder="Confirm your admin password"
            required
            minlength="8"
            @input="validateConfirmation"
          >
        </div>
        
        <div v-if="passwordError" class="error-message">{{ passwordError }}</div>
        <div v-if="confirmError" class="error-message">{{ confirmError }}</div>
        <div v-if="apiError" class="error-message">{{ apiError }}</div>
        <div v-if="successMessage" class="success-message">{{ successMessage }}</div>
        
        <button 
          type="submit"
          :disabled="isLoading || !isValid" 
          class="setup-button"
        >
          {{ isLoading ? 'Setting up...' : 'Set Admin Password' }}
        </button>
      </form>
      
      <div class="security-notice">
        <p><strong>Security Notice:</strong> This password will be used to access the Manifold admin interface. Choose a strong password and store it securely.</p>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue';

const newPassword = ref('');
const confirmPassword = ref('');
const passwordError = ref('');
const confirmError = ref('');
const apiError = ref('');
const successMessage = ref('');
const isLoading = ref(false);

const emit = defineEmits(['setup-complete']);

const validatePassword = () => {
  if (newPassword.value.length > 0 && newPassword.value.length < 8) {
    passwordError.value = 'Password must be at least 8 characters long';
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
  return newPassword.value && 
         confirmPassword.value && 
         newPassword.value === confirmPassword.value && 
         newPassword.value.length >= 8 &&
         !passwordError.value &&
         !confirmError.value;
});

const setupPassword = async () => {
  // Clear previous messages
  apiError.value = '';
  successMessage.value = '';
  
  if (!isValid.value) {
    return;
  }

  isLoading.value = true;
  
  try {
    const response = await fetch('/api/auth/admin-setup', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        password: newPassword.value
      })
    });

    const data = await response.json();

    if (!response.ok) {
      throw new Error(data.error || 'Failed to set admin password');
    }

    // Show success message
    successMessage.value = 'Admin password set successfully! Redirecting to login...';
    
    // Clear form
    newPassword.value = '';
    confirmPassword.value = '';
    
    // Emit event after a short delay to show success message
    setTimeout(() => {
      emit('setup-complete');
    }, 2000);
    
  } catch (error) {
    console.error('Admin setup error:', error);
    apiError.value = error.message || 'Failed to set admin password. Please try again.';
  } finally {
    isLoading.value = false;
  }
};
</script>

<style scoped>
.admin-setup-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  background-color: #303030;
  padding: 20px;
}

.admin-setup-box {
  background-color: #424242;
  border-radius: 8px;
  padding: 2rem;
  width: 100%;
  max-width: 400px;
  box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
}

.logo-section {
  text-align: center;
  margin-bottom: 2rem;
}

h1 {
  color: #f0f0f0;
  margin-bottom: 0.5rem;
  font-size: 2rem;
  font-weight: bold;
}

h2 {
  color: #38b2ac;
  margin-bottom: 1rem;
  font-size: 1.5rem;
}

.setup-description {
  color: #ccc;
  font-size: 0.9rem;
  line-height: 1.4;
  margin-bottom: 0;
}

.form-group {
  margin-bottom: 1rem;
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
}

input:focus {
  outline: none;
  border-color: #38b2ac;
  box-shadow: 0 0 0 2px rgba(56, 178, 172, 0.25);
}

.password-help {
  display: block;
  margin-top: 0.25rem;
  color: #999;
  font-size: 0.8rem;
}

.setup-button {
  width: 100%;
  padding: 0.75rem;
  background-color: #38b2ac;
  color: white;
  border: none;
  border-radius: 4px;
  font-size: 1rem;
  font-weight: 500;
  cursor: pointer;
  margin-top: 1rem;
  transition: background-color 0.2s;
}

.setup-button:hover:not([disabled]) {
  background-color: #319795;
}

.setup-button:disabled {
  background-color: #6c757d;
  cursor: not-allowed;
  opacity: 0.6;
}

.error-message {
  color: #ff6b6b;
  margin-top: 0.5rem;
  font-size: 0.9rem;
}

.success-message {
  color: #4caf50;
  margin-top: 0.5rem;
  font-size: 0.9rem;
  font-weight: 500;
}

.security-notice {
  margin-top: 2rem;
  padding: 1rem;
  background-color: rgba(56, 178, 172, 0.1);
  border-left: 3px solid #38b2ac;
  border-radius: 4px;
}

.security-notice p {
  color: #ccc;
  font-size: 0.8rem;
  line-height: 1.4;
  margin: 0;
}

.security-notice strong {
  color: #38b2ac;
}
</style>
