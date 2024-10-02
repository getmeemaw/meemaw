<p align="center">
    <img src="https://getmeemaw.com/static/img/logo/grandma.webp" width="180px" />
</p>

<h1 align="center">Meemaw</h1>

<h3 align="center">Trustless and Grandma-friendly Wallet as a Service</h3>

<p align="center">
    <img src="https://img.shields.io/badge/Latest-v1.2.0-blue"/>
    <img src="https://img.shields.io/badge/Stability-Beta-orange"/>
    <img src="https://img.shields.io/badge/License-AGPL%20v3-green.svg"/>
</p>

<br />

Deploy a trustless wallet for each of your users, in a few lines of code 🚀 
* Join us on [Discord](https://discord.gg/uf8Uzqp2)
* Get started [with our docs](https://getmeemaw.com/docs/getting-started)
* Hosted version with [Meemaw Cloud](https://getmeemaw.com)

Table of Contents
=================

  * [Introduction](#introduction)
  * [Features](#features)
  * [Getting Started](#getting-started)
      * [Web](#web)
      * [iOS](#ios)
  * [Documentation](#documentation)
  * [Contribute](#contribute)
  * [License](#license)

# Introduction

Onboarding users on Web3 projects is a pain. You mostly have a choice between 3 options: 
* letting your users fully manage their wallet with a terrible UX 🤮
* trusting another company with the private keys of your users 😨
* taking the burden on yourself and assume the risk of full custody 🤯

Not any more. We are building solutions to make it easier and more secure for developers to onboard new people to Web3, at scale.

Right now, **Meemaw allows you to deploy a trustless MPC-TSS wallet for each of your users.** [It only takes a few lines of code](#getting-started), even your Grandma would want to use it. It's also way less risky for you than trusting another company or holding the whole wallets yourself, as your server never sees the private keys.

# Features

Available right now:

- [x] **Trustless**: Meemaw wallets are MPC-TSS (Multi-Party Computation with Threshold Signature Scheme) wallets. They are non-custodial and zero-knowledge: the private keys of your users never appear on your servers so they don't need to trust you with their assets.
- [x] **Easy**: You can easily deploy Meemaw with Docker. You can then use the client SDKs to easily integrate it in a few lines of code. It's easy to offer an amazing web2-like experience to your users.
- [x] **Multi-device**: Most MPC-TSS Wallet providers only allow one device per wallet, because it's easier to build. With Meemaw, your users can add additional devices for practicality and redundancy.
- [x] **Web & iOS**: It's easy to integrate Meemaw in your Web & iOS apps. Use the appropriate SDK and you're good to go. Note that we are planning to add support for Android and cross-platform frameworks as well.
- [x] **Integrates with your Auth**: Meemaw is built so that you can easily integrate with your own Auth provider. We are also continuously adding one-click integrations for more and more third-party Auth providers.
- [x] **Battle-tested technologies**: Meemaw is NOT innovating on the actual technology (TSS and others). Instead, it aims at bringing together established and production-ready technologies to form a coherent experience while reducing risk. 
<!-- the next points are not as key and important, people will see it in docs or it's obvious -->
<!-- - [x] **Ethereum and more**: Meemaw is compatible with most blockchains (those based on ECDSA). On top of that, it provides helpers to send transactions and call smart contract on Ethereum and other EVM blockchains with one line of code. -->
<!--* **No vendor lock-in**: On top of being open-source and here to stay, Meemaw is built so that you can migrate at any time. From cloud or self-hosted to a competitor or to a different way of dealing with Web3 onboarding.-->
<!--* **Self-hosted**: Meemaw is built so that you can easily self-host it in just a few clicks. Depending on your objectives, you may want to install Meemaw with Docker or build it from source. Both options are available.-->


Exciting things we're looking forward to:

- [ ] **Android & Cross-platform frameworks**: If you're using Android or cross-platform frameworks, we plan to be able to cover your needs. Meemaw will be compatible with Web, iOS, Android, React Native, Flutter and Kotlin Multiplatform.
- [ ] **Biometrics & Passkeys**: Depending on platforms and applications, you will be able to encrypt and protect client shares with biometrics and/or passkeys. We want to reach state of the art security on all aspects.
- [ ] **Account abstraction**: People often oppose MPC wallets with account abstractions, but they are complementary! We will combine them so you can abstract gas payment from your users, innovate on UX and add one more layer of protection, for example.
<!-- - [ ] **Dual server mode**: You will be able to perform the TSS process between two servers, removing the need to store anything client-side. You will also be able to combine one server you self-host and one server we host on our cloud, maximising the benefits. -->

We're super excited ☀️

# Getting Started

If you want to start using Meemaw, you should check [the getting started section of our docs.](https://getmeemaw.com/docs/getting-started) You will find a complete example including deployment of the server and integration of the client SDK. It's really accessible for any dev, seriously, you should check it for yourself!

However, you deserve a sneak peak right here :

### Web
Here is a simple example using the Meemaw SDK in JS to create a wallet and sign a transaction. Web3.js is used to send the transaction.

```javascript
import meemaw
import web3

// Create or recover a wallet for this user
const meemaw = new Meemaw('meemaw-url');
const wallet = await meemaw.GetWallet(jwt); // will recover the wallet if exists for the user or create a new one

// Instantiate Web3
const web3 = new Web3(new Web3.providers.HttpProvider('rpc-url'));

// Craft the transaction (adapt to  your needs)   
var raw = {
    'to': '0x809ccc37d2dd55a8e8fa58fc51d101c6b22425a8',
    'value': 10000000000000, 
    'nonce': 1,
    'gasLimit': 21000,
    'gasPrice': 10,
};

// Sign the transaction
const signed = await wallet.SignEthTransaction(raw, chainId);

// Send the signed transaction
const txReceipt = await web3.eth.sendSignedTransaction(signed);
console.log("Look at my fabulous transaction:", txReceipt);

```

### iOS
Here is a simple example using the Meemaw SDK in Swift to create a wallet and sign a transaction. Web3.swift from the Argent team is used to send the transaction.

```swift
import meemaw
import web3

// Create or recover a wallet for this user
let meemaw = Meemaw(server: "meemaw-url")
let wallet = try await meemaw.GetWallet(auth: jwt) // will recover the wallet if exists for the user or create a new one

// Instantiate Web3
guard let clientUrl = URL(string: "rpc-url") else { return }
let client = EthereumHttpClient(url: clientUrl)

// Craft the transaction (adapt to  your needs) 
let transaction = EthereumTransaction(
    from: wallet.From(),
    to: "0x809ccc37d2dd55a8e8fa58fc51d101c6b22425a8",
    value: 10000000000000,
    data: Data(),
    nonce: 1,
    gasPrice: 10,
    gasLimit: 21000,
    chainId: 1
)

// Sign and send the transaction with wallet
let txHash = try await client.eth_sendRawTransaction(transaction, withAccount: wallet)
print("Look at my fabulous transaction: \(txHash)")
```

# Documentation

Want to know how it works in more details or want to start using Meemaw in your projects?

Check [our docs here.](https://getmeemaw.com/docs/intro) 🚀

# Contribute

If you care about Web3 and want to participate in its future, you should join us! Contributions are welcome and greatly appreciated.
If you want to help, we actually have a list of things you can start doing right now 😊
For ideas, contribution guidelines and more, see [our docs](https://getmeemaw.com/docs/contribute/).

# License

Most of Meemaw is under the ***AGPLv3*** license. The client SDKs are under the ***Apache 2.0*** license. You can find more info on [how we intend those licenses here](licenses.md).
