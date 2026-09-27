#!/usr/bin/env python3
"""Optional, bounded Jev routing advice for the engineering orchestrator.

The adapter is deliberately fail-open and shadow-only.  It never executes a
selection, changes host policy, or turns a provider failure into a failed run.
Input is one JSON object on stdin; output is one typed JSON object on stdout.
"""

from __future__ import annotations

import json
import math
import os
import signal
import sys
import threading
import time
import http.client
import urllib.error
import urllib.request
from contextlib import contextmanager
from typing import Any


API_URL = "https://api.typesafe.ai/v1/systemone"
MODEL = "jev-latest"
TIMEOUT_SECONDS = 10.0
MIN_CONFIDENCE = 0.60
# Decimal probability sums may differ by harmless serialization rounding only.
PROBABILITY_SUM_TOLERANCE = 1e-6

MAX_REQUEST_BYTES = 64 * 1024
MAX_INPUT_BYTES = 64 * 1024
MAX_RESPONSE_BYTES = 256 * 1024
MAX_GOAL_CHARS = 8_000
MAX_CONTEXT_BYTES = 32 * 1024
MAX_CANDIDATES_PER_DIMENSION = 32
MAX_CANDIDATE_ID_CHARS = 128
MAX_DESCRIPTION_CHARS = 1_024
MAX_API_KEY_CHARS = 512
READ_CHUNK_BYTES = 16 * 1024

DIMENSIONS = {
    "tools": "tool",
    "agents": "agent",
    "models": "model",
    "efforts": "effort",
}
ALLOWED_INPUT_FIELDS = frozenset({"goal", "context", *DIMENSIONS})
LOCAL_INPUT_REASONS = frozenset({
    "input_not_json",
    "invalid_api_key",
    "invalid_candidates",
    "invalid_goal",
    "invalid_input",
    "input_too_large",
    "no_candidates",
    "secret_in_input",
    "unknown_input_field",
})
_FROM_ENV = object()


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    """Prevent urllib from replaying the credential to another URL."""

    def redirect_request(self, req: Any, fp: Any, code: int, msg: str,
                         headers: Any, newurl: str) -> None:
        return None


class _InputError(ValueError):
    pass


class _ResponseError(ValueError):
    pass


class _DeadlineExceeded(TimeoutError):
    pass


class _DeadlineUnavailable(RuntimeError):
    pass


def _base_result(*, enabled: bool, status: str, reason: str) -> dict[str, Any]:
    return {
        "version": 1,
        "enabled": enabled,
        "advisory": True,
        "mode": "shadow",
        "status": status,
        "reason": reason,
        "selections": {},
    }


def _disabled_result() -> dict[str, Any]:
    return _base_result(enabled=False, status="disabled", reason="api_key_missing")


def _fallback_result(reason: str) -> dict[str, Any]:
    return _base_result(enabled=True, status="fallback", reason=reason)


def _normalize_api_key(value: Any) -> str | None:
    if value is None or (isinstance(value, str) and not value.strip()):
        return None
    if (not isinstance(value, str)
            or len(value) > MAX_API_KEY_CHARS
            or any(not 33 <= ord(character) <= 126 for character in value)):
        raise _InputError("invalid_api_key")
    return value


def _compact_json(value: Any) -> bytes:
    try:
        return json.dumps(
            value, ensure_ascii=False, allow_nan=False, separators=(",", ":")
        ).encode("utf-8")
    except (TypeError, ValueError, UnicodeError, RecursionError) as exc:
        raise _InputError("input_not_json") from exc


def _contains_secret(value: Any, secret: str) -> bool:
    """Check semantic input strings, before JSON escaping can hide a secret."""
    pending = [value]
    while pending:
        item = pending.pop()
        if isinstance(item, str):
            if secret in item:
                return True
        elif isinstance(item, dict):
            pending.extend(item.keys())
            pending.extend(item.values())
        elif isinstance(item, (list, tuple)):
            pending.extend(item)
    return False


def _validate_candidate_list(field: str, raw: Any) -> list[dict[str, str]]:
    if raw is None:
        return []
    if not isinstance(raw, list):
        raise _InputError("invalid_candidates")
    if len(raw) > MAX_CANDIDATES_PER_DIMENSION:
        raise _InputError("input_too_large")

    candidates: list[dict[str, str]] = []
    seen: set[str] = set()
    for item in raw:
        if not isinstance(item, dict) or set(item) != {"id", "description"}:
            raise _InputError("invalid_candidates")
        candidate_id = item["id"]
        description = item["description"]
        if not isinstance(candidate_id, str) or not candidate_id.strip():
            raise _InputError("invalid_candidates")
        if not isinstance(description, str) or not description.strip():
            raise _InputError("invalid_candidates")
        if (len(candidate_id) > MAX_CANDIDATE_ID_CHARS
                or len(description) > MAX_DESCRIPTION_CHARS):
            raise _InputError("input_too_large")
        if candidate_id in seen:
            raise _InputError("invalid_candidates")
        seen.add(candidate_id)
        candidates.append({"id": candidate_id, "description": description})
    return candidates


def _build_request_payload(
    caller_input: Any,
) -> tuple[bytes, dict[str, list[str]]]:
    if not isinstance(caller_input, dict):
        raise _InputError("invalid_input")
    if not set(caller_input).issubset(ALLOWED_INPUT_FIELDS):
        raise _InputError("unknown_input_field")

    goal = caller_input.get("goal")
    if not isinstance(goal, str) or not goal.strip():
        raise _InputError("invalid_goal")
    if len(goal) > MAX_GOAL_CHARS:
        raise _InputError("input_too_large")

    context = caller_input.get("context", {})
    if len(_compact_json(context)) > MAX_CONTEXT_BYTES:
        raise _InputError("input_too_large")

    questions: dict[str, Any] = {}
    candidate_ids: dict[str, list[str]] = {}
    for input_field, answer_key in DIMENSIONS.items():
        candidates = _validate_candidate_list(input_field, caller_input.get(input_field))
        if not candidates:
            continue
        candidate_ids[answer_key] = [candidate["id"] for candidate in candidates]
        criteria = {
            f"candidate_{index}": candidate["description"]
            for index, candidate in enumerate(candidates)
        }
        criteria["no_match"] = f"None of the supplied {answer_key} candidates fit."
        questions[answer_key] = {
            "type": "choice",
            "instructions": (
                f"Choose the supplied {answer_key} candidate that best fits the "
                "routing goal and context. Choose no_match when none fit."
            ),
            "criteria": criteria,
        }

    if not questions:
        raise _InputError("no_candidates")

    request_payload = {
        "state": {"goal": goal, "context": context},
        "model": MODEL,
        "questions": questions,
    }
    encoded = _compact_json(request_payload)
    if len(encoded) > MAX_REQUEST_BYTES:
        raise _InputError("input_too_large")
    return encoded, candidate_ids


def _parse_response(raw: bytes, candidate_ids: dict[str, list[str]]) -> dict[str, Any]:
    try:
        payload = json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, ValueError, RecursionError) as exc:
        raise _ResponseError("invalid_response") from exc
    if not isinstance(payload, dict) or not isinstance(payload.get("answers"), dict):
        raise _ResponseError("invalid_response")

    answers = payload["answers"]
    if set(answers) != set(candidate_ids):
        raise _ResponseError("invalid_response")

    selections: dict[str, Any] = {}
    for dimension, caller_ids in candidate_ids.items():
        answer = answers.get(dimension)
        if not isinstance(answer, dict) or answer.get("type") != "choice":
            raise _ResponseError("invalid_response")
        choice = answer.get("choice")
        confidence = answer.get("confidence")
        probabilities = answer.get("probabilities")
        if (not isinstance(choice, str)
                or isinstance(confidence, bool)
                or not isinstance(confidence, (int, float))
                or not math.isfinite(float(confidence))
                or not 0.0 <= float(confidence) <= 1.0):
            raise _ResponseError("invalid_response")

        expected_probability_keys = {
            *(f"candidate_{index}" for index in range(len(caller_ids))),
            "no_match",
        }
        if not isinstance(probabilities, dict) or set(probabilities) != expected_probability_keys:
            raise _ResponseError("invalid_response")
        probability_values: list[float] = []
        for probability in probabilities.values():
            if (isinstance(probability, bool)
                    or not isinstance(probability, (int, float))
                    or not math.isfinite(float(probability))
                    or not 0.0 <= float(probability) <= 1.0):
                raise _ResponseError("invalid_response")
            probability_values.append(float(probability))
        if not math.isclose(
            math.fsum(probability_values), 1.0,
            rel_tol=0.0, abs_tol=PROBABILITY_SUM_TOLERANCE,
        ):
            raise _ResponseError("invalid_response")

        confidence = float(confidence)
        if choice == "no_match":
            selections[dimension] = {
                "candidate": None,
                "confidence": confidence,
                "accepted": False,
                "reason": "no_match",
            }
            continue
        try:
            index = int(choice.removeprefix("candidate_"))
        except ValueError as exc:
            raise _ResponseError("invalid_response") from exc
        if index < 0 or index >= len(caller_ids) or choice != f"candidate_{index}":
            raise _ResponseError("invalid_response")
        accepted = confidence >= MIN_CONFIDENCE
        selections[dimension] = {
            "candidate": caller_ids[index],
            "confidence": confidence,
            "accepted": accepted,
            "reason": "accepted" if accepted else "low_confidence",
        }

    rejection_reasons = {
        selection["reason"] for selection in selections.values()
        if not selection["accepted"]
    }
    atomic_fallback = bool(rejection_reasons)
    if atomic_fallback:
        for selection in selections.values():
            if selection["accepted"]:
                selection["accepted"] = False
                selection["reason"] = "atomic_fallback"
    if "low_confidence" in rejection_reasons:
        fallback_reason = "low_confidence"
    elif "no_match" in rejection_reasons:
        fallback_reason = "no_match"
    else:
        fallback_reason = "advice_available"
    result = _base_result(
        enabled=True,
        status="fallback" if atomic_fallback else "ok",
        reason=fallback_reason,
    )
    result["selections"] = selections
    return result


def _remaining(deadline: float) -> float:
    remaining = deadline - time.monotonic()
    if remaining <= 0:
        raise _DeadlineExceeded()
    return remaining


@contextmanager
def _provider_deadline(seconds: float):
    """Interrupt the complete provider operation or refuse to start it.

    POSIX ``ITIMER_REAL`` interrupts blocking DNS/connect/write/read work in the
    calling main thread.  We never move the secret-bearing request to a worker.
    An existing real-time timer is left untouched and makes the optional route
    fail open before network rather than corrupting another subsystem's timer.
    """
    required = ("SIGALRM", "ITIMER_REAL", "setitimer", "getitimer")
    if (threading.current_thread() is not threading.main_thread()
            or any(not hasattr(signal, name) for name in required)):
        raise _DeadlineUnavailable()

    try:
        previous_timer = signal.getitimer(signal.ITIMER_REAL)
        previous_handler = signal.getsignal(signal.SIGALRM)
    except (OSError, ValueError) as exc:
        raise _DeadlineUnavailable() from exc
    if previous_timer != (0.0, 0.0):
        raise _DeadlineUnavailable()

    def interrupt(_signum: int, _frame: Any) -> None:
        raise _DeadlineExceeded()

    handler_installed = False
    try:
        try:
            signal.signal(signal.SIGALRM, interrupt)
            handler_installed = True
            signal.setitimer(signal.ITIMER_REAL, seconds)
        except (OSError, ValueError) as exc:
            raise _DeadlineUnavailable() from exc
        yield time.monotonic() + seconds
    finally:
        if handler_installed:
            try:
                signal.setitimer(signal.ITIMER_REAL, 0.0)
            except (OSError, ValueError):
                pass
            signal.signal(signal.SIGALRM, previous_handler)
            try:
                signal.setitimer(signal.ITIMER_REAL, *previous_timer)
            except (OSError, ValueError):
                pass


def _set_read_timeout(response: Any, timeout: float) -> None:
    """Apply the shrinking wall-clock budget to the underlying HTTP socket."""
    hook = getattr(response, "set_timeout_for_read", None)
    if callable(hook):  # controlled test transport
        hook(timeout)
        return
    fp = getattr(response, "fp", None)
    raw = getattr(fp, "raw", None)
    sock = getattr(raw, "_sock", None)
    if sock is not None:
        sock.settimeout(timeout)


def _read_bounded(response: Any, deadline: float) -> bytes:
    chunks: list[bytes] = []
    total = 0
    while True:
        remaining = _remaining(deadline)
        _set_read_timeout(response, remaining)
        reader = getattr(response, "read1", response.read)
        try:
            chunk = reader(min(READ_CHUNK_BYTES, MAX_RESPONSE_BYTES + 1 - total))
        except TimeoutError:
            _remaining(deadline)
            raise
        _remaining(deadline)
        if not isinstance(chunk, bytes):
            raise _ResponseError("invalid_response")
        if not chunk:
            return b"".join(chunks)
        chunks.append(chunk)
        total += len(chunk)
        if total > MAX_RESPONSE_BYTES:
            raise _ResponseError("response_too_large")


def route(caller_input: Any, *, api_key: Any = _FROM_ENV,
          opener: Any = None) -> dict[str, Any]:
    """Return typed, advisory routing data without ever raising provider errors."""
    if api_key is _FROM_ENV:
        api_key = os.environ.get("TYPESAFE_API_KEY")
    try:
        api_key = _normalize_api_key(api_key)
    except _InputError as exc:
        return _fallback_result(str(exc))
    if api_key is None:
        return _disabled_result()

    try:
        body, candidate_ids = _build_request_payload(caller_input)
    except _InputError as exc:
        return _fallback_result(str(exc))
    # Check semantic values rather than encoded bytes: JSON escaping must not
    # allow a quote, backslash, or control character in a key to evade this rule.
    if _contains_secret(caller_input, api_key):
        return _fallback_result("secret_in_input")

    request = urllib.request.Request(
        API_URL,
        data=body,
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
        },
        method="POST",
    )
    if opener is None:
        opener = urllib.request.build_opener(_NoRedirect())

    try:
        with _provider_deadline(TIMEOUT_SECONDS) as deadline:
            with opener.open(request, timeout=_remaining(deadline)) as response:
                status = response.getcode()
                if not isinstance(status, int) or not 200 <= status < 300:
                    return _fallback_result("provider_http_error")
                raw = _read_bounded(response, deadline)
        return _parse_response(raw, candidate_ids)
    except _DeadlineUnavailable:
        return _fallback_result("deadline_unavailable")
    except urllib.error.HTTPError:
        return _fallback_result("provider_http_error")
    except _DeadlineExceeded:
        return _fallback_result("provider_timeout")
    except urllib.error.URLError as exc:
        if isinstance(exc.reason, _DeadlineExceeded):
            return _fallback_result("provider_timeout")
        return _fallback_result("provider_unavailable")
    except (http.client.HTTPException, TimeoutError, OSError):
        return _fallback_result("provider_unavailable")
    except _ResponseError as exc:
        return _fallback_result(str(exc))


def main() -> int:
    try:
        api_key = _normalize_api_key(os.environ.get("TYPESAFE_API_KEY"))
    except _InputError as exc:
        result = _fallback_result(str(exc))
        json.dump(result, sys.stdout, ensure_ascii=False, allow_nan=False, sort_keys=True)
        sys.stdout.write("\n")
        return 2
    if api_key is None:
        result = _disabled_result()
        json.dump(result, sys.stdout, ensure_ascii=False, allow_nan=False, sort_keys=True)
        sys.stdout.write("\n")
        return 0

    try:
        source = getattr(sys.stdin, "buffer", sys.stdin)
        raw = source.read(MAX_INPUT_BYTES + 1)
        if isinstance(raw, str):
            raw = raw.encode("utf-8")
        if len(raw) > MAX_INPUT_BYTES:
            result = _fallback_result("input_too_large")
        else:
            caller_input = json.loads(raw.decode("utf-8"))
            result = route(caller_input, api_key=api_key)
    except (UnicodeError, ValueError, RecursionError, OSError):
        result = _fallback_result("invalid_input")
    json.dump(result, sys.stdout, ensure_ascii=False, allow_nan=False, sort_keys=True)
    sys.stdout.write("\n")
    return 2 if result["reason"] in LOCAL_INPUT_REASONS else 0


if __name__ == "__main__":
    raise SystemExit(main())
