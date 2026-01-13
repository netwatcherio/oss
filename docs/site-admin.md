# Site Administration

NetWatcher includes a site-wide administration panel for managing users, workspaces, and monitoring system health.

## Site Admin vs Workspace Admin

NetWatcher has two levels of administration:

| Level | Scope | Role Field |
|-------|-------|------------|
| **Site Admin** | System-wide access to all resources | `User.role = "SITE_ADMIN"` |
| **Workspace Admin** | Per-workspace management | `Member.role = "ADMIN"` or `"OWNER"` |

A user can be **both** a site admin AND have various workspace membership roles. Site admins can access the `/admin` panel and manage all resources regardless of workspace membership.

## Default Admin Bootstrap

On first deployment with an empty database, a default admin can be created automatically:

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DEFAULT_ADMIN_EMAIL` | `admin@netwatcher.local` | Email for default admin |
| `DEFAULT_ADMIN_PASSWORD` | *(none)* | Password for default admin |

If `DEFAULT_ADMIN_PASSWORD` is set and no users exist, the controller creates a verified site admin on startup.

### Example

```bash
# docker-compose.yml
services:
  controller:
    environment:
      DEFAULT_ADMIN_EMAIL: admin@mycompany.com
      DEFAULT_ADMIN_PASSWORD: ${ADMIN_INITIAL_PASSWORD}
```

## Admin Panel

Access the admin panel at `/admin` (requires SITE_ADMIN role).

### Dashboard (`/admin`)
- System statistics (users, workspaces, agents, probes)
- Per-workspace overview with member/agent counts
- Quick links to management sections

### User Management (`/admin/users`)
- List all users with search
- Edit user details (email, name, verified status)
- Promote/demote users to site admin
- Delete users

### Workspace Management (`/admin/workspaces`)
- List all workspaces with search
- View workspace details with members and agents
- Delete workspaces

### Agent Overview (`/admin/agents`)
- List all agents across all workspaces
- View online/offline status
- See agent versions and last-seen times

## API Endpoints

All endpoints require authentication with a `SITE_ADMIN` role user.

### Stats

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/admin/stats` | System-wide statistics |
| `GET` | `/admin/workspace-stats` | Per-workspace breakdown |

### Users

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/admin/users` | List users (paginated) |
| `GET` | `/admin/users/:id` | Get user details |
| `PUT` | `/admin/users/:id` | Update user |
| `DELETE` | `/admin/users/:id` | Delete user |
| `PUT` | `/admin/users/:id/role` | Set user role (USER or SITE_ADMIN) |

### Workspaces

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/admin/workspaces` | List workspaces |
| `GET` | `/admin/workspaces/:id` | Get workspace with members/agents |
| `PUT` | `/admin/workspaces/:id` | Update workspace |
| `DELETE` | `/admin/workspaces/:id` | Delete workspace |

### Workspace Members

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/admin/workspaces/:id/members` | List members |
| `POST` | `/admin/workspaces/:id/members` | Add member |
| `PUT` | `/admin/workspaces/:wID/members/:mID` | Update member role |
| `DELETE` | `/admin/workspaces/:wID/members/:mID` | Remove member |

### Agents

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/admin/agents` | List all agents (paginated) |
| `GET` | `/admin/agents/stats` | Agent statistics per workspace |

## Security Considerations

1. **Self-protection**: Admins cannot demote themselves or delete their own account
2. **Role validation**: Only `USER` and `SITE_ADMIN` roles are accepted via the API
3. **JWT verification**: All admin endpoints verify the user's role on every request
4. **Audit logging**: Consider enabling request logging for admin actions

## Promoting a User to Site Admin

### Via API

```bash
curl -X PUT http://localhost:8080/admin/users/123/role \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"role": "SITE_ADMIN"}'
```

### Via Admin Panel

1. Navigate to `/admin/users`
2. Find the user in the table
3. Click the shield icon to toggle admin status

## Troubleshooting

### "Access Denied" on /admin

Verify your user has `role = "SITE_ADMIN"`:

```sql
-- PostgreSQL
SELECT id, email, role FROM users WHERE email = 'you@example.com';

-- To promote manually:
UPDATE users SET role = 'SITE_ADMIN' WHERE email = 'you@example.com';
```

### Default Admin Not Created

Check controller logs for bootstrap messages:

```bash
docker compose logs controller | grep -i admin
```

Ensure:
- Database has no existing users
- `DEFAULT_ADMIN_PASSWORD` is set
