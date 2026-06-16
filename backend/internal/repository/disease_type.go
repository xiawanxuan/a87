package repository

import (
	"context"

	"gorm.io/gorm"
	"ultrasound-annotation/internal/models"
)

type DiseaseTypeRepository interface {
	List(ctx context.Context) ([]models.DiseaseType, error)
	GetByCode(ctx context.Context, code string) (*models.DiseaseType, error)
}

type diseaseTypeRepo struct{ db *gorm.DB }

func NewDiseaseTypeRepository(db *gorm.DB) DiseaseTypeRepository {
	return &diseaseTypeRepo{db: db}
}

func (r *diseaseTypeRepo) List(ctx context.Context) ([]models.DiseaseType, error) {
	var list []models.DiseaseType
	err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("severity_order ASC, id ASC").
		Find(&list).Error
	return list, err
}

func (r *diseaseTypeRepo) GetByCode(ctx context.Context, code string) (*models.DiseaseType, error) {
	var d models.DiseaseType
	if err := r.db.WithContext(ctx).Where("code = ?", code).First(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}
