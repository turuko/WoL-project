package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
	wol_device "wol-server/wol/device"
	wol_log "wol-server/wol/log"
	wol_network "wol-server/wol/network"
	wol_packet "wol-server/wol/packet"
)

func main() {
	var (
		port       = flag.Int("port", wol_network.DefaultWoLPort, "UDP port to send Wake-on-LAN packet (default: 9)")
		help       = flag.Bool("help", false, "Show help message")
		logFile    = flag.String("log", "", "Log file path (default: console only)")
		logLevel   = flag.String("level", "info", "Log level: debug, info, warn, error")
		verbose    = flag.Bool("verbose", false, "Enable verbose output (same as -level debug)")
		quiet      = flag.Bool("quiet", false, "Quiet mode - only errors (same as -level error)")
		configPath = flag.String("config", "", "Device configuration file path (default: system config directory)")
	)

	flag.Parse()

	if *help {
		showHelp()
		return
	}

	logger, err := setupLogging(*logFile, *logLevel, *verbose, *quiet)
	if err != nil {
		fmt.Printf("Error setting up logging: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	wol_network.SetLogger(logger)

	deviceConfig := wol_device.DefaultDeviceConfig()
	if *configPath != "" {
		deviceConfig.ConfigPath = *configPath
	}

	deviceStore, err := wol_device.NewDeviceStore(deviceConfig)
	if err != nil {
		fmt.Print("Error setting up device store: %v\n", err)
		logger.Error("Failed to initialize device store: %v", err)
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Error: MAC address is required")
		fmt.Println()
		showUsage()
		os.Exit(1)
	}

	command := args[0]

	switch command {
	case "add-device", "add":
		handleAddDevice(args, deviceStore, logger)
	case "list-devices", "list", "ls":
		handleListDevices(deviceStore, logger)
	case "remove-device", "remove", "rm":
		handleRemoveDevice(args, deviceStore, logger)
	case "show-device", "show":
		handleShowDevice(args, deviceStore, logger)
	case "wake":
		if len(args) < 2 {
			fmt.Println("Error: Device name or MAC address required for wake command")
			os.Exit(1)
		}
		handleWake(args[1], *port, deviceStore, logger)
	default:
		// Assume it's a device name or MAC address for wake-up
		handleWake(command, *port, deviceStore, logger)
	}
}

func handleAddDevice(args []string, store *wol_device.DeviceStore, logger *wol_log.Logger) {
	if len(args) < 3 {
		fmt.Println("Usage: wol-server add-device <name> <mac-address> [description] [ip-address] [port]")
		fmt.Println("Example: wol-server add-device desktop AA:BB:CC:DD:EE:FF \"My desktop computer\" 192.168.1.100 9")
		os.Exit(1)
	}

	name := args[1]
	macAddress := args[2]
	description := ""
	ipAddress := ""
	port := 0

	if len(args) > 3 {
		description = args[3]
	}

	if len(args) > 4 {
		ipAddress = args[4]
	}

	if len(args) > 5 {
		fmt.Sscanf(args[5], "%d", &port)
	}

	logger.Info("Adding device: name=%s, mac=%s", name, macAddress)

	err := store.AddDevice(name, macAddress, description, ipAddress, port)
	if err != nil {
		fmt.Printf("Error: Failed to add device: %v\n", err)
		logger.Error("Failed to add device %s: %v\n", name, err)
		os.Exit(1)
	}

	fmt.Printf("✓ Device '%s' added successfully\n", name)
	logger.Info("Device %s added successfully", name)
}

func handleListDevices(store *wol_device.DeviceStore, logger *wol_log.Logger) {
	devices := store.ListDevices()

	if len(devices) == 0 {
		fmt.Println("No devices configured.")
		fmt.Println("Use 'wol-server add-device <name> <mac>' to add a device.")
		return
	}

	fmt.Printf("Configured Devices (%d):\n", len(devices))
	fmt.Println(strings.Repeat("=", 80))

	for _, device := range devices {
		fmt.Printf("Name:        %s\n", device.Name)
		fmt.Printf("MAC:         %s\n", device.MACAddress)

		if device.Description != "" {
			fmt.Printf("Description: %s\n", device.Description)
		}

		if device.IPAddress != "" {
			fmt.Printf("IP Address:  %s\n", device.IPAddress)
		}

		fmt.Printf("Port:        %d\n", device.Port)
		fmt.Printf("Added:       %s\n", device.AddedAt.Format("2006-01-02 15:04:05"))

		if !device.LastWoken.IsZero() {
			fmt.Printf("Last Woken:  %s\n", device.LastWoken.Format("2006-01-02 15:04:05"))
		}

		fmt.Println(strings.Repeat("-", 80))
	}

	logger.Debug("Listed %d devices", len(devices))
}

func handleRemoveDevice(args []string, store *wol_device.DeviceStore, logger *wol_log.Logger) {
	if len(args) < 2 {
		fmt.Println("Usage: wol-server remove-device <name>")
		fmt.Println("Example: wol-server remove-device desktop")
		os.Exit(1)
	}

	name := args[1]

	if !store.DeviceExists(name) {
		fmt.Printf("Error: Device '%s' not found\n", name)
		fmt.Println("Use 'wol-server list-devices' to see available devices.")
		os.Exit(1)
	}

	logger.Info("Removing device: %s", name)

	err := store.RemoveDevice(name)
	if err != nil {
		fmt.Printf("Error: Failed to remove device: %v\n", err)
		logger.Error("Failed to remove device %s: %v", name, err)
		os.Exit(1)
	}

	fmt.Printf("✓ Device '%s' removed successfully\n", name)
	logger.Info("Device %s removed successfully", name)
}

func handleShowDevice(args []string, store *wol_device.DeviceStore, logger *wol_log.Logger) {
	if len(args) < 2 {
		fmt.Println("Usage: wol-server show-device <name>")
		fmt.Println("Example: wol-server show-device desktop")
		os.Exit(1)
	}

	name := args[1]

	device, err := store.GetDevice(name)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Use 'wol-server list-devices' to see available devices.")
		os.Exit(1)
	}

	fmt.Printf("Device Details: %s\n", device.Name)
	fmt.Println(strings.Repeat("=", 40))
	fmt.Printf("Name:        %s\n", device.Name)
	fmt.Printf("MAC Address: %s\n", device.MACAddress)

	if device.Description != "" {
		fmt.Printf("Description: %s\n", device.Description)
	}

	if device.IPAddress != "" {
		fmt.Printf("IP Address:  %s\n", device.IPAddress)
	}

	fmt.Printf("Port:        %d\n", device.Port)
	fmt.Printf("Added:       %s\n", device.AddedAt.Format("2006-01-02 15:04:05"))

	if !device.LastWoken.IsZero() {
		fmt.Printf("Last Woken:  %s\n", device.LastWoken.Format("2006-01-02 15:04:05"))
		fmt.Printf("Time Since:  %s\n", time.Since(device.LastWoken).Round(time.Second))
	} else {
		fmt.Println("Last Woken:  Never")
	}

	logger.Debug("Showed device details for %s", name)
}

func handleWake(target string, port int, store *wol_device.DeviceStore, logger *wol_log.Logger) {
	var macAddress string
	var deviceName string

	// Check if target is a device name
	if store.DeviceExists(target) {
		device, err := store.GetDevice(target)
		if err != nil {
			fmt.Printf("Error: Failed to get device %s: %v\n", target, err)
			os.Exit(1)
		}

		macAddress = device.MACAddress
		deviceName = device.Name

		// Use device's configured port if not overridden
		if port == wol_network.DefaultWoLPort && device.Port != wol_network.DefaultWoLPort {
			port = device.Port
		}

		logger.Info("Waking device by name: %s (MAC: %s)", deviceName, macAddress)
	} else {
		// Assume it's a MAC address
		if err := wol_packet.ValidateMAC(target); err != nil {
			fmt.Printf("Error: '%s' is not a valid device name or MAC address\n", target)
			fmt.Printf("MAC validation error: %v\n", err)
			fmt.Println("Use 'wol-server list-devices' to see available devices.")
			logger.Error("Invalid target %s: %v", target, err)
			os.Exit(1)
		}

		macAddress = target
		deviceName = "Unknown Device"
		logger.Info("Waking device by MAC: %s", macAddress)
	}

	// Send the Wake-on-LAN packet
	fmt.Printf("Sending Wake-on-LAN packet to %s (%s) on port %d...\n", deviceName, macAddress, port)

	err := wol_network.SendWakeOnLAN(macAddress, port)
	if err != nil {
		fmt.Printf("Error: Failed to send Wake-on-LAN packet: %v\n", err)
		os.Exit(1)
	}

	// Update last woken time if it's a known device
	if store.DeviceExists(target) {
		err = store.UpdateLastWoken(target)
		if err != nil {
			logger.Warn("Failed to update last woken time for %s: %v", target, err)
		}
	}

	fmt.Printf("✓ Wake-on-LAN packet sent successfully to %s\n", deviceName)
	logger.Info("Wake-on-LAN completed successfully for %s", deviceName)
}

func setupLogging(logFile, logLevel string, verbose, quiet bool) (*wol_log.Logger, error) {
	var level wol_log.LogLevel

	if verbose {
		level = wol_log.DEBUG
	} else if quiet {
		level = wol_log.ERROR
	} else {
		switch logLevel {
		case "debug":
			level = wol_log.DEBUG
		case "info":
			level = wol_log.INFO
		case "warn", "warning":
			level = wol_log.WARN
		case "error":
			level = wol_log.ERROR
		default:
			return nil, fmt.Errorf("invalid log level: %s (valid: debug, info, warn, error)", logLevel)
		}
	}

	config := wol_log.LoggerConfig{
		Level:        level,
		LogToConsole: true,
		LogToFile:    logFile != "",
		LogFilePath:  logFile,
	}

	logger, err := wol_log.NewLogger(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	return logger, nil
}

func showHelp() {
	fmt.Println("Wake-on-LAN Server")
	fmt.Println("==================")
	fmt.Println()
	fmt.Println("Send Wake-on-LAN magic packets to wake up sleeping computers on your network.")
	fmt.Println()
	showUsage()
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -port int")
	fmt.Printf("        UDP port to send Wake-on-LAN packet (default: %d)\n", wol_network.DefaultWoLPort)
	fmt.Println("  -help")
	fmt.Println("        Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  wol-server AA:BB:CC:DD:EE:FF")
	fmt.Println("  wol-server -port 7 AA-BB-CC-DD-EE-FF")
	fmt.Println("  wol-server AABBCCDDEEFF")
	fmt.Println()
	fmt.Println("Supported MAC address formats:")
	fmt.Println("  - Colon separated: AA:BB:CC:DD:EE:FF")
	fmt.Println("  - Hyphen separated: AA-BB-CC-DD-EE-FF")
	fmt.Println("  - No separators: AABBCCDDEEFF")
	fmt.Println("  - Case insensitive")
}

func showUsage() {
	fmt.Println("Usage:")
	fmt.Println("  wol-server [options] <MAC-address>")
}
