package handlers

import (
	"bytes"
	"context"
	"details-subscription-service/config"
	"details-subscription-service/models"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v82"
)

// --- Mock implementations ---

type mockSubscriptionService struct {
	mu      sync.Mutex
	subs    map[int]models.DetailSubscription
	reports map[int][]models.DetailSubscriptionReport
	nextID  int
}

func newMockSubscriptionService() *mockSubscriptionService {
	return &mockSubscriptionService{
		subs:    make(map[int]models.DetailSubscription),
		reports: make(map[int][]models.DetailSubscriptionReport),
		nextID:  1,
	}
}

func (m *mockSubscriptionService) CreateSubscription(_ context.Context, userID, email, areaGeoJSON, areaName, stripeCustomerID, stripeSubscriptionID, stripePaymentMethodID string) (*models.DetailSubscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	sub := models.DetailSubscription{
		ID:                    m.nextID,
		UserID:                userID,
		Email:                 email,
		AreaGeoJSON:           areaGeoJSON,
		AreaName:              areaName,
		StripeCustomerID:      stripeCustomerID,
		StripeSubscriptionID:  stripeSubscriptionID,
		StripePaymentMethodID: stripePaymentMethodID,
		Status:                "active",
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	m.subs[m.nextID] = sub
	m.nextID++
	return &sub, nil
}

func (m *mockSubscriptionService) GetSubscriptionsByUserID(_ context.Context, userID string) ([]models.DetailSubscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []models.DetailSubscription
	for _, sub := range m.subs {
		if sub.UserID == userID {
			result = append(result, sub)
		}
	}
	return result, nil
}

func (m *mockSubscriptionService) GetSubscriptionByID(_ context.Context, id int, userID string) (*models.DetailSubscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, ok := m.subs[id]
	if !ok || sub.UserID != userID {
		return nil, fmt.Errorf("subscription not found")
	}
	return &sub, nil
}

func (m *mockSubscriptionService) UpdateSubscription(_ context.Context, id int, userID string, req models.UpdateSubscriptionRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, ok := m.subs[id]
	if !ok || sub.UserID != userID {
		return fmt.Errorf("subscription not found")
	}
	if req.Email != nil {
		sub.Email = *req.Email
	}
	if req.AreaName != nil {
		sub.AreaName = *req.AreaName
	}
	if req.AreaGeoJSON != nil {
		b, _ := json.Marshal(req.AreaGeoJSON)
		sub.AreaGeoJSON = string(b)
	}
	sub.UpdatedAt = time.Now()
	m.subs[id] = sub
	return nil
}

func (m *mockSubscriptionService) CancelSubscription(_ context.Context, id int, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, ok := m.subs[id]
	if !ok || sub.UserID != userID {
		return fmt.Errorf("subscription not found")
	}
	sub.Status = "cancelled"
	m.subs[id] = sub
	return nil
}

func (m *mockSubscriptionService) UpdateSubscriptionStatus(_ context.Context, stripeSubscriptionID, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, sub := range m.subs {
		if sub.StripeSubscriptionID == stripeSubscriptionID {
			sub.Status = status
			m.subs[id] = sub
			return nil
		}
	}
	return nil
}

func (m *mockSubscriptionService) GetSubscriptionReports(_ context.Context, subscriptionID int, userID string) ([]models.DetailSubscriptionReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, ok := m.subs[subscriptionID]
	if !ok || sub.UserID != userID {
		return nil, fmt.Errorf("subscription not found")
	}
	return m.reports[subscriptionID], nil
}

func (m *mockSubscriptionService) GetReportsInArea(_ context.Context, _ string, _ time.Time) ([]models.AreaReport, error) {
	return []models.AreaReport{
		{
			Seq:            1,
			PublicID:       "rpt_abc123",
			Timestamp:      time.Now().Add(-1 * time.Hour),
			Latitude:       40.7128,
			Longitude:      -74.0060,
			Title:          "Litter on sidewalk",
			Description:    "Pile of trash bags left on the corner",
			Summary:        "Trash bags on sidewalk corner",
			SeverityLevel:  0.6,
			Classification: "physical",
			BrandName:      "",
		},
		{
			Seq:            2,
			PublicID:       "rpt_def456",
			Timestamp:      time.Now().Add(-30 * time.Minute),
			Latitude:       40.7130,
			Longitude:      -74.0055,
			Title:          "Broken glass",
			Description:    "Shattered bottle on bike lane",
			Summary:        "Broken glass on bike lane",
			SeverityLevel:  0.8,
			Classification: "physical",
			BrandName:      "acme",
		},
	}, nil
}

type mockStripeClient struct{}

func (m *mockStripeClient) CreateCustomer(email, userID string) (*stripe.Customer, error) {
	return &stripe.Customer{ID: "cus_mock_123", Email: email}, nil
}

func (m *mockStripeClient) AttachPaymentMethod(paymentMethodID, customerID string) (*stripe.PaymentMethod, error) {
	return &stripe.PaymentMethod{ID: paymentMethodID, Customer: &stripe.Customer{ID: customerID}}, nil
}

func (m *mockStripeClient) SetDefaultPaymentMethod(_, _ string) error {
	return nil
}

func (m *mockStripeClient) CreateSubscription(customerID, _ string) (*stripe.Subscription, error) {
	return &stripe.Subscription{ID: "sub_mock_456", Customer: &stripe.Customer{ID: customerID}, Status: "active"}, nil
}

func (m *mockStripeClient) CancelSubscription(subscriptionID string) (*stripe.Subscription, error) {
	return &stripe.Subscription{ID: subscriptionID, Status: "canceled"}, nil
}

func (m *mockStripeClient) ConstructWebhookEvent(payload []byte, _ string) (stripe.Event, error) {
	var event stripe.Event
	if err := json.Unmarshal(payload, &event); err != nil {
		return stripe.Event{}, err
	}
	return event, nil
}

type mockEmailSender struct{}

func (m *mockEmailSender) SendReportPDF(_, _ string, _, _ time.Time, _ []byte) error {
	return nil
}

// --- Test helpers ---

func setupTestRouter(h *Handlers) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	authed := router.Group("/api/v3")
	authed.Use(func(c *gin.Context) {
		c.Set("user_id", "test-user-001")
		c.Next()
	})
	{
		authed.POST("/subscriptions", h.CreateSubscription)
		authed.GET("/subscriptions", h.ListSubscriptions)
		authed.GET("/subscriptions/:id", h.GetSubscription)
		authed.PATCH("/subscriptions/:id", h.UpdateSubscription)
		authed.DELETE("/subscriptions/:id", h.CancelSubscription)
		authed.GET("/subscriptions/:id/reports", h.ListSubscriptionReports)
		authed.POST("/subscriptions/:id/preview", h.PreviewSubscription)
	}

	return router
}

func doRequest(router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	var reqBody *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// --- End-to-end lifecycle test ---

func TestSubscriptionLifecycle(t *testing.T) {
	svc := newMockSubscriptionService()
	h := NewHandlers(svc, &mockStripeClient{}, &mockEmailSender{}, &config.Config{})
	router := setupTestRouter(h)

	// Step 1: Create subscription
	createReq := map[string]interface{}{
		"email":                    "cleaning-co@example.com",
		"area_name":                "Downtown Manhattan",
		"stripe_payment_method_id": "pm_test_visa_4242",
		"area_geojson": map[string]interface{}{
			"type": "Polygon",
			"coordinates": []interface{}{
				[]interface{}{
					[]float64{-74.01, 40.71},
					[]float64{-74.00, 40.71},
					[]float64{-74.00, 40.72},
					[]float64{-74.01, 40.72},
					[]float64{-74.01, 40.71},
				},
			},
		},
	}

	w := doRequest(router, "POST", "/api/v3/subscriptions", createReq)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateSubscription: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created models.DetailSubscription
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("CreateSubscription: failed to parse response: %v", err)
	}
	if created.ID != 1 {
		t.Fatalf("CreateSubscription: expected id 1, got %d", created.ID)
	}
	if created.Email != "cleaning-co@example.com" {
		t.Fatalf("CreateSubscription: expected email cleaning-co@example.com, got %s", created.Email)
	}
	if created.Status != "active" {
		t.Fatalf("CreateSubscription: expected status active, got %s", created.Status)
	}
	if created.StripeCustomerID != "cus_mock_123" {
		t.Fatalf("CreateSubscription: expected stripe customer cus_mock_123, got %s", created.StripeCustomerID)
	}
	if created.StripeSubscriptionID != "sub_mock_456" {
		t.Fatalf("CreateSubscription: expected stripe sub sub_mock_456, got %s", created.StripeSubscriptionID)
	}

	subID := fmt.Sprintf("%d", created.ID)

	// Step 2: List subscriptions
	w = doRequest(router, "GET", "/api/v3/subscriptions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("ListSubscriptions: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var listed []models.DetailSubscription
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatalf("ListSubscriptions: failed to parse response: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("ListSubscriptions: expected 1 subscription, got %d", len(listed))
	}
	if listed[0].AreaName != "Downtown Manhattan" {
		t.Fatalf("ListSubscriptions: expected area name Downtown Manhattan, got %s", listed[0].AreaName)
	}

	// Step 3: Get single subscription
	w = doRequest(router, "GET", "/api/v3/subscriptions/"+subID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GetSubscription: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got models.DetailSubscription
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("GetSubscription: failed to parse response: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("GetSubscription: expected id %d, got %d", created.ID, got.ID)
	}

	// Step 4: Update subscription
	updateReq := map[string]interface{}{
		"email":     "new-email@cleaningservice.com",
		"area_name": "Lower Manhattan",
	}
	w = doRequest(router, "PATCH", "/api/v3/subscriptions/"+subID, updateReq)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateSubscription: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.DetailSubscription
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatalf("UpdateSubscription: failed to parse response: %v", err)
	}
	if updated.Email != "new-email@cleaningservice.com" {
		t.Fatalf("UpdateSubscription: expected email new-email@cleaningservice.com, got %s", updated.Email)
	}
	if updated.AreaName != "Lower Manhattan" {
		t.Fatalf("UpdateSubscription: expected area name Lower Manhattan, got %s", updated.AreaName)
	}

	// Step 5: Preview subscription (returns mock area reports)
	w = doRequest(router, "POST", "/api/v3/subscriptions/"+subID+"/preview", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("PreviewSubscription: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var preview models.PreviewResponse
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil {
		t.Fatalf("PreviewSubscription: failed to parse response: %v", err)
	}
	if preview.TotalCount != 2 {
		t.Fatalf("PreviewSubscription: expected 2 reports, got %d", preview.TotalCount)
	}
	if preview.AreaName != "Lower Manhattan" {
		t.Fatalf("PreviewSubscription: expected area name Lower Manhattan, got %s", preview.AreaName)
	}
	if preview.Reports[0].Title != "Litter on sidewalk" {
		t.Fatalf("PreviewSubscription: expected first report title 'Litter on sidewalk', got %s", preview.Reports[0].Title)
	}

	// Step 6: List subscription reports (empty at first — no scheduler has run)
	w = doRequest(router, "GET", "/api/v3/subscriptions/"+subID+"/reports", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("ListSubscriptionReports: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var subReports []models.DetailSubscriptionReport
	if err := json.Unmarshal(w.Body.Bytes(), &subReports); err != nil {
		t.Fatalf("ListSubscriptionReports: failed to parse response: %v", err)
	}
	if len(subReports) != 0 {
		t.Fatalf("ListSubscriptionReports: expected 0 reports, got %d", len(subReports))
	}

	// Step 7: Cancel subscription
	w = doRequest(router, "DELETE", "/api/v3/subscriptions/"+subID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("CancelSubscription: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var cancelMsg models.MessageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &cancelMsg); err != nil {
		t.Fatalf("CancelSubscription: failed to parse response: %v", err)
	}
	if cancelMsg.Message != "subscription cancelled" {
		t.Fatalf("CancelSubscription: expected message 'subscription cancelled', got %s", cancelMsg.Message)
	}

	// Verify cancelled — get should show cancelled status
	w = doRequest(router, "GET", "/api/v3/subscriptions/"+subID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GetSubscription after cancel: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var cancelled models.DetailSubscription
	if err := json.Unmarshal(w.Body.Bytes(), &cancelled); err != nil {
		t.Fatalf("GetSubscription after cancel: failed to parse response: %v", err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("GetSubscription after cancel: expected status cancelled, got %s", cancelled.Status)
	}
}

func TestCreateSubscription_MissingFields(t *testing.T) {
	svc := newMockSubscriptionService()
	h := NewHandlers(svc, &mockStripeClient{}, &mockEmailSender{}, &config.Config{})
	router := setupTestRouter(h)

	w := doRequest(router, "POST", "/api/v3/subscriptions", map[string]interface{}{
		"email": "test@example.com",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing fields, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetSubscription_NotFound(t *testing.T) {
	svc := newMockSubscriptionService()
	h := NewHandlers(svc, &mockStripeClient{}, &mockEmailSender{}, &config.Config{})
	router := setupTestRouter(h)

	w := doRequest(router, "GET", "/api/v3/subscriptions/999", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetSubscription_InvalidID(t *testing.T) {
	svc := newMockSubscriptionService()
	h := NewHandlers(svc, &mockStripeClient{}, &mockEmailSender{}, &config.Config{})
	router := setupTestRouter(h)

	w := doRequest(router, "GET", "/api/v3/subscriptions/abc", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHealthCheck(t *testing.T) {
	h := NewHandlers(newMockSubscriptionService(), &mockStripeClient{}, &mockEmailSender{}, &config.Config{})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/health", h.HealthCheck)

	w := doRequest(router, "GET", "/health", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["status"] != "healthy" {
		t.Fatalf("expected status healthy, got %s", resp["status"])
	}
}
