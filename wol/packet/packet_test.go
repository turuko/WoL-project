package wol_packet

import (
	"bytes"
	"testing"
)

func TestValidateMAC(t *testing.T) {
	tests := []struct {
		name    string
		mac     string
		wantErr bool
	}{
		{"valid colon format", "AA:BB:CC:DD:EE:FF", false},
		{"valid hyphen format", "AA-BB-CC-DD-EE-FF", false},
		{"valid lowercase", "aa:bb:cc:dd:ee:ff", false},
		{"valid mixed case", "Aa:Bb:Cc:Dd:Ee:Ff", false},
		{"valid no separators", "AABBCCDDEEFF", false},
		{"invalid too short", "AA:BB:CC:DD:EE", true},
		{"invalid too long", "AA:BB:CC:DD:EE:FF:00", true},
		{"invalid characters", "GG:BB:CC:DD:EE:FF", true},
		{"empty string", "", true},
		{"invalid format", "AA:BB:CC:DD:EE:FG", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMAC(tt.mac)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMAC() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildMagicPacket(t *testing.T) {
	tests := []struct {
		name    string
		mac     string
		wantErr bool
	}{
		{"valid MAC with colons", "AA:BB:CC:DD:EE:FF", false},
		{"valid MAC with hypens", "AA-BB-CC-DD-EE-FF", false},
		{"valid MAC lowercase", "aa:bb:cc:dd:ee:ff", false},
		{"invalid MAC", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet, err := BuildMagicPacket(tt.mac)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildMagicPacket() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(packet) != 102 {
					t.Errorf("BuildMagicPacket() packet length = %v, want 102", len(packet))
				}

				expectedHeader := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
				if !bytes.Equal(packet[:6], expectedHeader) {
					t.Errorf("BuildMagicPacket() header = %x, want %x", packet[:6], expectedHeader)
				}

				expectedMAC := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
				for i := 0; i < 16; i++ {
					start := 6 + i*6
					end := start + 6
					if !bytes.Equal(packet[start:end], expectedMAC) {
						t.Errorf("BuildMagicPacket() MAC repetition %d = %x, want %x", i, packet[start:end], expectedMAC)
					}
				}
			}
		})
	}
}

func TestBuildMagicPacketSpecificMAC(t *testing.T) {
	mac := "00:11:22:33:44:55"
	packet, err := BuildMagicPacket(mac)
	if err != nil {
		t.Fatalf("BuildMagicPacket() unexpected error = %v", err)
	}

	expectedMAC := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}

	for i := 0; i < 16; i++ {
		start := 6 + i*6
		end := start + 6
		if !bytes.Equal(packet[start:end], expectedMAC) {
			t.Errorf("MAC repetition %d = %x, want %x", i, packet[start:end], expectedMAC)
		}
	}
}
