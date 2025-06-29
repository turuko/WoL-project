package wol_device

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultDeviceConfig(t *testing.T) {
	config := DefaultDeviceConfig()

	if config.ConfigPath == "" {
		t.Error("DefaultDeviceConfig().ConfigPath should not be empty")
	}

	// Should end with devices.json
	if !(filepath.Base(config.ConfigPath) == "devices.json") {
		t.Errorf("Config path should end with devices.json, got: %s", config.ConfigPath)
	}
}

func TestNewDeviceStore(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-devices.json")

	config := DeviceConfig{
		ConfigPath: configPath,
	}

	store, err := NewDeviceStore(config)
	if err != nil {
		t.Fatalf("NewDeviceStore() error = %v, want nil", err)
	}

	if store == nil {
		t.Fatal("NewDeviceStore() returned nil store")
	}

	if store.Devices == nil {
		t.Error("DeviceStore.Devices should be initialized")
	}

	if len(store.Devices) != 0 {
		t.Errorf("New device store should be empty, got %d devices", len(store.Devices))
	}

	if store.configPath != configPath {
		t.Errorf("DeviceStore.configPath = %s, want %s", store.configPath, configPath)
	}
}

func TestDeviceStore_AddDevice(t *testing.T) {
	store := createTestStore(t)

	tests := []struct {
		name        string
		deviceName  string
		macAddress  string
		description string
		ipAddress   string
		port        int
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid device",
			deviceName:  "test-desktop",
			macAddress:  "AA:BB:CC:DD:EE:FF",
			description: "My test desktop",
			ipAddress:   "192.168.1.100",
			port:        9,
			wantErr:     false,
		},
		{
			name:        "valid device with default port",
			deviceName:  "laptop",
			macAddress:  "11:22:33:44:55:66",
			description: "Work laptop",
			ipAddress:   "",
			port:        0, // Should default to 9
			wantErr:     false,
		},
		{
			name:        "valid device different MAC format",
			deviceName:  "server",
			macAddress:  "22-33-44-55-66-77",
			description: "",
			ipAddress:   "",
			port:        7,
			wantErr:     false,
		},
		{
			name:        "empty device name",
			deviceName:  "",
			macAddress:  "AA:BB:CC:DD:EE:FF",
			description: "",
			ipAddress:   "",
			port:        9,
			wantErr:     true,
			errContains: "device name cannot be empty",
		},
		{
			name:        "whitespace only device name",
			deviceName:  "   ",
			macAddress:  "AA:BB:CC:DD:EE:FF",
			description: "",
			ipAddress:   "",
			port:        9,
			wantErr:     true,
			errContains: "device name cannot be empty",
		},
		{
			name:        "reserved device name",
			deviceName:  "add-device",
			macAddress:  "AA:BB:CC:DD:EE:FF",
			description: "",
			ipAddress:   "",
			port:        9,
			wantErr:     true,
			errContains: "device name 'add-device' is reserved",
		},
		{
			name:        "invalid MAC address",
			deviceName:  "invalid-mac",
			macAddress:  "invalid-mac-address",
			description: "",
			ipAddress:   "",
			port:        9,
			wantErr:     true,
			errContains: "invalid MAC address",
		},
		{
			name:        "duplicate device name",
			deviceName:  "test-desktop", // Already added in first test
			macAddress:  "FF:EE:DD:CC:BB:AA",
			description: "",
			ipAddress:   "",
			port:        9,
			wantErr:     true,
			errContains: "device 'test-desktop' already exists",
		},
		{
			name:        "duplicate MAC address",
			deviceName:  "duplicate-mac",
			macAddress:  "AA:BB:CC:DD:EE:FF", // Same as test-desktop
			description: "",
			ipAddress:   "",
			port:        9,
			wantErr:     true,
			errContains: "MAC address AA:BB:CC:DD:EE:FF is already used by device 'test-desktop'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.AddDevice(tt.deviceName, tt.macAddress, tt.description, tt.ipAddress, tt.port)

			if tt.wantErr {
				if err == nil {
					t.Errorf("AddDevice() expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("AddDevice() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("AddDevice() unexpected error = %v, name = %s", err, tt.deviceName)
					return
				}

				// Verify device was added correctly
				device, exists := store.Devices[tt.deviceName]
				if !exists {
					t.Errorf("Device %s was not added to store", tt.deviceName)
					return
				}

				if device.Name != tt.deviceName {
					t.Errorf("Device.Name = %s, want %s", device.Name, tt.deviceName)
				}

				if device.Description != tt.description {
					t.Errorf("Device.Description = %s, want %s", device.Description, tt.description)
				}

				if device.IPAddress != tt.ipAddress {
					t.Errorf("Device.IPAddress = %s, want %s", device.IPAddress, tt.ipAddress)
				}

				expectedPort := tt.port
				if expectedPort == 0 {
					expectedPort = 9 // Default port
				}
				if device.Port != expectedPort {
					t.Errorf("Device.Port = %d, want %d", device.Port, expectedPort)
				}

				// Verify MAC address is properly formatted
				if !isValidMACFormat(device.MACAddress) {
					t.Errorf("Device.MACAddress format is invalid: %s", device.MACAddress)
				}

				// Verify timestamps
				if device.AddedAt.IsZero() {
					t.Error("Device.AddedAt should be set")
				}

				if !device.LastWoken.IsZero() {
					t.Error("Device.LastWoken should be zero for new device")
				}
			}
		})
	}
}

func TestDeviceStore_RemoveDevice(t *testing.T) {
	store := createTestStore(t)

	// Add a device first
	err := store.AddDevice("test-device", "AA:BB:CC:DD:EE:FF", "Test device", "", 9)
	if err != nil {
		t.Fatalf("Failed to add test device: %v", err)
	}

	tests := []struct {
		name        string
		deviceName  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "remove existing device",
			deviceName: "test-device",
			wantErr:    false,
		},
		{
			name:        "remove non-existent device",
			deviceName:  "non-existent",
			wantErr:     true,
			errContains: "device 'non-existent' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initialCount := len(store.Devices)

			err := store.RemoveDevice(tt.deviceName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("RemoveDevice() expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("RemoveDevice() error = %v, want error containing %q", err, tt.errContains)
				}

				// Verify count didn't change
				if len(store.Devices) != initialCount {
					t.Errorf("Device count changed after failed removal: was %d, now %d", initialCount, len(store.Devices))
				}
			} else {
				if err != nil {
					t.Errorf("RemoveDevice() unexpected error = %v", err)
					return
				}

				// Verify device was removed
				if _, exists := store.Devices[tt.deviceName]; exists {
					t.Errorf("Device %s still exists after removal", tt.deviceName)
				}

				// Verify count decreased
				if len(store.Devices) != initialCount-1 {
					t.Errorf("Device count should be %d after removal, got %d", initialCount-1, len(store.Devices))
				}
			}
		})
	}
}

func TestDeviceStore_GetDevice(t *testing.T) {
	store := createTestStore(t)

	// Add test devices
	testDevices := map[string]string{
		"desktop": "AA:BB:CC:DD:EE:FF",
		"laptop":  "11:22:33:44:55:66",
	}

	for name, mac := range testDevices {
		err := store.AddDevice(name, mac, "Test device", "", 9)
		if err != nil {
			t.Fatalf("Failed to add test device %s: %v", name, err)
		}
	}

	tests := []struct {
		name        string
		deviceName  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "get existing device",
			deviceName: "desktop",
			wantErr:    false,
		},
		{
			name:       "get another existing device",
			deviceName: "laptop",
			wantErr:    false,
		},
		{
			name:        "get non-existent device",
			deviceName:  "non-existent",
			wantErr:     true,
			errContains: "device 'non-existent' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device, err := store.GetDevice(tt.deviceName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetDevice() expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("GetDevice() error = %v, want error containing %q", err, tt.errContains)
				}
				if device != nil {
					t.Error("GetDevice() should return nil device on error")
				}
			} else {
				if err != nil {
					t.Errorf("GetDevice() unexpected error = %v", err)
					return
				}
				if device == nil {
					t.Error("GetDevice() should return device on success")
					return
				}
				if device.Name != tt.deviceName {
					t.Errorf("Device.Name = %s, want %s", device.Name, tt.deviceName)
				}
			}
		})
	}
}

func TestDeviceStore_ListDevices(t *testing.T) {
	store := createTestStore(t)

	// Test empty store
	devices := store.ListDevices()
	if len(devices) != 0 {
		t.Errorf("Empty store should return 0 devices, got %d", len(devices))
	}

	// Add devices in non-alphabetical order
	testDevices := []struct {
		name string
		mac  string
	}{
		{"zebra", "AA:BB:CC:DD:EE:01"},
		{"alpha", "AA:BB:CC:DD:EE:02"},
		{"beta", "AA:BB:CC:DD:EE:03"},
	}

	for _, td := range testDevices {
		err := store.AddDevice(td.name, td.mac, "Test device", "", 9)
		if err != nil {
			t.Fatalf("Failed to add test device %s: %v", td.name, err)
		}
	}

	// Test populated store
	devices = store.ListDevices()

	if len(devices) != len(testDevices) {
		t.Errorf("ListDevices() returned %d devices, want %d", len(devices), len(testDevices))
	}

	// Verify alphabetical ordering
	expectedOrder := []string{"alpha", "beta", "zebra"}
	for i, device := range devices {
		if device.Name != expectedOrder[i] {
			t.Errorf("Device at index %d has name %s, want %s", i, device.Name, expectedOrder[i])
		}
	}
}

func TestDeviceStore_UpdateLastWoken(t *testing.T) {
	store := createTestStore(t)

	// Add a test device
	err := store.AddDevice("test-device", "AA:BB:CC:DD:EE:FF", "Test device", "", 9)
	if err != nil {
		t.Fatalf("Failed to add test device: %v", err)
	}

	// Get initial LastWoken time (should be zero)
	device, _ := store.GetDevice("test-device")
	if !device.LastWoken.IsZero() {
		t.Error("Initial LastWoken should be zero")
	}

	// Update LastWoken
	beforeUpdate := time.Now()
	err = store.UpdateLastWoken("test-device")
	afterUpdate := time.Now()

	if err != nil {
		t.Errorf("UpdateLastWoken() unexpected error = %v", err)
	}

	// Verify LastWoken was updated
	device, _ = store.GetDevice("test-device")
	if device.LastWoken.IsZero() {
		t.Error("LastWoken should be set after update")
	}

	if device.LastWoken.Before(beforeUpdate) || device.LastWoken.After(afterUpdate) {
		t.Errorf("LastWoken time %v should be between %v and %v", device.LastWoken, beforeUpdate, afterUpdate)
	}

	// Test updating non-existent device
	err = store.UpdateLastWoken("non-existent")
	if err == nil {
		t.Error("UpdateLastWoken() should return error for non-existent device")
	}
	if !contains(err.Error(), "device 'non-existent' not found") {
		t.Errorf("UpdateLastWoken() error = %v, want error containing 'not found'", err)
	}
}

func TestDeviceStore_DeviceExists(t *testing.T) {
	store := createTestStore(t)

	// Test non-existent device
	if store.DeviceExists("non-existent") {
		t.Error("DeviceExists() should return false for non-existent device")
	}

	// Add a device
	err := store.AddDevice("test-device", "AA:BB:CC:DD:EE:FF", "Test device", "", 9)
	if err != nil {
		t.Fatalf("Failed to add test device: %v", err)
	}

	// Test existing device
	if !store.DeviceExists("test-device") {
		t.Error("DeviceExists() should return true for existing device")
	}

	// Test case sensitivity
	if store.DeviceExists("TEST-DEVICE") {
		t.Error("DeviceExists() should be case sensitive")
	}
}

func TestDeviceStore_GetDeviceCount(t *testing.T) {
	store := createTestStore(t)

	// Test empty store
	if store.GetDeviceCount() != 0 {
		t.Errorf("Empty store should have 0 devices, got %d", store.GetDeviceCount())
	}

	// Add devices and verify count
	deviceCount := 3
	for i := 0; i < deviceCount; i++ {
		name := fmt.Sprintf("device-%d", i)
		mac := fmt.Sprintf("AA:BB:CC:DD:EE:%02X", i)
		err := store.AddDevice(name, mac, "Test device", "", 9)
		if err != nil {
			t.Fatalf("Failed to add device %s: %v", name, err)
		}

		expectedCount := i + 1
		if store.GetDeviceCount() != expectedCount {
			t.Errorf("After adding %d devices, count should be %d, got %d", expectedCount, expectedCount, store.GetDeviceCount())
		}
	}
}

func TestDeviceStore_SaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "save-load-test.json")

	// Create store and add devices
	config := DeviceConfig{ConfigPath: configPath}
	store1, err := NewDeviceStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Add test devices
	testDevices := []struct {
		name, mac, desc, ip string
		port                int
	}{
		{"desktop", "AA:BB:CC:DD:EE:FF", "My desktop", "192.168.1.100", 9},
		{"laptop", "11:22:33:44:55:66", "Work laptop", "", 7},
		{"server", "FF:EE:DD:CC:BB:AA", "", "10.0.0.50", 9},
	}

	for _, td := range testDevices {
		err := store1.AddDevice(td.name, td.mac, td.desc, td.ip, td.port)
		if err != nil {
			t.Fatalf("Failed to add device %s: %v", td.name, err)
		}
	}

	// Update LastWoken for one device
	err = store1.UpdateLastWoken("desktop")
	if err != nil {
		t.Fatalf("Failed to update LastWoken: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("Config file was not created at %s", configPath)
	}

	// Load into new store
	store2, err := NewDeviceStore(config)
	if err != nil {
		t.Fatalf("Failed to load store: %v", err)
	}

	// Verify all devices were loaded correctly
	if store2.GetDeviceCount() != len(testDevices) {
		t.Errorf("Loaded store has %d devices, want %d", store2.GetDeviceCount(), len(testDevices))
	}

	for _, td := range testDevices {
		device, err := store2.GetDevice(td.name)
		if err != nil {
			t.Errorf("Failed to get device %s from loaded store: %v", td.name, err)
			continue
		}

		if device.Description != td.desc {
			t.Errorf("Device %s description = %s, want %s", td.name, device.Description, td.desc)
		}

		if device.IPAddress != td.ip {
			t.Errorf("Device %s IP = %s, want %s", td.name, device.IPAddress, td.ip)
		}

		if device.Port != td.port {
			t.Errorf("Device %s port = %d, want %d", td.name, device.Port, td.port)
		}
	}

	// Verify LastWoken was preserved
	desktopDevice, _ := store2.GetDevice("desktop")
	if desktopDevice.LastWoken.IsZero() {
		t.Error("LastWoken should be preserved after load")
	}
}

func TestDeviceStore_SaveError(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "corrupt.json")

	// Create a corrupt JSON file that should fail to load
	err := os.WriteFile(configPath, []byte("{invalid json content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create corrupt file: %v", err)
	}

	config := DeviceConfig{ConfigPath: configPath}

	// This should fail to load the corrupt JSON
	_, err = NewDeviceStore(config)
	if err == nil {
		t.Error("NewDeviceStore() should fail when loading corrupt JSON")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

// Helper functions

func createTestStore(t *testing.T) *DeviceStore {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-devices.json")

	config := DeviceConfig{ConfigPath: configPath}
	store, err := NewDeviceStore(config)
	if err != nil {
		t.Fatalf("Failed to create test store: %v", err)
	}

	return store
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func isValidMACFormat(mac string) bool {
	// Should be in format AA:BB:CC:DD:EE:FF
	if len(mac) != 17 {
		return false
	}

	for i, char := range mac {
		if i%3 == 2 { // Every third character should be ':'
			if char != ':' {
				return false
			}
		} else {
			// Should be hex digit
			if !((char >= '0' && char <= '9') || (char >= 'A' && char <= 'F')) {
				return false
			}
		}
	}

	return true
}
