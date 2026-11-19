"""Sample memory and CPU of each Instance (serve process plus its parser children) from /proc.

Usage: sample.py <seconds> [interval]. Prints one CSV row per sample and a summary per Instance.
PSS splits shared pages (the binary) between Instances; anon is private heap, what scales with count.
"""

import os
import sys
import time

HZ = os.sysconf("SC_CLK_TCK")


def rollup(pid):
    out = {}
    with open(f"/proc/{pid}/smaps_rollup") as f:
        for line in f:
            k, _, v = line.partition(":")
            if k in ("Rss", "Pss", "Pss_Anon"):
                out[k] = int(v.split()[0])
    return out


def procs():
    out = {}
    for pid in filter(str.isdigit, os.listdir("/proc")):
        try:
            with open(f"/proc/{pid}/stat") as f:
                stat = f.read().rsplit(")", 1)[1].split()
            with open(f"/proc/{pid}/environ", "rb") as f:
                env = dict(e.split(b"=", 1) for e in f.read().split(b"\0") if b"=" in e)
            mem = rollup(pid)
        except OSError:
            continue
        data_dir = env.get(b"AGENTSVIEW_DATA_DIR", b"").decode()
        # utime+stime plus reaped children's time, so exited parser children still count.
        out[int(pid)] = {"ppid": int(stat[1]), "cpu": sum(int(x) for x in stat[11:15]), "mem": mem,
                         "inst": os.path.basename(data_dir) if data_dir.startswith("/data/") else ""}
    return out


def by_instance(ps):
    totals = {}
    for p in ps.values():
        # Parser children get a minimal environment, so attribute them through their parent.
        q, inst = p, p["inst"]
        while not inst and q["ppid"] in ps:
            q = ps[q["ppid"]]
            inst = q["inst"]
        if inst:
            t = totals.setdefault(inst, {"cpu": 0, "Rss": 0, "Pss": 0, "Pss_Anon": 0, "n": 0})
            t["cpu"] += p["cpu"]
            for k in ("Rss", "Pss", "Pss_Anon"):
                t[k] += p["mem"].get(k, 0)
            t["n"] += 1
    return totals


def main():
    seconds, interval = float(sys.argv[1]), float(sys.argv[2]) if len(sys.argv) > 2 else 1.0
    prev, t0 = by_instance(procs()), time.time()
    stats = {}
    print("t,instance,cpu_pct,pss_mib,anon_mib,procs")
    while time.time() - t0 < seconds:
        time.sleep(interval)
        cur = by_instance(procs())
        for inst, t in sorted(cur.items()):
            pct = max(0.0, 100.0 * (t["cpu"] - prev.get(inst, t)["cpu"]) / HZ / interval)
            print(f"{time.time() - t0:.0f},{inst},{pct:.1f},{t['Pss'] / 1024:.1f},{t['Pss_Anon'] / 1024:.1f},{t['n']}", flush=True)
            s = stats.setdefault(inst, {"cpu": 0.0, "cpu_peak": 0.0, "pss": 0.0, "anon": 0.0, "pss_sum": 0.0, "n": 0})
            s["cpu"] += pct
            s["cpu_peak"] = max(s["cpu_peak"], pct)
            s["pss"] = max(s["pss"], t["Pss"] / 1024)
            s["anon"] = max(s["anon"], t["Pss_Anon"] / 1024)
            s["pss_sum"] += t["Pss"] / 1024
            s["n"] += 1
        prev = cur
    for inst, s in sorted(stats.items()):
        print(f"# {inst}: mean_cpu_pct={s['cpu'] / s['n']:.1f} peak_cpu_pct={s['cpu_peak']:.1f} "
              f"mean_pss_mib={s['pss_sum'] / s['n']:.1f} peak_pss_mib={s['pss']:.1f} peak_anon_mib={s['anon']:.1f} samples={s['n']}")


if __name__ == "__main__":
    main()
