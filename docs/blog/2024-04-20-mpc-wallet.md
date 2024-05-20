---
slug: mpc-wallet
title: MPC Wallets for Dummies
authors: [marceau]
tags: []
---

<p align="center">
  <img src="/img/steve3_small.webp" alt="Description of the image" width="300" />
</p>


Imagine a world where Steve Jobs and a Swiss Banker collaborate to create a crypto wallet. The result? We bet it would be MPC wallets: a more secure and ultimately user-friendly solution.

To understand the technology, let's compare it to a real-life situation. Imagine you're a millionaire with stacks of $100 bills to your name. How would you secure this wealth?

<!-- truncate -->

## From Mattress to Arnold Schwarzekeeper

The simplest way, albeit not the most practical, would be to stash all your money under your mattress. This is similar to conventional crypto wallets where you hold the private keys that unlock your funds. You are the only one in control, and the only one responsible for your funds security. When you use apps like Metamask, this is essentially what you are doing.

However, keeping your wealth under a mattress is risky and you're afraid of people breaking into your house. So, you decide to entrust your money to your neighbor, Arnold Schwarzekeeper. He's strong and knows how to fight cyborgs, plus he's also storing your other neighbors' cash. This is comforting. However, you also remember Conor McGator, Arnold's pal who lived just around the corner. He was in the same boat until he lost all his neighbors' cash in a blink.

<p align="center">
  <img src="/img/arnold_conor_small.webp" alt="Description of the image" width="500" />
</p>

:::warning
This is the equivalent of exchanges like Coinbase or conventional wallet-as-a-service platforms like Magic. They hold your private keys so that you don't have to worry about them. Obviously, that means you need to trust them to keep them secure, and not to block your funds! We all remember the times when Binance lost $586 million or when FTX blocked all customers from taking money out...
:::

## Splitting Bills with Grandma

But what if you're not comfortable with either option? An alternative would be splitting your $100 bills in half with your Grandma. You keep all the recto faces at your place, and you ask your Grandma to store all the verso faces at her place. Let's just imagine we live in a world where that's possible 😇

Now, if you want to buy, say a brand new Apple Vision Pro, you just need to meet with Grandma at the store. You both bring a stack of half-bills, re-combining them on the spot. If someone robs your house, all they'll find are useless half-bills. Your Grandma can't spend your money without your half, and there's no way for anyone to steal all of your funds just by recovering the small stack of re-combined bills.

The nice thing about splitting these magic bills is that you can actually split them in more than two parts, and then decide how many parts are required to spend the money.

<p align="center">
  <img src="/img/ar_small.webp" alt="Description of the image" width="250" />
</p>

:::tip
This setup is similar to how MPC wallets work: instead of having private keys in one place, you have multiple keys stored in different places and they combine their signatures to send a transaction. The magic of MPC wallets is that you can decide how many signatures are required from the total pool of keys. At no point in time are those keys united in one place, only signatures are. Take your smartphone and your laptop, for example. By storing keys on both these devices, you'd need both of them to sign a transaction before sending funds.
:::

## Helping thy neighbor

Your stash of cash is now safe 😎 As you realise the power of splitting bills, you decide to go one step further and to help your neighbors with their cash. You split their bills in three parts : you keep one, they keep one, and they give one to their Grandma. For this setup, you agree that only two out of the three parts are needed to make a purchase. 

When a neighbor wants to buy something, you provide them with split-bills to recombine with their own. If you're unavailable, they can turn to their Grandma for some split-bills. The beauty of this system lies in its security: you cannot spend your neighbors' funds, and even if your house is robbed, the split-bills are useless to the thief.

<p align="center">
  <img src="/img/neighbor_small.webp" alt="Description of the image" width="300" />
</p>

:::tip
This is how an MPC Wallet-as-a-Service platform like [Meemaw](/) works. You as a developer can deploy an MPC wallet for each of your user. The server stores one key, the user stores another, and even more keys can be stored on other devices. In order to spend funds, multiple keys are used to sign a transaction, and the wallet private keys never appear in any single place. Most importantly, there is no way for the server alone to spend funds. In many ways, this is more secure than having a platform store the wallet private keys directly.

Meemaw is an [open-source](https://github.com/getmeemaw/meemaw) MPC Wallet-as-a-Service. You can self-host it and not trust anyone else, or you can let us host it for you, knowing that you can verify the source-code at any point to understand how it works, or that you can switch back to self-hosting for any reason.
:::

## Did you say user-friendly?

Conventional wallets require users to remember passwords and mnemonics to secure private keys. It is quite the opposite of a great user experience. MPC wallets remove the need for complexity in the user experience. Also, wallet-as-a-service solutions like Meemaw make it super simple for developers to deploy MPC wallets for their users. Developers are free to build an amazing experience for their users, abstracting away the complexity of securing their funds.

## MPC, TSS... Dafuq?

Ok, stories are nice, but there are two terms we need to clarify: 
- **MPC (Multi-Party Computation):** This is the process where several parties, who may not trust each other, work together to perform cryptographic operations. The result is then combined to create a final output.
- **TSS (Threshold Signature Scheme):** This is a sub-field of MPC. It generates distributed keys without ever producing centralised private keys (a process known as DKG) and then merges the signatures (SIGN) to send funds. You can set up a "x of n" requirement, where you choose the values of x and n. In the example above, 2 out of 3 signatures are required.

## Conclusion

This blog post was written with the objective of simplifying the concept of MPC wallets. It does not go into great technical details, but you can easily find those online. It also simplifies a few things to make the comparison somewhat understandable.

If you only remember three things today, it should be that MPC wallet-as-a-service solutions like Meemaw are :
- **more secure** (non-custodial and trustless): private keys never appear anywhere, the server cannot spend funds by itself
- **more robust**: users can recover funds without the intervention of the server if they need to, avoiding funds being blocked
- **more user-friendly**: no need for crazy passwords or seed phrases, developers are free to build amazing experiences

Have a great day 😊