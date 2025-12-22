# Collaboration Backend Design

## 1. Goals
- Support optimistic locking, multi-user presence, comments, approvals, and task tracking for configuration objects.
- Persist collaboration artifacts with full auditability and multi-tenant isolation.
- Broadcast realtime updates through a WebSocket hub that leverages existing authentication and Kafka eventing.

## 2. Data Model (MySQL)
| Table | Purpose | Key Columns | Notes |
| --- | --- | --- | --- |
| `collab_sessions` | Track active editing sessions + presence | `id`, `tenant_id`, `resource_type`, `resource_id`, `user_id`, `status`, `lock_version`, `expires_at` | Enforce optimistic lock via `lock_version`. Index on `(tenant_id, resource_type, resource_id)` |
| `collab_locks` | Durable locks, prevent conflicting edits | `id`, `session_id`, `acquired_at`, `expires_at` | Unique constraint on `(resource_type, resource_id)` |
| `collab_comments` | Threaded comments attached to resources | `id`, `tenant_id`, `resource_type`, `resource_id`, `author_id`, `parent_id`, `body`, `status`, `created_at` | `status` handles draft/resolved |
| `collab_tasks` | Assignment & tracking | `id`, `tenant_id`, `title`, `assignee_id`, `resource_ref`, `state`, `due_at`, `priority` | `resource_ref` stores `{type,id}` JSON |
| `collab_approvals` | Multi-stage approvals for change sets | `id`, `workflow_id`, `stage`, `approver_id`, `decision`, `decided_at`, `comment` | `workflow_id` references `change_requests` |
| `collab_events` | Event sourcing/analytics | `id`, `tenant_id`, `session_id`, `event_type`, `payload`, `created_at` | Append-only, used for replay + audit |

### Indices & Constraints
- All tables include `created_at`, `updated_at`, `deleted_at` (soft delete) plus `version` int for optimistic concurrency.
- Partition heavy tables (`collab_events`, `collab_comments`) by `tenant_id` to reduce contention.

### Migration Outline
Create SQL migration `0012_collaboration.sql` with the above schema, default TTL (e.g., 2 minutes) for locks enforced via `expires_at` and periodic GC job.

## 3. WebSocket Hub
### Responsibilities
1. Authenticate incoming clients using API key or JWT (reuse existing middleware).
2. Register presence + lock heartbeats tied to `collab_sessions` table.
3. Broadcast events to interested parties based on tenant and resource scopes.
4. Back-pressure + rate-limit message fan-out using per-tenant quotas.

### Architecture
```
client <--WS--> edge hub (internal/collab/hub.go)
                       |
                       +--> event router (in-memory channels)
                       +--> Kafka producer (ui.collab.events)
                       +--> storage layer (sessions/locks/comments)
```

- Hub maintains `map[tenantID]*tenantRoom`, each room manages subscribers and fan-out.
- Heartbeat interval derived from config `ui.collaboration.presenceHeartbeat` (default 15s).
- When hub detects stale heartbeat, it releases locks and emits `session.expired` event.

### Message Protocol
```json
{
  "type": "lock.acquire",
  "resource": {"type": "pool", "id": "pool-123"},
  "payload": {"intent": "edit"}
}
```
Responses follow `{ "type": "ack", "correlationId": "...", "status": "granted" }`. Errors include `reason` and `retryAfterMs`.

### Scaling Strategy
- Start with single-node hub (goroutine per connection) using `github.com/gorilla/websocket`.
- Future: sharded hubs behind load balancer; rely on Kafka to synchronize presence updates across nodes.

## 4. APIs Needed
| Endpoint | Description |
| --- | --- |
| `GET /api/v2/collab/sessions` | List active sessions for a resource |
| `POST /api/v2/collab/locks` | Request lock token |
| `DELETE /api/v2/collab/locks/{id}` | Release lock |
| `POST /api/v2/collab/comments` | Add comment |
| `GET /api/v2/collab/comments` | Paginated comments |
| `POST /api/v2/collab/tasks` | Create task |
| `PATCH /api/v2/collab/tasks/{id}` | Update task state |
| `POST /api/v2/collab/approvals/{workflowId}/decision` | Approve/Reject stage |

## 5. Next Steps
1. Implement `internal/collab` package: repository (sqlx), hub (WebSocket), service (locks/comments/tasks), DTOs.
2. Wire hub to HTTP server (`/ws/collab`).
3. Add background job that cleans expired locks/sessions.
4. Provide CLI/admin endpoint to query collaboration health.
