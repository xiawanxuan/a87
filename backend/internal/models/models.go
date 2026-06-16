package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type BoundingBox struct {
	MinX float64 `json:"minX"`
	MinY float64 `json:"minY"`
	MaxX float64 `json:"maxX"`
	MaxY float64 `json:"maxY"`
}

type ComponentCategory struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:64;not null;unique" json:"name"`
	Description string    `gorm:"type:text" json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type WoodComponent struct {
	ID           uint64         `gorm:"primaryKey" json:"id"`
	ParentID     *uint64        `gorm:"index" json:"parentId,omitempty"`
	CategoryID   *uint64        `json:"categoryId,omitempty"`
	Name         string         `gorm:"size:128;not null" json:"name"`
	Code         string         `gorm:"size:64;unique" json:"code,omitempty"`
	Description  string         `gorm:"type:text" json:"description,omitempty"`
	BuildingName string         `gorm:"size:128" json:"buildingName,omitempty"`
	Location     string         `gorm:"size:256" json:"location,omitempty"`
	Material     string         `gorm:"size:64" json:"material,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`

	Parent    *WoodComponent   `gorm:"foreignKey:ParentID" json:"-"`
	Category  *ComponentCategory `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	Children  []WoodComponent  `gorm:"foreignKey:ParentID" json:"children,omitempty"`
	ScanImages []ScanImage     `gorm:"foreignKey:ComponentID" json:"-"`
}

type DiseaseType struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	Code          string    `gorm:"size:32;not null;unique" json:"code"`
	Name          string    `gorm:"size:64;not null" json:"name"`
	ColorHex      string    `gorm:"size:16;not null;default:'#FF6B6B'" json:"colorHex"`
	Description   string    `gorm:"type:text" json:"description,omitempty"`
	SeverityOrder int       `gorm:"not null;default:0" json:"severityOrder"`
	Enabled       bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt     time.Time `json:"createdAt"`
}

type ScanImage struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	ComponentID  uint64    `gorm:"not null;index" json:"componentId"`
	FileName     string    `gorm:"size:256;not null" json:"fileName"`
	FilePath     string    `gorm:"size:512;not null" json:"filePath"`
	FileSize     int64     `gorm:"not null;default:0" json:"fileSize"`
	MimeType     string    `gorm:"size:64" json:"mimeType,omitempty"`
	Width        int       `gorm:"not null;default:0" json:"width"`
	Height       int       `gorm:"not null;default:0" json:"height"`
	BitsDepth    int       `gorm:"not null;default:8" json:"bitsDepth"`
	PixelScaleMm float64   `gorm:"not null;default:0.1" json:"pixelScaleMm"`
	ScanDate     *time.Time `json:"scanDate,omitempty"`
	Operator     string    `gorm:"size:64" json:"operator,omitempty"`
	Equipment    string    `gorm:"size:128" json:"equipment,omitempty"`
	Remark       string    `gorm:"type:text" json:"remark,omitempty"`
	UploadedBy   string    `gorm:"size:64" json:"uploadedBy,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`

	Component *WoodComponent `gorm:"foreignKey:ComponentID" json:"component,omitempty"`
}

type AnnotationLayer struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	ScanImageID   uint64    `gorm:"not null;index" json:"scanImageId"`
	LayerName     string    `gorm:"size:64;not null" json:"layerName"`
	DiseaseTypeID *uint64   `json:"diseaseTypeId,omitempty"`
	Opacity       float64   `gorm:"not null;default:0.6" json:"opacity"`
	Visible       bool      `gorm:"not null;default:true" json:"visible"`
	Locked        bool      `gorm:"not null;default:false" json:"locked"`
	SortOrder     int       `gorm:"not null;default:0" json:"sortOrder"`
	CreatedBy     string    `gorm:"size:64" json:"createdBy,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`

	DiseaseType *DiseaseType `gorm:"foreignKey:DiseaseTypeID" json:"diseaseType,omitempty"`
}

type PolygonAnnotation struct {
	ID            uint64          `gorm:"primaryKey" json:"id"`
	ScanImageID   uint64          `gorm:"not null;index" json:"scanImageId"`
	LayerID       *uint64         `gorm:"index" json:"layerId,omitempty"`
	DiseaseTypeID uint64          `gorm:"not null;index" json:"diseaseTypeId"`
	Label         string          `gorm:"size:256" json:"label,omitempty"`
	Points        datatypes.JSON  `gorm:"type:jsonb;not null" json:"points"`
	BoundingBox   datatypes.JSON  `gorm:"type:jsonb" json:"boundingBox,omitempty"`
	AreaPx        float64         `json:"areaPx,omitempty"`
	AreaCm2       float64         `json:"areaCm2,omitempty"`
	Severity      int             `gorm:"not null;default:1" json:"severity"`
	Note          string          `gorm:"type:text" json:"note,omitempty"`
	Operator      string          `gorm:"size:64" json:"operator,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`

	DiseaseType *DiseaseType `gorm:"foreignKey:DiseaseTypeID" json:"diseaseType,omitempty"`
	Layer       *AnnotationLayer `gorm:"foreignKey:LayerID" json:"layer,omitempty"`
}

type AnnotationSnapshot struct {
	ID            uint64         `gorm:"primaryKey" json:"id"`
	ScanImageID   uint64         `gorm:"not null;index" json:"scanImageId"`
	VersionNumber int            `gorm:"not null" json:"versionNumber"`
	SnapshotData  datatypes.JSON `gorm:"type:jsonb;not null" json:"snapshotData"`
	DiffSummary   string         `gorm:"type:text" json:"diffSummary,omitempty"`
	Operator      string         `gorm:"size:64" json:"operator,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
}

type CollaborationSession struct {
	ID            uint64         `gorm:"primaryKey" json:"id"`
	ScanImageID   uint64         `gorm:"not null;index" json:"scanImageId"`
	UserID        string         `gorm:"size:64;not null" json:"userId"`
	UserName      string         `gorm:"size:64" json:"userName,omitempty"`
	CursorPos     datatypes.JSON `gorm:"type:jsonb" json:"cursorPos,omitempty"`
	ActiveTool    string         `gorm:"size:32" json:"activeTool,omitempty"`
	LastHeartbeat time.Time      `gorm:"not null" json:"lastHeartbeat"`
	JoinedAt      time.Time      `json:"joinedAt"`
}
