#!/usr/bin/env python3
"""Offline tests for the optional Jev routing adapter."""

from __future__ import annotations

import importlib.util
import http.client
import io
import json
import os
import signal
import socket
import subprocess
import sys
import threading
import time
import unittest
import urllib.error
from pathlib import Path
from unittest import mock


MODULE_PATH = Path(__file__).with_name("jev_route.py")
SPEC = importlib.util.spec_from_file_location("jev_route", MODULE_PATH)
assert SPEC and SPEC.loader
jev_route = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(jev_route)


def caller_input() -> dict:
    return {
        "goal": "Fix the failing authentication test",
        "context": {"risk": "low", "language": "python"},
        "tools": [
            {"id": "rg", "description": "Search repository text"},
            {"id": "pytest", "description": "Run focused Python tests"},
        ],
        "agents": [{"id": "worker", "description": "Implement a bounded change"}],
        "models": [{"id": "fast", "description": "Fast model for a small change"}],
        "efforts": [{"id": "low", "description": "Low reasoning effort"}],
    }


def answer(choice: str = "candidate_0", confidence: float = 0.9,
           candidate_count: int = 1, probabilities: dict | None = None) -> dict:
    if probabilities is None:
        keys = [*(f"candidate_{index}" for index in range(candidate_count)), "no_match"]
        if choice in keys:
            remainder = 0.2 / (len(keys) - 1)
            probabilities = {key: 0.8 if key == choice else remainder for key in keys}
        else:
            probabilities = {key: 1.0 / len(keys) for key in keys}
    return {"type": "choice", "choice": choice, "confidence": confidence,
            "probabilities": probabilities}


def success_payload(**overrides: dict) -> bytes:
    answers = {
        "tool": answer(candidate_count=2),
        "agent": answer(),
        "model": answer(),
        "effort": answer(),
    }
    answers.update(overrides)
    return json.dumps({"model": "jev-1.13.0", "answers": answers,
                       "usage": {"input_tokens": 1, "output_tokens": 1}}).encode()


class FakeResponse:
    def __init__(self, body: bytes, status: int = 200):
        self.body = body
        self.status = status
        self.read_sizes: list[int] = []
        self.position = 0
        self.read_timeout: float | None = None

    def __enter__(self):
        return self

    def __exit__(self, *args):
        return False

    def getcode(self) -> int:
        return self.status

    def read(self, size: int) -> bytes:
        self.read_sizes.append(size)
        chunk = self.body[self.position:self.position + size]
        self.position += len(chunk)
        return chunk

    def read1(self, size: int) -> bytes:
        return self.read(size)

    def set_timeout_for_read(self, timeout: float) -> None:
        self.read_timeout = timeout


class FakeOpener:
    def __init__(self, response: FakeResponse | None = None,
                 error: Exception | None = None):
        self.response = response
        self.error = error
        self.calls: list[tuple] = []

    def open(self, request, timeout):
        self.calls.append((request, timeout))
        if self.error:
            raise self.error
        return self.response


class SlowDripResponse(FakeResponse):
    def read1(self, size: int) -> bytes:
        time.sleep(0.006)
        return super().read(1)


class BlockingResponse(FakeResponse):
    def read1(self, size: int) -> bytes:
        assert self.read_timeout is not None
        time.sleep(self.read_timeout)
        raise socket.timeout("read exceeded shrinking deadline")


class JevRouteTests(unittest.TestCase):
    def test_missing_or_blank_key_is_disabled_without_network(self):
        for key in (None, "", "  \t\n"):
            opener = FakeOpener(error=AssertionError("network must not be called"))
            result = jev_route.route(caller_input(), api_key=key, opener=opener)
            self.assertEqual("disabled", result["status"])
            self.assertFalse(result["enabled"])
            self.assertEqual("api_key_missing", result["reason"])
            self.assertEqual([], opener.calls)

    def test_environment_key_controls_activation(self):
        opener = FakeOpener(FakeResponse(success_payload()))
        with mock.patch.dict(os.environ, {"TYPESAFE_API_KEY": "env-secret"}, clear=True):
            result = jev_route.route(caller_input(), opener=opener)
        self.assertEqual("ok", result["status"])
        self.assertEqual("Bearer env-secret",
                         opener.calls[0][0].get_header("Authorization"))

    def test_successful_multi_dimension_request_and_mapping(self):
        payload = success_payload(
            tool=answer("candidate_1", 0.91, candidate_count=2),
            agent=answer("candidate_0", 0.8),
            model=answer("candidate_0", 0.7),
            effort=answer("candidate_0", 0.6),
        )
        response = FakeResponse(payload)
        opener = FakeOpener(response)
        result = jev_route.route(caller_input(), api_key="secret", opener=opener)

        self.assertEqual("ok", result["status"])
        self.assertTrue(result["advisory"])
        self.assertEqual("shadow", result["mode"])
        self.assertEqual("pytest", result["selections"]["tool"]["candidate"])
        self.assertTrue(all(value["accepted"] for value in result["selections"].values()))
        request, timeout = opener.calls[0]
        self.assertEqual(jev_route.API_URL, request.full_url)
        self.assertEqual("POST", request.get_method())
        self.assertGreater(timeout, 0)
        self.assertLessEqual(timeout, jev_route.TIMEOUT_SECONDS)
        self.assertEqual("application/json", request.get_header("Content-type"))
        wire = json.loads(request.data)
        self.assertEqual("jev-latest", wire["model"])
        self.assertEqual(set(("tool", "agent", "model", "effort")),
                         set(wire["questions"]))
        self.assertEqual("choice", wire["questions"]["tool"]["type"])
        self.assertEqual({"candidate_0", "candidate_1", "no_match"},
                         set(wire["questions"]["tool"]["criteria"]))
        self.assertEqual(jev_route.READ_CHUNK_BYTES, response.read_sizes[0])

    def test_empty_dimensions_are_not_asked(self):
        data = {"goal": "Choose", "tools": caller_input()["tools"]}
        payload = json.dumps({"answers": {"tool": answer(candidate_count=2)}}).encode()
        opener = FakeOpener(FakeResponse(payload))
        result = jev_route.route(data, api_key="secret", opener=opener)
        self.assertEqual({"tool"}, set(result["selections"]))
        self.assertEqual({"tool"}, set(json.loads(opener.calls[0][0].data)["questions"]))

    def test_no_candidates_fails_before_network(self):
        opener = FakeOpener(error=AssertionError("no network"))
        result = jev_route.route({"goal": "Choose"}, api_key="secret", opener=opener)
        self.assertEqual("fallback", result["status"])
        self.assertEqual("no_candidates", result["reason"])
        self.assertEqual([], opener.calls)

    def test_low_confidence_and_no_match_are_not_accepted(self):
        low = success_payload(tool=answer("candidate_1", 0.5999, candidate_count=2))
        result = jev_route.route(caller_input(), api_key="secret",
                                 opener=FakeOpener(FakeResponse(low)))
        self.assertEqual("fallback", result["status"])
        self.assertEqual("low_confidence", result["reason"])
        self.assertFalse(result["selections"]["tool"]["accepted"])
        self.assertEqual("low_confidence", result["selections"]["tool"]["reason"])
        self.assertTrue(all(not item["accepted"] for item in result["selections"].values()))
        self.assertEqual("atomic_fallback", result["selections"]["agent"]["reason"])
        no_match = success_payload(tool=answer("no_match", 0.99, candidate_count=2))
        result = jev_route.route(caller_input(), api_key="secret",
                                 opener=FakeOpener(FakeResponse(no_match)))
        self.assertEqual("fallback", result["status"])
        self.assertEqual("no_match", result["reason"])
        self.assertIsNone(result["selections"]["tool"]["candidate"])
        self.assertTrue(all(not item["accepted"] for item in result["selections"].values()))
        self.assertEqual("no_match", result["selections"]["tool"]["reason"])

    def test_all_no_match_is_atomic_fallback(self):
        payload = success_payload(
            tool=answer("no_match", 0.9, candidate_count=2),
            agent=answer("no_match", 0.9),
            model=answer("no_match", 0.9),
            effort=answer("no_match", 0.9),
        )
        result = jev_route.route(caller_input(), api_key="secret",
                                 opener=FakeOpener(FakeResponse(payload)))
        self.assertEqual("fallback", result["status"])
        self.assertEqual("no_match", result["reason"])
        self.assertTrue(all(not item["accepted"] for item in result["selections"].values()))

    def test_key_is_only_in_authorization_header_and_errors_are_sanitized(self):
        secret = "never-print-this-key"
        opener = FakeOpener(error=urllib.error.URLError(f"header Bearer {secret}"))
        result = jev_route.route(caller_input(), api_key=secret, opener=opener)
        rendered = json.dumps(result)
        self.assertNotIn(secret, rendered)
        request = opener.calls[0][0]
        self.assertNotIn(secret.encode(), request.data)
        self.assertEqual(f"Bearer {secret}", request.get_header("Authorization"))
        self.assertEqual("provider_unavailable", result["reason"])

    def test_key_accidentally_present_in_input_is_not_sent(self):
        for secret in ("never-send-this-key", 'quote"key', "slash\\key"):
            with self.subTest(secret=repr(secret)):
                data = caller_input()
                data["context"] = {"accidental": secret}
                opener = FakeOpener(error=AssertionError("no network"))
                result = jev_route.route(data, api_key=secret, opener=opener)
                self.assertEqual("secret_in_input", result["reason"])
                self.assertEqual([], opener.calls)
                self.assertNotIn(secret, json.dumps(result))

    def test_key_nested_in_list_is_not_sent(self):
        secret = "nested-secret-key"
        data = caller_input()
        data["context"] = {"nested": ["safe", [secret]]}
        opener = FakeOpener(error=AssertionError("no network"))
        result = jev_route.route(data, api_key=secret, opener=opener)
        self.assertEqual("secret_in_input", result["reason"])
        self.assertEqual([], opener.calls)

    def test_invalid_api_keys_fail_before_input_or_network(self):
        for invalid_key in ("line\nkey", "é", "e\u0301", "x" *
                            (jev_route.MAX_API_KEY_CHARS + 1)):
            with self.subTest(key=repr(invalid_key)):
                opener = FakeOpener(error=AssertionError("no network"))
                result = jev_route.route(caller_input(), api_key=invalid_key, opener=opener)
                self.assertEqual("invalid_api_key", result["reason"])
                self.assertEqual([], opener.calls)

    def test_unexpected_programming_errors_propagate(self):
        opener = FakeOpener(error=RuntimeError("programming bug"))
        with self.assertRaisesRegex(RuntimeError, "programming bug"):
            jev_route.route(caller_input(), api_key="secret", opener=opener)

    def test_provider_failures_are_sanitized_fallbacks(self):
        failures = [
            (TimeoutError("secret"), "provider_unavailable"),
            (socket.timeout("secret"), "provider_unavailable"),
            (urllib.error.URLError("secret"), "provider_unavailable"),
            (urllib.error.HTTPError(jev_route.API_URL, 429, "secret", {}, None),
             "provider_http_error"),
            (http.client.BadStatusLine("secret"), "provider_unavailable"),
            (http.client.IncompleteRead(b"secret", 10), "provider_unavailable"),
        ]
        for error, reason in failures:
            with self.subTest(error=type(error).__name__):
                result = jev_route.route(caller_input(), api_key="secret",
                                         opener=FakeOpener(error=error))
                self.assertEqual("fallback", result["status"])
                self.assertEqual(reason, result["reason"])
                self.assertNotIn("secret", json.dumps(result))

    def test_slow_drip_response_respects_end_to_end_deadline(self):
        opener = FakeOpener(SlowDripResponse(success_payload()))
        started = time.monotonic()
        with mock.patch.object(jev_route, "TIMEOUT_SECONDS", 0.01):
            result = jev_route.route(caller_input(), api_key="secret", opener=opener)
        self.assertEqual("provider_timeout", result["reason"])
        self.assertLess(time.monotonic() - started, 0.1)

    def test_blocking_read_is_interrupted_at_total_deadline(self):
        opener = FakeOpener(BlockingResponse(success_payload()))
        started = time.monotonic()
        with mock.patch.object(jev_route, "TIMEOUT_SECONDS", 0.01):
            result = jev_route.route(caller_input(), api_key="secret", opener=opener)
        self.assertEqual("provider_timeout", result["reason"])
        self.assertLess(time.monotonic() - started, 0.1)

    def test_delayed_open_is_interrupted_at_total_deadline_without_background_work(self):
        completed = []

        class DelayedOpener(FakeOpener):
            def open(self, request, timeout):
                self.calls.append((request, timeout))
                time.sleep(0.05)
                completed.append(True)
                return self.response

        opener = DelayedOpener(FakeResponse(success_payload()))
        started = time.monotonic()
        with mock.patch.object(jev_route, "TIMEOUT_SECONDS", 0.01):
            result = jev_route.route(caller_input(), api_key="secret", opener=opener)
        self.assertEqual("provider_timeout", result["reason"])
        self.assertLess(time.monotonic() - started, 0.04)
        time.sleep(0.05)
        self.assertEqual([], completed)

    def test_blocking_dns_is_interrupted_by_total_deadline(self):
        def slow_dns(*_args, **_kwargs):
            time.sleep(0.05)
            raise socket.gaierror("secret dns detail")

        started = time.monotonic()
        with mock.patch.object(jev_route, "TIMEOUT_SECONDS", 0.01), \
                mock.patch.object(socket, "getaddrinfo", side_effect=slow_dns):
            result = jev_route.route(caller_input(), api_key="secret")
        self.assertEqual("provider_timeout", result["reason"])
        self.assertLess(time.monotonic() - started, 0.04)

    @unittest.skipUnless(hasattr(signal, "setitimer"), "POSIX interval timer required")
    def test_deadline_restores_previous_signal_handler_and_timer(self):
        previous_handler = signal.getsignal(signal.SIGALRM)
        previous_timer = signal.getitimer(signal.ITIMER_REAL)

        def custom_handler(_signum, _frame):
            return None

        try:
            signal.setitimer(signal.ITIMER_REAL, 0.0)
            signal.signal(signal.SIGALRM, custom_handler)
            result = jev_route.route(caller_input(), api_key="secret",
                                     opener=FakeOpener(FakeResponse(success_payload())))
            self.assertEqual("ok", result["status"])
            self.assertIs(custom_handler, signal.getsignal(signal.SIGALRM))
            self.assertEqual((0.0, 0.0), signal.getitimer(signal.ITIMER_REAL))
        finally:
            signal.setitimer(signal.ITIMER_REAL, 0.0)
            signal.signal(signal.SIGALRM, previous_handler)
            signal.setitimer(signal.ITIMER_REAL, *previous_timer)

    @unittest.skipUnless(hasattr(signal, "setitimer"), "POSIX interval timer required")
    def test_deadline_setup_failure_restores_handler_and_fails_open(self):
        previous_handler = signal.getsignal(signal.SIGALRM)
        previous_timer = signal.getitimer(signal.ITIMER_REAL)
        opener = FakeOpener(error=AssertionError("no network"))
        real_setitimer = signal.setitimer

        def fail_install(which, seconds, interval=0.0):
            if seconds == jev_route.TIMEOUT_SECONDS:
                raise ValueError("unsupported timer")
            return real_setitimer(which, seconds, interval)

        def custom_handler(_signum, _frame):
            return None

        try:
            signal.setitimer(signal.ITIMER_REAL, 0.0)
            signal.signal(signal.SIGALRM, custom_handler)
            with mock.patch.object(jev_route.signal, "setitimer", side_effect=fail_install):
                result = jev_route.route(caller_input(), api_key="secret", opener=opener)
            self.assertEqual("deadline_unavailable", result["reason"])
            self.assertEqual([], opener.calls)
            self.assertIs(custom_handler, signal.getsignal(signal.SIGALRM))
            self.assertEqual((0.0, 0.0), signal.getitimer(signal.ITIMER_REAL))
        finally:
            signal.setitimer(signal.ITIMER_REAL, 0.0)
            signal.signal(signal.SIGALRM, previous_handler)
            signal.setitimer(signal.ITIMER_REAL, *previous_timer)

    @unittest.skipUnless(hasattr(signal, "setitimer"), "POSIX interval timer required")
    def test_deadline_capability_missing_fails_open_before_network(self):
        opener = FakeOpener(error=AssertionError("no network"))
        saved = signal.setitimer
        try:
            del signal.setitimer
            result = jev_route.route(caller_input(), api_key="secret", opener=opener)
        finally:
            signal.setitimer = saved
        self.assertEqual("deadline_unavailable", result["reason"])
        self.assertEqual([], opener.calls)

    @unittest.skipUnless(hasattr(signal, "setitimer"), "POSIX interval timer required")
    def test_deadline_timer_probe_failure_fails_open_before_network(self):
        opener = FakeOpener(error=AssertionError("no network"))
        with mock.patch.object(jev_route.signal, "getitimer",
                               side_effect=OSError("unsupported timer")):
            result = jev_route.route(caller_input(), api_key="secret", opener=opener)
        self.assertEqual("deadline_unavailable", result["reason"])
        self.assertEqual([], opener.calls)

    @unittest.skipUnless(hasattr(signal, "setitimer"), "POSIX interval timer required")
    def test_existing_timer_fails_open_before_network_and_is_preserved(self):
        previous_handler = signal.getsignal(signal.SIGALRM)
        previous_timer = signal.getitimer(signal.ITIMER_REAL)
        opener = FakeOpener(error=AssertionError("no network"))
        try:
            signal.setitimer(signal.ITIMER_REAL, 5.0)
            before = signal.getitimer(signal.ITIMER_REAL)[0]
            result = jev_route.route(caller_input(), api_key="secret", opener=opener)
            after = signal.getitimer(signal.ITIMER_REAL)[0]
            self.assertEqual("deadline_unavailable", result["reason"])
            self.assertEqual([], opener.calls)
            self.assertLessEqual(after, before)
            self.assertGreater(after, 4.0)
        finally:
            signal.setitimer(signal.ITIMER_REAL, 0.0)
            signal.signal(signal.SIGALRM, previous_handler)
            signal.setitimer(signal.ITIMER_REAL, *previous_timer)

    def test_non_main_thread_fails_open_before_network(self):
        results = []
        opener = FakeOpener(error=AssertionError("no network"))

        thread = threading.Thread(
            target=lambda: results.append(
                jev_route.route(caller_input(), api_key="secret", opener=opener)
            )
        )
        thread.start()
        thread.join(timeout=1)
        self.assertFalse(thread.is_alive())
        self.assertEqual("deadline_unavailable", results[0]["reason"])
        self.assertEqual([], opener.calls)

    def test_non_success_status_is_fallback(self):
        result = jev_route.route(caller_input(), api_key="secret",
                                 opener=FakeOpener(FakeResponse(b"ignored", 529)))
        self.assertEqual("provider_http_error", result["reason"])

    def test_malformed_and_oversized_responses_are_fallbacks(self):
        cases = [
            (b"not-json", "invalid_response"),
            (json.dumps({"answers": []}).encode(), "invalid_response"),
            ((b'{"answers":' + b"[" * 1_100 + b"0" + b"]" * 1_100 + b"}"),
             "invalid_response"),
            (b"x" * (jev_route.MAX_RESPONSE_BYTES + 1), "response_too_large"),
        ]
        for body, reason in cases:
            with self.subTest(reason=reason):
                result = jev_route.route(caller_input(), api_key="secret",
                                         opener=FakeOpener(FakeResponse(body)))
                self.assertEqual("fallback", result["status"])
                self.assertEqual(reason, result["reason"])

    def test_invalid_choice_answers_are_rejected(self):
        valid_tool = answer(candidate_count=2)
        invalid_answers = [
            None,
            {**valid_tool, "type": "score"},
            {**valid_tool, "choice": 1},
            {**valid_tool, "choice": "candidate_x"},
            {**valid_tool, "choice": "candidate_99"},
            {**valid_tool, "choice": "candidate_00"},
            {**valid_tool, "confidence": -0.1},
            {**valid_tool, "confidence": 1.1},
            {**valid_tool, "confidence": float("nan")},
            {**valid_tool, "confidence": True},
        ]
        for invalid in invalid_answers:
            with self.subTest(answer=invalid):
                result = jev_route.route(
                    caller_input(), api_key="secret",
                    opener=FakeOpener(FakeResponse(success_payload(tool=invalid))),
                )
                self.assertEqual("invalid_response", result["reason"])

    def test_invalid_probability_distributions_are_rejected(self):
        valid = {"candidate_0": 0.7, "candidate_1": 0.2, "no_match": 0.1}
        invalid_probabilities = [
            {"candidate_0": 0.8, "no_match": 0.2},
            {**valid, "extra": 0.0},
            {**valid, "candidate_0": float("nan")},
            {**valid, "candidate_0": -0.1},
            {**valid, "candidate_0": 1.1},
            {"candidate_0": 0.6, "candidate_1": 0.2, "no_match": 0.1},
        ]
        for probabilities in invalid_probabilities:
            with self.subTest(probabilities=probabilities):
                invalid = answer(candidate_count=2, probabilities=probabilities)
                result = jev_route.route(
                    caller_input(), api_key="secret",
                    opener=FakeOpener(FakeResponse(success_payload(tool=invalid))),
                )
                self.assertEqual("invalid_response", result["reason"])

    def test_out_of_range_probability_is_rejected_even_when_sum_is_one(self):
        probabilities = {"candidate_0": 1.1, "candidate_1": -0.1, "no_match": 0.0}
        invalid = answer(candidate_count=2, probabilities=probabilities)
        result = jev_route.route(
            caller_input(), api_key="secret",
            opener=FakeOpener(FakeResponse(success_payload(tool=invalid))),
        )
        self.assertEqual("fallback", result["status"])
        self.assertEqual("invalid_response", result["reason"])

    def test_in_range_probability_values_with_wrong_sum_are_rejected(self):
        probabilities = {"candidate_0": 0.6, "candidate_1": 0.2, "no_match": 0.1}
        invalid = answer(candidate_count=2, probabilities=probabilities)
        result = jev_route.route(
            caller_input(), api_key="secret",
            opener=FakeOpener(FakeResponse(success_payload(tool=invalid))),
        )
        self.assertEqual("fallback", result["status"])
        self.assertEqual("invalid_response", result["reason"])

    def test_probability_sum_tolerance_is_documented_and_accepted(self):
        within_tolerance = {
            "candidate_0": 0.7,
            "candidate_1": 0.2,
            "no_match": 0.1 + jev_route.PROBABILITY_SUM_TOLERANCE / 2,
        }
        payload = success_payload(tool=answer(
            candidate_count=2, probabilities=within_tolerance,
        ))
        result = jev_route.route(caller_input(), api_key="secret",
                                 opener=FakeOpener(FakeResponse(payload)))
        self.assertEqual("ok", result["status"])

    def test_missing_or_extra_answers_are_rejected(self):
        missing = json.dumps({"answers": {"tool": answer()}}).encode()
        extra = json.loads(success_payload())
        extra["answers"]["surprise"] = answer()
        for body in (missing, json.dumps(extra).encode()):
            result = jev_route.route(caller_input(), api_key="secret",
                                     opener=FakeOpener(FakeResponse(body)))
            self.assertEqual("invalid_response", result["reason"])

    def test_input_shape_and_bounds_fail_before_network(self):
        too_many = [{"id": str(i), "description": "d"}
                    for i in range(jev_route.MAX_CANDIDATES_PER_DIMENSION + 1)]
        cases = [
            None,
            {},
            {"goal": "x", "unknown": "private data"},
            {"goal": "x", "tools": "not-a-list"},
            {"goal": "x", "tools": [{"id": "x"}]},
            {"goal": "x", "tools": [{"id": "x", "description": "d"},
                                      {"id": "x", "description": "d"}]},
            {"goal": "x" * (jev_route.MAX_GOAL_CHARS + 1)},
            {"goal": "x", "context": "x" * (jev_route.MAX_CONTEXT_BYTES + 1)},
            {"goal": "x", "tools": too_many},
            {"goal": "x", "tools": [{"id": "x" *
                (jev_route.MAX_CANDIDATE_ID_CHARS + 1), "description": "d"}]},
            {"goal": "x", "tools": [{"id": "x", "description": "d" *
                (jev_route.MAX_DESCRIPTION_CHARS + 1)}]},
        ]
        for data in cases:
            with self.subTest(data=type(data).__name__):
                opener = FakeOpener(error=AssertionError("no network"))
                result = jev_route.route(data, api_key="secret", opener=opener)
                self.assertEqual("fallback", result["status"])
                self.assertEqual([], opener.calls)

    def test_each_local_shape_and_bound_guard_has_valid_surrounding_input(self):
        valid = [{"id": "tool", "description": "valid tool"}]
        many_descriptions = [
            {"id": f"id-{index}", "description": "d" * 600}
            for index in range(jev_route.MAX_CANDIDATES_PER_DIMENSION)
        ]
        cases = [
            ({"goal": "x", "context": {1, 2}, "tools": valid}, "input_not_json"),
            ({"goal": "x", "tools": 1}, "invalid_candidates"),
            ({"goal": "x", "tools": [{"id": 1, "description": "d"}]},
             "invalid_candidates"),
            ({"goal": "x", "tools": [{"id": " ", "description": "d"}]},
             "invalid_candidates"),
            ({"goal": "x", "tools": [{"id": "x", "description": 1}]},
             "invalid_candidates"),
            ({"goal": "x", "tools": [{"id": "x", "description": " "}]},
             "invalid_candidates"),
            ({"goal": "x", "tools": valid, "unknown": "private"},
             "unknown_input_field"),
            ({"goal": "x" * (jev_route.MAX_GOAL_CHARS + 1), "tools": valid},
             "input_too_large"),
            ({"goal": "x", "context": "c" * jev_route.MAX_CONTEXT_BYTES,
              "tools": valid}, "input_too_large"),
            ({"goal": "x", "tools": many_descriptions, "agents": many_descriptions,
              "models": many_descriptions, "efforts": many_descriptions},
             "input_too_large"),
        ]
        for data, reason in cases:
            with self.subTest(reason=reason, fields=tuple(data)):
                opener = FakeOpener(error=AssertionError("no network"))
                result = jev_route.route(data, api_key="secret", opener=opener)
                self.assertEqual(reason, result["reason"])
                self.assertEqual([], opener.calls)

    def test_expired_remaining_budget_raises(self):
        with self.assertRaises(jev_route._DeadlineExceeded):
            jev_route._remaining(time.monotonic() - 1.0)

    def test_underlying_socket_receives_shrinking_timeout(self):
        class SocketRecorder:
            def __init__(self):
                self.values = []

            def settimeout(self, value):
                self.values.append(value)

        recorder = SocketRecorder()
        response = type("Response", (), {})()
        response.fp = type("FP", (), {})()
        response.fp.raw = type("Raw", (), {"_sock": recorder})()
        jev_route._set_read_timeout(response, 0.25)
        self.assertEqual([0.25], recorder.values)

    def test_non_bytes_response_chunk_is_invalid_response(self):
        response = FakeResponse("not-bytes")
        result = jev_route.route(caller_input(), api_key="secret",
                                 opener=FakeOpener(response))
        self.assertEqual("invalid_response", result["reason"])

    def test_redirect_handler_refuses_redirects(self):
        handler = jev_route._NoRedirect()
        self.assertIsNone(handler.redirect_request(None, None, 302, "redirect", {},
                                                   "https://example.invalid"))

    def test_real_cli_entry_emits_disabled_json(self):
        environment = dict(os.environ)
        environment.pop("TYPESAFE_API_KEY", None)
        completed = subprocess.run(
            [sys.executable, str(MODULE_PATH)], input="not-json", text=True,
            capture_output=True, env=environment, timeout=5, check=False,
        )
        self.assertEqual(0, completed.returncode)
        self.assertEqual("disabled", json.loads(completed.stdout)["status"])
        self.assertEqual("", completed.stderr)

    def test_cli_emits_typed_json_and_nonzero_for_invalid_input(self):
        stdout = io.StringIO()
        with mock.patch.dict(os.environ, {"TYPESAFE_API_KEY": "secret"}, clear=True), \
                mock.patch.object(sys, "stdin", io.StringIO("not-json")), \
                mock.patch.object(sys, "stdout", stdout):
            self.assertEqual(2, jev_route.main())
        output = json.loads(stdout.getvalue())
        self.assertEqual("fallback", output["status"])
        self.assertEqual("invalid_input", output["reason"])

    def test_cli_without_key_does_not_read_stdin(self):
        class Unreadable:
            def read(self, _size):
                raise AssertionError("disabled CLI must not read stdin")

        stdout = io.StringIO()
        with mock.patch.dict(os.environ, {}, clear=True), \
                mock.patch.object(sys, "stdin", Unreadable()), \
                mock.patch.object(sys, "stdout", stdout):
            self.assertEqual(0, jev_route.main())
        output = json.loads(stdout.getvalue())
        self.assertEqual("disabled", output["status"])
        self.assertFalse(output["enabled"])

    def test_cli_rejects_oversized_raw_input(self):
        stdout = io.StringIO()
        oversized = " " * (jev_route.MAX_INPUT_BYTES + 1)
        with mock.patch.dict(os.environ, {"TYPESAFE_API_KEY": "secret"}, clear=True), \
                mock.patch.object(sys, "stdin", io.StringIO(oversized)), \
                mock.patch.object(sys, "stdout", stdout):
            self.assertEqual(2, jev_route.main())
        output = json.loads(stdout.getvalue())
        self.assertEqual("fallback", output["status"])
        self.assertEqual("input_too_large", output["reason"])

    def test_cli_rejects_pathological_json_as_local_input(self):
        cases = ["[" * 1_100 + "0" + "]" * 1_100, "9" * 5_000]
        for raw in cases:
            with self.subTest(prefix=raw[:8]):
                stdout = io.StringIO()
                with mock.patch.dict(os.environ, {"TYPESAFE_API_KEY": "secret"}, clear=True), \
                        mock.patch.object(sys, "stdin", io.StringIO(raw)), \
                        mock.patch.object(sys, "stdout", stdout):
                    self.assertEqual(2, jev_route.main())
                self.assertEqual("invalid_input", json.loads(stdout.getvalue())["reason"])

    def test_cli_contract_errors_are_nonzero_but_provider_fallback_is_zero(self):
        stdout = io.StringIO()
        raw = json.dumps({"goal": "missing candidates"})
        with mock.patch.dict(os.environ, {"TYPESAFE_API_KEY": "secret"}, clear=True), \
                mock.patch.object(sys, "stdin", io.StringIO(raw)), \
                mock.patch.object(sys, "stdout", stdout):
            self.assertEqual(2, jev_route.main())
        self.assertEqual("no_candidates", json.loads(stdout.getvalue())["reason"])

        stdout = io.StringIO()
        raw = json.dumps(caller_input())
        provider_result = jev_route._fallback_result("provider_unavailable")
        with mock.patch.dict(os.environ, {"TYPESAFE_API_KEY": "secret"}, clear=True), \
                mock.patch.object(sys, "stdin", io.StringIO(raw)), \
                mock.patch.object(sys, "stdout", stdout), \
                mock.patch.object(jev_route, "route", return_value=provider_result):
            self.assertEqual(0, jev_route.main())

    def test_cli_provider_protocol_failure_is_sanitized_and_zero(self):
        stdout = io.StringIO()
        raw = json.dumps(caller_input())
        opener = FakeOpener(error=http.client.BadStatusLine("secret response"))
        with mock.patch.dict(os.environ, {"TYPESAFE_API_KEY": "secret"}, clear=True), \
                mock.patch.object(sys, "stdin", io.StringIO(raw)), \
                mock.patch.object(sys, "stdout", stdout), \
                mock.patch.object(jev_route.urllib.request, "build_opener",
                                  return_value=opener):
            self.assertEqual(0, jev_route.main())
        output = stdout.getvalue()
        self.assertEqual("provider_unavailable", json.loads(output)["reason"])
        self.assertNotIn("secret", output)

    def test_cli_invalid_unicode_key_is_nonzero_without_reading_stdin(self):
        class Unreadable:
            def read(self, _size):
                raise AssertionError("invalid key must be rejected before stdin")

        stdout = io.StringIO()
        with mock.patch.dict(os.environ, {"TYPESAFE_API_KEY": "é"}, clear=True), \
                mock.patch.object(sys, "stdin", Unreadable()), \
                mock.patch.object(sys, "stdout", stdout):
            self.assertEqual(2, jev_route.main())
        self.assertEqual("invalid_api_key", json.loads(stdout.getvalue())["reason"])


if __name__ == "__main__":
    unittest.main()
