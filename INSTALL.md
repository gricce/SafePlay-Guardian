# Installing SafePlay Guardian

This guide walks a non-technical parent through installing the SafePlay Guardian
background service. It runs on the parent's PC (macOS or Windows) and serves a
local web dashboard at `http://127.0.0.1:7878`.

## What this installs

- One small program (`platform`) that runs in the background.
- A local database in your user data directory.
- A login service registered with your OS so the program starts at sign-in.

It does **not** install anything on your children's devices — those need the
SafePlay agent, covered in a separate guide.

## Step 1 — Download the right binary

Pre-built binaries for each release are produced by `make cross`:

| Platform                | File                                  |
|-------------------------|---------------------------------------|
| macOS (Apple Silicon)   | `dist/darwin-arm64/platform`          |
| macOS (Intel)           | `dist/darwin-amd64/platform`          |
| Windows (64-bit)        | `dist/windows-amd64/platform.exe`     |

Save the file somewhere stable. Suggested locations:

- **macOS**: `~/Applications/SafePlayGuardian/platform`
- **Windows**: `C:\Program Files\SafePlayGuardian\platform.exe`

The service definition embeds the binary's absolute path, so don't move it after
installing.

## Step 2 — Install as a background service

Open a terminal (macOS) or PowerShell (Windows) in the folder containing the
binary, then run:

### macOS

```bash
chmod +x platform
./platform install
./platform start
```

By default this installs as a **user-level LaunchAgent** in
`~/Library/LaunchAgents/safeplay-guardian.plist`. It runs when you log in, under
your own user account — no admin password required.

### Windows

Open PowerShell as Administrator (services on Windows always need elevation):

```powershell
.\platform.exe install
.\platform.exe start
```

This registers a Windows service that starts at boot.

## Step 3 — First-run setup in the browser

Open `http://127.0.0.1:7878` in your browser. The dashboard will prompt you to
create the parent account. This is the **only** account that can be created
without an existing session — pick a strong password.

## Useful subcommands

| Command                       | What it does                              |
|-------------------------------|-------------------------------------------|
| `./platform start`            | Start the background service              |
| `./platform stop`             | Stop the background service               |
| `./platform restart`          | Restart                                   |
| `./platform status`           | Print `running` / `stopped` / not-installed |
| `./platform uninstall`        | Remove the service registration           |
| `./platform`                  | Run interactively in the foreground (for debugging) |

You can pass flags at install time and they'll be baked into the service
definition:

```bash
./platform install -addr 127.0.0.1:7878 -data-dir "/Users/me/Library/Application Support/SafePlayGuardian"
```

Common flags (full list via `./platform -h`):

- `-addr 127.0.0.1:7878` — HTTP listen address.
- `-data-dir <path>` — where the SQLite database and `platform.log` live.
- `-no-scan` — disable LAN discovery.
- `-no-sweep` — disable the active ping sweep (passive ARP still runs).
- `-retention-window 14d` — how long raw events are kept before auto-deletion.

## Where things live

| What        | macOS                                                  | Windows                                            |
|-------------|--------------------------------------------------------|----------------------------------------------------|
| Database    | `~/Library/Application Support/SafePlayGuardian/platform.db` | `%AppData%\SafePlayGuardian\platform.db`           |
| Logs        | `~/Library/Application Support/SafePlayGuardian/platform.log` | `%AppData%\SafePlayGuardian\platform.log`          |
| Service def | `~/Library/LaunchAgents/safeplay-guardian.plist`       | Registered with the Service Control Manager        |

## At-rest data protection

The bundled SQLite driver is pure Go (no SQLCipher), so the database file is
**not encrypted at the database layer**. The data directory is created with
mode `0o700` (owner-only) on macOS/Linux. For real protection of the database
contents, enable full-disk encryption at the OS level:

- **macOS**: System Settings → Privacy & Security → FileVault
- **Windows**: Settings → Privacy & Security → Device encryption / BitLocker

Without FDE, anyone with physical access to the disk could read the database.
The dashboard's parent login protects against network access; FDE protects
against device theft.

## Troubleshooting

- **`/healthz` returns nothing**: confirm the service is running with
  `./platform status`. If it says "stopped", run `./platform start`.
- **Tail the log**: `tail -f "~/Library/Application Support/SafePlayGuardian/platform.log"`.
- **Port already in use**: pick a different address with
  `./platform install -addr 127.0.0.1:7879` (you'll need to `uninstall` first if
  it's already installed with a different port).
- **Locked out**: deleting `auth_sessions` and `auth_users` from the database
  resets the parent account (you'll be prompted to set up again). The SQLite
  CLI works: `sqlite3 "<data-dir>/platform.db" "DELETE FROM auth_sessions; DELETE FROM auth_users;"`.

## Uninstalling

```bash
./platform stop
./platform uninstall
```

This removes the service registration. The database and logs in your data
directory are left in place — delete them manually if you want a clean state.
