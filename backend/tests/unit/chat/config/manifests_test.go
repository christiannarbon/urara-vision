// Ported from chat/tests/unit/test_configmap_settings.py and
// test_settings_are_reachable.py: every setting must be settable where chat runs.
package config_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"urara-vision/backend/internal/chat/config"
)

// Relative to this directory; the Makefile mounts both at the same place.
const (
	manifestPath = "../../../../../k8s/base/chat.yaml"
	composePath  = "../../../../../docker-compose.yml"
)

// Reaches the pod through the Deployment's secretKeyRef instead.
var suppliedElsewhere = map[string]bool{"BACKEND_API_TOKEN": true}

// Declared values that differ from the code's default on purpose.
var (
	configMapDiffers = map[string]string{
		"VERTEX_PROJECT":  "no default is possible; each overlay sets its own",
		"LLM_TEMPERATURE": "the code leaves it to the provider",
	}
	composeDiffers = map[string]string{
		"BACKEND_API_TOKEN": "the compose backend expects the committed dev token",
		"LLM_PROVIDER":      "compose still runs the Python service, which defaults to gemini-studio",
		"LLM_TEMPERATURE":   "the code leaves it to the provider",
	}
	// Published from :8090, so it is a committed literal.
	composeFixed = map[string]bool{"APP_ADDR": true}
)

// Not a skip: this is all that stands between a new setting and a silent default.
func read(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return data
}

type manifest struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Data map[string]string `yaml:"data"`
	Spec struct {
		Template struct {
			Spec struct {
				TerminationGracePeriodSeconds float64 `yaml:"terminationGracePeriodSeconds"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

func manifests(t *testing.T) []manifest {
	t.Helper()
	dec := yaml.NewDecoder(bytes.NewReader(read(t, manifestPath)))
	var docs []manifest
	for {
		var m manifest
		err := dec.Decode(&m)
		if errors.Is(err, io.EOF) {
			return docs
		}
		if err != nil {
			t.Fatalf("parsing %s: %v", manifestPath, err)
		}
		docs = append(docs, m)
	}
}

// Matched by kind as well as name: the Deployment's envFrom carries the same name.
func find(t *testing.T, kind, name string) manifest {
	t.Helper()
	for _, m := range manifests(t) {
		if m.Kind == kind && m.Metadata.Name == name {
			return m
		}
	}
	t.Fatalf("no %s named %s in %s", kind, name, manifestPath)
	return manifest{}
}

func configMap(t *testing.T) map[string]string {
	return find(t, "ConfigMap", "relviz-chat-config").Data
}

func composeEnv(t *testing.T) map[string]string {
	t.Helper()
	var f struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(read(t, composePath), &f); err != nil {
		t.Fatalf("parsing %s: %v", composePath, err)
	}
	env := f.Services["chat"].Environment
	if env == nil {
		t.Fatalf("no chat service environment in %s", composePath)
	}
	return env
}

// agrees reports whether a declared value means the same as the code's default.
func agrees(declared, def string) bool {
	d, err1 := strconv.ParseFloat(declared, 64)
	c, err2 := strconv.ParseFloat(def, 64)
	if err1 == nil && err2 == nil {
		return d == c
	}
	return declared == def
}

func TestEverySettingIsInTheConfigMap(t *testing.T) {
	data := configMap(t)
	for _, name := range config.EnvNames() {
		if _, ok := data[name]; !ok && !suppliedElsewhere[name] {
			t.Errorf("%s is absent from relviz-chat-config; add it with the code's default", name)
		}
	}
}

func TestConfigMapDefaultsMatchTheCode(t *testing.T) {
	data := configMap(t)
	for _, name := range config.EnvNames() {
		declared, ok := data[name]
		if !ok || configMapDiffers[name] != "" {
			continue
		}
		if !agrees(declared, config.Default(name)) {
			t.Errorf("%s: ConfigMap says %q, code says %q", name, declared, config.Default(name))
		}
	}
}

// A shorter grace period would kill a turn the service may still be running.
func TestGracePeriodCoversAWholeTurn(t *testing.T) {
	grace := find(t, "Deployment", "chat").Spec.Template.Spec.TerminationGracePeriodSeconds
	timeout, err := strconv.ParseFloat(configMap(t)["ANSWER_TIMEOUT_SECONDS"], 64)
	if err != nil {
		t.Fatalf("ANSWER_TIMEOUT_SECONDS: %v", err)
	}
	if grace < timeout {
		t.Errorf("terminationGracePeriodSeconds %gs is below ANSWER_TIMEOUT_SECONDS %gs", grace, timeout)
	}
}

var composeDefault = regexp.MustCompile(`^\$\{[A-Z0-9_]+:-(.*)\}$`)

func TestEverySettingIsInCompose(t *testing.T) {
	env := composeEnv(t)
	for _, name := range config.EnvNames() {
		value, ok := env[name]
		switch {
		case !ok:
			t.Errorf(`%s is absent from the chat service's environment; add it as "${%s:-default}"`, name, name)
		case !composeFixed[name] && !strings.HasPrefix(value, "${"):
			t.Errorf(`%s is a literal %q, so the host cannot override it; use "${%s:-default}"`, name, value, name)
		}
	}
}

func TestComposeDefaultsMatchTheCode(t *testing.T) {
	env := composeEnv(t)
	for _, name := range config.EnvNames() {
		m := composeDefault.FindStringSubmatch(env[name])
		if m == nil || composeDiffers[name] != "" {
			continue
		}
		if !agrees(m[1], config.Default(name)) {
			t.Errorf("%s: compose says %q, code says %q", name, m[1], config.Default(name))
		}
	}
}
