# Agent hooks

`tmux-session-sidebar` can install cmux-style agent hook integrations so the sidebar bell is driven by explicit lifecycle events instead of pane scraping.

## Commands

If `tmux-session-sidebar` is not on your `PATH`, call the plugin-local binary directly instead:

```bash
~/.tmux/plugins/tmux-session-sidebar/.bin/tmux-session-sidebar hooks setup --yes
# or from a local checkout:
./.bin/tmux-session-sidebar hooks setup --yes
```

Standard commands:

```bash
tmux-session-sidebar hooks setup
tmux-session-sidebar hooks setup <agent>
tmux-session-sidebar hooks setup --agent <agent>
tmux-session-sidebar hooks uninstall <agent>

tmux-session-sidebar hooks <agent> install
tmux-session-sidebar hooks <agent> uninstall
```

`hooks setup` installs every supported agent whose binary is found on `PATH`.

## Supported agents

| Agent | Binary checked | Installed file |
| --- | --- | --- |
| Codex | `codex` | `~/.codex/hooks.json` |
| Grok | `grok` | `~/.grok/hooks/cmux-session.json` |
| OpenCode | `opencode` | `~/.config/opencode/plugins/cmux-session.js` |
| Pi | `pi` | `~/.pi/agent/extensions/cmux-session.ts` |
| OMP | `omp` | `~/.omp/agent/extensions/cmux-session.ts` |
| Amp | `amp` | `~/.config/amp/plugins/cmux-session.ts` |
| Cursor CLI | `cursor-agent` | `~/.cursor/hooks.json` |
| Gemini | `gemini` | `~/.gemini/settings.json` |
| Rovo Dev | `acli` | `~/.rovodev/config.yml` |
| Copilot | `copilot` | `~/.copilot/config.json` |
| CodeBuddy | `codebuddy` | `~/.codebuddy/settings.json` |
| Factory | `droid` | `~/.factory/settings.json` |
| Qoder | `qodercli` | `~/.qoder/settings.json` |

## Behavior

The installed integrations emit generic sidebar events:

- running / prompt submit
- completion / notification / needs attention
- session end when the CLI exposes it

Those events feed the dedicated `AgentAttention` state in `tmux-session-sidebar`.
The bell marker appears for the session when an agent reports completion or user attention is needed, and clears when that session becomes current again in any attached tmux client. Unread bells animate with `@session-sidebar-agent-attention-animation`, which accepts `off`, `pulse`, `rainbow`, or `blink`.

## Disable the feature

Global tmux option:

```tmux
set -g @session-sidebar-agent-attention 'off'
set -g @session-sidebar-agent-attention-animation 'off'
```

Per-process escape hatch env vars:

- `TMUX_SESSION_SIDEBAR_CODEX_HOOKS_DISABLED=1`
- `TMUX_SESSION_SIDEBAR_GROK_HOOKS_DISABLED=1`
- `TMUX_SESSION_SIDEBAR_OPENCODE_HOOKS_DISABLED=1`
- `TMUX_SESSION_SIDEBAR_PI_HOOKS_DISABLED=1`
- `TMUX_SESSION_SIDEBAR_OMP_HOOKS_DISABLED=1`
- `TMUX_SESSION_SIDEBAR_AMP_HOOKS_DISABLED=1`
- `TMUX_SESSION_SIDEBAR_CURSOR_HOOKS_DISABLED=1`
- `TMUX_SESSION_SIDEBAR_GEMINI_HOOKS_DISABLED=1`
- `TMUX_SESSION_SIDEBAR_ROVODEV_HOOKS_DISABLED=1`
- `TMUX_SESSION_SIDEBAR_COPILOT_HOOKS_DISABLED=1`
- `TMUX_SESSION_SIDEBAR_CODEBUDDY_HOOKS_DISABLED=1`
- `TMUX_SESSION_SIDEBAR_FACTORY_HOOKS_DISABLED=1`
- `TMUX_SESSION_SIDEBAR_QODER_HOOKS_DISABLED=1`

## Generated file headers

Generated OpenCode, Pi, and Amp plugin/extension files carry explicit attribution to cmux’s integration design and mention the cmux repo copyright and dual-license notice, per the upstream source context. The OMP extension uses OMP’s own Pi-style extension API and imports `@oh-my-pi/pi-coding-agent`.

## Reinstalling or cleaning up

```bash
tmux-session-sidebar hooks <agent> install
tmux-session-sidebar hooks <agent> uninstall
# plugin-local alternative:
~/.tmux/plugins/tmux-session-sidebar/.bin/tmux-session-sidebar hooks <agent> install
```

Re-run install after plugin upgrades or if you intentionally deleted a generated integration file.
