# Deploy helper for the google-trends Actor, mirroring actors/_probe-gate0/push.py.
# Builds the sourceFiles payload from disk rather than a shell heredoc (pipeline-test.md).
import json
import os

ROOT = os.path.dirname(os.path.abspath(__file__))
WANT = [
    "main.go",
    "trends.go",
    "types.go",
    "input.go",
    "charge.go",
    "storage.go",
    "apify_api.go",
    "errors.go",
    "env.go",
    "go.mod",
    ".actor/actor.json",
    ".actor/Dockerfile",
    ".actor/input_schema.json",
    ".actor/output_schema.json",
    ".actor/dataset_schema.json",
]

files = []
for rel in WANT:
    path = os.path.join(ROOT, rel.replace("/", os.sep))
    with open(path, "r", encoding="utf-8", errors="replace") as fh:
        files.append({"name": rel, "format": "TEXT", "content": fh.read()})

payload = {
    "versionNumber": "0.1",
    "sourceType": "SOURCE_FILES",
    "buildTag": "latest",
    "sourceFiles": files,
}
with open(os.path.join(ROOT, "version.json"), "w", encoding="utf-8") as fh:
    json.dump(payload, fh)
print("wrote version.json with %d files" % len(files))
