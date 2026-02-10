# Contributing to nself Plugins

Thank you for your interest in contributing!

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

### Naming

- Use lowercase with hyphens: `my-service` not `MyService`
- Match the service name: `stripe`, `shopify`, `github`

### Code Style

- Use Bash 3.2+ compatible syntax
- Follow existing code patterns
- Include comments for complex logic
- Use `printf` not `echo -e`

### Schema Design

- Prefix tables with plugin name: `stripe_customers`
- Include standard columns: `created_at`, `synced_at`
- Use JSONB for flexible data
- Add indexes for common queries

### Documentation

Each plugin needs:
- README in plugin directory
- Wiki page in `plugins/<Plugin>.md`
- Environment variable documentation
- Usage examples

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
