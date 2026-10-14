package wrapper

import (
	"strings"
	"testing"
)

func TestResolveTrace(t *testing.T) {
	tests := []struct {
		name      string
		cfgFile   string
		cfgLevel  int
		envFile   string
		envLevel  string
		wantFile  string
		wantLevel int
		wantWarn  string
	}{
		{
			name: "nothing anywhere",
		},
		{
			name: "config only", cfgFile: "c.log", cfgLevel: 2,
			wantFile: "c.log", wantLevel: 2,
		},
		{
			// The case this exists for: a wrapper installed by wrap has no
			// config file at all.
			name: "environment only, defaulting to level 1", envFile: "e.log",
			wantFile: "e.log", wantLevel: 1,
		},
		{
			name: "environment file and level", envFile: "e.log", envLevel: "3",
			wantFile: "e.log", wantLevel: 3,
		},
		{
			// The environment wins: it is there to observe a wrapper without
			// editing the installation being observed.
			name: "environment overrides the config", cfgFile: "c.log", cfgLevel: 1,
			envFile: "e.log", envLevel: "3",
			wantFile: "e.log", wantLevel: 3,
		},
		{
			name:    "environment raises the level of a configured trace",
			cfgFile: "c.log", cfgLevel: 1, envLevel: "2",
			wantFile: "c.log", wantLevel: 2,
		},
		{
			name: "a level with nowhere to write warns and traces nothing", envLevel: "2",
			wantWarn: "nowhere",
		},
		{
			name:    "an unreadable level warns and leaves the config alone",
			cfgFile: "c.log", cfgLevel: 2, envLevel: "banana",
			wantFile: "c.log", wantLevel: 2, wantWarn: "banana",
		},
		{
			name: "a level out of range warns", cfgFile: "c.log", cfgLevel: 1, envLevel: "9",
			wantFile: "c.log", wantLevel: 1, wantWarn: "9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, level, warn := resolveTrace(tt.cfgFile, tt.cfgLevel, tt.envFile, tt.envLevel)
			if file != tt.wantFile {
				t.Errorf("file = %q, want %q", file, tt.wantFile)
			}
			if level != tt.wantLevel {
				t.Errorf("level = %d, want %d", level, tt.wantLevel)
			}
			switch {
			case tt.wantWarn == "" && warn != "":
				t.Errorf("warn = %q, want none", warn)
			case tt.wantWarn != "" && !strings.Contains(warn, tt.wantWarn):
				t.Errorf("warn = %q, want it to mention %q", warn, tt.wantWarn)
			}
		})
	}
}
