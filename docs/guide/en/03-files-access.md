---
id: files-access
title: Files: file manager, FTP, SFTP and SSH
roles: [admin, reseller, customer]
topics: [domain.files#file-manager, domain.access#ftp-sftp]
keywords: [ftp, sftp, ssh, filezilla, upload, download, public_html, key, command line, terminal]
---

There are three ways to reach your site's files: the file manager in the panel, a program such as FileZilla over FTP or SFTP, and a command line over SSH. Each one sees only your own subscription's folder; the rest of the server stays closed.

## Where to find them {#where}

Open **Domains** and choose the domain. Two of its tabs are about files:

- **Files** is the file manager.
- **Access** turns on FTP, SFTP and SSH, and shows how to connect.

Your website's files are in the `public_html` folder. A team member (user) sees **Files** if the customer opened it for them, but never **Access**.

## The file manager {#file-manager}

The **Files** tab lists the site's folders and files. With it you can:

- open a folder, and go back with **Go up**;
- **Upload** files from your computer;
- create a **New folder** or a **New file**;
- **Edit** a text file up to 1 MB, such as a configuration file, and save it;
- **Download** or **Delete** a file.

Pictures, archives and other binary files can be uploaded and downloaded, but not edited in the panel. For many files at once, FTP or SFTP is quicker.

:::warning
A deleted file is gone. Before you change or delete something important, download a copy, or take a backup under **Advanced** → **Backups** on the domain's page.
:::

## FTP and SFTP {#ftp-sftp}

On the **Access** tab:

1. Turn on **File access (FTP and SFTP)**.
2. Under **Set a password**, choose a password of 8 to 128 characters, or add an SSH key (see [SSH keys](#keys)).
3. Press **Review the change**. The panel shows what will change on the server.
4. Press **Apply the change**.

The box **Connect with FileZilla** then shows your server name, user name and folder, step by step. FTP is always encrypted (FTP over TLS); plain FTP is not offered. SFTP uses the same user name and password.

CelikPanel does not store your password and cannot show it again. If you forget it, set a new one in the same place.

:::note
The first time you connect over FTP, FileZilla may ask whether to trust the server's certificate. Trust it only if the SHA-256 fingerprint is the one shown on the **Access** tab.
:::

## SSH keys {#keys}

A key is safer than a password. Under **SSH keys**, paste one public key, the contents of your `.pub` file, for example:

```
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA… you@laptop
```

Never paste the private key, the file without `.pub`. Some servers accept only keys for SFTP and SSH; the **Access** tab tells you when that is the case, and FTP then still uses the password.

## The command line (SSH) {#ssh}

**Command line (SSH)** opens a terminal closed into your subscription's folder, with basic file tools only. Turn it on only if you need it, in the same way as file access: tick it, then **Review the change** and **Apply the change**. Connect with the same user name and your password or key:

```
ssh -p 22 user@server.example.com
```

Use the host, port and user name the **Access** tab shows; they may differ from this example. If **Command line (SSH)** is not part of your subscription's plan, the tab says so. File access is not affected, and your provider can add it.

## When SSH cannot be turned on yet {#ssh-tools}

The command line takes its tools from the server. If the server lacks some of them, the **Access** tab says which ones, and the SSH option stays hidden until they are there. FTP and SFTP keep working meanwhile.

- **Customers:** the server administrator has to install the tools. Ask your provider; the option appears on the tab once they are installed.
- **The administrator** sees the missing tools with their official package names and an **Install the tools** button. The option appears as soon as they are installed.

An account that already had the command line keeps it meanwhile, without the missing tools.

## Common messages {#messages}

| The tab says | What to do |
|---|---|
| File access is not part of this subscription's plan | Use the file manager, or ask your provider to add file access. |
| This subscription is not active | Access can only be closed now. Your provider renews the subscription. |
| This server accepts only SSH keys | Add a key under **SSH keys**. FTP still uses your password. |
| FTP is not installed on this server | Use SFTP; it works with the same user name and password. |
| This domain has no website files | Give the domain a website on the **Hosting** tab first. |

For any other message, read it as [Reading a state and getting help](guide:reading-states#problem) explains: it says who has to act and what to do next.
