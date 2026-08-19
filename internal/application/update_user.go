package application

import (
	"context"

	"golang.org/x/crypto/bcrypt"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// UpdateUserProfile changes email and/or password -- both are optional
// (pass "" to leave a field unchanged), so a single PATCH-style endpoint can
// cover "just change my email" and "just change my password" without two
// separate use cases.
func UpdateUserProfile(ctx context.Context, userRepo ports.UserRepository, userID, newEmail, newPlaintextPassword string) (*domain.User, error) {
	user, err := userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if newEmail != "" {
		emailVO, err := domain.NewEmail(newEmail)
		if err != nil {
			return nil, err
		}
		if err := user.ChangeEmail(emailVO); err != nil {
			return nil, err
		}
	}

	if newPlaintextPassword != "" {
		hashed, err := bcrypt.GenerateFromPassword([]byte(newPlaintextPassword), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		passwordHash, err := domain.NewPasswordHash(string(hashed))
		if err != nil {
			return nil, err
		}
		if err := user.ChangePassword(passwordHash); err != nil {
			return nil, err
		}
	}

	if err := userRepo.Save(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// DeleteUser soft-deletes the account (domain.User.Delete) -- past actions
// tied to this user's ID (e.g. Script.reviewerID) keep pointing at a real,
// if now-deleted, account instead of a dangling reference.
func DeleteUser(ctx context.Context, userRepo ports.UserRepository, userID string) error {
	user, err := userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := user.Delete(); err != nil {
		return err
	}
	return userRepo.Save(ctx, user)
}
