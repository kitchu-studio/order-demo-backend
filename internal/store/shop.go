package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	StatusPendingPayment = "pending_payment"
	StatusPaid           = "paid"
	StatusPreparing      = "preparing"
	StatusReady          = "ready"
	StatusCompleted      = "completed"
	StatusCancelled      = "cancelled"
)

var (
	ErrInvalidItems    = errors.New("invalid order items")
	ErrProductNotFound = errors.New("product not available")
	ErrInvalidStatus   = errors.New("status change not allowed")
	ErrNotCancellable  = errors.New("order can no longer be cancelled")
	maxLines           = 30
	maxQtyPerLine      = 20
	allowedTransitions = map[string][]string{
		StatusPendingPayment: {StatusPaid, StatusCancelled},
		StatusPaid:           {StatusPreparing, StatusCancelled},
		StatusPreparing:      {StatusReady},
		StatusReady:          {StatusCompleted},
	}
)

type Product struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	PriceBaht   int    `json:"price_baht"`
	Available   bool   `json:"available"`
	// ImageURL is a path under /media, empty when the shop has not uploaded a photo.
	ImageURL string `json:"image_url"`
}

const productCols = `id, name, description, category, price_baht, available, image_path`

func scanProduct(row pgx.Row) (*Product, error) {
	var p Product
	var image string
	if err := row.Scan(&p.ID, &p.Name, &p.Description, &p.Category, &p.PriceBaht, &p.Available, &image); err != nil {
		return nil, err
	}
	if image != "" {
		p.ImageURL = "/media/" + image
	}
	return &p, nil
}

type OrderItem struct {
	ProductID     int64  `json:"product_id"`
	Name          string `json:"name"`
	UnitPriceBaht int    `json:"unit_price_baht"`
	Qty           int    `json:"qty"`
}

type Order struct {
	ID        int64       `json:"id"`
	Status    string      `json:"status"`
	TotalBaht int         `json:"total_baht"`
	Note      string      `json:"note"`
	CreatedAt time.Time   `json:"created_at"`
	PaidAt    *time.Time  `json:"paid_at,omitempty"`
	Customer  string      `json:"customer,omitempty"` // filled for admin lists
	Items     []OrderItem `json:"items"`
}

type OrderLine struct {
	ProductID int64
	Qty       int
}

func (s *Store) ListProducts(ctx context.Context) ([]Product, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+productCols+` FROM products ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Product{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (s *Store) SetProductAvailable(ctx context.Context, id int64, available bool) (*Product, error) {
	p, err := scanProduct(s.pool.QueryRow(ctx,
		`UPDATE products SET available = $2 WHERE id = $1 RETURNING `+productCols, id, available))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// SetProductImage stores the new photo's file name and returns the previous one so the
// caller can delete that file.
func (s *Store) SetProductImage(ctx context.Context, id int64, file string) (oldFile string, p *Product, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	err = tx.QueryRow(ctx, `SELECT image_path FROM products WHERE id = $1 FOR UPDATE`, id).Scan(&oldFile)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrNotFound
	}
	if err != nil {
		return "", nil, err
	}

	p, err = scanProduct(tx.QueryRow(ctx,
		`UPDATE products SET image_path = $2 WHERE id = $1 RETURNING `+productCols, id, file))
	if err != nil {
		return "", nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", nil, err
	}
	return oldFile, p, nil
}

// CreateOrder prices the lines from the current menu (the client never sends prices)
// and stores order and items in one transaction.
func (s *Store) CreateOrder(ctx context.Context, userID int64, lines []OrderLine, note string) (*Order, error) {
	if len(lines) == 0 || len(lines) > maxLines {
		return nil, ErrInvalidItems
	}
	merged := map[int64]int{}
	ids := make([]int64, 0, len(lines))
	for _, l := range lines {
		if l.Qty < 1 || l.Qty > maxQtyPerLine {
			return nil, ErrInvalidItems
		}
		if _, seen := merged[l.ProductID]; !seen {
			ids = append(ids, l.ProductID)
		}
		merged[l.ProductID] += l.Qty
		if merged[l.ProductID] > maxQtyPerLine {
			return nil, ErrInvalidItems
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	rows, err := tx.Query(ctx,
		`SELECT id, name, price_baht FROM products WHERE id = ANY($1) AND available`, ids)
	if err != nil {
		return nil, err
	}
	prices := map[int64]OrderItem{}
	for rows.Next() {
		var it OrderItem
		if err := rows.Scan(&it.ProductID, &it.Name, &it.UnitPriceBaht); err != nil {
			rows.Close()
			return nil, err
		}
		prices[it.ProductID] = it
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	order := &Order{Status: StatusPendingPayment, Note: note, Items: make([]OrderItem, 0, len(ids))}
	for _, id := range ids { // keep the customer's order of items
		it, ok := prices[id]
		if !ok {
			return nil, ErrProductNotFound
		}
		it.Qty = merged[id]
		order.TotalBaht += it.UnitPriceBaht * it.Qty
		order.Items = append(order.Items, it)
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO orders (user_id, total_baht, note) VALUES ($1, $2, $3) RETURNING id, created_at`,
		userID, order.TotalBaht, note).Scan(&order.ID, &order.CreatedAt)
	if err != nil {
		return nil, err
	}
	for _, it := range order.Items {
		if _, err := tx.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, name, unit_price_baht, qty) VALUES ($1,$2,$3,$4,$5)`,
			order.ID, it.ProductID, it.Name, it.UnitPriceBaht, it.Qty); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return order, nil
}

func (s *Store) GetOrderForUser(ctx context.Context, userID, orderID int64) (*Order, error) {
	orders, err := s.queryOrders(ctx,
		`WHERE o.id = $1 AND o.user_id = $2`, orderID, userID)
	if err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		return nil, ErrNotFound
	}
	return &orders[0], nil
}

func (s *Store) ListOrdersByUser(ctx context.Context, userID int64) ([]Order, error) {
	return s.queryOrders(ctx, `WHERE o.user_id = $1 ORDER BY o.created_at DESC LIMIT 100`, userID)
}

// ListOrdersAdmin lists orders for the shop. An empty status means every open order
// (anything not completed or cancelled), oldest first so the queue reads top to bottom.
func (s *Store) ListOrdersAdmin(ctx context.Context, status string) ([]Order, error) {
	if status == "" {
		return s.queryOrders(ctx,
			`WHERE o.status NOT IN ('completed','cancelled') ORDER BY o.created_at ASC LIMIT 200`)
	}
	return s.queryOrders(ctx, `WHERE o.status = $1 ORDER BY o.created_at DESC LIMIT 200`, status)
}

func (s *Store) queryOrders(ctx context.Context, where string, args ...any) ([]Order, error) {
	q := `SELECT o.id, o.status, o.total_baht, o.note, o.created_at, o.paid_at,
	             COALESCE(NULLIF(u.name, ''), u.email)
	      FROM orders o JOIN users u ON u.id = o.user_id ` + where
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Order{}
	idx := map[int64]int{}
	ids := []int64{}
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.Status, &o.TotalBaht, &o.Note, &o.CreatedAt, &o.PaidAt, &o.Customer); err != nil {
			return nil, err
		}
		o.Items = []OrderItem{}
		idx[o.ID] = len(out)
		ids = append(ids, o.ID)
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return out, nil
	}

	itemRows, err := s.pool.Query(ctx,
		`SELECT order_id, product_id, name, unit_price_baht, qty
		 FROM order_items WHERE order_id = ANY($1) ORDER BY id`, ids)
	if err != nil {
		return nil, err
	}
	defer itemRows.Close()
	for itemRows.Next() {
		var orderID int64
		var it OrderItem
		if err := itemRows.Scan(&orderID, &it.ProductID, &it.Name, &it.UnitPriceBaht, &it.Qty); err != nil {
			return nil, err
		}
		i := idx[orderID]
		out[i].Items = append(out[i].Items, it)
	}
	return out, itemRows.Err()
}

// CancelOrderByUser lets a customer cancel only before the shop has taken payment.
func (s *Store) CancelOrderByUser(ctx context.Context, userID, orderID int64) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE orders SET status = 'cancelled', updated_at = now()
		 WHERE id = $1 AND user_id = $2 AND status = 'pending_payment'`, orderID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := s.pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM orders WHERE id = $1 AND user_id = $2)`,
			orderID, userID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		return ErrNotCancellable
	}
	return nil
}

// UpdateOrderStatus moves an order one step along allowedTransitions.
func (s *Store) UpdateOrderStatus(ctx context.Context, orderID int64, next string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	var current string
	err = tx.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1 FOR UPDATE`, orderID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	allowed := false
	for _, n := range allowedTransitions[current] {
		if n == next {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidStatus, current, next)
	}

	if next == StatusPaid {
		_, err = tx.Exec(ctx,
			`UPDATE orders SET status = $2, paid_at = now(), updated_at = now() WHERE id = $1`, orderID, next)
	} else {
		_, err = tx.Exec(ctx,
			`UPDATE orders SET status = $2, updated_at = now() WHERE id = $1`, orderID, next)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
