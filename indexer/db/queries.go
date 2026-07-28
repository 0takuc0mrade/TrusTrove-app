package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type DbInvoice struct {
	ID               string `json:"id"`
	Issuer           string `json:"issuer"`
	Buyer            string `json:"buyer"`
	FaceValue        string `json:"face_value"` // BigInt represented as string for JSON/SQL numeric safety
	DiscountBps      int    `json:"discount_bps"`
	FundedAmount     string `json:"funded_amount"`
	DueDate          int64  `json:"due_date"`
	Status           string `json:"status"`
	CreatedAt        int64  `json:"created_at"`
	FundedAt         *int64 `json:"funded_at"`
	ShippedAt        *int64 `json:"shipped_at"`
	IssuerConfirmed  bool   `json:"issuer_confirmed"`
	BuyerConfirmed   bool   `json:"buyer_confirmed"`
	BuyerConfirmedAt *int64 `json:"buyer_confirmed_at"`
	RepaidAt         *int64 `json:"repaid_at"`
}

type DbPoolStats struct {
	TotalDeposits         string    `json:"total_deposits"`
	TotalFunded           string    `json:"total_funded"`
	AvailableLiquidity    string    `json:"available_liquidity"`
	UtilizationRateBps    int       `json:"utilization_rate_bps"`
	TotalYieldDistributed string    `json:"total_yield_distributed"`
	ActiveInvoiceCount    int       `json:"active_invoice_count"`
	TotalShares           string    `json:"total_shares"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type ProtocolStats struct {
	TotalUSDCFinanced  string `json:"total_usdc_financed"`
	ActiveInvoiceCount int    `json:"active_invoice_count"`
	TotalInvoices      int    `json:"total_invoices"`
	TotalRepaid        int    `json:"total_repaid"`
	TotalDefaulted     int    `json:"total_defaulted"`
	AverageYieldBps    int    `json:"average_yield_bps"`
	PoolUtilizationBps int    `json:"pool_utilization_bps"`
	RegisteredIssuers  int    `json:"registered_issuers"`
}

func GetProtocolStats(ctx context.Context) (*ProtocolStats, error) {
	query := `
		SELECT
			COALESCE(SUM(funded_amount) FILTER (WHERE status IN ('funded', 'shipped', 'confirmed', 'repaid')), 0)::TEXT AS total_usdc_financed,
			COUNT(*) FILTER (WHERE status IN ('funded', 'shipped', 'confirmed')) AS active_invoice_count,
			COUNT(*) AS total_invoices,
			COUNT(*) FILTER (WHERE status = 'repaid') AS total_repaid,
			COUNT(*) FILTER (WHERE status = 'defaulted') AS total_defaulted,
			COALESCE(AVG(discount_bps) FILTER (WHERE status IN ('funded', 'shipped', 'confirmed', 'repaid')), 0)::INTEGER AS average_yield_bps,
			COALESCE((SELECT utilization_rate_bps FROM pool_snapshots WHERE id = 1), 0) AS pool_utilization_bps,
			COUNT(DISTINCT issuer) AS registered_issuers
		FROM invoices
	`
	var stats ProtocolStats
	err := Pool.QueryRow(ctx, query).Scan(
		&stats.TotalUSDCFinanced,
		&stats.ActiveInvoiceCount,
		&stats.TotalInvoices,
		&stats.TotalRepaid,
		&stats.TotalDefaulted,
		&stats.AverageYieldBps,
		&stats.PoolUtilizationBps,
		&stats.RegisteredIssuers,
	)
	if err != nil {
		return nil, fmt.Errorf("queries: get protocol stats: %w", err)
	}
	return &stats, nil
}

func InsertInvoice(ctx context.Context, inv *DbInvoice) error {
	query := `
		INSERT INTO invoices (
			id, issuer, buyer, face_value, discount_bps, funded_amount, due_date, status, created_at,
			funded_at, shipped_at, issuer_confirmed, buyer_confirmed, buyer_confirmed_at, repaid_at
		) VALUES (
			@id, @issuer, @buyer, @face_value, @discount_bps, @funded_amount, @due_date, @status, @created_at,
			@funded_at, @shipped_at, @issuer_confirmed, @buyer_confirmed, @buyer_confirmed_at, @repaid_at
		)
	`
	args := pgx.NamedArgs{
		"id": inv.ID, "issuer": inv.Issuer, "buyer": inv.Buyer, "face_value": inv.FaceValue,
		"discount_bps": inv.DiscountBps, "funded_amount": inv.FundedAmount, "due_date": inv.DueDate,
		"status": inv.Status, "created_at": inv.CreatedAt, "funded_at": inv.FundedAt,
		"shipped_at": inv.ShippedAt, "issuer_confirmed": inv.IssuerConfirmed,
		"buyer_confirmed": inv.BuyerConfirmed, "buyer_confirmed_at": inv.BuyerConfirmedAt,
		"repaid_at": inv.RepaidAt,
	}
	if _, err := Pool.Exec(ctx, query, args); err != nil {
		return fmt.Errorf("queries: insert invoice: %w", err)
	}
	return nil
}

func GetInvoiceByID(ctx context.Context, id string) (*DbInvoice, error) {
	query := `
		SELECT id, issuer, buyer, face_value, discount_bps, funded_amount, due_date, status, created_at,
			funded_at, shipped_at, issuer_confirmed, buyer_confirmed, buyer_confirmed_at, repaid_at
		FROM invoices WHERE id = $1
	`
	var inv DbInvoice
	err := Pool.QueryRow(ctx, query, id).Scan(
		&inv.ID, &inv.Issuer, &inv.Buyer, &inv.FaceValue, &inv.DiscountBps, &inv.FundedAmount,
		&inv.DueDate, &inv.Status, &inv.CreatedAt, &inv.FundedAt, &inv.ShippedAt,
		&inv.IssuerConfirmed, &inv.BuyerConfirmed, &inv.BuyerConfirmedAt, &inv.RepaidAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("queries: get invoice by id: %w", err)
	}
	return &inv, nil
}

func GetInvoicesPage(ctx context.Context, status, issuer string, limit, offset int) ([]*DbInvoice, int, error) {
	countQuery := `
		SELECT COUNT(*) FROM invoices
		WHERE ($1 = '' OR status = $1) AND ($2 = '' OR issuer = $2)
	`
	var total int
	if err := Pool.QueryRow(ctx, countQuery, status, issuer).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("queries: count invoices: %w", err)
	}

	query := `
		SELECT id, issuer, buyer, face_value, discount_bps, funded_amount, due_date, status, created_at,
			funded_at, shipped_at, issuer_confirmed, buyer_confirmed, buyer_confirmed_at, repaid_at
		FROM invoices
		WHERE ($1 = '' OR status = $1) AND ($2 = '' OR issuer = $2)
		ORDER BY created_at DESC LIMIT $3 OFFSET $4
	`
	rows, err := Pool.Query(ctx, query, status, issuer, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("queries: get invoices: %w", err)
	}
	defer rows.Close()

	invoices := make([]*DbInvoice, 0)
	for rows.Next() {
		var inv DbInvoice
		if err := rows.Scan(
			&inv.ID, &inv.Issuer, &inv.Buyer, &inv.FaceValue, &inv.DiscountBps, &inv.FundedAmount,
			&inv.DueDate, &inv.Status, &inv.CreatedAt, &inv.FundedAt, &inv.ShippedAt,
			&inv.IssuerConfirmed, &inv.BuyerConfirmed, &inv.BuyerConfirmedAt, &inv.RepaidAt,
		); err != nil {
			return nil, 0, fmt.Errorf("queries: scan invoice: %w", err)
		}
		invoices = append(invoices, &inv)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("queries: iterate invoices: %w", err)
	}
	return invoices, total, nil
}

// AreEventsProcessed returns the event IDs from ids that have already been
// recorded. It deliberately performs one query for the whole polling batch.
func AreEventsProcessed(ctx context.Context, ids []string) (map[string]bool, error) {
	processed := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return processed, nil
	}

	rows, err := Pool.Query(ctx, `SELECT event_id FROM events_log WHERE event_id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("queries: check processed events: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("queries: scan processed event: %w", err)
		}
		processed[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("queries: iterate processed events: %w", err)
	}
	return processed, nil
}

func IsEventProcessed(ctx context.Context, id string) (bool, error) {
	processed, err := AreEventsProcessed(ctx, []string{id})
	if err != nil {
		return false, err
	}
	return processed[id], nil
}
