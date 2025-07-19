package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
)

// UserDB represents a database connection for user operations
type UserDB struct {
	db *sql.DB
}

// User represents a user in the database
type User struct {
	ID                  string    `json:"id"`
	Username            string    `json:"username"`
	Email               string    `json:"email,omitempty"`
	PasswordHash        string    `json:"-"` // Never expose in JSON
	FullName            string    `json:"fullName,omitempty"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
	Role                string    `json:"role"`
	ForcePasswordChange bool      `json:"forcePasswordChange"`
}

// NewUserDB creates a new UserDB instance
func NewUserDB(connStr string) (*UserDB, error) {
	connConfig, err := pgx.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	// Use stdlib to get a standard sql.DB connection
	db := stdlib.OpenDB(*connConfig)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &UserDB{db: db}, nil
}

// InitSchema initializes the database schema for users
func (udb *UserDB) InitSchema(ctx context.Context) error {
	_, err := udb.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			email TEXT UNIQUE,
			password_hash TEXT NOT NULL,
			full_name TEXT,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			role TEXT DEFAULT 'user',
			force_password_change BOOLEAN DEFAULT FALSE
		)
	`)
	return err
}

// Close closes the database connection
func (udb *UserDB) Close() error {
	return udb.db.Close()
}

// CreateUser creates a new user in the database
func (udb *UserDB) CreateUser(ctx context.Context, username, password, email, fullName string) (*User, error) {
	// Check if username already exists
	var exists bool
	err := udb.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)", username).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("error checking username: %w", err)
	}
	if exists {
		return nil, errors.New("username already exists")
	}

	// Check if email exists if provided
	if email != "" {
		err = udb.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)", email).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("error checking email: %w", err)
		}
		if exists {
			return nil, errors.New("email already exists")
		}
	}

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("error hashing password: %w", err)
	}

	// Generate a unique ID for the user
	id := generateUniqueID()

	// Insert the new user
	now := time.Now().UTC()
	_, err = udb.db.ExecContext(ctx, `
		INSERT INTO users (id, username, email, password_hash, full_name, created_at, updated_at, role, force_password_change) 
		VALUES ($1, $2, $3, $4, $5, $6, $6, $7, $8)`,
		id, username, email, string(hashedPassword), fullName, now, "user", false,
	)
	if err != nil {
		return nil, fmt.Errorf("error creating user: %w", err)
	}

	return &User{
		ID:                  id,
		Username:            username,
		Email:               email,
		PasswordHash:        string(hashedPassword),
		FullName:            fullName,
		CreatedAt:           now,
		UpdatedAt:           now,
		Role:                "user",
		ForcePasswordChange: false,
	}, nil
}

// CreateUserWithoutPassword creates a new user in the database without a password (for initial setup)
func (udb *UserDB) CreateUserWithoutPassword(ctx context.Context, username, email, fullName string) (*User, error) {
	// Check if username already exists
	var exists bool
	err := udb.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)", username).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("error checking username: %w", err)
	}
	if exists {
		return nil, errors.New("username already exists")
	}

	// Check if email exists if provided
	if email != "" {
		err = udb.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)", email).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("error checking email: %w", err)
		}
		if exists {
			return nil, errors.New("email already exists")
		}
	}

	// Generate a unique ID for the user
	id := generateUniqueID()

	// Insert the new user without password hash
	now := time.Now().UTC()
	_, err = udb.db.ExecContext(ctx, `
		INSERT INTO users (id, username, email, password_hash, full_name, created_at, updated_at, role, force_password_change) 
		VALUES ($1, $2, $3, '', $4, $5, $5, $6, TRUE)`,
		id, username, email, fullName, now, "user",
	)
	if err != nil {
		return nil, fmt.Errorf("error creating user: %w", err)
	}

	return &User{
		ID:                  id,
		Username:            username,
		Email:               email,
		PasswordHash:        "",
		FullName:            fullName,
		CreatedAt:           now,
		UpdatedAt:           now,
		Role:                "user",
		ForcePasswordChange: true,
	}, nil
}

// GetUserByUsername retrieves a user from the database by username
func (udb *UserDB) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := udb.db.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, full_name, created_at, updated_at, role, force_password_change 
		FROM users WHERE username = $1`,
		username,
	).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.FullName,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.Role,
		&user.ForcePasswordChange,
	)

	if err == sql.ErrNoRows {
		return nil, errors.New("user not found")
	} else if err != nil {
		return nil, fmt.Errorf("error getting user: %w", err)
	}

	return &user, nil
}

// GetUserByID retrieves a user from the database by ID
func (udb *UserDB) GetUserByID(ctx context.Context, id string) (*User, error) {
	var user User
	err := udb.db.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, full_name, created_at, updated_at, role, force_password_change 
		FROM users WHERE id = $1`,
		id,
	).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.FullName,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.Role,
		&user.ForcePasswordChange,
	)

	if err == sql.ErrNoRows {
		return nil, errors.New("user not found")
	} else if err != nil {
		return nil, fmt.Errorf("error getting user: %w", err)
	}

	return &user, nil
}

// UpdateUser updates a user's information in the database
func (udb *UserDB) UpdateUser(ctx context.Context, id string, email, fullName string) error {
	_, err := udb.db.ExecContext(ctx, `
		UPDATE users 
		SET email = $2, full_name = $3, updated_at = NOW() 
		WHERE id = $1`,
		id, email, fullName,
	)
	return err
}

// UpdatePassword updates a user's password in the database
func (udb *UserDB) UpdatePassword(ctx context.Context, id string, newPassword string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("error hashing password: %w", err)
	}

	_, err = udb.db.ExecContext(ctx, `
		UPDATE users 
		SET password_hash = $2, force_password_change = FALSE, updated_at = NOW() 
		WHERE id = $1`,
		id, string(hashedPassword),
	)
	return err
}

// SetForcePasswordChange updates the force_password_change flag for a user
func (udb *UserDB) SetForcePasswordChange(ctx context.Context, id string, force bool) error {
	_, err := udb.db.ExecContext(ctx, `
		UPDATE users 
		SET force_password_change = $2, updated_at = NOW() 
		WHERE id = $1`,
		id, force,
	)
	return err
}

// VerifyPassword checks if the provided password matches the stored hash
func VerifyPassword(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// generateUniqueID generates a unique ID for a user
func generateUniqueID() string {
	return fmt.Sprintf("user_%d", time.Now().UnixNano())
}
