package config

import (
	"testing"
	"time"
)

func TestParseRelativeDuration(t *testing.T) {
	tests := map[string]struct {
		raw     string
		want    time.Duration
		wantErr bool
	}{
		"empty disables":       {raw: "", want: 0},
		"minutes":              {raw: "10m", want: 10 * time.Minute},
		"hours":                {raw: "2h", want: 2 * time.Hour},
		"days":                 {raw: "3d", want: 72 * time.Hour},
		"spaces and uppercase": {raw: " 24H ", want: 24 * time.Hour},
		"invalid unit":         {raw: "12w", wantErr: true},
		"bare zero":            {raw: "0", wantErr: true},
		"tmux off spelling":    {raw: "off", wantErr: true},
		"tmux on spelling":     {raw: "on", wantErr: true},
		"missing amount":       {raw: "h", wantErr: true},
		"negative amount":      {raw: "-1h", wantErr: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := ParseRelativeDuration(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseRelativeDuration(%q) error = nil, want error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRelativeDuration(%q) error = %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("ParseRelativeDuration(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}
