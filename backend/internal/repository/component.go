package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"ultrasound-annotation/internal/models"
)

type ComponentRepository interface {
	ListTree(ctx context.Context) ([]models.WoodComponent, error)
	Create(ctx context.Context, c *models.WoodComponent) error
	Update(ctx context.Context, c *models.WoodComponent) error
	Delete(ctx context.Context, id uint64) error
	GetByID(ctx context.Context, id uint64) (*models.WoodComponent, error)
}

type componentRepo struct{ db *gorm.DB }

func NewComponentRepository(db *gorm.DB) ComponentRepository {
	return &componentRepo{db: db}
}

func (r *componentRepo) ListTree(ctx context.Context) ([]models.WoodComponent, error) {
	var items []models.WoodComponent
	err := r.db.WithContext(ctx).
		Preload("Category").
		Order("parent_id NULLS FIRST, id").
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return buildTree(items), nil
}

func buildTree(items []models.WoodComponent) []models.WoodComponent {
	idx := make(map[uint64]*models.WoodComponent, len(items))
	for i := range items {
		idx[items[i].ID] = &items[i]
	}
	var roots []models.WoodComponent
	for i := range items {
		if items[i].ParentID == nil {
			roots = append(roots, items[i])
			continue
		}
		if p, ok := idx[*items[i].ParentID]; ok {
			p.Children = append(p.Children, items[i])
		}
	}
	return roots
}

func (r *componentRepo) Create(ctx context.Context, c *models.WoodComponent) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *componentRepo) Update(ctx context.Context, c *models.WoodComponent) error {
	if c.ID == 0 {
		return errors.New("id required")
	}
	return r.db.WithContext(ctx).Updates(c).Error
}

func (r *componentRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&models.WoodComponent{}, id).Error
}

func (r *componentRepo) GetByID(ctx context.Context, id uint64) (*models.WoodComponent, error) {
	var c models.WoodComponent
	if err := r.db.WithContext(ctx).Preload("Category").First(&c, id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}
