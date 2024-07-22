---
sidebar_position: 8
---

# Security Guidelines

There is inherent security associated with using MPC wallets: nothing can be done with only the client share or only the server share. However, security remains extremely important to avoid catastrophic failures. We compiled a list of guidelines that you should carefully follow to make sure your implementation is as secure as possible.

## Server

Probably the most important part is to secure your Meemaw server.

### Encrypted communication

This is web development 101, but it is important to repeat: all communications should be encrypted. We do that by using TLS when querying the server from clients.

#### SSL certificate for server

The first step is to get an SSL certificate for your server and domain. We recommend using Caddy as a reverse proxy in front of Meemaw, as it will automagically manage your certificates and enable TLS.

#### HTTPS and WSS

The next step is to make sure your client contacts your Meemaw server using HTTPS and WSS. You do that by providing an HTTPS host to Meemaw when initialising the client SDK.

Also, importantly, you should have the Meemaw server in production mode. This will verify that communications are encrypted and forbid operations otherwise.

### Separated & protected Meemaw backend

In our [example](/docs/getting-started), we ran everything from a single server. In production, you should never do that. Instead, the Meemaw server, the web server serving the client files (or the CDN) and the database should all be different machines.

For the Meemaw server machine in particular, you should follow the usual security good practices: use ssh keys, use a firewall (only allow HTTPS and SSH, for example), only allow access through the company VPN if possible, restrict access throughout your company, etc.

### Separated & secure database

Similarly to the server machine, your database should be properly secured, whether you use a fully managed machine or a cloud database. Pay particular attention to access control.

## Client

### Web

[For the moment](/docs/client/web), our Web SDK stores TSS shares in localStorage. That makes it vulnerable to XSS attacks. You should make sure to secure your website as much as you can. You can find many resources online on how to do this ([here is one example](https://cheatsheetseries.owasp.org/cheatsheets/Cross_Site_Scripting_Prevention_Cheat_Sheet.html)).

Another potential threat is for an attacker to modify the client SDK or your javascript files on the fly. This would allow them to gain access to the client share and to forge new transactions. All it takes is a single library you use being hacked. To fight back, follow those guidelines:
- Your javascript files should be served through HTTPS so that they can't be tampered with using [MITM attacks](https://en.wikipedia.org/wiki/Man-in-the-middle_attack)
- If you're using a CDN from an external company, you should also make sure you get the right version of the files using [Subresource Integrity](https://developer.mozilla.org/en-US/docs/Web/Security/Subresource_Integrity)

:::warning
Those risks are not unique to Meemaw. Any library that you use that allows for crypto operations on the client side of Web applications presents similar risks.
:::


### Mobile

Mobile application using the Meemaw SDK have fewer risks that need to be mitigated. Applications are signed, which makes it difficult to alter static files on the go, and storage is more secure and more reliable than on Web. That being said, you should still follow the usual security good practices for your platform.