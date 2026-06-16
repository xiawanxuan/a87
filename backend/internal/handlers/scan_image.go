package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"ultrasound-annotation/internal/config"
	"ultrasound-annotation/internal/models"
	"ultrasound-annotation/internal/repository"
)

type ScanImageHandler struct {
	repo    repository.ScanImageRepository
	compRepo repository.ComponentRepository
	cfg     config.UploadConfig
}

func NewScanImageHandler(repo repository.ScanImageRepository, compRepo repository.ComponentRepository, cfg config.UploadConfig) *ScanImageHandler {
	_ = os.MkdirAll(cfg.Dir, 0755)
	return &ScanImageHandler{repo: repo, compRepo: compRepo, cfg: cfg}
}

func (h *ScanImageHandler) Register(r *gin.RouterGroup) {
	g := r.Group("/scan-images")
	{
		g.GET("", h.ListByComponent)
		g.GET("/:id", h.Get)
		g.GET("/:id/download", h.Download)
		g.POST("", h.Create)
		g.POST("/upload", h.Upload)
		g.PUT("/:id", h.Update)
		g.DELETE("/:id", h.Delete)
	}
	r.Static("/static/uploads", h.cfg.Dir)
}

func (h *ScanImageHandler) ListByComponent(c *gin.Context) {
	compID, err := parseUint(c.Query("componentId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "componentId required"})
		return
	}
	list, err := h.repo.ListByComponent(c.Request.Context(), compID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *ScanImageHandler) Get(c *gin.Context) {
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

func (h *ScanImageHandler) Download(c *gin.Context) {
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
	c.FileAttachment(item.FilePath, item.FileName)
}

type createBody struct {
	ComponentID uint64 `json:"componentId" binding:"required"`
	FileName    string `json:"fileName" binding:"required"`
	FilePath    string `json:"filePath"`
	FileSize    int64  `json:"fileSize"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	MimeType    string `json:"mimeType"`
	ScanDate    string `json:"scanDate"`
	Operator    string `json:"operator"`
	Equipment   string `json:"equipment"`
	Remark      string `json:"remark"`
	UploadedBy  string `json:"uploadedBy"`
}

func (h *ScanImageHandler) Create(c *gin.Context) {
	var body createBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	img := &models.ScanImage{
		ComponentID: body.ComponentID,
		FileName:    body.FileName,
		FilePath:    body.FilePath,
		FileSize:    body.FileSize,
		Width:       body.Width,
		Height:      body.Height,
		MimeType:    body.MimeType,
		Operator:    body.Operator,
		Equipment:   body.Equipment,
		Remark:      body.Remark,
		UploadedBy:  body.UploadedBy,
	}
	if body.ScanDate != "" {
		if t, err := time.Parse(time.RFC3339, body.ScanDate); err == nil {
			img.ScanDate = &t
		}
	}
	if err := h.repo.Create(c.Request.Context(), img); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": img})
}

func (h *ScanImageHandler) Upload(c *gin.Context) {
	compID, err := parseUint(c.Query("componentId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "componentId required"})
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if file.Size > h.cfg.MaxSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file too large"})
		return
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".tif" && ext != ".tiff" && ext != ".bmp" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported image type"})
		return
	}
	subDir := filepath.Join(h.cfg.Dir, fmt.Sprintf("%d", compID), time.Now().Format("200601"))
	_ = os.MkdirAll(subDir, 0755)
	newName := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	savePath := filepath.Join(subDir, newName)
	if err := c.SaveUploadedFile(file, savePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	w, h := parseSizeHint(c.Query("w"), c.Query("h"))
	uploadedBy := c.GetHeader("X-User-Name")
	img := &models.ScanImage{
		ComponentID: compID,
		FileName:    file.Filename,
		FilePath:    savePath,
		FileSize:    file.Size,
		Width:       w,
		Height:      h,
		MimeType:    file.Header.Get("Content-Type"),
		UploadedBy:  uploadedBy,
	}
	if err := h.repo.Create(c.Request.Context(), img); err != nil {
		_ = os.Remove(savePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": img})
}

func parseSizeHint(ws, hs string) (int, int) {
	w, _ := strconv.Atoi(ws)
	h, _ := strconv.Atoi(hs)
	if w <= 0 {
		w = 4096
	}
	if h <= 0 {
		h = 4096
	}
	return w, h
}

func (h *ScanImageHandler) Update(c *gin.Context) {
	id, err := parseUint(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body models.ScanImage
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	body.ID = id
	if err := h.repo.Update(c.Request.Context(), &body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": body})
}

func (h *ScanImageHandler) Delete(c *gin.Context) {
	id, err := parseUint(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if item, err := h.repo.GetByID(c.Request.Context(), id); err == nil && item.FilePath != "" {
		_ = os.Remove(item.FilePath)
	}
	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
