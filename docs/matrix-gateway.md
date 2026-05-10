# Matrix Gateway

This guide explains Manifold's built-in Matrix gateway for people who have never used Matrix before.

## What Matrix Is

Matrix is an open chat protocol. You can think of it as a federated chat network, similar in spirit to email:

- a Matrix `homeserver` is the server that hosts accounts and rooms
- a Matrix `user ID` identifies an account, for example `@manifold:matrix.example.com`
- a Matrix `room` is a shared chat space, similar to a channel or group chat
- a Matrix `room ID` is the internal identifier for a room, for example `!abc123:matrix.example.com`

Manifold's Matrix gateway lets `agentd` act as a Matrix chat participant. When someone sends a message in a configured room, Manifold can route that message to the orchestrator or to a named specialist and send the reply back into the room.

## What The Gateway Does

When the gateway is enabled, `agentd`:

- connects to your Matrix homeserver using a Matrix account and access token
- long-polls Matrix for new room messages
- watches only the rooms you explicitly list in `config.yaml`
- routes matching messages to Manifold's orchestrator or to a named specialist
- posts the final reply back into the same Matrix room

The gateway is useful when you want people to talk to Manifold from an ordinary chat room instead of from Manifold's web UI.

## What It Does Not Do

The current implementation is intentionally narrow. End users should know these limits up front:

- It only watches rooms listed in `matrix.rooms`.
- It expects the real Matrix room ID, not a room alias or room name.
- It only routes plain Matrix message events.
- It ignores messages sent by the gateway account itself.
- It does not provide a dedicated Matrix room browser in the Manifold UI yet.
- Matrix-driven conversations currently show up indirectly through Manifold's normal chat/session storage.
- End-to-end encrypted Matrix rooms are not supported by this gateway.
- `deviceID`, `systemPromptRef`, and `maxConcurrent` exist in config today, but are not wired into runtime behavior yet.

## Before You Start

You need four things:

1. A working Matrix homeserver URL, such as `https://matrix.example.com`.
2. A Matrix account for Manifold, such as `@manifold:matrix.example.com`.
3. An access token for that Matrix account.
4. The internal room ID for each room Manifold should watch.

If you are new to Matrix, the most confusing part is usually the room ID. The gateway needs the room's internal ID, which usually starts with `!`.

Examples:

- Valid room ID: `!abc123:matrix.example.com`
- Not valid for this config: `#engineering:matrix.example.com`
- Not valid for this config: `engineering`

## Getting A Matrix Access Token

How you get a token depends on your homeserver and client, but one common path is to log in through the Matrix client API.

Example:

```bash
curl -X POST "https://matrix.example.com/_matrix/client/v3/login" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "m.login.password",
    "identifier": {
      "type": "m.id.user",
      "user": "manifold"
    },
    "password": "your-password",
    "initial_device_display_name": "Manifold Gateway"
  }'
```

That response usually includes:

- `access_token`
- `user_id`
- `device_id`

For Manifold today, the important value is `access_token`.

Store the token in `.env`, not directly in `config.yaml`, for example:

```dotenv
MATRIX_ACCESS_TOKEN="your_real_matrix_access_token"
```

## Finding The Room ID

If your Matrix client does not show the room ID directly, you can ask the homeserver which rooms the gateway account has joined.

Example:

```bash
curl -s \
  -H "Authorization: Bearer $MATRIX_ACCESS_TOKEN" \
  "https://matrix.example.com/_matrix/client/v3/joined_rooms"
```

That returns a list of joined room IDs. Pick the room you want and place that exact value into `matrix.rooms[].roomID`.

## Basic Configuration

The Matrix gateway lives under the `matrix:` section in `config.yaml`.

Minimal example:

```yaml
matrix:
  enabled: true
  homeserverURL: https://matrix.example.com
  userID: "@manifold:matrix.example.com"
  accessToken: "${MATRIX_ACCESS_TOKEN}"
  deviceID: manifold-agentd
  syncTimeoutSeconds: 30
  syncRetryDelaySeconds: 3
  processBacklog: false
  rooms:
    - roomID: "!abc123:matrix.example.com"
      defaultTarget: orchestrator
      allowUnmentioned: true
      mentions:
        "@gpt": gpt
        "@research": research
      systemPromptRef: assets/prompts/matrix/default.txt
      maxConcurrent: 1
```

## Configuration Reference

### Top-Level Matrix Settings

`enabled`

- Turns the gateway on or off.
- If `false`, `agentd` starts normally without connecting to Matrix.

`homeserverURL`

- The base URL of your Matrix homeserver.
- Example: `https://matrix.example.com`

`userID`

- The Matrix account Manifold will use.
- Example: `@manifold:matrix.example.com`

`accessToken`

- The access token for that Matrix account.
- Use an environment variable such as `${MATRIX_ACCESS_TOKEN}` instead of committing a real token into source control.

`deviceID`

- Present in config, but not used by the current gateway implementation.
- You can leave a descriptive value such as `manifold-agentd`.

`syncTimeoutSeconds`

- Controls how long each Matrix `/sync` long-poll can wait for new events.
- Higher values reduce request churn. Lower values make the connection cycle more frequently.
- `30` is a reasonable default.

`syncRetryDelaySeconds`

- Delay before retrying after a sync error.
- `3` is a reasonable default.

`processBacklog`

- If `false`, the gateway skips old room events on the first successful sync after startup.
- If `true`, it processes backlog events from the first sync response.
- Most users should start with `false` so Manifold does not answer old messages when you first turn it on.

### Per-Room Settings

Each item in `matrix.rooms` configures one Matrix room.

`roomID`

- Required.
- Must be the real Matrix room ID, such as `!abc123:matrix.example.com`.
- This is an exact allowlist entry. If the room is not listed here, the gateway ignores it.

`defaultTarget`

- Where unmatched messages go when `allowUnmentioned` is `true`.
- Use `orchestrator` for the main Manifold orchestrator.
- You can also use a specialist name if you want every unmentioned message in the room to go to one specialist.

`allowUnmentioned`

- If `true`, messages that do not match any mention rule still go to `defaultTarget`.
- If `false`, only explicitly matched messages are routed.

`mentions`

- Maps a chat alias to a Manifold route target.
- In practice, these targets should be specialist names, or `orchestrator`.
- Example:

```yaml
mentions:
  "@gpt": gpt
  "@small-bot": small-bot
```

`systemPromptRef`

- Present in config, but not used by the current gateway implementation.
- The current runtime does not load a per-room prompt from this path yet.

`maxConcurrent`

- Present in config, but not used by the current gateway implementation.

## How Message Routing Works

The gateway supports two matching styles.

### Inline `@mention` Matching

If a mention alias starts with `@`, it matches anywhere in the message as a standalone token.

Example config:

```yaml
mentions:
  "@gpt": gpt
```

Example messages that match:

- `@gpt summarize this thread`
- `Can @gpt review this?`

### Prefix Matching

If a mention alias does not start with `@`, it behaves like a command prefix and must appear at the start of the message.

Example config:

```yaml
mentions:
  "!gpt": gpt
```

Example messages that match:

- `!gpt summarize this thread`

When a prefix match is used, the prefix is removed before the prompt is passed into Manifold.

## How Replies Look In Matrix

When the gateway sends a final reply back to the room, it prefixes the reply with the selected route target name.

Example:

```text
gpt: Here is the summary of the discussion...
```

If the message contains Markdown, Manifold also tries to send Matrix-formatted rich text.

## How This Appears In Manifold

There is not yet a dedicated Matrix room viewer in the Manifold UI.

Today, Matrix-triggered conversations are stored as ordinary Manifold chat sessions, keyed by room and target. That means:

- you can inspect the resulting conversation indirectly through the Chat page
- you are not seeing the full Matrix room timeline as a first-class room browser yet

## Recommended First Configuration

If you are new to the feature, start simple:

```yaml
matrix:
  enabled: true
  homeserverURL: https://matrix.example.com
  userID: "@manifold:matrix.example.com"
  accessToken: "${MATRIX_ACCESS_TOKEN}"
  syncTimeoutSeconds: 30
  syncRetryDelaySeconds: 3
  processBacklog: false
  rooms:
    - roomID: "!abc123:matrix.example.com"
      defaultTarget: orchestrator
      allowUnmentioned: false
      mentions:
        "@gpt": gpt
        "@research": research
```

Why this is a good starting point:

- it avoids replying to old room history
- it avoids answering every message in the room
- it makes routing explicit through mentions

## Startup Logs To Expect

On a healthy startup, look for logs like:

- `matrix_gateway_initialized`
- `matrix_gateway_startup_sync_complete`

When a Matrix message is processed successfully, you should later see:

- `matrix_gateway_message_processed`

Potential warning logs:

- `matrix_gateway_sync_failed`
- `matrix_gateway_join_failed`
- `matrix_gateway_handler_failed`

## Troubleshooting

### The Gateway Starts But Never Replies

Check these first:

- `matrix.enabled` is `true`
- the gateway account is actually joined to the room
- `roomID` is the real Matrix room ID beginning with `!`
- the room is listed in `matrix.rooms`
- the message matched either `mentions` or `defaultTarget`

### Manifold Replies To Old Messages On Startup

- Set `processBacklog: false`

### The Gateway Ignores Messages In A Room

Common causes:

- the room ID is wrong
- the room is encrypted
- `allowUnmentioned` is `false` and the message did not match a configured mention alias

### Replies Go To The Wrong Specialist

Check your `mentions` mapping and make sure the target names match the specialist names you configured in Manifold.

### Shutdown Hangs Or Is Slow

Recent versions of the gateway cancel Matrix long-poll requests during shutdown. If shutdown still hangs, collect logs after `agentd shutdown requested` and verify you are running a build that includes the Matrix gateway shutdown fix.

## Security Notes

- Treat the Matrix access token like a password.
- Keep it in `.env` or another secret store, not in committed YAML.
- Use a dedicated Matrix account for Manifold rather than a personal account.
- Add the gateway account only to rooms where Manifold is supposed to operate.

## Advanced Automation: Pulse Tasks

The Matrix gateway can work with Manifold's pulse automation runtime. Pulse tasks are recurring background checks or routines associated with a Matrix room and route target.

This is an advanced feature. Most users do not need it to start using Matrix chat.

Important behavior:

- pulse execution runs inside `agentd`
- pulse logs are internal by default
- room-facing pulse messages should be sent intentionally through the Matrix message tool, not assumed automatically

## Summary

If you remember only five things, remember these:

1. Matrix is a chat network; Manifold can join it through one Matrix account.
2. You must configure explicit room IDs under `matrix.rooms`.
3. Use the real room ID starting with `!`, not a room alias starting with `#`.
4. `processBacklog: false` is the safest first-run setting.
5. `deviceID`, `systemPromptRef`, and `maxConcurrent` are not active runtime knobs yet.
