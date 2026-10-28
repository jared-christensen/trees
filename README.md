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

`trees switch` and `trees create` change your directory, which a program can't do on its own. Add this to your `~/.zshrc`:

```sh
eval "$(trees init zsh)"
```

Or, for bash, to your `~/.bashrc`:

```sh
eval "$(trees init bash)"
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
| `trees` / `trees list` | Show worktrees and branches and which are safe to delete. `--no-fetch` skips fetching, `--no-pr` skips pull request lookups, `--json` prints the report as JSON for scripts. |
| `trees clean` | Delete the safe-to-delete worktrees and branches after listing them and asking. It needs a successful fetch first, and deletes nothing if the remote can't be reached. It skips locked worktrees, the main checkout, the worktree you're in, and long-lived branches (`main`, `master`, `develop`, `trunk`). Before asking, it lists any ignored files (such as `.env`) that removing a folder will delete. `-y` skips the question, `--no-pr` skips pull request lookups. Remote branches are never touched. Each deletion prints a `git branch` command to restore it. |
| `trees create <branch>` | Make a worktree and branch with matching names, run any configured `setup` commands, and go there. A new branch starts from the main branch after a fetch (`origin/main`, not a possibly outdated local `main`; if the remote can't be reached, from what was last fetched). If the branch already exists, locally or only on the remote, that branch is reused instead. |
| `trees switch <name>` | Go to a worktree by branch or folder name. |
| `trees init <zsh\|bash>` | Print the shell function that lets `create` and `switch` change directories. |

`-C <path>` runs any command as if `trees` was started in that directory.

The bare-repo layout (the repository in `.bare` with a `.git` file pointing at it, and every branch in its own worktree) works too: `list`, `clean` and `switch` treat it as a repo with no main checkout.

## How states are decided

Rules are checked in this order, and the first match wins:

1. Uncommitted changes, an open PR, a worktree `trees` couldn't check, or a long-lived branch (`main`, `master`, `develop`, `trunk`): **still in use**.
2. No commits of its own: **still in use**, or **stale** once it is older than `stale_days` (counted from when the branch was created or last moved, not from its last commit).
3. Merged into the main branch (by git): **safe to delete**.
4. Closed PR, or older than `stale_days`: **stale, take a look**.
5. Anything else: **still in use**.

A merged PR is shown as a reason, but a merged PR alone never makes a branch safe to delete: only the git check in rule 3 does. If the PR merged but the branch has changes that aren't in the main branch, the reasons say so. `trees list` always prints the reasons next to the state. Pull request lookups work for GitHub and Bitbucket Cloud. For GitHub, `trees` uses the [GitHub CLI](https://cli.github.com) (`gh`) when it is installed. For Bitbucket, set `BITBUCKET_USERNAME` (your Atlassian account email) and `BITBUCKET_TOKEN` (an [API token](https://id.atlassian.com/manage-profile/security/api-tokens) with read access to pull requests; an app password in `BITBUCKET_APP_PASSWORD` also works). Without these, or with `--no-pr`, only the git checks apply.

## Config

Optional. `trees` reads `~/.config/trees/config.toml` (or `$XDG_CONFIG_HOME/trees/config.toml`). It is personal and not stored in your repos. Settings apply in layers: built-in defaults, then `[defaults]`, then `[repos."<path>"]`, where the path is the repo's main checkout.

```toml
[defaults]
path = "{repo_parent}/{repo}.{name}"  # where new worktrees go (this is the default)
stale_days = 14                       # age at which unfinished work is called stale
pull_requests = true                  # look up PRs on GitHub or Bitbucket
fetch_timeout_seconds = 5             # how long to wait on the remote

[repos."/Users/me/code/app"]
name_pattern = "^[A-Z]+-[0-9]+"       # regex branch names should match
name_pattern_mode = "warn"            # "warn" (default) or "require"
setup = ["npm install"]               # run after `trees create`
after_switch = "code ."               # run after `trees switch`
```

Unknown keys are an error, so typos don't go unnoticed.

In a bare-repo layout (`<repo>/.bare` with worktrees beside it), new worktrees go in `<repo>/<name>` unless you set `path`. Worktrees Claude Code makes under `.claude/worktrees/` are listed by name with "(Claude)".

## Requirements

- git 2.38 or newer
- `gh` (optional) for GitHub pull request state
- `BITBUCKET_USERNAME` and `BITBUCKET_TOKEN` (optional) for Bitbucket pull request state

## License

MIT
