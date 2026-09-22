package productserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stashapp/stash/internal/portablecatalog"
)

type portablePackageEntry struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	Size          int64  `json:"size"`
	CreatedAt     string `json:"createdAt"`
	FormatVersion int    `json:"formatVersion"`
	CheckStatus   string `json:"checkStatus"`
}

func portableTransferRoot() string {
	if value := os.Getenv("CGM_TRANSFER_ROOT"); filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return "/transfer"
}

func portablePackageAt(root, name string) (portablePackageEntry, bool) {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") || !strings.EqualFold(filepath.Ext(name), ".zip") {
		return portablePackageEntry{}, false
	}
	path := filepath.Join(root, name)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return portablePackageEntry{}, false
	}
	return portablePackageEntry{Name: name, Path: path, Size: info.Size(), CheckStatus: "UNCHECKED"}, true
}

func (s *Server) portablePackagesHandler(root string) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !s.Auth.AuthorizeRequest(request) {
			http.Error(response, "authentication required", http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case http.MethodGet:
			entries, err := os.ReadDir(root)
			if err != nil {
				http.Error(response, "transfer directory unavailable", http.StatusServiceUnavailable)
				return
			}
			packages := make([]portablePackageEntry, 0)
			for _, entry := range entries {
				if len(packages) >= 500 {
					break
				}
				item, ok := portablePackageAt(root, entry.Name())
				if !ok {
					continue
				}
				manifest, err := portablecatalog.ReadPackageManifest(request.Context(), item.Path)
				if err == nil {
					item.CreatedAt = manifest.CreatedAt
					item.FormatVersion = manifest.FormatVersion
				} else {
					item.CheckStatus = "UNRECOGNIZED"
				}
				packages = append(packages, item)
			}
			sort.Slice(packages, func(i, j int) bool { return packages[i].Name < packages[j].Name })
			_ = json.NewEncoder(response).Encode(struct {
				Root     string                 `json:"root"`
				Packages []portablePackageEntry `json:"packages"`
			}{root, packages})
		case http.MethodPost:
			var input struct {
				Name string `json:"name"`
			}
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2048))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&input) != nil {
				http.Error(response, "invalid package selection", http.StatusBadRequest)
				return
			}
			item, ok := portablePackageAt(root, input.Name)
			if !ok {
				http.Error(response, "package unavailable", http.StatusNotFound)
				return
			}
			inspection, err := portablecatalog.InspectFile(request.Context(), item.Path)
			if err != nil {
				item.CheckStatus = "INVALID"
			} else {
				item.CheckStatus = "VALID"
				item.CreatedAt = inspection.Manifest.CreatedAt
				item.FormatVersion = inspection.Manifest.FormatVersion
			}
			_ = json.NewEncoder(response).Encode(item)
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}
