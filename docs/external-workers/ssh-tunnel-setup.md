# SSH Tunnel Setup

Configure persistent SSH tunnels for external worker connectivity.

## Overview

SSH tunnels provide secure connectivity between external workers and the Beta9 gateway without requiring VPN infrastructure.

## Tunnel Types

### Forward Tunnel (Agent to Gateway)

Allows the Python agent on the external worker to reach the gateway's HTTP API.

```bash
ssh -L 1994:localhost:31994 gateway-host
```

- Local port `1994` forwards to gateway's NodePort `31994`
- Used for: Registration, keepalive, API calls

### Reverse Tunnel (Gateway to k3s) - Phase 2

Allows the gateway to reach the external worker's k3s API for pod deployment.

```bash
ssh -R 6443:localhost:6443 gateway-host
```

- Remote port `6443` forwards to local k3s API
- Used for: Worker pod deployment
- **Note**: Requires additional gateway configuration

## Basic Setup

### One-Time Tunnel

```bash
# Simple forward tunnel (foreground)
ssh -L 1994:localhost:31994 user@gateway-host

# Background tunnel
ssh -fNL 1994:localhost:31994 user@gateway-host
```

Flags:
- `-f` - Fork to background
- `-N` - No remote command (tunnel only)
- `-L` - Local port forward

### Verify Tunnel

```bash
# Test gateway health through tunnel
curl http://localhost:1994/api/v1/health
# {"status":"ok"}
```

## Persistent Tunnels

### Using autossh

`autossh` automatically reconnects dropped SSH tunnels.

```bash
# Install autossh
sudo apt install autossh  # Debian/Ubuntu
brew install autossh      # macOS

# Run persistent tunnel
autossh -M 0 -f -N -L 1994:localhost:31994 user@gateway-host
```

Flags:
- `-M 0` - Disable monitoring port (use SSH keepalive instead)
- `-f` - Fork to background
- `-N` - No remote command

### SSH Config

Add to `~/.ssh/config`:

```
Host beta9-tunnel
    HostName your-gateway-host.com
    User your-username
    IdentityFile ~/.ssh/id_ed25519
    LocalForward 1994 localhost:31994
    ServerAliveInterval 30
    ServerAliveCountMax 3
    ExitOnForwardFailure yes
```

Then connect with:

```bash
ssh -fN beta9-tunnel
```

### Systemd Service

Create `/etc/systemd/system/beta9-tunnel.service`:

```ini
[Unit]
Description=Beta9 SSH Tunnel
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=beta9
ExecStart=/usr/bin/autossh -M 0 -N -L 1994:localhost:31994 beta9-tunnel
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable beta9-tunnel
sudo systemctl start beta9-tunnel
```

## Multiple Tunnels

For full external worker functionality, you may need multiple tunnels:

```bash
# Forward: Agent API access
ssh -L 1994:localhost:31994 gateway-host

# Forward: gRPC (if needed)
ssh -L 1993:localhost:31993 gateway-host

# Reverse: k3s API (for pod deployment)
ssh -R 6443:localhost:6443 gateway-host
```

Combined in SSH config:

```
Host beta9-full
    HostName your-gateway-host.com
    User your-username
    LocalForward 1994 localhost:31994
    LocalForward 1993 localhost:31993
    RemoteForward 6443 localhost:6443
    ServerAliveInterval 30
```

## Troubleshooting

### Tunnel Won't Start

```bash
# Check if port is in use
lsof -i :1994

# Kill existing tunnel
pkill -f "ssh.*1994:localhost:31994"
```

### Connection Drops

Add to SSH config:

```
ServerAliveInterval 30
ServerAliveCountMax 3
TCPKeepAlive yes
```

### Permission Denied

```bash
# Ensure SSH key is loaded
ssh-add ~/.ssh/id_ed25519

# Test SSH connection directly
ssh -v user@gateway-host
```

### Firewall Issues

```bash
# Check if gateway port is accessible
nc -zv gateway-host 31994

# If blocked, may need VPN or different network path
```

## Security Best Practices

1. **Use key-based authentication** - Disable password auth
2. **Dedicated user** - Create `beta9` user for tunnel only
3. **Restricted shell** - Limit SSH user to tunnel-only access
4. **Firewall rules** - Only allow SSH from known IPs
5. **Audit logs** - Monitor SSH connections

### Restricted SSH User

On gateway host:

```bash
# Create restricted user
sudo useradd -m -s /bin/false beta9-tunnel

# Add SSH key
sudo mkdir -p /home/beta9-tunnel/.ssh
sudo cat >> /home/beta9-tunnel/.ssh/authorized_keys << 'EOF'
restrict,port-forwarding ssh-ed25519 AAAA... worker-key
EOF

# Set permissions
sudo chown -R beta9-tunnel:beta9-tunnel /home/beta9-tunnel/.ssh
sudo chmod 700 /home/beta9-tunnel/.ssh
sudo chmod 600 /home/beta9-tunnel/.ssh/authorized_keys
```

The `restrict` option disables shell access but allows port forwarding.
