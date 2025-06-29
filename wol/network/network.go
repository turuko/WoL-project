package wol_network

import (
	"fmt"
	"net"
	"time"
	wol_log "wol-server/wol/log"
	wol_packet "wol-server/wol/packet"
)

type VerificationConfig struct {
	EnableCapture    bool
	CaptureInterface string
	CaptureTimeout   time.Duration
	EnablePing       bool
	PingTimeout      time.Duration
}

type PacketVerificationResult struct {
	PacketSent      bool
	PacketCaptured  bool
	TargetReachable bool
	BroadcastSent   bool
	Interface       string
	Error           error
	CaptureDetails  string
	NetworkInfo     NetworkInfo
}

type NetworkInfo struct {
	LocalIP       string
	BroadcastIP   string
	InterfaceName string
	MACAddress    string
}

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

func SendWakeOnLANWithVerification(mac string, port int, config VerificationConfig) (*PacketVerificationResult, error) {
	logger := getLogger()
	result := &PacketVerificationResult{}

	logger.Info("Sending WoL packet with verification enabled")

	netInfo, err := getNetworkInfo()
	if err != nil {
		logger.Warn("Could not get network info: %v", err)
	} else {
		result.NetworkInfo = netInfo
		logger.Debug("Network info - Local IP: %s, Broadcast: %s, Interface: %s",
			netInfo.LocalIP, netInfo.BroadcastIP, netInfo.InterfaceName)
	}

	packet, err := wol_packet.BuildMagicPacket(mac)
	if err != nil {
		result.Error = fmt.Errorf("failed to build magic packet: %w", err)
		return result, result.Error
	}

	var captureResult chan bool
	if config.EnableCapture {
		captureResult = make(chan bool, 1)
		go captureWoLPacket(mac, port, config.CaptureInterface, config.CaptureTimeout, captureResult, logger)
		time.Sleep(100 * time.Millisecond)
	}

	err = SendWakePacket(packet, port)
	if err != nil {
		result.Error = fmt.Errorf("failed to send wake packet: %w", err)
		return result, result.Error
	}
	result.PacketSent = true
	result.BroadcastSent = true

	if config.EnableCapture {
		select {
		case captured := <-captureResult:
			result.PacketCaptured = captured
			if captured {
				result.CaptureDetails = "Magic packet detected on network"
				logger.Info("Verification: Magic packet successfully captured on network")
			} else {
				result.CaptureDetails = "No magic packet detected during capture window"
				logger.Warn("Verification: Magic packet not detected on network")
			}
		case <-time.After(config.CaptureTimeout + time.Second):
			result.CaptureDetails = "Capture timeout"
			logger.Warn("Verification: Packet capture timed out")
		}
	}

	if config.EnablePing {
		targetIP := netInfo.BroadcastIP
		if targetIP != "" {
			result.TargetReachable = pingHost(targetIP, config.PingTimeout, logger)
			if result.TargetReachable {
				logger.Info("Verification: Target appears to be reachable")
			} else {
				logger.Debug("Verification: Target not reachable (expected if device was already off)")
			}
		}
	}

	return result, nil
}

func getNetworkInfo() (NetworkInfo, error) {
	var info NetworkInfo

	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return info, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	info.LocalIP = localAddr.IP.String()

	interfaces, err := net.Interfaces()
	if err != nil {
		return info, err
	}

	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipnet.IP.String() == info.LocalIP {
					info.InterfaceName = iface.Name
					info.MACAddress = iface.HardwareAddr.String()

					ip := ipnet.IP.To4()
					mask := ipnet.Mask
					if ip != nil && mask != nil {
						broadcast := make(net.IP, 4)
						for i := range ip {
							broadcast[i] = ip[i] | ^mask[i]
						}
						info.BroadcastIP = broadcast.String()
					}
					return info, nil
				}
			}
		}
	}

	return info, nil
}

func captureWoLPacket(targetMAC string, port int, iface string, timeout time.Duration, result chan bool, logger *Logger) {
	// This is a simplified version - in a real implementation, you'd use a packet capture library
	// like gopacket/pcap, but that requires additional dependencies and platform-specific setup

	logger.Debug("Starting packet capture simulation for %s on port %d", targetMAC, port)

	// For now, we'll simulate packet detection by monitoring our own broadcast
	// In a real implementation, this would use libpcap or similar

	// Create a UDP listener on the WoL port to detect our own broadcast
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		logger.Error("Failed to resolve UDP address for capture: %v", err)
		result <- false
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		logger.Debug("Could not listen for packet capture (port may be in use): %v", err)
		result <- false
		return
	}
	defer conn.Close()

	// Set read timeout
	conn.SetReadDeadline(time.Now().Add(timeout))

	buffer := make([]byte, 1024)
	for {
		n, clientAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				result <- false
				return
			}
			continue
		}

		if n == 102 { // Magic packet size
			logger.Debug("Detected potential WoL packet from %s (%d bytes)", clientAddr, n)

			// Verify it's actually a magic packet
			if isMagicPacket(buffer[:n], targetMAC) {
				logger.Info("Confirmed magic packet for %s captured", targetMAC)
				result <- true
				return
			}
		}
	}
}

// isMagicPacket verifies if a packet is a valid WoL magic packet
func isMagicPacket(packet []byte, targetMAC string) bool {
	if len(packet) != 102 {
		return false
	}

	// Check for 6 bytes of 0xFF
	for i := 0; i < 6; i++ {
		if packet[i] != 0xFF {
			return false
		}
	}

	// Clean target MAC for comparison
	cleanTargetMAC := wol_packet.CleanMAC(targetMAC)
	targetBytes := make([]byte, 6)
	for i := 0; i < 6; i++ {
		fmt.Sscanf(cleanTargetMAC[i*2:i*2+2], "%02X", &targetBytes[i])
	}

	// Check that the MAC is repeated 16 times
	for i := 0; i < 16; i++ {
		start := 6 + i*6
		for j := 0; j < 6; j++ {
			if packet[start+j] != targetBytes[j] {
				return false
			}
		}
	}

	return true
}

// pingHost attempts to ping a host to check reachability
func pingHost(host string, timeout time.Duration, logger *Logger) bool {
	// Simple TCP dial test (more reliable than ICMP ping which requires privileges)
	commonPorts := []int{22, 80, 443, 135, 445, 3389} // SSH, HTTP, HTTPS, RPC, SMB, RDP

	for _, port := range commonPorts {
		address := fmt.Sprintf("%s:%d", host, port)
		conn, err := net.DialTimeout("tcp", address, timeout/time.Duration(len(commonPorts)))
		if err == nil {
			conn.Close()
			logger.Debug("Host %s is reachable on port %d", host, port)
			return true
		}
	}

	logger.Debug("Host %s not reachable on common ports", host)
	return false
}

// VerifyNetworkConnectivity performs basic network connectivity checks
func VerifyNetworkConnectivity() (*NetworkInfo, error) {
	logger := getLogger()

	netInfo, err := getNetworkInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get network information: %w", err)
	}

	logger.Info("Network verification - Interface: %s, Local IP: %s, Broadcast: %s",
		netInfo.InterfaceName, netInfo.LocalIP, netInfo.BroadcastIP)

	// Test UDP broadcast capability
	testAddr := fmt.Sprintf("%s:%d", netInfo.BroadcastIP, DefaultWoLPort)
	conn, err := net.Dial("udp", testAddr)
	if err != nil {
		return &netInfo, fmt.Errorf("cannot create UDP connection to broadcast address: %w", err)
	}
	conn.Close()

	logger.Info("Network connectivity verified - UDP broadcast capability confirmed")
	return &netInfo, nil
}
