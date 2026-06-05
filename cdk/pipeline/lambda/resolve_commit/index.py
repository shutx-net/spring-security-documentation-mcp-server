"""resolve-commit Lambda.

Input:  { "ref": "6.5.x" }
Output: { "ref": "6.5.x", "commitSha": "abc123..." }

Uses the GitHub API to resolve a branch ref to its HEAD commit SHA,
acting as a coordination step before CodeBuild / ECS tasks (which cannot
surface their computed values as Step Functions output).
"""

import json
import urllib.request

REPO = "spring-projects/spring-security"


def handler(event, context):
    ref = event["ref"]
    url = f"https://api.github.com/repos/{REPO}/git/ref/heads/{ref}"
    req = urllib.request.Request(url, headers={"User-Agent": "spring-security-mcp/1.0"})
    with urllib.request.urlopen(req, timeout=10) as resp:
        data = json.loads(resp.read())
    commit_sha = data["object"]["sha"]
    return {"ref": ref, "commitSha": commit_sha}
