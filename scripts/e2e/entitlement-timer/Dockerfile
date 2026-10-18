# Purpose: a throwaway debian:12 host with systemd and cron for scripts/e2e/entitlement-timer.sh.
# Run privileged by the script only, and removed on exit. Never published.
FROM debian:12
RUN apt-get update \
 && apt-get install -y --no-install-recommends systemd systemd-sysv cron procps ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && useradd -m -u 1000 -s /bin/sh app \
 && chmod 755 /home/app \
 && systemctl mask getty.target console-getty.service systemd-logind.service
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
