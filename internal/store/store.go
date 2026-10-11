package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrEmailTaken     = errors.New("email already registered")
	uniqueViolation   = "23505"
	defaultPingTimout = 5 * time.Second
)

type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	IsAdmin      bool      `json:"is_admin"`
	CreatedAt    time.Time `json:"created_at"`
}

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, defaultPingTimout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *Store) CreateUser(ctx context.Context, email, name, passwordHash string, isAdmin bool) (*User, error) {
	const q = `
		INSERT INTO users (email, name, password_hash, is_admin)
		VALUES ($1, $2, $3, $4)
		RETURNING id, email, name, password_hash, is_admin, created_at`
	var u User
	err := s.pool.QueryRow(ctx, q, email, name, passwordHash, isAdmin).
		Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return nil, ErrEmailTaken
		}
		return nil, err
	}
	return &u, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	const q = `SELECT id, email, name, password_hash, is_admin, created_at FROM users WHERE email = $1`
	return s.scanUser(s.pool.QueryRow(ctx, q, email))
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	const q = `SELECT id, email, name, password_hash, is_admin, created_at FROM users WHERE id = $1`
	return s.scanUser(s.pool.QueryRow(ctx, q, id))
}

func (s *Store) scanUser(row pgx.Row) (*User, error) {
	var u User
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}
