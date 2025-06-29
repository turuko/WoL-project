package wol_network

import (
	"fmt"
	"net"
	"time"
	wol_log "wol-server/wol/log"
	wol_packet "wol-server/wol/packet"
)

const (
	DefaultWoLPort = 9

	AlternativeWoLPort = 7
)

type Logger = wol_log.Logger

var globalLogger *Logger

func SetLogger(logger *Logger) {
	globalLogger = logger
}

func getLogger() *Logger {
	if globalLogger == nil {
		config := wol_log.LoggerConfig{
			Level:        wol_log.ERROR + 1,
			LogToConsole: false,
			LogToFile:    false,
		}
		logger, _ := wol_log.NewLogger(config)
		return logger
	}
	return globalLogger
}

func SendWakePacket(packet []byte, port int) error {
	logger := getLogger()

	if len(packet) != 102 {
		err := fmt.Errorf("invalid packet length: expected 102 bytes, got %d", len(packet))
		logger.Error("Packet validation failed: %v", err)
		return err
	}

	logger.Debug("Validated magic packet: %d bytes", len(packet))

	broadcastAddr := fmt.Sprintf("255.255.255.255:%d", port)
	logger.Debug("Target broadcast address: %s", broadcastAddr)

	addr, err := net.ResolveUDPAddr("udp", broadcastAddr)
	if err != nil {
		logger.Error("Failed to resolve UDP address %s: %v", broadcastAddr, err)
		return fmt.Errorf("failed to resolve UDP address %s: %w", broadcastAddr, err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		logger.Error("Failed to create UDP connection: %v", err)
		return fmt.Errorf("failed to create UDP connection: %w", err)
	}

	defer conn.Close()

	logger.Debug("UDP connection established")

	err = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		logger.Warn("Failed to set write deadline: %v", err)
		return fmt.Errorf("failed to set write deadline: %v", err)
	}

	logger.Debug("Sending magic packet...")
	bytesWritten, err := conn.Write(packet)
	if err != nil {
		logger.Error("Failed to send magic packet: %v", err)
		return fmt.Errorf("failed to send magic packet: %w", err)
	}

	if bytesWritten != len(packet) {
		err := fmt.Errorf("incomplete packet sent: sent %d bytes, expected %d", bytesWritten, len(packet))
		logger.Error("Packet transmission incomplete: %v", err)
		return err
	}

	logger.Debug("Magic packet sent successfully: %d bytes", bytesWritten)
	return nil
}

func SendWakeOnLAN(mac string, port int) error {
	logger := getLogger()

	logger.Info("Initiating Wake-on-LAN for MAC=%s on port=%d", mac, port)

	packet, err := wol_packet.BuildMagicPacket(mac)
	if err != nil {
		logger.LogWakeAttempt(mac, port, false, err)
		return fmt.Errorf("failed to build magic packet: %w", err)
	}

	logger.LogPacketDetails(mac, len(packet), port)

	err = SendWakePacket(packet, port)
	if err != nil {
		logger.LogWakeAttempt(mac, port, false, err)
		return fmt.Errorf("failed to send wake packet: %w", err)
	}

	logger.LogWakeAttempt(mac, port, true, nil)
	return nil
}

func SendWakeOnLANDefault(mac string) error {
	return SendWakeOnLAN(mac, DefaultWoLPort)
}
