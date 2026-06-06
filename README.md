flect — universal project runtime
==================================

What is flect?
--------------

flect is a portable, zero-dependency runtime environment for software
projects. It replaces Docker, docker-compose, Kubernetes, and custom
shell scripts with a single compiled binary that manages your project's
services consistently across every operating system and architecture.

A flect environment is not a container. It is not a virtual machine.
It is not a hypervisor. It is an abstraction layer that sits between
your project and the operating system, translating your intent into
platform-native operations with identical semantics everywhere.

The name "flect" comes from "flexible" and "reflect" — it flexibly
adapts to the host platform while reflecting the same environment
guarantees back to your project.


Why does flect exist?
---------------------

Modern software projects depend on multiple runtime services: databases,
caches, application servers, build tools. Every team solves the same
problems independently:

  - "It works on my machine" discrepancies between developers
  - Docker is heavyweight and requires daemon privileges
  - Docker Compose adds orchestration complexity
  - Kubernetes is designed for production clusters, not development
  - Shell scripts are platform-dependent and fragile
  - Every architecture (x86, ARM, RISC-V) needs separate handling
  - Android, iOS, and embedded platforms lack container support

flect solves all of these with a single, statically compiled binary
that requires no daemon, no elevated privileges, no interpreter, and
no external dependencies.


How does flect work?
--------------------

At the deepest level, flect is a cross-platform process supervisor
with deterministic lifecycle management. Here is how it operates:

Project Configuration

Every flect project declares its runtime topology in a declarative
configuration file placed at the root of the project directory. This
file describes each service — its executable, arguments, working
directory, environment variables, port bindings, and dependencies on
other services. The file is static, version-controllable, and requires
no scripting logic.

Initialization

When flect init runs, it creates a state directory within the project
(.flect) that will hold all runtime data, logs, and persistent state.
This directory is intentionally separated from source code so that
cleaning runtime state never affects project files. The initialization
process validates the configuration, checks for required executables
on the host, detects port availability, and reports any issues before
any process starts.

Service Lifecycle

Services are started in dependency order. flect builds a directed
acyclic graph from the dependency declarations and traverses it
topologically. Each service process is spawned as a child of the
flect process using operating-system-native process creation — fork
and exec on POSIX systems, CreateProcess on Windows.

Every child process is placed into its own process group (or job
object on Windows) so that flect can manage the entire subtree
collectively. This is critical for clean shutdown: when flect
terminates, it sends the appropriate platform signal to each process
group in reverse dependency order, ensuring databases flush their
buffers before application processes are terminated.

Environment Isolation

flect does not virtualize the kernel or use namespace isolation.
Instead, it provides deterministic environment management through
three mechanisms:

  - Working directory: each service runs in its declared working
    directory, which flect resolves to an absolute path on the host.
  - Environment variables: flect sets PATH, HOME, and any custom
    variables before spawning, shielding services from host
    environment inconsistencies.
  - Port management: flect detects port conflicts before starting
    services and reports them clearly, preventing silent failures.

This approach gives each service a consistent view of the world
without requiring kernel-level isolation, making flect portable to
any operating system that can spawn a child process.

Log Management

Standard output and standard error from every service are captured
and written to rotating log files in the state directory. Logs are
annotated with timestamps and service names. The flect logs command
provides filtered, tailed, or full-history access to any service's
output without requiring pipe redirection or external log aggregators.

State Persistence

Services declare data directories that should persist across restarts.
These are bound to paths inside the state directory. When a service
stops and starts again, its data survives. When flect clean runs,
these directories are removed, restoring the project to a pristine
state.

Signal Propagation

On POSIX systems, flect intercepts SIGINT, SIGTERM, and SIGHUP and
translates them into ordered shutdown of all services. On Windows,
it uses the console control handler to achieve the same effect.
This means pressing Ctrl-C in a terminal running flect start will
gracefully shut down every service in reverse dependency order.

Portability Layer

The entire implementation uses only the standard library of the
compiled language, which provides identical APIs across all target
platforms. Platform-specific behavior (signals, process groups, path
separators) is abstracted through conditional compilation and
interface dispatch. The result is a single source tree that produces
functionally identical binaries for any platform.


How does flect compare to Docker?
----------------------------------

Docker is a container runtime that uses Linux kernel namespaces and
control groups to create isolated environments with their own
filesystem, network stack, and process tree. It requires a running
daemon with root privileges.

flect is a process supervisor that uses OS-native process management
to run services with deterministic environment guarantees. It runs
as a regular user process with no daemon.

  Docker                      flect
  ─────────────────────────── ─────────────────────────────
  Requires root/daemon        Runs as regular user
  Kernel namespace isolation  Environment abstraction
  Container images (GBs)      Configuration file (KB)
  Network virtualization      Host networking
  Filesystem layers           Host filesystem
  Registry/docker hub         No registry needed
  Linux-only (native)         Runs on every OS
  Daemon memory overhead      No background processes

flect does not aim to replace Docker for all use cases. Docker is
appropriate when you need full filesystem isolation, multi-tenant
security boundaries, or production orchestration. flect is designed
for development environments, single-tenant deployments, and
platforms where Docker cannot run (Android, embedded Linux, resource-
constrained devices).


How does flect compare to Kubernetes?
--------------------------------------

Kubernetes is a production-grade container orchestration platform
designed for managing containerized applications across clusters of
machines. It requires a control plane, etcd, container runtime,
network overlay, and significant operational expertise.

flect manages processes on a single machine with no cluster, no
control plane, and no network configuration. It is to Kubernetes
what a bicycle is to a freight train: different tools for different
scales.

  Kubernetes                    flect
  ────────────────────────────  ─────────────────────────────
  Multi-node clusters           Single machine
  Control plane + workers       Single process
  Pods, services, ingresses     Services declared in config
  Horizontal scaling            Vertical (single instance)
  Self-healing                  Manual restart or wrapper
  Minutes to deploy             Milliseconds to start
  Requires container runtime    No dependencies
  Operational complexity        One binary

flect is not an alternative to Kubernetes for production
infrastructure. It is an alternative to Docker Compose, shell
scripts, Makefiles, and custom launchers for local development.


Why custom process management instead of proot?
-----------------------------------------------

proot is a userspace utility that uses ptrace to intercept system
calls and translate paths, user identities, and filesystem
operations. While powerful, it has fundamental limitations that make
it unsuitable for flect's goals:

  ptrace overhead. Every system call made by the traced process
  is intercepted, inspected, and potentially modified. This adds
  latency to every read, write, open, stat, and execve call.
  Database workloads and Node.js module loading become noticeably
  slower under ptrace.

  Platform lock-in. proot is Linux-only because it depends on the
  Linux ptrace interface and procfs. It cannot run on macOS, Windows,
  FreeBSD, or any non-Linux kernel. This directly contradicts
  flect's requirement of universal portability.

  Permission model. proot requires either root or the
  CAP_SYS_PTRACE capability. On systems where ptrace is restricted
  (some Android configurations, hardened kernels, containers),
  proot silently fails or produces incorrect behavior.

  Complexity surface. ptrace-based interception is inherently
  fragile. Kernel version differences, seccomp filters, and
  architecture-specific syscall tables all introduce edge cases
  that are difficult to test exhaustively.

  Philosophical mismatch. proot emulates a chroot environment by
  lying to the process about filesystem layout. flect's approach
  is not to deceive processes about their environment but to
  provide a consistent, predictable environment through explicit
  management of working directories, environment variables, and
  process groups.

flect's design philosophy is to do less but do it portably. Instead
of intercepting system calls, it sets up the environment correctly
before the process starts. Instead of emulating a root filesystem,
it manages data directories explicitly. Instead of namespacing
process IDs, it uses process groups and job objects.

This minimalism is deliberate: the less flect does at the kernel
boundary, the more platforms it can run on, and the fewer bugs it
can encounter.


How to use flect on your projects
----------------------------------

Using flect requires three steps that integrate into any project
regardless of language or framework.

Step one: declare. Create a flect configuration file in your project
root describing the services your project needs. Each service entry
specifies the executable, any arguments, the working directory, and
port bindings. Dependencies between services are declared explicitly
so flect can determine the correct startup and shutdown order.

Step two: initialize. Run flect init to create the state directory
and validate the configuration against the host environment. flect
will check that each required executable exists on the PATH, that
all declared ports are available, and that the configuration is
internally consistent. Any issues are reported immediately.

Step three: run. Run flect start to bring up all services in
dependency order. Logs stream to the terminal in real time. When
you press Ctrl-C or send SIGTERM, flect shuts down every service
in reverse order, ensuring clean termination.

At any point during development, you can inspect running services
with flect ps, view logs with flect logs, execute commands in the
project context with flect run, or reset to a clean state with
flect clean.

The workflow is identical on Linux, macOS, Windows, Android, and
any other operating system flect supports. Developers switching
between platforms experience no change in tooling. CI/CD pipelines
use the same commands as local development. The configuration file
is checked into version control, ensuring every team member and
every deployment target uses identical runtime topology.


Architecture independence
-------------------------

flect achieves architecture independence through a simple insight:
the binary itself must be compiled for the target architecture, but
the BEHAVIOR of the binary is identical everywhere.

When you download or build flect for your platform, you get a binary
tailored to your specific operating system and CPU architecture.
This binary then provides the same interface, the same configuration
format, the same command structure, and the same runtime guarantees
as the binary running on any other platform.

The project configuration file is platform-agnostic. It contains no
architecture-specific paths, no platform-dependent commands, and no
conditional logic. A flect.json written on an ARM64 Mac works
identically on an x86_64 Linux server and a Windows workstation.

Data stored in the .flect state directory is also platform-agnostic
whenever possible. Database files, log files, and state records use
portable formats that can be read across platforms. This enables
workflows like developing on a laptop and deploying the same project
directory to a server.

The runtime executables that flect manages (Node.js, PostgreSQL,
Redis, language runtimes) must of course be native to the platform.
flect does not bundle or virtualize these — it uses the versions
installed on the host. But it manages them consistently, ensuring
that the same configuration produces the same service topology
regardless of where the host executables came from.


Project status
--------------

flect is in its initial release. The core command set (init, start,
stop, restart, run, shell, ps, status, logs, clean) is functional
and stable across Linux, macOS, and Windows. Future releases will
add health check integration, service scaling, network tunnel
support, and declarative environment templating.

flect is published as open source software. Contributions are
welcome. The source, issue tracker, and documentation are available
at the project repository.
