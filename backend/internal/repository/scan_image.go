package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"ultrasound-annotation/internal/models"
)

type ScanImageRepository interface {
	ListByComponent(ctx context.Context, componentID uint64) ([]models.ScanImage, error)
	Create(ctx context.Context, img *models.ScanImage) error
	Update(ctx context.Context, img *models.ScanImage) error
	Delete(ctx context.Context, id uint64) error
	GetByID(ctx context.Context, id uint64) (*models.ScanImage, error)
}

type scanImageRepo struct{ db *gorm.DB }

func NewScanImageRepository(db *gorm.DB) ScanImageRepository {
	return &scanImageRepo{db: db}
}

func (r *scanImageRepo) ListByComponent(ctx context.Context, componentID uint64) ([]models.ScanImage, error) {
	var list []models.ScanImage
	err := r.db.WithContext(ctx).
		Where("component_id = ?", componentID).
		Order("created_at DESC").
		Find(&list).Error
	return list, err
}

func (r *scanImageRepo) Create(ctx context.Context, img *models.ScanImage) error {
	return r.db.WithContext(ctx).Create(img).Error
}

func (r *scanImageRepo) Update(ctx context.Context, img *models.ScanImage) error {
	if img.ID == 0 {
		return errors.New("id required")
	}
	return r.db.WithContext(ctx).Updates(img).Error
}

func (r *scanImageRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&models.ScanImage{}, id).Error
}

func (r *scanImageRepo) GetByID(ctx context.Context, id uint64) (*models.ScanImage, error) {
	var img models.ScanImage
	if err := r.db.WithContext(ctx).First(&img, id).Error; err != nil {
		return nil, err
	}
	return &img, nil
}
