package checkin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

type Handler struct{ service *Service }

func RegisterRoutes(mux *http.ServeMux, service *Service) {
	h := &Handler{service: service}
	mux.HandleFunc("GET /api/checkin/config", h.getConfig)
	mux.HandleFunc("PUT /api/checkin/config", h.updateConfig)
	mux.HandleFunc("POST /api/checkin/config/rotate-token", h.rotateToken)
	mux.HandleFunc("GET /api/checkin/overview", h.overview)
	mux.HandleFunc("GET /api/checkin/records", h.records)
	mux.HandleFunc("GET /api/checkin/leaderboard", h.leaderboard)
	mux.HandleFunc("POST /api/embed/checkin/session", h.createEmbedSession)
	mux.HandleFunc("GET /api/embed/checkin/status", h.embedStatus)
	mux.HandleFunc("POST /api/embed/checkin", h.checkIn)
}

func (h *Handler) records(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	page, err := positiveQueryInt(r, "page", 1)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorValidation)
		return
	}
	pageSize, err := positiveQueryInt(r, "pageSize", 20)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorValidation)
		return
	}
	payload, err := h.service.AdminRecords(r.Context(), userID, AdminRecordsQuery{Page: page, PageSize: pageSize, DateFrom: strings.TrimSpace(r.URL.Query().Get("dateFrom")), DateTo: strings.TrimSpace(r.URL.Query().Get("dateTo")), UserQuery: strings.TrimSpace(r.URL.Query().Get("user"))})
	writeResult(w, payload, err)
}

func (h *Handler) leaderboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	payload, err := h.service.AdminLeaderboard(r.Context(), userID, r.URL.Query().Get("period"))
	writeResult(w, payload, err)
}

func (h *Handler) getConfig(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	payload, err := h.service.GetConfig(r.Context(), userID)
	writeResult(w, payload, err)
}

func (h *Handler) updateConfig(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var req UpdateConfigRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	payload, err := h.service.UpdateConfig(r.Context(), userID, req)
	writeResult(w, payload, err)
}

func (h *Handler) rotateToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	payload, err := h.service.RotateEmbedToken(r.Context(), userID)
	writeResult(w, payload, err)
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	payload, err := h.service.AdminOverview(r.Context(), userID)
	writeResult(w, payload, err)
}

func (h *Handler) createEmbedSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorEmbedRequest)
		return
	}
	payload, err := h.service.CreateEmbedSession(r.Context(), req)
	writeResult(w, payload, err)
}

func (h *Handler) embedStatus(w http.ResponseWriter, r *http.Request) {
	payload, err := h.service.EmbedStatus(r.Context(), bearerToken(r.Header.Get("Authorization")))
	writeResult(w, payload, err)
}

func (h *Handler) checkIn(w http.ResponseWriter, r *http.Request) {
	payload, err := h.service.CheckIn(r.Context(), bearerToken(r.Header.Get("Authorization")))
	writeResult(w, payload, err)
}

func writeResult[T any](w http.ResponseWriter, payload T, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, payload)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := ErrorUnknown
	var reqErr requestError
	if errors.As(err, &reqErr) {
		message = reqErr.Error()
		switch reqErr {
		case requestError(ErrorNoCurrentAccount), requestError(ErrorEmbedAlreadyChecked):
			status = http.StatusConflict
		case requestError(ErrorAdminOnly), requestError(ErrorInvalidSourceOrigin), requestError(ErrorEmbedAdminSession), requestError(ErrorEmbedSourceBinding), requestError(ErrorEmbedSrcHostMismatch), requestError(ErrorEmbedDisabled):
			status = http.StatusForbidden
		case requestError(ErrorEmbedSessionInvalid), requestError(ErrorEmbedSub2apiAuth):
			status = http.StatusUnauthorized
		default:
			status = http.StatusBadRequest
		}
	}
	httpjson.WriteError(w, status, message)
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func positiveQueryInt(r *http.Request, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, requestError(ErrorValidation)
	}
	return value, nil
}
