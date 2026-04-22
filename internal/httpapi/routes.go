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
	// Static SPA. Register at "/" so unknown paths fall through to index.html.
	if static, err := StaticHandler(); err == nil {
		s.mux.Handle("/", static)
	} else {
		s.log.Warn("no static web assets embedded", "err", err)
	}

	// Public.
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("POST /api/auth/telegram/callback", s.handleTelegramLogin)
	s.mux.HandleFunc("POST /api/auth/telegram/initdata", s.handleTelegramInitData)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)

	// Session-gated.
	s.mux.HandleFunc("GET /api/me", s.requireSession(s.handleMe))
	s.mux.HandleFunc("GET /api/region", s.requireSession(s.handleGetRegion))
	s.mux.HandleFunc("PUT /api/region", s.requireSession(s.handleSetRegion))
	s.mux.HandleFunc("GET /api/cheapest", s.requireSession(s.handleCheapest))
	s.mux.HandleFunc("GET /api/plan-now", s.requireSession(s.handlePlanNow))
	s.mux.HandleFunc("GET /api/next", s.requireSession(s.handleNext))
	s.mux.HandleFunc("GET /api/rates", s.requireSession(s.handleRates))
	s.mux.HandleFunc("GET /api/status", s.requireSession(s.handleStatus))

	s.mux.HandleFunc("GET /api/subscription", s.requireSession(s.handleGetSubscription))
	s.mux.HandleFunc("PUT /api/subscription", s.requireSession(s.handlePutSubscription))
	s.mux.HandleFunc("DELETE /api/subscription", s.requireSession(s.handleDeleteSubscription))

	s.mux.HandleFunc("GET /api/charge-plans", s.requireSession(s.handleListChargePlans))
	s.mux.HandleFunc("POST /api/charge-plans", s.requireSession(s.handleCreateChargePlan))
	s.mux.HandleFunc("DELETE /api/charge-plans/{id}", s.requireSession(s.handleCancelChargePlan))

	s.mux.HandleFunc("GET /api/alert", s.requireSession(s.handleGetAlert))
	s.mux.HandleFunc("PUT /api/alert", s.requireSession(s.handlePutAlert))
	s.mux.HandleFunc("DELETE /api/alert", s.requireSession(s.handleDeleteAlert))

	s.mux.HandleFunc("GET /api/octopus", s.requireSession(s.handleGetOctopus))
	s.mux.HandleFunc("PUT /api/octopus", s.requireSession(s.handlePutOctopus))
	s.mux.HandleFunc("DELETE /api/octopus", s.requireSession(s.handleDeleteOctopus))

	s.mux.HandleFunc("GET /api/consumption", s.requireSession(s.handleConsumption))
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

// handleTelegramInitData exchanges a Telegram Mini App initData string for a
// session cookie. Front-end calls this with `window.Telegram.WebApp.initData` when
// running inside Telegram so users don't have to go through the Login Widget.
func (s *Server) handleTelegramInitData(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InitData string `json:"init_data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.InitData == "" {
		writeError(w, http.StatusBadRequest, "missing init_data")
		return
	}
	data, err := tgauth.VerifyInitData(s.botToken, body.InitData)
	if err != nil {
		s.log.Warn("telegram initdata verify failed", "err", err)
		writeError(w, http.StatusUnauthorized, "initdata verification failed")
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

// handlePlanNow is the instant charge planner. ?duration=4h &by=HH:MM (optional).
func (s *Server) handlePlanNow(w http.ResponseWriter, r *http.Request) {
	chatID := claimsOf(r).TelegramUserID
	d, err := time.ParseDuration(r.URL.Query().Get("duration"))
	if err != nil || d <= 0 {
		writeError(w, http.StatusBadRequest, "missing or invalid ?duration=")
		return
	}
	byLocal := r.URL.Query().Get("by")
	sug, err := s.svc.SuggestCharge(r.Context(), chatID, d, byLocal)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"window":          toWindowJSON(sug.Window),
		"start_in_seconds": int(sug.StartIn.Seconds()),
	})
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

// handleRates returns every half-hour known for the chat's region inside [from, to].
// Defaults: from=now, to=now+48h. Useful for charts/tables.
func (s *Server) handleRates(w http.ResponseWriter, r *http.Request) {
	chatID := claimsOf(r).TelegramUserID
	now := time.Now().UTC()
	from := now
	to := now.Add(48 * time.Hour)

	if v := r.URL.Query().Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid ?from= (want RFC3339)")
			return
		}
		from = t.UTC()
	}
	if v := r.URL.Query().Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid ?to= (want RFC3339)")
			return
		}
		to = t.UTC()
	}

	rates, err := s.svc.Rates(r.Context(), chatID, from, to)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	out := make([]slotJSON, 0, len(rates))
	for _, hh := range rates {
		out = append(out, slotJSON{
			ValidFrom: hh.ValidFrom, ValidTo: hh.ValidTo,
			IncVAT: hh.UnitRateIncVAT, ExcVAT: hh.UnitRateExcVAT,
		})
	}
	writeJSON(w, http.StatusOK, out)
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

// ---- price alerts --------------------------------------------------------

type alertBody struct {
	ThresholdIncVAT float64 `json:"threshold_inc_vat"`
}

func (s *Server) handleGetAlert(w http.ResponseWriter, r *http.Request) {
	st, err := s.svc.Status(r.Context(), claimsOf(r).TelegramUserID)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	if st.PriceAlert == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"threshold_inc_vat": st.PriceAlert.ThresholdIncVAT,
		"enabled":           st.PriceAlert.Enabled,
	})
}

func (s *Server) handlePutAlert(w http.ResponseWriter, r *http.Request) {
	var body alertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.svc.SetPriceAlert(r.Context(), claimsOf(r).TelegramUserID, body.ThresholdIncVAT); err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) handleDeleteAlert(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.DisablePriceAlert(r.Context(), claimsOf(r).TelegramUserID); err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ---- octopus link --------------------------------------------------------

type octopusLinkBody struct {
	AccountNumber string `json:"account_number"`
	APIKey        string `json:"api_key"`
}

func (s *Server) handleGetOctopus(w http.ResponseWriter, r *http.Request) {
	la, err := s.svc.LinkedAccountFor(r.Context(), claimsOf(r).TelegramUserID)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"linked":         la.Linked,
		"account_number": la.AccountNumber,
	})
}

func (s *Server) handlePutOctopus(w http.ResponseWriter, r *http.Request) {
	var body octopusLinkBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	info, err := s.svc.LinkOctopusAccount(r.Context(), claimsOf(r).TelegramUserID, body.AccountNumber, body.APIKey)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrLinkNotConfigured):
			writeError(w, http.StatusServiceUnavailable, err.Error())
		case errors.Is(err, service.ErrLinkInvalid):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeServiceErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"account_number": info.Number,
		"address_line_1": info.AddressLine1,
		"postcode":       info.Postcode,
		"current_tariff": info.CurrentTariff,
		"mpan":           info.MPAN,
	})
}

func (s *Server) handleDeleteOctopus(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.UnlinkOctopusAccount(r.Context(), claimsOf(r).TelegramUserID); err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ---- consumption ---------------------------------------------------------

type consumptionPointJSON struct {
	IntervalStart time.Time `json:"interval_start"`
	IntervalEnd   time.Time `json:"interval_end"`
	KWh           float64   `json:"consumption_kwh"`
}

func (s *Server) handleConsumption(w http.ResponseWriter, r *http.Request) {
	chatID := claimsOf(r).TelegramUserID

	// Defaults: last 7 days, half-hourly.
	now := time.Now().UTC()
	from := now.Add(-7 * 24 * time.Hour)
	to := now
	groupBy := r.URL.Query().Get("group_by") // "", "hour", "day", "week", "month", "quarter"

	if v := r.URL.Query().Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid ?from= (want RFC3339)")
			return
		}
		from = t.UTC()
	}
	if v := r.URL.Query().Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid ?to= (want RFC3339)")
			return
		}
		to = t.UTC()
	}

	points, err := s.svc.Consumption(r.Context(), chatID, from, to, groupBy)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrLinkNotConfigured):
			writeError(w, http.StatusServiceUnavailable, err.Error())
		case errors.Is(err, service.ErrLinkInvalid):
			writeError(w, http.StatusPreconditionRequired, err.Error())
		default:
			writeServiceErr(w, err)
		}
		return
	}
	out := make([]consumptionPointJSON, len(points))
	for i, p := range points {
		out[i] = consumptionPointJSON{
			IntervalStart: p.IntervalStart,
			IntervalEnd:   p.IntervalEnd,
			KWh:           p.KWh,
		}
	}
	writeJSON(w, http.StatusOK, out)
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
