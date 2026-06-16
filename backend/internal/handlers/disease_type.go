package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"ultrasound-annotation/internal/repository"
)

type DiseaseTypeHandler struct {
	repo repository.DiseaseTypeRepository
}

func NewDiseaseTypeHandler(repo repository.DiseaseTypeRepository) *DiseaseTypeHandler {
	return &DiseaseTypeHandler{repo: repo}
}

func (h *DiseaseTypeHandler) Register(r *gin.RouterGroup) {
	g := r.Group("/disease-types")
	{
		g.GET("", h.List)
		g.GET("/:code", h.GetByCode)
	}
}

func (h *DiseaseTypeHandler) List(c *gin.Context) {
	list, err := h.repo.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *DiseaseTypeHandler) GetByCode(c *gin.Context) {
	code := c.Param("code")
	d, err := h.repo.GetByCode(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": d})
}
