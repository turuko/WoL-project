// main.go
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	wol_device "wol-server/wol/device"
	wol_log "wol-server/wol/log"
	wol_network "wol-server/wol/network"
	wol_packet "wol-server/wol/packet"
	wol_server "wol-server/wol/server"
)

func main() {
	var (
		port          = flag.Int("port", wol_network.DefaultWoLPort, "UDP port to send Wake-on-LAN packet (default: 9)")
		help          = flag.Bool("help", false, "Show help message")
		logFile       = flag.String("log", "", "Log file path (default: console only)")
		logLevel      = flag.String("level", "info", "Log level: debug, info, warn, error")
		verbose       = flag.Bool("verbose", false, "Enable verbose output (same as -level debug)")
		quiet         = flag.Bool("quiet", false, "Quiet mode - only errors (same as -level error)")
		configPath    = flag.String("config", "", "Device configuration file path (default: system config directory)")
		serverMode    = flag.Bool("server", false, "Run in server mode")
		serverPort    = flag.Int("server-port", 8080, "Server port (default: 8080)")
		serverHost    = flag.String("server-host", "0.0.0.0", "Server host (default: 0.0.0.0)")
		enableCORS    = flag.Bool("cors", true, "Enable CORS headers (default: true)")
		verify        = flag.Bool("verify", false, "Enable packet verification")
		verifyCapture = flag.Bool("verify-capture", false, "Enable packet capture verification")
		verifyPing    = flag.Bool("verify-ping", false, "Enable ping verification after wake")
		netInfo       = flag.Bool("net-info", false, "Show network information and exit")
	)

	flag.Parse()

	if *netInfo {
		logger, err := setupLogging(*logFile, *logLevel, *verbose, *quiet)
		if err != nil {
			fmt.Printf("Error setting up logging: %v\n", err)
			os.Exit(1)
		}
		defer logger.Close()

		wol_network.SetLogger(logger)
		handleNetworkInfo(logger)
		return
	}

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
		fmt.Printf("Error setting up device store: %v\n", err)
		logger.Error("Failed to initialize device store: %v", err)
		os.Exit(1)
	}

	if *serverMode {
		runServer(deviceStore, logger, *serverHost, *serverPort, *enableCORS)
		return
	}

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Error: Command or MAC address is required")
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
		handleWake(args[1], *port, deviceStore, logger, *verify, *verifyCapture, *verifyPing)
	case "verify-network", "net-info":
		handleNetworkInfo(logger)
	case "test-broadcast":
		if len(args) < 2 {
			fmt.Println("Usage: wol-server test-broadcast <MAC-address>")
			os.Exit(1)
		}
		handleTestBroadcast(args[1], *port, logger)
	default:
		// Assume it's a device name or MAC address for wake-up
		handleWake(command, *port, deviceStore, logger, *verify, *verifyCapture, *verifyPing)
	}
}

func handleNetworkInfo(logger *wol_log.Logger) {
	fmt.Println("Network Information")
	fmt.Println("==================")

	netInfo, err := wol_network.VerifyNetworkConnectivity()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		logger.Error("Network verification failed: %v", err)
		os.Exit(1)
	}

	fmt.Printf("Interface:    %s\n", netInfo.InterfaceName)
	fmt.Printf("Local IP:     %s\n", netInfo.LocalIP)
	fmt.Printf("Broadcast IP: %s\n", netInfo.BroadcastIP)
	fmt.Printf("MAC Address:  %s\n", netInfo.MACAddress)
	fmt.Println()
	fmt.Println("✓ Network connectivity verified")
	fmt.Println("✓ UDP broadcast capability confirmed")

	logger.Info("Network information displayed successfully")
}

func handleTestBroadcast(mac string, port int, logger *wol_log.Logger) {
	fmt.Printf("Testing broadcast to %s on port %d...\n", mac, port)

	config := wol_network.VerificationConfig{
		EnableCapture:  true,
		CaptureTimeout: 5 * time.Second,
		EnablePing:     false,
	}

	result, err := wol_network.SendWakeOnLANWithVerification(mac, port, config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nVerification Results:")
	fmt.Println("====================")
	fmt.Printf("Packet Sent:      %v\n", result.PacketSent)
	fmt.Printf("Broadcast Sent:   %v\n", result.BroadcastSent)
	fmt.Printf("Packet Captured:  %v\n", result.PacketCaptured)
	fmt.Printf("Capture Details:  %s\n", result.CaptureDetails)

	if result.NetworkInfo.LocalIP != "" {
		fmt.Printf("Local IP:         %s\n", result.NetworkInfo.LocalIP)
		fmt.Printf("Broadcast IP:     %s\n", result.NetworkInfo.BroadcastIP)
		fmt.Printf("Interface:        %s\n", result.NetworkInfo.InterfaceName)
	}

	if result.PacketSent && result.PacketCaptured {
		fmt.Println("\n✓ Wake-on-LAN packet successfully sent and verified on network")
	} else if result.PacketSent {
		fmt.Println("\n⚠ Wake-on-LAN packet sent but not verified on network")
		fmt.Println("  This could be normal depending on network configuration")
	} else {
		fmt.Println("\n✗ Failed to send Wake-on-LAN packet")
	}
}

func handleWake(target string, port int, store *wol_device.DeviceStore, logger *wol_log.Logger, verify, verifyCapture, verifyPing bool) {
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

	// Send the Wake-on-LAN packet with or without verification
	fmt.Printf("Sending Wake-on-LAN packet to %s (%s) on port %d...\n", deviceName, macAddress, port)

	if verify || verifyCapture || verifyPing {
		config := wol_network.VerificationConfig{
			EnableCapture:  verifyCapture,
			CaptureTimeout: 3 * time.Second,
			EnablePing:     verifyPing,
			PingTimeout:    2 * time.Second,
		}

		result, err := wol_network.SendWakeOnLANWithVerification(macAddress, port, config)
		if err != nil {
			fmt.Printf("Error: Failed to send Wake-on-LAN packet: %v\n", err)
			os.Exit(1)
		}

		// Show verification results
		if verifyCapture {
			if result.PacketCaptured {
				fmt.Println("✓ Packet verified on network")
			} else {
				fmt.Println("⚠ Packet not detected on network")
			}
		}

		if verifyPing && result.TargetReachable {
			fmt.Println("✓ Target appears reachable")
		}

	} else {
		err := wol_network.SendWakeOnLAN(macAddress, port)
		if err != nil {
			fmt.Printf("Error: Failed to send Wake-on-LAN packet: %v\n", err)
			os.Exit(1)
		}
	}

	// Update last woken time if it's a known device
	if store.DeviceExists(target) {
		err := store.UpdateLastWoken(target)
		if err != nil {
			logger.Warn("Failed to update last woken time for %s: %v", target, err)
		}
	}

	fmt.Printf("✓ Wake-on-LAN packet sent successfully to %s\n", deviceName)
	logger.Info("Wake-on-LAN completed successfully for %s", deviceName)
}

func runServer(deviceStore *wol_device.DeviceStore, logger *wol_log.Logger, host string, port int, cors bool) {
	wol_network.SetLogger(logger)

	config := wol_server.ServerConfig{
		Port:        port,
		Host:        host,
		DeviceStore: deviceStore,
		Logger:      logger,
		EnableCORS:  cors,
	}

	server := wol_server.NewWoLServer(config)

	logger.Info("WoL Server starting in HTTP server mode on %s:%d", host, port)

	err := server.Start()
	if err != nil && err != http.ErrServerClosed {
		logger.Error("Server failed: %v", err)
		os.Exit(1)
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
		logger.Error("Failed to add device %s: %v", name, err)
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
	fmt.Println("Wake-on-LAN Server with Device Management")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("Send Wake-on-LAN magic packets to wake up sleeping computers on your network.")
	fmt.Println("Manage devices with friendly names for easy access.")
	fmt.Println()
	showUsage()
	fmt.Println()
	fmt.Println("Device Management Commands:")
	fmt.Println("  add-device <name> <mac> [desc] [ip] [port]")
	fmt.Println("        Add a new device to the configuration")
	fmt.Println("  list-devices")
	fmt.Println("        List all configured devices")
	fmt.Println("  remove-device <name>")
	fmt.Println("        Remove a device from the configuration")
	fmt.Println("  show-device <name>")
	fmt.Println("        Show detailed information about a device")
	fmt.Println()
	fmt.Println("Wake Commands:")
	fmt.Println("  wake <name-or-mac>")
	fmt.Println("        Wake a device by name or MAC address")
	fmt.Println("  <name-or-mac>")
	fmt.Println("        Wake a device (shorthand)")
	fmt.Println()
	fmt.Println("Verification Options:")
	fmt.Println("  -verify")
	fmt.Println("        Enable basic packet verification")
	fmt.Println("  -verify-capture")
	fmt.Println("        Enable packet capture verification")
	fmt.Println("  -verify-ping")
	fmt.Println("        Enable ping verification after wake")
	fmt.Println()
	fmt.Println("Network Commands:")
	fmt.Println("  verify-network")
	fmt.Println("        Show network information and test connectivity")
	fmt.Println("  test-broadcast <mac>")
	fmt.Println("        Test broadcast capability with packet verification")
	fmt.Println()
	fmt.Println("Server Mode:")
	fmt.Println("  -server")
	fmt.Println("        Run in HTTP server mode")
	fmt.Println("  -server-port int")
	fmt.Println("        Server port (default: 8080)")
	fmt.Println("  -server-host string")
	fmt.Println("        Server host (default: 0.0.0.0)")
	fmt.Println("  -cors")
	fmt.Println("        Enable CORS headers (default: true)")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -port int")
	fmt.Printf("        UDP port to send Wake-on-LAN packet (default: %d)\n", wol_network.DefaultWoLPort)
	fmt.Println("  -config string")
	fmt.Println("        Device configuration file path")
	fmt.Println("  -log string")
	fmt.Println("        Log file path (default: console only)")
	fmt.Println("  -level string")
	fmt.Println("        Log level: debug, info, warn, error (default: info)")
	fmt.Println("  -verbose")
	fmt.Println("        Enable verbose output (same as -level debug)")
	fmt.Println("  -quiet")
	fmt.Println("        Quiet mode - only errors (same as -level error)")
	fmt.Println("  -help")
	fmt.Println("        Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Device management")
	fmt.Println("  wol-server.exe add-device desktop AA:BB:CC:DD:EE:FF \"My desktop computer\"")
	fmt.Println("  wol-server.exe list-devices")
	fmt.Println("  wol-server.exe show-device desktop")
	fmt.Println("  wol-server.exe remove-device desktop")
	fmt.Println()
	fmt.Println("  # Wake devices")
	fmt.Println("  wol-server.exe wake desktop")
	fmt.Println("  wol-server.exe desktop")
	fmt.Println("  wol-server.exe AA:BB:CC:DD:EE:FF")
	fmt.Println("  wol-server.exe -port 7 laptop")
	fmt.Println()
	fmt.Println("  # Network verification")
	fmt.Println("  wol-server.exe verify-network")
	fmt.Println("  wol-server.exe test-broadcast AA:BB:CC:DD:EE:FF")
	fmt.Println("  wol-server.exe -verify-capture desktop")
	fmt.Println()
	fmt.Println("  # Server mode")
	fmt.Println("  wol-server.exe -server")
	fmt.Println("  wol-server.exe -server -server-port 8080 -log server.log")
	fmt.Println()
	fmt.Println("Supported MAC address formats:")
	fmt.Println("  - Colon separated: AA:BB:CC:DD:EE:FF")
	fmt.Println("  - Hyphen separated: AA-BB-CC-DD-EE-FF")
	fmt.Println("  - No separators: AABBCCDDEEFF")
	fmt.Println("  - Case insensitive")
}

func showUsage() {
	fmt.Println("Usage:")
	fmt.Println("  wol-server.exe [options] <command> [arguments]")
	fmt.Println("  wol-server.exe [options] <device-name-or-mac>")
	fmt.Println("  wol-server.exe -server [server-options]")
}
