# Agent browser-safety prompt

A reusable preamble to give any agent (or `wb` worktree task) that will verify this
project's UI or otherwise drive a browser on the developer's machine.

## Why this exists

Two agents verifying this project's treemap broke the developer's Mac:

1. They ran `pkill -f "Google Chrome"` / `pkill -9 -f 'Google Chrome'` — 34 times
   across two sessions — to "clean up leftover test instances". Because `pkill -f`
   matches the whole command line, and the real browser's command line is literally
   `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`, this SIGKILLed the
   developer's actual browser and every open window.
2. They launched headless Chrome with `HOME` redirected to an empty scratch directory.
   Chrome then could not find `$HOME/Library/Keychains/login.keychain-db`, logged
   `Encryption is not available`, and triggered a macOS "Chrome Safe Storage" keychain
   dialog on every launch — roughly once a minute.

Both are avoided by a small set of hard rules. `AGENTS.md` carries the rules for this
repository; this file is the portable version to hand to an agent up front.

## Usage

Paste everything inside the block below, replacing `<paste the actual task here>` with
the real task. It is deliberately self-contained so the receiving agent needs no
prior context.

````text
You are working in /Users/alex/projects/cover-100/cover100-cli — the Go CLI "cover100"
plus a vanilla-JS treemap front-end served from public/.

STEP 0: Read AGENTS.md in that repo root. It is binding. The rules below restate it,
because a previous agent violated them and broke the developer's machine twice.

═══════════════════════════════════════════════════════════════════
NON-NEGOTIABLE: DO NOT DISTURB THE DEVELOPER'S BROWSER
═══════════════════════════════════════════════════════════════════
This is the developer's daily-driver Mac, not a disposable CI box.

1) NEVER kill Chrome by name. These are forbidden:
       pkill -f "Google Chrome"
       pkill -9 -f 'Google Chrome'
       killall "Google Chrome"   /   killall Chrome

   `pkill -f` matches the ENTIRE command line, and the developer's real browser's
   command line is literally:
       /Applications/Google Chrome.app/Contents/MacOS/Google Chrome
   So this SIGKILLs their actual browser and every open window. A previous agent did
   exactly this 34 times to "clean up leftover test instances". It closed Chrome
   repeatedly for over an hour.

   Kill precisely — only what you started:
       # preferred: the PID you launched
       ./run-chrome.sh ... & CPID=$!
       kill "$CPID" 2>/dev/null

       # or a discriminator unique to your run
       pkill -f -- '--user-data-dir=/tmp/chrome-cover100'

   If a pattern could possibly match /Applications/Google Chrome.app, do not use it.

2) EVERY headless Chrome launch MUST include these two flags:
       --use-mock-keychain --password-store=basic
   Without them Chrome opens the real login keychain for the "Chrome Safe Storage"
   item, and macOS throws a permission dialog at the developer. This is exactly why
   Playwright never trips it — Playwright passes both.

3) NEVER redirect HOME for a Chrome run. Do not do:
       HOME=/some/scratch/dir "/Applications/Google Chrome.app/..."
   Chrome then looks for $HOME/Library/Keychains/login.keychain-db, fails with
   "Encryption is not available" (login_database_async_helper.cc), and pops a
   keychain dialog on EVERY launch — the developer saw one roughly every 60 seconds.
   Isolate with --user-data-dir=/tmp/chrome-<task> instead.

4) PREFER PLAYWRIGHT over hand-rolled Chrome invocations. It is already installed
   under ~/Library/Caches/ms-playwright, it handles keychain and profile isolation
   correctly, and it cleans up after itself.

5) Never kill any process you did not start. No pkill/killall on generic names
   (chrome, node, go, python) that appear in unrelated commands.

6) Clean up only your own scratch state. Use a task-unique directory such as
   /tmp/chrome-cover100 and `rm -rf` it when finished.

═══════════════════════════════════════════════════════════════════
HOW TO VERIFY THIS PROJECT
═══════════════════════════════════════════════════════════════════
    go build ./... && go vet ./... && go test ./...

`cover100` opens its report through the OS opener (internal/serve/serve.go →
open / xdg-open / rundll32). That is safe and needs no headless browser at all.
Headless Chrome is ONLY needed for screenshot or --dump-dom checks, and only under
the rules above.

═══════════════════════════════════════════════════════════════════
REPORTING RULES
═══════════════════════════════════════════════════════════════════
- Before running ANY command containing pkill, killall, or kill -9, state in your
  reply which PID or unique pattern you intend to target and why. Then run it.
- If a command is denied by the sandbox, STOP and ask. Do not seek a wider
  permission to work around a denial, and do not retry it another way.
- Never request danger-full-access to read or append a log file. If you cannot
  write a log, report the exact path you needed and stop.
- Work in the workspace you were given. Do not write outside it.

═══════════════════════════════════════════════════════════════════
TASK
═══════════════════════════════════════════════════════════════════
<paste the actual task here>
````
