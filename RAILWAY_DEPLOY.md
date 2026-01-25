# Railway Deployment Guide

This guide will walk you through deploying the Bingo backend to Railway.

## Prerequisites

1. A Railway account (sign up at [railway.app](https://railway.app))
2. Your code pushed to a Git repository (GitHub, GitLab, or Bitbucket)

## Step-by-Step Deployment

### Step 1: Create a New Project on Railway

1. Go to [railway.app](https://railway.app) and sign in
2. Click **"New Project"**
3. Select **"Deploy from GitHub repo"** (or your Git provider)
4. Select your repository containing the backend code
5. Railway will automatically detect it's a Go project

### Step 2: Add PostgreSQL Database

1. In your Railway project, click **"+ New"**
2. Select **"Database"** → **"Add PostgreSQL"**
3. Railway will automatically create a PostgreSQL service
4. Note the connection details (they'll be available as environment variables)

### Step 3: Add Redis (Optional but Recommended)

1. In your Railway project, click **"+ New"**
2. Select **"Database"** → **"Add Redis"**
3. Railway will automatically create a Redis service

### Step 4: Configure Environment Variables

Go to your service settings and add these environment variables:

#### Required Variables:

```bash
# Server Configuration
SERVER_PORT=8080
SERVER_HOST=0.0.0.0

# Database Configuration (Railway provides these automatically)
# These are automatically set by Railway when you add PostgreSQL:
# DATABASE_URL (Railway provides this)
# Or manually set:
DB_HOST=${{Postgres.PGHOST}}
DB_PORT=${{Postgres.PGPORT}}
DB_USER=${{Postgres.PGUSER}}
DB_PASSWORD=${{Postgres.PGPASSWORD}}
DB_NAME=${{Postgres.PGDATABASE}}
DB_SSLMODE=require

# JWT Configuration
JWT_SECRET=your-super-secret-jwt-key-change-this-in-production
JWT_EXPIRATION_HOURS=24

# Redis Configuration (if using Redis)
# Railway provides these automatically:
# REDIS_URL (Railway provides this)
# Or manually set:
REDIS_HOST=${{Redis.REDIS_HOST}}
REDIS_PORT=${{Redis.REDIS_PORT}}
REDIS_PASSWORD=${{Redis.REDIS_PASSWORD}}
```

**Important Notes:**
- Railway automatically provides `DATABASE_URL` when you add PostgreSQL
- You can reference other services using `${{ServiceName.VARIABLE}}` syntax
- Generate a strong `JWT_SECRET` (use a random string generator)

### Step 5: Update Config to Use DATABASE_URL (Optional)

Railway provides `DATABASE_URL` which is a connection string. You can either:
- Use `DATABASE_URL` directly, or
- Parse it into individual components

For now, the current config uses individual variables which Railway can provide.

### Step 6: Run Database Migrations

After deployment, you need to run migrations. You have two options:

#### Option A: Using Railway CLI (Recommended)

1. Install Railway CLI:
   ```bash
   npm i -g @railway/cli
   # Or using Homebrew:
   brew install railway
   ```

2. Login to Railway:
   ```bash
   railway login
   ```

3. Link your project:
   ```bash
   railway link
   ```

4. Run migrations:
   ```bash
   railway run psql $DATABASE_URL -f migrations/002_update_schema.sql
   railway run psql $DATABASE_URL -f migrations/003_add_auth_fields.sql
   ```

#### Option B: Using Railway Shell

1. Go to your PostgreSQL service in Railway dashboard
2. Click on **"Connect"** → **"Query"**
3. Copy and paste the contents of:
   - `migrations/002_update_schema.sql`
   - `migrations/003_add_auth_fields.sql`

### Step 7: Configure Build Settings

Railway should auto-detect Go, but verify:

1. Go to your service → **Settings** → **Build**
2. Ensure:
   - **Build Command**: `go build -o bin/server ./cmd/server`
   - **Start Command**: `./bin/server`
   - **Root Directory**: `/` (or leave empty if root)

### Step 8: Deploy

1. Railway will automatically deploy when you push to your connected branch
2. Or manually trigger a deployment from the dashboard
3. Check the **Deployments** tab for build logs

### Step 9: Verify Deployment

1. Once deployed, Railway will provide a public URL (e.g., `https://your-app.railway.app`)
2. Test the health endpoint:
   ```bash
   curl https://your-app.railway.app/health
   ```
3. Should return: `{"status":"ok"}`

### Step 10: Create Admin User

After deployment, create an admin user:

1. Register a user via API:
   ```bash
   curl -X POST https://your-app.railway.app/api/v1/user/register \
     -H "Content-Type: application/json" \
     -d '{
       "telegram_id": 123456789,
       "first_name": "Admin",
       "last_name": "User",
       "phone": "+1234567890"
     }'
   ```

2. Hash a password:
   ```bash
   # Locally
   go run scripts/create_admin.go your_password
   ```

3. Update user to admin using Railway CLI:
   ```bash
   railway run psql $DATABASE_URL -c \
     "UPDATE users SET role = 'admin', password = '\$2a\$10\$hashed_password_here' WHERE telegram_id = 123456789;"
   ```

## Environment Variables Reference

### Server Config
- `SERVER_PORT`: Port to run server on (default: 8080)
- `SERVER_HOST`: Host to bind to (use `0.0.0.0` for Railway)

### Database Config
- `DB_HOST`: PostgreSQL host (provided by Railway)
- `DB_PORT`: PostgreSQL port (provided by Railway)
- `DB_USER`: PostgreSQL user (provided by Railway)
- `DB_PASSWORD`: PostgreSQL password (provided by Railway)
- `DB_NAME`: Database name (provided by Railway)
- `DB_SSLMODE`: SSL mode (use `require` for Railway)

### JWT Config
- `JWT_SECRET`: Secret key for JWT tokens (generate a strong random string)
- `JWT_EXPIRATION_HOURS`: Token expiration in hours (default: 24)

### Redis Config (Optional)
- `REDIS_HOST`: Redis host (provided by Railway)
- `REDIS_PORT`: Redis port (provided by Railway)
- `REDIS_PASSWORD`: Redis password (if set)

## Troubleshooting

### Build Fails
- Check build logs in Railway dashboard
- Ensure Go version is compatible (Go 1.21+)
- Verify `go.mod` is correct

### Database Connection Fails
- Verify environment variables are set correctly
- Check that PostgreSQL service is running
- Ensure `DB_SSLMODE=require` for Railway's PostgreSQL

### Application Crashes
- Check application logs in Railway dashboard
- Verify all required environment variables are set
- Check database migrations have been run

### Port Issues
- Railway automatically assigns a `PORT` environment variable
- You may need to update config to use `PORT` instead of `SERVER_PORT`
- Or set `SERVER_PORT=$PORT` in environment variables

## Updating Your Application

1. Push changes to your Git repository
2. Railway will automatically detect and deploy
3. Or manually trigger deployment from dashboard

## Monitoring

- View logs: Railway dashboard → Your service → **Logs**
- View metrics: Railway dashboard → Your service → **Metrics**
- Set up alerts: Railway dashboard → Project → **Settings** → **Notifications**

## Production Checklist

- [ ] Set strong `JWT_SECRET` environment variable
- [ ] Run database migrations
- [ ] Create admin user
- [ ] Test all endpoints
- [ ] Set up custom domain (optional)
- [ ] Configure CORS for your frontend domain
- [ ] Set up monitoring and alerts
- [ ] Review and optimize connection pool settings

## Custom Domain (Optional)

1. Go to your service → **Settings** → **Networking**
2. Click **"Generate Domain"** or **"Add Custom Domain"**
3. Follow the DNS configuration instructions

## Cost Optimization

- Railway offers a free tier with usage limits
- Monitor your usage in the dashboard
- Consider upgrading if you exceed free tier limits

