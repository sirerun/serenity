# Compute review options — unapproved, 2026-09-19

These are Linux on-demand compute line items in Oregon, at 730 hours per month, without purchase commitments. They are not quotes for a complete hosted service and do not establish that a machine can meet the frozen workload. No template or deployment has changed.

| Instance | vCPU | Memory | Hourly USD | 730-hour base USD | CPU-credit assumption |
| --- | ---: | --- | ---: | ---: | --- |
| c6g.large | 2 | 4 GiB | 0.068 | 49.64 | No burst-credit surcharge |
| c6g.medium | 1 | 2 GiB | 0.034 | 24.82 | No burst-credit surcharge |
| c7g.medium | 1 | 2 GiB | 0.0363 | 26.50 | No burst-credit surcharge |
| c8g.medium | 1 | 2 GiB | 0.03988 | 29.11 | No burst-credit surcharge |
| t4g.small | 2 | 2 GiB | 0.0168 | 12.26 | Credit charges additional in Unlimited mode |

For t4g.small, the two vCPUs have a 20% baseline. At a steady 70% mean CPU with credits already depleted, the illustrative surplus is `2 × (0.70 − 0.20) × 730 × $0.04 = $29.20/month`; at 100%, it is `$46.72/month`. Add the base $12.264 compute charge. Initial earned credits can reduce a particular period's charge; they do not establish a sustained-cost bound. The template does not explicitly select credit mode, and the actual account configuration was not queried. Switching to Standard would trade charges for throttling and is not an approved workaround.

A one-vCPU compute instance could reduce sustained compute expense, but it also changes the capacity envelope. The existing macOS synthetic measurements are not an EC2 comparison. Test the same workload and full cardinalities, preserve the CPU/RSS and admission thresholds, and measure the target architecture before adopting any alternative. A two-vCPU/4-GiB machine offers a different memory envelope at a higher fixed base. Current full-snapshot retention remains the largest modeled cost regardless of these instance choices.

The review needs both an accepted backup policy and measured runtime behavior. This table does not recommend weakening either to fit the historical budget. Provider, disk, backup, network, key rotation, logs, control-history growth and shared-account allowances still belong in the total.

Rates: [AWS regional price map](https://b0.p.awsstatic.com/pricing/2.0/meteredUnitMaps/ec2/USD/current/ec2-ondemand-without-sec-sel/US%20West%20(Oregon)/Linux/index.json), captured source SHA-256 `858d2bdd6067c75ef9ae994b9640ab0153507723a676f161e165f364b603bb6a`. CPU credit rate from the versioned regional AWS EC2 offer extract `ec2-ebs-oregon-rate-extract.json`; baseline behavior from [AWS burstable credit documentation](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/burstable-credits-baseline-concepts.html).
