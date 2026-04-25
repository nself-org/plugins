---
id: certificate-expiry
title: Certificate Expiry
trigger:
  alert: CertificateExpiringSoon
  severity: warning
steps:
  - action: check_logs
    params:
      service: "nginx"
      last: "5m"
      level: "warn"
    requires_confirmation: false
  - action: run_command
    params:
      command: "nself ssl status"
    requires_confirmation: false
  - action: run_command
    params:
      command: "nself ssl renew"
    requires_confirmation: false
  - action: notify_slack
    params:
      channel: "#nself-alerts"
      message: "SSL certificate renewal attempted. Verify with: nself ssl status"
    requires_confirmation: false
  - action: escalate
    params:
      message: "SSL renewal failed — manual intervention required before certificate expiry"
    requires_confirmation: true
---

# Certificate Expiry

**Trigger:** SSL certificate expires within 14 days.

## Diagnosis

```bash
nself ssl status
# Or directly:
openssl x509 -enddate -noout -in /etc/nginx/ssl/nself.crt
```

## Resolution

1. **Attempt automatic renewal**: `nself ssl renew` (uses certbot / Let's Encrypt under the hood).
2. **Check DNS propagation** if renewal fails: `dig +short yourdomain.com` — must resolve to this server.
3. **Check port 80 is reachable** for ACME HTTP challenge: `curl http://yourdomain.com/.well-known/acme-challenge/test`
4. **Manual renewal**: `certbot renew --nginx --non-interactive`
5. **If all else fails**: generate a self-signed cert for emergency continuity, then fix DNS/firewall and renew properly.
