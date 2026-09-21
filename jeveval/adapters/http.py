from __future__ import annotations

import json
from typing import Any

import httpx

from .base import ModelError, TransientModelError


TRANSIENT_STATUS = {408, 409, 425, 429, 500, 502, 503, 504, 529}


async def post_json(client: httpx.AsyncClient, url: str, *, headers: dict[str, str], payload: dict[str, Any]) -> dict[str, Any]:
    try:
        response = await client.post(url, headers=headers, json=payload)
    except (httpx.TimeoutException, httpx.NetworkError) as exc:
        raise TransientModelError(str(exc)) from exc
    if response.status_code in TRANSIENT_STATUS:
        retry_after = response.headers.get("retry-after")
        try:
            delay = float(retry_after) if retry_after else None
        except ValueError:
            delay = None
        raise TransientModelError(f"HTTP {response.status_code}: {response.text[:500]}", delay)
    if response.is_error:
        raise ModelError(f"HTTP {response.status_code}: {response.text[:1000]}")
    try:
        data = response.json()
    except json.JSONDecodeError as exc:
        raise ModelError(f"Model endpoint returned invalid JSON: {response.text[:500]}") from exc
    if not isinstance(data, dict):
        raise ModelError("Model endpoint returned a non-object JSON response")
    return data

