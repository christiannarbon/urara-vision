"""Shared pytest configuration.

`asyncio_mode = "auto"` in pyproject.toml means pytest-asyncio adopts every
async test without a per-test marker, so no backend fixture is needed here yet.
"""
