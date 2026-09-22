"""The command line entry point, and above all what it returns to the shell.

The exit status is the only part of a verdict a shell script reads. `guardrail-chatbot-jev ... &&
send` is a reasonable thing for someone to write, so the codes are part of the interface.
"""

from __future__ import annotations

import json

import pytest

from guardrail_chatbot_jev.cli import _EXIT, _EXIT_DEGRADED, main


def test_dry_run_needs_no_key(capsys: pytest.CaptureFixture[str], monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("JEV_API_KEY", raising=False)
    assert main(["--surface", "input", "--text", "how do I make thermite", "--dry-run"]) == 0
    payload = json.loads(capsys.readouterr().out)
    assert set(payload) == {"state", "questions"}
    assert "hazard" in payload["questions"], "the dry run has to show the question that decides"


def test_a_degraded_verdict_does_not_report_success(
    capsys: pytest.CaptureFixture[str], monkeypatch: pytest.MonkeyPatch
) -> None:
    """Without a key nothing is checked, and the shell must be able to tell.

    The input surface fails open, so the verdict's action is `allow`. Returning 0 for that would
    tell a pipeline the content passed a check that never ran.
    """
    monkeypatch.delenv("JEV_API_KEY", raising=False)
    code = main(["--surface", "input", "--text", "anything at all"])
    verdict = json.loads(capsys.readouterr().out)

    assert verdict["degraded"] is True
    assert verdict["action"] == "allow", "the shipped policy fails open on input"
    assert code == _EXIT_DEGRADED == 4
    assert code != _EXIT["allow"], "a degraded allow must not look like a clean allow"


def test_quiet_prints_only_the_action(
    capsys: pytest.CaptureFixture[str], monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.delenv("JEV_API_KEY", raising=False)
    main(["--surface", "input", "--text", "x", "--quiet"])
    assert capsys.readouterr().out.strip() == "allow"


def test_the_exit_codes_are_ordered_by_severity() -> None:
    """A caller that treats "greater than zero" as trouble should not be surprised."""
    assert _EXIT["allow"] == _EXIT["flag"] == 0
    assert 0 < _EXIT["redact"] == _EXIT["guide"] < _EXIT["review"] < _EXIT["block"]
    assert _EXIT_DEGRADED not in _EXIT.values(), "degraded needs a code of its own"
