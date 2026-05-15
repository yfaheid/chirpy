# Chirpy

Chirpy is a lightweight Twitter/X-style REST API built with Go and PostgreSQL.

## What It Does

* User registration and login
* JWT-based authentication
* Create, read, and delete short posts (“chirps”)
* Refresh and revoke auth tokens
* Basic profanity filtering
* Webhook endpoint support
* Admin metrics and reset endpoints

## Tech Stack

* Go
* PostgreSQL
* SQLC
* JWT authentication

## Installation

### 1. Clone the repository

```bash
git clone https://github.com/yfaheid/chirpy.git
cd chirpy
```

### 2. Install dependencies

```bash
go mod download
```

### 3. Set up PostgreSQL

Create a PostgreSQL database and add your connection string to a `.env` file:

```env
DB_URL=postgres://USERNAME:PASSWORD@localhost:5432/chirpy?sslmode=disable
SECRET=your_jwt_secret
PLATFORM=dev
POLKA_KEY=your_webhook_key
```

### 4. Run database migrations

If you have `goose` installed, run:

```bash
goose postgres $DB_URL up
```

This applies all migrations in the `sql/schema/` directory to your database.

## Run the Project

Start the API server:

```bash
go run .
```

The server runs on:

```text
http://localhost:8080
```

## Example Endpoints

| Method | Endpoint           | Description    |
| ------ | ------------------ | -------------- |
| GET    | `/api/healthz`     | Health check   |
| POST   | `/api/users`       | Create a user  |
| POST   | `/api/login`       | Login          |
| POST   | `/api/chirps`      | Create a chirp |
| GET    | `/api/chirps`      | Get all chirps |
| DELETE | `/api/chirps/{id}` | Delete a chirp |

## Project Structure

```text
internal/        Application packages
sql/schema/      Database migrations
sql/queries/     SQL queries used by SQLC
assets/          Static assets
```
