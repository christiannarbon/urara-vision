"""The package imports.

A placeholder while the service has no behaviour: it keeps the suite from being
empty, and it fails loudly if the src/ layout ever stops resolving.
"""


def test_package_imports() -> None:
    import urara_chat

    assert urara_chat.__doc__
