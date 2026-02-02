# Discord Bot Setup Guide

This guide walks through creating a Discord bot for llm-bridge and configuring it to relay messages between Discord channels and Claude CLI sessions. By the end, you will have a working bot that reads messages from a Discord channel and forwards them to Claude, then sends Claude's responses back to the channel.

## Prerequisites

- A Discord account with permission to add bots to your target server
- llm-bridge built and ready to run (see the main [README](../README.md) or [CLAUDE.md](../CLAUDE.md) for build instructions)

---

## 1. Create a Discord Application

1. Go to the [Discord Developer Portal](https://discord.com/developers/applications).
2. Click **New Application** in the top-right corner.
3. Name it something recognizable (e.g., `llm-bridge`), accept the terms, and click **Create**.
4. On the **General Information** page, note the **Application ID** (also called Client ID). You will need this later to generate the bot invite URL.

---

## 2. Create a Bot User

1. In the left sidebar, click the **Bot** tab.
2. Click **Reset Token** to generate a new bot token.
3. Copy the token immediately and store it somewhere secure. **This token is shown only once.** If you lose it, you must reset it to get a new one.
4. This token becomes your `DISCORD_BOT_TOKEN` environment variable.

**Security notes:**

- Treat the bot token like a password. Anyone with this token can control your bot.
- If the bot is for private or personal use, disable **Public Bot** on this page. This prevents others from adding your bot to their servers.

---

## 3. Enable Privileged Gateway Intents

On the **Bot** tab, scroll down to the **Privileged Gateway Intents** section. You need to enable one privileged intent:

- **Message Content Intent** -- toggle this **ON**.

**Why this is required:** llm-bridge reads the text content of messages sent in Discord channels and forwards that text to Claude. Without the Message Content Intent enabled, the bot receives message events but the `content` field is empty, which means Claude receives nothing.

**Notes on other intents:**

- **Guild Messages** -- llm-bridge uses the `GuildMessages` gateway intent to receive messages in server channels. This is a non-privileged intent and is requested automatically in code; no manual toggle is needed.
- **Direct Messages** -- llm-bridge also listens for direct messages. This is also non-privileged and requires no manual toggle.
- If your bot will be in **75 or more servers**, Discord requires you to go through a verification process before the Message Content Intent can be used. For most self-hosted llm-bridge deployments (typically 1-2 servers), this does not apply.

---

## 4. Generate the Bot Invite URL

1. In the left sidebar, go to **OAuth2** (the URL Generator may appear as a sub-section or directly on the page — Discord occasionally updates the portal layout).
2. Under **Scopes**, select **bot**.
3. Under **Bot Permissions**, select the following:
   - **View Channels** -- allows the bot to see the channels it is added to
   - **Send Messages** -- allows the bot to send Claude's responses back to the channel
   - **Attach Files** -- allows the bot to send long outputs as file attachments (llm-bridge automatically sends responses exceeding the output threshold as attached text files instead of inline messages)
   - **Read Message History** -- allows the bot to read messages in channels

4. Copy the generated URL at the bottom of the page and open it in your browser to invite the bot to your server.

Alternatively, you can construct the invite URL manually using your Application ID:

```
https://discord.com/oauth2/authorize?client_id=YOUR_CLIENT_ID&scope=bot&permissions=101376
```

Replace `YOUR_CLIENT_ID` with the Application ID from step 1. The permissions integer `101376` encodes exactly the four permissions listed above:

| Permission           | Bit Value  | Hex      |
| -------------------- | ---------- | -------- |
| View Channels        | 1024       | `0x400`  |
| Send Messages        | 2048       | `0x800`  |
| Attach Files         | 32768      | `0x8000` |
| Read Message History | 65536      | `0x10000`|
| **Total**            | **101376** |          |

---

## 5. Get Channel IDs

To configure llm-bridge, you need the numeric ID of the Discord text channel where the bot should listen and respond.

1. Open Discord (desktop or web app).
2. Go to **User Settings > App Settings > Advanced**.
3. Enable **Developer Mode**.
4. Close settings, then right-click the text channel you want to use.
5. Click **Copy Channel ID**.

This channel ID goes into the `repos.<name>.channel_id` field in your configuration file.

---

## 6. Configure llm-bridge

### Configuration file

Create or edit your `llm-bridge.yaml` with the Discord provider and at least one repo:

```yaml
providers:
  discord:
    bot_token: "${DISCORD_BOT_TOKEN}"

repos:
  my-repo:
    provider: discord
    channel_id: "123456789012345678"
    llm: claude
    working_dir: /path/to/your/repo
```

For a complete list of configuration options, see `llm-bridge.yaml.example` in the repository root.

The `bot_token` field supports environment variable expansion. At load time, llm-bridge replaces `${DISCORD_BOT_TOKEN}` with the value of the corresponding environment variable. You can also hardcode the token directly in the YAML, but using an environment variable is recommended to avoid committing secrets.

### Environment variable

Set the bot token as an environment variable before starting llm-bridge:

```bash
export DISCORD_BOT_TOKEN=your_bot_token_here
```

### Adding a repo via CLI

You can also add repos using the `add-repo` command:

```bash
./llm-bridge add-repo my-repo \
  --provider discord \
  --channel 123456789012345678 \
  --llm claude \
  --dir /path/to/your/repo
```

### Start the bridge

```bash
export DISCORD_BOT_TOKEN=your_bot_token_here
./llm-bridge serve --config llm-bridge.yaml
```

You should see log output indicating the Discord provider started:

```
level=INFO msg="discord provider started" channels=1
level=INFO msg="terminal provider started"
```

---

## 7. Permissions Reference

| Requirement      | Discord Setting      | Type           | Why                                              |
| ---------------- | -------------------- | -------------- | ------------------------------------------------ |
| Guild Messages   | Gateway Intent       | Non-privileged | Receive messages posted in server text channels   |
| Direct Messages  | Gateway Intent       | Non-privileged | Receive direct messages sent to the bot           |
| Message Content  | Gateway Intent       | Privileged     | Read the actual text content of messages          |
| View Channels    | Bot Permission       | Standard       | See channels the bot has been granted access to   |
| Send Messages    | Bot Permission       | Standard       | Send Claude's responses to the channel            |
| Attach Files     | Bot Permission       | Standard       | Send long outputs as file attachments             |
| Read Message History | Bot Permission   | Standard       | Access message history in channels                |

**How these map to the code:**

- `discordgo.IntentsGuildMessages` and `discordgo.IntentsDirectMessages` are set in `internal/provider/discord.go` as non-privileged gateway intents.
- `discordgo.IntentMessageContent` is set alongside them as a privileged gateway intent that must be manually enabled in the Developer Portal.
- `ChannelMessageSend` requires the Send Messages permission.
- `ChannelFileSend` requires the Attach Files permission.
- Receiving messages in real time via the gateway requires View Channels. Read Message History is included so the bot can view prior channel context if needed.

---

## 8. Troubleshooting

### Bot receives messages but they are empty

The **Message Content Intent** is not enabled in the Discord Developer Portal. Go to the Bot tab, scroll to Privileged Gateway Intents, and toggle Message Content Intent on. Restart llm-bridge after making the change.

### "Disallowed Intents" error at startup

The bot is requesting an intent that is not enabled in the Developer Portal. This typically means Message Content Intent is not toggled on. Enable it in the Bot tab under Privileged Gateway Intents and restart.

### Bot is not responding to messages

Check the following in order:

1. **Channel ID mismatch** -- Verify the `channel_id` in your config matches the actual channel where you are sending messages. Use Developer Mode to copy the channel ID again and compare.
2. **Bot not in server** -- Make sure you completed the invite step and the bot appears in the server's member list.
3. **Bot not started** -- Confirm llm-bridge is running and the log shows `discord provider started`.
4. **Wrong channel** -- The bot only listens on channels explicitly listed in the config. Messages in other channels are ignored.

### Permission denied when sending messages

The bot is missing the **Send Messages** or **Attach Files** permission in the channel. Either re-invite the bot with the correct permissions (use the URL from step 4) or manually adjust channel permissions in Discord's server settings to grant the bot these permissions.

### Bot appears offline in Discord

- The bot token may be invalid or expired. Reset the token in the Developer Portal and update your `DISCORD_BOT_TOKEN` environment variable.
- llm-bridge may not be running. Check your terminal or process manager for errors.
- If the token is set in the YAML via `${DISCORD_BOT_TOKEN}` but the environment variable is not exported, the token will be an empty string and the Discord provider will not start (no error is logged; it simply skips Discord initialization).

### Rate limit messages appearing

llm-bridge includes built-in per-user and per-channel rate limiting. If users see "Rate limited" responses, this is expected behavior to prevent abuse. Rate limits can be adjusted in the config under `defaults.rate_limit`. See the main documentation for details.
