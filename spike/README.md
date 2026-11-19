# Spike: hosted agentsview inside the Platform container

Evidence for ticket #2 and ADR 0003. One container stands in for the Platform: it provisions each Instance as the non-superuser `platform` role and runs each as a child `agentsview replica pg serve` with hosted raw derivation on. Non-root, no capabilities, `no-new-privileges`, read-only root, seccomp `seccomp.json`. Throwaway: no TLS, fixed passwords.

```sh
cd spike
docker compose up -d --build                                    # Instances alice :8081, admin :8082
docker compose exec platform /spike/enrol.sh alice laptop
SESSIONS=50 TURNS=20 RUN_ID=g1 docker compose run --rm host      # synthetic Claude sessions, backfilled to alice
SESSIONS=50 TURNS=40 RUN_ID=g2 docker compose run --rm host      # same files, longer: second generation
docker compose exec platform /spike/wait-parsed.sh alice
```

| Question | Run |
| --- | --- |
| Sandbox | `PLATFORM_SECCOMP=builtin` (Docker 29; on older Docker pass Docker's own `default.json`) fails closed with `hosted parser isolation unavailable`. `seccomp.json` comes from `python3 mkseccomp.py default.json seccomp.json`. On an AppArmor host, `PLATFORM_APPARMOR=docker-default` also fails. |
| Provisioning | `/data/<instance>.provision.log`. Restarting `platform` re-provisions every Instance over existing data. |
| Resources | `docker compose exec platform python3 /spike/sample.py 60 2`, idle or during a `host` run. `INSTANCES="$(echo i{01..20})" docker compose up -d` for many. |
| Replay | `docker compose exec platform /spike/replay.sh alice admin alice@example.com/laptop`, then `wait-parsed.sh admin` and `diff` the output of `compare.sh alice` and `compare.sh admin`. |

`docker compose down -v` removes everything (`docker volume rm avspike_host-checkpoint` if left).
