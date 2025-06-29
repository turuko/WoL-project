package main

import (
	"flag"
	"fmt"
	"os"
	wol_log "wol-server/wol/log"
	wol_network "wol-server/wol/network"
	wol_packet "wol-server/wol/packet"
)

func main() {
	var (
		port     = flag.Int("port", wol_network.DefaultWoLPort, "UDP port to send Wake-on-LAN packet (default: 9)")
		help     = flag.Bool("help", false, "Show help message")
		logFile  = flag.String("log", "", "Log file path (default: console only)")
		logLevel = flag.String("level", "info", "Log level: debug, info, warn, error")
		verbose  = flag.Bool("verbose", false, "Enable verbose output (same as -level debug)")
		quiet    = flag.Bool("quiet", false, "Quiet mode - only errors (same as -level error)")
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

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Error: MAC address is required")
		fmt.Println()
		showUsage()
		os.Exit(1)
	}

	mac := args[0]

	logger.Info("WoL Server starting - MAC=%s, port=%d", mac, *port)

	if err := wol_packet.ValidateMAC(mac); err != nil {
		fmt.Printf("Error: %v\n", err)
		logger.Error("MAC validation failed: %v", err)
		os.Exit(1)
	}

	fmt.Printf("Sending Wake-on-LAN packet to %s on port %d...\n", mac, *port)

	err = wol_network.SendWakeOnLAN(mac, *port)
	if err != nil {
		fmt.Printf("Error: Failed to send Wake-on-LAN packet: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Wake-on-LAN packet sent successfully to %s\n", mac)
	logger.Info("WoL Server completed successfully")
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
