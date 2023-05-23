// SPDX-License-Identifier: ice License 1.0

package emaillink

import (
	"context"
	stdlibtime "time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/wintr/connectors/storage/v2"
	time "github.com/ice-blockchain/wintr/time"
)

func (r *repository) generateRefreshToken(now *time.Time, userID users.UserID, email string, seq int64) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Token{
		RegisteredClaims: &jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(r.cfg.RefreshExpirationTime)),
			NotBefore: jwt.NewNumericDate(*now.Time),
			IssuedAt:  jwt.NewNumericDate(*now.Time),
		},
		EMail: email,
		Seq:   seq,
	})

	tokenStr, err := token.SignedString([]byte(r.cfg.JWTSecret))

	return tokenStr, errors.Wrapf(err, "failed to generate refresh token for user %v %v", userID, email)
}

func (r *repository) RenewRefreshToken(ctx context.Context, previousRefreshToken string) (string, error) {
	userID, email, seq, err := r.parseToken(previousRefreshToken)
	if err != nil {
		return "", errors.Wrapf(err, "failed to verify token")
	}
	now := time.Now()
	refreshTokenSeq, err := r.updateRefreshTokenForUser(ctx, userID, seq, now, email)
	if err != nil {
		if storage.IsErr(err, storage.ErrNotFound) {
			return "", errors.Wrapf(ErrInvalidToken, "refreshToken with wrong sequence provided")
		}

		return "", errors.Wrapf(err, "failed to update pending confirmation for email:%v", email)
	}

	return r.generateRefreshToken(now, userID, email, refreshTokenSeq)
}

func (r *repository) IssueAccessToken(ctx context.Context, refreshToken string) (string, error) {
	userID, email, seq, err := r.parseToken(refreshToken)
	if err != nil {
		return "", errors.Wrapf(err, "failed to verify token")
	}
	user, err := r.getUserByID(ctx, userID)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get user by id %v", userID)
	}
	if user.Email != email {
		return "", errors.Wrapf(ErrUserDataMismatch, "user's email %v does not match token's email %v", user.Email, email)
	}

	return r.generateAccessToken(seq, user)
}
func (r *repository) generateAccessToken(refreshTokenSeq int64, user *users.User) (string, error) {
	now := time.Now().In(stdlibtime.UTC)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Token{
		RegisteredClaims: &jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Subject:   user.ID,
			ExpiresAt: jwt.NewNumericDate(now.Add(r.cfg.AccessExpirationTime)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		EMail:    user.Email,
		HashCode: user.HashCode,
		Role:     "app",
		Seq:      refreshTokenSeq,
	})

	tokenStr, err := token.SignedString([]byte(r.cfg.JWTSecret))

	return tokenStr, errors.Wrapf(err, "failed to generate access token for userID %v and email %v", user.ID, user.Email)
}
