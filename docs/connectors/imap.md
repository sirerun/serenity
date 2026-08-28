# IMAP connector

The IMAP connector (`internal/connector/imap`, plan T1.4) polls a mailbox
over IMAP and turns each message into a source. It produces `email`-kind
sources.

Gmail with an app password is the only certified provider for now (ADR
001). The connector is written against the generic IMAP surface —
UID-based cursors, `UIDVALIDITY` invalidation — so adding a second
provider is a fixture and a doc page, not a code fork.

## Setting up Gmail with an app password

An app password requires 2-Step Verification on your Google account.

1. Turn on 2-Step Verification, if it isn't already on: visit your
   [Google Account security settings](https://myaccount.google.com/security)
   and follow the 2-Step Verification setup flow.
2. Create an app password at
   [myaccount.google.com/apppasswords](https://myaccount.google.com/apppasswords).
   Name it something you'll recognize later, such as "serenity".
3. Copy the 16-character password Google generates. You won't see it
   again after you leave the page.
4. Run:

   ```sh
   serenity connectors auth imap --email you@gmail.com
   ```

5. When prompted, paste the app password and press Enter.

The command stores the password in your OS keychain, under the service
name reported in its output, and writes only your email address (never
the password) to `serenity.yml`. A grep of your brain repo and its
`.serenity/` runtime directory finds no trace of the secret.

The prompt reads your input as plain text — it isn't masked on your
terminal the way a login prompt usually is. Rather than typing the app
password where it might echo, pipe it in from a password manager's CLI,
for example:

```sh
op read "op://Personal/Gmail app password/password" | serenity connectors auth imap --email you@gmail.com
```

## Re-authenticating

Run the same command again with the same email address:

```sh
serenity connectors auth imap --email you@gmail.com
```

The new password overwrites the old one in the keychain. There's no
separate reset or delete step — re-auth is always this one command,
whether you're rotating a password on a schedule or recovering from a
revoked one. Re-authenticate when:

- Google reports the app password was revoked or removed.
- You regenerate the app password for any reason.
- The connector's `Poll` call fails with an authentication error citing
  `serenity connectors auth imap`.

Connecting a second mailbox is the same command with a different
`--email` — each account gets its own keychain entry, so authenticating
one account never disturbs another.

## How polling works

- **Mailbox:** `INBOX` by default. There's no CLI flag yet to poll a
  different mailbox.
- **Cursor:** a `UIDVALIDITY` and last-seen UID pair. If the mailbox's
  `UIDVALIDITY` changes — which IMAP defines as meaning every previously
  assigned UID may now refer to a different message, or none — the
  connector discards its cursor and re-polls from the start. The source
  store's content-address dedup keeps that full re-poll from creating
  duplicate sources.
- **Batching:** UID fetches are requested in batches of 200 messages at a
  time.
- **Mid-fetch deletions:** a message expunged between the connector's
  search and its fetch is simply absent from the server's response. The
  connector skips it and still advances the cursor past its UID, so a
  message deleted mid-poll can never wedge ingestion.

## Limitations

- Gmail with an app password is the only certified provider. OAuth and
  other IMAP providers (Fastmail, iCloud) aren't supported yet.
- Only `INBOX` is polled; there's no CLI flag to select a different
  mailbox.
- `serenity connectors auth imap` stores credentials, but nothing polls
  the mailbox automatically yet — wiring this connector into
  `serenity sync` is plan task T1.15.
