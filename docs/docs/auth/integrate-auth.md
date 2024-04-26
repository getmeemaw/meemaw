---
sidebar_position: 1
---

# Link your Auth system

Meemaw links to your own auth system. It allows you to use whichever auth provider your prefer, or to use your own custom one.

:::info
We purposefully use "auth" in a vague way, as it's both authentication and authorization. For the moment, we only rely on the external provider for authentication, upon which Meemaw grants authorization to perform MPC-TSS operations. If you have a specific use case requiring authorization from the external provider first, please reach out on Discord!
:::

When linking Meemaw to an auth system, you have two options:
* **One-click Auth integrations** : For some auth providers, Meemaw comes with all batteries included and just requires the API key to be good to go. 
  * Here is the current list of one-click integrations:
    * [Supabase](/docs/auth/supabase)
  * Coming soon:
    * SuperTokens
    * FusionAuth
    * Auth0
    * Firebase Auth
* **Custom integration** : For your custom auth system or for other auth providers, you just need to [provide a specific webhook](/docs/auth/custom) for Meemaw to use.

## Authentication token

Regardless of the system you use, Meemaw needs one piece of information to authenticate your users: a token provided by your auth system. This is provided through your client when you use `GetWallet(token)`. Here it is in our [example](/docs/getting-started):

```javascript title="client/src/app/tx.jsx"
...

const {
    data: { session },
} = await supabase.auth.getSession()
const { access_token } = session || {}

...

const wallet = await meemaw.GetWallet(access_token);
```