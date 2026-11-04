---
id: vpn
title: VPN: secure access to the server
roles: [admin, reseller, customer]
topics: [vpn]
keywords: [vpn, wireguard, device, laptop, phone, tunnel, qr]
---

**VPN** gives a laptop or phone a secure connection through the server's built-in WireGuard VPN.

## Turning it on (administrator) {#server}

1. Install WireGuard with **Install WireGuard**, or from **Components**.
2. Press **Start VPN server**.
3. The server must accept UDP port 51820. If your hosting provider has its own firewall, open that port there too.

The **Server policy** tab shows the fixed settings; they cannot be edited.

## Adding a device {#devices}

Each device belongs to a subscription whose plan includes VPN.

1. On **Devices**, press **Add a device**, choose the subscription and type a name, such as `work-laptop`.
2. Press **Create config**.
3. Scan the QR code in the WireGuard app, or press **Download .conf** and import the file.
4. Press **I saved it — close**.

:::warning
The configuration is shown only once and its private key is not stored. If it is lost, remove the device and add it again.
:::

Removing a device cuts its access at once.

## If something does not match {#sync}

The device list shows the wanted state and the applied state separately, so a failed change stays visible. The administrator's **Re-sync safely** applies the wanted state to WireGuard again. If VPN is not part of your plan, the page says so; your provider can add it.
