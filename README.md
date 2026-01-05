# GitHub Analyzer
### https://www.gitanalyze.online

AI-powered code analysis for GitHub repositories. Analyze your code for bugs, security vulnerabilities, and performance issues using AI.

![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)

## Features

- Deep code analysis powered by Perplexity AI
- Security vulnerability detection (SQL injection, XSS, auth issues)
- Performance analysis (N+1 queries, memory leaks)
- GitHub OAuth integration
- Token-based usage quotas

## Tech Stack

Go, PostgreSQL, Chi router, Perplexity AI, Docker, Tailwind CSS

## Getting Started

### Prerequisites

- Go 1.24+
- PostgreSQL 15+
- Docker & Docker Compose (optional)
- GitHub OAuth App credentials
- Perplexity API key

### Docker Setup (Recommended)

```bash
git clone https://github.com/rahul4469/github-analyzer.git
cd github-analyzer
cp .env.example .env
# Edit .env with your credentials

docker-compose up -d
```

Visit http://localhost:3000

### Manual Setup

```bash
git clone https://github.com/rahul4469/github-analyzer.git
cd github-analyzer

go mod download
createdb github_analyzer
make migrate-up

cp .env.example .env
# Edit .env

make run
```

## Configuration

### Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `DATABASE_URL` | PostgreSQL connection string | Yes |
| `CSRF_SECRET` | CSRF token secret (32+ chars) | Yes |
| `ENCRYPTION_KEY` | AES-256 key (32 chars) | Yes |
| `GITHUB_CLIENT_ID` | GitHub OAuth client ID | Yes |
| `GITHUB_CLIENT_SECRET` | GitHub OAuth client secret | Yes |
| `PERPLEXITY_API_KEY` | Perplexity AI API key | Yes |
| `APP_ENV` | Environment (development/production) | No |
| `SERVER_PORT` | HTTP port (default: 3000) | No |
| `BASE_URL` | Public URL for OAuth callbacks | No |

### Setup OAuth

Create a GitHub OAuth App at https://github.com/settings/developers with callback URL `http://localhost:3000/auth/github/callback`. Add the credentials to your `.env` file.

Get a Perplexity API key from https://www.perplexity.ai/settings/api and add it to `.env`.

## AWS Deployment

Run `./scripts/aws/setup-aws.sh` to create infrastructure (ECR, EC2, security groups). Then SSH to the instance and run the setup script. Costs ~$10-15/month after free tier.

## Development

```bash
make run          # Start server
make test         # Run tests
make migrate-up   # Run migrations
make lint         # Run linter
```


## Security

Bcrypt password hashing, AES-256 token encryption, CSRF protection, parameterized queries.
