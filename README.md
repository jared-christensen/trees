# trees

List, clean, create and switch git worktrees.

Worktrees pile up. After a few weeks you have a dozen folders and branches and no idea which are finished. `trees` shows every worktree and branch in a repo, says which are safe to delete and why, and removes the finished ones in one command. It also creates a worktree with a matching branch and takes you there.

## Install

```sh
brew install jared-christensen/tap/trees
```

Or build from source (Go 1.23+):

```sh
go install github.com/jared-christensen/trees/cmd/trees@latest
```

## Shell setup

`trees switch` and `trees create` change your directory, which a program can't do on its own. Add this to your `~/.zshrc` (or `~/.bashrc` with `bash`):

```sh
eval "$(trees init zsh)"
```

## Usage

Run `trees` with no arguments to see the current repo:

```
app  /work/app, compared to origin/main

Worktrees
  ● still in use  feat-x  1h ago
      3 uncommitted files
  ● stale, take a look  /elsewhere/lost-dir (folder missing) (locked)  5d ago
      branch lost
      not merged yet
  ● safe to delete  done-folder  2h ago
      branch done
      merged into origin/main

Branches not in any worktree
  ● stale, take a look  gone-branch  40d ago
      PR #3 closed
  ● safe to delete  old-merged  3d ago
      PR #9 merged

2 safe to delete. Run trees clean to remove them.
```

| Command | What it does |
| --- | --- |
| `trees` / `trees list` | Show worktrees and branches and which are safe to delete. `--no-fetch` skips fetching, `--no-pr` skips pull request lookups. |
| `trees clean` | Delete the safe-to-delete worktrees and branches after listing them and asking. `-y` skips the question, `--no-pr` skips pull request lookups. Remote branches are never touched. Each deletion prints a `git branch` command to restore it. |
| `trees create <branch>` | Make a worktree and branch with matching names (from the up-to-date main branch), run any configured `setup` commands, and go there. |
| `trees switch <name>` | Go to a worktree by branch or folder name. |
| `trees init <zsh\|bash>` | Print the shell function that lets `create` and `switch` change directories. |

`-C <path>` runs any command as if `trees` was started in that directory.

## How states are decided

Rules are checked in this order, and the first match wins:

1. Uncommitted changes or an open PR: **still in use**.
2. No commits of its own: **still in use**, or **stale** once it is older than `stale_days`.
3. Merged into the main branch (by git): **safe to delete**.
4. Closed PR, or older than `stale_days`: **stale, take a look**.
5. Anything else: **still in use**.

A merged PR also counts as a reason, and `trees list` always prints the reasons next to the state. Pull request lookups use the [GitHub CLI](https://cli.github.com) (`gh`) when it is installed; without it, or with `--no-pr`, only the git checks apply.

## Config

Optional. `trees` reads `~/.config/trees/config.toml` (or `$XDG_CONFIG_HOME/trees/config.toml`). It is personal and not stored in your repos. Settings apply in layers: built-in defaults, then `[defaults]`, then `[repos."<path>"]`, where the path is the repo's main checkout.

```toml
[defaults]
path = "{repo_parent}/{repo}.{name}"  # where new worktrees go (this is the default)
stale_days = 14                       # age at which unfinished work is called stale
pull_requests = true                  # look up PRs with gh
fetch_timeout_seconds = 5             # how long to wait on the remote

[repos."/Users/me/code/app"]
name_pattern = "^[A-Z]+-[0-9]+"       # regex branch names should match
name_pattern_mode = "warn"            # "warn" (default) or "require"
setup = ["npm install"]               # run after `trees create`
after_switch = "code ."               # run after `trees switch`
```

Unknown keys are an error, so typos don't go unnoticed.

## Requirements

- git 2.38 or newer
- `gh` (optional) for pull request state

## License

MIT
