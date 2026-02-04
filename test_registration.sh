#!/bin/bash

# Test Registration Endpoint
# Usage: ./test_registration.sh

BACKEND_URL="https://biruh-bingo-backend.onrender.com"

echo "Testing registration endpoint..."
echo "URL: ${BACKEND_URL}/api/v1/user/register"
echo ""

# Test with sample data
curl -X POST "${BACKEND_URL}/api/v1/user/register" \
  -H "Content-Type: application/json" \
  -d '{
    "telegram_id": 123456789,
    "first_name": "John",
    "last_name": "Doe",
    "phone": "+1234567890"
  }' \
  -w "\n\nHTTP Status: %{http_code}\n" \
  -v

echo ""
echo "Note: If you get a 409 error, the user already exists. Try with a different telegram_id."

