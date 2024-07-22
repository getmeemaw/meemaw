---
sidebar_position: 3
---

# Web

Before moving any further on the Javascript SDK, one note of caution:

:::info
For now, **the Javascript SDK stores the client share of the wallet in LocalStorage**. Even though an attacker cannot do anything with the client share only (it requires the server share as well), LocalStorage does present security limitations. It is, for example, quite exposed to XSS attacks. There is also a risk for users to clear their cache and data, deleting the share and removing any access to the wallet.

In the future, we will be using client-side encryption and/or authentication. We are considering two options:
- Using client-side encryption : the TSS share is encrypted client side and stored on the server. This would solve both problems listed above. The problem is that the technology is not vastly supported yet. We're looking at WebAuthn with the PRF extension quite attentively.
- Using client-side authentication : the TSS share is encrypted client side using a unique encryption key stored on the server. The encryption key is only provided to the client when properly authenticated (passkeys). This would reduce the risk of XSS attacks but it would not reduce the risk of users clearing their data.
:::

## Install SDK

If you're using Node:

```
npm install @getmeemaw/meemaw-js
```

If you want to import it from CDN, add this to your `head` section:

```html
<script src="https://cdn.jsdelivr.net/npm/package@version/file"></script>
```

## Use SDK in Javascript

### Init Meemaw and get wallet

Your first step is to initialise Meemaw and to "get" a wallet. If no wallet currently exists for a given user, the SDK will generate a new one in concert with the server (TSS process). If one exists, the SDK will use it.

```javascript
import Meemaw from 'meemaw-js'

// Create or recover a wallet for this user
const meemaw = await Meemaw.init('MEEMAW_SERVER');
const wallet = await meemaw.GetWallet(TOKEN); // will recover the wallet if exists for the user or create a new one
```

The first line imports Meemaw.

The second line initialises the library with the server address, and the third line "gets" the wallet. One interesting bit is the `TOKEN`. It represents the user connexion and depends on your Auth mechanism. For example, if you're using Supabase, the token is the Supabase's `access_token`. Behind the scenes, Meemaw will authenticate the user using that token and only procede if the user exists AND is logged in.

Once you create a wallet for a user, potentially at sign up, you will most probably want to display or store the public key for that wallet. You can easily do that by calling :

```javascript
const public_key = wallet.From()
```

### Sign normal transaction (Ethereum)

In order to send funds, the first step is to craft the raw transaction, then to sign it using the Meemaw wallet.

```javascript
// Craft the transaction (adapt to  your needs)   
const raw = {
    'to': 'RECIPIENT_ADDRESS',
    'value': 10000000000000, 
    'nonce': Number(nonce),
    'gasLimit': 21000,
    'gasPrice': Number(gasPrice),
};

// Sign the transaction
const signedTransaction = await wallet.SignTransaction(raw, chainId);
```

The SDK will automatically sign the transaction in concert with the server (TSS process) and return the signed transaction, ready to be published to an Ethereum-compatible blockchain. Note that we need to provide the `chainId` to sign the transaction in order to avoid replay attacks on other chains. 

Also, you should get the `nonce` and `gasPrice` using your favorite web3 library.

### Sign smart contract call (Ethereum)

You can sign smart contract calls using the same procedure. It would look something like this using web3.js, but you obviously need to adapt:

```javascript
// Craft the transaction (adapt to  your needs)   
const contract_address = 'SMART_CONTRACT_ADDRESS';
const contract_abi = JSON.parse(`SMART_CONTRACT_ABI`);

var MyContract = new web3.eth.Contract(contract_abi, contract_address);
const data = MyContract.methods.mysupermethod(5).encodeABI();

const raw = {
    'to': contract_address,
    'value': 0, 
    'data': data,
    'nonce': Number(nonce),
    'gasLimit': Number(gas),
    'gasPrice': Math.round(Number(gasPrice)*1.2), // https://github.com/web3/web3.js/issues/6276
};

// Sign the transaction
const signedTransaction = await wallet.SignTransaction(raw, chainId);
```

### Send transaction (Ethereum)

To send a transaction, there is nothing particular to Meemaw. You can just use the web3 library you like, providing it with the signed transaction. Here is an example using web3.js :

```javascript
const txReceipt = await web3.eth.sendSignedTransaction(signedTransaction);
```

Note that you most probably need to import and initialise your web3 library beforehand! Check our [example](/docs/getting-started) to see a full working code.

### Sign message (all ECDSA blockchains)

It is also possible to sign an hex encoded message:

```javascript
const signature = await wallet.SignBytes(message);
```

Note that this just signs arbitrary bytes, it does not comply with Ethereum specifics standards. You probably want to check [eip-191](https://eips.ethereum.org/EIPS/eip-191) and [eip-712](https://eips.ethereum.org/EIPS/eip-712). We will probably add some helpers in the future, similarly to our iOS SDK.

### Multi-device

Before you start using it, it's probably important you learn [how multi-device works](/docs/multi-device).

Multi-device works in two steps: 
1. starting registration on a new device
2. accepting on an existing device.

#### Register device

You actually don't have to do anything. The function we used above (`meemaw.GetWallet(...)`) automatically recognizes that it's a new device when a wallet already exists.

However, we recommend that you provide two callback functions: one called when the function starts the registration process, and one called when the process is finished. This allows you to display a call to action in the UI, prompting the user to use his other device to confirm.

Here is how you would adapt the code above:

```javascript
import Meemaw from 'meemaw-js'

const meemaw = await Meemaw.init('MEEMAW_SERVER');
const wallet = await meemaw.GetWallet(TOKEN, function() {
            // RegisterDevice started => prompt user for confirmation on existing device
        }, function() {
            // RegisterDevice done => hide prompt
        });
```

This will recover the wallet if it already exists on the device, create a new one if the user doesn't have one at all, or start the multi-device process if the user already has a wallet created on another device.

#### Accept device

On the existing device, it is now time to confirm the new device with a simple call:

```javascript
await wallet.AcceptDevice()
```

That's it, now the multi-device process will happen with both devices and the server communicating with each other. At the end of the process, the new device will have it's own part of the overall MPC wallet, ready for operations.

### Export private key

You can offer your users to export their private key:

```javascript
const privateKey = await wallet.Export();
```

This is useful if your users want to manage their wallet outside of your platform, for example.

:::warning
Meemaw wallets are MPC wallets. Until you decide otherwise, **no private key exists**, transactions are signed through a collaboration process between the server and the client.

However, **it is possible to generate the private key corresponding to the wallet**. The way it works is the following: the client sends its TSS share to the server, which then combines the client share with its own share to form a private key.

It is important to understand that by doing so, **you completely loose the security and decentralization benefits of MPC wallets**: anyone with access to this private key can spend the funds contained in the wallet.

It is also important to note that **the MPC wallet still exist**, it's just that the private key now exists as well and can bypass the whole TSS process.
:::