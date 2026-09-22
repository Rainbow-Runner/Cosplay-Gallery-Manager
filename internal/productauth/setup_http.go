package productauth

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"
)

type SetupOptions struct {
	RuntimeEnvironment  string
	AllowDirectLoopback bool
	AllowDockerLocal    bool
	CoserMetadataRoot   string
	BackupRoot          string
	ValidateStorage     func(SetupInput) error
}

func (o SetupOptions) directAllowed(request *http.Request) bool {
	// Docker cannot observe the host-side port binding. AllowDockerLocal is an
	// explicit deployment opt-in and must only be set with a loopback-published
	// host port; Host alone is not an authentication boundary.
	return o.AllowDirectLoopback && isLoopbackRequest(request) || o.AllowDockerLocal && isLoopbackHost(request.Host)
}

func (s *Service) SetupStatusHandler(options SetupOptions) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		status, err := s.SetupStatus(request.Context())
		if err != nil {
			http.Error(response, "Setup unavailable", http.StatusServiceUnavailable)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(response).Encode(struct {
			Complete           bool   `json:"complete"`
			RuntimeEnvironment string `json:"runtimeEnvironment"`
			TicketRequired     bool   `json:"ticketRequired"`
			CoserMetadataRoot  string `json:"coserMetadataRoot"`
			BackupRoot         string `json:"backupRoot"`
		}{status.Complete, options.RuntimeEnvironment, !options.directAllowed(request), options.CoserMetadataRoot, options.BackupRoot})
	})
}

func (s *Service) SetupTicketExchangeHandler() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var input struct {
			Ticket string `json:"ticket"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2048))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil {
			http.Error(response, "invalid ticket", http.StatusUnauthorized)
			return
		}
		token, expires, err := s.ExchangeSetupTicket(request.Context(), input.Ticket)
		if err != nil {
			http.Error(response, "invalid ticket", http.StatusUnauthorized)
			return
		}
		http.SetCookie(response, &http.Cookie{Name: SetupCookieName, Value: token, Path: "/setup", Expires: expires,
			MaxAge: int(timeUntil(expires, s.now()).Seconds()), HttpOnly: true, Secure: request.TLS != nil, SameSite: http.SameSiteStrictMode})
		response.WriteHeader(http.StatusNoContent)
	})
}

func (s *Service) CompleteSetupHandler(options SetupOptions) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !options.directAllowed(request) {
			cookie, _ := request.Cookie(SetupCookieName)
			if cookie == nil {
				http.Error(response, "Setup authorization required", http.StatusUnauthorized)
				return
			}
			valid, err := s.AuthenticateSetupSession(request.Context(), cookie.Value)
			if err != nil || !valid {
				http.Error(response, "Setup authorization required", http.StatusUnauthorized)
				return
			}
		}
		var input SetupInput
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 64*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			http.Error(response, "invalid Setup input", http.StatusBadRequest)
			return
		}
		if input.RuntimeEnvironment != options.RuntimeEnvironment {
			http.Error(response, "Setup environment mismatch", http.StatusBadRequest)
			return
		}
		if options.ValidateStorage != nil && options.ValidateStorage(input) != nil {
			http.Error(response, "Setup storage is unavailable or not writable", http.StatusBadRequest)
			return
		}
		if err := s.CompleteSetup(request.Context(), input); err != nil {
			http.Error(response, "Setup could not be completed", http.StatusBadRequest)
			return
		}
		http.SetCookie(response, &http.Cookie{Name: SetupCookieName, Path: "/setup", MaxAge: -1, HttpOnly: true,
			Secure: request.TLS != nil, SameSite: http.SameSiteStrictMode})
		response.WriteHeader(http.StatusNoContent)
	})
}

func isLoopbackHost(value string) bool {
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		host = value
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func isLoopbackRequest(request *http.Request) bool {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		host = request.RemoteAddr
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func timeUntil(value, now time.Time) time.Duration {
	if value.Before(now) {
		return 0
	}
	return value.Sub(now)
}
