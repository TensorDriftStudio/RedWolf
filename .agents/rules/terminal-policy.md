# Terminal Execution Policy: Forbidden Commands

## Critical Rule for All AI Assistants
To ensure stability and responsiveness of the development environment, the following terminal commands are **strictly prohibited**:

### 🚫 Strictly Forbidden Commands:
1. **Direct Host `npm` / `npx` Invocations:**
   - **DO NOT RUN:** Do not execute `npm` or `npx` directly in the host shell.
   - **Allowed Scope:** `npm` and `npx` commands are strictly permitted ONLY inside Docker containers (`docker run` / `docker exec`).
2. **`npm run build` / `npx vite build` / `tsc -b`:**
   - **DO NOT RUN:** Do not execute production bundling commands inside the host terminal.
   - **Reason:** Bundling TypeScript and Vite assets spawns intensive worker processes that exhaust memory and cause the runner to hang.
3. **Unbounded Blocking Commands:**
   - Never run interactive CLI tools that wait on user standard input without non-interactive flags.
