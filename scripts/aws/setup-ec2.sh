#!/bin/bash
# ==============================================================================
# EC2 INSTANCE SETUP SCRIPT
# ==============================================================================
#
# Run this script ON the EC2 instance after SSH'ing in.
# It creates the application directory and configuration files.
#
# USAGE:
#   1. SSH into EC2:
#      ssh -i ~/.ssh/github-analyzer-key.pem ec2-user@<IP>
#
#   2. Download and run this script:
#      curl -sL https://raw.githubusercontent.com/YOUR_USER/github-analyzer/main/scripts/aws/setup-ec2.sh -o setup-ec2.sh
#      chmod +x setup-ec2.sh
#      ./setup-ec2.sh
#
# ==============================================================================

set -e
echo "  {~} GitHub Analyzer - EC2 Instance Setup {~}"

echo ""

# Application directory
APP_DIR="/home/ec2-user/github-analyzer"


# 1. Wait for cloud init to complete ------------------------------
echo "[1/5] Waiting for cloud-init to complete..."
echo "────────────────────────────────────────────────────────────"

# cloud-init is the AWS system that runs user-data scripts
# We need to wait for Docker to be installed
sudo cloud-init status --wait 2>/dev/null || echo "  cloud-init not running"
echo "  System initialization complete"
echo ""

# 2. Verify Docker installation -----------------------------------
echo "[2/5] Verifying Docker installation..."
echo "────────────────────────────────────────────────────────────"

# Check Docker
if command -v docker &> /dev/null; then
    echo " Docker: $(docker --version)"

else
    echo "  Docker not found! Installing..."
    sudo dnf install -y docker
    sudo systemctl start docker
    sudo systemctl enable docker
    sudo usermod -aG docker ec2-user
    echo "  Docker installed. Please log out and back in for group changes."
fi    

# Check Docker Compose
if command -v docker-compose &> /dev/null; then
    echo "  Docker Compose: $(docker-compose --version)"
else
    echo "  Docker Compose not found! Installing..."
    COMPOSE_VERSION=$(curl -s https://api.github.com/repos/docker/compose/releases/latest | grep -Po '"tag_name": "\K.*?(?=")')
    sudo curl -L "https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
    sudo chmod +x /usr/local/bin/docker-compose
    sudo ln -sf /usr/local/bin/docker-compose /usr/bin/docker-compose
    echo "  Docker Compose installed"
fi

# Verify Docker daemon is running
if sudo systemctl is-active --quiet docker; then
    echo "  Docker daemon is running"
else
    echo "  Starting Docker daemon..."
    sudo systemctl start docker
fi

echo ""

# 3. Create application directory ---------------------------------
echo "[3/5] Creating application directory..."
echo "────────────────────────────────────────────────────────────"

mkdir -p $APP_DIR/migrations
mkdir -p $APP_DIR/scripts

cd $APP_DIR
echo "  Directory created: $APP_DIR"
echo ""

# 4. CREATE CONFIG FILE .env---------------------------------------
echo "[4/5] Creating configuration files..."
echo "────────────────────────────────────────────────────────────"

if [ ! -f .env ]; then
    cat > .env << 'ENVOF'
# ============================================
# PRODUCTION ENVIRONMENT CONFIGURATION
# ============================================
# Fill in ALL values before deploying!    
# -----------------------------
# Server Configuration
SERVER_PORT=3000
SERVER_ADDRESS=:3000

# Environment: development, staging, production
APP_ENV=production

# For Developmment
BASE_URL=http://localhost:3000

#---------------
# For Production
# IMPORTANT: Set this to your Elastic IP or domain name
# Example: http://54.123.45.67 or https://your-domain.com
BASE_URL=http://YOUR_ELASTIC_IP_OR_DOMAIN
#---------------

# ----------------------------- -----------------------
# Database Configuration


#postgres://user:password@host:port/database?sslmode=disable
DATABASE_URL=postgres://postgres:postgres@db:5432/github_analyzer?sslmode=disable

# Legacy PSQL variables
PSQL_HOST=db
PSQL_PORT=5432
PSQL_USER=postgres
PSQL_PASSWORD=<password>
PSQL_DATABASE=github_analyzer
PSQL_SSLMODE=disable

# -----------------------------
# Security Secrets

# CSRF secret key (32+ bytes, generate with: openssl rand -base64 32)
CSRF_SECRET=change-me-to-a-32-byte-random-string-in-production

# Encryption key(32 bytes for AES-256)
ENCRYPTION_KEY=change-me-to-a-32-byte-encryption-key-here

# Session cookie settings
SESSION_COOKIE_NAME=github_analyzer_session
SESSION_DURATION_HOURS=24

# bcrypt cost factor (12-14 recommended, higher = slower but more secure)
BCRYPT_COST=12

# -----------------------------
# GitHub OAuth2 Configuration

# callback URL to: http://localhost:3000/auth/github/callback
GITHUB_CLIENT_ID=your_github_client_id_here
GITHUB_CLIENT_SECRET=your_github_client_secret_here
GITHUB_REDIRECT_URL=http://localhost:3000/auth/github/callback

# -----------------------------
# External APIs

PERPLEXITY_API_KEY=pplx-xxxxxxxxxxxxxxxxxxxxxxxxxxxx

# Perplexity model
PERPLEXITY_MODEL=sonar

# GitHub API settings (optional, for higher rate limits)
# If not set, uses unauthenticated requests (60/hour)
# With token: 5000/hour
GITHUB_API_BASE_URL=https://api.github.com

# -----------------------------
# Rate Limiting & Quotas

# Default token quota per user (Perplexity tokens)
DEFAULT_USER_QUOTA=100000

# Maximum repositories per analysis batch
MAX_REPOS_PER_USER=50


# AWS CONFIGS ------------------------------------------------------------------

# Your AWS Account ID (12-digit number)
AWS_ACCOUNT_ID=123456789012

# AWS Region where resources are deployed
AWS_REGION=us-east-1

# Docker image tag (updated automatically by CI/CD)
IMAGE_TAG=<img_tag>  
ENVOF
    echo "  Created .env file"
    echo ""
    echo "   IMPORTANT: Edit .env with your actual values!"
    echo "     nano $APP_DIR/.env"
else
    echo "  .env file already exists"
fi

# Create Caddyfile ----------
if [ ! -f Caddyfile ]; then
    cat > Caddyfile << 'CADDYOF'
# Caddy Configuration
# For HTTPS with domain: replace :80 with your-domain.com
www.gitanalyze.online {
    reverse_proxy app:3000
    
    header {
        X-Frame-Options "DENY"
        X-Content-Type-Options "nosniff"
        X-XSS-Protection "1; mode=block"
        Referrer-Policy "strict-origin-when-cross-origin"
        -Server
    }
    
    encode gzip
}
CADDYOF
    echo "  Created Caddyfile"
    echo ""
    echo "   IMPORTANT: Edit Caddyfile with your actual values!"
    echo "     nano $APP_DIR/Caddyfile"
else
    echo "  Caddyfile file already exists"
fi  

# Create docker-compose.prod.yml
if [ ! -f docker-compose.prod.yml ]; then
    cat > docker-compose.prod.yml << 'COMPOSEOF'
version: '3.8'

services:
  # CADDY ---------------------------------------------------
  caddy:
    image: caddy:2-alpine
    container_name: github-analyzer-caddy
    restart: unless-stopped
    ports:
      - "80:80"     # HTTP (for Let's Encrypt challenge & redirect)
      - "443:443"   # HTTPS (main traffic)
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    networks:
      - frontend
    depends_on:
      app:
        condition: service_healthy
    deploy:
      resources:
        limits:
          cpus: '0.25'
          memory: 128M
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

  # DATABASE - PostgreSQL -------------------------------------
  db:
    image: postgres:15-alpine
    container_name: github-analyzer-db
    restart: unless-stopped
    environment:
      POSTGRES_USER: ${PSQL_USER}
      POSTGRES_PASSWORD: ${PSQL_PASSWORD}
      POSTGRES_DB: ${PSQL_DATABASE}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    networks:
      - backend
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${PSQL_USER} -d ${PSQL_DATABASE}"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 30s

  # MIGRATIONS ------------------------------------------------
  migrate:
    image: golang:1.24-alpine
    container_name: github-analyzer-migrate
    depends_on:
      db:
        condition: service_healthy
    volumes:
      - ./migrations:/migrations:ro
    environment:
      DATABASE_URL: postgres://${PSQL_USER:-postgres}:${PSQL_PASSWORD:-postgres}@db:5432/${PSQL_DATABASE:-github_analyzer}?sslmode=disable
    networks:
      - backend
    command: >
      sh -c "
        echo 'Installing goose...' &&
        apk add --no-cache git &&
        go install github.com/pressly/goose/v3/cmd/goose@latest &&
        echo 'Running migrations...' &&
        goose -dir /migrations postgres \"$$DATABASE_URL\" up &&
        echo 'Migrations complete!'
      "

  # APPLICATION - Go Backend ---------------------------------------------
  app:
    image: ${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/github-analyzer:${IMAGE_TAG:-latest}
    container_name: github-analyzer-app
    restart: unless-stopped
    depends_on:
      db:
        condition: service_healthy
      migrate:
        condition: service_completed_successfully
    environment:
      - APP_ENV=production
      - SERVER_PORT=3000
      - BASE_URL=${BASE_URL}
      - DATABASE_URL=postgres://${PSQL_USER:-postgres}:${PSQL_PASSWORD:-postgres}@db:5432/${PSQL_DATABASE:-github_analyzer}?sslmode=disable
      - CSRF_SECRET=${CSRF_SECRET}
      - ENCRYPTION_KEY=${ENCRYPTION_KEY}
      - GITHUB_CLIENT_ID=${GITHUB_CLIENT_ID}
      - GITHUB_CLIENT_SECRET=${GITHUB_CLIENT_SECRET}
      - GITHUB_REDIRECT_URL=${BASE_URL}/auth/github/callback
      - PERPLEXITY_API_KEY=${PERPLEXITY_API_KEY}
    networks:
      - frontend
      - backend
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:3000/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 512M
        reservations:
          cpus: '0.25'
          memory: 256M
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"      

# NETWORKS ------------------------------------------------------------
# Isolated networks for security
# Services can only talk to others on same network

networks:
  # Frontend network: Caddy <-> App
  frontend:
    driver: bridge
  
  # Backend network: App <-> Database
  backend:
    driver: bridge
  
  # NETWORK ISOLATION:
  # - Caddy can talk to App (both on frontend)
  # - App can talk to DB (both on backend)
  # - Caddy CANNOT talk to DB directly (different networks)
  # - DB CANNOT be reached from internet (not on frontend, no ports exposed)

# VOLUMES ------------------------------------------------------------

# Persistent storage that survives container restarts
volumes:
  # PostgreSQL data
  postgres_data:
    driver: local
  
  # Caddy certificate storage
  # IMPORTANT: Persist this to avoid re-requesting certs
  caddy_data:
    driver: local
  
  # Caddy config storage
  caddy_config:
    driver: local            
COMPOSEOF
    echo "  Created docker-compose.prod.yml"
    echo ""
    echo "   IMPORTANT: Edit docker-compose.prod.yml with your actual values!"
    echo "     nano $APP_DIR/docker-compose.prod.yml"
else
    echo "  docker-compose.prod.yml file already exists"
fi 


# 5. CREATE HELPER SCRIPTS -----------------------------------------
echo "[5/5] Creating helper scripts..."
echo "────────────────────────────────────────────────────────────"

# Deploy script
cat > deploy.sh << 'DEPLOYEOF'
#!/bin/bash
# Deploy latest version
set -e
cd /home/ec2-user/github-analyzer

echo "Logging into ECR..."
aws ecr get-login-password --region ${AWS_REGION:-us-east-1} | \
    docker login --username AWS --password-stdin \
    ${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION:-us-east-1}.amazonaws.com

echo "Pulling latest image..."
docker-compose -f docker-compose.prod.yml pull app

echo "Stopping old containers..."
docker-compose -f docker-compose.prod.yml down

echo "Starting new containers..."
docker-compose -f docker-compose.prod.yml up -d

echo "Cleaning up old images..."
docker image prune -f

echo ""
echo "Waiting for health check..."
sleep 10

if curl -s http://localhost/health | grep -q "ok"; then
    echo " Deployment successful!"
else
    echo " Health check failed. Check logs with: ./logs.sh"
fi
DEPLOYEOF
chmod +x deploy.sh
echo "  Created deploy.sh"

# Logs script
cat > logs.sh << 'LOGSEOF'
#!/bin/bash
# View application logs
docker-compose -f docker-compose.prod.yml logs -f ${1:-app}
LOGSEOF
chmod +x logs.sh
echo "  Created logs.sh"

# Status script
cat > status.sh << 'STATUSEOF'
#!/bin/bash
# Show container status
echo "Container Status:"
echo "─────────────────"
docker-compose -f docker-compose.prod.yml ps

echo ""
echo "Resource Usage:"
echo "───────────────"
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}"
STATUSEOF
chmod +x status.sh
echo "  Created status.sh"

# Backup script
cat > backup.sh << 'BACKUPEOF'
#!/bin/bash
# Backup PostgreSQL database
set -e
cd /home/ec2-user/github-analyzer

BACKUP_FILE="backup_$(date +%Y%m%d_%H%M%S).sql.gz"
mkdir -p backups

echo "Creating backup..."
docker-compose -f docker-compose.prod.yml exec -T db \
    pg_dump -U ${DB_USER:-postgres} ${DB_NAME:-github_analyzer} | gzip > backups/$BACKUP_FILE

echo " Backup created: backups/$BACKUP_FILE"
ls -lh backups/
BACKUPEOF
chmod +x backup.sh
echo "  ✓ Created backup.sh"

# Restart script
cat > restart.sh << 'RESTARTEOF'
#!/bin/bash
# Restart all services
docker-compose -f docker-compose.prod.yml restart
RESTARTEOF
chmod +x restart.sh
echo "   Created restart.sh"

echo ""

# ==============================================================================
# SUMMARY
# ==============================================================================
echo "============================================================"
echo "   EC2 SETUP COMPLETE!"
echo "============================================================"
echo ""
echo "FILES CREATED:"
echo "──────────────"
echo "  .env                  - Environment configuration (EDIT THIS!)"
echo "  Caddyfile             - Caddy reverse proxy config"
echo "  docker-compose.prod.yml - Docker services definition"
echo "  deploy.sh             - Deploy latest version"
echo "  logs.sh               - View container logs"
echo "  status.sh             - Check container status"
echo "  backup.sh             - Backup database"
echo "  restart.sh            - Restart services"
echo ""
echo "============================================================"
echo "  NEXT STEPS"
echo "============================================================"
echo ""
echo "1. EDIT the .env file with your values:"
echo "   ────────────────────────────────────"
echo "   nano .env"
echo ""
echo "2. COPY migration files from your local machine:"
echo "   ─────────────────────────────────────────────"
echo "   (Run this on your LOCAL machine)"
echo "   scp -i ~/.ssh/github-analyzer-key.pem -r migrations/* \\"
echo "       ec2-user@<IP>:~/github-analyzer/migrations/"
echo ""
echo "3. UPDATE GitHub OAuth callback URL to:"
echo "   ────────────────────────────────────"
echo "   http://<YOUR_IP>/auth/github/callback"
echo ""
echo "4. PUSH code to trigger deployment, OR manually deploy:"
echo "   ─────────────────────────────────────────────────────"
echo "   ./deploy.sh"
echo ""
echo "============================================================"