// Package config loads the value half of framework configuration; the
// behavior half is assembled in code around the Application.
//
// Values are read from config/app.yaml, overridden for the active environment
// by config/web-<env>.yaml, and finally overridden by environment variables:
// any leaf field is addressable by its field path upper-cased and joined with
// underscores (DB.DSN -> DB_DSN, App.Debug -> APP_DEBUG, Cache.Redis.Addr ->
// CACHE_REDIS_ADDR). The active environment is selected by the GOYII_ENV
// environment variable, defaulting to "dev".
package config
