package store

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

var ErrBadLogin = errors.New("invalid username or password")

// CheckLogin reports nil when the username and password match.
func (s *Store) CheckLogin(user, pass string) error {
	var hash string
	if err := s.db.QueryRow(`SELECT pass_hash FROM users WHERE username = ?`, user).Scan(&hash); err != nil {
		return ErrBadLogin
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(pass)) != nil {
		return ErrBadLogin
	}
	return nil
}

// SetPassword changes a user's password after checking the current one.
func (s *Store) SetPassword(user, oldPass, newPass string) error {
	if len(newPass) < 4 {
		return errors.New("the new password must be at least 4 characters")
	}
	if err := s.CheckLogin(user, oldPass); err != nil {
		return errors.New("the current password is wrong")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE users SET pass_hash = ? WHERE username = ?`, string(hash), user)
	return err
}
