package wol_server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
	wol_device "wol-server/wol/device"
	wol_log "wol-server/wol/log"
	wol_network "wol-server/wol/network"

	"github.com/gorilla/mux"
)

type ServerConfig struct {
	Port        int
	Host        string
	DeviceStore *wol_device.DeviceStore
	Logger      *wol_log.Logger
	EnableCORS  bool
}

type WoLServer struct {
	config     ServerConfig
	router     *mux.Router
	httpServer *http.Server
	startTime  time.Time
}

type AddDeviceRequest struct {
	Name        string `json:"name"`
	MACAddress  string `json:"mac"`
	Description string `json:"description,omitempty"`
	IPAddress   string `json:"ip_address,omitempty"`
	Port        int    `json:"port,omitempty"`
}

type UpdateDeviceRequest struct {
	Description string `json:"description,omitempty"`
	IPAddress   string `json:"ip_address,omitempty"`
	Port        int    `json:"port,omitempty"`
}

type WakeRequest struct {
	MAC  string `json:"mac"`
	Port int    `json:"port,omitempty"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type HealthData struct {
	Status      string `json:"status"`
	Uptime      string `json:"uptime"`
	DeviceCount int    `json:"device_count"`
	Version     string `json:"version"`
}

func NewWoLServer(config ServerConfig) *WoLServer {
	server := &WoLServer{
		config:    config,
		router:    mux.NewRouter(),
		startTime: time.Now(),
	}

	server.setupRoutes()
	return server
}

func (s *WoLServer) setupRoutes() {
	api := s.router.PathPrefix("/api").Subrouter()

	api.HandleFunc("/devices", s.handleListDevices).Methods("GET")
	api.HandleFunc("/devices", s.handleAddDevice).Methods("POST")
	api.HandleFunc("/devices/{name}", s.handleGetDevice).Methods("GET")
	api.HandleFunc("/devices/{name}", s.handleUpdateDevice).Methods("PUT")
	api.HandleFunc("/devices/{name}", s.handleRemoveDevice).Methods("DELETE")

	api.HandleFunc("/wake/{name}", s.handleWakeByName).Methods("POST")
	api.HandleFunc("/wake", s.handleWakeByMAC).Methods("POST")

	api.HandleFunc("/health", s.handleHealth).Methods("GET")

	s.router.HandleFunc("/", s.handleRoot).Methods("GET")

	if s.config.EnableCORS {
		s.router.Use(s.corsMiddleware)
	}
	s.router.Use(s.loggingMiddleware)
}

func (s *WoLServer) handleListDevices(w http.ResponseWriter, r *http.Request) {
	devices := s.config.DeviceStore.ListDevices()
	s.config.Logger.Debug("API: Listed %d devices", len(devices))

	s.writeJSONResponse(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    devices,
		Message: fmt.Sprintf("Found %d devices", len(devices)),
	})
}

func (s *WoLServer) handleAddDevice(w http.ResponseWriter, r *http.Request) {
	var req AddDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.config.Logger.Warn("API: Invlaid JSON in add device request: %v", err)
		s.writeJSONError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	if req.Name == "" {
		s.writeJSONError(w, http.StatusBadRequest, "Device name is required")
		return
	}
	if req.MACAddress == "" {
		s.writeJSONError(w, http.StatusBadRequest, "MAC address is required")
		return
	}

	err := s.config.DeviceStore.AddDevice(req.Name, req.MACAddress, req.Description, req.IPAddress, req.Port)
	if err != nil {
		s.config.Logger.Error("API: Failed to add device %s: %v", req.Name, err)
		s.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.config.Logger.Info("API: Device %s added successfully", req.Name)
	s.writeJSONResponse(w, http.StatusCreated, APIResponse{
		Success: true,
		Message: fmt.Sprintf("Device '%s' added successfully", req.Name),
	})
}

func (s *WoLServer) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	device, err := s.config.DeviceStore.GetDevice(name)
	if err != nil {
		s.config.Logger.Debug("API: Device %s not found", name)
		s.writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	s.config.Logger.Debug("API: Retrieved device %s", name)
	s.writeJSONResponse(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    device,
	})
}

func (s *WoLServer) handleUpdateDevice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	// Check if device exists
	device, err := s.config.DeviceStore.GetDevice(name)
	if err != nil {
		s.writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	var req UpdateDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	// Update fields (keep existing MAC and name)
	description := req.Description
	if description == "" {
		description = device.Description
	}

	ipAddress := req.IPAddress
	if ipAddress == "" {
		ipAddress = device.IPAddress
	}

	port := req.Port
	if port == 0 {
		port = device.Port
	}

	// Remove and re-add device with updated info
	err = s.config.DeviceStore.RemoveDevice(name)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to update device")
		return
	}

	err = s.config.DeviceStore.AddDevice(name, device.MACAddress, description, ipAddress, port)
	if err != nil {
		s.config.Logger.Error("API: Failed to update device %s: %v", name, err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to update device: "+err.Error())
		return
	}

	s.config.Logger.Info("API: Device %s updated successfully", name)
	s.writeJSONResponse(w, http.StatusOK, APIResponse{
		Success: true,
		Message: fmt.Sprintf("Device '%s' updated successfully", name),
	})
}

func (s *WoLServer) handleRemoveDevice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	err := s.config.DeviceStore.RemoveDevice(name)
	if err != nil {
		s.config.Logger.Error("API: Failed to remove device %s: %v", name, err)
		s.writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	s.config.Logger.Info("API: Device %s removed successfully", name)
	s.writeJSONResponse(w, http.StatusOK, APIResponse{
		Success: true,
		Message: fmt.Sprintf("Device '%s' removed successfully", name),
	})
}

func (s *WoLServer) handleWakeByName(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	port := s.getPortFromQuery(r)

	device, err := s.config.DeviceStore.GetDevice(name)
	if err != nil {
		s.config.Logger.Debug("API: Wake failed - device %s not found", name)
		s.writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	if port == 0 {
		port = device.Port
	}

	s.config.Logger.Info("API: Attempting to wake devise %s (%s) on port %d", name, device.MACAddress, port)

	err = wol_network.SendWakeOnLAN(device.MACAddress, port)
	if err != nil {
		s.config.Logger.Error("API: Failed to wake device %s: %v", name, err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to send wake packet: "+err.Error())
		return
	}

	err = s.config.DeviceStore.UpdateLastWoken(name)
	if err != nil {
		s.config.Logger.Warn("API: Failed to update last woken time for %s: %v", name, err)
	}

	s.config.Logger.Info("API: Device %s woken successfully", name)
	s.writeJSONResponse(w, http.StatusOK, APIResponse{
		Success: true,
		Message: fmt.Sprintf("Wake packet sent to '%s' (%s) on port %d", name, device.MACAddress, port),
	})
}

func (s *WoLServer) handleWakeByMAC(w http.ResponseWriter, r *http.Request) {
	var req WakeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	if req.MAC == "" {
		s.writeJSONError(w, http.StatusBadRequest, "MAC address is required")
		return
	}

	port := req.Port
	if port == 0 {
		port = wol_network.DefaultWoLPort
	}

	s.config.Logger.Info("API: Attempting to wake MAC %s on port %d", req.MAC, port)

	err := wol_network.SendWakeOnLAN(req.MAC, port)
	if err != nil {
		s.config.Logger.Error("API: Failed to wake MAC %s: %v", req.MAC, err)
		s.writeJSONError(w, http.StatusBadRequest, "Failed to send wake packet: "+err.Error())
		return
	}

	s.config.Logger.Info("API: MAC %s woken successfully", req.MAC)
	s.writeJSONResponse(w, http.StatusOK, APIResponse{
		Success: true,
		Message: fmt.Sprintf("Wake packet sent to %s on port %d", req.MAC, port),
	})
}

func (s *WoLServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startTime)

	s.writeJSONResponse(w, http.StatusOK, APIResponse{
		Success: true,
		Data: HealthData{
			Status:      "healthy",
			Uptime:      uptime.Round(time.Second).String(),
			DeviceCount: s.config.DeviceStore.GetDeviceCount(),
			Version:     "1.0.0",
		},
	})
}

func (s *WoLServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"service": "Wake-on-LAN Server",
		"version": "1.0.0",
		"status":  "running",
		"endpoints": map[string]string{
			"health":       "/api/health",
			"devices":      "/api/devices",
			"wake_by_name": "/api/wake/{name}",
			"wake_by_mac":  "/api/wake",
		},
	}

	s.writeJSONResponse(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    response,
	})
}

func (s *WoLServer) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.config.Logger.Info("Starting WoL HTTP server on %s", addr)
	fmt.Printf("WoL Server starting on http://%s\n", addr)
	fmt.Printf("API endpoints available at http://%s/api/\n", addr)

	return s.httpServer.ListenAndServe()
}

func (s *WoLServer) Stop() error {
	if s.httpServer != nil {
		s.config.Logger.Info("Stopping WoL HTTP server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

func (s *WoLServer) writeJSONResponse(w http.ResponseWriter, status int, response APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.config.Logger.Error("Failed to encode JSON response: %v", err)
	}
}

func (s *WoLServer) writeJSONError(w http.ResponseWriter, status int, message string) {
	s.writeJSONResponse(w, status, APIResponse{
		Success: false,
		Error:   message,
	})
}

func (s *WoLServer) getPortFromQuery(r *http.Request) int {
	portStr := r.URL.Query().Get("port")
	if portStr == "" {
		return 0
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}

	return port
}

func (s *WoLServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *WoLServer) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		s.config.Logger.Info("HTTP %s %s - %d - %v", r.Method, r.URL.Path, wrapped.statusCode, duration)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
