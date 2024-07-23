---
sidebar_position: 4
"sidebar_label": "Multi-device"
---

# Multi-device

With Meemaw, your users can easily add multiple devices. All of these devices have access to the MPC wallet. They also act as backups, improving redundancy. You end up with a super secure and resilient system, maintaining web2 levels of experience.

## How does it work?

In order to understand how multi-device works, you should first understand [how TSS works](/docs/how-does-it-work). If you remember, after the DKG process, we end up with 2 shares: one on the server, one on the device. Those 2 shares are required to sign a transaction. With Multi-device, we're basically adding one share on a new device. We now have 3 shares in total, but we only need 2 to sign a transaction. That means that any of the devices can be used to use the wallet.

In order to add a new share, we need 2 existing shares. In our case, one existing device will work in collaboration with the server and the new device. This TSS process will generate a new share for the new device. It's important to understand that no full private key ever gets created, and the new share is only known by the new device. We're maintaining the security offered by MPC wallets.

## How to use?

Even though the process in itself is complex, Meemaw abstracts everything away. For you as a developer, it's extremely easy:

1. ***meemaw.GetWallet()***: remember how you called *meemaw.GetWallet()* [during the getting-started](/docs/getting-started)? Well, this function also initiates the multi-device process if Meemaw recognizes this as a new device when a wallet already exists.
2. ***wallet.AcceptDevice()***: once the multi-device process is initiated on the new device, it's just a matter of using *wallet.AcceptDevice()* on a device that is already registered.

That's it! If you want to start using it (and learn more about callback functions, etc), head to [our SDK section](/docs/client/).

## Backup file

You can also generate a backup file for your users. Behind the scenes, it uses multi-device to create a new fully-functional share. This means that it can be used if the user looses his devices, or if the server looses his shares.

You just need to call *wallet.Backup()* to generate a long string, i.e. the backup. You're free to package it as a file or print it on the page, depending on the experience you want to provide.

You will find more info in [our SDK section](/docs/client/).