# Contributing

Thanks for looking. Issues and pull requests are both welcome.

## Running the tests

Neither suite needs an API key or a network. Both run the decision engine against recorded
answers, which is also how you should test your own changes.

```bash
pip install -e './python[dev]'
cd python && python3 -m pytest -q

cd ts && npm install && npm test
```

## Changing the policy

`policies/standard-v1.json` is the source of truth, and both packages ship a copy of it. After any
edit:

```bash
./scripts/sync-policies.sh
```

CI fails if you forget, because two languages enforcing different policies is the worst kind of
bug here: it only shows up in production, on one side.

Remember that the descriptions in the pack are not documentation. They are the text sent to Jev as
question criteria, so changing a description changes behaviour. Say what the content *does*, not
what it is *about*.

## What a good pull request looks like

**A change to the decision engine comes with a test.** Add recorded answers to the fixtures and
assert the verdict. Every rule, threshold band and route in the policy should have at least one
case that would fail without it.

**A new hazard category cites a standard.** The taxonomy is drawn from MLCommons AILuminate, Llama
Guard and the OWASP LLM Top 10 rather than invented, so that a verdict maps back to something an
auditor recognises. A category with no `refs` needs a reason.

**Threshold changes come with evidence.** Numbers tuned against a labelled set beat numbers that
feel right. `scripts/sweep.py` replays a recorded calibration offline; include the before and
after.

**Both languages stay in step.** The Python and TypeScript engines are ports of each other. A
change to `decide` in one needs the same change and the same test in the other. `tuning.py` is the
exception: it is offline analysis tooling and lives in Python only.

## Adding labelled cases

`examples/` holds the labelled sets. Each case needs an `expected_action` and, where a specific
hazard is being exercised, an `expected_category`. The second one matters: without it the
separation analysis counts every non-allow case against every category and reports overlap
everywhere.

Cases in languages that are not yet represented are especially useful. So are cases that are
*close* to a violation without being one, because that is where thresholds actually get decided.

## Security-sensitive changes

If a change would let something through that the policy says it should not, say so in the pull
request rather than only in the code. See [SECURITY.md](SECURITY.md) for reporting a vulnerability
privately.

## Style

Code is formatted as it already appears; there is no formatter to run. Comments explain why
something is the way it is, not what the line does. No em-dashes in prose.
