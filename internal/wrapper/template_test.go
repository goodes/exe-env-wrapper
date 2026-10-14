package wrapper

import (
	"reflect"
	"strings"
	"testing"
)

// The template duplicates what help config explains, which is exactly how the
// option flags went undocumented. This walks the keys off the parser rather
// than listing them, so adding a sixth key cannot quietly skip the template.
func TestConfigTemplateMentionsEveryKeyTheParserKnows(t *testing.T) {
	template := configTemplate("yt-dlp", "")

	rt := reflect.TypeOf(rawConfig{})
	for i := 0; i < rt.NumField(); i++ {
		key := strings.Split(rt.Field(i).Tag.Get("yaml"), ",")[0]
		if key == "" || key == "-" {
			t.Fatalf("field %s has no yaml key; the walk would miss it", rt.Field(i).Name)
		}
		if !strings.Contains(template, key+":") {
			t.Errorf("the template never mentions %q:\n%s", key, template)
		}
	}
}

// A template that does not parse would be worse than no template, and all
// those ${...} inside comments are the kind of thing that bites.
func TestConfigTemplateRoundTrips(t *testing.T) {
	template := configTemplate("yt-dlp", "")

	cfg, err := ParseConfig([]byte(template))
	if err != nil {
		t.Fatalf("the generated template does not parse: %v\n%s", err, template)
	}

	// Only the env block is live: everything else is commented, so the wrapper
	// behaves exactly as it would with no config file at all.
	if cfg.Target != "" {
		t.Errorf("Target = %q, want it commented out", cfg.Target)
	}
	if cfg.Log != "" {
		t.Errorf("Log = %q, want it commented out", cfg.Log)
	}
	if cfg.Trace != "" || cfg.TraceLevel != TraceOff {
		t.Errorf("Trace = %q level %d, want both commented out", cfg.Trace, cfg.TraceLevel)
	}

	want := []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}
	if len(cfg.Env) != len(want) {
		t.Fatalf("env = %v, want the three proxy variables", cfg.Env)
	}
	for i, name := range want {
		if cfg.Env[i].Name != name {
			t.Errorf("env[%d] = %q, want %q", i, cfg.Env[i].Name, name)
		}
		if !strings.HasPrefix(cfg.Env[i].Value, "${system.") {
			t.Errorf("env[%d] value = %q, want a ${system.*} name", i, cfg.Env[i].Value)
		}
	}
}

// Where there is a target to pin, it is live rather than commented, or the
// wrapper would resolve from PATH and ignore the file it was given.
func TestConfigTemplateWithAPinnedTarget(t *testing.T) {
	template := configTemplate("yt-dlp", "C:\\tools\\yt-dlp.exe")

	cfg, err := ParseConfig([]byte(template))
	if err != nil {
		t.Fatalf("the generated template does not parse: %v\n%s", err, template)
	}
	if want := "C:\\tools\\yt-dlp.exe"; cfg.Target != want {
		t.Errorf("Target = %q, want %q live", cfg.Target, want)
	}
	// The commented explanation of target still has to be there.
	if !strings.Contains(template, "# target:") && !strings.Contains(template, "#   What to run") {
		t.Errorf("the template lost its explanation of target:\n%s", template)
	}
}

// The template is for someone opening the file to work out what to change, so
// it has to name the things that are not guessable.
func TestConfigTemplateExplainsItself(t *testing.T) {
	template := configTemplate("yt-dlp", "")

	for _, want := range []string{
		"help config",        // where the full reference is
		"yt-dlp",             // which wrapper this belongs to
		"${system.no_proxy}", // the value that matters
		"${VAR}",             // the other expansion form
		"next match",         // what an absent target means
		"before and after",   // what trace level 3 gives
	} {
		if !strings.Contains(template, want) {
			t.Errorf("the template does not mention %q:\n%s", want, template)
		}
	}
}

// Every live line has to be a key the parser accepts, since an unknown key is
// an error. A typo in the template would break every config it generates.
func TestConfigTemplateHasNoStrayLiveKeys(t *testing.T) {
	for _, target := range []string{"", "C:\\tools\\yt-dlp.exe"} {
		if _, err := ParseConfig([]byte(configTemplate("yt-dlp", target))); err != nil {
			t.Errorf("template with target %q does not parse: %v", target, err)
		}
	}
}
