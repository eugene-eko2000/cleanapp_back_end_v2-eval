package handlers

import (
	"context"
	"details-subscription-service/config"
	"details-subscription-service/models"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v82"
)

type SubscriptionServiceInterface interface {
	CreateSubscription(ctx context.Context, userID, email, areaGeoJSON, areaName, stripeCustomerID, stripeSubscriptionID, stripePaymentMethodID string) (*models.DetailSubscription, error)
	GetSubscriptionsByUserID(ctx context.Context, userID string) ([]models.DetailSubscription, error)
	GetSubscriptionByID(ctx context.Context, id int, userID string) (*models.DetailSubscription, error)
	UpdateSubscription(ctx context.Context, id int, userID string, req models.UpdateSubscriptionRequest) error
	CancelSubscription(ctx context.Context, id int, userID string) error
	UpdateSubscriptionStatus(ctx context.Context, stripeSubscriptionID, status string) error
	GetSubscriptionReports(ctx context.Context, subscriptionID int, userID string) ([]models.DetailSubscriptionReport, error)
	GetReportsInArea(ctx context.Context, areaGeoJSON string, since time.Time) ([]models.AreaReport, error)
}

type StripeClientInterface interface {
	CreateCustomer(email, userID string) (*stripe.Customer, error)
	AttachPaymentMethod(paymentMethodID, customerID string) (*stripe.PaymentMethod, error)
	SetDefaultPaymentMethod(customerID, paymentMethodID string) error
	CreateSubscription(customerID, paymentMethodID string) (*stripe.Subscription, error)
	CancelSubscription(subscriptionID string) (*stripe.Subscription, error)
	ConstructWebhookEvent(payload []byte, header string) (stripe.Event, error)
}

type EmailSenderInterface interface {
	SendReportPDF(recipientEmail, areaName string, periodStart, periodEnd time.Time, pdfBytes []byte) error
}

type Handlers struct {
	service      SubscriptionServiceInterface
	stripeClient StripeClientInterface
	emailSender  EmailSenderInterface
	config       *config.Config
}

func NewHandlers(service SubscriptionServiceInterface, stripeClient StripeClientInterface, emailSender EmailSenderInterface, cfg *config.Config) *Handlers {
	return &Handlers{
		service:      service,
		stripeClient: stripeClient,
		emailSender:  emailSender,
		config:       cfg,
	}
}

func (h *Handlers) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy", "service": "details-subscription-service"})
}

func (h *Handlers) CreateSubscription(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized"})
		return
	}

	var req models.CreateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	areaJSON, err := json.Marshal(req.AreaGeoJSON)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid area_geojson"})
		return
	}

	stripeCustomer, err := h.stripeClient.CreateCustomer(req.Email, userID)
	if err != nil {
		log.Printf("ERROR: Stripe customer creation failed for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to create payment customer"})
		return
	}

	_, err = h.stripeClient.AttachPaymentMethod(req.StripePaymentMethodID, stripeCustomer.ID)
	if err != nil {
		log.Printf("ERROR: Stripe payment method attach failed for user %s: %v", userID, err)
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "failed to attach payment method"})
		return
	}

	if err := h.stripeClient.SetDefaultPaymentMethod(stripeCustomer.ID, req.StripePaymentMethodID); err != nil {
		log.Printf("ERROR: Stripe set default PM failed for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to set default payment method"})
		return
	}

	stripeSub, err := h.stripeClient.CreateSubscription(stripeCustomer.ID, req.StripePaymentMethodID)
	if err != nil {
		log.Printf("ERROR: Stripe subscription creation failed for user %s: %v", userID, err)
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "failed to create subscription — payment may have been declined"})
		return
	}

	sub, err := h.service.CreateSubscription(c.Request.Context(), userID, req.Email, string(areaJSON), req.AreaName,
		stripeCustomer.ID, stripeSub.ID, req.StripePaymentMethodID)
	if err != nil {
		log.Printf("ERROR: DB subscription creation failed for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to save subscription"})
		return
	}

	c.JSON(http.StatusCreated, sub)
}

func (h *Handlers) ListSubscriptions(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized"})
		return
	}

	subs, err := h.service.GetSubscriptionsByUserID(c.Request.Context(), userID)
	if err != nil {
		log.Printf("ERROR: Failed to list subscriptions for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to list subscriptions"})
		return
	}

	if subs == nil {
		subs = []models.DetailSubscription{}
	}
	c.JSON(http.StatusOK, subs)
}

func (h *Handlers) GetSubscription(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid subscription id"})
		return
	}

	sub, err := h.service.GetSubscriptionByID(c.Request.Context(), id, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "subscription not found"})
		return
	}

	c.JSON(http.StatusOK, sub)
}

func (h *Handlers) UpdateSubscription(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid subscription id"})
		return
	}

	var req models.UpdateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.service.UpdateSubscription(c.Request.Context(), id, userID, req); err != nil {
		if err.Error() == "subscription not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
			return
		}
		log.Printf("ERROR: Failed to update subscription %d for user %s: %v", id, userID, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to update subscription"})
		return
	}

	sub, err := h.service.GetSubscriptionByID(c.Request.Context(), id, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to retrieve updated subscription"})
		return
	}

	c.JSON(http.StatusOK, sub)
}

func (h *Handlers) CancelSubscription(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid subscription id"})
		return
	}

	sub, err := h.service.GetSubscriptionByID(c.Request.Context(), id, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "subscription not found"})
		return
	}

	if sub.StripeSubscriptionID != "" {
		if _, err := h.stripeClient.CancelSubscription(sub.StripeSubscriptionID); err != nil {
			log.Printf("ERROR: Stripe subscription cancellation failed for subscription %d: %v", id, err)
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to cancel payment subscription"})
			return
		}
	}

	if err := h.service.CancelSubscription(c.Request.Context(), id, userID); err != nil {
		log.Printf("ERROR: DB subscription cancellation failed for subscription %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to cancel subscription"})
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse{Message: "subscription cancelled"})
}

func (h *Handlers) ListSubscriptionReports(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid subscription id"})
		return
	}

	reports, err := h.service.GetSubscriptionReports(c.Request.Context(), id, userID)
	if err != nil {
		log.Printf("ERROR: Failed to list subscription reports for subscription %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to list reports"})
		return
	}

	if reports == nil {
		reports = []models.DetailSubscriptionReport{}
	}
	c.JSON(http.StatusOK, reports)
}

func (h *Handlers) PreviewSubscription(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid subscription id"})
		return
	}

	sub, err := h.service.GetSubscriptionByID(c.Request.Context(), id, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "subscription not found"})
		return
	}

	since := time.Now().Add(-24 * time.Hour)
	reports, err := h.service.GetReportsInArea(c.Request.Context(), sub.AreaGeoJSON, since)
	if err != nil {
		log.Printf("ERROR: Failed to preview reports for subscription %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to preview reports"})
		return
	}

	if reports == nil {
		reports = []models.AreaReport{}
	}

	c.JSON(http.StatusOK, models.PreviewResponse{
		Reports:    reports,
		TotalCount: len(reports),
		AreaName:   sub.AreaName,
	})
}

func (h *Handlers) HandleStripeWebhook(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("ERROR: Failed to read webhook body: %v", err)
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "failed to read request body"})
		return
	}

	sigHeader := c.GetHeader("Stripe-Signature")
	event, err := h.stripeClient.ConstructWebhookEvent(payload, sigHeader)
	if err != nil {
		log.Printf("ERROR: Webhook signature verification failed: %v", err)
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid webhook signature"})
		return
	}

	log.Printf("Received Stripe webhook event: %s", event.Type)

	switch event.Type {
	case "customer.subscription.updated":
		var sub struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			log.Printf("ERROR: Failed to parse subscription event: %v", err)
			break
		}
		status := mapStripeStatus(sub.Status)
		if err := h.service.UpdateSubscriptionStatus(c.Request.Context(), sub.ID, status); err != nil {
			log.Printf("ERROR: Failed to update subscription status for %s: %v", sub.ID, err)
		}

	case "customer.subscription.deleted":
		var sub struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			log.Printf("ERROR: Failed to parse subscription deleted event: %v", err)
			break
		}
		if err := h.service.UpdateSubscriptionStatus(c.Request.Context(), sub.ID, "cancelled"); err != nil {
			log.Printf("ERROR: Failed to cancel subscription %s: %v", sub.ID, err)
		}

	case "invoice.payment_failed":
		var invoice struct {
			Subscription string `json:"subscription"`
		}
		if err := json.Unmarshal(event.Data.Raw, &invoice); err != nil {
			log.Printf("ERROR: Failed to parse invoice event: %v", err)
			break
		}
		if invoice.Subscription != "" {
			if err := h.service.UpdateSubscriptionStatus(c.Request.Context(), invoice.Subscription, "past_due"); err != nil {
				log.Printf("ERROR: Failed to mark subscription %s as past_due: %v", invoice.Subscription, err)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}

func mapStripeStatus(stripeStatus string) string {
	switch stripeStatus {
	case "active":
		return "active"
	case "past_due":
		return "past_due"
	case "canceled", "cancelled":
		return "cancelled"
	case "paused":
		return "paused"
	default:
		return "active"
	}
}
