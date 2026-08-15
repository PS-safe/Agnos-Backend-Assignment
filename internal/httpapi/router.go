package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"agnos.dev/hospital-middleware/internal/core"
	"agnos.dev/hospital-middleware/internal/patient"
	"agnos.dev/hospital-middleware/internal/staff"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxRequestBodyBytes = 1 << 20

const authContextKey = "auth-context"

func init() {
	gin.SetMode(gin.ReleaseMode)
}

type StaffUseCase interface {
	Create(ctx context.Context, input staff.CreateInput) (core.Staff, error)
	Login(ctx context.Context, input staff.LoginInput) (staff.LoginResult, error)
}

type PatientUseCase interface {
	Search(ctx context.Context, auth core.AuthContext, criteria patient.Criteria) ([]core.Patient, error)
}

type TokenVerifier interface {
	Verify(raw string) (core.AuthContext, error)
}

type Dependencies struct {
	Staff    StaffUseCase
	Patients PatientUseCase
	Tokens   TokenVerifier
	Logger   *slog.Logger
}

type handler struct {
	staff    StaffUseCase
	patients PatientUseCase
	tokens   TokenVerifier
	logger   *slog.Logger
}

func NewRouter(deps Dependencies) *gin.Engine {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	h := &handler{
		staff:    deps.Staff,
		patients: deps.Patients,
		tokens:   deps.Tokens,
		logger:   deps.Logger,
	}

	router := gin.New()
	router.Use(h.recovery(), h.requestMetadata(), h.requestLogger(), securityHeaders())
	router.GET("/health", h.health)
	router.POST("/staff/create", h.createStaff)
	router.POST("/staff/login", h.loginStaff)
	router.GET("/patient/search", h.authenticate(), h.searchPatients)
	router.NoRoute(func(c *gin.Context) {
		writeError(c, http.StatusNotFound, "route_not_found", "route not found")
	})
	return router
}

func (h *handler) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type createStaffRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Hospital string `json:"hospital"`
}

func (h *handler) createStaff(c *gin.Context) {
	var request createStaffRequest
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	created, err := h.staff.Create(c.Request.Context(), staff.CreateInput{
		Username:     request.Username,
		Password:     request.Password,
		HospitalCode: request.Hospital,
	})
	if err != nil {
		writeApplicationError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": staffResponse(created)})
}

type loginStaffRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *handler) loginStaff(c *gin.Context) {
	var request loginStaffRequest
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	result, err := h.staff.Login(c.Request.Context(), staff.LoginInput{
		Username: request.Username,
		Password: request.Password,
	})
	if err != nil {
		writeApplicationError(c, err)
		return
	}

	expiresIn := int64(time.Until(result.ExpiresAt).Seconds())
	if expiresIn < 0 {
		expiresIn = 0
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"access_token": result.Token,
		"token_type":   "Bearer",
		"expires_in":   expiresIn,
		"staff":        staffResponse(result.Staff),
	}})
}

var allowedPatientFilters = map[string]struct{}{
	"national_id":   {},
	"passport_id":   {},
	"first_name":    {},
	"middle_name":   {},
	"last_name":     {},
	"date_of_birth": {},
	"phone_number":  {},
	"email":         {},
}

func (h *handler) searchPatients(c *gin.Context) {
	for key := range c.Request.URL.Query() {
		if _, allowed := allowedPatientFilters[key]; !allowed {
			writeError(c, http.StatusBadRequest, "invalid_request", fmt.Sprintf("unknown query parameter %q", key))
			return
		}
	}

	criteria := patient.Criteria{
		NationalID:  c.Query("national_id"),
		PassportID:  c.Query("passport_id"),
		FirstName:   c.Query("first_name"),
		MiddleName:  c.Query("middle_name"),
		LastName:    c.Query("last_name"),
		PhoneNumber: c.Query("phone_number"),
		Email:       c.Query("email"),
	}
	if rawDate := strings.TrimSpace(c.Query("date_of_birth")); rawDate != "" {
		parsed, err := time.Parse("2006-01-02", rawDate)
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "date_of_birth must use YYYY-MM-DD")
			return
		}
		criteria.DateOfBirth = &parsed
	}

	auth, ok := authFromContext(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}

	results, err := h.patients.Search(c.Request.Context(), auth, criteria)
	if err != nil {
		writeApplicationError(c, err)
		return
	}

	responses := make([]patientResponse, 0, len(results))
	for _, result := range results {
		responses = append(responses, newPatientResponse(result))
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"patients": responses,
		"count":    len(responses),
	}})
}

type staffResponseBody struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Hospital string `json:"hospital"`
	Created  string `json:"created_at"`
}

func staffResponse(value core.Staff) staffResponseBody {
	return staffResponseBody{
		ID:       value.ID,
		Username: value.Username,
		Hospital: value.HospitalCode,
		Created:  value.CreatedAt.UTC().Format(time.RFC3339),
	}
}

type patientResponse struct {
	FirstNameTH  string `json:"first_name_th"`
	MiddleNameTH string `json:"middle_name_th"`
	LastNameTH   string `json:"last_name_th"`
	FirstNameEN  string `json:"first_name_en"`
	MiddleNameEN string `json:"middle_name_en"`
	LastNameEN   string `json:"last_name_en"`
	DateOfBirth  string `json:"date_of_birth"`
	PatientHN    string `json:"patient_hn"`
	NationalID   string `json:"national_id"`
	PassportID   string `json:"passport_id"`
	PhoneNumber  string `json:"phone_number"`
	Email        string `json:"email"`
	Gender       string `json:"gender"`
}

func newPatientResponse(value core.Patient) patientResponse {
	return patientResponse{
		FirstNameTH:  value.FirstNameTH,
		MiddleNameTH: value.MiddleNameTH,
		LastNameTH:   value.LastNameTH,
		FirstNameEN:  value.FirstNameEN,
		MiddleNameEN: value.MiddleNameEN,
		LastNameEN:   value.LastNameEN,
		DateOfBirth:  value.DateOfBirth.Format("2006-01-02"),
		PatientHN:    value.PatientHN,
		NationalID:   value.NationalID,
		PassportID:   value.PassportID,
		PhoneNumber:  value.PhoneNumber,
		Email:        value.Email,
		Gender:       value.Gender,
	}
}

func (h *handler) authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeError(c, http.StatusUnauthorized, "unauthorized", "a valid Bearer token is required")
			c.Abort()
			return
		}

		auth, err := h.tokens.Verify(parts[1])
		if err != nil {
			writeError(c, http.StatusUnauthorized, "unauthorized", "a valid Bearer token is required")
			c.Abort()
			return
		}
		c.Set(authContextKey, auth)
		c.Next()
	}
}

func authFromContext(c *gin.Context) (core.AuthContext, bool) {
	value, exists := c.Get(authContextKey)
	if !exists {
		return core.AuthContext{}, false
	}
	auth, ok := value.(core.AuthContext)
	return auth, ok
}

func decodeJSON(c *gin.Context, destination any) error {
	contentType := c.GetHeader("Content-Type")
	if contentType != "" && !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		return errors.New("Content-Type must be application/json")
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("request body is required")
		}
		return errors.New("request body must contain valid JSON with known fields")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func writeApplicationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, core.ErrInvalidInput):
		writeError(c, http.StatusBadRequest, "invalid_request", publicMessage(err, "request validation failed"))
	case errors.Is(err, core.ErrUnauthorized):
		writeError(c, http.StatusUnauthorized, "unauthorized", "invalid username, password, or access token")
	case errors.Is(err, core.ErrNotFound):
		writeError(c, http.StatusNotFound, "not_found", publicMessage(err, "resource not found"))
	case errors.Is(err, core.ErrConflict):
		writeError(c, http.StatusConflict, "conflict", publicMessage(err, "resource already exists"))
	case errors.Is(err, core.ErrUpstream):
		writeError(c, http.StatusBadGateway, "hospital_unavailable", "the hospital information system is unavailable")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "an internal error occurred")
	}
}

func publicMessage(err error, fallback string) string {
	message := err.Error()
	if index := strings.Index(message, ": "); index >= 0 && index+2 < len(message) {
		return message[index+2:]
	}
	return fallback
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{
		"code":    code,
		"message": message,
	}})
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Next()
	}
}

func (h *handler) requestMetadata() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if requestID == "" || len(requestID) > 128 {
			requestID = uuid.NewString()
		}
		c.Header("X-Request-ID", requestID)
		c.Set("request-id", requestID)
		c.Next()
	}
}

func (h *handler) requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		h.logger.Info("request completed",
			"request_id", c.GetString("request-id"),
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(started).Milliseconds(),
		)
	}
}

func (h *handler) recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				h.logger.Error("request panic",
					"request_id", c.GetString("request-id"),
					"panic", recovered,
					"stack", string(debug.Stack()),
				)
				writeError(c, http.StatusInternalServerError, "internal_error", "an internal error occurred")
				c.Abort()
			}
		}()
		c.Next()
	}
}
