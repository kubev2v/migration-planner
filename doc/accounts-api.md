# Accounts API Frontend Contract

This document describes the current frontend-facing contract and runtime behavior of the accounts-related API.

It is based on the current OpenAPI contract plus the handler/service behavior in the backend. Where the implementation behaves more specifically than the schema alone, that behavior is called out explicitly.

## Scope

This document covers:

- `GET /api/v1/identity`
- `GET/POST /api/v1/organizations`
- `GET/PUT/DELETE /api/v1/organizations/{id}`
- `GET/POST /api/v1/organizations/{id}/users`
- `PUT/DELETE /api/v1/organizations/{id}/users/{username}`

All endpoints require authentication. Error payloads use the shared shape:

```json
{
  "message": "..."
}
```

## Core Concepts

### Identity vs User

There are two different concepts in this API:

- `Identity`: bootstrap information for the currently authenticated user
- `User`: a persisted backend account record

This distinction matters:

- `/api/v1/identity` may return a synthesized response even when no backend user record exists
- user management endpoints only deal with persisted backend users

### Organization scope

The `Organization` API model is intentionally minimal. It represents local backend organizations only and does not include:

- member usernames
- child organization IDs
- expanded nested relationships

If the frontend needs the users of an organization, it must call:

- `GET /api/v1/organizations/{id}/users`

### User organization invariant

A persisted backend user without an organization is not a valid state in this system.

In practice this means:

- every persisted `User` always has `organizationId`
- users are always created under a specific organization via `POST /api/v1/organizations/{id}/users`
- backend flows must not produce or keep a persisted user with no organization
- a person without a backend org/account mapping is represented only through synthesized `Identity` as a `regular` user

### Important identity rule

`/api/v1/identity` resolves `organizationId` with this precedence:

- if the authenticated user has a matching backend account with a local organization, `organizationId` comes from the backend account's organization mapping
- if the authenticated user does not have a backend account, `organizationId` falls back to the JWT org from authentication context

## Models

### Identity

Used only by `GET /api/v1/identity`.

```json
{
  "username": "string",
  "kind": "partner | customer | regular | admin",
  "organizationId": "string",
  "partnerId": "string?"
}
```

Fields:

- `username`: authenticated username
- `kind`: frontend bootstrap classification
- `organizationId`: resolved org identifier
- `partnerId`: present only when `kind` is `partner`, contains the partner organization ID

Behavior notes:

- `organizationId` is always present
- `organizationId` is a plain string in the contract
- for local users, it is the backend/local organization ID
- for regular users without a backend account, it falls back to the JWT org ID
- the current implementation produces `regular`, `partner`, or `admin`
- `customer` exists in the schema enum but is not currently derived by the service
- `partnerId` is `null` for `regular` and `admin` users

### Organization

```json
{
  "id": "uuid",
  "name": "string",
  "description": "string?",
  "kind": "partner | admin",
  "icon": "string",
  "company": "string",
  "createdAt": "date-time",
  "updatedAt": "date-time"
}
```

Fields:

- `id`: backend organization ID
- `name`: display name
- `description`: optional
- `kind`: only `partner` or `admin`
- `icon`: required string
- `company`: required
- `createdAt`: required
- `updatedAt`: required

Behavior notes:

- the current API model is flat and minimal
- no usernames or child-org references are returned in this model
- organization `name` is unique within a given `company`

### User

Represents a persisted backend account.

```json
{
  "username": "string",
  "email": "email",
  "organizationId": "uuid",
  "createdAt": "date-time",
  "updatedAt": "date-time?"
}
```

Fields:

- `username`: backend account username, also used in path params
- `email`: required
- `organizationId`: backend org reference
- `createdAt`: required
- `updatedAt`: optional in practice

Behavior notes:

- `User` does not carry `kind`
- a persisted `User` without `organizationId` is not possible
- regular users are not exposed as `User` resources unless they exist as backend records

### OrganizationCreate

```json
{
  "name": "string",
  "description": "string",
  "kind": "partner | admin",
  "icon": "string",
  "company": "string"
}
```

Required by contract:

- `name`
- `description`
- `kind`
- `icon`
- `company`

### OrganizationUpdate

```json
{
  "name": "string?",
  "description": "string?",
  "icon": "string?",
  "company": "string?"
}
```

All fields are optional.

Behavior note:

- organization `name` must remain unique within its `company`

### UserCreate

Used by `POST /api/v1/organizations/{id}/users`. The organization is determined by the path parameter, not the body.

```json
{
  "username": "string",
  "email": "email"
}
```

Required:

- `username`
- `email`

### UserUpdate

```json
{
  "email": "email?"
}
```

All fields are optional.

## Endpoint Behavior

### `GET /api/v1/identity`

Purpose:

- bootstrap the authenticated user for the UI

Response:

- `200` with `Identity`
- `401` if unauthenticated
- `500` on unexpected backend error

Exact behavior:

- if a backend account exists for the authenticated username:
  - `username` comes from the backend account
  - `organizationId` comes from the backend account's local organization mapping
  - `kind` is derived from the local organization kind
  - `partnerId` is set to the organization ID when `kind` is `partner`, otherwise `null`
  - currently:
    - admin org => `admin`
    - partner org => `partner` (with `partnerId`)
    - no mapped local org => `regular`
- if no backend account exists:
  - the backend synthesizes an identity response
  - `kind = regular`
  - `organizationId` falls back to the JWT org if present
  - `partnerId` is `null`

Example regular user:

```json
{
  "username": "alice",
  "kind": "regular",
  "organizationId": "jwt-org-id",
  "partnerId": null
}
```

Example local partner user:

```json
{
  "username": "bob",
  "kind": "partner",
  "organizationId": "0b67d1c0-1771-4f87-b71d-a0c7e28f5f41",
  "partnerId": "0b67d1c0-1771-4f87-b71d-a0c7e28f5f41"
}
```

Important current limitation:

- although the enum includes `customer`, the service does not currently derive that value

### `GET /api/v1/organizations`

Purpose:

- list organizations

Query params:

- `kind`: optional, `partner | admin`
- `name`: optional case-insensitive partial match
- `company`: optional case-insensitive partial match

Response:

- `200` with `Organization[]`
- `401`
- `500`

### `POST /api/v1/organizations`

Purpose:

- create an organization

Body:

- `OrganizationCreate`

Response:

- `201` with created `Organization`
- `400` for empty body or duplicate-like organization creation failure
- `401`
- `500`

Important note:

- duplicate org creation is surfaced as `400`, not `409`

### `GET /api/v1/organizations/{id}`

Purpose:

- fetch one organization by ID

Response:

- `200` with `Organization`
- `401`
- `404` if the org does not exist
- `500`

Important note:

- the returned model contains only organization metadata
- it does not include members or children

### `PUT /api/v1/organizations/{id}`

Purpose:

- update organization metadata

Body:

- `OrganizationUpdate`

Response:

- `200` with updated `Organization`
- `400` for empty body
- `401`
- `404` if the org does not exist
- `500`

### `DELETE /api/v1/organizations/{id}`

Purpose:

- delete an organization

Response:

- `200` with the deleted `Organization`
- `401`
- `404` if the org does not exist
- `500`

### `GET /api/v1/organizations/{id}/users`

Purpose:

- list backend users belonging to the organization

Response:

- `200` with `User[]`
- `401`
- `404` if the org does not exist
- `500`

### `POST /api/v1/organizations/{id}/users`

Purpose:

- create a backend user under the specified organization

Body:

- `UserCreate`

Response:

- `201` with created `User`
- `400` for empty body
- `401`
- `404` if the org does not exist
- `409` if the username already exists
- `500`

Behavior:

- the backend verifies the organization exists
- the user is created with `organizationId` set to the path org ID
- `username` must be unique across all organizations

### `PUT /api/v1/organizations/{id}/users/{username}`

Purpose:

- assign a backend user to an organization

Response:

- `200` with no response body
- `401`
- `404` if either the org or user does not exist
- `500`

Behavior:

- the backend verifies the org exists
- the backend verifies the user exists
- the user's `organizationId` is set to the provided org ID

### `DELETE /api/v1/organizations/{id}/users/{username}`

Purpose:

- delete the backend user record from the specified organization

This is a destructive operation. Because a backend user must always belong to an organization, removing a user from their organization permanently deletes the user record — not just the membership.

Response:

- `200` with no response body
- `400` if the user is not currently a member of the org in the path
- `401`
- `404` if the org or user does not exist
- `500`

Behavior:

- the backend validates the org exists
- the backend validates the user exists
- the backend checks that the user's current `organizationId` matches the path org ID
- the user record is permanently deleted from the database

## Frontend Guidance

### Use `/api/v1/identity` for session bootstrap

Frontend code should use `/api/v1/identity` to decide:

- who is logged in
- what `kind` the backend considers them
- which `organizationId` should scope the session
- whether the user is a partner (check `partnerId`)

### Use organization-scoped endpoints for user management

All user management is scoped to organizations:

- `POST /api/v1/organizations/{id}/users` to create a user
- `GET /api/v1/organizations/{id}/users` to list users in an org
- `PUT /api/v1/organizations/{id}/users/{username}` to move a user to an org
- `DELETE /api/v1/organizations/{id}/users/{username}` to permanently delete a user

### Treat `customer` as reserved for now

The contract includes `customer` in the identity enum, but the current backend implementation does not yet derive it. Frontend code should not assume it will be returned until that logic is implemented.

## Quick Reference

| Action | Endpoint |
|---|---|
| Session bootstrap | `GET /api/v1/identity` |
| List org members | `GET /api/v1/organizations/{id}/users` |
| Create user | `POST /api/v1/organizations/{id}/users` |
| Move user to org | `PUT /api/v1/organizations/{id}/users/{username}` |
| Delete user (destructive) | `DELETE /api/v1/organizations/{id}/users/{username}` |
