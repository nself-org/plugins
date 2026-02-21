# Feature Flags Plugin Testing Guide

## Manual Testing Checklist

### 1. Initialize Database
```bash
cd ts
npm run cli init
```

Expected: Schema created successfully.

### 2. Create Test Flags

#### Simple Flag (Disabled)
```bash
curl -X POST http://localhost:3305/v1/flags \
  -H "Content-Type: application/json" \
  -d '{
    "key": "test-disabled",
    "name": "Test Disabled Flag",
    "enabled": false,
    "default_value": false
  }'
```

Expected: Flag created with `enabled: false`.

#### Simple Flag (Enabled)
```bash
curl -X POST http://localhost:3305/v1/flags \
  -H "Content-Type: application/json" \
  -d '{
    "key": "test-enabled",
    "name": "Test Enabled Flag",
    "enabled": true,
    "default_value": true
  }'
```

Expected: Flag created with `enabled: true`.

#### Release Flag with Percentage Rollout
```bash
curl -X POST http://localhost:3305/v1/flags \
  -H "Content-Type: application/json" \
  -d '{
    "key": "new-feature",
    "name": "New Feature Rollout",
    "flag_type": "release",
    "enabled": true,
    "default_value": false,
    "tags": ["frontend", "beta"]
  }'
```

Expected: Flag created successfully.

### 3. Add Targeting Rules

#### Percentage Rule (25% rollout)
```bash
curl -X POST http://localhost:3305/v1/flags/new-feature/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "25% Rollout",
    "rule_type": "percentage",
    "conditions": {"percentage": 25},
    "value": true,
    "priority": 100
  }'
```

Expected: Rule created with priority 100.

#### User List Rule
```bash
curl -X POST http://localhost:3305/v1/flags/new-feature/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Beta Users",
    "rule_type": "user_list",
    "conditions": {"users": ["alice", "bob", "charlie"]},
    "value": true,
    "priority": 200
  }'
```

Expected: Rule created with higher priority (200).

#### Attribute Rule
```bash
curl -X POST http://localhost:3305/v1/flags/new-feature/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Premium Users",
    "rule_type": "attribute",
    "conditions": {
      "attribute": "plan",
      "operator": "eq",
      "attribute_value": "premium"
    },
    "value": true,
    "priority": 150
  }'
```

Expected: Rule created with priority 150.

### 4. Test Evaluations

#### Disabled Flag
```bash
curl -X POST http://localhost:3305/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flag_key": "test-disabled",
    "user_id": "user123"
  }'
```

Expected: `{"value": false, "reason": "disabled"}`

#### Enabled Flag (No Rules)
```bash
curl -X POST http://localhost:3305/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flag_key": "test-enabled",
    "user_id": "user123"
  }'
```

Expected: `{"value": true, "reason": "default"}`

#### User List Match (Highest Priority)
```bash
curl -X POST http://localhost:3305/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flag_key": "new-feature",
    "user_id": "alice"
  }'
```

Expected: `{"value": true, "reason": "rule_match"}` (matches beta users rule)

#### Attribute Match
```bash
curl -X POST http://localhost:3305/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flag_key": "new-feature",
    "user_id": "user123",
    "context": {"plan": "premium"}
  }'
```

Expected: `{"value": true, "reason": "rule_match"}` (matches premium rule)

#### Percentage Match (Consistent)
Test same user multiple times - should get same result:
```bash
# Test 1
curl -X POST http://localhost:3305/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flag_key": "new-feature",
    "user_id": "testuser456"
  }'

# Test 2 (same user)
curl -X POST http://localhost:3305/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flag_key": "new-feature",
    "user_id": "testuser456"
  }'
```

Expected: Both return same value (consistent hashing).

#### No Match - Default Value
```bash
curl -X POST http://localhost:3305/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flag_key": "new-feature",
    "user_id": "excluded-user"
  }'
```

Expected: `{"value": false, "reason": "default"}` (if user doesn't match any rule and hash > 25)

### 5. Test Segments

#### Create Segment
```bash
curl -X POST http://localhost:3305/v1/segments \
  -H "Content-Type: application/json" \
  -d '{
    "name": "premium-us-users",
    "match_type": "all",
    "rules": [
      {
        "attribute": "country",
        "operator": "eq",
        "value": "US"
      },
      {
        "attribute": "plan",
        "operator": "eq",
        "value": "premium"
      }
    ]
  }'
```

Expected: Segment created with ID.

#### Add Segment Rule to Flag
```bash
curl -X POST http://localhost:3305/v1/flags/new-feature/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "US Premium Only",
    "rule_type": "segment",
    "conditions": {"segment_id": "<SEGMENT_ID>"},
    "value": true,
    "priority": 175
  }'
```

Expected: Rule created successfully.

#### Test Segment Match
```bash
curl -X POST http://localhost:3305/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flag_key": "new-feature",
    "user_id": "user999",
    "context": {
      "country": "US",
      "plan": "premium"
    }
  }'
```

Expected: `{"value": true, "reason": "rule_match"}` (matches segment)

### 6. Test Batch Evaluation
```bash
curl -X POST http://localhost:3305/v1/evaluate/batch \
  -H "Content-Type: application/json" \
  -d '{
    "flag_keys": ["test-disabled", "test-enabled", "new-feature"],
    "user_id": "user123",
    "context": {"plan": "free"}
  }'
```

Expected: Array with results for all three flags.

### 7. Test Kill Switch
```bash
# Disable flag instantly
curl -X POST http://localhost:3305/v1/flags/new-feature/disable

# Evaluate - should return default immediately
curl -X POST http://localhost:3305/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flag_key": "new-feature",
    "user_id": "alice"
  }'
```

Expected: `{"value": false, "reason": "disabled"}` even for user "alice" who was in beta list.

### 8. Test Schedule Rule
```bash
# Create flag with schedule
curl -X POST http://localhost:3305/v1/flags \
  -H "Content-Type: application/json" \
  -d '{
    "key": "holiday-promo",
    "enabled": true,
    "default_value": false
  }'

# Add schedule rule (adjust dates as needed)
curl -X POST http://localhost:3305/v1/flags/holiday-promo/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Black Friday",
    "rule_type": "schedule",
    "conditions": {
      "start_at": "2026-11-27T00:00:00Z",
      "end_at": "2026-11-28T23:59:59Z"
    },
    "value": true,
    "priority": 100
  }'
```

Expected: Rule created. Will be active only during specified dates.

### 9. Test Statistics
```bash
curl http://localhost:3305/v1/stats
```

Expected: JSON with flag/rule/segment/evaluation counts.

### 10. Query Evaluations
```bash
curl "http://localhost:3305/v1/evaluations?flag_key=new-feature&limit=10"
```

Expected: Array of recent evaluations (if logging enabled).

## Evaluation Engine Tests

### Test Consistent Hashing
Run this multiple times to verify same user gets same result:
```bash
for i in {1..10}; do
  curl -s -X POST http://localhost:3305/v1/evaluate \
    -H "Content-Type: application/json" \
    -d '{"flag_key":"new-feature","user_id":"consistent-test"}' \
    | jq -r '.value'
done
```

Expected: All 10 results identical.

### Test Priority Order
1. Create flag
2. Add rule with priority 100 → value: "low"
3. Add rule with priority 200 → value: "high"
4. Evaluate - should return "high" (higher priority wins)

### Test Attribute Operators
```bash
# Greater than
curl -X POST http://localhost:3305/v1/flags/age-gate/rules \
  -H "Content-Type: application/json" \
  -d '{
    "rule_type": "attribute",
    "conditions": {
      "attribute": "age",
      "operator": "gte",
      "attribute_value": 18
    },
    "value": true
  }'

# Evaluate
curl -X POST http://localhost:3305/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flag_key": "age-gate",
    "context": {"age": 21}
  }'
```

Expected: `{"value": true}` for age >= 18, false otherwise.

### Test Regex Operator
```bash
curl -X POST http://localhost:3305/v1/flags/email-pattern/rules \
  -H "Content-Type: application/json" \
  -d '{
    "rule_type": "attribute",
    "conditions": {
      "attribute": "email",
      "operator": "regex",
      "attribute_value": ".*@company\\.com$"
    },
    "value": true
  }'

# Test match
curl -X POST http://localhost:3305/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flag_key": "email-pattern",
    "context": {"email": "user@company.com"}
  }'
```

Expected: `{"value": true}` for company.com emails.

## Performance Tests

### Load Test Evaluation Endpoint
```bash
# Install Apache Bench if needed: brew install httpd
ab -n 1000 -c 10 -p evaluate.json -T application/json \
  http://localhost:3305/v1/evaluate
```

Where `evaluate.json` contains:
```json
{"flag_key":"new-feature","user_id":"loadtest"}
```

Expected: Handle 1000 requests across 10 concurrent connections.

## CLI Tests

```bash
# View all flags
npm run cli flags

# Filter by type
npm run cli flags --type release

# View segments
npm run cli segments

# Evaluate from CLI
npm run cli evaluate new-feature --user alice

# Stats
npm run cli stats
```

## Multi-Account Tests

```bash
# Create flag for app1
curl -X POST http://localhost:3305/v1/flags \
  -H "Content-Type: application/json" \
  -H "X-App-Id: app1" \
  -d '{"key":"test","enabled":true,"default_value":true}'

# Create flag for app2
curl -X POST http://localhost:3305/v1/flags \
  -H "Content-Type: application/json" \
  -H "X-App-Id: app2" \
  -d '{"key":"test","enabled":true,"default_value":false}'

# List flags for app1
curl -H "X-App-Id: app1" http://localhost:3305/v1/flags

# List flags for app2
curl -H "X-App-Id: app2" http://localhost:3305/v1/flags
```

Expected: Each app sees only its own flags.

## Edge Cases

1. **Non-existent flag**: Should return reason="not_found"
2. **Missing user_id for percentage**: Should return default
3. **Invalid segment_id**: Should log warning, return default
4. **Malformed regex**: Should log warning, rule doesn't match
5. **Empty rules array**: Should return default_value
6. **Overlapping schedule rules**: Higher priority wins

## Success Criteria

- ✅ All flags CRUD operations work
- ✅ All rule types evaluate correctly
- ✅ Consistent hashing produces stable results
- ✅ Priority ordering works correctly
- ✅ Kill switch disables instantly
- ✅ Segments evaluate correctly
- ✅ Batch evaluation works
- ✅ Multi-account isolation works
- ✅ Statistics are accurate
- ✅ Evaluation logging works (when enabled)
