package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// EnvVarName is the environment variable selecting the active environment
// when Load is called without an explicit one.
const EnvVarName = "GOYII_ENV"

// Load loads configuration for the environment named by the GOYII_ENV
// environment variable (default "dev") from the given directory.
func Load(dir string) (Config, error) {
	return LoadEnv(dir, CurrentEnv())
}

// CurrentEnv returns the active environment name: GOYII_ENV when set to a
// non-empty value, otherwise "dev".
func CurrentEnv() string {
	if env := os.Getenv(EnvVarName); env != "" {
		return env
	}
	return "dev"
}

// LoadEnv loads dir/app.yaml, overlays dir/web-<env>.yaml when present
// (fields absent from the overlay keep their base values), then applies
// environment variable overrides and fills defaults. At least one of the two
// files must exist; the resolved environment name is recorded in Config.Env.
func LoadEnv(dir, env string) (Config, error) {
	var cfg Config
	found := false
	for _, name := range []string{"app.yaml", "web-" + env + ".yaml"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return Config{}, fmt.Errorf("config: read %s: %w", name, err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("config: parse %s: %w", name, err)
		}
		found = true
	}
	if !found {
		return Config{}, fmt.Errorf("config: no configuration files found in %s (expected app.yaml and/or web-%s.yaml)", dir, env)
	}

	if err := applyEnvOverrides(&cfg); err != nil {
		return Config{}, err
	}
	// The environment name comes from the selector (GOYII_ENV / argument),
	// never from a config value.
	cfg.Env = env
	return WithDefaults(cfg), nil
}

// applyEnvOverrides walks every exported leaf field of cfg and, when its
// environment variable is set (path upper-cased, joined by '_'), parses the
// value into the field. An empty environment value counts as not set.
func applyEnvOverrides(cfg *Config) error {
	return walkOverrides(reflect.ValueOf(cfg).Elem(), nil)
}

func walkOverrides(v reflect.Value, path []string) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name := yamlKey(field)
		if name == "-" {
			continue
		}
		fv := v.Field(i)
		if fv.Kind() == reflect.Struct && fv.Type() != durationType {
			sub := make([]string, 0, len(path)+1)
			sub = append(sub, path...)
			sub = append(sub, name)
			if err := walkOverrides(fv, sub); err != nil {
				return err
			}
			continue
		}
		key := strings.ToUpper(strings.Join(append(path, name), "_"))
		raw, ok := os.LookupEnv(key)
		if !ok || raw == "" {
			continue
		}
		if err := setFromString(fv, raw, key); err != nil {
			return err
		}
	}
	return nil
}

// yamlKey returns a field's configuration key: the yaml tag name when
// present, otherwise the field name.
func yamlKey(field reflect.StructField) string {
	name := field.Tag.Get("yaml")
	if i := strings.IndexByte(name, ','); i >= 0 {
		name = name[:i]
	}
	if name == "" {
		name = field.Name
	}
	return name
}

var durationType = reflect.TypeOf(time.Duration(0))

// setFromString parses raw into the field fv; key names the environment
// variable for error messages.
func setFromString(fv reflect.Value, raw, key string) error {
	if fv.Type() == durationType {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("config: env %s: invalid duration %q", key, raw)
		}
		fv.SetInt(int64(d))
		return nil
	}
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("config: env %s: invalid bool %q", key, raw)
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, fv.Type().Bits())
		if err != nil {
			return fmt.Errorf("config: env %s: invalid integer %q", key, raw)
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, fv.Type().Bits())
		if err != nil {
			return fmt.Errorf("config: env %s: invalid unsigned integer %q", key, raw)
		}
		fv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw, fv.Type().Bits())
		if err != nil {
			return fmt.Errorf("config: env %s: invalid number %q", key, raw)
		}
		fv.SetFloat(f)
	default:
		return fmt.Errorf("config: env %s: unsupported field kind %s", key, fv.Kind())
	}
	return nil
}
