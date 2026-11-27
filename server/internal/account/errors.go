package account

import "errors"

var (
	ErrNotFound       = errors.New("account: not found")
	ErrIPLimit        = errors.New("account: ip registration limit reached")
	ErrPhoneExists    = errors.New("account: phone already bound")
	ErrInvalidCode    = errors.New("account: invalid wx code")
	ErrUserBlocked    = errors.New("account: user is blocked")
	ErrInvalidRefresh = errors.New("account: invalid refresh token")
	ErrTokenRevoked   = errors.New("account: token revoked")
)
