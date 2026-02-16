# Geocoding Plugin Tests

## Running Tests

The tests in this directory are integration tests that require a PostgreSQL database connection.

### Prerequisites

1. **PostgreSQL Database**: Set up a test database with the following environment variables:
   ```bash
   export POSTGRES_HOST=localhost
   export POSTGRES_PORT=5432
   export POSTGRES_DB=nself_geocoding_test
   export POSTGRES_USER=postgres
   export POSTGRES_PASSWORD=your_password
   ```

2. **Install Dependencies**:
   ```bash
   pnpm install
   ```

3. **Build the Plugin**:
   ```bash
   pnpm build
   ```

### Run Tests

```bash
pnpm test
```

## Test Coverage

### Rate Limiting Tests
- ✅ Per-user rate limits enforced
- ✅ Different users have separate rate limits
- ✅ Rate limit headers are present in responses

### Quota Tracking Tests
- ✅ API calls are tracked in database
- ✅ Cache hits vs geocode calls tracked separately
- ✅ Quota limit enforcement with 429 responses

## Test Architecture

- Tests use Node.js built-in test runner with TypeScript support via `tsx`
- Each test creates a temporary server instance with test-specific configuration
- Database quota tracking uses the same schema as production
- Rate limiting uses the `ApiRateLimiter` class with configurable limits

## Notes

- Tests create a server on a random port (`port: 0`) to avoid conflicts
- Each test properly cleans up by calling `server.stop()` in the `finally` block
- The `X-App-Name` header is used to simulate different users/tenants
- Quota tracking increments both daily and monthly counters in the database
