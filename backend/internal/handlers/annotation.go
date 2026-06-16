package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"ultrasound-annotation/internal/models"
	"ultrasound-annotation/internal/repository"
	"ultrasound-annotation/internal/service"
)

type AnnotationHandler struct {
	repo     repository.PolygonAnnotationRepository
	imgRepo  repository.ScanImageRepository
	snapRepo repository.SnapshotRepository
	svc      service.AnnotationService
	collab   service.CollaborationService
	maxVer   int
}

func NewAnnotationHandler(
	repo repository.PolygonAnnotationRepository,
	imgRepo repository.ScanImageRepository,
	snapRepo repository.SnapshotRepository,
	svc service.AnnotationService,
	collab service.CollaborationService,
	maxVersions int,
) *AnnotationHandler {
	return &AnnotationHandler{
		repo: repo, imgRepo: imgRepo, snapRepo: snapRepo,
		svc: svc, collab: collab, maxVer: maxVersions,
	}
}

func (h *AnnotationHandler) Register(r *gin.RouterGroup) {
	g := r.Group("/annotations")
	{
		g.GET("", h.ListByImage)
		g.GET("/:id", h.Get)
		g.POST("", h.Create)
		g.PUT("/:id", h.Update)
		g.DELETE("/:id", h.Delete)
		g.POST("/bulk-replace", h.BulkReplace)
		g.GET("/stats", h.Stats)

		v := r.Group("/snapshots")
		{
			v.GET("", h.ListSnapshots)
			v.GET("/latest-version", h.LatestVersion)
			v.POST("/create", h.CreateSnapshot)
			v.POST("/restore", h.RestoreSnapshot)
		}
	}
}

func (h *AnnotationHandler) ListByImage(c *gin.Context) {
	imageID, err := parseUint(c.Query("imageId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "imageId required"})
		return
	}
	list, err := h.repo.ListByImage(c.Request.Context(), imageID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *AnnotationHandler) Get(c *gin.Context) {
	id, err := parseUint(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	item, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *AnnotationHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	var body models.PolygonAnnotation
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	operator := operatorOf(c)
	w, hh := h.imageDims(ctx, body.ScanImageID)
	created, err := h.svc.CreateAnnotation(ctx, &body, w, hh, operator)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = h.autoSnapshot(ctx, body.ScanImageID, operator, "create:"+itoa(int(created.ID)))
	_ = h.broadcastOp(ctx, body.ScanImageID, operator, service.OpTypeAdd, created)
	c.JSON(http.StatusCreated, gin.H{"data": created})
}

func (h *AnnotationHandler) Update(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := parseUint(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body models.PolygonAnnotation
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	body.ID = id
	operator := operatorOf(c)
	w, hh := h.imageDims(ctx, body.ScanImageID)
	if err := h.svc.UpdateAnnotation(ctx, &body, w, hh, operator); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, _ := h.repo.GetByID(ctx, id)
	_ = h.autoSnapshot(ctx, body.ScanImageID, operator, "update:"+itoa(int(id)))
	_ = h.broadcastOp(ctx, body.ScanImageID, operator, service.OpTypeUpdate, updated)
	c.JSON(http.StatusOK, gin.H{"data": updated})
}

func (h *AnnotationHandler) Delete(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := parseUint(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	old, err := h.repo.GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	operator := operatorOf(c)
	if err := h.svc.DeleteAnnotation(ctx, id, operator); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.autoSnapshot(ctx, old.ScanImageID, operator, "delete:"+itoa(int(id)))
	_ = h.broadcastOp(ctx, old.ScanImageID, operator, service.OpTypeDelete, old)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type bulkBody struct {
	ImageID     uint64                    `json:"imageId" binding:"required"`
	Annotations []models.PolygonAnnotation `json:"annotations"`
}

func (h *AnnotationHandler) BulkReplace(c *gin.Context) {
	ctx := c.Request.Context()
	var body bulkBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	operator := operatorOf(c)
	w, hh := h.imageDims(ctx, body.ImageID)
	if err := h.svc.BulkReplace(ctx, body.ImageID, body.Annotations, w, hh, operator); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	list, _ := h.repo.ListByImage(ctx, body.ImageID)
	_ = h.autoSnapshot(ctx, body.ImageID, operator, "bulk_replace")
	_ = h.broadcastOp(ctx, body.ImageID, operator, service.OpTypeBulkReplace, list)
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *AnnotationHandler) Stats(c *gin.Context) {
	imageID, err := parseUint(c.Query("imageId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "imageId required"})
		return
	}
	s, err := h.svc.ComputeStats(c.Request.Context(), imageID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": s})
}

func (h *AnnotationHandler) ListSnapshots(c *gin.Context) {
	imageID, err := parseUint(c.Query("imageId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "imageId required"})
		return
	}
	list, err := h.snapRepo.ListVersions(c.Request.Context(), imageID, h.maxVer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *AnnotationHandler) LatestVersion(c *gin.Context) {
	imageID, err := parseUint(c.Query("imageId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "imageId required"})
		return
	}
	v, err := h.snapRepo.GetLatestVersion(c.Request.Context(), imageID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"version": v})
}

type createSnapBody struct {
	ImageID uint64 `json:"imageId" binding:"required"`
	Summary string `json:"summary"`
}

func (h *AnnotationHandler) CreateSnapshot(c *gin.Context) {
	var body createSnapBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	list, err := h.repo.ListByImage(c.Request.Context(), body.ImageID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	operator := operatorOf(c)
	version, err := h.snapRepo.CreateSnapshot(c.Request.Context(), body.ImageID, list, operator, body.Summary, h.maxVer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"version": version})
}

type restoreBody struct {
	ImageID uint64 `json:"imageId" binding:"required"`
	Version int    `json:"version" binding:"required"`
}

func (h *AnnotationHandler) RestoreSnapshot(c *gin.Context) {
	ctx := c.Request.Context()
	var body restoreBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	operator := operatorOf(c)
	list, err := h.snapRepo.RestoreFromVersion(ctx, body.ImageID, body.Version, h.repo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.broadcastOp(ctx, body.ImageID, operator, service.OpTypeRollback, map[string]interface{}{
		"version":     body.Version,
		"annotations": list,
	})
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *AnnotationHandler) imageDims(ctx context.Context, imageID uint64) (int, int) {
	if imageID == 0 {
		return 4096, 4096
	}
	img, err := h.imgRepo.GetByID(ctx, imageID)
	if err != nil || img.Width == 0 {
		return 4096, 4096
	}
	return img.Width, img.Height
}

func (h *AnnotationHandler) autoSnapshot(ctx context.Context, imageID uint64, operator, summary string) error {
	list, err := h.repo.ListByImage(ctx, imageID)
	if err != nil {
		return err
	}
	_, err = h.snapRepo.CreateSnapshot(ctx, imageID, list, operator, summary, h.maxVer)
	return err
}

func (h *AnnotationHandler) broadcastOp(ctx context.Context, imageID uint64, operator, kind string, payload interface{}) error {
	b, _ := json.Marshal(payload)
	return h.collab.PublishOp(ctx, imageID, service.OperationMessage{
		Type:     kind,
		UserID:   operator,
		UserName: operator,
		Payload:  b,
	})
}

func operatorOf(c *gin.Context) string {
	if v := c.GetHeader("X-User-ID"); v != "" {
		return v
	}
	if v := c.GetHeader("X-User-Name"); v != "" {
		return v
	}
	return "system"
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
