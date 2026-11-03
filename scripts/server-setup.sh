#!/bin/bash
# server-setup.sh
#
# One-time setup for rulebook.mchugh.au on Linode Debian.
# Run as root or with sudo:
#   curl -fsSL https://raw.githubusercontent.com/exploded/rulebook/main/scripts/server-setup.sh | sudo bash
#
# Assumes Caddy and the "deploy" user (with the GitHub Actions SSH key) already
# exist from an earlier project's setup. Caddy auto-provisions TLS via
# Let's Encrypt.
#
# The web app only reads rulebook.db, which is built on the PC and uploaded
# (see README, "Deploy"). Nothing on the server writes to it.

set -e

REPO_RAW="https://raw.githubusercontent.com/exploded/rulebook/main"

echo "=== rulebook - Server Deployment Setup ==="

# ---------------------------------------------------------------
# 1. Application directory
# ---------------------------------------------------------------
APP_DIR="/var/www/rulebook"
if [ -d "$APP_DIR" ]; then
    echo "[ok] Application directory $APP_DIR already exists"
else
    mkdir -p "$APP_DIR"
    chown www-data:www-data "$APP_DIR"
    echo "[ok] Created application directory $APP_DIR"
fi

# ---------------------------------------------------------------
# 2. .env template
# ---------------------------------------------------------------
ENV_FILE="$APP_DIR/.env"
if [ -f "$ENV_FILE" ]; then
    echo "[ok] .env file already exists at $ENV_FILE (not overwriting)"
else
    cat > "$ENV_FILE" << 'ENV_TEMPLATE'
PORT=8999
DB_PATH=/var/www/rulebook/rulebook.db

# Log shipping to monitor (warnings and errors). Copy MONITOR_API_KEY from
# another app's .env.
MONITOR_URL=https://monitor.mchugh.au
MONITOR_API_KEY=
ENV_TEMPLATE
    chown www-data:www-data "$ENV_FILE"
    chmod 600 "$ENV_FILE"
    echo "[ok] Created .env at $ENV_FILE - add MONITOR_API_KEY"
fi

# ---------------------------------------------------------------
# 3. systemd service
# ---------------------------------------------------------------
SERVICE_FILE="/etc/systemd/system/rulebook.service"
cat > "$SERVICE_FILE" << 'SERVICE'
[Unit]
Description=Rulebook (rulebook.mchugh.au)
After=network.target

[Service]
Type=simple
User=www-data
Group=www-data
WorkingDirectory=/var/www/rulebook
EnvironmentFile=/var/www/rulebook/.env
ExecStart=/var/www/rulebook/rulebook
Restart=on-failure
RestartSec=5

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
# The app opens the DB read-only, but SQLite may need to create a -shm file
# beside a WAL-mode database.
ReadWritePaths=/var/www/rulebook

[Install]
WantedBy=multi-user.target
SERVICE

systemctl daemon-reload
echo "[ok] Created systemd service at $SERVICE_FILE"

# ---------------------------------------------------------------
# 4. Install the deploy and database scripts (run as root via sudo).
# ---------------------------------------------------------------
for script in deploy-rulebook install-db-rulebook; do
    if curl -fsSL "$REPO_RAW/scripts/$script" -o "/usr/local/bin/$script"; then
        chmod +x "/usr/local/bin/$script"
        echo "[ok] Installed /usr/local/bin/$script"
    else
        echo "[error] Failed to download $script (the first deploy installs it)"
    fi
done

# ---------------------------------------------------------------
# 5. sudoers - allow the existing deploy user to run our deploy script
# ---------------------------------------------------------------
SUDOERS_FILE="/etc/sudoers.d/rulebook-deploy"
cat > "$SUDOERS_FILE" << 'EOF'
deploy ALL=(ALL) NOPASSWD: /usr/local/bin/deploy-rulebook
deploy ALL=(ALL) NOPASSWD: /usr/bin/systemctl stop rulebook
EOF
chmod 440 "$SUDOERS_FILE"
visudo -c -f "$SUDOERS_FILE"
echo "[ok] sudoers entry created at $SUDOERS_FILE"

# ---------------------------------------------------------------
# 6. Caddy site block - print snippet for the user to add manually
# ---------------------------------------------------------------
echo "[note] Add the following block to /etc/caddy/Caddyfile, then run:"
echo "       sudo systemctl reload caddy"
cat << 'CADDY'

rulebook.mchugh.au {
    import access_log
    encode zstd gzip
    reverse_proxy 127.0.0.1:8999 {
        import go_proxy
    }
}

CADDY

echo ""
echo "=== Setup complete ==="
echo "Next manual steps:"
echo "  1. Add MONITOR_API_KEY to $ENV_FILE"
echo "  2. Add the Caddy block above to /etc/caddy/Caddyfile, then:"
echo "     sudo systemctl reload caddy"
echo "  3. Upload the database from the PC, then:"
echo "     sudo install-db-rulebook /tmp/rulebook.db"
