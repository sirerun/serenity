# Early-access launch content packet

Prepared 2026-09-08 for David's review. This is a publication-ready draft, **not a published announcement or a v1.0 launch decision**. T7.6 remains human-owned. The canonical installation walkthrough is [serenity.sire.run/get-started/](https://serenity.sire.run/get-started/); subsequent first-memory steps are linked from that page. Do not maintain a competing installation guide in announcement copy.

## Announcement draft

Serenity gives agents and applications a memory layer in a Git repository you own. Keep source material inspectable, search it locally, and use the documented interfaces to connect memory to your existing tools. [See how it fits](https://serenity.sire.run/product/) and [how ownership works](https://serenity.sire.run/docs/ownership/).

This is early-access software. The current-source guides include work newer than packaged v0.1.1, and there is no standalone Serenity Brain app today. Start with a small folder of notes you know well. [Choose the appropriate installation path](https://serenity.sire.run/get-started/).

Full-text search works without a model key. Model-assisted extraction and cited answers require model configuration; using an agent from the brain directory alone does not configure those services. [First local memory](https://serenity.sire.run/docs/quickstart/) · [Models](https://serenity.sire.run/docs/models/) · [Agent and embedded interfaces](https://serenity.sire.run/docs/agents/).

Questions about setup? The [public documentation helper](https://serenity.sire.run/chat/) can point to relevant guides. It cannot access your personal brain. Questions and recent chat history may go to OpenRouter and its selected model provider; leave out private notes and secrets. [Chat privacy](https://serenity.sire.run/docs/privacy/#chat).

Explore the [source](https://github.com/sirerun/serenity), try one small workflow, and report issues with sanitized examples. Serenity credits [gbrain](https://github.com/garrytan/gbrain) and builds on the documented memory/protocol lineage. [Project README](https://github.com/sirerun/serenity#readme).

## Short demo script (about three minutes)

| Time | Show | Say / verify |
| --- | --- | --- |
| 0:00–0:30 | [Product page](https://serenity.sire.run/product/) and [installation guide](https://serenity.sire.run/get-started/) | “Serenity runs behind your agent or application. This is an early-access source walkthrough, not a new standalone app or v1.0 release.” Show the source/release distinction. |
| 0:30–1:10 | A disposable brain and a separate folder containing one invented note | Follow the [quickstart](https://serenity.sire.run/docs/quickstart/): initialize, check the keychain, configure the one file connector. Use no actual personal mailbox or private files. |
| 1:10–1:50 | `serenity sync`, then a search for a phrase present in the note | Verify the result contains that exact invented phrase and a source reference. Explain that this step is keyword search without a model key; do not describe it as semantic recall or generated reasoning. |
| 1:50–2:20 | The brain's files and Git history | Show the imported source and the repository boundary. Explain that backup requires a configured private remote or another chosen backup; local Git alone is not an off-device backup. [Ownership guide](https://serenity.sire.run/docs/ownership/). |
| 2:20–3:00 | [Agent interfaces](https://serenity.sire.run/docs/agents/) and [model guide](https://serenity.sire.run/docs/models/) | Show where users configure their chosen interface and models. End with the same canonical install URL and the early-access boundary. Avoid suggesting that opening an agent automatically configures MCP. |

Presenter preflight: use a fresh throwaway brain, confirm the operating-system keychain is available, and ensure the note is older than the connector's two-second settling interval. Do not record credentials, daemon tokens, real source folders or a personal inbox. If a step fails, show its actual error and the [troubleshooting guide](https://serenity.sire.run/docs/troubleshooting/), not a prerecorded success presented as live.

## Copy-paste installation excerpt

Prerequisites and supported platforms are maintained in the [canonical guide](https://serenity.sire.run/get-started/). For its current-source path (Go 1.26+, Git, supported OS/keychain):

```sh
GOWORK=off go install github.com/sirerun/serenity/cmd/serenity@main
serenity --help
```

Ensure the Go binary directory is on `PATH`; the guide explains `go env GOBIN` and the `bin` directory under `go env GOPATH`. Then follow its link to the first-memory walkthrough. Packaged Homebrew installation remains a separate v0.1.1 path; do not claim it includes every current-source command.

Verification on 2026-09-08: installation through the public Go module proxy succeeded without repository credentials, with `GOPRIVATE`, `GONOPROXY` and `GONOSUMDB` empty and no direct fallback. The binary's `--help` succeeded. Installed module version: `v0.1.2-0.20260908130709-28af2c8fc348`. This verifies public source distribution; it is not a v1.0 release or real-mailbox performance claim.

## FAQ answers

**Is this an app I open to chat with my memory?** There is no standalone Serenity Brain app today. Use the documented agent or application interfaces. The website chat is a separate public-documentation helper. [Interfaces](https://serenity.sire.run/docs/agents/) · [FAQ](https://serenity.sire.run/docs/faq/).

**Do I need an API key to start?** Not for full-text search over imported sources. Extraction and generated answers are separate model-backed steps. [Quickstart](https://serenity.sire.run/docs/quickstart/) · [Models](https://serenity.sire.run/docs/models/).

**Does all data always stay on my machine?** The brain's canonical files are local, but configured cloud models and backups can send data elsewhere. Choose providers and destinations deliberately; the website chat has its own data path. [Ownership](https://serenity.sire.run/docs/ownership/) · [Privacy](https://serenity.sire.run/docs/privacy/).

**Can the website helper read my brain?** No. It answers from public documentation. Its question/history can be sent to OpenRouter and the selected model provider. It does not persist chat transcripts; rate limiting retains salted IP-derived counts with an expiry, and provider operational policies still apply. [Chat privacy](https://serenity.sire.run/docs/privacy/#chat).

**Is this v1.0, and has a real 10,000-message laptop import passed?** No such release/performance claim is made. The synthetic cached-model benchmark measures pipeline overhead; the real-mailbox laptop gate and human release decision remain pending. [M5 evidence and limits](https://github.com/sirerun/serenity/blob/main/docs/evals/m5-report.md).

**Where should I report a problem?** Use the [issue tracker](https://github.com/sirerun/serenity/issues) with a minimal sanitized example. Never include API keys, tokens or personal source content. [Troubleshooting](https://serenity.sire.run/docs/troubleshooting/).

## Publication and cross-site status

Serenity's live Install navigation already points to the canonical installation guide. The matching labeled link for ndungu.dev is prepared in [draft PR #6](https://github.com/dndungu/dndungu.github.io/pull/6), including its HTML, Markdown and llms.txt surfaces. It targets `main`, the actual Pages source branch; that repository's default branch is `master`. Its static checks passed. **The ndungu.dev link is not yet deployed**, so T7.4 remains open on that cross-site acceptance clause. David owns that site's review/deployment and the announcement go/no-go.

Before publication, recheck the [launch checklist](checklist.md), the actual source/release versions and live links. No post, email, announcement or social message was sent from this preparation.
