# flect

light speed · zero gravity · universal runtime

```
  ╻ ╻┏━┓┏┳┓┏━┓┳ ┳
  ┃┏┛┃ ┃ ┃ ┃ ┃┃┏┛
  ┗┛ ┗━┛ ┻ ┗━┛┗┛ 
          universal project runtime
```

one binary. zero dependencies. every platform.

---

## what is flect

flect (pronounced "flekt") is a zero-dependency, cross-platform process
supervisor for software projects. it replaces docker compose, shell
scripts, makefiles, and kubernetes manifests with a single statically
compiled binary that runs anywhere go compiles to.

you declare what services your project needs in `flect.json`. flect
starts them in the right order, checks they are healthy, restarts them
if they crash, and shuts them down gracefully. all data lives in
`.flect/` inside your project — you can stop, restart, and clean
without losing state.

no daemon. no root. no containers. no vm. one file.

---

## quickstart

```
# in your project directory
flect init          # creates flect.json + .flect/
flect start         # starts all services in dependency order
flect ps            # shows running services
flect logs          # shows service logs
flect stop          # stops everything gracefully
```

---

## why not docker

| feature | docker | flect |
|---------|--------|-------|
| binary size | 2.4 GB (desktop) | 2.6 MB |
| daemon required | yes (dockerd) | no |
| root required | yes | no |
| architecture | linux-only kernel | every os + arch |
| filesystem | layers + overlay | host filesystem |
| network | virtual bridge | host networking |
| startup | seconds (pull images) | milliseconds |
| config | yaml + dsl | json (a single file) |
| learning curve | steep | 5 commands |

flect is not a replacement for docker in production. it is a
replacement for the complexity of running services locally during
development, on ci, on edge devices, and on platforms where docker
cannot run.

---

## why not kubernetes

kubernetes solves multi-node cluster orchestration. flect solves
single-machine service management. they are not competitors.

use kubernetes when you need:
- multi-node scaling and scheduling
- service mesh and network policies
- rolling deployments and canary releases
- cluster-wide observability

use flect when you need:
- a single service or small set of services running locally
- development environments that match production topology
- ci/cd pipelines that need postgres + redis + app
- embedded, iot, or android platforms
- zero-ops simplicity

---

## features

### service lifecycle

flect starts services in dependency order using topological sorting.
if A depends on B, B starts first. flect waits for B's health check
to pass before starting A.

shutdown is reverse order: A stops, then B. signals propagate to
entire process groups, so subprocesses are cleaned up.

### health checks

every service can declare a health check:

```json
"health_check": {
  "http_url": "http://localhost:3000/health",
  "tcp_port": 3000,
  "exec_command": "pg_isready",
  "interval_secs": 10,
  "timeout_secs": 5,
  "retries": 3
}
```

flect waits for health before starting dependent services and
restarts unhealthy services according to the restart policy.

### replicas

run multiple instances of a service for concurrency or redundancy:

```json
"backend": {
  "command": "node server.js",
  "replicas": 3,
  "port": 3000
}
```

flect assigns each replica a unique `FLECT_<NAME>_HOSTS` and
`FLECT_<NAME>_PORTS` environment variable so consumers can discover
all instances.

### restart policies

- `"always"` — restart on any exit
- `"on-failure"` — restart only on non-zero exit code
- `"never"` — do not restart

restart includes exponential backoff (1s, 2s, 4s, 8s, max 30s) to
avoid restart storms.

### service discovery

dependent services receive environment variables with connection
details for their upstream dependencies:

```
FLECT_POSTGRES_HOST=127.0.0.1
FLECT_POSTGRES_PORT=5432
FLECT_REDIS_HOST=127.0.0.1
FLECT_REDIS_PORT=6379
```

for replicated services:

```
FLECT_BACKEND_HOSTS=127.0.0.1,127.0.0.1,127.0.0.1
FLECT_BACKEND_PORTS=3000,3001,3002
```

### resource limits

```json
"resources": {
  "cpu_quota": 50,
  "memory_mb": 256
}
```

cpu_quota is a percentage (50 = half a core). memory_mb is the max
rss in megabytes. on linux these use setrlimit. on other platforms
they are advisory.

### security

linux security features when available:

- `PR_SET_NO_NEW_PRIVS` — prevent privilege escalation via setuid
- `PR_SET_PDEATHSIG` — child dies if parent dies
- setrlimit for cpu and memory caps
- read-only root option via chroot
- user switching via `User` field in config

on non-linux platforms, these are silently skipped.

---

## commands

| command | description |
|---------|-------------|
| `flect init` | create flect.json and .flect/ directory |
| `flect start` | start all services with health checks |
| `flect stop` | stop all services gracefully |
| `flect restart` | stop then start |
| `flect scale <s>=<n>` | set replica count for a service |
| `flect run <cmd>` | run a command in project context |
| `flect shell` | open an interactive shell |
| `flect ps` | list services with status, pid, port, uptime |
| `flect status` | show project health overview |
| `flect logs` [service] | view service logs |
| `flect pull <image>` | download a runtime environment |
| `flect images` | list cached environments |
| `flect clean` | delete .flect/ directory (keeps source) |
| `flect version` | print version |
| `flect help` | show help |

---

## running any program

flect can download and run pre-built runtime environments, similar to
docker pull + docker run but without containers:

```
flect pull node:20           # download node 20 runtime
flect pull python:3          # download python 3 runtime
flect images                 # list cached environments
flect run --image node:20 node script.js
flect run --image python:3 python app.py
```

environments are cached in `~/.flect/images/` and include musl-linked
binaries that work on any linux architecture. the `--image` flag can
be used with `flect run` and `flect shell` to execute inside the
environment.

you can also pull from any url:

```
flect pull https://example.com/runtimes/my-env.tar.gz
```

each environment contains an `env.json` that specifies PATH, default
command, and environment variables.

---

## configuration reference

a complete `flect.json`:

```json
{
  "version": "2",
  "project": "my-app",
  "services": {
    "db": {
      "command": "postgres -D .flect/data/db -p 5432",
      "port": 5432,
      "replicas": 1,
      "restart": "always",
      "depends_on": [],
      "health_check": {
        "tcp_port": 5432,
        "interval_secs": 5,
        "timeout_secs": 5,
        "retries": 3
      },
      "resources": {
        "memory_mb": 256
      },
      "env": {
        "PGDATA": ".flect/data/db"
      },
      "data_dirs": [".flect/data/db"]
    },
    "app": {
      "command": "node --import tsx src/server.ts",
      "workdir": "backend",
      "port": 3000,
      "replicas": 2,
      "restart": "on-failure",
      "depends_on": ["db"],
      "health_check": {
        "http_url": "http://localhost:3000/health",
        "interval_secs": 10,
        "timeout_secs": 10,
        "retries": 5
      },
      "resources": {
        "cpu_quota": 50,
        "memory_mb": 512
      },
      "env": {
        "NODE_ENV": "production"
      }
    }
  }
}
```

---

## how it works

flect does not use containers, virtual machines, namespaces, or
ptrace. it uses what every operating system already provides:

- **process creation**: fork/exec on unix, createprocess on windows
- **process groups**: setpgid on unix, job objects on windows
- **signals**: sigterm/sigint for graceful shutdown
- **file descriptors**: stdout/stderr capture to rotating log files
- **environment variables**: explicit environment per process
- **working directories**: explicit chdir per process
- **resource limits**: setrlimit on linux

the isolation is environmental, not kernel-level. flect gives each
service a consistent view of the world — its own working directory,
its own environment variables, its own log stream — without lying
about the filesystem or network.

this design is intentional. kernel-level isolation (containers) is
linux-specific, requires root, adds overhead, and breaks on
platforms like android, ios, and embedded linux. flect's
environmental isolation works everywhere go compiles to.

---

## memory safety

flect is written in go, a memory-safe language with garbage
collection, bounds-checked arrays, and no dangling pointers. the
standard library provides safe abstractions for:

- file i/o (bufio, os.File with automatic close)
- process management (os/exec with proper reaping)
- signal handling (os/signal with dedicated channel)
- network operations (net.DialTimeout with context support)
- json parsing (encoding/json with schema validation)

no unsafe code is used. no cgo is required. the binary is a pure go
static build with zero external dependencies.

---

## building from source

prerequisites: go 1.21+

```
git clone https://github.com/NaveenSingh9999/flect
cd flect
go build -ldflags="-s -w" -o flect .
```

cross-compile for any platform:

```
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o flect-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o flect-linux-arm64 .
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o flect-darwin-amd64 .
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o flect-darwin-arm64 .
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o flect.exe .
```

---

## platform support

| os | arch | status |
|----|------|--------|
| linux | amd64, arm64, arm, riscv64 | tested |
| macos | amd64, arm64 | tested |
| windows | amd64 | builds |
| android (termux) | arm64 | tested |
| freebsd | amd64 | builds |
| openbsd | amd64 | builds |

---

## license

mit
