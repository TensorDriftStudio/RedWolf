# Enterprise Engineering Standards for AI Assistants & Contributors

All code contributed to RedWolf must adhere to enterprise production standards. AI assistants must generate robust, defensive, idiomatic, and maintainable code without shortcuts or placeholders.

---

## 1. Core Engineering Principles

1. **Defensive Programming & Fail-Fast:**
   - Validate all inputs at system boundaries (HTTP request payloads, agent telemetry JSON, CLI arguments, environment variables).
   - Never assume hardware fields exist: disks may have no serials, NICs may have no carrier, BMCs may be unconfigured. Use fallback logic and defensive checks.
2. **Strict Idempotency:**
   - Every hardware, network, and database operation must be safe to execute multiple times. If an operation partially failed, repeating it must recover cleanly without corrupting state.
3. **Zero Shell Injection Vulnerabilities:**
   - Never use `sh -c`, `bash -c`, or string-concatenated commands when executing host processes in Go or Python.
   - Always use `exec.CommandContext(ctx, binary, arg1, arg2, ...)` with discrete argument slices.
4. **No Placeholders or Stubs:**
   - Never leave `// TODO: implement later`, `/* pass */`, or dummy mock data in production backend code. All error cases and state transitions must be fully handled.

---

## 2. Go Backend Architecture & Standards (Go 1.23+)

1. **Clean Layered / Hexagonal Architecture:**
   - **Domain / Models:** Pure business logic and entity structs (`Node`, `StorageDevice`, `DeploymentConfig`). No database or HTTP dependencies.
   - **Ports / Interfaces:** Define interfaces for repositories and external adapters (`NodeRepository`, `BMCClient`, `DHCPManager`, `ImageStreamer`).
   - **Adapters / Infrastructure:** Implementations (`sqlite_repo.go`, `ipmi_client.go`, `dnsmasq_manager.go`, `http_handler.go`).
2. **Context Propagation & Graceful Cancellation:**
   - The first parameter of every I/O, database, network, or process execution function must be `ctx context.Context`.
   - Always check `select { case <-ctx.Done(): return ctx.Err() }` in long-running loops or polling routines.
3. **Structured Logging (`log/slog`):**
   - Use the standard library `log/slog` package exclusively. Never use `fmt.Println` or `log.Printf`.
   - All log messages must be in English with structured key-value attributes:
     ```go
     slog.InfoContext(ctx, "bare-metal deployment initiated",
         "node_id", node.ID,
         "target_drive", cfg.TargetDrivePath,
         "os", cfg.OS,
         "ip", cfg.StaticIP,
     )
     ```
4. **Explicit, Typed Error Handling:**
   - Never discard errors (`_ = ...`).
   - Wrap errors with domain context using `fmt.Errorf("operation failed: %w", err)`.
   - Define sentinel errors (`ErrNodeNotFound`, `ErrDiskBusy`, `ErrBMCUnreachable`) and verify them using `errors.Is` and `errors.As`.
5. **Database Concurrency & Transactions (SQLite WAL):**
   - SQLite connections must enable Write-Ahead Logging (`PRAGMA journal_mode=WAL;`), foreign keys (`PRAGMA foreign_keys=ON;`), and busy timeouts (`PRAGMA busy_timeout=5000;`).
   - Multi-step state transitions (e.g. `READY` -> `PROVISIONING` -> `ACTIVE`) must execute within an explicit database transaction (`tx, err := db.BeginTx(...)`).
6. **Thread Safety & Race Conditions:**
   - All shared in-memory state must be guarded by `sync.RWMutex` or atomic primitives.
   - All code must pass `go test -race ./...` without data races.

---

## 3. Frontend Standards (React 19 + TypeScript + TailwindCSS)

1. **Strict TypeScript Typing:**
   - `any` and unconstrained type assertions (`as any`) are strictly prohibited.
   - Explicitly define interfaces/types for all component props, API payloads, and state objects.
2. **Defensive Rendering:**
   - Use optional chaining (`?.`) and nullish coalescing (`??`) for all deeply nested hardware telemetry fields.
   - Every list rendering (`.map()`) must use a stable, unique key (e.g. `device.byId` or `nic.mac`, never array index).
3. **State Management & Custom Hooks:**
   - Encapsulate complex state machines and API interactions in dedicated custom hooks (e.g., `useNodeDetails`, `useProvisioningStream`).
   - Separate presentational components from business/orchestration logic.
4. **Design System Consistency:**
   - Use Tailwind theme tokens defined in `tailwind.config.js` (`surface-card`, `surface-panel`, `redwolf-primary`, etc.). Ad-hoc inline CSS or non-standard hex values are prohibited.

---

## 4. Hardware Abstraction & System Security

1. **Immutable Storage Targeting:**
   - Destructive image flashing operations must strictly target `/dev/disk/by-id/...` or verified WWN paths. Writing to volatile kernel names (e.g. `/dev/sda`) without serial validation is prohibited.
2. **Non-Destructive Defaults:**
   - Telemetry discovery is strictly read-only.
   - Destructive actions (re-partitioning, disk formatting, BMC credential updates) require explicit operator confirmation and double-check validation.
3. **Cryptographic & Secret Security:**
   - Generated BMC credentials and user passwords must be encrypted at rest using AES-256-GCM.
   - Root user password hashes in Cloud-Init `user-data` must use SHA-512 crypt (`$6$`) or Yescrypt (`$y$`) with random salt. Plaintext passwords must never appear in persistent logs or telemetry dumps.
