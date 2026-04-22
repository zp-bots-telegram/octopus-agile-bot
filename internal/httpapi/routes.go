package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zp-bots-telegram/octopus-agile-bot/internal/agile"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/service"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/tgauth"
)

func (s *Server) routes() {
	// Public.
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("POST /api/auth/telegram/callback", s.handleTelegramLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)

	// Session-gated.
	s.mux.HandleFunc("GET /api/me", s.requireSession(s.handleMe))
	s.mux.HandleFunc("GET /api/region", s.requireSession(s.handleGetRegion))
	s.mux.HandleFunc("PUT /api/region", s.requireSession(s.handleSetRegion))
	s.mux.HandleFunc("GET /api/cheapest", s.requireSession(s.handleCheapest))
	s.mux.HandleFunc("GET /api/next", s.requireSession(s.handleNext))
	s.mux.HandleFunc("GET /api/status", s.requireSession(s.handleStatus))

	s.mux.HandleFunc("GET /api/subscription", s.requireSession(s.handleGetSubscription))
	s.mux.HandleFunc("PUT /api/subscription", s.requireSession(s.handlePutSubscription))
	s.mux.HandleFunc("DELETE /api/subscription", s.requireSession(s.handleDeleteSubscription))

	s.mux.HandleFunc("GET /api/charge-plans", s.requireSession(s.handleListChargePlans))
	s.mux.HandleFunc("POST /api/charge-plans", s.requireSession(s.handleCreateChargePlan))
	s.mux.HandleFunc("DELETE /api/charge-plans/{id}", s.requireSession(s.handleCancelChargePlan))
}

// ---- public --------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleTelegramLogin accepts the widget's user object as JSON and issues a session.
func (s *Server) handleTelegramLogin(w http.ResponseWriter, r *http.Request) {
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	// Coerce everything to string for tgauth.Verify.
	params := map[string]string{}
	for k, v := range raw {
		switch vv := v.(type) {
		case string:
			params[k] = vv
		case float64:
			if vv == float64(int64(vv)) {
				params[k] = strconv.FormatInt(int64(vv), 10)
			} else {
				params[k] = fmt.Sprintf("%v", vv)
			}
		default:
			params[k] = fmt.Sprintf("%v", vv)
		}
	}
	data, err := tgauth.Verify(s.botToken, params)
	if err != nil {
		s.log.Warn("telegram login verify failed", "err", err)
		writeError(w, http.StatusUnauthorized, "telegram login verification failed")
		return
	}
	if err := s.sessions.Issue(w, data.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"telegram_user_id": data.ID,
		"first_name":       data.FirstName,
		"username":         data.Username,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, _ *http.Request) {
	s.sessions.Clear(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---- me / region ---------------------------------------------------------

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	c := claimsOf(r)
	writeJSON(w, http.StatusOK, map[string]any{"telegram_user_id": c.TelegramUserID})
}

func (s *Server) handleGetRegion(w http.ResponseWriter, r *http.Request) {
	chatID := claimsOf(r).TelegramUserID
	st, err := s.svc.Status(r.Context(), chatID)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"region":      st.Region,
		"region_name": agile.RegionName(st.Region),
		"timezone":    st.Timezone,
	})
}

type regionPutBody struct {
	Region   string `json:"region,omitempty"`
	Postcode string `json:"postcode,omitempty"`
}

func (s *Server) handleSetRegion(w http.ResponseWriter, r *http.Request) {
	chatID := claimsOf(r).TelegramUserID
	var body regionPutBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	var region string
	switch {
	case strings.TrimSpace(body.Region) != "":
		if err := s.svc.SetRegion(r.Context(), chatID, body.Region); err != nil {
			writeServiceErr(w, err)
			return
		}
		region = strings.ToUpper(strings.TrimSpace(body.Region))
	case strings.TrimSpace(body.Postcode) != "":
		got, err := s.svc.SetRegionByPostcode(r.Context(), chatID, body.Postcode)
		if err != nil {
			writeError(w, http.StatusBadRequest, "postcode lookup failed: "+err.Error())
			return
		}
		region = got
	default:
		writeError(w, http.StatusBadRequest, "provide either region or postcode")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"region":      region,
		"region_name": agile.RegionName(region),
	})
}

// ---- rate lookups --------------------------------------------------------

type windowJSON struct {
	Start      time.Time  `json:"start"`
	End        time.Time  `json:"end"`
	MeanIncVAT float64    `json:"mean_inc_vat_p_per_kwh"`
	Slots      []slotJSON `json:"slots"`
}

type slotJSON struct {
	ValidFrom time.Time `json:"valid_from"`
	ValidTo   time.Time `json:"valid_to"`
	IncVAT    float64   `json:"inc_vat_p_per_kwh"`
	ExcVAT    float64   `json:"exc_vat_p_per_kwh"`
}

func toWindowJSON(w agile.Window) windowJSON {
	out := windowJSON{Start: w.Start, End: w.End, MeanIncVAT: w.MeanIncVAT}
	for _, s := range w.Slots {
		out.Slots = append(out.Slots, slotJSON{
			ValidFrom: s.ValidFrom, ValidTo: s.ValidTo,
			IncVAT: s.UnitRateIncVAT, ExcVAT: s.UnitRateExcVAT,
		})
	}
	return out
}

func (s *Server) handleCheapest(w http.ResponseWriter, r *http.Request) {
	chatID := claimsOf(r).TelegramUserID
	d, err := time.ParseDuration(r.URL.Query().Get("duration"))
	if err != nil || d <= 0 {
		writeError(w, http.StatusBadRequest, "missing or invalid ?duration=")
		return
	}
	win, err := s.svc.CheapestWindow(r.Context(), chatID, d)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toWindowJSON(win))
}

func (s *Server) handleNext(w http.ResponseWriter, r *http.Request) {
	chatID := claimsOf(r).TelegramUserID
	threshold, err := strconv.ParseFloat(r.URL.Query().Get("threshold"), 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing or invalid ?threshold=")
		return
	}
	hh, err := s.svc.NextBelowThreshold(r.Context(), chatID, threshold)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, slotJSON{
		ValidFrom: hh.ValidFrom, ValidTo: hh.ValidTo,
		IncVAT: hh.UnitRateIncVAT, ExcVAT: hh.UnitRateExcVAT,
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	chatID := claimsOf(r).TelegramUserID
	st, err := s.svc.Status(r.Context(), chatID)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// ---- subscription --------------------------------------------------------

type subscriptionBody struct {
	DurationMinutes int    `json:"duration_minutes"`
	NotifyAtLocal   string `json:"notify_at_local"`
}

func (s *Server) handleGetSubscription(w http.ResponseWriter, r *http.Request) {
	st, err := s.svc.Status(r.Context(), claimsOf(r).TelegramUserID)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	if st.Subscription == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"duration_minutes": int(st.Subscription.Duration / time.Minute),
		"notify_at_local":  st.Subscription.NotifyAtLocal,
	})
}

func (s *Server) handlePutSubscription(w http.ResponseWriter, r *http.Request) {
	var body subscriptionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DurationMinutes <= 0 {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	err := s.svc.SetSubscription(r.Context(), claimsOf(r).TelegramUserID,
		time.Duration(body.DurationMinutes)*time.Minute, body.NotifyAtLocal)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) handleDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Unsubscribe(r.Context(), claimsOf(r).TelegramUserID); err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ---- charge plans --------------------------------------------------------

type chargePlanBody struct {
	DurationMinutes  int    `json:"duration_minutes"`
	WindowStartLocal string `json:"window_start_local"`
	WindowEndLocal   string `json:"window_end_local"`
}

func (s *Server) handleListChargePlans(w http.ResponseWriter, r *http.Request) {
	plans, err := s.svc.ListChargePlans(r.Context(), claimsOf(r).TelegramUserID)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plans)
}

func (s *Server) handleCreateChargePlan(w http.ResponseWriter, r *http.Request) {
	var body chargePlanBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DurationMinutes <= 0 {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	p, err := s.svc.CreateChargePlan(r.Context(), claimsOf(r).TelegramUserID,
		time.Duration(body.DurationMinutes)*time.Minute,
		body.WindowStartLocal, body.WindowEndLocal)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleCancelChargePlan(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ok, err := s.svc.CancelChargePlan(r.Context(), claimsOf(r).TelegramUserID, id)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "plan not found")
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ---- error mapping -------------------------------------------------------

func writeServiceErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidRegion):
		writeError(w, http.StatusBadRequest, "region must be a letter A-P")
	case errors.Is(err, service.ErrBadTime):
		writeError(w, http.StatusBadRequest, "time must be HH:MM")
	case errors.Is(err, service.ErrNoChat):
		writeError(w, http.StatusPreconditionRequired, "set a region first")
	case errors.Is(err, agile.ErrNoRates):
		writeError(w, http.StatusNotFound, "no rates available yet")
	case errors.Is(err, agile.ErrDurationTooLong):
		writeError(w, http.StatusBadRequest, "duration longer than published horizon")
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
