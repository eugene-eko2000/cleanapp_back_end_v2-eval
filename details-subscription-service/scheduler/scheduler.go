package scheduler

import (
	"context"
	"details-subscription-service/database"
	"details-subscription-service/utils"
	"log"
	"time"
)

type Scheduler struct {
	service     *database.SubscriptionService
	emailSender *utils.EmailSender
	interval    time.Duration
	stopCh      chan struct{}
	doneCh      chan struct{}
}

func NewScheduler(service *database.SubscriptionService, emailSender *utils.EmailSender, interval time.Duration) *Scheduler {
	return &Scheduler{
		service:     service,
		emailSender: emailSender,
		interval:    interval,
	}
}

func (s *Scheduler) Start() {
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	go s.run()
	log.Printf("Scheduler started with interval %v", s.interval)
}

func (s *Scheduler) Stop() {
	close(s.stopCh)
	<-s.doneCh
	log.Println("Scheduler stopped")
}

func (s *Scheduler) run() {
	defer close(s.doneCh)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.processAllSubscriptions()

	for {
		select {
		case <-ticker.C:
			s.processAllSubscriptions()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) processAllSubscriptions() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	subs, err := s.service.GetActiveSubscriptions(ctx)
	if err != nil {
		log.Printf("ERROR: Failed to get active subscriptions: %v", err)
		return
	}

	if len(subs) == 0 {
		log.Println("No active subscriptions to process")
		return
	}

	log.Printf("Processing %d active subscriptions", len(subs))
	sent, failed := 0, 0

	for _, sub := range subs {
		since := sub.CreatedAt
		if sub.LastReportAt != nil {
			since = *sub.LastReportAt
		}
		periodEnd := time.Now()

		reports, err := s.service.GetReportsInArea(ctx, sub.AreaGeoJSON, since)
		if err != nil {
			log.Printf("ERROR: Failed to query reports for subscription %d: %v", sub.ID, err)
			_ = s.service.LogSubscriptionReport(ctx, sub.ID, 0, since, periodEnd, "failed", err.Error())
			failed++
			continue
		}

		pdfBytes, err := utils.GenerateReportPDF(sub.AreaName, since, periodEnd, reports)
		if err != nil {
			log.Printf("ERROR: Failed to generate PDF for subscription %d: %v", sub.ID, err)
			_ = s.service.LogSubscriptionReport(ctx, sub.ID, len(reports), since, periodEnd, "failed", err.Error())
			failed++
			continue
		}

		if err := s.emailSender.SendReportPDF(sub.Email, sub.AreaName, since, periodEnd, pdfBytes); err != nil {
			log.Printf("ERROR: Failed to send email for subscription %d: %v", sub.ID, err)
			_ = s.service.LogSubscriptionReport(ctx, sub.ID, len(reports), since, periodEnd, "failed", err.Error())
			failed++
			continue
		}

		_ = s.service.UpdateLastReportAt(ctx, sub.ID, periodEnd)
		_ = s.service.LogSubscriptionReport(ctx, sub.ID, len(reports), since, periodEnd, "sent", "")
		sent++
		log.Printf("Sent report for subscription %d (%s): %d reports", sub.ID, sub.AreaName, len(reports))
	}

	log.Printf("Scheduler run complete: %d sent, %d failed out of %d subscriptions", sent, failed, len(subs))
}
