package server

import (
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type StatusCode int

type ErrorType string

type ErrorCode int

const (
	StatusOK                  StatusCode = http.StatusOK
	StatusCreated             StatusCode = http.StatusCreated
	StatusAccepted            StatusCode = http.StatusAccepted
	StatusNoContent           StatusCode = http.StatusNoContent
	StatusBadRequest          StatusCode = http.StatusBadRequest
	StatusUnauthorized        StatusCode = http.StatusUnauthorized
	StatusForbidden           StatusCode = http.StatusForbidden
	StatusNotFound            StatusCode = http.StatusNotFound
	StatusInternalServerError StatusCode = http.StatusInternalServerError
)

const (
	ErrorTypeValidation ErrorType = "ValidationError"
	ErrorTypeBusiness   ErrorType = "BusinessError"
	ErrorTypeSystem     ErrorType = "SystemError"
	ErrorTypeAuth       ErrorType = "AuthError"
	ErrorTypeNetwork    ErrorType = "NetworkError"
)

const (
	CodeSuccess ErrorCode = 0
)

const (
	CodeValidationFailed ErrorCode = 10001
)

const (
	CodeBusinessRule     ErrorCode = 20001
	CodeResourceNotFound ErrorCode = 20004
)

const (
	CodeAuthRequired ErrorCode = 30001
	CodeForbidden    ErrorCode = 30003
)

const (
	CodeSystemError ErrorCode = 50000
)

type responseEnvelope struct {
	Code      ErrorCode   `json:"code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	RequestID string      `json:"requestId,omitempty"`
	Timestamp string      `json:"timestamp"`
}

type errorResponse struct {
	Code      ErrorCode   `json:"code"`
	Message   string      `json:"message"`
	Details   interface{} `json:"details,omitempty"`
	RequestID string      `json:"requestId,omitempty"`
	Timestamp string      `json:"timestamp"`
}

type errorDetails struct {
	Type    ErrorType    `json:"type"`
	Hint    string       `json:"hint,omitempty"`
	Fields  []fieldError `json:"fields,omitempty"`
	ErrorID string       `json:"errorId,omitempty"`
	Cause   string       `json:"cause,omitempty"`
	Stack   []string     `json:"stack,omitempty"`
}

type pagePayload struct {
	Items    interface{} `json:"items"`
	Total    int64       `json:"total"`
	Page     int64       `json:"page"`
	PageSize int64       `json:"pageSize"`
	HasMore  bool        `json:"hasMore"`
}

type fieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

type ResponseBuilder struct {
	ctx     echo.Context
	status  StatusCode
	code    ErrorCode
	message string
	data    interface{}
	details interface{}
	isError bool
}

func NewResponseBuilder(c echo.Context) *ResponseBuilder {
	return &ResponseBuilder{ctx: c, status: StatusOK, code: CodeSuccess}
}

func (b *ResponseBuilder) WithStatus(status StatusCode) *ResponseBuilder {
	if status != 0 {
		b.status = status
	}
	return b
}

func (b *ResponseBuilder) WithMessage(message string) *ResponseBuilder {
	b.message = message
	return b
}

func (b *ResponseBuilder) WithData(data interface{}) *ResponseBuilder {
	b.data = data
	return b
}

func (b *ResponseBuilder) WithCode(code ErrorCode) *ResponseBuilder {
	if code != 0 {
		b.code = code
	}
	return b
}

func (b *ResponseBuilder) WithDetails(details interface{}) *ResponseBuilder {
	b.details = details
	return b
}

func (b *ResponseBuilder) AsError(flag bool) *ResponseBuilder {
	b.isError = flag
	if flag && b.code == CodeSuccess {
		b.code = CodeSystemError
	}
	return b
}

func (b *ResponseBuilder) Send() error {
	if b == nil || b.ctx == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "response builder context missing")
	}
	if b.isError || b.code != CodeSuccess {
		return respondError(b.ctx, b.status, b.code, b.message, b.details)
	}
	return respondSuccess(b.ctx, b.status, b.message, b.data)
}

func FieldError(field, message string) fieldError {
	return fieldError{Field: field, Message: message}
}

func FieldErrorWithCode(field, code, message string) fieldError {
	return fieldError{Field: field, Code: code, Message: message}
}

func respondSuccess(c echo.Context, status StatusCode, message string, data interface{}) error {
	if status == 0 {
		status = StatusOK
	}
	resolvedMessage := strings.TrimSpace(message)
	if resolvedMessage == "" {
		resolvedMessage = http.StatusText(int(status))
	}
	payload := responseEnvelope{
		Code:      CodeSuccess,
		Message:   resolvedMessage,
		Data:      data,
		RequestID: requestIDFromEcho(c),
		Timestamp: timestampNow(),
	}
	return c.JSON(int(status), payload)
}

func respondPage(c echo.Context, status StatusCode, items interface{}, total, page, pageSize int64) error {
	hasMore := page > 0 && pageSize > 0 && page*pageSize < total
	pageData := pagePayload{Items: items, Total: total, Page: page, PageSize: pageSize, HasMore: hasMore}
	return respondSuccess(c, status, "", pageData)
}

func respondError(c echo.Context, status StatusCode, code ErrorCode, message string, details interface{}) error {
	if status == 0 {
		status = StatusInternalServerError
	}
	if code == 0 {
		code = CodeSystemError
	}
	resolvedMessage := strings.TrimSpace(message)
	if resolvedMessage == "" {
		resolvedMessage = http.StatusText(int(status))
	}
	reqCtx := c.Request()
	includeStack := false
	if reqCtx != nil {
		includeStack = shouldExposeDebugDetails(reqCtx.Context())
	}
	requestID := requestIDFromEcho(c)
	details = normalizeErrorDetails(details, requestID, includeStack)
	payload := errorResponse{
		Code:      code,
		Message:   resolvedMessage,
		Details:   details,
		RequestID: requestID,
		Timestamp: timestampNow(),
	}
	return c.JSON(int(status), payload)
}

func newErrorDetails(kind ErrorType, hint string, fields ...fieldError) errorDetails {
	return errorDetails{Type: kind, Hint: hint, Fields: fields}
}

func respondValidationError(c echo.Context, message string, fields ...fieldError) error {
	details := newErrorDetails(ErrorTypeValidation, "请检查字段输入并重试", fields...)
	return respondError(c, StatusBadRequest, CodeValidationFailed, message, details)
}

func respondNotFound(c echo.Context, message string) error {
	details := newErrorDetails(ErrorTypeBusiness, "请确认资源存在且未被删除")
	return respondError(c, StatusNotFound, CodeResourceNotFound, message, details)
}

func RespondCreated(c echo.Context, message string, data interface{}) error {
	return NewResponseBuilder(c).
		WithStatus(StatusCreated).
		WithMessage(message).
		WithData(data).
		Send()
}

func RespondBusinessError(c echo.Context, message, hint string, fields ...fieldError) error {
	details := newErrorDetails(ErrorTypeBusiness, hint, fields...)
	return respondError(c, StatusBadRequest, CodeBusinessRule, message, details)
}

func RespondUnauthorized(c echo.Context, message string) error {
	if strings.TrimSpace(message) == "" {
		message = "authentication required"
	}
	details := newErrorDetails(ErrorTypeAuth, "请重新认证后重试")
	return respondError(c, StatusUnauthorized, CodeAuthRequired, message, details)
}

func RespondForbidden(c echo.Context, message string) error {
	if strings.TrimSpace(message) == "" {
		message = "access forbidden"
	}
	details := newErrorDetails(ErrorTypeAuth, "当前凭证缺少所需权限")
	return respondError(c, StatusForbidden, CodeForbidden, message, details)
}

func timestampNow() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func normalizeErrorDetails(details interface{}, requestID string, includeStack bool) interface{} {
	switch d := details.(type) {
	case errorDetails:
		finalized := finalizeErrorDetails(d, requestID, includeStack)
		return finalized
	case *errorDetails:
		if d == nil {
			finalized := finalizeErrorDetails(errorDetails{}, requestID, includeStack)
			return finalized
		}
		finalized := finalizeErrorDetails(*d, requestID, includeStack)
		*d = finalized
		return d
	case nil:
		return finalizeErrorDetails(errorDetails{}, requestID, includeStack)
	default:
		return details
	}
}

func finalizeErrorDetails(details errorDetails, requestID string, includeStack bool) errorDetails {
	if details.Type == "" {
		details.Type = ErrorTypeSystem
	}
	if details.ErrorID == "" {
		if requestID != "" {
			details.ErrorID = requestID
		} else {
			details.ErrorID = uuid.NewString()
		}
	}
	if includeStack && len(details.Stack) == 0 {
		if stack := strings.TrimSpace(string(debug.Stack())); stack != "" {
			details.Stack = strings.Split(stack, "\n")
		}
	}
	return details
}
