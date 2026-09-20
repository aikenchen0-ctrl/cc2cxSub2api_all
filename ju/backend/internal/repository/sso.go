package repository

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"infinite-canvas/backend/internal/model"
)

func (r *Repository) ConsumeSSOTicket(digest string, expiresAt, now time.Time) error {
	if err := r.db.Where("expires_at <= ?", now).Delete(&model.SSOTicket{}).Error; err != nil {
		return err
	}
	result := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.SSOTicket{ID: digest, ExpiresAt: expiresAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("sso ticket already used")
	}
	return nil
}

func (r *Repository) CreateSSOHandoff(handoff *model.SSOHandoff, now time.Time) error {
	if err := r.db.Where("expires_at <= ?", now).Delete(&model.SSOHandoff{}).Error; err != nil {
		return err
	}
	return r.db.Create(handoff).Error
}

func (r *Repository) ConsumeSSOHandoff(digest string, now time.Time) (*model.SSOHandoff, error) {
	var handoff model.SSOHandoff
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&handoff, "id = ? AND expires_at > ?", digest, now).Error; err != nil {
			return err
		}
		result := tx.Where("id = ? AND expires_at > ?", digest, now).Delete(&model.SSOHandoff{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	return &handoff, err
}

func (r *Repository) SSORevoked(subject string, issuedAt int64) (bool, error) {
	var count int64
	err := r.db.Model(&model.SSORevocation{}).Where("subject = ? AND issued_at >= ?", subject, issuedAt).Count(&count).Error
	return count > 0, err
}

func (r *Repository) RevokeSSOSessions(subject string, issuedAt int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		record := model.SSORevocation{Subject: subject, IssuedAt: issuedAt}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "subject"}},
			DoUpdates: clause.Assignments(map[string]any{"issued_at": gorm.Expr("CASE WHEN sso_revocations.issued_at < ? THEN ? ELSE sso_revocations.issued_at END", issuedAt, issuedAt)}),
		}).Create(&record).Error; err != nil {
			return err
		}
		users := tx.Model(&model.UserIdentity{}).Select("user_id").Where("provider = ? AND subject = ?", "sub2api", subject)
		return tx.Where("user_id IN (?) AND COALESCE(sso_issued_at, 0) <= ?", users, issuedAt).Delete(&model.AuthSession{}).Error
	})
}

func (r *Repository) TouchSSOUserLogin(userID string, now time.Time) error {
	result := r.db.Model(&model.User{}).Where("id = ? AND status = ?", userID, model.UserStatusActive).
		Updates(map[string]any{"last_login_at": now, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
