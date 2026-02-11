# Contributing to nself Plugins

Thank you for your interest in contributing!

## 🚨 IMPORTANT: Read Standards First

**BEFORE contributing, you MUST read and follow [STANDARDS.md](STANDARDS.md).**

All plugins must comply with mandatory standards:

- ✅ Universal `np_` table prefix
- ✅ `source_account_id` multi-app isolation
- ✅ Official category assignment (1 of 13)
- ✅ Lowercase-with-hyphens naming

**Violations will cause automated build failures.**

---

## Ways to Contribute

1. **New Plugins** - Create integrations for new services
2. **Bug Fixes** - Fix issues in existing plugins
3. **Documentation** - Improve guides and examples
4. **Tests** - Add test coverage

## Development Setup

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/nself-plugins.git
   cd nself-plugins
   ```
3. Create a branch:
   ```bash
   git checkout -b feature/my-new-plugin
   ```
4. Install local policy hooks:
   ```bash
   bash .github/scripts/install-hooks.sh
   ```

## Plugin Guidelines

### Naming ⚠️ MANDATORY

- Use lowercase with hyphens: `my-service` not `MyService`
- Match the service name: `stripe`, `shopify`, `github`
- Plugin directory: `plugins/my-service/`

### Code Style

- Use Bash 3.2+ compatible syntax
- Follow existing code patterns
- Include comments for complex logic
- Use `printf` not `echo -e`

### Schema Design ⚠️ MANDATORY

**ALL tables MUST follow these rules:**

- **Prefix with `np_`**: `np_stripe_customers` not `stripe_customers`
- **Use `source_account_id`** for multi-tenant isolation
- Include standard columns: `created_at`, `synced_at`
- Use JSONB for flexible data
- Add indexes for common queries

**Example**:

```sql
CREATE TABLE np_myplugin_resources (
    id UUID PRIMARY KEY,
    source_account_id VARCHAR(255) NOT NULL,  -- REQUIRED
    name VARCHAR(255) NOT NULL,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    INDEX idx_np_myplugin_resources_account (source_account_id)
);
```

### Multi-App Configuration ⚠️ MANDATORY

**plugin.json must include**:

```json
{
  "multiApp": {
    "supported": true,
    "isolationColumn": "source_account_id",
    "pkStrategy": "uuid",
    "defaultValue": "primary"
  }
}
```

### Category Assignment ⚠️ MANDATORY

Choose ONE of the 13 official categories:

- authentication, automation, commerce, communication, content
- data, development, infrastructure, integrations, media
- streaming, sports, compliance

See [STANDARDS.md](STANDARDS.md) for category descriptions.

### Documentation

Each plugin needs:

- README in plugin directory
- Wiki page in `plugins/<Plugin>.md`
- Environment variable documentation
- Usage examples

## Pre-Submission Validation

**BEFORE submitting, run these checks:**

```bash
# 1. Validate JSON syntax
jq empty plugins/my-plugin/plugin.json

# 2. Check table prefixes (should return nothing)
jq -r '.tables[] | select(startswith("np_") | not)' plugins/my-plugin/plugin.json

# 3. Check multi-app isolation (should output: source_account_id)
jq -r '.multiApp.isolationColumn' plugins/my-plugin/plugin.json

# 4. Check category is valid
jq -r '.category' plugins/my-plugin/plugin.json
```

**All checks must pass or your PR will fail automated validation.**

## Submitting Changes

1. Commit your changes:
   ```bash
   git add .
   git commit -m "feat(stripe): add invoice sync"
   ```

2. Push to your fork:
   ```bash
   git push origin feature/my-new-plugin
   ```

3. Create a Pull Request

### Commit Messages

Follow conventional commits:
- `feat(plugin): add new feature`
- `fix(plugin): fix bug`
- `docs(plugin): update documentation`
- `test(plugin): add tests`

Authorship policy:
- Keep commit messages free of assistant/tool authorship language.
- Do not add `Co-authored-by` trailers.
- Product capability language in code/docs is allowed (for example, feature descriptions such as `AI-powered`).

## Review Process

1. Automated checks run on PR
2. Maintainer reviews code
3. Feedback addressed
4. Merged when approved

## Questions?

Open an issue for questions or discussion.
