package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Traliaa/vpn-manager/internal/db"
	"github.com/Traliaa/vpn-manager/internal/vpn/importer"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

// importRequest is the JSON body for importing a config.
type importRequest struct {
	ConfigText string `json:"config_text"`
	Name       string `json:"name,omitempty"`
}

// ImportConfig parses a WireGuard/AmneziaWG .conf file and creates a provider.
// POST /api/v1/providers/import
//
// Accepts:
//   - multipart/form-data with field "config_file" (.conf upload)
//   - application/json with {"config_text": "...", "name": "..."}
func (h *Handlers) ImportConfig(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")

	var configText, providerName string

	switch {
	case isMultipart(contentType):
		// Parse multipart form (max 1MB)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "failed to parse multipart form: "+err.Error())
			return
		}

		file, header, err := r.FormFile("config_file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "config_file field is required")
			return
		}
		defer file.Close()

		bytes, err := io.ReadAll(file)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read file")
			return
		}
		configText = string(bytes)

		// Derive name from filename if not provided via form
		if fname := r.FormValue("name"); fname != "" {
			providerName = fname
		} else {
			// Strip extension from filename
			providerName = stripExt(header.Filename)
		}

	case contentType == "application/json":
		var req importRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		configText = req.ConfigText
		providerName = req.Name

	default:
		writeError(w, http.StatusBadRequest, "unsupported content type; use multipart/form-data or application/json")
		return
	}

	if configText == "" {
		writeError(w, http.StatusBadRequest, "config text is empty")
		return
	}

	// Parse the config
	parsed, err := importer.Parse(configText)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse config: "+err.Error())
		return
	}

	// Determine provider type and marshal config
	var providerType string
	var configJSON []byte

	if parsed.IsAmneziaWG {
		providerType = "amneziawg"
		awgCfg := parsed.ToAmneziaWGConfig()
		configJSON, err = json.Marshal(awgCfg)
	} else {
		providerType = "wireguard"
		wgCfg := parsed.ToWireGuardConfig()
		configJSON, err = json.Marshal(wgCfg)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to marshal config")
		return
	}

	// Derive name from endpoint if not provided
	if providerName == "" {
		providerName = parsed.ProviderName
	}
	if providerName == "" {
		providerName = "imported-vpn"
	}

	// Create provider using existing queries
	provider, err := h.q.CreateProvider(r.Context(), db.CreateProviderParams{
		Name:         providerName,
		ProviderType: db.ProviderType(providerType),
		Config:       string(configJSON),
		Enabled:      true,
		Priority:     100,
		HealthHost:   pgtype.Text{},
	})
	if err != nil {
		h.logger.Error("import: create provider",
			zap.Error(err),
		)
		writeError(w, http.StatusInternalServerError, "failed to create provider: "+err.Error())
		return
	}

	h.audit(r.Context(), "import", "vpn_providers", provider.ID, map[string]any{
		"name": providerName,
		"type": providerType,
	})

	writeJSON(w, http.StatusCreated, map[string]any{
		"provider": provider,
		"detected": providerType,
	})
}

// isMultipart checks if the content type is multipart/form-data.
func isMultipart(contentType string) bool {
	// Handle cases like "multipart/form-data; boundary=..."
	if len(contentType) < 19 {
		return false
	}
	return contentType[:19] == "multipart/form-data"
}

// stripExt removes the file extension from a filename.
func stripExt(filename string) string {
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '.' {
			return filename[:i]
		}
		if filename[i] == '/' || filename[i] == '\\' {
			break
		}
	}
	return filename
}
