# Recommendation Engine Plugin

Hybrid collaborative filtering and content-based filtering recommendation engine for nself. Replaces the Python/FastAPI CS_4 recommendation service from nself-tv with a pure TypeScript implementation.

## Features

- **Collaborative Filtering** (60% default weight): User-user cosine similarity on a sparse interaction matrix
- **Content-Based Filtering** (40% default weight): TF-IDF vectors from item metadata with pairwise cosine similarity
- **Hybrid Blending**: Weighted merge of both algorithms with score normalization to 0-1 range
- **Cold-Start Handling**: Falls back to content-based for new users, then to popularity-based
- **Redis Caching**: Optional Redis layer for sub-millisecond recommendation serving
- **Auto-Rebuild**: Background model rebuild on a configurable schedule
- **Multi-App Isolation**: Full `source_account_id` scoping across all tables and queries

## Quick Start

```bash
# Install dependencies
cd plugins/recommendation-engine/ts
pnpm install

# Initialize the database schema
pnpm start -- init

# Start the server
pnpm start
# or in development mode:
pnpm dev
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `RECOMMENDATION_PORT` | `5004` | HTTP server port |
| `POSTGRES_HOST` | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | `5432` | PostgreSQL port |
| `POSTGRES_DB` | `nself` | Database name |
| `POSTGRES_USER` | `postgres` | Database user |
| `POSTGRES_PASSWORD` | (empty) | Database password |
| `REDIS_URL` | (disabled) | Redis connection URL |
| `COLLABORATIVE_WEIGHT` | `0.6` | Weight for collaborative filtering |
| `CONTENT_WEIGHT` | `0.4` | Weight for content-based filtering |
| `CACHE_TTL_SECONDS` | `3600` | Cache time-to-live in seconds |
| `REBUILD_INTERVAL_HOURS` | `24` | Auto-rebuild interval in hours |
| `MIN_INTERACTIONS_FOR_COLLABORATIVE` | `5` | Minimum user interactions before using CF |

## API Endpoints

### GET /v1/recommendations/:userId

Get personalized recommendations for a user.

**Query Parameters:**
- `limit` (default: 20) - Maximum number of recommendations
- `type` - Filter by media type (e.g., `movie`, `tv`)

**Response:**
```json
{
  "data": [
    { "id": "media-123", "title": "The Matrix", "type": "movie", "score": 0.95, "reason": "Users with similar taste enjoyed this" }
  ],
  "total": 1
}
```

### GET /v1/similar/:mediaId

Get similar content items.

**Query Parameters:**
- `limit` (default: 10) - Maximum number of similar items

**Response:**
```json
{
  "data": [
    { "id": "media-456", "title": "Inception", "type": "movie", "similarity_score": 0.87 }
  ],
  "total": 1
}
```

### POST /v1/rebuild

Trigger a model rebuild.

**Response:**
```json
{ "started": true, "estimated_time_seconds": 12 }
```

### GET /v1/status

Get model status.

**Response:**
```json
{
  "last_rebuild": "2026-02-14T12:00:00Z",
  "item_count": 5000,
  "user_count": 1200,
  "model_ready": true,
  "rebuild_duration_seconds": 12.5
}
```

### POST /v1/items

Add or update an item profile.

**Body:**
```json
{
  "media_id": "movie-123",
  "title": "The Matrix",
  "media_type": "movie",
  "genres": ["sci-fi", "action"],
  "cast_members": ["Keanu Reeves", "Laurence Fishburne"],
  "director": "Lana Wachowski",
  "description": "A computer hacker learns about the true nature of reality.",
  "view_count": 5000,
  "avg_rating": 4.5
}
```

### POST /v1/users

Add or update a user profile.

**Body:**
```json
{
  "user_id": "user-456",
  "interaction_count": 42,
  "preferred_genres": ["sci-fi", "action"],
  "avg_rating": 3.8
}
```

### GET /health

Health check endpoint.

## CLI Commands

```bash
nself-recommendation-engine init              # Initialize database schema
nself-recommendation-engine server            # Start the HTTP server
nself-recommendation-engine status            # Show model and system status
nself-recommendation-engine rebuild           # Trigger model rebuild
nself-recommendation-engine recommendations <user_id> [limit]
nself-recommendation-engine similar <media_id> [limit]
nself-recommendation-engine stats             # Show database statistics
```

## Database Tables

| Table | Description |
|-------|-------------|
| `np_recom_user_profiles` | User preference profiles with interaction counts and genre preferences |
| `np_recom_item_profiles` | Item metadata with TF-IDF vectors |
| `np_recom_cached_recommendations` | Pre-computed recommendations with TTL expiry |
| `np_recom_similar_items` | Pre-computed item similarity pairs |
| `np_recom_model_state` | Model build state and statistics |

## Algorithm Details

### Collaborative Filtering

1. Builds a sparse user-item interaction matrix from watch history and ratings
2. Computes implicit scores: `score = (rating / 5) * 0.6 + watch_pct * 0.4`
3. Calculates cosine similarity between all user pairs
4. For target user, finds top-K similar users
5. Predicts scores for unseen items: `weighted_avg = sum(similarity * rating) / sum(|similarity|)`
6. Normalizes output scores to 0-1

### Content-Based Filtering (TF-IDF)

1. Constructs "documents" from item metadata with weighted terms:
   - Genres: 3x weight
   - Cast members: 2x weight
   - Director: 2x weight
   - Description + Title: 1x weight
2. Computes TF-IDF: `tfidf = (term_count / total_terms) * log(N / (1 + df))`
3. Calculates pairwise cosine similarity between all item vectors
4. For recommendations, aggregates similarity scores from user's liked items
5. Normalizes output scores to 0-1

### Hybrid Blend

1. Gets collaborative recommendations (weighted by `COLLABORATIVE_WEIGHT`)
2. Gets content-based recommendations (weighted by `CONTENT_WEIGHT`)
3. For items recommended by both: `score = collab_score * collab_weight + content_score * content_weight`
4. For single-source items: `score = algorithm_score * algorithm_weight`
5. Final normalization to 0-1

### Cold-Start Handling

- **New user (0 interactions)**: Returns popularity-ranked items
- **Few interactions (< MIN_INTERACTIONS_FOR_COLLABORATIVE)**: Content-based only
- **Sufficient interactions**: Full hybrid blend

## License

Source-Available
