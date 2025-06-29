package wol_network

import (
	"net"
	"testing"
)

func TestSendPacket(t *testing.T) {
	tests := []struct {
		name    string
		packet  []byte
		port    int
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid packet length",
			packet:  make([]byte, 102),
			port:    9,
			wantErr: false,
		},
		{
			name:    "invalid packet too short",
			packet:  make([]byte, 50),
			port:    9,
			wantErr: true,
			errMsg:  "invalid packet length",
		},
		{
			name:    "invalid packet too long",
			packet:  make([]byte, 150),
			port:    9,
			wantErr: true,
			errMsg:  "invalid packet length",
		},
		{
			name:    "empty packet",
			packet:  []byte{},
			port:    9,
			wantErr: true,
			errMsg:  "invalid packet length",
		},
		{
			name:    "valid packet with alternative port",
			packet:  make([]byte, 102),
			port:    7,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SendWakePacket(tt.packet, tt.port)

			if tt.wantErr {
				if err == nil {
					t.Errorf("SendWakePacket() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("SendWakePacket() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					if !isNetworkError(err) {
						t.Errorf("SendWakePacket() unexpected erro = %v", err)
					}
				}
			}
		})
	}
}

func TestSendOnWakeLAN(t *testing.T) {
	tests := []struct {
		name    string
		mac     string
		port    int
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid MAC address",
			mac:     "AA:BB:CC:DD:EE:FF",
			port:    9,
			wantErr: false,
		},
		{
			name:    "invalid MAC address",
			mac:     "invalid-mac",
			port:    9,
			wantErr: true,
			errMsg:  "failed to build magic packet",
		},
		{
			name:    "empty MAC address",
			mac:     "",
			port:    9,
			wantErr: true,
			errMsg:  "failed to build magic packet",
		},
		{
			name:    "valid MAC with alternative port",
			mac:     "00:11:22:33:44:55",
			port:    7,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SendWakeOnLAN(tt.mac, tt.port)

			if tt.wantErr {
				if err == nil {
					t.Errorf("SendWakeOnLAN() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("SendWakeOnLAN() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					if !isNetworkError(err) {
						t.Errorf("SendWakeOnLAN() unexpected error = %v", err)
					}
				}
			}
		})
	}
}

func TestSendWakeOnLANDefault(t *testing.T) {
	tests := []struct {
		name    string
		mac     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid MAC with default port",
			mac:     "AA:BB:CC:DD:EE:FF",
			wantErr: false,
		},
		{
			name:    "invalid MAC with default port",
			mac:     "invalid",
			wantErr: true,
			errMsg:  "failed to build magic packet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SendWakeOnLANDefault(tt.mac)

			if tt.wantErr {
				if err == nil {
					t.Errorf("SendWakeOnLANDefault() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("SendWakeOnLANDefault() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					if !isNetworkError(err) {
						t.Errorf("SendWakeOnLANDefault() unexpected error = %v", err)
					}
				}
			}
		})
	}
}

func TestConstants(t *testing.T) {
	if DefaultWoLPort != 9 {
		t.Errorf("DefaultWolPort = %d, want 9", DefaultWoLPort)
	}

	if AlternativeWoLPort != 7 {
		t.Errorf("AlternativeWoLPort = %d, want 7", AlternativeWoLPort)
	}
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

func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	if netErr, ok := err.(*net.OpError); ok {
		return netErr != nil
	}

	if _, ok := err.(*net.DNSError); ok {
		return true
	}

	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}

	errMsg := err.Error()
	networkKeywords := []string{
		"network is unreachable",
		"permission denied",
		"connection refused",
		"no route to host",
		"timeout",
		"dial",
		"bind",
	}

	for _, keyword := range networkKeywords {
		if contains(errMsg, keyword) {
			return true
		}
	}

	return false
}
