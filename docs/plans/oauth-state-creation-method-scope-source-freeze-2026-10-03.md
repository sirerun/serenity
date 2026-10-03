# OAuth shared state budget method scope source freeze

Freeze: `oauth-state-method-scope-v1`.
Exact author contract `6bfe7fcaa93ef4907b27693cd2c58d743bc84376` independently CLEAR; integrated contract bytes identical.
Narrow resource claim `R-oauth-state-method-scope` held by coordinator at `e2ee9bfd969930c062689fffaeed12e73a3ba5da` until independently qualified landed proof.
Source ownership: existing `internal/hosted/oauth/hosted.go`, `ratelimit.go`, `ratelimit_test.go` and one new owned implementation receipt only. No dependency/module/provider/service/identity change.
Charge shared state budget only POST register and GET|POST authorize, without changing downstream response routing or other limits. Pinned b90 accepts GET authorize only; POST remains contractually metered and still405. HEAD remains excluded. Keep5000/minute capacity policy and live acceptance open.
