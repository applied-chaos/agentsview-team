"""Write seccomp.json: Docker's default profile plus what agentsview's hosted parser sandbox needs.

Usage: mkseccomp.py <moby default.json> <out> [syscalls, default clone,mount,chroot]
Source: https://raw.githubusercontent.com/moby/moby/v27.2.0/profiles/seccomp/default.json
"""

import json
import sys

profile = json.load(open(sys.argv[1]))
profile["syscalls"].append({
    "names": sys.argv[3].split(",") if len(sys.argv) > 3 else ["clone", "mount", "chroot"],
    "action": "SCMP_ACT_ALLOW",
    "comment": "agentsview hosted parser: clone(CLONE_NEWUSER|CLONE_NEWNS|CLONE_NEWNET), then bind mounts and chroot inside that user namespace (chroot is cap-gated in the default profile)",
})
with open(sys.argv[2], "w") as out:
    json.dump(profile, out, indent=1)
    out.write("\n")
