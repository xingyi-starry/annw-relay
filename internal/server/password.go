package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"

	"crypto/pbkdf2"
)

const passwordIterations = 210_000

type passwordVerifier struct {
	salt []byte
	hash []byte
}

func derivePassword(password string) (passwordVerifier, error) {
	if password == "" {
		return passwordVerifier{}, nil
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return passwordVerifier{}, err
	}
	hash, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
	if err != nil {
		return passwordVerifier{}, err
	}
	return passwordVerifier{salt: salt, hash: hash}, nil
}

func (v passwordVerifier) enabled() bool { return len(v.hash) != 0 }

func (v passwordVerifier) matches(password string) bool {
	if !v.enabled() {
		return password == ""
	}
	candidate, err := pbkdf2.Key(sha256.New, password, v.salt, passwordIterations, len(v.hash))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(candidate, v.hash) == 1
}
