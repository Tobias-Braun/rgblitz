#!/usr/bin/env bash
set -euo pipefail

# Firewall for a Caddy-fronted deploy: SSH in, HTTP/HTTPS in (80 for the
# Let's Encrypt HTTP-01 challenge + redirect, 443 for the app), everything
# else stays closed.
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw --force enable

sudo ufw status verbose
