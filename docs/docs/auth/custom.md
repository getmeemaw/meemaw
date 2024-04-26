---
sidebar_position: 2
---

# Custom auth

Meemaw integrates with your custom auth system. You need to do two things: 
1. provide an endpoint available to Meemaw as webhook
2. configure Meemaw to use that webhook for auth

Everytime your users need to perform a wallet operation, Meemaw will make sure they're properly authenticated through your custom system for added security.

## Webhook

You need to provide an endpoint with the following characteristics :
- **Input:** POST request with a JSON similar to this
    ```json
    {
        "token": "user-identifying-token"
    }
    ```
- **Output:** The UserID or other identifier of the user that can be used as a distant foreign key inside Meemaw. It should be immutable for a given user. Note that this should not be formatted as a JSON.
- **Errors:** In case of errors the endpoint should return at least the following http error codes: 400 (badly formatted payload, for example), 401 (token is outdated, for example), 404 (token does not exist).

We wrote a [small Golang example](https://github.com/getmeemaw/example-auth-custom/blob/main/main.go) to illustrate how the webhook should behave. Of course, this is utterly simplistic and should never be used in production.

## Configure Meemaw

Modify `config.toml` to use your custom auth system and provide the URL of the webhook we created above:

```toml title="config.toml"
...
authType='custom'
authServerUrl='your-webhook-url'
```

Now, everytime your users need to perform a wallet operation, Meemaw will make sure they're properly authenticated by calling your custom webhook, providing it with an identifying token and using the returned UserID.