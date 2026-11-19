"""Write deterministic synthetic Claude Code sessions.

Usage: gensessions.py <claude projects dir> <sessions> <turns per session> [seed]
"""

import json
import random
import sys
import uuid
from datetime import datetime, timedelta, timezone
from pathlib import Path

WORDS = "agent build check commit deploy diff error file fix lint merge parse query refactor review schema server test token update".split()


def text(rng, n):
    return " ".join(rng.choice(WORDS) for _ in range(n))


def session(rng, project, sid, turns, start):
    cwd = f"/home/dev/{project}"
    parent = None
    t = start
    for i in range(turns):
        u = str(uuid.UUID(int=rng.getrandbits(128)))
        base = {"sessionId": sid, "cwd": cwd, "version": "2.0.30", "gitBranch": "main", "isSidechain": False, "userType": "external"}
        yield {**base, "type": "user", "uuid": u, "parentUuid": parent, "timestamp": t.isoformat().replace("+00:00", "Z"),
               "message": {"role": "user", "content": text(rng, rng.randint(8, 60))}}
        t += timedelta(seconds=rng.randint(2, 30))
        a = str(uuid.UUID(int=rng.getrandbits(128)))
        tool = f"toolu_{rng.getrandbits(64):016x}"
        content = [{"type": "text", "text": text(rng, rng.randint(40, 400))}]
        if i % 3 == 0:
            content.append({"type": "tool_use", "id": tool, "name": "Bash", "input": {"command": f"go test ./{rng.choice(WORDS)}/..."}})
        yield {**base, "type": "assistant", "uuid": a, "parentUuid": u, "timestamp": t.isoformat().replace("+00:00", "Z"),
               "requestId": f"req_{rng.getrandbits(64):016x}",
               "message": {"id": f"msg_{rng.getrandbits(64):016x}", "type": "message", "role": "assistant", "model": "claude-sonnet-4-5-20250929",
                           "content": content, "stop_reason": "end_turn",
                           "usage": {"input_tokens": rng.randint(5, 50), "output_tokens": rng.randint(50, 2000),
                                     "cache_read_input_tokens": rng.randint(0, 50000), "cache_creation_input_tokens": rng.randint(0, 5000)}}}
        parent = a
        if i % 3 == 0:
            r = str(uuid.UUID(int=rng.getrandbits(128)))
            t += timedelta(seconds=rng.randint(1, 20))
            yield {**base, "type": "user", "uuid": r, "parentUuid": a, "timestamp": t.isoformat().replace("+00:00", "Z"),
                   "message": {"role": "user", "content": [{"type": "tool_result", "tool_use_id": tool, "content": text(rng, rng.randint(20, 300))}]}}
            parent = r
        t += timedelta(seconds=rng.randint(5, 120))


def session_rng(seed, n):
    return random.Random(f"{seed}:{n}")


def main():
    root, count, turns = Path(sys.argv[1]), int(sys.argv[2]), int(sys.argv[3])
    seed = sys.argv[4] if len(sys.argv) > 4 else "1"
    start = datetime(2026, 10, 1, tzinfo=timezone.utc)
    for n in range(count):
        rng = session_rng(seed, n)
        project = f"project-{n % 7}"
        sid = str(uuid.UUID(int=rng.getrandbits(128)))
        d = root / f"-home-dev-{project}"
        d.mkdir(parents=True, exist_ok=True)
        with open(d / f"{sid}.jsonl", "w") as f:
            for line in session(rng, project, sid, turns, start + timedelta(hours=n * 3)):
                f.write(json.dumps(line) + "\n")


if __name__ == "__main__":
    main()
