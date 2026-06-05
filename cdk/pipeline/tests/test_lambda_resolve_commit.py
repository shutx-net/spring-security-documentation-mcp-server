import importlib.util
import json
import pathlib
from unittest.mock import MagicMock, patch

_path = pathlib.Path(__file__).parent.parent / "lambda" / "resolve_commit" / "index.py"
_spec = importlib.util.spec_from_file_location("resolve_commit_handler", _path)
_mod = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_mod)


def _mock_urlopen(sha: str) -> MagicMock:
    resp = MagicMock()
    resp.__enter__ = lambda s: s
    resp.__exit__ = MagicMock(return_value=False)
    resp.read.return_value = json.dumps({"object": {"sha": sha}}).encode()
    return resp


def test_handler_returns_ref_and_commit_sha():
    with patch("urllib.request.urlopen", return_value=_mock_urlopen("abc123")):
        result = _mod.handler({"ref": "6.5.x"}, None)

    assert result == {"ref": "6.5.x", "commitSha": "abc123"}


def test_handler_different_ref():
    with patch("urllib.request.urlopen", return_value=_mock_urlopen("def456")):
        result = _mod.handler({"ref": "7.0.x"}, None)

    assert result == {"ref": "7.0.x", "commitSha": "def456"}


def test_handler_calls_github_api_with_correct_url():
    with patch("urllib.request.urlopen", return_value=_mock_urlopen("abc123")) as mock_open:
        _mod.handler({"ref": "6.5.x"}, None)

    req = mock_open.call_args[0][0]
    assert "heads/6.5.x" in req.full_url
    assert "spring-projects/spring-security" in req.full_url
