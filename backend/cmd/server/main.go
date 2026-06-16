package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"ultrasound-annotation/internal/config"
	"ultrasound-annotation/internal/handlers"
	"ultrasound-annotation/internal/middleware"
	"ultrasound-annotation/internal/repository"
	"ultrasound-annotation/internal/service"
	"ultrasound-annotation/internal/ws"
)

func main() {
	configPath := filepath.Join("..", "..", "configs", "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		configPath = filepath.Join("configs", "config.yaml")
	}
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		configPath = p
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	gin.SetMode(cfg.Server.Mode)

	db, err := config.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	log.Println("postgresql connected")

	rdb, err := config.InitRedis(cfg.Redis)
	if err != nil {
		log.Fatalf("init redis: %v", err)
	}
	log.Println("redis connected")

	compRepo := repository.NewComponentRepository(db)
	imgRepo := repository.NewScanImageRepository(db)
	disRepo := repository.NewDiseaseTypeRepository(db)
	polyRepo := repository.NewPolygonAnnotationRepository(db)
	snapRepo := repository.NewSnapshotRepository(db)

	collabSvc := service.NewCollaborationService(rdb)
	annSvc := service.NewAnnotationService(polyRepo, imgRepo)

	compH := handlers.NewComponentHandler(compRepo)
	imgH := handlers.NewScanImageHandler(imgRepo, compRepo, cfg.Upload)
	disH := handlers.NewDiseaseTypeHandler(disRepo)
	annH := handlers.NewAnnotationHandler(polyRepo, imgRepo, snapRepo, annSvc, collabSvc, cfg.Annotation.MaxVersions)
	collabH := handlers.NewCollaborationHandler(collabSvc)

	hub := ws.NewHub(collabSvc)

	r := gin.New()
	r.Use(gin.Recovery(), middleware.RequestLogger())
	r.Use(middleware.CORSMiddleware([]string{"*"}))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "time": time.Now().Format(time.RFC3339)})
	})

	api := r.Group("/api/v1")
	{
		compH.Register(api)
		imgH.Register(api)
		disH.Register(api)
		annH.Register(api)
		collabH.Register(api)
	}

	r.GET("/ws/scan-images/:imageId", hub.ServeWS)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("server listening on :%d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced shutdown: %v", err)
	}
	log.Println("server exiting")
}
