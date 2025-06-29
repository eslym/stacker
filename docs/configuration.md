# Stacker Configuration Guide

This document describes the configuration system for Stacker, including all available options, their types, defaults, and usage examples. Stacker supports configuration in YAML or JSON format.

## Configuration File Structure

The root of the configuration file is a mapping with the following top-level keys:

- `delay` (optional): Default initial delay before restarting a service. Accepts a duration string (e.g., `5s`, `1m`). Default: `5s`.
- `grace` (optional): Default grace period for service shutdown. Accepts a duration string. Default: `30s`.
- `admin` (optional): Admin interface configuration. Can be a boolean, or a mapping with `host`, `port`, and/or `unix` socket path.
- `services` (required): Mapping of service names to service definitions.

## Services Section

Each service entry supports the following fields:

- `cmd` (required): Command to run. String or list of strings.
- `workDir` (optional): Working directory for the service.
- `env` (optional): Mapping of environment variable names to values.
- `grace` (optional): Grace period for this service (overrides global `grace`).
- `optional` (optional): Boolean. If true, service is optional.
- `restart` (optional): Restart policy. Mutually exclusive with `cron`.
- `cron` (optional): Cron expression for scheduled jobs. Mutually exclusive with `restart`.
- `single` (optional, cron only): Boolean. If true, only one instance runs at a time.

### Restart Policy

The `restart` field can be:
- Boolean (`true`/`false`): `true` = always restart, `false` = never restart.
- String: One of `always`, `on-failure`, `never`, `exponential`.
- Mapping with advanced options:
  - `mode`: (string) One of `always`, `on-failure`, `never`, `exponential`.
  - `exponential`: (bool) Enable exponential backoff.
  - `delay`: (duration) Initial delay before restart.
  - `maxDelay`: (duration) Maximum delay for exponential backoff.
  - `maxRetries`: (int or "infinity") Maximum restart attempts.

### Admin Section

The `admin` field enables the HTTP admin interface. It can be:
- Boolean: `true` to enable with defaults, `false` to disable.
- Mapping:
  - `host`: (string) Bind address. Default: `localhost`.
  - `port`: (int or string) Port number. Default: `8080`.
  - `unix`: (string) Path to Unix socket.

## Special Variable: `$configDir`

All string fields that support environment variable expansion (e.g., `${VAR}`) also support a special variable `${configDir}`. This variable expands to the absolute directory path containing the loaded configuration file, regardless of your current working directory or how you invoke Stacker. This is especially useful for referencing files or scripts that are stored alongside your configuration, making your setup portable and robust.

**How `${configDir}` Works:**
- If your config file is `/etc/stacker/stacker.yaml`, then `${configDir}` will expand to `/etc/stacker`.
- If you run Stacker from any directory, `${configDir}` always points to the config file's directory, not your shell's working directory.
- This variable is available in all string fields, including `cmd`, `workDir`, `env` values, and any custom string field that supports environment expansion.
- You can combine `${configDir}` with other environment variables or path segments.

**Practical Examples:**
```yaml
services:
  myservice:
    cmd: ["${configDir}/bin/start.sh"]
    workDir: "${configDir}/work"
    env:
      CONFIG_PATH: "${configDir}/myservice.conf"
```
This ensures that your service always uses files relative to the configuration file, even if you move your deployment or run Stacker from a different directory.

## Example Configuration

```yaml
# Minimal example
services:
  echo:
    cmd: ["echo", "hello from echo"]
    restart: always
  cron:
    cmd: ["echo", "hello from cron"]
    cron: '* * * * *'

# Advanced example
admin:
  host: 0.0.0.0
  port: 9000
  unix: /tmp/stacker.sock
delay: 10s
grace: 60s
services:
  web:
    cmd: ["/usr/bin/webserver"]
    workDir: /srv/web
    env:
      ENV: production
      PORT: "8081"
    restart:
      mode: always
      exponential: true
      delay: 5s
      maxDelay: 1m
      maxRetries: 10
    grace: 45s
  job:
    cmd: ["/usr/bin/job"]
    cron: '0 * * * *'
    single: true
    optional: true
```

## Environment Variable Expansion

All string fields support `${VAR}` syntax for environment variable expansion, including the special `${configDir}` variable.

## Validation Rules
- Service names: Alphanumeric, `_`, or `-`.
- Environment variable names: Uppercase letters, numbers, and underscores, starting with a letter or underscore.
- `restart` and `cron` are mutually exclusive for a service.
- At least one service must be defined.

## Configuration Resolution Order
Stacker loads configuration from the following locations (highest priority first):
1. `STACKER_CONFIG_PATH` environment variable
2. Current working directory
3. User config directory
4. OS-specific defaults

For more details, see the [User Guide](user-guide.md).
