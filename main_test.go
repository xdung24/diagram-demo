package main

import "testing"

func TestResolvePort(t *testing.T) {
	tests := []struct {
		name     string
		argPort  string
		envPort  string
		wantPort string
	}{
		{name: "argument overrides env", argPort: "9090", envPort: "8080", wantPort: "9090"},
		{name: "env used when no argument", argPort: "", envPort: "9090", wantPort: "9090"},
		{name: "default used when nothing provided", argPort: "", envPort: "", wantPort: "8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolvePort(tt.argPort, tt.envPort); got != tt.wantPort {
				t.Fatalf("resolvePort(%q, %q) = %q, want %q", tt.argPort, tt.envPort, got, tt.wantPort)
			}
		})
	}
}
