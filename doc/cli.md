# Planner CLI

## Introduction

Planner CLI is a command line client for the `planner-api`. 

It can be used in local development to manage sources, deploy agents and generating private keys and tokens for local authentication.

## Configuration

The user must provide the server url if the backend does not run locally at `http://localhost:3443` using `--server-url flag`.

If the backend has been deployed using `local` authentication, the user must have a valid token for each request.

The token can be set using `--token` flag.


## Build

The planner cli can be build by running:
```bash
$ make build-cli
```

This is generating a binary `planner` in `bin` folder.

## Commands

The list of available commands is:
```bash
$ planner -h
planner controls the Migration Planner service.

Usage:
  planner [flags] [options]
  planner [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  create      Create a source
  delete      Delete resources by resources or owner.
  deploy      Deploy an agent
  e2e         Running the e2e test locally
  generate    Generate an image
  get         Display one or many resources.
  help        Help about any command
  sso         Generate either the token or the signing private key
  version     Print Planner version information
```

#### get
The `get` command display the available sources.
> If the backend is deployed with `local` authentication, the command return only sources owned by the jwt token'user.


#### create

The `create` command creates a sources. 
The required argument is the `name` of the source. 
```bash
$ planner create new-sources --token <jwt_token>
b1f69517-7cbe-4416-bb06-82865b76ea41
```

The returned value is the `id` of the new source.

#### delete

```bash
$ planner delete sources/b1f69517-7cbe-4416-bb06-82865b76ea41
```

#### generate

Generate an iso or ova agent's image.

The user must suply the source-id as required argument. 

The optional flags are:
```bash
Flags:
      --agent-image-url string    Quay url of the agent's image. Defaults to quay.io/kubev2v/migration-planner-agent:latest (default "quay.io/kubev2v/migration-planner-agent:latest")
  -h, --help                      help for generate
      --http-proxy string         Url of HTTP_PROXY
      --https-proxy string        Url of HTTPS_PROXY
      --image-type string         Type of the image. Only accepts ova and iso (default "ova")
      --no-proxy string           list of domains without proxy
      --output-file string        Output image file path
      --rhcos-base-image string   path to the rhcos base image
```

For example, generating an iso image:
```bash
$ planner generate d27c8245-60ab-4e83-ab00-345fff49c01a --image-type iso --output-file /tmp/image.iso
Image wrote to /tmp/image.iso
```

#### deploy

Agents can be deployed on kvm only using `deploy` command.
```bash
$ planner deploy SOURCE_ID [FLAGS] [flags]

Examples:
deploy <source_id> -s ~/.ssh/some_key.pub --name agent_vm --network bridge

Flags:
  -h, --help                  help for deploy
      --image-file string     Path the iso image. If not set the image will be generated with default values.
      --name string           Name of the vm
      --network string        Name of the network (default "default")
      --qemu-url string       Url of qemu (default "qemu:///session")
  -u, --server-url string     Address of the server (default "http://localhost:3443")
      --storage-pool string   Name of the storage pool (default "default")
      --token string          Token used to authenticate the user
```

`source_id` is required as long with the name of the agent's vm. 
It is recommanded to use a bridge network to deploy the agent. For this, use `--network` flag.

If the hypervisor is not local one, use `--qemu-url` to specify the address of the remote hypervisor.
```bash
$ planner deploy source-1 --name agent1 --qemu-url qemu+ssh://virtuser@remote-host
```

#### e2e

The e2e command is used to run the end-to-end (E2E) test suite for the Migration Planner. This is primarily intended for development and QA purposes,  
allowing you to verify the full end-to-end workflow locally using libvirt, simulated vSphere (VCSIM), and a local registry.

```bash
$ planner e2e [flags]
```
| Flag                                                                                      | Description                                                       |
|-------------------------------------------------------------------------------------------|-------------------------------------------------------------------|
| `-k`, `--keep-env`                                                                        | Keep the environment after the test completes (default: false)    |                                           |

```bash
$ planner e2e [command]
```
| command                                                                          | Description                             |
|----------------------------------------------------------------------------------|-----------------------------------------|
| `destroy`                                                                        | Destroy the E2E environment manually    |                                         

#### sso

#### private-key

Generate a private-key used for local authentication.
```bash
$ planner sso private-key
-----BEGIN RSA PRIVATE KEY-----
...
-----END RSA PRIVATE KEY-----
```

Use the generated key to set up local authentication.
```bash
$ MIGRATION_PLANNER_AUTH=local MIGRATION_PLANNER_PRIVATE_KEY=$private_key make run
```

#### token

Generate a jwt to be used when local authentication is set.

```bash
$ planner sso token --private-key $private_key --username admin --org admin
```