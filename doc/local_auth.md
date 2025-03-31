# Setup local authentication for Assisted Migration

In order to use local authentication, we must generate a RSA private key used to sign the jwt.

### Generating a private key

The private key can be generated using the cli:
```
bin/planner sso private-key
-----BEGIN RSA PRIVATE KEY-----
...
-----END RSA PRIVATE KEY-----
```

In order to pass the private key to the `planner-api`, you need to set `MIGRATION_PLANNER_PRIVATE_KEY`.
```
MIGRATION_PLANNER_PRIVATE_KEY=`bin/planner sso private-key`
```

### Executing `planner-api`

Starting `planner-api` with local authentication can be done by setting the `MIGRATION_PLANNER_AUTH=local`:
```
MIGRATION_PLANNER_AUTH=local bin/planner-api run
```

### Generating an user token

After setting up the `planner-api` to use local authentication, the user needs to generate a jwt using the same key that was provided to the `planner-api`.
```
bin/planner-api sso token --private-key $MIGRATION_PLANNER_PRIVATE_KEY --username some-user --org some-org
```
> The default expiration time for the generated jwt is 24h

If you want to examine the jwt:
```
bin/planner sso token --private-key $MIGRATION_PLANNER_PRIVATE_KEY --username some-username --org some-org | jq -R 'split(".") | .[0],.[1] | @base64d | fromjson'
{
  "alg": "RS256",
  "typ": "JWT"
}
{
  "preferred_username": "some-username",
  "org_id": "some-org",
  "iss": "test",
  "sub": "somebody",
  "aud": [
    "somebody_else"
  ],
  "exp": 1743503063,
  "nbf": 1743416663,
  "iat": 1743416663,
  "jti": "1"
}
```

### Use the token

The token can be used in any command by passing it to the flag `--token`.
```
bin/planner get --server-url localhost:3333 --token $token
```

or if you prefer `curl`:
```
curl -H "Authorization: Bearer $token" http://localhost:3443/api/v1/sources
```


