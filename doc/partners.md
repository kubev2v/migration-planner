# Partners API

This document describes the partner feature API: how regular users request a partner, how partners manage customer requests, and how the customer relationship works.

## Scope

This document covers:

- `GET /api/v1/partners` — list available partner organizations
- `POST /api/v1/partners/{id}/request` — request to join a partner
- `GET /api/v1/partners/requests` — list own requests
- `DELETE /api/v1/partners/requests/{id}` — cancel a pending request
- `PUT /api/v1/partners/requests/{id}` — accept or reject a request (partner only)
- `GET /api/v1/partners/{id}` — get partner details (customer only)
- `DELETE /api/v1/partners/{id}` — leave a partner (customer only)
- `GET /api/v1/customers` — list customers (partner only)
- `DELETE /api/v1/customers/{username}` — remove a customer (partner only)

All endpoints require authentication. Error payloads use the shared shape:

```json
{
  "message": "..."
}
```

## Core Concepts

### User Roles

A user's role is resolved dynamically via `GET /api/v1/identity`. The partner feature introduces role transitions:

- **regular**: default. Can browse partners and submit requests.
- **customer**: a regular user whose partner request was accepted. Scoped to one partner.
- **partner**: a member of a partner organization (managed via the accounts API). Can manage customer requests.

### Request Lifecycle

```
regular user                          partner member
     |                                     |
     |--- POST /partners/{id}/request ---->|
     |         (status: awaiting)           |
     |                                     |
     |<--- PUT /partners/requests/{id} ----|
     |         (status: accepted)          |
     |                                     |
     | user is now "customer"              |
```

A request transitions through these statuses:

| Status | Meaning |
|--------|---------|
| `awaiting` | Created by user, awaiting partner decision |
| `accepted` | Partner approved — user becomes customer |
| `rejected` | Partner declined — user remains regular |

### Constraints

- A user can have at most one active request (pending or accepted) at a time.
- A customer cannot create new requests. They must leave their current partner first.
- A rejected request does not block future requests.
- Rejecting a request requires a reason.

## Models

### PartnerRequest

Represents a request from a user to join a partner organization.

```json
{
  "id": "uuid",
  "username": "string",
  "partnerId": "string",
  "requestStatus": "pending | accepted | rejected",
  "name": "string",
  "contactName": "string",
  "contactPhone": "string",
  "email": "string",
  "location": "string",
  "reason": "string?"
}
```

Fields:

- `id`: unique request identifier (UUID), returned on creation
- `username`: the requesting user
- `partnerId`: the target partner group ID
- `requestStatus`: current status
- `name`: company/organization name of the requester
- `contactName`, `contactPhone`, `email`, `location`: contact details
- `reason`: set when a request is rejected, `null` otherwise

### PartnerRequestCreate

Used by `POST /api/v1/partners/{id}/request`.

```json
{
  "name": "string",
  "contactName": "string",
  "contactPhone": "string",
  "email": "email",
  "location": "string"
}
```

All fields are required.

### PartnerRequestUpdate

Used by `PUT /api/v1/partners/requests/{id}`.

```json
{
  "status": "accepted | rejected",
  "reason": "string?"
}
```

- `status` is required
- `reason` is required when `status` is `rejected`, optional otherwise

## Endpoint Behavior

### `GET /api/v1/partners`

Purpose: list all partner organizations available to join.

Response:

- `200` with `Group[]` (only groups with `kind: partner`)
- `401`
- `500`

Any authenticated user can call this endpoint.

### `POST /api/v1/partners/{id}/request`

Purpose: submit a request to join a partner organization.

Path params:

- `id`: partner group UUID

Body: `PartnerRequestCreate`

Response:

- `201` with created `PartnerRequest` (includes `id` for subsequent operations)
- `400` if the user is not a regular user (e.g., already a customer or partner member)
- `400` if the user already has a pending request
- `401`
- `500`

Behavior:

- the request is created with `status: awaiting`
- `username` and `partnerId` are set server-side from the authenticated user and path param
- `id` (UUID) is generated server-side and returned in the response

### `GET /api/v1/partners/requests`

Purpose: list all partner requests for the authenticated user.

Response:

- `200` with `PartnerRequest[]`
- `401`
- `500`

Returns all requests (pending, accepted, rejected) for the calling user.

### `DELETE /api/v1/partners/requests/{id}`

Purpose: cancel a pending partner request.

Path params:

- `id`: request UUID (from the `id` field in `PartnerRequest`)

Response:

- `200` on success
- `401`
- `404` if the request does not exist or belongs to another user
- `500`

Behavior:

- only the user who created the request can cancel it
- the request is permanently deleted

### `PUT /api/v1/partners/requests/{id}`

Purpose: accept or reject a customer request. Partner members only.

Path params:

- `id`: request UUID

Body: `PartnerRequestUpdate`

Response:

- `200` with updated `PartnerRequest`
- `400` if body is empty, rejecting without a reason, or the caller is not a partner member
- `401`
- `403` if the caller is a partner member but the request belongs to a different partner group
- `404` if the request does not exist
- `500`

Behavior:

- the calling user must be a member of the partner group that the request targets
- accepting a request makes the requesting user a customer of that partner
- rejecting requires a `reason` field

### `GET /api/v1/partners/{id}`

Purpose: get partner organization details. Customer only.

Path params:

- `id`: partner group UUID

Response:

- `200` with `Group`
- `401`
- `404` if the user is not a customer of this partner
- `500`

### `DELETE /api/v1/partners/{id}`

Purpose: leave a partner organization. Customer only.

Path params:

- `id`: partner group UUID

Response:

- `200` on success
- `401`
- `404` if the user is not associated with this partner
- `500`

Behavior:

- the customer relationship is permanently deleted
- the user reverts to `regular` identity

### `GET /api/v1/customers`

Purpose: list all customer requests for the partner's group. Partner members only.

Response:

- `200` with `PartnerRequest[]`
- `400` if the caller is not a partner member
- `401`
- `403` reserved for future cross-group authz checks
- `500`

Returns all requests (pending, accepted, rejected) targeting the caller's partner group.

### `DELETE /api/v1/customers/{username}`

Purpose: remove a customer from the partner's group. Partner members only.

Path params:

- `username`: the customer's username

Response:

- `200` on success
- `400` if the caller is not a partner member
- `401`
- `403` reserved for future cross-group authz checks
- `404` if the customer does not exist in this group
- `500`

Behavior:

- the customer relationship is permanently deleted
- the removed user reverts to `regular` identity

## Identity Impact

The partner feature affects `GET /api/v1/identity`:

| Scenario | `kind` | `partnerId` |
|----------|--------|-------------|
| No accepted request, not a group member | `regular` | `null` |
| Has an accepted partner request | `customer` | partner group UUID |
| Is a member of a partner group | `partner` | `null` |

Note: `partnerId` on the identity is set for customers (pointing to their partner), not for partner members. Partner members use `groupId` instead.

## Authorization

The partner feature uses a two-layer authorization model: the **service layer** validates business rules (correct user kind), and the **authz layer** (`AuthzPartnerService`) enforces cross-group ownership checks.

### 400 vs 403

- **400 (Bad Request)**: the caller's identity kind is wrong for the operation. A regular user calling `PUT /partners/requests/{id}` or a customer calling `POST /partners/{id}/request` gets 400 — they are not the right kind of user.
- **403 (Forbidden)**: the caller has the right identity kind but is acting on a resource they don't own. A partner member trying to accept a request that belongs to a different partner group gets 403.

### Where checks happen

| Check | Layer | Error |
|-------|-------|-------|
| User is not a regular user (CreateRequest) | Service | 400 |
| User is not a partner (ListCustomers, RemoveCustomer) | Service | 400 |
| User is not a partner (UpdateRequest) | Authz | 400 |
| Partner's group ≠ request's partnerId (UpdateRequest) | Authz | 403 |
| Request belongs to another user (CancelRequest) | Service | 404 |
| User is not a customer of this partner (GetPartner) | Service | 404 |

### Design rationale

- `ListCustomers` and `RemoveCustomer` don't need authz-layer checks because the inner service already filters by the caller's group. A partner can only see/remove their own customers — there's no resource ID from another group to cross-check.
- `UpdateRequest` needs the authz layer because the request ID in the path could belong to any partner group. The authz layer loads the request, compares `partnerId` to the caller's group, and returns 403 on mismatch.
- Non-partner callers get 400 (not 403) because the issue is not authorization — it's that the endpoint doesn't apply to their user kind.

## Quick Reference

| Action | Endpoint | Who |
|--------|----------|-----|
| Browse partners | `GET /api/v1/partners` | any user |
| Request to join | `POST /api/v1/partners/{id}/request` | regular user |
| List own requests | `GET /api/v1/partners/requests` | any user |
| Cancel request | `DELETE /api/v1/partners/requests/{id}` | request owner |
| Accept/reject request | `PUT /api/v1/partners/requests/{id}` | partner member |
| View partner details | `GET /api/v1/partners/{id}` | customer |
| Leave partner | `DELETE /api/v1/partners/{id}` | customer |
| List customers | `GET /api/v1/customers` | partner member |
| Remove customer | `DELETE /api/v1/customers/{username}` | partner member |
