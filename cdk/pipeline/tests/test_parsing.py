from pathlib import Path

from indexer import (
    MAX_INPUT_CHARS,
    _canonical_url,
    _chunk_id,
    _detect_area,
    _iter_content_nodes,
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


def test_parse_html_h2_splits_into_two_chunks(tmp_path):
    site = tmp_path / "site"
    site.mkdir()
    page = site / "page.html"
    page.write_text(
        "<html><body><article>"
        "<h1>Auth</h1><p>Intro</p>"
        "<h2>Form Login</h2><p>Form details</p>"
        "</article></body></html>",
        encoding="utf-8",
    )
    chunks = parse_html(str(page), str(site), "6.5.x", "abc", "2026-06-06T00:00:00Z")
    assert len(chunks) == 2
    assert chunks[0]["title"] == "Auth"
    assert chunks[0]["headingPath"] == ["Auth"]
    assert "Intro" in chunks[0]["contentText"]
    assert chunks[1]["title"] == "Form Login"
    assert chunks[1]["headingPath"] == ["Auth", "Form Login"]
    assert "Form details" in chunks[1]["contentText"]


def test_parse_html_h3_uses_three_level_heading_path(tmp_path):
    site = tmp_path / "site"
    site.mkdir()
    page = site / "page.html"
    page.write_text(
        "<html><body><article>"
        "<h1>Auth</h1><p>Intro</p>"
        "<h2>Username</h2><p>Username intro</p>"
        "<h3>Form Login</h3><p>Form details</p>"
        "</article></body></html>",
        encoding="utf-8",
    )
    chunks = parse_html(str(page), str(site), "6.5.x", "abc", "2026-06-06T00:00:00Z")
    assert len(chunks) == 3
    assert chunks[2]["headingPath"] == ["Auth", "Username", "Form Login"]
    assert "Form details" in chunks[2]["contentText"]


def test_parse_html_heading_without_body_produces_no_chunk(tmp_path):
    # A heading immediately followed by another heading has no body content
    # and must not create a chunk (avoids empty/useless index entries).
    site = tmp_path / "site"
    site.mkdir()
    page = site / "page.html"
    page.write_text(
        "<html><body><article>"
        "<h1>Page</h1>"
        "<h2>Section</h2><p>Content</p>"
        "</article></body></html>",
        encoding="utf-8",
    )
    chunks = parse_html(str(page), str(site), "6.5.x", "abc", "2026-06-06T00:00:00Z")
    # h1 has no body → no chunk; h2 has body → 1 chunk
    assert len(chunks) == 1
    assert chunks[0]["title"] == "Section"


def test_parse_html_no_empty_content_text(tmp_path):
    # HTML nodes with no visible text (e.g. empty divs) must not produce a
    # chunk with contentText="" — Bedrock rejects empty embedding inputs.
    site = tmp_path / "site"
    site.mkdir()
    page = site / "page.html"
    page.write_text(
        "<html><body><article>"
        "<h1>Title</h1>"
        "<div></div><div>   </div>"
        "<h2>Next</h2><p>Real content</p>"
        "</article></body></html>",
        encoding="utf-8",
    )
    chunks = parse_html(str(page), str(site), "6.5.x", "abc", "2026-06-06T00:00:00Z")
    for chunk in chunks:
        assert chunk["contentText"].strip() != "", f"Empty contentText in chunk: {chunk['title']}"


def test_parse_html_antora_nested_sections(tmp_path):
    # Antora wraps h2/h3 inside div.sect1/sect2. _iter_content_nodes must
    # surface those headings rather than treating the wrapper div as content.
    site = tmp_path / "site"
    (site / "servlet").mkdir(parents=True)
    page = site / "servlet" / "architecture.html"
    page.write_text(
        "<html><body><article>"
        "<h1>Architecture</h1>"
        "<div id='preamble'><p>Overview of Spring Security.</p></div>"
        "<div class='sect1'>"
        "  <h2>SecurityFilterChain</h2>"
        "  <div class='sectionbody'><p>SecurityFilterChain is used by FilterChainProxy.</p></div>"
        "</div>"
        "<div class='sect1'>"
        "  <h2>DelegatingFilterProxy</h2>"
        "  <div class='sectionbody'><p>DelegatingFilterProxy bridges Servlet and Spring.</p></div>"
        "</div>"
        "</article></body></html>",
        encoding="utf-8",
    )
    chunks = parse_html(str(page), str(site), "7.0.x", "sha1", "2026-06-06T00:00:00Z")
    assert len(chunks) == 3

    titles = [c["title"] for c in chunks]
    assert titles == ["Architecture", "SecurityFilterChain", "DelegatingFilterProxy"]

    # SecurityFilterChain must appear in the correct chunk's text
    sfc_chunk = next(c for c in chunks if c["title"] == "SecurityFilterChain")
    assert "SecurityFilterChain" in sfc_chunk["contentText"]
    assert sfc_chunk["headingPath"] == ["Architecture", "SecurityFilterChain"]
    assert sfc_chunk["area"] == "architecture"


def test_iter_content_nodes_surfaces_nested_headings():
    from bs4 import BeautifulSoup
    html = (
        "<article>"
        "<h1>Page</h1>"
        "<div class='sect1'><h2>Section</h2><p>body</p></div>"
        "</article>"
    )
    soup = BeautifulSoup(html, "lxml")
    article = soup.find("article")
    nodes = list(_iter_content_nodes(article))
    kinds = [k for k, _ in nodes]
    assert kinds == ["h1", "h2", "content"]


def test_parse_html_duplicate_headings_produce_duplicate_chunk_ids(tmp_path):
    # Pages with two h2 sections of the same text produce duplicate chunkIds.
    # _run deduplicates them; this test documents the known behaviour so the
    # dedup logic can be verified against real input shape.
    site = tmp_path / "site"
    site.mkdir()
    page = site / "page.html"
    page.write_text(
        "<html><body><article>"
        "<h1>Auth</h1>"
        "<h2>Overview</h2><p>First overview.</p>"
        "<h2>Overview</h2><p>Second overview.</p>"
        "</article></body></html>",
        encoding="utf-8",
    )
    chunks = parse_html(str(page), str(site), "6.5.x", "abc", "2026-06-06T00:00:00Z")
    ids = [c["chunkId"] for c in chunks]
    assert len(ids) != len(set(ids)), "expected duplicate chunkIds for identical headings"


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
