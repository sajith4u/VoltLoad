# VoltLoad – SRS Update
## Version 1.1 – Standalone Cross-Platform Application

**Status:** Updated Requirements

## 1. Product Deployment Model

### 1.1 Standalone Application

VoltLoad shall be developed as a fully standalone desktop application that users can download, install and execute directly on their local computers.

The application shall operate independently without requiring:

- A separately deployed backend server.
- Docker or Docker Compose.
- Kubernetes.
- A cloud-hosted service.
- An external database server.
- An internet connection for core functionality.
- A separate runtime installation of Go.
- A separate runtime installation of Node.js.
- A browser installation.

The application shall bundle the necessary application components and runtime dependencies into its distributable packages.

### 1.2 Supported Operating Systems

The initial release shall support:

| Operating System | Architecture | Distribution |
|---|---|---|
| Windows 10 | x64 | Installer / Executable |
| Windows 11 | x64 | Installer / Executable |
| Ubuntu 22.04+ | x64 | AppImage / DEB |
| Ubuntu 24.04+ | ARM64 where supported | AppImage / DEB |
| Other compatible Linux distributions | x64 | AppImage where dependencies permit |

Additional Linux distributions may be supported after compatibility testing.

### 1.3 Application Execution

Users shall be able to launch VoltLoad directly from their operating system.

**Windows:**

- Install using a Windows installer.
- Launch through the Start Menu.
- Launch using a desktop shortcut.
- Launch the standalone executable.
- Uninstall through Windows application management.

**Linux:**

- Install using a DEB package.
- Launch through the application menu.
- Launch using an AppImage.
- Optionally launch through a terminal command.
- Uninstall through the package manager or application management.

### 1.4 Offline Capability

Core application functionality shall work without internet connectivity.

The following capabilities must work offline:

- Application startup.
- Project management.
- Environment management.
- Scenario creation.
- HTTP request configuration.
- Local load test execution.
- Performance metrics collection.
- Test history.
- Performance comparison.
- Report generation.
- Configuration import and export.

Internet access shall only be required when testing remote targets, downloading updates or accessing optional external integrations.

---

## 2. Updated System Architecture

### 2.1 Architecture Principles

VoltLoad shall use a local-first desktop architecture.

All core components shall execute within the user's local operating system environment.

The Go application shall provide the execution engine and application services. Wails shall provide the desktop integration layer, while React shall provide the user interface.

The application shall not require a separate backend deployment.

### 2.2 Updated Architecture Diagram

```text
+--------------------------------------------------+
|               USER'S OPERATING SYSTEM            |
|                                                  |
|  Windows / Linux                                 |
|                                                  |
|  +--------------------------------------------+  |
|  |              VOLTLOAD DESKTOP              |  |
|  |                                            |  |
|  |  +--------------------------------------+  |  |
|  |  |          React + TypeScript           |  |  |
|  |  |                                      |  |  |
|  |  | Dashboard                            |  |  |
|  |  | Scenario Builder                     |  |  |
|  |  | Test Configuration                   |  |  |
|  |  | Live Monitoring                      |  |  |
|  |  | Reports                              |  |  |
|  |  +------------------+-------------------+  |  |
|  |                     |                      |  |
|  |              Wails Bindings                |  |
|  |              Wails Events                  |  |
|  |                     |                      |  |
|  |  +------------------v-------------------+  |  |
|  |  |             GO APPLICATION           |  |  |
|  |  |                                      |  |  |
|  |  | Project Manager                      |  |  |
|  |  | Scenario Manager                     |  |  |
|  |  | Environment Manager                  |  |  |
|  |  | Test Manager                         |  |  |
|  |  | Reporting                            |  |  |
|  |  +------------------+-------------------+  |  |
|  |                     |                      |  |
|  |  +------------------v-------------------+  |  |
|  |  |          GO LOAD ENGINE              |  |  |
|  |  |                                      |  |  |
|  |  | Scheduler                            |  |  |
|  |  | Worker Pool                          |  |  |
|  |  | Virtual Users                        |  |  |
|  |  | HTTP Engine                          |  |  |
|  |  | Metrics Collector                    |  |  |
|  |  | Metrics Aggregator                   |  |  |
|  |  +------------------+-------------------+  |  |
|  |                     |                      |  |
|  |  +------------------v-------------------+  |  |
|  |  |            LOCAL STORAGE             |  |  |
|  |  |                                      |  |  |
|  |  | SQLite Database                      |  |  |
|  |  | Project Configurations               |  |  |
|  |  | Test Results                         |  |  |
|  |  | Reports                              |  |  |
|  |  +--------------------------------------+  |  |
|  +--------------------------------------------+  |
|                                                  |
+--------------------------------------------------+
                       |
                       v
              TARGET APPLICATIONS
             HTTP / HTTPS / APIs
```

### 2.3 Local Execution Requirements

| ID | Requirement |
|---|---|
| FR-APP-001 | The application shall run as a standalone desktop application. |
| FR-APP-002 | The application shall not require a separately deployed backend. |
| FR-APP-003 | All core application services shall execute locally. |
| FR-APP-004 | The load testing engine shall execute locally. |
| FR-APP-005 | The application shall use local storage for projects and test results. |
| FR-APP-006 | The application shall support offline operation. |
| FR-APP-007 | The application shall not require Docker or Kubernetes. |
| FR-APP-008 | The application shall not require a cloud account. |
| FR-APP-009 | The application shall package its required frontend assets. |
| FR-APP-010 | The application shall provide native installers for supported operating systems. |
| FR-APP-011 | The application shall support importing and exporting project configurations. |
| FR-APP-012 | The application shall support local backup and restoration. |

---

## 3. Desktop Runtime Requirements

### 3.1 Wails Integration

The application shall use Wails as the desktop application framework.

Wails shall provide:

- Native application window.
- Native operating system integration.
- Go-to-frontend communication.
- Frontend-to-Go method bindings.
- Event-based metric streaming.
- Application lifecycle management.
- Native application packaging.

The application shall bundle the React frontend during the build process.

The application shall not require a separately running React development server in production.

### 3.2 Frontend Runtime

React and TypeScript shall be compiled into static production assets.

These assets shall be embedded into the Go application.

The application shall load its frontend through the Wails runtime rather than requiring an externally hosted web application.

### 3.3 Go Runtime

The Go execution engine shall be compiled into the native desktop executable.

Users shall not need to install Go separately.

The Go application shall manage:

- HTTP connections.
- Worker pools.
- Virtual users.
- Test scheduling.
- Metrics collection.
- Local database operations.
- Report generation.

---

## 4. Local Data Storage Requirements

### 4.1 Storage Strategy

VoltLoad shall use SQLite as its embedded database.

No database server installation shall be required.

The database file shall be stored in the operating system's standard application data directory.

Suggested locations:

**Windows:**

```text
%APPDATA%\VoltLoad\
    database\
        voltload.db

    projects\
    scenarios\
    reports\
    logs\
    backups\
    config\
```

**Linux:**

```text
~/.local/share/voltload/
    database/
        voltload.db

    projects/
    scenarios/
    reports/
    logs/
    backups/
    config/
```

Application preferences shall be stored using the appropriate operating system configuration directory.

### 4.2 Storage Requirements

| ID | Requirement |
|---|---|
| FR-STO-001 | Use embedded SQLite. |
| FR-STO-002 | Create the database automatically on first launch. |
| FR-STO-003 | Apply database migrations automatically. |
| FR-STO-004 | Store projects locally. |
| FR-STO-005 | Store test scenarios locally. |
| FR-STO-006 | Store execution history locally. |
| FR-STO-007 | Store aggregated metrics locally. |
| FR-STO-008 | Support local backups. |
| FR-STO-009 | Support restoration from backups. |
| FR-STO-010 | Support configurable data retention. |
| FR-STO-011 | Avoid storing every individual HTTP request indefinitely. |
| FR-STO-012 | Protect sensitive credentials using available operating system security facilities. |

### 4.3 Data Portability

Users shall be able to:

- Export projects.
- Import projects.
- Export scenarios.
- Export test results.
- Export reports.
- Backup the local database.
- Restore previous application data.

A project export shall include the required project configuration and scenario definitions.

Sensitive credentials shall be excluded from exports by default unless explicitly selected by the user.

---

## 5. Updated Non-Functional Requirements

### 5.1 Portability

| ID | Requirement |
|---|---|
| NFR-PORT-001 | Application shall support Windows and Linux. |
| NFR-PORT-002 | Application shall not require Go installation. |
| NFR-PORT-003 | Application shall not require Node.js installation. |
| NFR-PORT-004 | Application shall not require a database server. |
| NFR-PORT-005 | Application shall not require a container runtime. |
| NFR-PORT-006 | Application shall use operating-system-specific application directories. |
| NFR-PORT-007 | Application shall provide native installation packages. |
| NFR-PORT-008 | Core functionality shall be available offline. |

### 5.2 Resource Efficiency

VoltLoad shall be designed to have low resource overhead when idle.

Initial engineering targets:

| Metric | Target |
|---|---|
| Idle memory usage | Target under 150 MB |
| Startup time | Target under 3 seconds |
| Idle CPU usage | Minimal |
| UI responsiveness | Normal interactions under 100 ms |
| Metrics update interval | Default 500 ms |
| Database | Embedded SQLite |
| Backend server | Not required |

These are engineering targets that must be validated on defined reference hardware.

During load testing, memory and CPU consumption will naturally increase according to workload size and configuration.

### 5.3 Application Stability

The application shall:

- Handle application shutdown gracefully.
- Stop active tests safely during shutdown.
- Prevent orphaned goroutines.
- Release network resources.
- Persist completed test results.
- Handle unexpected application termination.
- Detect database corruption where possible.
- Provide recovery options for interrupted test runs.

---

## 6. CLI Requirements

The CLI shall be available as a separate executable built from the same Go codebase.

It shall not require the desktop application to be installed.

Example:

```bash
voltload version

voltload validate ./tests/api.yaml

voltload run ./tests/api.yaml

voltload run ./tests/api.yaml \
  --vus 100 \
  --duration 5m

voltload report ./results/run-001.json
```

The CLI shall support Windows PowerShell, Windows Command Prompt and Linux terminals.

Desktop and CLI executions shall share the same engine and scenario configuration format.

The CLI shall support headless execution without launching any graphical interface.

---

## 7. Build and Distribution Requirements

### 7.1 Windows Distribution

The build pipeline shall generate:

```text
VoltLoad-Setup-x64.exe
VoltLoad.exe
```

Requirements:

- Windows native executable.
- Installer.
- Application icon.
- Start Menu shortcut.
- Optional desktop shortcut.
- Uninstaller.
- Application version information.

### 7.2 Linux Distribution

The build pipeline shall generate:

```text
voltload-amd64.AppImage
voltload-amd64.deb
```

Requirements:

- Linux native executable.
- Application icon.
- Desktop entry.
- Application menu integration.
- Package installation and removal support.
- Required system dependency documentation.

### 7.3 Build Pipeline

Build automation shall support:

```text
Windows AMD64
Linux AMD64
Linux ARM64
```

Builds shall be reproducible and versioned.

Each release shall include:

- Application binaries.
- Installation packages.
- Release notes.
- Checksums.
- Version information.
- Installation instructions.

### 7.4 Cross-Platform Testing

The application shall be tested independently on supported operating systems.

Validation shall cover:

- Application startup.
- Database initialization.
- Project management.
- HTTP execution.
- Concurrent load generation.
- Metrics accuracy.
- Test cancellation.
- Report generation.
- Backup and restoration.
- Installation and uninstallation.

---

## 8. Updated Release Definition

### VoltLoad 1.0 – Standalone Desktop Release

The first public release shall provide:

1. Standalone Windows application.
2. Standalone Linux application.
3. Go-powered HTTP load engine.
4. Embedded SQLite database.
5. Offline core functionality.
6. Project and environment management.
7. HTTP request management.
8. Concurrent virtual users.
9. Constant concurrency and arrival-rate workloads.
10. Smoke, load, stress and spike testing.
11. Real-time performance dashboard.
12. Latency percentiles.
13. Assertions and thresholds.
14. Historical test results.
15. Performance comparison.
16. HTML, JSON and CSV reports.
17. CLI executable.
18. Local backup and restoration.

### Final Product Definition

VoltLoad is a standalone, cross-platform desktop performance testing application powered by Go and Wails.

It shall run directly on Windows and Linux without requiring a separately deployed backend, external database, container runtime or cloud infrastructure.

All load generation, application services, metrics collection and local persistence shall execute on the user's machine.

The application shall remain fully functional offline for local project management, test configuration, load execution against reachable targets, analytics and reporting.

**Product tagline:**

VoltLoad – Put Your Systems Under Pressure.