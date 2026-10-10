package productauth

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Service) AuthorizeRequest(request *http.Request) bool {
	cookie, err := request.Cookie(CookieName)
	token := ""
	if err == nil {
		token = cookie.Value
	}
	ok, err := s.AuthenticateToken(request.Context(), token)
	return err == nil && ok
}

func (s *Service) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !s.AuthorizeRequest(request) {
			http.Error(response, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(response, request)
	})
}

func (s *Service) SessionStatusHandler() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		setup, setupErr := s.SetupStatus(request.Context())
		auth, authErr := s.Status(request.Context())
		if setupErr != nil || authErr != nil {
			http.Error(response, "session unavailable", http.StatusServiceUnavailable)
			return
		}
		payload := struct {
			SetupComplete bool `json:"setupComplete"`
			Authenticated bool `json:"authenticated"`
			TrustedMode   bool `json:"trustedMode"`
		}{SetupComplete: setup.Complete, TrustedMode: auth.TrustedMode}
		payload.Authenticated = s.AuthorizeRequest(request)
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(response).Encode(payload)
	})
}

func (s *Service) LoginHandler() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || !strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
			http.Error(response, "invalid request", http.StatusBadRequest)
			return
		}
		client := request.RemoteAddr
		if host, _, err := net.SplitHostPort(client); err == nil {
			client = host
		}
		if !s.loginAllowed(client) {
			response.Header().Set("Retry-After", strconv.Itoa(60))
			http.Error(response, "too many attempts", http.StatusTooManyRequests)
			return
		}
		var input struct {
			Password string `json:"password"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2048))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || s.VerifyPassword(request.Context(), input.Password) != nil {
			s.recordLoginFailure(client)
			http.Error(response, "invalid credentials", http.StatusUnauthorized)
			return
		}
		s.clearLoginFailures(client)
		token, expires, err := s.CreateSession(request.Context())
		if err != nil {
			http.Error(response, "authentication unavailable", http.StatusServiceUnavailable)
			return
		}
		http.SetCookie(response, &http.Cookie{Name: CookieName, Value: token, Path: "/", Expires: expires,
			MaxAge: int(s.sessionLifetime.Seconds()), HttpOnly: true, Secure: request.TLS != nil, SameSite: http.SameSiteStrictMode})
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusNoContent)
	})
}

func (s *Service) loginAllowed(client string) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	now := s.now()
	attempt := s.loginAttempts[client]
	if now.Sub(attempt.windowStart) >= time.Minute {
		delete(s.loginAttempts, client)
		return true
	}
	return attempt.count < 5
}

func (s *Service) recordLoginFailure(client string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	now := s.now()
	attempt := s.loginAttempts[client]
	if attempt.windowStart.IsZero() || now.Sub(attempt.windowStart) >= time.Minute {
		attempt = loginAttempt{windowStart: now}
	}
	attempt.count++
	s.loginAttempts[client] = attempt
	if len(s.loginAttempts) > 1024 {
		for key, value := range s.loginAttempts {
			if now.Sub(value.windowStart) >= time.Minute {
				delete(s.loginAttempts, key)
			}
		}
	}
}

func (s *Service) clearLoginFailures(client string) {
	s.loginMu.Lock()
	delete(s.loginAttempts, client)
	s.loginMu.Unlock()
}

func (s *Service) LogoutHandler() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(response, "invalid request", http.StatusBadRequest)
			return
		}
		cookie, _ := request.Cookie(CookieName)
		if cookie != nil {
			_ = s.RevokeSession(request.Context(), cookie.Value)
		}
		http.SetCookie(response, &http.Cookie{Name: CookieName, Path: "/", MaxAge: -1, HttpOnly: true,
			Secure: request.TLS != nil, SameSite: http.SameSiteStrictMode})
		response.WriteHeader(http.StatusNoContent)
	})
}
