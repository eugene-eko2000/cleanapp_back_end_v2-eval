package models

import "time"

type DetailSubscription struct {
	ID                    int        `json:"id"`
	UserID                string     `json:"user_id"`
	Email                 string     `json:"email"`
	AreaGeoJSON           string     `json:"area_geojson"`
	AreaName              string     `json:"area_name"`
	StripeCustomerID      string     `json:"stripe_customer_id"`
	StripeSubscriptionID  string     `json:"stripe_subscription_id"`
	StripePaymentMethodID string     `json:"stripe_payment_method_id"`
	Status                string     `json:"status"`
	LastReportAt          *time.Time `json:"last_report_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type DetailSubscriptionReport struct {
	ID             int       `json:"id"`
	SubscriptionID int       `json:"subscription_id"`
	ReportCount    int       `json:"report_count"`
	PeriodStart    time.Time `json:"period_start"`
	PeriodEnd      time.Time `json:"period_end"`
	SentAt         time.Time `json:"sent_at"`
	Status         string    `json:"status"`
	ErrorMessage   string    `json:"error_message,omitempty"`
}

type AreaReport struct {
	Seq              int       `json:"seq"`
	PublicID         string    `json:"public_id"`
	Timestamp        time.Time `json:"timestamp"`
	Latitude         float64   `json:"latitude"`
	Longitude        float64   `json:"longitude"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	Summary          string    `json:"summary"`
	SeverityLevel    float64   `json:"severity_level"`
	Classification   string    `json:"classification"`
	BrandName        string    `json:"brand_name"`
	BrandDisplayName string    `json:"brand_display_name"`
}

type CreateSubscriptionRequest struct {
	Email                 string      `json:"email" binding:"required,email"`
	AreaGeoJSON           interface{} `json:"area_geojson" binding:"required"`
	AreaName              string      `json:"area_name" binding:"required"`
	StripePaymentMethodID string      `json:"stripe_payment_method_id" binding:"required"`
}

type UpdateSubscriptionRequest struct {
	Email       *string     `json:"email,omitempty"`
	AreaGeoJSON interface{} `json:"area_geojson,omitempty"`
	AreaName    *string     `json:"area_name,omitempty"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type PreviewResponse struct {
	Reports    []AreaReport `json:"reports"`
	TotalCount int          `json:"total_count"`
	AreaName   string       `json:"area_name"`
}
