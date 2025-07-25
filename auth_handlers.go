package main

import (
	"context"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo-contrib/session"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/pterm/pterm"
)

// JWTCustomClaims are custom claims extending default ones
type JWTCustomClaims struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// LoginRequest represents the login request body
type LoginRequest struct {
	Username string `json:"username" form:"username"`
	Password string `json:"password" form:"password"`
}

// RegisterRequest represents the registration request body
type RegisterRequest struct {
	Username string `json:"username" form:"username"`
	Password string `json:"password" form:"password"`
	Email    string `json:"email" form:"email"`
	FullName string `json:"fullName" form:"fullName"`
}

// LoginResponse represents the response from a successful login
type LoginResponse struct {
	Token               string `json:"token"`
	UserID              string `json:"userId"`
	Username            string `json:"username"`
	Role                string `json:"role"`
	ForcePasswordChange bool   `json:"forcePasswordChange"`
}

// PasswordChangeRequest represents the password change request body
type PasswordChangeRequest struct {
	CurrentPassword string `json:"currentPassword" form:"currentPassword"`
	NewPassword     string `json:"newPassword" form:"newPassword"`
}

// FirstTimePasswordChangeRequest represents the first-time password change request body
type FirstTimePasswordChangeRequest struct {
	CurrentPassword string `json:"currentPassword" form:"currentPassword"`
	NewPassword     string `json:"newPassword" form:"newPassword"`
}

// AdminSetupStatusResponse represents the response for checking admin setup status
type AdminSetupStatusResponse struct {
	NeedsSetup bool `json:"needsSetup"`
}

// AdminSetupRequest represents the request for setting up admin password
type AdminSetupRequest struct {
	Password string `json:"password"`
}

// UserResponse represents a user response without sensitive information
type UserResponse struct {
	ID                  string    `json:"id"`
	Username            string    `json:"username"`
	Email               string    `json:"email,omitempty"`
	FullName            string    `json:"fullName,omitempty"`
	Role                string    `json:"role"`
	CreatedAt           time.Time `json:"createdAt"`
	ForcePasswordChange bool      `json:"forcePasswordChange"`
}

// Global user database instance
var userDB *UserDB

// Initialize the user database and create tables
func initUserDB(config *Config) error {
	var err error
	userDB, err = NewUserDB(config.Database.ConnectionString)
	if err != nil {
		return err
	}

	// Initialize schema
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := userDB.InitSchema(ctx); err != nil {
		return err
	}

	pterm.Success.Println("User database initialized successfully.")
	return nil
}

// Configure JWT middleware
func configureJWTMiddleware(config *Config) echo.MiddlewareFunc {
	jwtConfig := echojwt.Config{
		NewClaimsFunc: func(c echo.Context) jwt.Claims {
			return new(JWTCustomClaims)
		},
		SigningKey: []byte(config.Auth.SecretKey),
	}
	return echojwt.WithConfig(jwtConfig)
}

// checkAdminSetupHandler checks if the admin user needs password setup
func checkAdminSetupHandler(c echo.Context) error {
	ctx := c.Request().Context()

	// Check if admin user exists and needs password setup
	user, err := userDB.GetUserByUsername(ctx, "admin")
	if err != nil {
		// Admin user doesn't exist, needs setup
		return c.JSON(http.StatusOK, AdminSetupStatusResponse{
			NeedsSetup: true,
		})
	}

	// Check if admin user has empty password or force password change is set
	needsSetup := user.PasswordHash == "" || user.ForcePasswordChange

	return c.JSON(http.StatusOK, AdminSetupStatusResponse{
		NeedsSetup: needsSetup,
	})
}

// setupAdminPasswordHandler handles setting the initial admin password
func setupAdminPasswordHandler(c echo.Context) error {
	// Bind request body
	req := new(AdminSetupRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid request body",
		})
	}

	// Validate password
	if req.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Password is required",
		})
	}

	if len(req.Password) < 8 {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Password must be at least 8 characters long",
		})
	}

	ctx := c.Request().Context()

	// Get admin user
	user, err := userDB.GetUserByUsername(ctx, "admin")
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error": "Admin user not found",
		})
	}

	// Only allow password setup if user has no password or force change is set
	if user.PasswordHash != "" && !user.ForcePasswordChange {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Admin password is already set",
		})
	}

	// Update admin password
	if err := userDB.UpdatePassword(ctx, user.ID, req.Password); err != nil {
		pterm.Error.Printf("Failed to set admin password: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to set password",
		})
	}

	pterm.Success.Printf("Admin password set successfully\n")

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Admin password set successfully",
	})
}

// loginHandler handles user login and returns JWT token
func loginHandler(c echo.Context) error {
	// Bind request body to LoginRequest struct
	loginReq := new(LoginRequest)
	if err := c.Bind(loginReq); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if loginReq.Username == "" || loginReq.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Username and password are required",
		})
	}

	// Get user from database
	ctx := c.Request().Context()
	user, err := userDB.GetUserByUsername(ctx, loginReq.Username)
	if err != nil {
		pterm.Debug.Printf("Login failed for user %s: %v\n", loginReq.Username, err)
		return c.JSON(http.StatusUnauthorized, map[string]interface{}{
			"error": "Invalid username or password",
		})
	}

	// Verify password
	if !VerifyPassword(user.PasswordHash, loginReq.Password) {
		pterm.Debug.Printf("Invalid password for user %s\n", loginReq.Username)
		return c.JSON(http.StatusUnauthorized, map[string]interface{}{
			"error": "Invalid username or password",
		})
	}

	// Get config from context
	config := c.Get("config").(*Config)

	// Set token expiration time based on config
	expirationTime := time.Now().Add(time.Duration(config.Auth.TokenExpiry) * time.Hour)

	// Set custom claims
	claims := &JWTCustomClaims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	// Create token with claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Generate encoded token using the secret signing key from config
	tokenString, err := token.SignedString([]byte(config.Auth.SecretKey))
	if err != nil {
		pterm.Error.Printf("Error generating token: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Could not generate token",
		})
	}

	// Also store in session for frontend access
	sess, err := session.Get("manifold-session", c)
	if err == nil {
		sess.Values["jwt"] = tokenString
		sess.Values["isLoggedIn"] = true
		sess.Values["username"] = user.Username
		sess.Values["userId"] = user.ID
		sess.Values["role"] = user.Role
		sess.Values["forcePasswordChange"] = user.ForcePasswordChange
		sess.Save(c.Request(), c.Response())
	}

	// Return the JWT token and user info
	return c.JSON(http.StatusOK, LoginResponse{
		Token:               tokenString,
		UserID:              user.ID,
		Username:            user.Username,
		Role:                user.Role,
		ForcePasswordChange: user.ForcePasswordChange,
	})
}

// registerHandler handles user registration
func registerHandler(c echo.Context) error {
	// Bind request body to RegisterRequest struct
	registerReq := new(RegisterRequest)
	if err := c.Bind(registerReq); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if registerReq.Username == "" || registerReq.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Username and password are required",
		})
	}

	// Create user in database
	ctx := c.Request().Context()
	user, err := userDB.CreateUser(ctx, registerReq.Username, registerReq.Password, registerReq.Email, registerReq.FullName)
	if err != nil {
		pterm.Error.Printf("User registration failed: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusCreated, UserResponse{
		ID:                  user.ID,
		Username:            user.Username,
		Email:               user.Email,
		FullName:            user.FullName,
		Role:                user.Role,
		CreatedAt:           user.CreatedAt,
		ForcePasswordChange: user.ForcePasswordChange,
	})
}

// getUserInfoHandler returns information about the currently authenticated user
func getUserInfoHandler(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTCustomClaims)

	// Get user details from database
	ctx := c.Request().Context()
	dbUser, err := userDB.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to retrieve user information",
		})
	}

	return c.JSON(http.StatusOK, UserResponse{
		ID:                  dbUser.ID,
		Username:            dbUser.Username,
		Email:               dbUser.Email,
		FullName:            dbUser.FullName,
		Role:                dbUser.Role,
		CreatedAt:           dbUser.CreatedAt,
		ForcePasswordChange: dbUser.ForcePasswordChange,
	})
}

// changePasswordHandler allows a user to change their password
func changePasswordHandler(c echo.Context) error {
	// Get the user claims from the JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTCustomClaims)
	userID := claims.UserID

	// Bind request body
	req := new(PasswordChangeRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid request body",
		})
	}

	// Validate request
	if req.CurrentPassword == "" || req.NewPassword == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Current password and new password are required",
		})
	}

	// If new password is too short
	if len(req.NewPassword) < 8 {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "New password must be at least 8 characters long",
		})
	}

	// Get user from database
	ctx := c.Request().Context()
	dbUser, err := userDB.GetUserByID(ctx, userID)
	if err != nil {
		pterm.Error.Printf("Failed to retrieve user with ID %s: %v\n", userID, err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to retrieve user information",
		})
	}

	// Verify current password
	if !VerifyPassword(dbUser.PasswordHash, req.CurrentPassword) {
		return c.JSON(http.StatusUnauthorized, map[string]interface{}{
			"error": "Current password is incorrect",
		})
	}

	// Update password in database
	if err := userDB.UpdatePassword(ctx, userID, req.NewPassword); err != nil {
		pterm.Error.Printf("Failed to update password for user %s: %v\n", userID, err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to update password",
		})
	}

	pterm.Success.Printf("Password changed successfully for user %s\n", claims.Username)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Password changed successfully",
	})
}

// firstTimePasswordChangeHandler allows a user to change their password for the first time
func firstTimePasswordChangeHandler(c echo.Context) error {
	// Get the user claims from the JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTCustomClaims)
	userID := claims.UserID

	// Bind request body
	req := new(FirstTimePasswordChangeRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid request body",
		})
	}

	// Validate request
	if req.CurrentPassword == "" || req.NewPassword == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Current password and new password are required",
		})
	}

	// If new password is too short
	if len(req.NewPassword) < 8 {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "New password must be at least 8 characters long",
		})
	}

	// Get user from database
	ctx := c.Request().Context()
	dbUser, err := userDB.GetUserByID(ctx, userID)
	if err != nil {
		pterm.Error.Printf("Failed to retrieve user with ID %s: %v\n", userID, err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to retrieve user information",
		})
	}

	// Verify current password
	if !VerifyPassword(dbUser.PasswordHash, req.CurrentPassword) {
		return c.JSON(http.StatusUnauthorized, map[string]interface{}{
			"error": "Current password is incorrect",
		})
	}

	// Update password in database and clear force change flag
	if err := userDB.UpdatePassword(ctx, userID, req.NewPassword); err != nil {
		pterm.Error.Printf("Failed to update password for user %s: %v\n", userID, err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to update password",
		})
	}

	pterm.Success.Printf("First-time password changed successfully for user %s\n", claims.Username)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Password changed successfully",
	})
}

// logoutHandler handles user logout
func logoutHandler(c echo.Context) error {
	// Clear session
	sess, err := session.Get("manifold-session", c)
	if err == nil {
		sess.Values = make(map[interface{}]interface{})
		sess.Save(c.Request(), c.Response())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Logged out successfully",
	})
}

// restrictedHandler is a sample protected route
func restrictedHandler(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTCustomClaims)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Access granted",
		"user":    claims.Username,
		"role":    claims.Role,
	})
}
