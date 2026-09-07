# BrainBench trend

[BrainBench](https://github.com/dndungu/gbrain) is an external, independently
authored memory-agent benchmark, vendored verbatim at a pinned commit under
`evals/brainbench/` (`evals/brainbench/PIN`, plan T1.21). Serenity's adapter
(`internal/eval/brainbench`) scores the one part of it Serenity's current
surface can honestly measure: for every gold conversation turn marked
`should_retrieve: true`, does `serenity search` rank the right page(s)? See
[`evals/brainbench/SERENITY-ADAPTER.md`](https://github.com/sirerun/serenity/blob/main/evals/brainbench/SERENITY-ADAPTER.md)
for the full scope disclosure -- what's scored, what's skipped, and why.

Every push runs one scoring pass with zero network calls (the adapter's
search call always passes a nil embedder, T1.21) and uploads the result as a
build artifact. Plan T5.10 persists a row from that pass every night to a
durable, append-only file on the orphan
[`results/brainbench-trend`](https://github.com/sirerun/serenity/tree/results/brainbench-trend)
branch -- never on `main` -- so precision, recall, and F1 can be tracked over
time instead of only per push. That nightly job enforces a hard USD cap and
refuses to publish a row on a cache miss (a vendored-corpus failure that
would otherwise score as a meaningless 0/0/0) rather than silently
publishing a bad or falsely-billed point; see
`internal/eval/brainbench/trend.go` and `evals/brainbench/publish_trend.go`
for the enforcement.

<div id="brainbench-trend-chart" data-source="https://raw.githubusercontent.com/sirerun/serenity/results/brainbench-trend/evals/brainbench-trend.json">
  <p><em>Loading the trend from the results branch…</em></p>
</div>
<script>
(function () {
  var container = document.getElementById("brainbench-trend-chart");
  if (!container) return;
  var src = container.getAttribute("data-source");

  function escapeHTML(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  }

  function fallback(message) {
    container.innerHTML =
      "<p>" + escapeHTML(message) + " See the raw trend file: " +
      '<a href="' + src + '">' + escapeHTML(src) + "</a></p>";
  }

  function renderChart(rows) {
    if (!rows.length) {
      fallback("The trend has no rows yet.");
      return;
    }

    var width = 640, height = 260, padding = 36;
    var innerW = width - padding * 2, innerH = height - padding * 2;
    var series = [
      { key: "precision", label: "Precision", color: "#2563eb" },
      { key: "recall", label: "Recall", color: "#16a34a" },
      { key: "f1", label: "F1", color: "#d97706" }
    ];

    function xAt(i) {
      return rows.length === 1 ? padding + innerW / 2 : padding + (innerW * i) / (rows.length - 1);
    }
    function yAt(v) {
      return padding + innerH * (1 - Math.max(0, Math.min(1, v)));
    }

    var svg = '<svg viewBox="0 0 ' + width + " " + height + '" role="img" ' +
      'aria-label="BrainBench precision, recall, and F1 over ' + rows.length + ' runs" ' +
      'data-points="' + rows.length + '" xmlns="http://www.w3.org/2000/svg">';

    // Axis lines and gridlines at 0.0/0.5/1.0.
    svg += '<line x1="' + padding + '" y1="' + (height - padding) + '" x2="' + (width - padding) +
      '" y2="' + (height - padding) + '" stroke="currentColor" stroke-opacity="0.3"/>';
    [0, 0.5, 1].forEach(function (v) {
      var y = yAt(v);
      svg += '<line x1="' + padding + '" y1="' + y + '" x2="' + (width - padding) + '" y2="' + y +
        '" stroke="currentColor" stroke-opacity="0.12"/>';
      svg += '<text x="4" y="' + (y + 4) + '" font-size="10" fill="currentColor">' + v.toFixed(1) + "</text>";
    });

    series.forEach(function (s) {
      var pts = rows.map(function (r, i) {
        var v = (r.overall && typeof r.overall[s.key] === "number") ? r.overall[s.key] : 0;
        return xAt(i) + "," + yAt(v);
      });
      svg += '<polyline fill="none" stroke="' + s.color + '" stroke-width="2" points="' + pts.join(" ") + '"/>';
      rows.forEach(function (r, i) {
        var v = (r.overall && typeof r.overall[s.key] === "number") ? r.overall[s.key] : 0;
        svg += '<circle cx="' + xAt(i) + '" cy="' + yAt(v) + '" r="3" fill="' + s.color + '">' +
          "<title>" + escapeHTML(s.label + " = " + v.toFixed(3) + " (" + (r.ts || "?") + ", " + (r.commit || "?").slice(0, 8) + ")") + "</title></circle>";
      });
    });

    var legend = series.map(function (s) {
      return '<span style="color:' + s.color + '">&#9679;</span> ' + escapeHTML(s.label);
    }).join(" &nbsp; ");

    svg += "</svg>";
    container.innerHTML = svg + '<p class="brainbench-trend-legend">' + legend +
      " &nbsp; (" + rows.length + (rows.length === 1 ? " point" : " points") + ", latest " +
      escapeHTML((rows[rows.length - 1].commit || "unknown").slice(0, 8)) + ")</p>";
  }

  fetch(src)
    .then(function (resp) {
      if (!resp.ok) throw new Error("HTTP " + resp.status);
      return resp.json();
    })
    .then(renderChart)
    .catch(function (err) {
      fallback("Could not load the live trend (" + err.message + ").");
    });
})();
</script>

Each point is one nightly (or manually dispatched) run: `overall.precision`,
`overall.recall`, and `overall.f1` from `internal/eval/brainbench.Report`,
scored against the same pinned vendored corpus every time. A flat line
across many nights is expected and correct -- the corpus and the scored code
path are both deterministic; the trend moves only when the vendored
`PIN` advances or `internal/search`/`internal/eval/brainbench` itself
changes.
