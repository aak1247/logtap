package store

import (
	"context"
	"errors"
	"strings"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/gorm"
)

type UserRow struct {
	ID           int64
	Email        string
	PasswordHash string
}

func CountUsers(ctx context.Context, db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, nil
	}
	var n int64
	err := db.WithContext(ctx).Model(&model.User{}).Count(&n).Error
	return n, err
}

func CreateUser(ctx context.Context, db *gorm.DB, email string, passwordHash string) (int64, error) {
	if db == nil {
		return 0, nil
	}
	email = strings.ToLower(strings.TrimSpace(email))
	u := model.User{Email: email, PasswordHash: passwordHash}
	if err := db.WithContext(ctx).Create(&u).Error; err != nil {
		return 0, err
	}
	return u.ID, nil
}

func GetUserByEmail(ctx context.Context, db *gorm.DB, email string) (UserRow, bool, error) {
	if db == nil {
		return UserRow{}, false, nil
	}
	email = strings.ToLower(strings.TrimSpace(email))
	var u model.User
	err := db.WithContext(ctx).Where("email = ?", email).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return UserRow{}, false, nil
		}
		return UserRow{}, false, err
	}
	return UserRow{ID: u.ID, Email: u.Email, PasswordHash: u.PasswordHash}, true, nil
}

func GetUserByID(ctx context.Context, db *gorm.DB, id int64) (UserRow, bool, error) {
	if db == nil || id <= 0 {
		return UserRow{}, false, nil
	}
	var u model.User
	err := db.WithContext(ctx).Where("id = ?", id).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return UserRow{}, false, nil
		}
		return UserRow{}, false, err
	}
	return UserRow{ID: u.ID, Email: u.Email, PasswordHash: u.PasswordHash}, true, nil
}
