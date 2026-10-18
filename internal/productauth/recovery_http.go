package productauth

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
)

func (s *Service) RecoveryHandler() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
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
			Token       string `json:"token"`
			NewPassword string `json:"newPassword"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || s.RecoverPassword(request.Context(), input.Token, input.NewPassword) != nil {
			s.recordLoginFailure(client)
			http.Error(response, "invalid recovery request", http.StatusBadRequest)
			return
		}
		s.clearLoginFailures(client)
		response.WriteHeader(http.StatusNoContent)
	})
}
