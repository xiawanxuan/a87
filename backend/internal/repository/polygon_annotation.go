package repository

import (
	"context"
	"errors"
	"math"

	"gorm.io/gorm"
	"ultrasound-annotation/internal/models"
)

type PolygonAnnotationRepository interface {
	ListByImage(ctx context.Context, imageID uint64) ([]models.PolygonAnnotation, error)
	GetByID(ctx context.Context, id uint64) (*models.PolygonAnnotation, error)
	Create(ctx context.Context, a *models.PolygonAnnotation) error
	Update(ctx context.Context, a *models.PolygonAnnotation) error
	Delete(ctx context.Context, id uint64) error
	BulkReplace(ctx context.Context, imageID uint64, items []models.PolygonAnnotation) error
}

type polygonAnnotationRepo struct{ db *gorm.DB }

func NewPolygonAnnotationRepository(db *gorm.DB) PolygonAnnotationRepository {
	return &polygonAnnotationRepo{db: db}
}

func (r *polygonAnnotationRepo) ListByImage(ctx context.Context, imageID uint64) ([]models.PolygonAnnotation, error) {
	var list []models.PolygonAnnotation
	err := r.db.WithContext(ctx).
		Preload("DiseaseType").
		Preload("Layer").
		Where("scan_image_id = ?", imageID).
		Order("id ASC").
		Find(&list).Error
	return list, err
}

func (r *polygonAnnotationRepo) GetByID(ctx context.Context, id uint64) (*models.PolygonAnnotation, error) {
	var a models.PolygonAnnotation
	if err := r.db.WithContext(ctx).Preload("DiseaseType").First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *polygonAnnotationRepo) Create(ctx context.Context, a *models.PolygonAnnotation) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *polygonAnnotationRepo) Update(ctx context.Context, a *models.PolygonAnnotation) error {
	if a.ID == 0 {
		return errors.New("id required")
	}
	return r.db.WithContext(ctx).Updates(a).Error
}

func (r *polygonAnnotationRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&models.PolygonAnnotation{}, id).Error
}

func (r *polygonAnnotationRepo) BulkReplace(ctx context.Context, imageID uint64, items []models.PolygonAnnotation) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("scan_image_id = ?", imageID).Delete(&models.PolygonAnnotation{}).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		return tx.Create(&items).Error
	})
}

func ComputeAreaAndBBox(points []models.Point) (areaPx float64, bbox models.BoundingBox) {
	if len(points) < 3 {
		return
	}
	bbox.MinX, bbox.MinY = math.MaxFloat64, math.MaxFloat64
	bbox.MaxX, bbox.MaxY = -math.MaxFloat64, -math.MaxFloat64
	sum := 0.0
	n := len(points)
	for i := 0; i < n; i++ {
		p := points[i]
		next := points[(i+1)%n]
		sum += p.X*next.Y - next.X*p.Y
		if p.X < bbox.MinX {
			bbox.MinX = p.X
		}
		if p.Y < bbox.MinY {
			bbox.MinY = p.Y
		}
		if p.X > bbox.MaxX {
			bbox.MaxX = p.X
		}
		if p.Y > bbox.MaxY {
			bbox.MaxY = p.Y
		}
	}
	areaPx = math.Abs(sum) / 2.0
	return
}
