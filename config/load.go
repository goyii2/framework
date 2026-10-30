// Package config loads a project's own configuration values. It owns no
// configuration fields: LoadAs[T] fills whatever struct the project defines —
// identity and server values, the component packages' parameter bundles,
// fully custom sections — from the same three machines every project gets:
//
//  1. the active environment is selected by the GOYII_ENV environment
//     variable, defaulting to "dev" (see Env);
//  2. <dir>/app.yaml loads first and <dir>/web-<env>.yaml overlays it
//     (fields absent from the overlay keep their base values);
//  3. environment variables override leaf fields: a field's path, upper-cased
//     and joined with underscores, addresses it (DB.DSN -> DB_DSN,
//     App.Debug -> APP_DEBUG, Cache.Redis.Addr -> CACHE_REDIS_ADDR,
//     Email.APIKey -> EMAIL_APIKEY).
//
// File values themselves support compose-style interpolation: before parsing,
// ${VAR} placeholders in both files expand from the process environment —
// ${DB_USER} splices into a DSN, ${VAR:-fallback} supplies a default for unset
// variables, ${VAR:?message} fails the load when a value is mandatory, and a
// variable that is unset without a default expands to the empty string.
// Substitution shapes file values only: whole-field overrides (machine 3)
// still win over whatever the files said.
//
// The loader fills no defaults — zero values stay zero and every component
// package defaults inside its own constructor. A project that prefers a
// different loader (viper, koanf, plain flags) skips this package entirely;
// no framework code depends on it.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/a8m/envsubst"
	"github.com/goccy/go-yaml"
)

// EnvVarName is the environment variable selecting the active environment.
const EnvVarName = "GOYII_ENV"

// Env returns the active environment name: GOYII_ENV when set to a non-empty
// value, otherwise "dev".
func Env() string {
	if env := os.Getenv(EnvVarName); env != "" {
		return env
	}
	return "dev"
}

// LoadAs loads the configuration of type T from dir: app.yaml first, then the
// web-<env>.yaml overlay for the active environment (Env), then environment
// variable overrides. ${VAR} placeholders in file values expand from the
// process environment before parsing (see the package comment for the exact
// forms). At least one of the two files must exist. T must be a struct; every
// other shape is rejected. No defaults are filled — components default in
// their own constructors, so a project's zero values reach them unchanged.
func LoadAs[T any](dir string) (T, error) {
	var cfg T
	v := reflect.ValueOf(&cfg).Elem()
	if v.Kind() != reflect.Struct {
		return cfg, fmt.Errorf("config: LoadAs requires a struct type, got %s", v.Kind())
	}
	found := false
	for _, name := range []string{"app.yaml", "web-" + Env() + ".yaml"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return cfg, fmt.Errorf("config: read %s: %w", name, err)
		}
		data, err = expandEnv(data, name)
		if err != nil {
			return cfg, err
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("config: parse %s: %w", name, err)
		}
		found = true
	}
	if !found {
		return cfg, fmt.Errorf("config: no configuration files found in %s (expected app.yaml and/or web-%s.yaml)", dir, Env())
	}
	if err := walkOverrides(v, nil); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// requiredVar matches ${VAR:?message} / ${VAR?message} placeholders, whose
// POSIX meaning is "unset (or empty) -> fail". The underlying expander does
// not enforce these forms on its own — it substitutes the message text — so
// the loader checks the named variables up front and fails the load instead.
var requiredVar = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*):?\?([^}]*)\}`)

// expandEnv substitutes ${VAR} placeholders in raw file bytes with process
// environment values, compose-style: a variable that is unset and carries no
// default expands to the empty string, ${VAR:-fallback} supplies the default,
// ${VAR:?message} fails the load when the variable is unset or empty. name is
// the file being expanded, for error context.
func expandEnv(raw []byte, name string) ([]byte, error) {
	for _, m := range requiredVar.FindAllStringSubmatch(string(raw), -1) {
		if v, ok := os.LookupEnv(m[1]); !ok || v == "" {
			detail := strings.TrimSpace(m[2])
			if detail != "" {
				detail = ": " + detail
			}
			return nil, fmt.Errorf("config: expand %s: required variable %s is not set%s", name, m[1], detail)
		}
	}
	expanded, err := envsubst.Bytes(raw)
	if err != nil {
		return nil, fmt.Errorf("config: expand %s: %w", name, err)
	}
	return expanded, nil
}

// walkOverrides visits every exported leaf field of v and, when its
// environment variable is set (path upper-cased, joined by '_'), parses the
// value into the field. An empty environment value counts as not set.
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
