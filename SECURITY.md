# Security policy

## Scope

This project decides whether chatbot content should be delivered. Two kinds of problem matter, and
they are not the same.

**A bypass** is content that the policy says should be held and that this code delivers anyway: a
category that fails to fire, a rule that softens something it should not, a prefilter that can be
evaded, a streaming path that releases a chunk before its check. Report these privately.

**A false positive** is ordinary content that gets blocked. Please open a normal issue; it is a
quality problem, not a vulnerability, and discussing it in the open helps everyone.

Note what is *not* in scope. This library does not enforce anything: it returns a verdict and your
deployment acts on it. A deployment that ignores `degraded: true`, or that treats `review` as
`allow`, has a bug in its own code. Jev's own behaviour belongs to whoever serves the model.

## Reporting

Use GitHub's private vulnerability reporting on this repository ("Security" tab, "Report a
vulnerability"). That keeps the details out of public view until there is a fix.

Please include the surface (`input`, `output` or `conversation`), the content or a description of
it, the verdict you got, the verdict you expected, and the policy version from `policy_id`. If you
have the raw Jev answers from a `RecordingTransport`, include them: they make the problem
reproducible without an API key.

Do not include a real API key, real personal data, or real content from someone else's
conversation. A constructed example that shows the same thing is better in every way.

## What to expect

An acknowledgement within a week, and an assessment of whether it reproduces.

A confirmed bypass is fixed with a test that would have caught it, and the fix goes into both the
Python and TypeScript engines. Credit in the release notes if you want it.

## A word on thresholds

The thresholds shipped here are a starting point, derived from how a wide choice question
distributes probability, not from measurement against labelled data. A category that misses
something at the default threshold may be working exactly as designed and simply mis-tuned for
your deployment.

That is not a reason to stay quiet. Tell us, with the raw answers if you have them, and it will be
clear from the probabilities whether the fix is a number or a category description.
