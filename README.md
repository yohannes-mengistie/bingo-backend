# Bingo Backend

A high-performance backend service for the Bingo bot built with Go, Gin framework, PostgreSQL, and Redis.

## Tech Stack

- **Backend**: Golang (Gin)
- **Database**: PostgreSQL
- **Cache**: Redis
- **Realtime**: WebSockets (to be implemented)
- **Security**: JWT + HTTPS + Transactions

## Architecture

This project follows Clean Architecture principles:

```
backend/
├── cmd/server/          # Application entry point
├── internal/
│   ├── domain/          # Domain entities and interfaces
│   ├── usecase/         # Business logic
│   ├── handler/         # HTTP handlers
│   └── repository/      # Data access layer
├── pkg/                 # Shared utilities
├── config/              # Configuration management
└── migrations/          # Database migrations
```

## Quick Start

**Fastest way to get started (using Docker):**

```bash
# 1. Start PostgreSQL and Redis
make docker-up

# 2. Install Go dependencies
make deps

# 3. Run the server
make run
```

Server will be available at `http://localhost:8080`

For detailed instructions, see [QUICKSTART.md](QUICKSTART.md)

## Setup

### Prerequisites

- Go 1.21 or higher
- Docker and Docker Compose (recommended)
- OR PostgreSQL 12+ and Redis 6+ installed locally

### Installation

#### Option 1: Using Docker (Recommended)

1. Start database services:
```bash
make docker-up
```

2. Install dependencies:
```bash
make deps
```

3. Run the server:
```bash
make run
```

The database will be automatically initialized on first start.

#### Option 2: Local Installation

1. Install dependencies:
```bash
go mod download
```

2. Create the database:
```bash
createdb bingo
```

3. Run migrations:
```bash
make migrate-up
```

4. (Optional) Set up environment variables:
Create a `.env` file with your configuration, or use the defaults:
- Server: `localhost:8080`
- Database: `postgres@localhost:5432/bingo`
- Redis: `localhost:6379`

5. Run the server:
```bash
make run
# Or: go run cmd/server/main.go
```

## Authentication

### POST /api/v1/auth/login

Admin login endpoint. Returns a JWT token for accessing protected admin endpoints.

**Request Body:**
```json
{
  "telegram_id": 123456789,
  "password": "admin_password"
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "telegram_id": 123456789,
    "first_name": "Admin",
    "last_name": "User",
    "phone_number": "1234567890",
    "referal_code": "ADMIN001",
    "role": "admin",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**
- `400`: Invalid request data
- `401`: Invalid credentials
- `403`: User is not an admin

**Usage:**
Include the token in the Authorization header for admin endpoints:
```
Authorization: Bearer <token>
```

## API Endpoints

### POST /api/v1/user/register

Register a new user and create their wallet. This operation is atomic - both user and wallet are created in a single transaction.

**Request Body:**
```json
{
  "telegram_id": 123456789,
  "first_name": "John",
  "last_name": "Doe",
  "phone": "+1234567890"
}
```

**Response:**
```json
{
  "message": "User and wallet created successfully",
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "telegram_id": 123456789,
    "first_name": "John",
    "last_name": "Doe",
    "phone_number": "1234567890",
    "referal_code": "ABC123XY",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  },
  "wallet": {
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "balance": 5.0,
    "demo_balance": 0.0,
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Response (409):**
```json
{
  "error": "user with this telegram ID already exists"
}
```

### GET /api/v1/user/telegram/:telegram_id

Find a user by their Telegram ID.

**Path Parameters:**
- `telegram_id` (int64, required): The Telegram ID of the user

**Response:**
```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "telegram_id": 123456789,
    "first_name": "John",
    "last_name": "Doe",
    "phone_number": "1234567890",
    "referal_code": "ABC123XY",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Response (404):**
```json
{
  "error": "User not found"
}
```

### GET /api/v1/user/phone?phone=+1234567890

Find a user by their phone number. The phone number will be automatically normalized.

**Query Parameters:**
- `phone` (string, required): The phone number of the user

**Response:**
```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "telegram_id": 123456789,
    "first_name": "John",
    "last_name": "Doe",
    "phone_number": "1234567890",
    "referal_code": "ABC123XY",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Response (404):**
```json
{
  "error": "User not found"
}
```

### GET /api/v1/user/referral/:referral_code

Find a user by their referral code.

**Path Parameters:**
- `referral_code` (string, required): The referral code of the user

**Response:**
```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "telegram_id": 123456789,
    "first_name": "John",
    "last_name": "Doe",
    "phone_number": "1234567890",
    "referal_code": "ABC123XY",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Response (404):**
```json
{
  "error": "User not found"
}
```

## Wallet Endpoints

### POST /api/v1/wallet/deposit

Create a deposit request. The transaction is created with `pending` status and **balance is NOT updated** until admin approval.

**Request Body:**
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "amount": 100.00,
  "transaction_type": "CBE",
  "transaction_id": "tx_123456789"
}
```

**Response:**
```json
{
  "message": "Deposit request created successfully",
  "transaction": {
    "id": "660e8400-e29b-41d4-a716-446655440001",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "deposit",
    "amount": 100.00,
    "status": "pending",
    "transaction_type": "CBE",
    "transaction_id": "tx_123456789",
    "reference": null,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**
- `400`: Invalid amount (must be > 0)
- `404`: User not found

### POST /api/v1/wallet/withdraw

Create a withdrawal request. The balance is **immediately subtracted** and transaction is created with `pending` status. If admin rejects, balance will be refunded.

**Request Body:**
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "amount": 50.00
}
```

**Response:**
```json
{
  "message": "Withdrawal processed successfully",
  "transaction": {
    "id": "660e8400-e29b-41d4-a716-446655440002",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "withdraw",
    "amount": 50.00,
    "status": "pending",
    "transaction_type": null,
    "transaction_id": null,
    "reference": null,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**
- `400`: Invalid amount or insufficient balance
- `404`: User or wallet not found

### POST /api/v1/wallet/transfer

Transfer money from one user to another. This is an **atomic operation** - both wallets are updated and two transactions are created in a single database transaction.

**Request Body:**
```json
{
  "sender_id": "550e8400-e29b-41d4-a716-446655440000",
  "receiver_id": "770e8400-e29b-41d4-a716-446655440000",
  "amount": 25.00
}
```

**Response:**
```json
{
  "message": "Transfer completed successfully",
  "sender_tx": {
    "id": "660e8400-e29b-41d4-a716-446655440003",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "transfer_out",
    "amount": 25.00,
    "status": "completed",
    "transaction_type": null,
    "transaction_id": null,
    "reference": "770e8400-e29b-41d4-a716-446655440000",
    "created_at": "2024-01-01T00:00:00Z"
  },
  "receiver_tx": {
    "id": "660e8400-e29b-41d4-a716-446655440004",
    "user_id": "770e8400-e29b-41d4-a716-446655440000",
    "type": "transfer_in",
    "amount": 25.00,
    "status": "completed",
    "transaction_type": null,
    "transaction_id": null,
    "reference": "550e8400-e29b-41d4-a716-446655440000",
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**
- `400`: Invalid amount, insufficient balance, or self-transfer attempt
- `404`: Sender or receiver not found

**Transfer Rules:**
- ❌ No self-transfers
- ❌ No negative or zero amounts
- ❌ Receiver must exist
- ✅ Atomic operation (all-or-nothing)

### GET /api/v1/wallet/telegram/:telegram_id

Get wallet information by Telegram ID (convenient for bot access).

**Path Parameters:**
- `telegram_id` (int64, required): The Telegram ID of the user

**Response:**
```json
{
  "wallet": {
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "balance": 75.00,
    "demo_balance": 0.00,
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Response (404):**
```json
{
  "error": "Wallet not found"
}
```

### GET /api/v1/wallet/:user_id

Get wallet information by user UUID.

**Path Parameters:**
- `user_id` (UUID, required): The user ID

**Response:**
```json
{
  "wallet": {
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "balance": 75.00,
    "demo_balance": 0.00,
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Response (404):**
```json
{
  "error": "Wallet not found"
}
```

### GET /api/v1/wallet/:user_id/deposits

Get the top 10 deposit transactions for a user, ordered by most recent first.

**Path Parameters:**
- `user_id` (UUID, required): The user ID

**Response:**
```json
{
  "deposits": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "deposit",
      "amount": 100.00,
      "status": "completed",
      "transaction_type": "CBE",
      "transaction_id": "tx_123456789",
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1
}
```

**Error Responses:**
- `400`: Invalid user ID
- `500`: Failed to fetch deposit history

### GET /api/v1/wallet/:user_id/withdrawals

Get the top 10 withdrawal transactions for a user, ordered by most recent first.

**Path Parameters:**
- `user_id` (UUID, required): The user ID

**Response:**
```json
{
  "withdrawals": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440002",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "withdraw",
      "amount": 50.00,
      "status": "completed",
      "transaction_type": null,
      "transaction_id": null,
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1
}
```

**Error Responses:**
- `400`: Invalid user ID
- `500`: Failed to fetch withdrawal history

### GET /api/v1/wallet/:user_id/transfers

Get the top 10 transfer transactions (both incoming and outgoing) for a user, ordered by most recent first.

**Path Parameters:**
- `user_id` (UUID, required): The user ID

**Response:**
```json
{
  "transfers": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440003",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "transfer_out",
      "amount": 25.00,
      "status": "completed",
      "transaction_type": null,
      "transaction_id": null,
      "reference": "770e8400-e29b-41d4-a716-446655440000",
      "created_at": "2024-01-01T00:00:00Z"
    },
    {
      "id": "660e8400-e29b-41d4-a716-446655440004",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "transfer_in",
      "amount": 15.00,
      "status": "completed",
      "transaction_type": null,
      "transaction_id": null,
      "reference": "880e8400-e29b-41d4-a716-446655440000",
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 2
}
```

**Note:** 
- `transfer_out`: Money sent to another user (reference contains receiver's user_id)
- `transfer_in`: Money received from another user (reference contains sender's user_id)

**Error Responses:**
- `400`: Invalid user ID
- `500`: Failed to fetch transfer history

## Admin Endpoints

**All admin endpoints require JWT authentication and admin role.**

Include the JWT token in the Authorization header:
```
Authorization: Bearer <your_jwt_token>
```

**Security:**
- JWT tokens expire after 24 hours (configurable via `JWT_EXPIRATION_HOURS`)
- Only users with `role='admin'` can access these endpoints
- Invalid or expired tokens will return `401 Unauthorized`
- Non-admin users will receive `403 Forbidden`

## Admin User Management Endpoints

### GET /api/v1/admin/users

Get all users with pagination.

**Query Parameters:**
- `limit` (optional): Number of users to return (default: 50)
- `offset` (optional): Number of users to skip (default: 0)

**Response:**
```json
{
  "users": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "telegram_id": 123456789,
      "first_name": "John",
      "last_name": "Doe",
      "phone_number": "1234567890",
      "referal_code": "ABC123",
      "role": "user",
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

**Note:** Passwords are never included in the response for security.

**Error Responses:**
- `401`: Unauthorized (missing or invalid token)
- `403`: Forbidden (user is not an admin)
- `500`: Failed to fetch users

## Admin Transaction Query Endpoints

All query endpoints support pagination via query parameters:
- `limit` (default: 50): Number of transactions to return
- `offset` (default: 0): Number of transactions to skip

### GET /api/v1/admin/transactions

Get all transactions with pagination.

**Query Parameters:**
- `limit` (optional): Number of transactions to return (default: 50)
- `offset` (optional): Number of transactions to skip (default: 0)

**Response:**
```json
{
  "transactions": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "deposit",
      "amount": 100.00,
      "status": "pending",
      "transaction_type": "CBE",
      "transaction_id": "tx_123456789",
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### GET /api/v1/admin/transactions/pending/deposits

Get all pending deposit transactions.

**Query Parameters:**
- `limit` (optional): Number of transactions to return (default: 50)
- `offset` (optional): Number of transactions to skip (default: 0)

**Response:**
```json
{
  "transactions": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "deposit",
      "amount": 100.00,
      "status": "pending",
      "transaction_type": "CBE",
      "transaction_id": "tx_123456789",
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### GET /api/v1/admin/transactions/pending/withdrawals

Get all pending withdrawal transactions.

**Query Parameters:**
- `limit` (optional): Number of transactions to return (default: 50)
- `offset` (optional): Number of transactions to skip (default: 0)

**Response:**
```json
{
  "transactions": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440002",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "withdraw",
      "amount": 50.00,
      "status": "pending",
      "transaction_type": null,
      "transaction_id": null,
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### GET /api/v1/admin/transactions/completed/deposits

Get all completed (approved) deposit transactions.

**Query Parameters:**
- `limit` (optional): Number of transactions to return (default: 50)
- `offset` (optional): Number of transactions to skip (default: 0)

**Response:**
```json
{
  "transactions": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "deposit",
      "amount": 100.00,
      "status": "completed",
      "transaction_type": "CBE",
      "transaction_id": "tx_123456789",
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### GET /api/v1/admin/transactions/completed/withdrawals

Get all completed (approved) withdrawal transactions.

**Query Parameters:**
- `limit` (optional): Number of transactions to return (default: 50)
- `offset` (optional): Number of transactions to skip (default: 0)

**Response:**
```json
{
  "transactions": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440002",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "withdraw",
      "amount": 50.00,
      "status": "completed",
      "transaction_type": null,
      "transaction_id": null,
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### GET /api/v1/admin/transactions/failed

Get all failed transactions (any type).

**Query Parameters:**
- `limit` (optional): Number of transactions to return (default: 50)
- `offset` (optional): Number of transactions to skip (default: 0)

**Response:**
```json
{
  "transactions": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "deposit",
      "amount": 100.00,
      "status": "failed",
      "transaction_type": "CBE",
      "transaction_id": "tx_123456789",
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### GET /api/v1/admin/transactions/transfers

Get all transfer transactions (both `transfer_in` and `transfer_out`).

**Query Parameters:**
- `limit` (optional): Number of transactions to return (default: 50)
- `offset` (optional): Number of transactions to skip (default: 0)

**Response:**
```json
{
  "transactions": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440003",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "transfer_out",
      "amount": 25.00,
      "status": "completed",
      "transaction_type": null,
      "transaction_id": null,
      "reference": "770e8400-e29b-41d4-a716-446655440000",
      "created_at": "2024-01-01T00:00:00Z"
    },
    {
      "id": "660e8400-e29b-41d4-a716-446655440004",
      "user_id": "770e8400-e29b-41d4-a716-446655440000",
      "type": "transfer_in",
      "amount": 25.00,
      "status": "completed",
      "transaction_type": null,
      "transaction_id": null,
      "reference": "550e8400-e29b-41d4-a716-446655440000",
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 2,
  "limit": 50,
  "offset": 0
}
```

## Admin Transaction Action Endpoints

### POST /api/v1/admin/transactions/:id/approve-deposit

Approve a pending deposit transaction. This will update the transaction status to `completed` and add the amount to the user's wallet balance.

**Path Parameters:**
- `id` (UUID, required): The transaction ID

**Response:**
```json
{
  "message": "Deposit approved successfully",
  "transaction": {
    "id": "660e8400-e29b-41d4-a716-446655440001",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "deposit",
    "amount": 100.00,
    "status": "completed",
    "transaction_type": "CBE",
    "transaction_id": "tx_123456789",
    "reference": null,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**
- `400`: Transaction is not a deposit or not pending
- `404`: Transaction not found

**Operation Flow:**
1. Lock wallet row (FOR UPDATE)
2. Update transaction status → `completed`
3. Add amount to wallet balance
4. Commit transaction

### POST /api/v1/admin/transactions/:id/reject-deposit

Reject a pending deposit transaction. The transaction status is updated to `failed` and **no balance change** occurs.

**Path Parameters:**
- `id` (UUID, required): The transaction ID

**Response:**
```json
{
  "message": "Deposit rejected successfully",
  "transaction": {
    "id": "660e8400-e29b-41d4-a716-446655440001",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "deposit",
    "amount": 100.00,
    "status": "failed",
    "transaction_type": "CBE",
    "transaction_id": "tx_123456789",
    "reference": null,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**
- `400`: Transaction is not a deposit or not pending
- `404`: Transaction not found

### POST /api/v1/admin/transactions/:id/approve-withdrawal

Approve a pending withdrawal transaction. The transaction status is updated to `completed`. The balance was already subtracted when the withdrawal was created.

**Path Parameters:**
- `id` (UUID, required): The transaction ID

**Response:**
```json
{
  "message": "Withdrawal approved successfully",
  "transaction": {
    "id": "660e8400-e29b-41d4-a716-446655440002",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "withdraw",
    "amount": 50.00,
    "status": "completed",
    "transaction_type": null,
    "transaction_id": null,
    "reference": null,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**
- `400`: Transaction is not a withdrawal or not pending
- `404`: Transaction not found

### POST /api/v1/admin/transactions/:id/reject-withdrawal

Reject a pending withdrawal transaction. The transaction status is updated to `failed` and the balance is **refunded** (added back to the wallet).

**Path Parameters:**
- `id` (UUID, required): The transaction ID

**Response:**
```json
{
  "message": "Withdrawal rejected and balance refunded",
  "transaction": {
    "id": "660e8400-e29b-41d4-a716-446655440002",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "withdraw",
    "amount": 50.00,
    "status": "failed",
    "transaction_type": null,
    "transaction_id": null,
    "reference": null,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**
- `400`: Transaction is not a withdrawal or not pending
- `404`: Transaction not found

**Operation Flow:**
1. Lock wallet row (FOR UPDATE)
2. Refund balance (add amount back)
3. Update transaction status → `failed`
4. Commit transaction

### POST /api/v1/admin/transactions/:id/cancel

Cancel any pending transaction. For deposits, no balance change occurs. For withdrawals, the balance is refunded.

**Path Parameters:**
- `id` (UUID, required): The transaction ID

**Response:**
```json
{
  "message": "Transaction cancelled successfully",
  "transaction": {
    "id": "660e8400-e29b-41d4-a716-446655440001",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "deposit",
    "amount": 100.00,
    "status": "cancelled",
    "transaction_type": "CBE",
    "transaction_id": "tx_123456789",
    "reference": null,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**
- `400`: Transaction is not pending
- `404`: Transaction not found

**Behavior:**
- **Deposit**: Status → `cancelled`, no balance change
- **Withdrawal**: Status → `cancelled`, balance refunded

## Database Schema

The application uses three main tables:

- **users**: User information (UUID primary key, telegram_id, phone_number, referal_code, role, password)
- **wallets**: User wallets (user_id foreign key, balance, demo_balance)
- **transactions**: Transaction ledger (deposit, withdraw, transfer_in, transfer_out)

**User Roles:**
- `user`: Regular bot users (default)
- `admin`: Admin users with access to admin endpoints

All wallet operations use database transactions with row-level locking (`FOR UPDATE`) to ensure data consistency and prevent race conditions.

## Creating Admin Users

To create an admin user:

1. **Register a user** via `/api/v1/user/register` or directly in the database
2. **Hash a password** using the helper script:
   ```bash
   go run scripts/create_admin.go your_password
   ```
3. **Update the user** in the database:
   ```sql
   UPDATE users 
   SET role = 'admin', 
       password = '$2a$10$hashed_password_here' 
   WHERE telegram_id = 123456789;
   ```

**Example:**
```bash
# 1. Hash password
go run scripts/create_admin.go myadminpassword
# Output: $2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy

# 2. Update user in database
export DOCKER_HOST=unix:///run/docker.sock
docker exec bingo-postgres psql -U postgres -d bingo -c \
  "UPDATE users SET role = 'admin', password = '\$2a\$10\$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy' WHERE telegram_id = 123456789;"
```

## Performance Optimizations

- Connection pooling for PostgreSQL (max 100 connections)
- Indexed database queries
- Efficient phone number normalization
- Unique referral code generation with collision handling
- Row-level locking for concurrent wallet operations
- Atomic transactions for transfers
- Graceful server shutdown
- Request timeouts configured

## Development

### Running Tests
```bash
go test ./...
```

### Building
```bash
make build
# Or manually:
# go build -o bin/server cmd/server/main.go
```

### Using Docker

Start PostgreSQL and Redis:
```bash
make docker-up
# Or:
# docker-compose up -d
```

The database will be automatically initialized with the schema on first start.

Stop services:
```bash
make docker-down
```

## License

MIT

