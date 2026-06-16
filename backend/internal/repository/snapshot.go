package repository

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
	"ultrasound-annotation/internal/models"
)

type SnapshotRepository interface {
	ListVersions(ctx context.Context, imageID uint64, limit int) ([]models.AnnotationSnapshot, error)
	GetByVersion(ctx context.Context, imageID uint64, version int) (*models.AnnotationSnapshot, error)
	CreateSnapshot(ctx context.Context, imageID uint64, annotations []models.PolygonAnnotation, operator, summary string, maxVersions int) (int, error)
	GetLatestVersion(ctx context.Context, imageID uint64) (int, error)
	RestoreFromVersion(ctx context.Context, imageID uint64, version int, repo PolygonAnnotationRepository) ([]models.PolygonAnnotation, error)
}

type snapshotRepo struct{ db *gorm.DB }

func NewSnapshotRepository(db *gorm.DB) SnapshotRepository {
	return &snapshotRepo{db: db}
}

func (r *snapshotRepo) ListVersions(ctx context.Context, imageID uint64, limit int) ([]models.AnnotationSnapshot, error) {
	var list []models.AnnotationSnapshot
	err := r.db.WithContext(ctx).
		Where("scan_image_id = ?", imageID).
		Order("version_number DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

func (r *snapshotRepo) GetByVersion(ctx context.Context, imageID uint64, version int) (*models.AnnotationSnapshot, error) {
	var s models.AnnotationSnapshot
	if err := r.db.WithContext(ctx).
		Where("scan_image_id = ? AND version_number = ?", imageID, version).
		First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *snapshotRepo) GetLatestVersion(ctx context.Context, imageID uint64) (int, error) {
	var s models.AnnotationSnapshot
	err := r.db.WithContext(ctx).
		Where("scan_image_id = ?", imageID).
		Order("version_number DESC").
		First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return s.VersionNumber, nil
}

func (r *snapshotRepo) CreateSnapshot(ctx context.Context, imageID uint64, annotations []models.PolygonAnnotation, operator, summary string, maxVersions int) (int, error) {
	data, err := json.Marshal(annotations)
	if err != nil {
		return 0, err
	}

	var version int
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var s models.AnnotationSnapshot
		err := tx.Where("scan_image_id = ?", imageID).
			Order("version_number DESC").
			First(&s).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			version = 1
		} else if err != nil {
			return err
		} else {
			version = s.VersionNumber + 1
		}

		if err := tx.Create(&models.AnnotationSnapshot{
			ScanImageID:   imageID,
			VersionNumber: version,
			SnapshotData:  data,
			DiffSummary:   summary,
			Operator:      operator,
		}).Error; err != nil {
			return err
		}

		cutoff := version - maxVersions
		if cutoff > 0 {
			_ = tx.Where("scan_image_id = ? AND version_number <= ?", imageID, cutoff).
				Delete(&models.AnnotationSnapshot{}).Error
		}
		return nil
	})
	return version, err
}

func (r *snapshotRepo) RestoreFromVersion(ctx context.Context, imageID uint64, version int, polyRepo PolygonAnnotationRepository) ([]models.PolygonAnnotation, error) {
	s, err := r.GetByVersion(ctx, imageID, version)
	if err != nil {
		return nil, err
	}
	var list []models.PolygonAnnotation
	if err := json.Unmarshal(s.SnapshotData, &list); err != nil {
		return nil, err
	}
	for i := range list {
		list[i].ID = 0
		list[i].ScanImageID = imageID
	}
	if err := polyRepo.BulkReplace(ctx, imageID, list); err != nil {
		return nil, err
	}
	return list, nil
}
