package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/datatypes"
	"ultrasound-annotation/internal/models"
	"ultrasound-annotation/internal/repository"
)

type AnnotationValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e AnnotationValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

type AnnotationService interface {
	ValidatePolygon(ctx context.Context, imageID uint64, points []models.Point) []AnnotationValidationError
	ValidateAnnotation(ctx context.Context, a *models.PolygonAnnotation, imageMaxW, imageMaxH int) []AnnotationValidationError
	CreateAnnotation(ctx context.Context, a *models.PolygonAnnotation, imageMaxW, imageMaxH int, operator string) (*models.PolygonAnnotation, error)
	UpdateAnnotation(ctx context.Context, a *models.PolygonAnnotation, imageMaxW, imageMaxH int, operator string) error
	DeleteAnnotation(ctx context.Context, id uint64, operator string) error
	BulkReplace(ctx context.Context, imageID uint64, items []models.PolygonAnnotation, imageMaxW, imageMaxH int, operator string) error
	ComputeStats(ctx context.Context, imageID uint64) (map[string]interface{}, error)
}

type annotationSvc struct {
	repo     repository.PolygonAnnotationRepository
	imgRepo  repository.ScanImageRepository
}

func NewAnnotationService(repo repository.PolygonAnnotationRepository, imgRepo repository.ScanImageRepository) AnnotationService {
	return &annotationSvc{repo: repo, imgRepo: imgRepo}
}

func (s *annotationSvc) ValidatePolygon(ctx context.Context, imageID uint64, points []models.Point) []AnnotationValidationError {
	var errs []AnnotationValidationError
	if len(points) < 3 {
		errs = append(errs, AnnotationValidationError{"points", "多边形至少需要 3 个点"})
		return errs
	}
	if len(points) > 500 {
		errs = append(errs, AnnotationValidationError{"points", "多边形顶点数过多"})
	}
	for i, p := range points {
		if p.X < 0 || p.Y < 0 {
			errs = append(errs, AnnotationValidationError{
				fmt.Sprintf("points[%d]", i), "坐标不能为负数",
			})
		}
	}
	return errs
}

func (s *annotationSvc) ValidateAnnotation(ctx context.Context, a *models.PolygonAnnotation, imageMaxW, imageMaxH int) []AnnotationValidationError {
	var errs []AnnotationValidationError
	if a.ScanImageID == 0 {
		errs = append(errs, AnnotationValidationError{"scanImageId", "缺少图谱 ID"})
	}
	if a.DiseaseTypeID == 0 {
		errs = append(errs, AnnotationValidationError{"diseaseTypeId", "缺少病害分类"})
	}
	if a.Severity < 1 || a.Severity > 5 {
		errs = append(errs, AnnotationValidationError{"severity", "严重等级 1-5"})
	}
	var points []models.Point
	if len(a.Points) == 0 {
		errs = append(errs, AnnotationValidationError{"points", "points 不能为空"})
		return errs
	}
	if err := a.Points.Scan(&points); err != nil {
		errs = append(errs, AnnotationValidationError{"points", "points JSON 解析失败"})
		return errs
	}
	errs = append(errs, s.ValidatePolygon(ctx, a.ScanImageID, points)...)

	if imageMaxW > 0 && imageMaxH > 0 {
		for i, p := range points {
			if p.X > float64(imageMaxW) || p.Y > float64(imageMaxH) {
				errs = append(errs, AnnotationValidationError{
					fmt.Sprintf("points[%d]", i),
					fmt.Sprintf("坐标超出画布范围 (%d,%d)", imageMaxW, imageMaxH),
				})
			}
		}
	}
	return errs
}

func (s *annotationSvc) CreateAnnotation(ctx context.Context, a *models.PolygonAnnotation, imageMaxW, imageMaxH int, operator string) (*models.PolygonAnnotation, error) {
	if errs := s.ValidateAnnotation(ctx, a, imageMaxW, imageMaxH); len(errs) > 0 {
		return nil, errors.New(errs[0].Message)
	}
	var points []models.Point
	_ = a.Points.Scan(&points)
	areaPx, bbox := repository.ComputeAreaAndBBox(points)
	a.AreaPx = areaPx

	pixelScale := s.getPixelScale(ctx, a.ScanImageID)
	a.AreaCm2 = ComputeAreaCm2(areaPx, pixelScale)

	b, _ := bboxToJSON(bbox)
	a.BoundingBox = b
	a.Operator = operator
	if err := s.repo.Create(ctx, a); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, a.ID)
}

func (s *annotationSvc) getPixelScale(ctx context.Context, imageID uint64) float64 {
	if s.imgRepo == nil {
		return 0.1
	}
	img, err := s.imgRepo.GetByID(ctx, imageID)
	if err != nil || img == nil {
		return 0.1
	}
	if img.PixelScaleMm <= 0 {
		return 0.1
	}
	return img.PixelScaleMm
}

func ComputeAreaCm2(areaPx float64, pixelScaleMm float64) float64 {
	if pixelScaleMm <= 0 {
		return 0
	}
	areaMm2 := areaPx * pixelScaleMm * pixelScaleMm
	return areaMm2 / 100.0
}

func (s *annotationSvc) UpdateAnnotation(ctx context.Context, a *models.PolygonAnnotation, imageMaxW, imageMaxH int, operator string) error {
	if errs := s.ValidateAnnotation(ctx, a, imageMaxW, imageMaxH); len(errs) > 0 {
		return errors.New(errs[0].Message)
	}
	var points []models.Point
	_ = a.Points.Scan(&points)
	areaPx, bbox := repository.ComputeAreaAndBBox(points)
	a.AreaPx = areaPx

	pixelScale := s.getPixelScale(ctx, a.ScanImageID)
	a.AreaCm2 = ComputeAreaCm2(areaPx, pixelScale)

	b, _ := bboxToJSON(bbox)
	a.BoundingBox = b
	a.Operator = operator
	return s.repo.Update(ctx, a)
}

func (s *annotationSvc) DeleteAnnotation(ctx context.Context, id uint64, operator string) error {
	return s.repo.Delete(ctx, id)
}

func (s *annotationSvc) BulkReplace(ctx context.Context, imageID uint64, items []models.PolygonAnnotation, imageMaxW, imageMaxH int, operator string) error {
	pixelScale := s.getPixelScale(ctx, imageID)
	for i := range items {
		items[i].ScanImageID = imageID
		if errs := s.ValidateAnnotation(ctx, &items[i], imageMaxW, imageMaxH); len(errs) > 0 {
			return errors.New(errs[0].Message)
		}
		var points []models.Point
		_ = items[i].Points.Scan(&points)
		areaPx, bbox := repository.ComputeAreaAndBBox(points)
		items[i].AreaPx = areaPx
		items[i].AreaCm2 = ComputeAreaCm2(areaPx, pixelScale)
		b, _ := bboxToJSON(bbox)
		items[i].BoundingBox = b
		items[i].Operator = operator
	}
	return s.repo.BulkReplace(ctx, imageID, items)
}

func (s *annotationSvc) ComputeStats(ctx context.Context, imageID uint64) (map[string]interface{}, error) {
	list, err := s.repo.ListByImage(ctx, imageID)
	if err != nil {
		return nil, err
	}
	pixelScale := s.getPixelScale(ctx, imageID)
	totalAreaPx := 0.0
	totalAreaCm2 := 0.0
	byDisease := make(map[string]int)
	byDiseaseAreaCm2 := make(map[string]float64)
	for _, a := range list {
		totalAreaPx += a.AreaPx
		if a.AreaCm2 > 0 {
			totalAreaCm2 += a.AreaCm2
		} else {
			totalAreaCm2 += ComputeAreaCm2(a.AreaPx, pixelScale)
		}
		code := "unknown"
		if a.DiseaseType != nil {
			code = a.DiseaseType.Code
		}
		byDisease[code]++
		if a.AreaCm2 > 0 {
			byDiseaseAreaCm2[code] += a.AreaCm2
		} else {
			byDiseaseAreaCm2[code] += ComputeAreaCm2(a.AreaPx, pixelScale)
		}
	}
	return map[string]interface{}{
		"totalCount":        len(list),
		"totalAreaPx":       totalAreaPx,
		"totalAreaCm2":      totalAreaCm2,
		"pixelScaleMm":      pixelScale,
		"byDiseaseCount":    byDisease,
		"byDiseaseAreaCm2":  byDiseaseAreaCm2,
	}, nil
}

func bboxToJSON(b models.BoundingBox) (datatypes.JSON, error) {
	b2, err := jsonMarshal(b)
	return datatypes.JSON(b2), err
}

func jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
