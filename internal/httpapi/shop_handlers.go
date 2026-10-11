package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kitchu-studio/order-demo-backend/internal/promptpay"
	"github.com/kitchu-studio/order-demo-backend/internal/store"
)

func parseID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		abort(c, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}
	return id, true
}

func (s *Server) internalError(c *gin.Context, what string, err error) {
	slog.Error(what, "error", err)
	abort(c, http.StatusInternalServerError, "internal error")
}

// GET /api/v1/shop
func (s *Server) shopInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"name": s.shopName})
}

// GET /api/v1/products
func (s *Server) listProducts(c *gin.Context) {
	products, err := s.store.ListProducts(c.Request.Context())
	if err != nil {
		s.internalError(c, "list products", err)
		return
	}
	c.JSON(http.StatusOK, products)
}

type payment struct {
	Method     string `json:"method"`
	AmountBaht int    `json:"amount_baht"`
	// Payload is the text to render as a QR code in the app.
	Payload string `json:"payload"`
}

type orderView struct {
	store.Order
	Payment *payment `json:"payment,omitempty"`
}

// view attaches the PromptPay QR while the order is still waiting for payment.
func (s *Server) view(o *store.Order) (orderView, error) {
	v := orderView{Order: *o}
	if o.Status == store.StatusPendingPayment {
		payload, err := promptpay.Payload(s.promptPayID, float64(o.TotalBaht))
		if err != nil {
			return v, err
		}
		v.Payment = &payment{Method: "promptpay", AmountBaht: o.TotalBaht, Payload: payload}
	}
	return v, nil
}

func (s *Server) views(orders []store.Order) ([]orderView, error) {
	out := make([]orderView, 0, len(orders))
	for i := range orders {
		v, err := s.view(&orders[i])
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

type createOrderRequest struct {
	Items []struct {
		ProductID int64 `json:"product_id" binding:"required"`
		Qty       int   `json:"qty" binding:"required"`
	} `json:"items" binding:"required"`
	Note string `json:"note" binding:"max=300"`
}

// POST /api/v1/orders
func (s *Server) createOrder(c *gin.Context) {
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid request: items (product_id, qty) are required")
		return
	}
	lines := make([]store.OrderLine, 0, len(req.Items))
	for _, it := range req.Items {
		lines = append(lines, store.OrderLine{ProductID: it.ProductID, Qty: it.Qty})
	}

	order, err := s.store.CreateOrder(c.Request.Context(), c.GetInt64(ctxUserID), lines, strings.TrimSpace(req.Note))
	switch {
	case errors.Is(err, store.ErrInvalidItems):
		abort(c, http.StatusBadRequest, "order must have 1-30 different items, 1-20 of each")
		return
	case errors.Is(err, store.ErrProductNotFound):
		abort(c, http.StatusConflict, "an item is no longer available")
		return
	case err != nil:
		s.internalError(c, "create order", err)
		return
	}

	v, err := s.view(order)
	if err != nil {
		s.internalError(c, "build payment", err)
		return
	}
	c.JSON(http.StatusCreated, v)
}

// GET /api/v1/orders
func (s *Server) listMyOrders(c *gin.Context) {
	orders, err := s.store.ListOrdersByUser(c.Request.Context(), c.GetInt64(ctxUserID))
	if err != nil {
		s.internalError(c, "list orders", err)
		return
	}
	vs, err := s.views(orders)
	if err != nil {
		s.internalError(c, "build payment", err)
		return
	}
	c.JSON(http.StatusOK, vs)
}

// GET /api/v1/orders/:id
func (s *Server) getMyOrder(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	o, err := s.store.GetOrderForUser(c.Request.Context(), c.GetInt64(ctxUserID), id)
	if errors.Is(err, store.ErrNotFound) {
		abort(c, http.StatusNotFound, "order not found")
		return
	}
	if err != nil {
		s.internalError(c, "get order", err)
		return
	}
	v, err := s.view(o)
	if err != nil {
		s.internalError(c, "build payment", err)
		return
	}
	c.JSON(http.StatusOK, v)
}

// POST /api/v1/orders/:id/cancel
func (s *Server) cancelMyOrder(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	err := s.store.CancelOrderByUser(c.Request.Context(), c.GetInt64(ctxUserID), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		abort(c, http.StatusNotFound, "order not found")
		return
	case errors.Is(err, store.ErrNotCancellable):
		abort(c, http.StatusConflict, "order can no longer be cancelled")
		return
	case err != nil:
		s.internalError(c, "cancel order", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": store.StatusCancelled})
}

// GET /api/v1/admin/orders?status=paid  (no status = all open orders)
func (s *Server) adminListOrders(c *gin.Context) {
	status := c.Query("status")
	if status != "" && !validStatus(status) {
		abort(c, http.StatusBadRequest, "unknown status")
		return
	}
	orders, err := s.store.ListOrdersAdmin(c.Request.Context(), status)
	if err != nil {
		s.internalError(c, "admin list orders", err)
		return
	}
	// Admins see the QR too, so a customer can pay at the counter from the shop's screen.
	vs, err := s.views(orders)
	if err != nil {
		s.internalError(c, "build payment", err)
		return
	}
	c.JSON(http.StatusOK, vs)
}

type setStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// POST /api/v1/admin/orders/:id/status
func (s *Server) adminSetStatus(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req setStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil || !validStatus(req.Status) {
		abort(c, http.StatusBadRequest, "status must be one of pending_payment, paid, preparing, ready, completed, cancelled")
		return
	}
	err := s.store.UpdateOrderStatus(c.Request.Context(), id, req.Status)
	switch {
	case errors.Is(err, store.ErrNotFound):
		abort(c, http.StatusNotFound, "order not found")
		return
	case errors.Is(err, store.ErrInvalidStatus):
		abort(c, http.StatusConflict, err.Error())
		return
	case err != nil:
		s.internalError(c, "update status", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "status": req.Status})
}

type setProductRequest struct {
	Available *bool `json:"available" binding:"required"`
}

// PATCH /api/v1/admin/products/:id  -- mark an item sold out or back on the menu.
func (s *Server) adminSetProduct(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req setProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "available (true/false) is required")
		return
	}
	p, err := s.store.SetProductAvailable(c.Request.Context(), id, *req.Available)
	if errors.Is(err, store.ErrNotFound) {
		abort(c, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		s.internalError(c, "set product", err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func validStatus(s string) bool {
	switch s {
	case store.StatusPendingPayment, store.StatusPaid, store.StatusPreparing,
		store.StatusReady, store.StatusCompleted, store.StatusCancelled:
		return true
	}
	return false
}

const maxImageBytes = 5 << 20

var imageExts = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// POST /api/v1/admin/products/:id/image   (multipart form, field "image")
func (s *Server) adminSetProductImage(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImageBytes+(1<<20))
	fh, err := c.FormFile("image")
	if err != nil {
		abort(c, http.StatusBadRequest, "image file is required (form field: image, max 5 MB)")
		return
	}
	if fh.Size > maxImageBytes {
		abort(c, http.StatusRequestEntityTooLarge, "image must be 5 MB or smaller")
		return
	}

	f, err := fh.Open()
	if err != nil {
		s.internalError(c, "open upload", err)
		return
	}
	defer f.Close()

	// Trust the file's own bytes, not the name or content-type the client sent.
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	ext, ok := imageExts[http.DetectContentType(head[:n])]
	if !ok {
		abort(c, http.StatusUnsupportedMediaType, "image must be JPEG, PNG or WebP")
		return
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		s.internalError(c, "seek upload", err)
		return
	}

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		s.internalError(c, "random name", err)
		return
	}
	name := fmt.Sprintf("p%d-%s%s", id, hex.EncodeToString(suffix), ext)
	path := filepath.Join(s.uploadDir, name)

	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		s.internalError(c, "create image file", err)
		return
	}
	_, copyErr := io.Copy(dst, f)
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(path) //nolint:errcheck // best effort
		s.internalError(c, "write image file", errors.Join(copyErr, closeErr))
		return
	}

	old, p, err := s.store.SetProductImage(c.Request.Context(), id, name)
	if errors.Is(err, store.ErrNotFound) {
		os.Remove(path) //nolint:errcheck // best effort
		abort(c, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		os.Remove(path) //nolint:errcheck // best effort
		s.internalError(c, "save image", err)
		return
	}
	if old != "" {
		os.Remove(filepath.Join(s.uploadDir, filepath.Base(old))) //nolint:errcheck // best effort
	}
	c.JSON(http.StatusOK, p)
}
