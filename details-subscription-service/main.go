package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cleanapp-common/serverx"

	"details-subscription-service/config"
	"details-subscription-service/database"
	"details-subscription-service/handlers"
	"details-subscription-service/middleware"
	"details-subscription-service/scheduler"
	"details-subscription-service/utils"
	"details-subscription-service/version"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("ERROR: Failed to load config: %v", err)
	}

	db, err := database.OpenDB(cfg)
	if err != nil {
		log.Fatalf("ERROR: Failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := database.RunMigrations(ctx, db); err != nil {
		log.Fatalf("ERROR: Failed to run migrations: %v", err)
	}

	stripeClient := utils.NewStripeClient(cfg)
	emailSender := utils.NewEmailSender(cfg.SendGridAPIKey, cfg.SendGridFromName, cfg.SendGridFromEmail)
	service := database.NewSubscriptionService(db)

	router := setupRouter(service, stripeClient, emailSender, cfg, db)

	sched := scheduler.NewScheduler(service, emailSender, time.Duration(cfg.SchedulerIntervalMinutes)*time.Minute)
	sched.Start()

	srv := serverx.New(":"+cfg.Port, router)

	go func() {
		log.Printf("INFO: Server starting on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ERROR: Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")

	sched.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited")
}

func setupRouter(service *database.SubscriptionService, stripeClient *utils.StripeClient, emailSender *utils.EmailSender, cfg *config.Config, db *sql.DB) *gin.Engine {
	router := gin.Default()
	router.SetTrustedProxies(cfg.TrustedProxies)
	router.Use(middleware.CORSMiddleware(cfg.AllowedOrigins))
	router.Use(middleware.SecurityHeaders())
	router.Use(middleware.RateLimitMiddleware(cfg.RateLimitRPS, cfg.RateLimitBurst))

	h := handlers.NewHandlers(service, stripeClient, emailSender, cfg)

	router.GET("/health", h.HealthCheck)
	router.GET("/version", func(c *gin.Context) {
		c.JSON(200, version.Get("details-subscription-service"))
	})

	public := router.Group("/api/v3")
	{
		public.GET("/version", func(c *gin.Context) {
			c.JSON(200, version.Get("details-subscription-service"))
		})
		public.GET("/health", h.HealthCheck)
	}

	protected := router.Group("/api/v3")
	protected.Use(middleware.AuthMiddleware(cfg, db))
	{
		protected.POST("/subscriptions", h.CreateSubscription)
		protected.GET("/subscriptions", h.ListSubscriptions)
		protected.GET("/subscriptions/:id", h.GetSubscription)
		protected.PATCH("/subscriptions/:id", h.UpdateSubscription)
		protected.DELETE("/subscriptions/:id", h.CancelSubscription)
		protected.GET("/subscriptions/:id/reports", h.ListSubscriptionReports)
		protected.POST("/subscriptions/:id/preview", h.PreviewSubscription)
	}

	webhooks := router.Group("/api/v3/webhooks")
	{
		webhooks.POST("/stripe", h.HandleStripeWebhook)
	}

	return router
}
