---
sidebar_position: 3
"sidebar_label": "How Does It Work?"
---

# Meemaw: how does it work?

:::info
If you're not yet familiar with MPC wallets and TSS, we suggest you read [our blog post on the subject](/blog/mpc-wallet) first. 
:::

Meemaw is a wallet as a service, which means that it allows you as an developer to deploy a trustless wallet for each of your user. Those wallets are trustless because they are MPC wallets using TSS.

This page goes into more details about how Meemaw actually implements those MPC wallets, as a service.

## Components of a Meemaw installation

Meemaw can be understood as a set of 3 main components :
* **Server:** the server runs on your backend (self-hosting) or on our servers (cloud). It generates and stores the [server-side TSS shares](/blog/mpc-wallet) of the wallets and manages the TSS process from the server side. It also checks if your users are who they say they are, and only lets them access their own wallets.
* **Client SDK:** in order to manage [the client-side TSS shares](/blog/mpc-wallet) of your users' wallets, you can use Meemaw's client SDKs for each platform. It generates and stores the shares on the device and manages the TSS process from the client side. It also makes it easy to sign messages and transactions on Ethereum and other EVM blockchains.
* **Auth provider:** Meemaw purposefully does not provide an auth provider. Instead, it makes it [easy to plug your own](/docs/auth/integrate-auth). Whether you use a custom auth system or use a third-party one, you will be able to link it to your Meemaw setup. For some third-party providers, it's as simple as providing your API key.

In order to get started, you need to run each of these components. Don't worry, [it's super easy](/docs/getting-started) 👌

## What happens behind the scenes?

During normal operations, the TSS process happens between the server and the device used by the user, thanks to the client SDK. The server communicates with the Auth provider to authenticate the user.

**When the wallet gets created (DKG)**, the server confirms with the Auth provider that the user is who he says he is. Then the server and the device work in concert through the TSS process to generate separate shares. There is no "complete" wallet private key that ever appears on either side. The shares get stored separately, one in the server database, one in the client storage.

**When a transaction needs to be signed (SIGN)**, the server confirms again with the Auth provider that the user is who he says he is. Then the server and the device work in concert through the TSS process to iteratively sign the transaction using their shares. Again, there is no "complete" wallet private key that ever appears on either side. The transaction ends up being fully signed and ready to be broadcast to the blockchain.

Meemaw requires 2 signatures out of N shares to send funds. There will always be at least a server share and a client share. On top of that, the user can add other devices or generate a backup, which will create additional shares. Those shares can be used in different ways depending on the use case:

## Other flows

### Add device (to be implemented)
Meemaw will allow you to add additional devices. Read further to understand why this is useful. In order to add devices, the user will need to have access to all existing shares, so that new shares can be generated.

### Backup (to be implemented)
Another way to add shares will be to download a backup file. This would allow the user to recover funds based on that backup file and the server share, even if the user looses access to all his devices.

### No server (to be implemented)
When the user wants to use funds without going through the server, he will have the possibility of signing transactions using two client shares, for example using two different devices. This allows for clients' funds not to be blocked by the server, ever. 

Note: we will open source a small utility that will help with the process outside of a given Meemaw deployment. This will allow anyone to recover their funds if a company goes rogue, gets hacked, gets bankrupt, etc.

## Technical implementation

Meemaw does not reinvent the wheel. It uses battle tested technologies for all critical components.

The technical implementation of Meemaw revolves around a few elements:

### Go library
Meemaw uses a central Go library, based on the audited [Alice implementation](https://github.com/getamis/alice) of TSS for all MPC operations.

The library is used in multiple parts of the project, leading to a few releases:
- server: running on your backend or on our cloud
- wasm: used by the Web SDK, running in Web browsers
- mobile libraries: used by the Swift and Android packages

### Websocket
For now, the transport layer for TSS operations is Websockets. A secure connection is established between the client and the server, allowing for TSS processes to happen.

### Storage
The DKG process of the TSS protocol generates multiple keys (or shares) that need to be stored by each participant. Here is how those keys are stored:
- iOS: storage on iOS devices uses the Secure Enclave of the device
- Web: storage on Web browsers uses localstorage - [this will need to be improved](/docs/client/web)
- Android (to be implemented): storage on Android devices uses the Trusted Execution Environment of the device