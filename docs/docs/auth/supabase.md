---
sidebar_position: 3
---

# Supabase

Meemaw integrates with Supabase extremely easily. You just need to configure Meemaw properly and provide your Supabase URL and API key, and your users will benefit from MPC-TSS wallets without realising it. Nothing else to do.

## Configure Meemaw

Modify `config.toml` to use Supabase and provide the URL and API key:

```toml title="config.toml"
...
authType='supabase'
supabaseUrl='https://your-url.supabase.co'
supabaseApiKey='your-api-key'
```
That's it ! 🥳

Now, everytime your users need to perform a wallet operation, Meemaw will make sure they're properly authenticated through Supabase for added security.