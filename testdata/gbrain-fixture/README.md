# Synthetic gbrain migration fixture

This fixture contains invented people, projects, and claims. It has no personal
or customer data. Its markdown formats follow gbrain's facts/takes fence parsers
at `dndungu/gbrain@d35c9c9e441e`, credited to the upstream gbrain project
(https://github.com/garrytan/gbrain). Serenity's mapping is specified in ADR 009.

Coverage: every baseline facts/takes column, both visibility values, all five
fact kinds, supersession and forgotten rows, repeated row numbers across the
independent facts/takes fences, a take date range, aliases, external IDs,
timeline entries, wiki links, escaped pipes, and literal backslashes.

The standalone public fixture is
https://github.com/sirerun/serenity-gbrain-fixture. `PIN` records the full commit
used by CI. Its two entity pages must stay byte-identical to these local pages;
CI verifies that membership before running all eight row expectations against
the independently cloned fixture. Updating a page requires publishing and
reviewing a new fixture commit, then updating this pin and the local copy together.
