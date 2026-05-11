package utils

import (
	"details-subscription-service/config"
	"fmt"
	"log"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/customer"
	"github.com/stripe/stripe-go/v82/paymentmethod"
	"github.com/stripe/stripe-go/v82/subscription"
	"github.com/stripe/stripe-go/v82/webhook"
)

type StripeClient struct {
	config *config.Config
}

func NewStripeClient(cfg *config.Config) *StripeClient {
	if cfg.StripeSecretKey == "" {
		log.Fatal("STRIPE_SECRET_KEY is not set")
	}

	stripe.Key = cfg.StripeSecretKey
	log.Printf("Stripe client initialized with key starting with: %s...", cfg.StripeSecretKey[:7])

	return &StripeClient{config: cfg}
}

func (c *StripeClient) CreateCustomer(email, userID string) (*stripe.Customer, error) {
	params := &stripe.CustomerParams{
		Email: stripe.String(email),
		Metadata: map[string]string{
			"user_id": userID,
			"service": "details-subscription",
		},
	}

	result, err := customer.New(params)
	if err != nil {
		log.Printf("Failed to create Stripe customer: %v", err)
		return nil, fmt.Errorf("failed to create Stripe customer: %w", err)
	}
	return result, nil
}

func (c *StripeClient) AttachPaymentMethod(paymentMethodID, customerID string) (*stripe.PaymentMethod, error) {
	params := &stripe.PaymentMethodAttachParams{
		Customer: stripe.String(customerID),
	}

	result, err := paymentmethod.Attach(paymentMethodID, params)
	if err != nil {
		log.Printf("Failed to attach payment method %s to customer %s: %v", paymentMethodID, customerID, err)
		return nil, fmt.Errorf("failed to attach payment method: %w", err)
	}
	return result, nil
}

func (c *StripeClient) SetDefaultPaymentMethod(customerID, paymentMethodID string) error {
	params := &stripe.CustomerParams{
		InvoiceSettings: &stripe.CustomerInvoiceSettingsParams{
			DefaultPaymentMethod: stripe.String(paymentMethodID),
		},
	}

	_, err := customer.Update(customerID, params)
	if err != nil {
		log.Printf("Failed to set default payment method for customer %s: %v", customerID, err)
		return fmt.Errorf("failed to set default payment method: %w", err)
	}
	return nil
}

func (c *StripeClient) CreateSubscription(customerID, paymentMethodID string) (*stripe.Subscription, error) {
	priceID := c.config.StripeDetailsPriceID
	if priceID == "" {
		return nil, fmt.Errorf("STRIPE_DETAILS_PRICE_ID is not configured")
	}

	params := &stripe.SubscriptionParams{
		Customer: stripe.String(customerID),
		Items: []*stripe.SubscriptionItemsParams{
			{
				Price: stripe.String(priceID),
			},
		},
		PaymentBehavior:      stripe.String("error_if_incomplete"),
		DefaultPaymentMethod: &paymentMethodID,
		Expand: []*string{
			stripe.String("latest_invoice"),
			stripe.String("latest_invoice.payment_intent"),
		},
		PaymentSettings: &stripe.SubscriptionPaymentSettingsParams{
			SaveDefaultPaymentMethod: stripe.String("on_subscription"),
		},
		Metadata: map[string]string{
			"service": "details-subscription",
		},
	}

	result, err := subscription.New(params)
	if err != nil {
		log.Printf("Failed to create subscription: %v", err)
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}
	return result, nil
}

func (c *StripeClient) CancelSubscription(subscriptionID string) (*stripe.Subscription, error) {
	params := &stripe.SubscriptionCancelParams{
		InvoiceNow: stripe.Bool(true),
		Prorate:    stripe.Bool(true),
	}

	result, err := subscription.Cancel(subscriptionID, params)
	if err != nil {
		log.Printf("Failed to cancel subscription %s: %v", subscriptionID, err)
		return nil, fmt.Errorf("failed to cancel subscription: %w", err)
	}
	return result, nil
}

func (c *StripeClient) ConstructWebhookEvent(payload []byte, header string) (stripe.Event, error) {
	if c.config.StripeWebhookSecret == "" {
		return stripe.Event{}, fmt.Errorf("webhook secret not configured")
	}

	event, err := webhook.ConstructEvent(payload, header, c.config.StripeWebhookSecret)
	if err != nil {
		log.Printf("Failed to construct webhook event: %v", err)
		return stripe.Event{}, fmt.Errorf("failed to construct webhook event: %w", err)
	}
	return event, nil
}
