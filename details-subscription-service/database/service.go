package database

import (
	"context"
	"database/sql"
	"details-subscription-service/models"
	"fmt"
	"time"
)

type SubscriptionService struct {
	db *sql.DB
}

func NewSubscriptionService(db *sql.DB) *SubscriptionService {
	return &SubscriptionService{db: db}
}

func (s *SubscriptionService) CreateSubscription(ctx context.Context, userID, email, areaGeoJSON, areaName, stripeCustomerID, stripeSubscriptionID, stripePaymentMethodID string) (*models.DetailSubscription, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO detail_subscriptions (user_id, email, area_geojson, area_name, stripe_customer_id, stripe_subscription_id, stripe_payment_method_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, email, areaGeoJSON, areaName, stripeCustomerID, stripeSubscriptionID, stripePaymentMethodID)
	if err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get insert id: %w", err)
	}

	return s.GetSubscriptionByID(ctx, int(id), userID)
}

func (s *SubscriptionService) GetSubscriptionsByUserID(ctx context.Context, userID string) ([]models.DetailSubscription, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, email, area_geojson, area_name, stripe_customer_id, stripe_subscription_id, stripe_payment_method_id, status, last_report_at, created_at, updated_at
		FROM detail_subscriptions
		WHERE user_id = ?
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []models.DetailSubscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (s *SubscriptionService) GetSubscriptionByID(ctx context.Context, id int, userID string) (*models.DetailSubscription, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, email, area_geojson, area_name, stripe_customer_id, stripe_subscription_id, stripe_payment_method_id, status, last_report_at, created_at, updated_at
		FROM detail_subscriptions
		WHERE id = ? AND user_id = ?`, id, userID)

	sub, err := scanSubscriptionRow(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("subscription not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}
	return &sub, nil
}

func (s *SubscriptionService) UpdateSubscription(ctx context.Context, id int, userID string, req models.UpdateSubscriptionRequest) error {
	query := "UPDATE detail_subscriptions SET "
	args := []interface{}{}
	setClauses := []string{}

	if req.Email != nil {
		setClauses = append(setClauses, "email = ?")
		args = append(args, *req.Email)
	}
	if req.AreaGeoJSON != nil {
		setClauses = append(setClauses, "area_geojson = ?")
		args = append(args, fmt.Sprintf("%v", req.AreaGeoJSON))
	}
	if req.AreaName != nil {
		setClauses = append(setClauses, "area_name = ?")
		args = append(args, *req.AreaName)
	}

	if len(setClauses) == 0 {
		return nil
	}

	for i, clause := range setClauses {
		if i > 0 {
			query += ", "
		}
		query += clause
	}
	query += " WHERE id = ? AND user_id = ?"
	args = append(args, id, userID)

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("subscription not found")
	}
	return nil
}

func (s *SubscriptionService) CancelSubscription(ctx context.Context, id int, userID string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE detail_subscriptions SET status = 'cancelled'
		WHERE id = ? AND user_id = ? AND status IN ('active','paused','past_due')`, id, userID)
	if err != nil {
		return fmt.Errorf("failed to cancel subscription: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("subscription not found or already cancelled")
	}
	return nil
}

func (s *SubscriptionService) GetActiveSubscriptions(ctx context.Context) ([]models.DetailSubscription, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, email, area_geojson, area_name, stripe_customer_id, stripe_subscription_id, stripe_payment_method_id, status, last_report_at, created_at, updated_at
		FROM detail_subscriptions
		WHERE status = 'active'`)
	if err != nil {
		return nil, fmt.Errorf("failed to query active subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []models.DetailSubscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (s *SubscriptionService) UpdateLastReportAt(ctx context.Context, id int, t time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE detail_subscriptions SET last_report_at = ? WHERE id = ?`, t, id)
	if err != nil {
		return fmt.Errorf("failed to update last_report_at: %w", err)
	}
	return nil
}

func (s *SubscriptionService) LogSubscriptionReport(ctx context.Context, subscriptionID, reportCount int, periodStart, periodEnd time.Time, status, errorMsg string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO detail_subscription_reports (subscription_id, report_count, period_start, period_end, status, error_message)
		VALUES (?, ?, ?, ?, ?, ?)`,
		subscriptionID, reportCount, periodStart, periodEnd, status, errorMsg)
	if err != nil {
		return fmt.Errorf("failed to log subscription report: %w", err)
	}
	return nil
}

func (s *SubscriptionService) UpdateSubscriptionStatus(ctx context.Context, stripeSubscriptionID, status string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE detail_subscriptions SET status = ? WHERE stripe_subscription_id = ?`, status, stripeSubscriptionID)
	if err != nil {
		return fmt.Errorf("failed to update subscription status: %w", err)
	}
	return nil
}

func (s *SubscriptionService) GetSubscriptionReports(ctx context.Context, subscriptionID int, userID string) ([]models.DetailSubscriptionReport, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT dsr.id, dsr.subscription_id, dsr.report_count, dsr.period_start, dsr.period_end, dsr.sent_at, dsr.status, COALESCE(dsr.error_message, '')
		FROM detail_subscription_reports dsr
		INNER JOIN detail_subscriptions ds ON dsr.subscription_id = ds.id
		WHERE ds.id = ? AND ds.user_id = ?
		ORDER BY dsr.sent_at DESC`, subscriptionID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscription reports: %w", err)
	}
	defer rows.Close()

	var reports []models.DetailSubscriptionReport
	for rows.Next() {
		var r models.DetailSubscriptionReport
		if err := rows.Scan(&r.ID, &r.SubscriptionID, &r.ReportCount, &r.PeriodStart, &r.PeriodEnd, &r.SentAt, &r.Status, &r.ErrorMessage); err != nil {
			return nil, fmt.Errorf("failed to scan subscription report: %w", err)
		}
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

func (s *SubscriptionService) GetReportsInArea(ctx context.Context, areaGeoJSON string, since time.Time) ([]models.AreaReport, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.seq, r.public_id, r.ts, r.latitude, r.longitude,
		       COALESCE(ra.title, ''), COALESCE(ra.description, ''), COALESCE(ra.summary, ''),
		       COALESCE(ra.severity_level, 0), COALESCE(ra.classification, ''),
		       COALESCE(ra.brand_name, ''), COALESCE(ra.brand_display_name, '')
		FROM reports r
		JOIN reports_geometry rg ON r.seq = rg.seq
		INNER JOIN report_analysis ra ON r.seq = ra.seq
		LEFT JOIN report_raw rr ON r.seq = rr.report_seq
		LEFT JOIN report_status rs ON r.seq = rs.seq
		LEFT JOIN reports_owners ro ON r.seq = ro.seq
		WHERE ST_Within(rg.geom, ST_SRID(ST_GeomFromGeoJSON(?), 4326))
		  AND (rs.status IS NULL OR rs.status = 'active')
		  AND ra.is_valid = TRUE
		  AND (rr.visibility IS NULL OR rr.visibility = 'public')
		  AND (ro.owner IS NULL OR ro.owner = '' OR ro.is_public = TRUE)
		  AND r.ts > ?
		ORDER BY r.ts DESC
		LIMIT 10000`, areaGeoJSON, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query reports in area: %w", err)
	}
	defer rows.Close()

	var reports []models.AreaReport
	for rows.Next() {
		var r models.AreaReport
		if err := rows.Scan(&r.Seq, &r.PublicID, &r.Timestamp, &r.Latitude, &r.Longitude,
			&r.Title, &r.Description, &r.Summary, &r.SeverityLevel, &r.Classification,
			&r.BrandName, &r.BrandDisplayName); err != nil {
			return nil, fmt.Errorf("failed to scan area report: %w", err)
		}
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanSubscriptionFields(s scannable) (models.DetailSubscription, error) {
	var sub models.DetailSubscription
	err := s.Scan(&sub.ID, &sub.UserID, &sub.Email, &sub.AreaGeoJSON, &sub.AreaName,
		&sub.StripeCustomerID, &sub.StripeSubscriptionID, &sub.StripePaymentMethodID,
		&sub.Status, &sub.LastReportAt, &sub.CreatedAt, &sub.UpdatedAt)
	return sub, err
}

func scanSubscription(rows *sql.Rows) (models.DetailSubscription, error) {
	sub, err := scanSubscriptionFields(rows)
	if err != nil {
		return sub, fmt.Errorf("failed to scan subscription: %w", err)
	}
	return sub, nil
}

func scanSubscriptionRow(row *sql.Row) (models.DetailSubscription, error) {
	sub, err := scanSubscriptionFields(row)
	if err != nil {
		return sub, err
	}
	return sub, nil
}
