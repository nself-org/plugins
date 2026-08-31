# ID.me Plugin - Quick Start Guide

Get up and running with ID.me authentication in 5 minutes.

## Prerequisites

- nself CLI v0.4.8 or higher
- PostgreSQL database
- Node.js >= 20.0.0
- ID.me developer account

## Step 1: Register with ID.me

1. Visit [developers.id.me](https://developers.id.me)
2. Create a new OAuth application
3. Note your:
   - Client ID
   - Client Secret
   - Configure redirect URI: `https://your-domain.com/callback/idme`

## Step 2: Install Plugin

```bash
cd ~/Sites/nself-plugins/plugins/idme
./install.sh
```

## Step 3: Configure

```bash
cp .env.example .env
```

Edit `.env`:

```bash
IDME_CLIENT_ID=your_client_id_here
IDME_CLIENT_SECRET=your_client_secret_here
IDME_REDIRECT_URI=https://your-domain.com/callback/idme
DATABASE_URL=postgresql://user:pass@localhost:5432/nself

# Optional: Request verification for specific groups
IDME_SCOPES=openid,email,profile,military,veteran,first_responder
```

## Step 4: Install TypeScript Dependencies

```bash
cd ts
npm install
npm run build
```

## Step 5: Test

```bash
# Test configuration
nself plugin idme test

# Start server
npm run dev
```

Visit `http://localhost:3010/health` to verify the server is running.

## Step 6: Try OAuth Flow

1. Visit `http://localhost:3010/auth/idme`
2. Authenticate with ID.me
3. Grant permissions
4. You'll be redirected to the callback with user data

## Usage Examples

### Check Verification Status

```bash
nself plugin idme verify user@example.com
```

### List All Groups

```bash
nself plugin idme groups list
```

### Generate Authorization URL

```bash
nself plugin idme init auth
```

## Next Steps

- Configure webhooks for real-time updates
- Integrate with your application's authentication
- Customize badge display in your UI
- Review the [full documentation](README.md)

## Verification Groups

Add these to `IDME_SCOPES` to verify users:

- `military` - Active duty military
- `veteran` - Military veterans
- `first_responder` - First responders
- `government` - Government employees
- `teacher` - Teachers/educators
- `student` - Students
- `nurse` - Healthcare workers

## Common Issues

### "Missing required configuration"
→ Ensure all required env vars are set in `.env`

### "Database connection failed"
→ Check `DATABASE_URL` is correct

### "Invalid redirect_uri"
→ Redirect URI must match exactly what's registered in ID.me dashboard

## Production Checklist

- [ ] Use production credentials (not sandbox)
- [ ] Set `IDME_SANDBOX=false`
- [ ] Configure webhook secret
- [ ] Enable HTTPS for callback URLs
- [ ] Encrypt tokens at rest
- [ ] Set up monitoring
- [ ] Review security best practices

## Support

- Plugin Issues: https://github.com/acamarata/nself-plugins/issues
- ID.me Support: support@id.me
- Documentation: [README.md](README.md)
