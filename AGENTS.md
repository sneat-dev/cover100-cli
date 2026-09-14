# Agent Instructions — cover100-cli

<!-- project-specific agent instructions.
     If this repository is initialised with `specstudio:init`, the SpecStudio
     Producer-Shape snippet may be appended below this section; keep these rules. -->

## Browser and process safety — read before running any browser-based verification

These rules exist because a previous agent verifying this UI repeatedly killed the
developer's **own** Google Chrome and triggered a macOS "Chrome Safe Storage"
keychain dialog roughly every 60 seconds.

### 1. Never kill Chrome by name

`pkill -f` matches the **whole command line**, and the developer's real browser's
command line is literally:

```
/Applications/Google Chrome.app/Contents/MacOS/Google Chrome
```

So `pkill -f "Google Chrome"`, `pkill -9 -f 'Google Chrome'` and
`killall "Google Chrome"` SIGKILL the user's actual browser — not just your test
instance. This is what closed Chrome repeatedly.

Kill precisely instead:

- Preferred: kill the exact PID you launched — `kill "$CPID"`
- Or match a discriminator unique to your run —
  `pkill -f -- '--user-data-dir=/tmp/chrome-cover100'`
- Or signal the process group you created.

Never use a pattern that can also match the real browser.

### 2. Always pass `--use-mock-keychain --password-store=basic` to headless Chrome

```bash
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  --headless=new --no-sandbox --disable-gpu \
  --use-mock-keychain --password-store=basic \
  --user-data-dir=/tmp/chrome-cover100 \
  --dump-dom "$URL"
```

Without these two flags Chrome opens the **real login keychain** for the
`Chrome Safe Storage` item, which makes macOS prompt for permission. This is also
why Playwright never trips the prompt — it passes both flags.

### 3. Never redirect `HOME` for a Chrome run

Do **not** run Chrome with `HOME=/some/scratch/dir`. Chrome then looks for
`$HOME/Library/Keychains/login.keychain-db`, cannot find it, logs
`Encryption is not available` (`login_database_async_helper.cc`), and shows a
keychain dialog on **every** launch. Isolate with `--user-data-dir` instead.

### 4. Clean up your own scratch state

Put throwaway profiles under `/tmp` with a task-unique name and remove them when
finished (`rm -rf /tmp/chrome-cover100`). Do not kill anything you did not start.

## Verifying this project

```bash
go build ./...
go vet ./...
go test ./...
```

`cover100` opens the report through the OS opener
(`internal/serve/serve.go` → `open` / `xdg-open` / `rundll32`), which is safe and
needs no headless browser. Headless Chrome is only needed for screenshot or
`--dump-dom` verification, and only under the rules above.
