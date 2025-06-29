package wol_packet

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

func CleanMAC(mac string) string {
	return strings.ToUpper(
		strings.ReplaceAll(
			strings.ReplaceAll(mac, ":", ""),
			"-", ""),
	)
}

func ValidateMAC(mac string) error {
	cleanMAC := CleanMAC(mac)

	if len(cleanMAC) != 12 {
		return fmt.Errorf("MAC address must be 12 hex characters, got %d", len(cleanMAC))
	}

	hexPattern := regexp.MustCompile("^[0-9A-F]+$")
	if !hexPattern.MatchString(cleanMAC) {
		return fmt.Errorf("MAC address contains invalid characters: %s", mac)
	}

	return nil
}

func BuildMagicPacket(mac string) ([]byte, error) {

	if err := ValidateMAC(mac); err != nil {
		return nil, err
	}

	cleanMAC := CleanMAC(mac)

	macBytes, err := hex.DecodeString(cleanMAC)
	if err != nil {
		return nil, fmt.Errorf("failed to decode MAC address: %w", err)
	}

	if len(macBytes) != 6 {
		return nil, fmt.Errorf("MAC address must be exactly 6 bytes, got %d", len(macBytes))
	}

	packet := make([]byte, 102)

	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}

	for i := 0; i < 16; i++ {
		copy(packet[6+i*6:6+(i+1)*6], macBytes)
	}

	return packet, nil
}
