package database

import (
	"context"
	"database/sql"
	"fmt"

	"cleanapp-common/migrator"
)

func RunMigrations(ctx context.Context, db *sql.DB) error {
	return migrator.Run(ctx, db, "details-subscription-service", []migrator.Step{
		{ID: "0001_detail_subscriptions", Description: "create detail_subscriptions table", Up: createDetailSubscriptionsTable},
		{ID: "0002_detail_subscription_reports", Description: "create detail_subscription_reports table", Up: createDetailSubscriptionReportsTable},
	})
}

func createDetailSubscriptionsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS detail_subscriptions (
			id INT AUTO_INCREMENT PRIMARY KEY,
			user_id VARCHAR(256) NOT NULL,
			email VARCHAR(256) NOT NULL,
			area_geojson JSON NOT NULL,
			area_name VARCHAR(256) NOT NULL DEFAULT '',
			stripe_customer_id VARCHAR(256) NOT NULL DEFAULT '',
			stripe_subscription_id VARCHAR(256) NOT NULL DEFAULT '',
			stripe_payment_method_id VARCHAR(256) NOT NULL DEFAULT '',
			status ENUM('active','paused','cancelled','past_due') NOT NULL DEFAULT 'active',
			last_report_at TIMESTAMP NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			INDEX idx_detail_sub_user_id (user_id),
			INDEX idx_detail_sub_status (status),
			INDEX idx_detail_sub_stripe_sub (stripe_subscription_id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create detail_subscriptions table: %w", err)
	}
	return nil
}

func createDetailSubscriptionReportsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS detail_subscription_reports (
			id INT AUTO_INCREMENT PRIMARY KEY,
			subscription_id INT NOT NULL,
			report_count INT NOT NULL DEFAULT 0,
			period_start TIMESTAMP NOT NULL,
			period_end TIMESTAMP NOT NULL,
			sent_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			status ENUM('sent','failed') NOT NULL DEFAULT 'sent',
			error_message TEXT,
			FOREIGN KEY (subscription_id) REFERENCES detail_subscriptions(id) ON DELETE CASCADE,
			INDEX idx_detail_sub_reports_sub_id (subscription_id),
			INDEX idx_detail_sub_reports_sent_at (sent_at)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create detail_subscription_reports table: %w", err)
	}
	return nil
}
