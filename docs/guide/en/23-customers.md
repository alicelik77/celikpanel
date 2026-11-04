---
id: customers
title: Customers and subscriptions
roles: [admin, reseller]
topics: [users]
keywords: [account, customer, reseller, invitation, suspend, block, transfer, delete, log in as]
---

**Users** holds the accounts you manage: resellers and customers for the administrator, customers for a reseller.

## Creating an account {#create}

Press **Add account**, fill in **Username**, **Email** and **Role**, and a **Password**, or tick **Send an invitation instead of setting a password** (valid 7 days). **Rights** starts at **Unlimited**; choose a plan or custom rights there (see [Plans and rights](guide:plans-rights#plans)). Press **Create**.

## Blocking or suspending {#suspend}

| Button | What stops |
|---|---|
| **Block sign-in** | Only signing in. Sites and mail keep running. |
| **Suspend services** | Sites show that they are suspended; mail is held back so senders retry; mailbox logins, databases, VPN, scheduled tasks and file access stop. |

Suspending asks for a reason the owner sees, and suspending a reseller suspends its customers. Nothing is deleted; **Resume services** restores exactly what ran. An end date, or 7 days over the disk limit, also suspends; the disk case ends by itself under the limit.

## Transferring a subscription {#transfer}

**Transfer a subscription**: choose the **New owner**, **Review the transfer**, then **Transfer**. Keep **Change all passwords and keys** on, so the previous owner keeps no access. The receiving side's rights and pool are checked first.

The administrator transfers between any accounts, a reseller between its own customers. A customer's **Ask for a transfer** arrives under **Transfer requests**.

## Deleting {#delete}

**Delete** works only when the account holds nothing live; otherwise the panel names what is left. The **Deletable** tab lists suspended subscriptions and what they still hold. Delete their domains and databases first.

:::warning
Deleting a domain cannot be undone. Take a backup first, or suspend instead.
:::

**Log in as** opens the panel as that customer, to look at a problem together. The audit log records it with both names.
