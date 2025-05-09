// session_handlers.go
package main

import (
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
)

// SessionData represents the structure of data stored in a session
type SessionData struct {
	UserID     string                 `json:"userId,omitempty"`
	Username   string                 `json:"username,omitempty"`
	IsLoggedIn bool                   `json:"isLoggedIn"`
	Data       map[string]interface{} `json:"data,omitempty"`
}

// createSessionHandler creates a new session or modifies an existing one
func createSessionHandler(c echo.Context) error {
	// Get session
	sess, err := session.Get("manifold-session", c)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to get session: " + err.Error(),
		})
	}

	// Parse request body
	var sessionData SessionData
	if err := c.Bind(&sessionData); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid request body",
		})
	}

	// Store values in session
	if sessionData.UserID != "" {
		sess.Values["userId"] = sessionData.UserID
	}
	if sessionData.Username != "" {
		sess.Values["username"] = sessionData.Username
	}
	sess.Values["isLoggedIn"] = sessionData.IsLoggedIn

	// Store any additional data
	if sessionData.Data != nil {
		for key, value := range sessionData.Data {
			sess.Values[key] = value
		}
	}

	// Save session
	if err := sess.Save(c.Request(), c.Response()); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to save session: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Session created successfully",
	})
}

// getSessionHandler retrieves current session data
func getSessionHandler(c echo.Context) error {
	// Get session
	sess, err := session.Get("manifold-session", c)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to get session: " + err.Error(),
		})
	}

	// Check if session exists
	if sess.IsNew {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"exists": false,
			"data":   nil,
		})
	}

	// Prepare response
	response := SessionData{
		UserID:     getSessionString(sess, "userId"),
		Username:   getSessionString(sess, "username"),
		IsLoggedIn: getSessionBool(sess, "isLoggedIn"),
		Data:       make(map[string]interface{}),
	}

	// Get all other values
	for key, value := range sess.Values {
		if k, ok := key.(string); ok {
			if k != "userId" && k != "username" && k != "isLoggedIn" {
				response.Data[k] = value
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"exists": true,
		"data":   response,
	})
}

// destroySessionHandler deletes the current session
func destroySessionHandler(c echo.Context) error {
	// Get session
	sess, err := session.Get("manifold-session", c)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to get session: " + err.Error(),
		})
	}

	// Delete session by setting MaxAge to -1
	sess.Options.MaxAge = -1

	// Save to apply changes
	if err := sess.Save(c.Request(), c.Response()); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to destroy session: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Session destroyed successfully",
	})
}

// Helper functions to safely get typed values from session
func getSessionString(sess *sessions.Session, key string) string {
	if val, ok := sess.Values[key]; ok {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return ""
}

func getSessionBool(sess *sessions.Session, key string) bool {
	if val, ok := sess.Values[key]; ok {
		if boolVal, ok := val.(bool); ok {
			return boolVal
		}
	}
	return false
}
