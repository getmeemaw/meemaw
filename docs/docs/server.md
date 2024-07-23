---
sidebar_position: 5
---

# Deploy Server

There are a few ways you can deploy Meemaw's server:
* [**Docker (recommended)**](#docker-recommended) : in most cases, this is what you should do.
* [Download binary](#download-binary) : if you want to run and manage the binary by yourself.
* [From source](#from-source) : if you want to contribute or modify code more granularly.

## Docker (recommended)

### Config

If you've followed the [Getting Started](/docs/getting-started), you have a `config.toml` file that looks like that:

```toml
devMode = true
port = 8421
dbConnectionUrl = 'postgresql://meemaw:meemaw@db:5432/meemaw'
clientOrigin = 'http://localhost:3000'
authType = 'supabase'
supabaseUrl = 'YOUR_SUPABASE_URL'
supabaseApiKey = 'YOUR_SUPABASE_API_KEY'
```

Before starting Meemaw with Docker, you will need to have such a config file.
Here is a list of all fields and their description:

| Field | Mandatory | Type | Default Value | Description |
|----------------------|----------------|-----------------|---------------------|---------------------|
| devMode | no | bool | true | devMode allows for unsecure connexions and more logging. Make sure to turn it off in production. |
| port | no | int | 8421 | Port where Meemaw's server should be exposed. |
| dbConnectionUrl | yes | string | - | URL to the DB in the Postgresql format. |
| clientOrigin | yes | string | - | Client origin of the web client. Basically, it should be your website URL most of the time. |
| authType | yes | string | - | Defines the Auth mechanism, whether custom or pre-integrated (e.g. Supabase) |
| authServerUrl | maybe | string | - | URL of the Auth server when using the custom integration. |
| supabaseUrl | maybe | string | - | URL of your Supabase instance when using the Supabase integration. |
| supabaseApiKey | maybe | string | - | Supabase API Key when using the Supabase integration. |

Although `authServerUrl`, `supabaseUrl` and `supabaseApiKey` are not mandatory per se, you need to provide them depending on the `authType`. If `authType=custom`, then `authServerUrl` needs to be provided. If `authType=supabase`, then `supabaseUrl` and `supabaseApiKey` need to be provided. 

You can learn more in the [Auth section](/docs/auth/integrate-auth).

### Docker Compose

If you've followed the [Getting Started](/docs/getting-started), you have a `docker-compose.yml` file that looks like that:

```yaml
# NOTE: This docker-compose.yml is just meant to get you started.
# It is not intented to be run in production as is.
# Please follow our server security guidelines : https://getmeemaw.com/docs/security

version: "3.7"

services:
  db:
    image: postgres:13
    restart: unless-stopped
    ports:
      - "9432:5432"
    networks:
      - meemaw
    environment:
      - POSTGRES_PASSWORD=meemaw
      - POSTGRES_USER=meemaw
      - POSTGRES_DB=meemaw
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U meemaw"]
      interval: 10s
      timeout: 5s
      retries: 6
    container_name: meemaw_db
    volumes:
      - type: volume
        source: meemaw-data
        target: /var/lib/postgresql/data

  app:
    image: ghcr.io/getmeemaw/meemaw:latest
    restart: unless-stopped
    ports:
      - "8421:8421"
    networks:
      - meemaw
    container_name: meemaw_app
    depends_on:
      - db
    volumes:
      - ./config.toml:/config.toml

networks:
  meemaw:

volumes:
  meemaw-data:
```

This docker-compose file works well in development as it deploys a Postgre DB on your server. It's a very simple way to get up and running quickly. However, you should not do that in production. Instead, you should have your DB hosted somewhere else, with proper access management. You can find more details in our [security guidelines](/docs/security).

This translates to a simplified docker-compose file:

```yaml
# NOTE: This docker-compose.yml is just meant to get you started.
# It is not intented to be run in production as is.
# Please follow our server security guidelines : https://getmeemaw.com/docs/security

version: "3.7"

services:
  app:
    image: ghcr.io/getmeemaw/meemaw:latest
    restart: unless-stopped
    ports:
      - "8421:8421"
    container_name: meemaw_app
    volumes:
      - ./config.toml:/config.toml
```

Once you have configured your docker-compose file, it's time to start everything:

```
docker compose up
```

Youhouu our server runs 🥳

### Reverse Proxy

If you're running Meemaw locally in development, you're good to go, you can just [configure your client](/docs/client/) to reach the server at `http://localhost:8421` if you haven't changed the port in the `config.toml` and `docker-compose.yml` files.

However, if you want to run Meemaw in production, there is still one more step to go through: expose Meemaw through a reverse proxy like Nginx or Caddy. The goal is to allow for a TLS connexion through your web domain. 

We like [Caddy](https://caddyserver.com/) as it manages the TLS certificates automatically. Here is how a very simple Caddyfile would look like:

```yaml
mydomain.com {
	handle {
		reverse_proxy 127.0.0.1:8421
	}
}
```

### Security

Just to be sure you did not miss it: if you run Meemaw in production, you should follow our [security guidelines](/docs/security).

## Download Binary

If you want to run the binary and manage it by yourself, there are at least four things you need to do :
1. [Download](https://github.com/getmeemaw/meemaw/releases) the binary for your architecture and operating system
2. [Configure](#config) Meemaw based on your DB, Auth system, etc
3. Make sure it keeps running, by using systemctl or something similar
4. Expose Meemaw with a [reverse proxy](#reverse-proxy)

If you're not sure about the third step, we recommend you use [Docker](#docker-recommended) to deploy Meemaw.

## From Source

If you want to contribute (yeay 🎉) or modify Meemaw's code more granularly, you will need to build it from source.

These docs are still a work in progress, but here is high-level what you will need to do:
- ```git clone``` the project
- ```go build``` the server
- ```gomobile bind``` the library used in the mobile SDKs
- ```GOOS=js GOARCH=wasm go build``` the wasm library for the web SDK
- publish or host the SDK packages

In general, you can learn a lot from the [modd.conf](https://github.com/getmeemaw/meemaw/blob/main/modd.conf) file.

Most importantly, if you want to contribute, join us on Discord so that we can help you 😎