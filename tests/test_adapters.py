import json

import httpx
import pytest

from jeveval.adapters import JevAdapter, MalformedModelError, OpenAICompatibleAdapter
from jeveval.models import Example


@pytest.mark.asyncio
async def test_jev_noul_response(monkeypatch):
    monkeypatch.setenv("TEST_TYPESAFE_KEY", "secret")
    captured = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured.update(json.loads(request.content))
        return httpx.Response(200, json={"model": "jev-test", "answers": {"answer": {"type": "noul", "noul": 0.8}}, "usage": {"input_tokens": 10, "output_tokens": 2}})

    adapter = JevAdapter(api_key_env="TEST_TYPESAFE_KEY")
    await adapter.client.aclose()
    adapter.client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
    example = Example("1", "cladder", "facts", "question", {"yes": "Yes", "no": "No"}, "yes", question_type="noul")
    prediction = await adapter.predict(example)
    await adapter.aclose()
    assert captured["questions"]["answer"]["type"] == "noul"
    assert prediction.label == "yes"
    assert prediction.probabilities == pytest.approx({"yes": 0.8, "no": 0.2})


@pytest.mark.asyncio
async def test_openai_compatible_structured_response(monkeypatch):
    monkeypatch.setenv("TEST_OPENAI_KEY", "secret")
    captured = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured.update(json.loads(request.content))
        content = json.dumps({"label": "A", "probabilities": {"A": 0.7, "B": 0.3}})
        return httpx.Response(200, json={"model": "reference", "choices": [{"message": {"content": content}}], "usage": {"prompt_tokens": 11, "completion_tokens": 7}})

    adapter = OpenAICompatibleAdapter(name="ref", model="m", base_url="https://example.test/v1", api_key_env="TEST_OPENAI_KEY")
    await adapter.client.aclose()
    adapter.client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
    example = Example("1", "b", "state", "question", {"A": "first", "B": "second"}, "A")
    prediction = await adapter.predict(example)
    await adapter.aclose()
    assert captured["reasoning_effort"] == "none"
    assert captured["response_format"]["type"] == "json_schema"
    assert prediction.label == "A"
    assert prediction.usage.input_tokens == 11


@pytest.mark.asyncio
async def test_openai_compatible_corrects_one_malformed_response(monkeypatch):
    monkeypatch.setenv("TEST_OPENAI_KEY", "secret")
    requests = []

    def handler(request: httpx.Request) -> httpx.Response:
        payload = json.loads(request.content)
        requests.append(payload)
        if len(requests) == 1:
            content = json.dumps({"A": 0.7, "B": 0.3})
        else:
            content = json.dumps({"label": "A", "probabilities": {"A": 0.7, "B": 0.3}})
        return httpx.Response(200, json={
            "model": "reference", "choices": [{"message": {"content": content}}],
            "usage": {"prompt_tokens": 11, "completion_tokens": 7},
        })

    adapter = OpenAICompatibleAdapter(name="ref", model="m", base_url="https://example.test/v1", api_key_env="TEST_OPENAI_KEY")
    await adapter.client.aclose()
    adapter.client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
    example = Example("1", "b", "state", "question", {"A": "first", "B": "second"}, "A")
    prediction = await adapter.predict(example)
    await adapter.aclose()
    assert len(requests) == 2
    assert all(request["reasoning_effort"] == "none" for request in requests)
    assert "exact required JSON Schema" in requests[0]["messages"][0]["content"]
    assert "Do not return a flat" in requests[1]["messages"][-1]["content"]
    assert prediction.label == "A"
    assert prediction.usage.input_tokens == 22
    assert prediction.usage.output_tokens == 14


@pytest.mark.asyncio
async def test_openai_compatible_stops_after_one_corrective_retry(monkeypatch):
    monkeypatch.setenv("TEST_OPENAI_KEY", "secret")
    calls = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        content = json.dumps({"A": 0.7, "B": 0.3})
        return httpx.Response(200, json={"model": "reference", "choices": [{"message": {"content": content}}]})

    adapter = OpenAICompatibleAdapter(name="ref", model="m", base_url="https://example.test/v1", api_key_env="TEST_OPENAI_KEY")
    await adapter.client.aclose()
    adapter.client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
    example = Example("1", "b", "state", "question", {"A": "first", "B": "second"}, "A")
    with pytest.raises(MalformedModelError, match="after one corrective retry"):
        await adapter.predict(example)
    await adapter.aclose()
    assert calls == 2

def test_openai_compatible_rejects_reasoning_effort_other_than_none(monkeypatch):
    monkeypatch.setenv("TEST_OPENAI_KEY", "secret")
    with pytest.raises(ValueError, match="require reasoning_effort='none'"):
        OpenAICompatibleAdapter(
            name="ref",
            model="m",
            base_url="https://example.test/v1",
            api_key_env="TEST_OPENAI_KEY",
            reasoning_effort="low",
        )
