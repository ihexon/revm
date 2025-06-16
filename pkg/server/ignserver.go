package server

import (
	"context"
	"encoding/json"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/vmconfig"
	"net/http"

	"github.com/sirupsen/logrus"
)

type Handler struct {
	vmc *vmconfig.VMConfig
}

// MountResponse represents the HTTP response structure for mount information
type MountResponse struct {
	Mounts []filesystem.Mount `json:"mounts"`
}

func (h *Handler) HandleMounts(w http.ResponseWriter, r *http.Request) {
	response := MountResponse{
		Mounts: h.vmc.Mounts,
	}
	
	WriteJSON(w, http.StatusOK, response)
}

func NewHandler(vmc *vmconfig.VMConfig) *Handler {
	return &Handler{vmc: vmc}
}

func IgnServer(ctx context.Context, vmc *vmconfig.VMConfig) error {
	http.HandleFunc("/host/virtiofs", NewHandler(vmc).HandleMounts)
	return http.ListenAndServe(":8080", nil)
}

// WriteJSON writes an interface value encoded as JSON to w
func WriteJSON(w http.ResponseWriter, code int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	coder := json.NewEncoder(w)
	coder.SetEscapeHTML(true)
	if err := coder.Encode(value); err != nil {
		logrus.Errorf("Unable to write json: %q", err)
	}
}
