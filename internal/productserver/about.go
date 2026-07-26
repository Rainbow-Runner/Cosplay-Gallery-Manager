package productserver

import (
	"encoding/json"
	"net/http"

	"github.com/stashapp/stash/internal/build"
	"github.com/stashapp/stash/internal/product"
)

type aboutResponse struct {
	Product              string `json:"product"`
	Version              string `json:"version"`
	GitHash              string `json:"gitHash,omitempty"`
	BuildTime            string `json:"buildTime,omitempty"`
	SourceCodeURL        string `json:"sourceCodeURL"`
	ExactSourceAvailable bool   `json:"exactSourceAvailable"`
	License              string `json:"license"`
	Warranty             string `json:"warranty"`
	Attribution          string `json:"attribution"`
}

func aboutHandler() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			response.Header().Set("Allow", "GET, HEAD")
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		version, gitHash, buildTime := build.Version()
		result := aboutResponse{
			Product: product.WorkingName, Version: product.CurrentVersions(version).Product,
			GitHash: gitHash, BuildTime: buildTime, SourceCodeURL: product.SourceCodeURL(gitHash),
			ExactSourceAvailable: product.SourceCodeURL(gitHash) != product.SourceRepositoryURL,
			License:              "GNU Affero General Public License v3.0 or later (AGPL-3.0-or-later)",
			Warranty:             "This program comes with absolutely no warranty, to the extent permitted by applicable law.",
			Attribution:          "Derived from Stash; Stash copyright and third-party notices are preserved with the source distribution.",
		}
		response.Header().Set("Content-Type", "application/json; charset=utf-8")
		response.Header().Set("Cache-Control", "no-store")
		if request.Method == http.MethodHead {
			response.WriteHeader(http.StatusOK)
			return
		}
		_ = json.NewEncoder(response).Encode(result)
	})
}
