---
sidebar_position: 2
---

# Getting Started

So you want to start using Meemaw?! Excellent! 🤘

:::info
**Meemaw is still under heavy development, we would not advise you to use it in production just yet.** That being said, you should totally experiment with it and let us know how it goes!
:::

Also, this is just scratching the surface, you can find a whole lot more in the rest of the docs.

## Let's get started

Ok let's start with a simple yet complete example. We will deploy the server with Docker, link it with a Supabase instance for authentication, and use our Web SDK to deploy a trustless wallet for each of your users.

## Requirements

You need:

* a machine (VPS, dedicated server, your computer, etc) with [NodeJS](https://nodejs.org/), [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/) installed
* access to a JSON-RPC API to ensure your application can access the Sepolia blockchain ([Alchemy](https://www.alchemy.com/), [Infura](https://www.infura.io/), etc)
* a [Supabase](https://supabase.com/) project and your `public API key` and `URL` handy (the free tier is fine)
* familiarity with basic Web3 concepts (i.e. you understand what is required to send a transaction)

Also, let's download a working example so that we can get our feet wet super easily:

```
git clone https://github.com/getmeemaw/example-js.git
```

We will explain the Meemaw specific parts of this example in this page. The rest is typical React and some [Supabase specific stuff](https://supabase.com/docs/guides/auth/auth-helpers/auth-ui).

## Deploy server

You will learn a bit later how to [configure Meemaw](/docs/server#config) and modify the [Docker compose configuration](/docs/server#docker-compose) for your specific needs, but if you downloaded the example above, **there is just one thing you need to do**: in `server/config.toml`, update `supabaseUrl` and `supabaseApiKey` with yours.

Then you can actually deploy the server. Make sure you're in the server directory and that Docker is running, then just start the server with:

```
docker compose up
```

You should be greeted with something like this:
```
meemaw_app  | 2042/05/04 11:59:59 Logging enabled
meemaw_app  | 2042/05/04 11:59:59 Connected to DB
meemaw_app  | 2042/05/04 11:59:59 Schema does not exist, creating...
meemaw_app  | 2042/05/04 11:59:59 Schema loaded
meemaw_app  | 2042/05/04 11:59:59 Starting server on port 8421
```

Congrats, Meemaw's server is running 🎉

If it does not work as expected, join us on Discord! We will happily help get you started.

## Run the example

Before we actually run the example, I suggest we go through the code together. In particular, we'll look at what is specific to Meemaw.

### Configure your App
Before we do anything, there are just two things you need to configure on the client side: 
- the URL of your JSON-RPC API. Update the following line:
    ```javascript title="client/src/app/tx.jsx"
    const web3 = new Web3(new Web3.providers.HttpProvider("http://localhost:8421/rpc"));
    ```
- your Supabase URL and API Key. Create a `.env.local` file in the `client/` folder with the following lines:
    ```toml title=".env.local"
    SUPABASE_URL=YOUR_SUPABASE_URL
    SUPABASE_ANON_KEY=YOUR_SUPABASE_API_KEY
    ```

### Install dependencies
In order to be able to run our example, we need to install all dependencies:

```
npm install
```

### Meemaw specific code
We will check what happens when you click on the button to send an ETH transaction.

The first step is to get a wallet for the given user. If it's the first time the user tries to do anything that requires a wallet, the Meemaw SDK will automatically [create a new wallet](/docs/how-does-it-work) for that user. Otherwise, it will just [recover the wallet](/docs/how-does-it-work) from storage. It's as easy as instantiating Meemaw with the server URL then calling `GetWallet` with the Supabase JWT :

```javascript title="client/src/app/tx.jsx"
// Create or recover a wallet for this user
const meemaw = new Meemaw('meemaw-url');
const wallet = await meemaw.GetWallet(jwt); // will recover the wallet if exists for the user or create a new one
```

Once we've got a wallet, we can create an ETH transaction. We do that with the web3.js library. Note that we also use `wallet.From()` to easily get the user's wallet address.

```javascript title="client/src/app/tx.jsx"
// Instantiate Web3
const web3 = new Web3(new Web3.providers.HttpProvider('rpc-url'));

// Get important information for the transaction
const nonce = await web3.eth.getTransactionCount(wallet.From())
const gasPrice = await web3.eth.getGasPrice();
const chainId = 11155111; // We use the Sepolia test net to easily test the whole process
// const chainId = await web3.eth.getChainId();

// Craft the transaction (adapt to  your needs)   
const raw = {
    'to': '0x809ccc37d2dd55a8e8fa58fc51d101c6b22425a8',
    'value': 10000000000000, 
    'nonce': Number(nonce),
    'gasLimit': 21000,
    'gasPrice': Number(gasPrice),
};
```

Last thing is to sign and send the transaction. It's as easy as using `wallet.SignTransaction()` then sending the transaction with web3.js. Note that we need to provide the `chainId` to sign the transaction in order to avoid replay attacks on other chains :

```javascript title="client/src/app/tx.jsx"
// Sign the transaction
const signedTransaction = await wallet.SignTransaction(raw, chainId);

// Send the signed transaction
const txReceipt = await web3.eth.sendSignedTransaction(signedTransaction);
```

That's it! 

### Start the app

Cool so you've got the source code downloaded on your machine and you understand it. Now is finally time to run your app! Open a new terminal and make sure you're in the client directory then run this command:

```
npm run dev
```

Boom ! 🎉

You should now see something like this in your terminal, amongst other things :

```
Local:        http://localhost:3000
```

Just visit [http://localhost:3000](http://localhost:3000) and you should see our app 🥳

### Send your first transaction
When you open the example for the first time as a "fresh" user, **you will have to sign up on Supabase**. Simply click on "I don't have an account yet", and sign up using your email and password. Note that an email will be sent to confirm your registration. Once you've confirmed the registration, you should be logged in our example app. If this is not the case, simply log in.

The next step is to **create a wallet** for this user. Just click on "Generate Wallet", and everything will happen magically behind the scenes. A wallet will be generated in concert between the client and the server through the MPC-TSS process. Note that there is an obvious potential improvement here: you could automatically create a wallet at sign up!

Once the wallet is created, we have one last step we need to take before being able to send a transaction: **add some funds to our wallet**. Otherwise, obviously, there will be nothing to send... One way to get some funds on the Sepolia network is to use a faucet like this one: https://sepolia-faucet.pk910.de/ Just enter the ETH address of your new wallet and "start mining".

Excellent, now is finally time to **send our first transaction!** Wait the few seconds necessary for funds to arrive in your new wallet, then send the transaction to whatever wallet you wish to make slightly richer 😁 Again, everything will happen magically behind the scenes in concert between the client and the server. The transaction will be signed through the MPC-TSS process then broadcast on the blockchain.


## Next steps

Okay, we've got a nice example running. However, this is clearly not a production setup. Here is a non-exhaustive list of a few elements that you would need to change to get your App ready for production:

* **Fix Meemaw's version:** in the Docker Compose file, replace the "latest" tag with the actual version you want to use. This avoids braking changes affecting your setup.
* **Separate Meemaw's DB from the server:** we cheated a bit here by magically deploying a Postgresql DB with Docker Compose. You probably want to have a proper managed DB with adequate accesses.
* **Separate Meemaw from the App server:** your App is served from the same machine as Meemaw. You should separate them and greatly reduce access to the Meemaw machine. Keep it secure.
* **Buid the App and serve static files:** you should not run `npm run dev` in production. Instead, you should build the App and serve the files through a proper web server.
* **Follow security guidelines:** your Meemaw installation is only as secure as the security measures you follow. Please make sure you follow all [our security guidelines](/docs/security)

We will soon offer a Cloud service to make it super easy for you to have a production-ready setup in a few minutes.