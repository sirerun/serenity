#!/usr/bin/env python3
"""Opt-in local fixture: the ACTUAL hosted service binary, on loopback, behind a
synthetic embedding provider (T23.43).

What runs for real: `serenity hosted serve` (the same binary a deployment
runs), its dashboard signup in development mode, its scoped credential issuance,
its MCP endpoint, its memory store on disk, and its own clock.

What is synthetic: the embedding provider. A tiny loopback server inside this
file answers `POST /v1/embeddings` with deterministic vectors, and the two
accounts belong to a throwaway data directory. The fake MCP in
`fake_hosted_mcp.py` is NOT this: it is a stand-in for the wire protocol, and it
is never called a real server anywhere.

What this proves: plumbing, the TTL/forget seed protocol, the actual server
clock, account isolation and the ordinary APIs (dashboard, export, MCP). What it
does NOT prove: retrieval quality, model or dimension fitness, provider latency,
provider cost, deployment, billing, mail, or anything about production. The
provider is a cached-vector oracle plus a hash-bag fallback, so a passing run
says nothing about semantic embeddings. Provider and semantic qualification
stay BLOCKED.

Safety rules the fixture enforces (each has a test that does not need the
binary):
- it creates one fresh directory under an OWNED root, marks it, and removes only
  a directory whose marker matches; a pre-existing path is refused;
- the bind address, public origin and embedding base URL must be the literal
  loopback address `127.0.0.1`, and the embedding base URL must be this
  fixture's own provider;
- the child gets a minimal environment (no proxy variables, no credentials, its
  own HOME and TMPDIR);
- every synthetic credential is registered for redaction and the report is
  refused if any survives;
- it signals only the PID it spawned, after re-checking that PID's command line,
  and confirms the process is gone.

Nothing here spends money, calls a paid provider, sends mail, or touches a
network beyond 127.0.0.1.
"""

from __future__ import annotations

import argparse
import atexit
import hashlib
import html
import http.client
import http.server
import io
import json
import math
import os
import re
import secrets
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
import zipfile
from dataclasses import dataclass, field
from pathlib import Path
from urllib.parse import parse_qs, quote, urlencode, urlsplit

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parent.parent
SCRIPTS_HOSTED = REPO_ROOT / "scripts" / "hosted"
for _p in (str(HERE), str(SCRIPTS_HOSTED)):
    if _p not in sys.path:
        sys.path.insert(0, _p)

from lib import forgotten_targets as ft, ledger as ledger_lib, scoring, seeding  # noqa: E402
from lib.mcp_client import MCPClient  # noqa: E402
from lib.textutil import tokenize  # noqa: E402

OPT_IN_ENV = "T2343_LOCAL_SERVICE"
BIN_ENV = "SERENITY_BIN"
LOOPBACK = "127.0.0.1"
OWNED_PREFIX = "t2343-local-svc-"
OWNED_MARKER = ".t2343-owned"
DIMENSIONS = 1536  # a realistic embedding width; the vectors carry no meaning beyond their hash
MODEL = "local-synthetic-oracle"
MODEL_VERSION = "local-synthetic-oracle-v1"
PROVIDER_PATH = "/v1"
TOKEN_RE = re.compile(r"^sk_live_[0-9a-f]{8}_[A-Za-z0-9_-]{43}$")
_GENERIC_SECRET_PATTERNS = (
    re.compile(r"sk_live_[0-9a-f]{8}_[A-Za-z0-9_-]{20,}"),
    re.compile(r"(?<=token=)[A-Za-z0-9_%-]{20,}"),
    re.compile(r"(?<=serenity_session=)[A-Za-z0-9_-]{20,}"),
)
MAX_HTTP_BYTES = 16 * 1024 * 1024
# The service listens on <data_dir>/.hosted-admin.sock. A unix socket path over
# 103 bytes fails to bind on macOS ("invalid argument"), and the default temp
# directory there is already about 60 bytes, so the owned tree lives under /tmp.
SOCKET_NAME = ".hosted-admin.sock"
MAX_SOCKET_PATH_BYTES = 103
DEFAULT_PARENT = Path("/tmp")

LABEL = (
    "Verifies plumbing, the TTL and forget seed protocol, the actual server clock, account isolation and the "
    "ordinary APIs against the local hosted service binary, behind a synthetic loopback embedding provider. "
    "It measures no retrieval quality."
)
NOT_PROVED = (
    "Retrieval quality: the loopback provider is a cached-vector oracle plus a hash-bag fallback, not a semantic model.",
    "Any real provider's latency, dimensions, model pin, availability or cost. The provider and semantic "
    "qualification stay BLOCKED.",
    "Anything about production, deployment, billing, mail delivery or a hosted account that is not this throwaway one.",
    "The lexical-negative criterion, which the task41 reviewer must still resolve.",
)


class FixtureRefused(RuntimeError):
    """The fixture refused to start or to clean up because a safety rule failed.
    Messages name the rule, never a credential."""


# --------------------------------------------------------------------------
# Redaction
# --------------------------------------------------------------------------
class Redactor:
    """Replaces every registered secret, and anything shaped like a token, with
    a fixed placeholder. `assert_clean` is the fail-closed check the report
    goes through last."""

    def __init__(self) -> None:
        self._secrets: dict[str, str] = {}

    def add(self, label: str, value: str | None) -> None:
        if value and len(value) >= 8:
            self._secrets[value] = label

    def scrub(self, text: str) -> str:
        for value, label in sorted(self._secrets.items(), key=lambda kv: -len(kv[0])):
            text = text.replace(value, f"[redacted:{label}]")
            text = text.replace(quote(value, safe=""), f"[redacted:{label}]")
        for pattern in _GENERIC_SECRET_PATTERNS:
            text = pattern.sub("[redacted:token-shaped]", text)
        return text

    def scrub_json(self, value: object) -> object:
        return json.loads(self.scrub(json.dumps(value)))

    def leaks_in(self, text: str) -> list[str]:
        found = [label for value, label in self._secrets.items() if value in text or quote(value, safe="") in text]
        found += ["token-shaped" for p in _GENERIC_SECRET_PATTERNS if p.search(text)]
        return found

    def count(self) -> int:
        return len(self._secrets)

    def assert_clean(self, text: str) -> None:
        """Refuses `text` when any registered secret, or anything token-shaped,
        survives in it. The label is named, never the value."""
        found = self.leaks_in(text)
        if found:
            raise FixtureRefused(f"the report still contains secret material ({', '.join(sorted(set(found)))}); not writing it")


# --------------------------------------------------------------------------
# Loopback rules
# --------------------------------------------------------------------------
def require_loopback_url(url: str, what: str, *, scheme: str = "http") -> None:
    """Refuses anything but `scheme://127.0.0.1:<port>[path]` with no userinfo,
    query or fragment. `localhost` is refused too: a name can resolve
    elsewhere, a literal address cannot."""
    try:
        parts = urlsplit(url)
        port = parts.port
    except ValueError as e:
        raise FixtureRefused(f"{what} is not a valid URL") from e
    if parts.scheme != scheme:
        raise FixtureRefused(f"{what} must use {scheme}")
    if parts.hostname != LOOPBACK:
        raise FixtureRefused(f"{what} must be the literal loopback address {LOOPBACK}, not {parts.hostname!r}")
    if not port:
        raise FixtureRefused(f"{what} must name a port")
    if parts.username or parts.password or parts.query or parts.fragment:
        raise FixtureRefused(f"{what} must carry no userinfo, query or fragment")


def validate_service_config(cfg: dict, *, root: Path, provider_base_url: str) -> None:
    """The fixture's own check of the config it is about to hand the service,
    run before anything starts. The service validates too; this is the
    fixture refusing on its own account."""
    bind = str(cfg.get("bind", ""))
    host, _, port = bind.rpartition(":")
    if host != LOOPBACK or not port.isdigit():
        raise FixtureRefused(f"bind must be {LOOPBACK}:<port>, got {bind!r}")
    require_loopback_url(str(cfg.get("public_origin", "")), "public_origin")
    if urlsplit(cfg["public_origin"]).path not in ("", "/"):
        raise FixtureRefused("public_origin must be an origin without a path")
    base = str(cfg.get("embedding_base_url", ""))
    require_loopback_url(base, "embedding_base_url")
    if base != provider_base_url:
        raise FixtureRefused("embedding_base_url must be this fixture's own loopback provider, and nothing else")
    for key in ("data_dir", "secrets_dir"):
        path = Path(str(cfg.get(key, "")))
        if not path.is_absolute():
            raise FixtureRefused(f"{key} must be absolute")
        try:
            path.relative_to(root)
        except ValueError as e:
            raise FixtureRefused(f"{key} must live under the owned root") from e
    if cfg.get("billing_enabled"):
        raise FixtureRefused("billing must stay disabled")
    if len(os.fsencode(str(Path(str(cfg["data_dir"])) / SOCKET_NAME))) > MAX_SOCKET_PATH_BYTES:
        raise FixtureRefused(
            f"the service's admin socket path would exceed {MAX_SOCKET_PATH_BYTES} bytes; use a shorter parent directory"
        )


# --------------------------------------------------------------------------
# Owned directory tree
# --------------------------------------------------------------------------
class OwnedTree:
    """One directory this fixture created, marked with a random token. Cleanup
    removes it only when the marker still matches, so a wrong path, a swapped
    directory or a symlink is never deleted."""

    def __init__(self, root: Path, token: str) -> None:
        self.root = root
        self.token = token
        self._removed = False

    @classmethod
    def create(cls, parent: Path | None = None) -> "OwnedTree":
        base = str(parent if parent is not None else DEFAULT_PARENT)
        if not os.path.isdir(base):
            raise FixtureRefused("the parent directory for the owned tree does not exist")
        root = Path(tempfile.mkdtemp(prefix=OWNED_PREFIX, dir=base)).resolve()
        return cls._mark(root)

    @classmethod
    def create_at(cls, root: Path) -> "OwnedTree":
        """Creates exactly `root`, which must not exist (not even as a dangling
        symlink) and must carry the owned prefix."""
        root = Path(os.path.abspath(root))
        if not root.name.startswith(OWNED_PREFIX):
            raise FixtureRefused(f"an owned directory's name must start with {OWNED_PREFIX!r}")
        if os.path.lexists(root):
            raise FixtureRefused("refusing to use a path that already exists")
        os.mkdir(root, 0o700)
        return cls._mark(root.resolve())

    @classmethod
    def _mark(cls, root: Path) -> "OwnedTree":
        token = secrets.token_hex(16)
        fd = os.open(root / OWNED_MARKER, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w") as f:
            f.write(token)
        return cls(root, token)

    def subdir(self, name: str, *, mode: int = 0o700) -> Path:
        if not name or "/" in name or name in (".", ".."):
            raise FixtureRefused("a subdirectory name must be a single plain component")
        path = self.root / name
        if os.path.lexists(path):
            raise FixtureRefused(f"refusing to use existing path {name!r}")
        os.mkdir(path, mode)
        os.chmod(path, mode)
        return path

    def owns(self) -> bool:
        try:
            if self.root.is_symlink() or not self.root.is_dir() or not self.root.name.startswith(OWNED_PREFIX):
                return False
            return (self.root / OWNED_MARKER).read_text() == self.token
        except OSError:
            return False

    def cleanup(self) -> None:
        if self._removed:
            return
        if not self.owns():
            raise FixtureRefused("refusing to remove a directory whose ownership marker does not match")
        # A helper the service started (a git or sync child) can still be writing into the tree for a moment after
        # the service exits, and rmtree then fails with "directory not empty". Retry a few times; the marker is gone
        # after the first partial pass, so between attempts only the path's identity is re-checked.
        last: OSError | None = None
        for attempt in range(6):
            if attempt:
                if self.root.is_symlink() or not self.root.is_dir() or not self.root.name.startswith(OWNED_PREFIX):
                    raise FixtureRefused("the owned directory changed identity during cleanup; not removing it")
                time.sleep(0.5 * attempt)
            try:
                shutil.rmtree(self.root)
            except OSError as e:
                last = e
            if not os.path.lexists(self.root):
                break
        if os.path.lexists(self.root):
            raise FixtureRefused(
                f"the owned directory {self.root.name} could not be fully removed "
                f"({type(last).__name__ if last else 'still present'}); it is left for the operator"
            )
        self._removed = True


# --------------------------------------------------------------------------
# Loopback synthetic embedding provider
# --------------------------------------------------------------------------
def hash_bag_vector(text: str, dim: int = DIMENSIONS) -> list[float]:
    """Deterministic unit vector from token hashes. No model, no randomness."""
    vec = [0.0] * dim
    tokens = tokenize(text)
    if not tokens:
        vec[0] = 1.0
        return vec
    for tok in tokens:
        digest = hashlib.sha256(tok.encode("utf-8")).digest()
        vec[int.from_bytes(digest[:4], "big") % dim] += 1.0 if digest[4] % 2 == 0 else -1.0
    norm = math.sqrt(sum(x * x for x in vec))
    if norm == 0:
        vec[0] = 1.0
        return vec
    return [round(x / norm, 8) for x in vec]


class LoopbackEmbeddingProvider:
    """`POST /v1/embeddings` on 127.0.0.1, the shape internal/router/
    openai_embeddings.go speaks. It checks the bearer token against the one
    synthetic key, so a request carrying anything else is refused and counted.

    `oracle` maps an input string to the text whose vector it should share (a
    cached vector: a query is made retrievable against the fact it should find).
    Every other input gets its own hash-bag vector. This makes seeded targets
    retrievable so the protocol can be exercised. It says nothing about quality."""

    def __init__(self, api_key: str, *, dim: int = DIMENSIONS, oracle: dict[str, str] | None = None) -> None:
        self.api_key = api_key
        self.dim = dim
        self.oracle = dict(oracle or {})
        self._lock = threading.Lock()
        self.requests = 0
        self.auth_failures = 0
        self.bad_requests = 0
        self.input_sha256: dict[str, int] = {}
        self.models_seen: set[str] = set()
        self.total_input_bytes = 0
        self.max_body_bytes = 0
        self._server: http.server.ThreadingHTTPServer | None = None
        self._thread: threading.Thread | None = None

    @property
    def base_url(self) -> str:
        assert self._server is not None
        return f"http://{LOOPBACK}:{self._server.server_address[1]}{PROVIDER_PATH}"

    def start(self) -> str:
        provider = self

        class Handler(http.server.BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def log_message(self, *_a) -> None:  # silence: nothing from the provider reaches a log
                return

            def _send(self, status: int, payload: dict) -> None:
                body = json.dumps(payload).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def do_POST(self) -> None:  # noqa: N802
                length = int(self.headers.get("Content-Length") or 0)
                raw = self.rfile.read(min(length, 1 << 20))
                if self.headers.get("Authorization") != f"Bearer {provider.api_key}":
                    with provider._lock:
                        provider.auth_failures += 1
                    return self._send(401, {"error": {"message": "unauthorized"}})
                if self.path != f"{PROVIDER_PATH}/embeddings":
                    with provider._lock:
                        provider.bad_requests += 1
                    return self._send(404, {"error": {"message": "not found"}})
                try:
                    req = json.loads(raw)
                    text, model = req["input"], req["model"]
                    if not isinstance(text, str) or not isinstance(model, str):
                        raise TypeError
                except (ValueError, KeyError, TypeError):
                    with provider._lock:
                        provider.bad_requests += 1
                    return self._send(400, {"error": {"message": "bad request"}})
                vector = hash_bag_vector(provider.oracle.get(text, text), provider.dim)
                tokens = max(1, len(text.encode("utf-8")) // 4)
                with provider._lock:
                    provider.requests += 1
                    digest = hashlib.sha256(text.encode("utf-8")).hexdigest()
                    provider.input_sha256[digest] = provider.input_sha256.get(digest, 0) + 1
                    provider.models_seen.add(model)
                    provider.total_input_bytes += len(text.encode("utf-8"))
                    provider.max_body_bytes = max(provider.max_body_bytes, len(raw))
                self._send(200, {
                    "object": "list",
                    "data": [{"object": "embedding", "index": 0, "embedding": vector}],
                    "model": model,
                    "usage": {"prompt_tokens": tokens, "total_tokens": tokens},
                })

        self._server = http.server.ThreadingHTTPServer((LOOPBACK, 0), Handler)
        self._server.daemon_threads = True
        self._thread = threading.Thread(target=self._server.serve_forever, kwargs={"poll_interval": 0.1}, daemon=True)
        self._thread.start()
        require_loopback_url(self.base_url, "provider base URL")
        return self.base_url

    def stop(self) -> None:
        server, thread = self._server, self._thread
        self._server = self._thread = None
        if server is not None:
            server.shutdown()
            server.server_close()
        if thread is not None:
            thread.join(timeout=5)

    def saw(self, text: str) -> int:
        return self.input_sha256.get(hashlib.sha256(text.encode("utf-8")).hexdigest(), 0)


# --------------------------------------------------------------------------
# Exact-PID process ownership
# --------------------------------------------------------------------------
def command_line_of(pid: int) -> str | None:
    """The full command line `ps` reports for `pid`, or None when no such
    process exists."""
    out = subprocess.run(["ps", "-p", str(pid), "-o", "command="], capture_output=True, text=True)
    text = out.stdout.strip()
    return text or None


class OwnedProcess:
    """A child this fixture spawned. It signals only that PID, and only after
    `ps` shows the command line still carries the marker recorded at spawn."""

    def __init__(self, proc: subprocess.Popen, marker: str) -> None:
        self.proc = proc
        self.pid = proc.pid
        self.marker = marker
        self.stopped_by: str | None = None

    def alive(self) -> bool:
        return self.proc.poll() is None

    def stop(self, *, term_timeout: float = 15.0, kill_timeout: float = 5.0) -> dict:
        """SIGTERM, then SIGKILL after `term_timeout`. Returns what happened."""
        record = {"pid": self.pid, "signals": [], "exit_code": None}
        if self.proc.poll() is None:
            command = command_line_of(self.pid)
            if command is None or self.marker not in command:
                raise FixtureRefused("the process at the recorded PID no longer carries the owned marker; not signalling it")
            self.proc.send_signal(signal.SIGTERM)
            record["signals"].append("SIGTERM")
            try:
                self.proc.wait(timeout=term_timeout)
            except subprocess.TimeoutExpired:
                command = command_line_of(self.pid)
                if command is None or self.marker not in command:
                    raise FixtureRefused("the process at the recorded PID no longer carries the owned marker; not signalling it")
                self.proc.kill()
                record["signals"].append("SIGKILL")
                self.proc.wait(timeout=kill_timeout)
        record["exit_code"] = self.proc.returncode
        gone = command_line_of(self.pid)
        if gone is not None and self.marker in gone:
            raise FixtureRefused("the owned process is still listed by ps after it was stopped")
        record["confirmed_gone"] = True
        self.stopped_by = "signal" if record["signals"] else "already-exited"
        return record


# --------------------------------------------------------------------------
# HTTP helper (dashboard side; the MCP side uses lib.mcp_client)
# --------------------------------------------------------------------------
def http_exchange(port: int, method: str, path: str, *, headers: dict | None = None, body: bytes | None = None,
                  timeout: float = 20.0) -> tuple[int, dict, bytes]:
    conn = http.client.HTTPConnection(LOOPBACK, port, timeout=timeout)
    try:
        conn.request(method, path, body=body, headers=headers or {})
        resp = conn.getresponse()
        raw = resp.read(MAX_HTTP_BYTES + 1)
        if len(raw) > MAX_HTTP_BYTES:
            raise FixtureRefused("a loopback response exceeded the size bound")
        return resp.status, {k.lower(): v for k, v in resp.getheaders()}, raw
    finally:
        conn.close()


def free_loopback_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind((LOOPBACK, 0))
        return s.getsockname()[1]


@dataclass
class Account:
    """A synthetic account created through the dashboard's real signup. The
    repr never shows a credential."""

    label: str
    email: str
    cookie: str = field(repr=False)
    csrf: str = field(repr=False)
    brain_id: str = ""
    token: str = field(default="", repr=False)


class LocalHostedService:
    """The real `serenity hosted serve` on a fresh owned tree, behind a
    LoopbackEmbeddingProvider. Use as a context manager."""

    def __init__(self, serenity_bin: str | os.PathLike, *, parent: Path | None = None, root: Path | None = None,
                 oracle: dict[str, str] | None = None, redactor: Redactor | None = None) -> None:
        binary = Path(serenity_bin)
        if not binary.is_file() or not os.access(binary, os.X_OK):
            raise FixtureRefused("the serenity binary path is not an executable file")
        self.binary = binary
        self.redactor = redactor or Redactor()
        self.tree = OwnedTree.create_at(root) if root is not None else OwnedTree.create(parent)
        self.port = 0
        self.origin = ""
        self.provider: LoopbackEmbeddingProvider | None = None
        self.process: OwnedProcess | None = None
        self.accounts: dict[str, Account] = {}
        self.stop_record: dict | None = None
        self._oracle = oracle or {}
        self._log = None
        self._atexit = None
        self.log_path = self.tree.root / "service.log"
        self.config_path = self.tree.root / "config.json"
        self.data_dir = self.tree.root / "data"
        self.secrets_dir = self.tree.root / "secrets"
        self.api_key = ""
        self.started_monotonic: float | None = None

    # -- lifecycle ---------------------------------------------------------
    def __enter__(self) -> "LocalHostedService":
        try:
            self.start()
        except BaseException:
            self.close()
            raise
        return self

    def __exit__(self, *_exc) -> None:
        self.close()

    def start(self, *, ready_timeout: float = 60.0) -> None:
        self.api_key = "sk-synthetic-" + secrets.token_urlsafe(32)
        self.redactor.add("embedding-key", self.api_key)
        self.provider = LoopbackEmbeddingProvider(self.api_key, oracle=self._oracle)
        provider_url = self.provider.start()

        self.data_dir = self.tree.subdir("data")
        self.secrets_dir = self.tree.subdir("secrets")
        home = self.tree.subdir("home")
        tmp = self.tree.subdir("tmp")
        secret_path = self.secrets_dir / "EMBEDDINGS_API_KEY"
        fd = os.open(secret_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w") as f:
            f.write(self.api_key)

        last_error = "the service did not start"
        for _attempt in range(3):
            self.port = free_loopback_port()
            self.origin = f"http://{LOOPBACK}:{self.port}"
            cfg = {
                "bind": f"{LOOPBACK}:{self.port}",
                "data_dir": str(self.data_dir),
                "secrets_dir": str(self.secrets_dir),
                "public_origin": self.origin,
                "embedding_model": MODEL,
                "embedding_version": MODEL_VERSION,
                "embedding_base_url": provider_url,
            }
            validate_service_config(cfg, root=self.tree.root, provider_base_url=provider_url)
            self.config_path.write_text(json.dumps(cfg))
            os.chmod(self.config_path, 0o600)
            env = {
                "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
                "HOME": str(home),
                "TMPDIR": str(tmp),
                "LANG": "C",
                "SERENITY_HOSTED_DEV": "1",
            }
            self._log = open(self.log_path, "ab", buffering=0)
            os.chmod(self.log_path, 0o600)
            proc = subprocess.Popen(
                [str(self.binary), "hosted", "serve", "--config", str(self.config_path)],
                stdin=subprocess.DEVNULL, stdout=self._log, stderr=self._log, env=env, cwd=str(self.tree.root),
            )
            self.process = OwnedProcess(proc, marker=str(self.config_path))
            self._atexit = atexit.register(self.close)
            deadline = time.monotonic() + ready_timeout
            while time.monotonic() < deadline:
                if not self.process.alive():
                    break
                try:
                    status, _h, _b = http_exchange(self.port, "GET", "/readyz", timeout=5)
                except OSError:
                    status = 0
                if status == 200:
                    self.started_monotonic = time.monotonic()
                    return
                time.sleep(0.25)
            last_error = self.redactor.scrub(self.log_tail())
            if self.process.alive():
                self.process.stop()
                break  # started but never became ready: a real failure, not a port race
            self._log.close()
            self.process = None
        raise FixtureRefused(f"the service did not become ready; log tail: {last_error}")

    def log_tail(self, n: int = 1200) -> str:
        try:
            return self.log_path.read_text(errors="replace")[-n:]
        except OSError:
            return ""

    def stop_service(self) -> dict | None:
        if self.process is not None and self.stop_record is None:
            self.stop_record = self.process.stop()
        return self.stop_record

    def close(self) -> None:
        """Stops the exact owned PID, then the provider, then removes the owned
        tree. Safe to call twice. Never raises on a partial start."""
        problems: list[str] = []
        try:
            self.stop_service()
        except FixtureRefused as e:
            problems.append(str(e))
        if self._atexit is not None:
            atexit.unregister(self.close)
            self._atexit = None
        if self.provider is not None:
            self.provider.stop()
        if self._log is not None:
            self._log.close()
            self._log = None
        try:
            self.tree.cleanup()
        except FixtureRefused as e:
            problems.append(str(e))
        if problems:
            raise FixtureRefused("; ".join(problems))

    # -- dashboard: real dev-mode signup and scoped credential issuance -----
    def _origin_headers(self, extra: dict | None = None) -> dict:
        return {"Origin": self.origin, **(extra or {})}

    def _wait_for_login_link(self, offset: int, timeout: float = 15.0) -> str:
        deadline = time.monotonic() + timeout
        pattern = re.compile(rb"Development login: (\S+)")
        while time.monotonic() < deadline:
            with open(self.log_path, "rb") as f:
                f.seek(offset)
                m = pattern.search(f.read())
            if m:
                return m.group(1).decode()
            time.sleep(0.1)
        raise FixtureRefused("the service printed no development login link")

    def signup(self, label: str) -> Account:
        """Real signup: POST /login, read the link the service printed for
        development, consume it, open the dashboard, POST /credentials."""
        email = f"t2343-{label}-{secrets.token_hex(4)}@example.invalid"
        offset = self.log_path.stat().st_size
        status, _h, _b = http_exchange(
            self.port, "POST", "/login",
            headers=self._origin_headers({"Content-Type": "application/x-www-form-urlencoded"}),
            body=urlencode({"email": email}).encode(),
        )
        if status != 200:
            raise FixtureRefused(f"POST /login answered {status}")
        link = self._wait_for_login_link(offset)
        parts = urlsplit(link)
        self.redactor.add(f"{label}-login-link", link)
        for tok in parse_qs(parts.query).get("token", []):
            self.redactor.add(f"{label}-login-token", tok)
        if f"{parts.scheme}://{parts.netloc}" != self.origin or parts.path != "/login/consume":
            raise FixtureRefused("the login link does not point at this loopback service; not following it")
        status, headers, _b = http_exchange(self.port, "GET", f"{parts.path}?{parts.query}")
        cookie = _cookie_value(headers.get("set-cookie", ""), "serenity_session")
        if status != 303 or not cookie:
            raise FixtureRefused(f"consuming the login link answered {status} without a session")
        self.redactor.add(f"{label}-session", cookie)
        account = Account(label=label, email=email, cookie=cookie, csrf="")
        page = self._dashboard_get(account, "/")
        account.csrf = _first(r'name="csrf" value="([^"]+)"', page, "csrf token")
        self.redactor.add(f"{label}-csrf", account.csrf)
        account.brain_id = _first(r'name="brain_id" value="([0-9a-zA-Z_-]{16,})"', page, "brain id")
        status, _h, body = http_exchange(
            self.port, "POST", "/credentials",
            headers=self._origin_headers({"Content-Type": "application/x-www-form-urlencoded", "Cookie": f"serenity_session={cookie}"}),
            body=urlencode({"csrf": account.csrf, "brain_id": account.brain_id}).encode(),
        )
        if status != 200:
            raise FixtureRefused(f"POST /credentials answered {status}")
        token = html.unescape(_first(r'<textarea id="token"[^>]*>([^<]+)</textarea>', body.decode(), "bearer token"))
        if not TOKEN_RE.match(token):
            raise FixtureRefused("the issued credential does not have the documented shape")
        account.token = token
        self.redactor.add(f"{label}-bearer", token)
        self.accounts[label] = account
        return account

    def _dashboard_get(self, account: Account, path: str) -> str:
        status, _h, body = http_exchange(self.port, "GET", path, headers={"Cookie": f"serenity_session={account.cookie}"})
        if status != 200:
            raise FixtureRefused(f"GET {path} answered {status}")
        return body.decode(errors="replace")

    def live_memories(self, account: Account) -> dict:
        """The dashboard's own count: Gateway.Inventory on the server clock."""
        page = self._dashboard_get(account, "/")
        m = re.search(r"<tr><td>Live memories</td><td>(\d+)</td><td>(\d+)</td>", page)
        if not m:
            raise FixtureRefused("the dashboard shows no Live memories row")
        return {"used": int(m.group(1)), "limit": int(m.group(2))}

    def export_facts(self, account: Account) -> dict:
        """The brain export: facts.json holds current facts only, on the server
        clock; brain.bundle is the canonical store."""
        status, headers, body = http_exchange(
            self.port, "GET", f"/brains/{account.brain_id}/export",
            headers={"Cookie": f"serenity_session={account.cookie}"}, timeout=60,
        )
        if status != 200 or headers.get("content-type") != "application/zip":
            raise FixtureRefused(f"export answered {status}")
        with zipfile.ZipFile(io.BytesIO(body)) as z:
            names = sorted(z.namelist())
            facts = json.loads(z.read("facts.json"))
            bundle_bytes = len(z.read("brain.bundle"))
        return {"names": names, "facts": facts, "bundle_bytes": bundle_bytes}

    def server_clock_skew_seconds(self) -> float:
        """Local clock minus the service's `Date` header, in seconds. `Date` has
        one-second resolution, so this is good to about one second."""
        before = time.time()
        _s, headers, _b = http_exchange(self.port, "GET", "/readyz", timeout=5)
        after = time.time()
        import email.utils

        server = email.utils.parsedate_to_datetime(headers["date"]).timestamp()
        return round((before + after) / 2 - server, 3)


def _cookie_value(set_cookie: str, name: str) -> str:
    m = re.search(rf"(?:^|,\s*){re.escape(name)}=([^;]+)", set_cookie)
    return m.group(1) if m else ""


def _first(pattern: str, text: str, what: str) -> str:
    m = re.search(pattern, text)
    if not m:
        raise FixtureRefused(f"could not find the {what} in the dashboard page")
    return m.group(1)


# --------------------------------------------------------------------------
# The verification scenario
# --------------------------------------------------------------------------
POS_ENV = "T2343_LOCAL_POSITIVE_CREDENTIAL"
EMPTY_ENV = "T2343_LOCAL_EMPTY_CASE_CREDENTIAL"
AUTHORIZATION_REF = "local-synthetic-fixture-no-spend"
ISOLATION_MARKER = "Isolation marker qq-4419: this synthetic fact is written only to the empty-case account."
# The service clock is read from a `Date` header that has one-second resolution,
# so a clock comparison is good to about this much.
DATE_HEADER_RESOLUTION_SECONDS = 1.0


@dataclass
class Check:
    name: str
    expected: str
    observed: str
    passed: bool

    def as_dict(self) -> dict:
        return {"name": self.name, "expected": self.expected, "observed": self.observed, "passed": self.passed}


class Checks:
    def __init__(self) -> None:
        self.items: list[Check] = []

    def add(self, name: str, expected: str, observed: object, passed: bool) -> bool:
        self.items.append(Check(name, expected, str(observed), bool(passed)))
        return bool(passed)

    @property
    def all_passed(self) -> bool:
        return bool(self.items) and all(c.passed for c in self.items)


def build_oracle(corpus: dict, fact_by_id: dict) -> dict[str, str]:
    """query -> the text whose vector it should share. Covers the five
    expected-empty queries (against their synthetic targets) and the 95 positive
    queries (against their expected facts), so the protocol's presence probes are
    satisfied and a live run scores every ranking correct. That is exactly why a
    local run can never be quality evidence: the oracle is the answer key."""
    oracle: dict[str, str] = {}
    for case in corpus["cases"]:
        if case["category"] == "empty":
            oracle[case["query"]] = seeding.fact_text(ft.target_id(case["id"]), fact_by_id)
        else:
            oracle[case["query"]] = fact_by_id[case["expected_fact_id"]]["text"]
    oracle[ft.SENTINEL_FACT_TEXT] = ft.SENTINEL_FACT_TEXT
    return oracle


def _stub_lexical_control(corpus: dict):
    """The local-service run does not run the lexical control (that is the
    task41 criterion, measured elsewhere); every case is a miss."""
    pos = [c for c in corpus["cases"] if c["category"] != "empty"]
    return [scoring.score_positive_case(c["id"], c["category"], c["expected_fact_id"], []) for c in pos], []


def build_manifest(svc: "LocalHostedService", template: dict, corpus_sha256: str, ledger_path: Path) -> dict:
    m = json.loads(json.dumps(template))
    m["task_id"] = "T23.43"
    m["corpus_sha256"] = corpus_sha256
    m["environment"] = {"kind": "local-fixture", "origin": svc.origin, "allowed_hosts": [LOOPBACK], "production_target_allowed": False}
    m["provider"].update(
        base_url=svc.provider.base_url, model=MODEL, version_pin=MODEL_VERSION, dimensions=DIMENSIONS,
        serving_provider="local-synthetic-oracle", privacy_review_ref="local-synthetic-no-personal-data", secret_ref="EMBEDDINGS_API_KEY",
    )
    m["budget"] = {
        "approved_max_usd": 5.0, "max_calls": 5000, "max_input_tokens": 10**8, "max_cost_per_call_usd": 0.001,
        "max_elapsed_seconds": 900, "authorization_ref": AUTHORIZATION_REF, "automatic_reset": False,
        "auto_top_up": False, "ledger_path": str(ledger_path),
    }
    m["seeding"] = {"authorized": True, "authorization_ref": AUTHORIZATION_REF}
    for key, env in (("hosted_mcp", POS_ENV), ("empty_case_hosted_mcp", EMPTY_ENV)):
        m[key] = {"endpoint_url": f"{svc.origin}/mcp", "credential_secret_ref": env, "allowed_origins": [svc.origin]}
    return m


def read_canonical_records(brain_dir: Path) -> list[dict]:
    """Read-only walk of a brain's canonical memory records on disk
    (`brain/sources/<xx>/<sha>/bytes`, JSON). Nothing is written or repaired."""
    records = []
    sources = brain_dir / "brain" / "sources"
    for path in sorted(sources.glob("*/*/bytes")):
        try:
            rec = json.loads(path.read_bytes())
        except ValueError:
            continue
        if isinstance(rec, dict) and rec.get("record_type") in ("memory_fact", "memory_expiry"):
            rec["_sha256"] = path.parent.name
            records.append(rec)
    return records


def with_rate_limit_retry(call, waits: list[float], *, attempts: int = 8, pause: float = 15.0):
    """The gateway allows 120 requests a minute per account, and a full-speed
    harness phase uses most of that. A 429 is the service enforcing its own limit,
    so wait it out (recorded in `waits`) rather than treating it as a failure."""
    for attempt in range(attempts):
        try:
            return call()
        except Exception as e:  # noqa: BLE001 -- only the fixed 429 text is retried
            if "HTTP 429" not in str(e) or attempt == attempts - 1:
                raise
            waits.append(pause)
            time.sleep(pause)


class RateWindow:
    """The gateway allows 120 requests a minute per account. The seed and live
    phases each send close to that at full speed, so the fixture waits out the
    window between phases instead of treating the service's own limit as a fault.
    Every wait is recorded."""

    WINDOW_SECONDS = 61.0

    def __init__(self) -> None:
        self.last = time.monotonic()
        self.waits: list[dict] = []

    def mark(self) -> None:
        self.last = time.monotonic()

    def wait(self, before: str) -> None:
        remaining = self.WINDOW_SECONDS - (time.monotonic() - self.last)
        if remaining > 0:
            time.sleep(remaining)
            self.waits.append({"before": before, "seconds": round(remaining, 1)})
        self.mark()


def _sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def _git(*args: str) -> str:
    out = subprocess.run(["git", "-C", str(REPO_ROOT), *args], capture_output=True, text=True)
    return out.stdout.strip()


def run_verification(
    serenity_bin: str | os.PathLike, *, parent: Path | None = None, allow_dirty_tree: bool = False,
    ttl_seconds: int | None = None, margin_seconds: int | None = None,
) -> dict:
    """Runs the whole scenario against a fresh local service and returns the
    redacted report. `ttl_seconds`/`margin_seconds` exist ONLY for a shortened
    smoke run and are recorded in the report as not the frozen constants; with
    them unset the production constants (TTL 60 s + margin 5 s) apply."""
    import eval_embeddings  # noqa: PLC0415 -- scripts/hosted is on sys.path

    binary_path = Path(serenity_bin)
    if not binary_path.is_file() or not os.access(binary_path, os.X_OK):
        raise FixtureRefused("the serenity binary path is not an executable file")
    redactor = Redactor()
    checks = Checks()
    report: dict = {"label": LABEL, "not_proved": list(NOT_PROVED)}
    corpus, _facts, fact_by_id = eval_embeddings.load_corpus(HERE / "corpus.json", HERE / "facts.json")
    corpus_sha256 = corpus["meta"]["corpus_hash_sha256"]
    template = json.loads((REPO_ROOT / "docs/launch/hosted-completion/qualification.example.json").read_text())

    production = ttl_seconds is None and margin_seconds is None
    constants = {
        "ttl_seconds": ft.EXPIRY_TTL_SECONDS if ttl_seconds is None else ttl_seconds,
        "margin_seconds": ft.EXPIRY_MARGIN_SECONDS if margin_seconds is None else margin_seconds,
        "production_constants": production,
        "supplemental_sha256": ft.targets_sha256(),
    }
    dirty = eval_embeddings.dirty_paths(REPO_ROOT)
    report["source"] = {"sha": _git("rev-parse", "HEAD"), "tree_was_dirty": bool(dirty), "dirty_paths": dirty[:10]}
    report["binary"] = {"name": Path(serenity_bin).name, "sha256": _sha256_file(Path(serenity_bin))}
    if dirty and not allow_dirty_tree:
        raise FixtureRefused("the tree has uncommitted harness changes, so source_sha would not name the code that ran; commit them")

    patches = []
    if not production:
        import unittest.mock as mock

        patches += [
            mock.patch.object(ft, "EXPIRY_TTL_SECONDS", constants["ttl_seconds"]),
            mock.patch.object(ft, "EXPIRY_MARGIN_SECONDS", constants["margin_seconds"]),
            mock.patch.object(ft, "EXPIRY_TAIL_SECONDS", 20),
        ]
    if allow_dirty_tree and dirty:
        import unittest.mock as mock

        patches.append(mock.patch.object(eval_embeddings, "dirty_paths", return_value=[]))
    for patch in patches:
        patch.start()
    started_at = time.time()
    root_path = None
    pid = None
    try:
        with LocalHostedService(serenity_bin, parent=parent, oracle=build_oracle(corpus, fact_by_id), redactor=redactor) as svc:
            root_path, pid = svc.tree.root, svc.process.pid
            _scenario(svc, eval_embeddings, corpus, fact_by_id, corpus_sha256, template, checks, report, constants)
            report["provider_requests"] = {
                "requests": svc.provider.requests, "auth_failures": svc.provider.auth_failures,
                "bad_requests": svc.provider.bad_requests, "models_seen": sorted(svc.provider.models_seen),
                "distinct_inputs": len(svc.provider.input_sha256),
            }
            checks.add(
                "loopback provider was used only as configured", "auth_failures=0, bad_requests=0, models_seen=[MODEL], requests>0",
                report["provider_requests"],
                svc.provider.auth_failures == 0 and svc.provider.bad_requests == 0
                and svc.provider.models_seen == {MODEL} and svc.provider.requests > 0,
            )
            report["process"] = svc.stop_service()
        checks.add(
            "the exact owned PID was stopped and is gone", "SIGTERM (or already exited), exit recorded, ps shows no such PID",
            {"pid_confirmed_gone": command_line_of(pid) is None, "record": report["process"]},
            report["process"] is not None and report["process"].get("confirmed_gone") is True and command_line_of(pid) is None,
        )
        checks.add("the owned directory was removed", "no path left", "removed" if not os.path.lexists(root_path) else "still present", not os.path.lexists(root_path))
    finally:
        for patch in reversed(patches):
            patch.stop()
        for name in (POS_ENV, EMPTY_ENV):
            os.environ.pop(name, None)
    report["constants"] = constants
    report["elapsed_seconds"] = round(time.time() - started_at, 1)
    report["checks"] = [c.as_dict() for c in checks.items]
    report["status"] = "PARTIAL" if checks.all_passed else "FAIL"
    report["evidence_level"] = "local-process"
    report["provider_and_semantic_qualification"] = "BLOCKED"
    if not production:
        report["status"] = "SMOKE-NOT-QUALIFYING" if checks.all_passed else "FAIL"
        report["limitations"] = ["Shortened TTL/margin constants: this run is a smoke test and is not the frozen 65 s protocol."]
    serialized = redactor.scrub(json.dumps(report, indent=2, sort_keys=True))
    redactor.assert_clean(serialized)
    report = json.loads(serialized)
    report["redaction"] = {"registered_secrets": redactor.count(), "verified_clean": True}
    return report


def _scenario(svc, ev, corpus, fact_by_id, corpus_sha256, template, checks: Checks, report: dict, constants: dict) -> None:
    pos = svc.signup("positive")
    emp = svc.signup("empty-case")
    checks.add("two fresh synthetic accounts, real dev-mode signup and scoped credential issuance", "distinct brains, bearer tokens of the documented shape",
               {"brains_distinct": pos.brain_id != emp.brain_id, "tokens_distinct": pos.token != emp.token},
               pos.brain_id != emp.brain_id and pos.token != emp.token and bool(TOKEN_RE.match(pos.token)) and bool(TOKEN_RE.match(emp.token)))
    checks.add("both accounts start empty on the dashboard", "Live memories 0 and 0",
               [svc.live_memories(pos)["used"], svc.live_memories(emp)["used"]],
               svc.live_memories(pos)["used"] == 0 and svc.live_memories(emp)["used"] == 0)
    os.environ[POS_ENV], os.environ[EMPTY_ENV] = pos.token, emp.token

    evaldir = svc.tree.subdir("eval")
    manifest = build_manifest(svc, template, corpus_sha256, evaldir / "ledger.jsonl")
    manifest_path = evaldir / "manifest.json"
    manifest_path.write_text(json.dumps(manifest))
    os.chmod(manifest_path, 0o600)

    def cli(mode: str, out: str, *extra: str):
        return ev.parse_args([f"--{mode}", "--manifest", str(manifest_path), "--output", str(evaldir / out), *extra])

    skew_before = svc.server_clock_skew_seconds()
    init_result, init_code = ev.run_init_ledger(cli("init-ledger", "init.json"))
    checks.add("--init-ledger began the cumulative ledger, offline", "exit 0, ledger calls 0",
               {"exit": init_code, "calls": (init_result.get("ledger") or {}).get("cumulative_calls")},
               init_code == ev.EXIT_PASS and (init_result.get("ledger") or {}).get("cumulative_calls") == 0)

    # ---- the seed, with the real waiting, against the real service ----
    window = RateWindow()
    seed_started = time.monotonic()
    rdir = evaldir / "receipts"
    seed_result, seed_code = ev.run_seed(cli("seed", "seed.json", "--receipt-dir", str(rdir)))
    seed_seconds = round(time.monotonic() - seed_started, 1)
    window.mark()
    skew_after = svc.server_clock_skew_seconds()
    report["seed"] = {
        "status": seed_result["status"], "exit": seed_code, "evidence_level": seed_result["evidence_level"],
        "elapsed_seconds": seed_seconds, "usage": {k: v for k, v in seed_result["usage"].items() if k != "ledger"},
        "acceptance": [{k: r[k] for k in ("criterion", "status")} for r in seed_result["acceptance"]],
        "blockers": seed_result.get("blockers", []),
        "provider": seed_result.get("provider"),
    }
    checks.add("the seed run completed against the real service", "PARTIAL, exit 0, both receipts complete",
               {"status": seed_result["status"], "exit": seed_code, "blockers": seed_result.get("blockers", [])[:2]},
               seed_result["status"] == "PARTIAL" and seed_code == ev.EXIT_PASS)
    checks.add("a local run is labelled fixture, never live-provider", "evidence_level=fixture; endpoints classed loopback",
               {"evidence_level": seed_result["evidence_level"], "classes": sorted({e["class"] for e in seed_result["provider"]["observed"]["hosted_endpoints"].values()})},
               seed_result["evidence_level"] == "fixture"
               and {e["class"] for e in seed_result["provider"]["observed"]["hosted_endpoints"].values()} == {"loopback"})
    if seed_code != ev.EXIT_PASS:
        return  # nothing below is meaningful without complete receipts

    receipts = {role: json.loads(Path(info["path"]).read_text()) for role, info in seed_result["seed_receipts"].items()}
    pos_r, emp_r = receipts[seeding.ROLE_POSITIVE], receipts[seeding.ROLE_EMPTY_CASE]

    # ---- seed receipt vs the actual current facts, through ordinary interfaces ----
    expected_keys = {x["operation_key"]: seeding.fact_text(x["corpus_fact_id"], fact_by_id) for x in pos_r["seeded"]}
    pos_export, emp_export = svc.export_facts(pos), svc.export_facts(emp)
    exported = {f["operation_key"]: f["fact"] for f in pos_export["facts"]}
    live_pos = svc.live_memories(pos)["used"]
    checks.add("positive account: receipt, dashboard count and export agree", "receipt facts == dashboard Live memories == export facts (keys and texts)",
               {"receipt": len(expected_keys), "dashboard": live_pos, "export": len(exported), "texts_equal": exported == expected_keys},
               exported == expected_keys and live_pos == len(expected_keys) == len(pos_r["id_map"]))
    checks.add("empty-case account holds ZERO current facts by the dashboard and the export", "Live memories 0 and export facts []",
               {"dashboard": svc.live_memories(emp)["used"], "export": len(emp_export["facts"])},
               svc.live_memories(emp)["used"] == 0 and emp_export["facts"] == [])

    # ---- the canonical store on disk: expiry lapsed, forget recorded ----
    records = read_canonical_records(svc.data_dir / "brains" / emp.brain_id)
    facts_on_disk = [r for r in records if r["record_type"] == "memory_fact"]
    expiries_on_disk = [r for r in records if r["record_type"] == "memory_expiry"]
    expire_keys = {seeding.operation_key(corpus_sha256, ft.target_id(c)) for c in ft.cases_with_mode(ft.MODE_EXPIRE)}
    forget_ids = {x["remember_id"] for x in emp_r["seeded"] if x["corpus_fact_id"] in {ft.target_id(c) for c in ft.cases_with_mode(ft.MODE_FORGET)}}
    ttl_records = [r for r in facts_on_disk if r.get("valid_until")]
    checks.add("canonical store: every target still exists on disk; the TTL ones carry the fixed instant; only the forgotten ones have expiry records",
               "5 memory_fact records; the expire-mode ones carry valid_until == the receipt's instant; 3 memory_expiry records for exactly the forget-mode ids",
               {"facts_on_disk": len(facts_on_disk), "with_valid_until": len(ttl_records), "expiry_records": len(expiries_on_disk)},
               len(facts_on_disk) == 5
               and {r["operation_key"] for r in ttl_records} == expire_keys
               and all(abs(seeding.parse_iso_utc(r["valid_until"]) - emp_r["expiry"]["valid_until_epoch"]) < 1.0 for r in ttl_records)
               and {r["target_sha256"] for r in expiries_on_disk} == forget_ids)

    # ---- the server clock, not a fixture's ----
    probed = emp_r["expiry"]["probed_after_valid_until_seconds"]
    skew_worst = max(abs(skew_before), abs(skew_after))
    report["server_clock"] = {"skew_before_seconds": skew_before, "skew_after_seconds": skew_after,
                              "resolution_seconds": DATE_HEADER_RESOLUTION_SECONDS, "probed_after_valid_until_seconds": probed}
    checks.add("the expiry was crossed on the SERVICE's clock", "probe was past valid_until by more than the clock skew plus the Date header's resolution",
               {"probed_after_valid_until_seconds": probed, "worst_skew_seconds": skew_worst},
               probed - skew_worst - DATE_HEADER_RESOLUTION_SECONDS > 0 and emp_r["expiry_check"]["expired_absent_from_inventory"] is True
               and emp_r["expiry_check"]["forget_targets_still_present"] is True)
    checks.add("the wait was a real one at the frozen or declared constants", "valid_until - remember time == ttl; probe >= margin past it",
               {"ttl_seconds": emp_r["expiry"]["ttl_seconds"], "margin_seconds": emp_r["expiry"]["margin_seconds"], "production": constants["production_constants"]},
               emp_r["expiry"]["ttl_seconds"] == constants["ttl_seconds"] and probed >= constants["margin_seconds"])

    # ---- a second seed is refused: the accounts are no longer empty ----
    window.wait("the reuse-refusal seed")
    calls_before = ledger_lib.parse_ledger((evaldir / "ledger.jsonl").read_text()).calls
    live_counts = (svc.live_memories(pos)["used"], svc.live_memories(emp)["used"])
    again, again_code = ev.run_seed(cli("seed", "seed-again.json", "--receipt-dir", str(evaldir / "receipts-again")))
    calls_after = ledger_lib.parse_ledger((evaldir / "ledger.jsonl").read_text()).calls
    after_counts = (svc.live_memories(pos)["used"], svc.live_memories(emp)["used"])
    same_dir, same_code = ev.run_seed(cli("seed", "seed-same.json", "--receipt-dir", str(rdir)))
    calls_same = ledger_lib.parse_ledger((evaldir / "ledger.jsonl").read_text()).calls
    report["reuse_refusal"] = {
        "second_seed": {"status": again["status"], "exit": again_code, "blocker": (again.get("blockers") or [{}])[0].get("required_input"),
                        "ledger_calls_spent": calls_after - calls_before, "accounts_unchanged": live_counts == after_counts},
        "same_receipt_dir": {"status": same_dir["status"], "exit": same_code, "ledger_calls_spent": calls_same - calls_after},
    }
    checks.add("reusing the seeded accounts is refused before anything is written", "second seed BLOCKED after the empty-account proof, accounts unchanged; same receipt dir BLOCKED with 0 calls",
               report["reuse_refusal"], again["status"] == "BLOCKED" and again_code == ev.EXIT_BLOCKED and live_counts == after_counts
               and 0 < calls_after - calls_before <= 6 and same_dir["status"] == "BLOCKED" and calls_same == calls_after)

    # ---- a live run over the same accounts: fixture, PARTIAL, whatever the score ----
    live_manifest = json.loads(manifest_path.read_text())
    for role, key in ev.TARGETS:
        live_manifest[key]["seed_receipt_path"] = seed_result["seed_receipts"][role]["path"]
        live_manifest[key]["corpus_seeded_confirmation"] = seed_result["seed_receipts"][role]["confirmation"]
    live_path = evaldir / "live-manifest.json"
    live_path.write_text(json.dumps(live_manifest))
    window.mark()  # the refusal sent a handful of requests; the live phase needs headroom for ~100 on the positive account
    live_args = ev.parse_args(["--live", "--manifest", str(live_path), "--output", str(evaldir / "live.json")])
    live_result, live_code = ev.run_live(live_args, lexical_provider=lambda: _stub_lexical_control(corpus))
    window.mark()
    positives = live_result["per_query"]["positive"] if "per_query" in live_result else []
    hit = sum(1 for r in positives if r["hit"])
    quality = next((r for r in live_result["acceptance"] if r["criterion"].startswith("Hit@5")), {})
    report["live"] = {
        "status": live_result["status"], "exit": live_code, "evidence_level": live_result["evidence_level"],
        "positive_hits": f"{hit}/{len(positives)}", "cases_executed": live_result["cases"]["executed"],
        "quality_row": quality.get("status"),
        "usage": {k: v for k, v in live_result.get("usage", {}).items() if k != "ledger"},
        "blockers": live_result.get("blockers", [])[:3],
    }
    checks.add("a live run over the real service is a fixture PARTIAL even when every ranking is correct", "status PARTIAL, evidence_level fixture, quality row NOT_RUN, 95/95 hits (the oracle is the answer key)",
               report["live"], live_result["status"] == "PARTIAL" and live_result["evidence_level"] == "fixture"
               and quality.get("status") == "NOT_RUN" and hit == len(positives) == 95 and live_result["cases"]["executed"] == 100)

    # ---- isolation control: a fact written to one account is invisible to the other ----
    window.wait("the isolation control")
    waits: list[float] = []
    isolation = MCPClient(f"{svc.origin}/mcp", emp.token, [svc.origin])
    with_rate_limit_retry(isolation.initialize, waits)
    written = with_rate_limit_retry(
        lambda: isolation.call_tool("remember", {"fact": ISOLATION_MARKER, "provenance": "local fixture isolation control", "operation_key": "t2343-isolation-marker"}),
        waits,
    )
    positive_client = MCPClient(f"{svc.origin}/mcp", pos.token, [svc.origin])
    with_rate_limit_retry(positive_client.initialize, waits)
    from_positive = with_rate_limit_retry(lambda: positive_client.call_tool("recall", {"query": ISOLATION_MARKER, "limit": 5}), waits)
    from_empty = with_rate_limit_retry(lambda: isolation.call_tool("recall", {"query": ft.SENTINEL_FACT_TEXT, "limit": 5}), waits)
    report["rate_limit"] = {
        "gateway_limit": "120 requests per minute per account (internal/hosted/gateway)",
        "waits_between_phases": window.waits,
        "retried_after_429_seconds": sum(waits),
        "note": "a full-speed harness phase against one account uses most of the per-account limit; back-to-back phases see HTTP 429",
    }
    marker_id = written.get("id")
    sentinel_id = next(k for k, v in pos_r["id_map"].items() if v == ft.SENTINEL_FACT_ID)
    positive_ids = {r.get("slug") for r in from_positive.get("results", [])} | {f.get("fact_id") for f in from_positive.get("facts", [])}
    # A vector search returns the nearest facts it has, so the empty account (which now holds the marker) answers the
    # sentinel query with the marker. What matters is that the SENTINEL is not among them.
    empty_hits_for_sentinel = [x for x in [*from_empty.get("results", []), *from_empty.get("facts", [])] if sentinel_id in json.dumps(x)]
    exports_after = (svc.export_facts(pos)["facts"], svc.export_facts(emp)["facts"])
    report["isolation"] = {
        "marker_written_to": "empty-case account", "marker_status": written.get("status"),
        "positive_account_sees_marker": any(marker_id and marker_id in str(x) for x in positive_ids),
        "empty_account_reaches_sentinel": bool(empty_hits_for_sentinel),
        "dashboard": {"positive": svc.live_memories(pos)["used"], "empty_case": svc.live_memories(emp)["used"]},
    }
    checks.add("isolation: neither account can reach the other's facts, and only the owner's export holds them", "positive never sees the marker; empty never reaches the sentinel; exports hold only their own facts",
               report["isolation"], written.get("status") == "inserted" and not report["isolation"]["positive_account_sees_marker"]
               and not empty_hits_for_sentinel and len(exports_after[0]) == len(expected_keys)
               and [f["operation_key"] for f in exports_after[1]] == ["t2343-isolation-marker"]
               and ISOLATION_MARKER not in json.dumps(exports_after[0]))


# --------------------------------------------------------------------------
# Command line
# --------------------------------------------------------------------------
def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=(__doc__ or "").split("\n\n")[0])
    parser.add_argument("--serenity-bin", default=None, help=f"the serenity binary (or ${BIN_ENV})")
    parser.add_argument("--output", required=True, help="where to write the redacted JSON report; an existing file is refused")
    parser.add_argument("--parent", default=None, help=f"parent for the owned directory (default {DEFAULT_PARENT}; keep it short)")
    parser.add_argument("--allow-dirty-tree", action="store_true", help="development only: run on an uncommitted tree; the report says so")
    parser.add_argument("--smoke-ttl-seconds", type=int, default=None, help="SHORTENED TTL for a smoke run; the report is marked not qualifying")
    parser.add_argument("--smoke-margin-seconds", type=int, default=None, help="SHORTENED margin for a smoke run; the report is marked not qualifying")
    args = parser.parse_args(argv)
    if os.environ.get(OPT_IN_ENV) != "1":
        print(f"local_service_fixture: refused; set {OPT_IN_ENV}=1 to run the actual service on loopback (several minutes with the production TTL)", file=sys.stderr)
        return 2
    binary = args.serenity_bin or os.environ.get(BIN_ENV, "")
    output = Path(args.output)
    if os.path.lexists(output):
        print("local_service_fixture: refused; --output already exists", file=sys.stderr)
        return 2
    try:
        report = run_verification(
            binary, parent=Path(args.parent) if args.parent else None, allow_dirty_tree=args.allow_dirty_tree,
            ttl_seconds=args.smoke_ttl_seconds, margin_seconds=args.smoke_margin_seconds,
        )
    except FixtureRefused as e:
        print(f"local_service_fixture: refused: {e}", file=sys.stderr)
        return 2
    except Exception as e:  # noqa: BLE001 -- the class name only; nothing else is known to be free of secrets
        print(f"local_service_fixture: failed with {type(e).__name__}; the owned service was stopped by its exact PID; "
              f"check the parent directory for a leftover {OWNED_PREFIX}* directory", file=sys.stderr)
        return 2
    fd = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o644)
    with os.fdopen(fd, "w") as f:
        f.write(json.dumps(report, indent=2, sort_keys=True) + "\n")
    failed = [c["name"] for c in report["checks"] if not c["passed"]]
    print(f"local_service_fixture: status={report['status']} checks={len(report['checks'])} failed={len(failed)} written to {output}")
    for name in failed:
        print(f"  FAILED: {name}", file=sys.stderr)
    return 0 if not failed else 1


if __name__ == "__main__":
    raise SystemExit(main())
