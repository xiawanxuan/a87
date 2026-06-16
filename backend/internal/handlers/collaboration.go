package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"ultrasound-annotation/internal/service"
)

type CollaborationHandler struct {
	collab service.CollaborationService
}

func NewCollaborationHandler(collab service.CollaborationService) *CollaborationHandler {
	return &CollaborationHandler{collab: collab}
}

func (h *CollaborationHandler) Register(r *gin.RouterGroup) {
	g := r.Group("/collab")
	{
		g.GET("/users", h.ListUsers)
		g.POST("/heartbeat", h.Heartbeat)
		g.GET("/draft", h.GetDraft)
		g.PUT("/draft", h.SaveDraft)
		g.POST("/lock", h.Lock)
		g.POST("/unlock", h.Unlock)
	}
}

func (h *CollaborationHandler) ListUsers(c *gin.Context) {
	imageID, err := parseUint(c.Query("imageId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "imageId required"})
		return
	}
	users, err := h.collab.ListUsers(c.Request.Context(), imageID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": users})
}

type heartbeatBody struct {
	ImageID    uint64       `json:"imageId" binding:"required"`
	UserID     string       `json:"userId" binding:"required"`
	UserName   string       `json:"userName"`
	CursorPos  *CursorPoint `json:"cursorPos"`
	ActiveTool string       `json:"activeTool"`
}

type CursorPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func (h *CollaborationHandler) Heartbeat(c *gin.Context) {
	var b heartbeatBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sess := service.CollabSession{
		UserID:     b.UserID,
		UserName:   b.UserName,
		ActiveTool: b.ActiveTool,
	}
	if b.CursorPos != nil {
		sess.CursorPos = &struct{ X, Y float64 }{
			X: b.CursorPos.X, Y: b.CursorPos.Y,
		}
	}
	if err := h.collab.Heartbeat(c.Request.Context(), b.ImageID, sess); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *CollaborationHandler) GetDraft(c *gin.Context) {
	imageID, err := parseUint(c.Query("imageId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "imageId required"})
		return
	}
	userID := c.Query("userId")
	if userID == "" {
		userID = c.GetHeader("X-User-ID")
	}
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId required"})
		return
	}
	data, err := h.collab.GetDraft(c.Request.Context(), imageID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var payload interface{}
	if len(data) > 0 {
		_ = json.Unmarshal(data, &payload)
	}
	c.JSON(http.StatusOK, gin.H{"data": payload})
}

type draftBody struct {
	ImageID uint64      `json:"imageId" binding:"required"`
	UserID  string      `json:"userId" binding:"required"`
	Data    interface{} `json:"data"`
}

func (h *CollaborationHandler) SaveDraft(c *gin.Context) {
	var b draftBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	data, err := json.Marshal(b.Data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid data"})
		return
	}
	if err := h.collab.SetDraft(c.Request.Context(), b.ImageID, b.UserID, data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type lockBody struct {
	ImageID      uint64 `json:"imageId" binding:"required"`
	AnnotationID uint64 `json:"annotationId" binding:"required"`
	UserID       string `json:"userId" binding:"required"`
	TTLSeconds   int    `json:"ttlSeconds"`
}

func (h *CollaborationHandler) Lock(c *gin.Context) {
	var b lockBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ttl := 30
	if b.TTLSeconds > 0 {
		ttl = b.TTLSeconds
	}
	ok, err := h.collab.LockAnnotation(c.Request.Context(), b.ImageID, b.AnnotationID, b.UserID,
		time.Duration(ttl)*time.Second)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"locked": ok})
}

type unlockBody = lockBody

func (h *CollaborationHandler) Unlock(c *gin.Context) {
	var b unlockBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.collab.UnlockAnnotation(c.Request.Context(), b.ImageID, b.AnnotationID, b.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
