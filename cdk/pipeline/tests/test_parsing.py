from pathlib import Path

from indexer import (
    MAX_INPUT_CHARS,
    _canonical_url,
    _chunk_id,
    _detect_area,
    parse_html,
)


def test_detect_area_servlet():
    # Use a filename that is not itself an area key ("csrf" is not in AREA_PREFIXES)
    assert _detect_area("/site/servlet/csrf.html") == "servlet"


def test_detect_area_oauth2():
    assert _detect_area("/site/oauth2/login.html") == "oauth2"


def test_detect_area_method_security():
    assert _detect_area("/site/method-security/index.html") == "method-security"


def test_detect_area_unknown():
    assert _detect_area("/site/index.html") == "other"


# Versioned Antora paths: oauth2/saml2/architecture docs live under servlet/
# in the actual Spring Security site structure. The most specific directory
# component must win over the shallower "servlet" prefix.
def test_detect_area_oauth2_nested_under_servlet():
    assert _detect_area("/site/6.5-SNAPSHOT/servlet/oauth2/login/core.html") == "oauth2"


def test_detect_area_saml2_nested_under_servlet():
    assert _detect_area("/site/6.5-SNAPSHOT/servlet/saml2/login/overview.html") == "saml2"


def test_detect_area_architecture_nested_under_servlet():
    assert _detect_area("/site/6.5-SNAPSHOT/servlet/architecture.html") == "architecture"


def test_detect_area_method_security_under_native_image():
    assert _detect_area("/site/7.0-SNAPSHOT/native-image/method-security.html") == "method-security"


def test_detect_area_servlet_non_specific_page():
    # Pages without a more specific area component stay as "servlet"
    assert _detect_area("/site/6.5-SNAPSHOT/servlet/exploits/csrf.html") == "servlet"


def test_canonical_url():
    url = _canonical_url("/site/servlet/auth.html", "/site")
    assert url == "https://docs.spring.io/spring-security/reference/servlet/auth.html"


def test_chunk_id_deterministic():
    a = _chunk_id("6.5.x", "abc", "https://example.com/page", ["Title"])
    b = _chunk_id("6.5.x", "abc", "https://example.com/page", ["Title"])
    assert a == b


def test_chunk_id_unique_on_different_ref():
    a = _chunk_id("6.5.x", "abc", "https://example.com/page", ["Title"])
    b = _chunk_id("7.0.x", "abc", "https://example.com/page", ["Title"])
    assert a != b


def test_chunk_id_unique_on_different_sha():
    a = _chunk_id("6.5.x", "sha1", "https://example.com/page", ["Title"])
    b = _chunk_id("6.5.x", "sha2", "https://example.com/page", ["Title"])
    assert a != b


def test_parse_html_basic(tmp_path):
    site = tmp_path / "site"
    site.mkdir()
    page = site / "index.html"
    page.write_text(
        "<html><body><article><h1>Spring Security</h1><p>Intro</p></article></body></html>",
        encoding="utf-8",
    )
    chunks = parse_html(str(page), str(site), "6.5.x", "abc123", "2026-06-06T00:00:00Z")
    assert len(chunks) == 1
    chunk = chunks[0]
    assert chunk["title"] == "Spring Security"
    assert chunk["ref"] == "6.5.x"
    assert chunk["commitSha"] == "abc123"
    assert chunk["area"] == "other"
    assert chunk["builtAt"] == "2026-06-06T00:00:00Z"
    assert "Intro" in chunk["contentText"]
    assert chunk["canonicalUrl"] == "https://docs.spring.io/spring-security/reference/index.html"


def test_parse_html_area_from_path(tmp_path):
    site = tmp_path / "site"
    (site / "servlet").mkdir(parents=True)
    page = site / "servlet" / "csrf.html"
    page.write_text(
        "<html><body><article><h1>CSRF</h1></article></body></html>",
        encoding="utf-8",
    )
    chunks = parse_html(str(page), str(site), "6.5.x", "abc123", "2026-06-06T00:00:00Z")
    assert chunks[0]["area"] == "servlet"


def test_parse_html_strips_nav(tmp_path):
    site = tmp_path / "site"
    site.mkdir()
    page = site / "page.html"
    page.write_text(
        "<html><body><nav>NAV_CONTENT</nav><article><h1>T</h1><p>Body</p></article></body></html>",
        encoding="utf-8",
    )
    chunks = parse_html(str(page), str(site), "6.5.x", "abc", "2026-06-06T00:00:00Z")
    assert "NAV_CONTENT" not in chunks[0]["contentText"]
    assert "NAV_CONTENT" not in chunks[0]["contentHtml"]


def test_parse_html_title_falls_back_to_filename(tmp_path):
    site = tmp_path / "site"
    site.mkdir()
    page = site / "my-page.html"
    page.write_text(
        "<html><body><article><p>No heading here</p></article></body></html>",
        encoding="utf-8",
    )
    chunks = parse_html(str(page), str(site), "6.5.x", "abc", "2026-06-06T00:00:00Z")
    assert chunks[0]["title"] == "my-page"


def test_parse_html_truncates_content(tmp_path):
    site = tmp_path / "site"
    site.mkdir()
    page = site / "long.html"
    long_text = "x" * (MAX_INPUT_CHARS + 1000)
    page.write_text(
        f"<html><body><article><h1>T</h1><p>{long_text}</p></article></body></html>",
        encoding="utf-8",
    )
    chunks = parse_html(str(page), str(site), "6.5.x", "abc", "2026-06-06T00:00:00Z")
    assert len(chunks[0]["contentText"]) <= MAX_INPUT_CHARS
    assert len(chunks[0]["contentHtml"]) <= MAX_INPUT_CHARS


def test_parse_html_source_path(tmp_path):
    site = tmp_path / "site"
    (site / "oauth2").mkdir(parents=True)
    page = site / "oauth2" / "login.html"
    page.write_text("<html><body><article><h1>OAuth2</h1></article></body></html>", encoding="utf-8")
    chunks = parse_html(str(page), str(site), "6.5.x", "abc", "2026-06-06T00:00:00Z")
    assert chunks[0]["sourcePath"] == str(Path("oauth2") / "login.html")


def test_api_files_excluded_from_indexing(tmp_path):
    site = tmp_path / "site"
    (site / "api" / "java").mkdir(parents=True)
    (site / "servlet").mkdir(parents=True)
    (site / "api" / "java" / "Foo.html").write_text("x", encoding="utf-8")
    (site / "servlet" / "auth.html").write_text("x", encoding="utf-8")

    all_html = sorted(site.rglob("*.html"))
    html_files = [f for f in all_html if "api" not in f.relative_to(site).parts]

    assert len(html_files) == 1
    assert html_files[0].name == "auth.html"
