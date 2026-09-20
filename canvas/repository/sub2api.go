package repository

import (
	"errors"

	"github.com/tigerowo/infinite-canvas/model"
	"gorm.io/gorm/clause"
)

func ConsumeSSOTicket(digest string, expiresAt, now int64) (bool, error) {
	db, err := DB()
	if err != nil {
		return false, err
	}
	if err := db.Where("expires_at <= ?", now).Delete(&model.SSOTicket{}).Error; err != nil {
		return false, err
	}
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.SSOTicket{Digest: digest, ExpiresAt: expiresAt})
	return result.RowsAffected == 1, result.Error
}

// Deterministic IDs and INSERT ON CONFLICT make simultaneous first logins converge.
func FindOrCreateSub2APIUser(candidate model.User) (model.User, error) {
	db, err := DB()
	if err != nil {
		return model.User{}, err
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
		return model.User{}, err
	}
	user, ok, err := GetUserByID(candidate.ID)
	if err != nil {
		return user, err
	}
	if !ok || user.Sub2APISubject != candidate.Sub2APISubject {
		return model.User{}, errors.New("SSO identity conflict")
	}
	return user, nil
}

func RecordSub2APILogin(id, now string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.User{}).Where("id = ?", id).Updates(map[string]any{"last_login_at": now, "updated_at": now}).Error
}
