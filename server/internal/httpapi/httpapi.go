// Package httpapi hosts the unified response envelope, error codes,
// shared middleware (traceId, requestId, auth), and the Gin router.
//
// Design constraints from AGENTS.md:
//   - Unified response: { code, message, data, traceId }
//   - Never return bare arrays
//   - Handler → service → repository; handlers do NOT touch the DB
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Response is the unified JSON envelope for all API responses.
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
	TraceID string      `json:"traceId"`
}

// ErrorCode ranges per AGENTS.md §三.
const (
	CodeOK           = 0
	CodeInvalidArg   = 10001
	CodeUnauthorized = 10002
	CodeForbidden    = 10003
	CodeRateLimited  = 10004
	CodeInternal     = 10099

	CodeUserNotFound  = 20001
	CodeUserBlocked   = 20002
	CodeIPLimit       = 20003
	CodeTokenRevoked  = 20004
	CodePhoneRequired = 20005

	CodeResourceNotFound  = 30001
	CodeDuplicateResource = 30002
	CodeStatusConflict    = 30003
	CodeCategoryDisabled  = 30004
)

// DomainError is returned by services when they want a specific error code
// and HTTP status. Handlers wrap it with JSON and set the HTTP status.
type DomainError struct {
	Code       int
	Message    string
	HTTPStatus int
}

func (e *DomainError) Error() string { return e.Message }

func NewDomainError(code int, msg string, httpStatus int) *DomainError {
	return &DomainError{Code: code, Message: msg, HTTPStatus: httpStatus}
}

// traceIdKey is the context key for the per-request trace id.
type traceIdKey struct{}

// TraceID returns the trace id from ctx (or empty string if none).
func TraceID(ctx context.Context) string {
	if v, ok := ctx.Value(traceIdKey{}).(string); ok {
		return v
	}
	return ""
}

// OK writes a successful response.
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeOK,
		Message: "ok",
		Data:    data,
		TraceID: TraceID(c.Request.Context()),
	})
}

// Fail writes an error response. If err is a *DomainError, its code and
// HTTP status are used; otherwise we default to CodeInternal / 500.
func Fail(c *gin.Context, err error) {
	traceID := TraceID(c.Request.Context())

	var de *DomainError
	if errors.As(err, &de) {
		c.JSON(de.HTTPStatus, Response{
			Code:    de.Code,
			Message: de.Message,
			TraceID: traceID,
		})
		return
	}

	c.JSON(http.StatusInternalServerError, Response{
		Code:    CodeInternal,
		Message: "internal error",
		TraceID: traceID,
	})
}

// NewRouter constructs the Gin engine with global middleware.
func NewRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(TraceIDMiddleware())
	r.Use(CORSMiddleware())
	return r
}

// TraceIDMiddleware injects a trace id into the context (from X-Trace-Id
// header or freshly generated uuid) and echoes it in the response header.
func TraceIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-Id")
		if traceID == "" {
			traceID = uuid.NewString()
		}
		c.Set("traceId", traceID)
		c.Request = c.Request.WithContext(
			context.WithValue(c.Request.Context(), traceIdKey{}, traceID),
		)
		c.Header("X-Trace-Id", traceID)
		c.Next()
	}
}

// CORSMiddleware — permissive for dev/staging. Prod should tighten origins.
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Trace-Id")
		c.Header("Access-Control-Expose-Headers", "X-Trace-Id")
		if strings.EqualFold(c.Request.Method, "OPTIONS") {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
