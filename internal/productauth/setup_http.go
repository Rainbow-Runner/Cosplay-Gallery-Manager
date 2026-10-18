package productauth

import (
	"encoding/json"
	"net/http"
)

type SetupOptions struct {
	RuntimeEnvironment string
	CoserMetadataRoot  string
	BackupRoot         string
	ValidateStorage    func(SetupInput) error
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
			CoserMetadataRoot  string `json:"coserMetadataRoot"`
			BackupRoot         string `json:"backupRoot"`
		}{status.Complete, options.RuntimeEnvironment, options.CoserMetadataRoot, options.BackupRoot})
	})
}

func (s *Service) CompleteSetupHandler(options SetupOptions) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
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
		response.WriteHeader(http.StatusNoContent)
	})
}
