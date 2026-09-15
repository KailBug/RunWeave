// Package identity defines independent principal and node credentials.
package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"

	"runweave/internal/contract"
)

type Role string

const (
	Principal Role = "principal"
	Node      Role = "node"
)

var ErrUnauthorized = errors.New("unauthorized")

type Subject struct {
	Role Role   `json:"role"`
	ID   string `json:"id"`
}

func (s Subject) Valid() bool {
	return (s.Role == Principal || s.Role == Node) && contract.ValidIdentifier(s.ID)
}

func prefix(role Role) string {
	switch role {
	case Principal:
		return "rw_p_"
	case Node:
		return "rw_n_"
	}
	return ""
}

func NewToken(role Role) (string, error) {
	p := prefix(role)
	if p == "" {
		return "", ErrUnauthorized
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return p + base64.RawURLEncoding.EncodeToString(random[:]), nil
}

func Digest(token string, role Role) ([]byte, error) {
	p := prefix(role)
	if p == "" || len(token) != 48 || token[:5] != p {
		return nil, ErrUnauthorized
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token[5:])
	if err != nil || len(raw) != 32 {
		return nil, ErrUnauthorized
	}
	sum := sha256.Sum256([]byte(token))
	return sum[:], nil
}
