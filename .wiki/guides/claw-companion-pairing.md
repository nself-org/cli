# ɳClaw Companion App Pairing

This guide walks through pairing the ɳClaw companion app (iOS, Android) with your nSelf server
running the `nself-claw` plugin.

## Prerequisites

- nSelf v0.9.9+ with `nself-claw` installed (Max tier license required)
- The ɳClaw companion app installed on your device

```bash
nself license set <your-max-key>
nself plugin install ai claw mux
nself build && nself start
```

## Option A — Telegram Bot (recommended)

Requires a Telegram bot configured with `nclaw notify add telegram`.

Once your bot is active, send `/pair` in your bot chat. The bot responds with a 6-char code
and a deep link. Tap the link on your device or enter the code manually in the app.

```
📱 Pairing code: ABCDEF
Tap: nclaw://pair?server=https%3A%2F%2Fapi.example.com&code=ABCDEF
Web: https://claw.nself.org/pair/ABCDEF?server=...
Expires in 10 minutes.
```

## Option B — nclaw CLI Wizard

Run `nclaw` on your server and follow the setup wizard. At the "Companion app" step:

```
nclaw notify add companion
```

The wizard generates a 6-char pairing code and shows it as:

```
 ┌──────────────┐
 │   ABCDEF     │
 └──────────────┘

Scan the QR code with ɳClaw, or enter the code manually.
nclaw://pair?server=https%3A%2F%2Fapi.example.com&code=ABCDEF
```

## Using the Code in the App

1. Open ɳClaw on your device
2. Tap **Pair with code**
3. Enter your server URL (e.g. `https://api.example.com`)
4. Enter the 6-char code (case-insensitive)
5. Tap **Verify code** — the server confirms the code is valid
6. Enter your nSelf account password to complete sign-in

Alternatively, scan the QR code by tapping the **Scan QR code** button in the app.

## Using the Web Relay

If you received a `claw.nself.org/pair/ABCDEF?server=...` link:

1. Open the link on your mobile device
2. Tap **Open in ɳClaw** — this launches the app with the fields pre-filled
3. Complete sign-in as normal

## Pairing Code Details

- Codes are 6 characters from the alphabet `ACDEFGHJKLMNPQRTUVWXY34679`
  (visually unambiguous — no 0/O, 1/I/l, B, S, Z)
- Valid for 10 minutes
- Single-use — redeemed immediately on first successful verify
- Rate-limited: 5 attempts per IP per 10-minute window

## Multiple Devices

Repeat the pairing process for each device. The server tracks all paired devices in
`np_claw_devices`. View or revoke devices with:

```
nclaw devices list
nclaw devices revoke <device-id>
```

Or via Telegram: send `/devices` in your bot chat.

## Troubleshooting

**"Invalid or expired pairing code"** — the code has expired (10 min limit) or was already used.
Run `nclaw notify add companion` or send `/pair` in Telegram to generate a new one.

**"Too many attempts"** — 5 failed attempts from your IP. Wait 10 minutes.

**"Could not reach server"** — check your server URL includes the scheme (`https://`) and that
the nself-claw plugin is running (`nself status`).

**Deep link doesn't open the app** — make sure ɳClaw is installed. On Android, ensure the
app was not installed via APK sideload (Play Store install gets URL scheme registration).
