# Local Authentication Setup

Quick guide to configure local authentication for testing with different user types.

## Quick Start

```bash
# 1. Generate private key and tokens
./hack/create-tokens.sh

# 2. Start API with local auth
export MIGRATION_PLANNER_PRIVATE_KEY="$(cat .auth/private-key.txt)"
export MIGRATION_PLANNER_AUTH=local
make deploy-db build-api run

# 3. Create test groups (in another terminal)
./hack/create-groups.sh

# 4. Test
source .auth/tokens.env
curl -H "X-Authorization: Bearer $REGULAR_TOKEN" http://localhost:3443/api/v1/identity
```

## User Types

- **Regular User**: Basic user without special privileges
- **Partner**: Member of a "partner" type group
- **Customer**: User with an accepted partner-customer relationship
- **Admin**: Member of an "admin" type group

## Important Notes

**⚠️ Use port 3443** for API requests

**⚠️ Use header `X-Authorization`** (not `Authorization`)

## Testing with Different Users

Load tokens:
```bash
source .auth/tokens.env
```

Test each user:
```bash
# Regular user
curl -H "X-Authorization: Bearer $REGULAR_TOKEN" \
     http://localhost:3443/api/v1/identity

# Admin user
curl -H "X-Authorization: Bearer $ADMIN_TOKEN" \
     http://localhost:3443/api/v1/identity

# Partner user
curl -H "X-Authorization: Bearer $PARTNER_TOKEN" \
     http://localhost:3443/api/v1/identity

# Customer user
curl -H "X-Authorization: Bearer $CUSTOMER_TOKEN" \
     http://localhost:3443/api/v1/identity
```

## View Token Content

```bash
echo $REGULAR_TOKEN | python3 -c "import sys, base64, json; print(json.dumps(json.loads(base64.b64decode(sys.stdin.read().split('.')[1] + '==').decode('utf-8')), indent=2))"
```

## Cleanup

```bash
make kill-db
rm -rf .auth/
```


