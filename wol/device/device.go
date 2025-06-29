package wol_device

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	wol_packet "wol-server/wol/packet"
)

type Device struct {
	Name        string    `json:"name"`
	MACAddress  string    `json:"mac_address"`
	Description string    `json:"description,omitempty"`
	IPAddress   string    `json:"ip_address,omitempty"`
	Port        int       `json:"port,omitempty"`
	LastWoken   time.Time `json:"last_woken,omitempty"`
	AddedAt     time.Time `json:"added_at"`
}

type DeviceStore struct {
	Devices    map[string]*Device `json:"devices"`
	configPath string
}

type DeviceConfig struct {
	ConfigPath string
}

func DefaultDeviceConfig() DeviceConfig {
	return DeviceConfig{
		ConfigPath: getDefaultConfigPath(),
	}
}

func NewDeviceStore(config DeviceConfig) (*DeviceStore, error) {
	store := &DeviceStore{
		Devices:    make(map[string]*Device),
		configPath: config.ConfigPath,
	}

	err := store.Load()
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load device store: %w", err)
	}

	return store, nil
}

func (ds *DeviceStore) AddDevice(name, macAddress, description, ipAddress string, port int) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("device name cannot be empty")
	}

	reservedNames := []string{"add-device", "list-devices", "remove-device", "show-device", "wake", "help"}
	for _, reserved := range reservedNames {
		if strings.ToLower(name) == reserved {
			return fmt.Errorf("device name '%s' is reserved", name)
		}
	}

	if err := wol_packet.ValidateMAC(macAddress); err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}

	cleanMAC := wol_packet.CleanMAC(macAddress)
	formattedMAC := fmt.Sprintf("%s:%s:%s:%s:%s:%s",
		cleanMAC[0:2], cleanMAC[2:4], cleanMAC[4:6],
		cleanMAC[6:8], cleanMAC[8:10], cleanMAC[10:12],
	)

	if _, exists := ds.Devices[name]; exists {
		return fmt.Errorf("device '%s' already exists", name)
	}

	for existingName, device := range ds.Devices {
		if wol_packet.CleanMAC(device.MACAddress) == cleanMAC {
			return fmt.Errorf("MAC address %s is already used by device '%s'", formattedMAC, existingName)
		}
	}

	if port == 0 {
		port = 9
	}

	device := &Device{
		Name:        name,
		MACAddress:  formattedMAC,
		Description: strings.TrimSpace(description),
		IPAddress:   strings.TrimSpace(ipAddress),
		Port:        port,
		AddedAt:     time.Now(),
	}

	ds.Devices[name] = device

	return ds.Save()

}

func (ds *DeviceStore) RemoveDevice(name string) error {

	if _, exists := ds.Devices[name]; !exists {
		return fmt.Errorf("device '%s' not found", name)
	}

	delete(ds.Devices, name)
	return ds.Save()
}

func (ds *DeviceStore) GetDevice(name string) (*Device, error) {
	device, exists := ds.Devices[name]
	if !exists {
		return nil, fmt.Errorf("device '%s' not found", name)
	}

	return device, nil
}

func (ds *DeviceStore) ListDevices() []*Device {
	devices := make([]*Device, 0, len(ds.Devices))
	for _, device := range ds.Devices {
		devices = append(devices, device)
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Name < devices[j].Name
	})

	return devices
}

func (ds *DeviceStore) UpdateLastWoken(name string) error {
	device, exists := ds.Devices[name]
	if !exists {
		return fmt.Errorf("device '%s' not found", name)
	}

	device.LastWoken = time.Now()
	return ds.Save()
}

func (ds *DeviceStore) DeviceExists(name string) bool {
	_, exists := ds.Devices[name]
	return exists
}

func (ds *DeviceStore) GetDeviceCount() int {
	return len(ds.Devices)
}

func (ds *DeviceStore) Load() error {
	data, err := os.ReadFile(ds.configPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, ds)
}

func (ds *DeviceStore) Save() error {
	configDir := filepath.Dir(ds.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(ds, "", "	")
	if err != nil {
		return fmt.Errorf("failed to marshal devices: %w", err)
	}

	err = os.WriteFile(ds.configPath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func getDefaultConfigPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "wol-devices.json"
	}

	return filepath.Join(configDir, "wol-server", "devices.json")
}
