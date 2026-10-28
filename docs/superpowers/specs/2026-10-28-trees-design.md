# trees — design

A command-line tool for git worktrees: create them, switch between them, see what state they're in, and clean them up. Public, open source, distributed on GitHub and Homebrew. Meant to replace WorkTrunk for its author, and to be useful to anyone who finds it.

## Why

Creating worktrees is easy now, especially with AI coding agents making them on your behalf. Keeping track of them afterward is not. People end up with a pile of worktree folders and branches and can't easily answer:

- How many worktrees do I have, and which ones am I still using?
- Which branch is in which worktree?
- What can I safely delete without losing work?

The author's own repo, on the day this was written, had 8 worktrees and 12 local branches. Several worktrees still held uncommitted files long after their branch had merged (one had 249). Folder names often didn't match the branch inside them (folder `ABC-2475-move` held branch `ABC-2475-no-next`, while a separate branch named `ABC-2475-move` had no worktree at all).

**Success:** you run one command and see the true state of every worktree and branch, and one more command cleans up everything that's done, safely.

**The anti-goal that matters most:** if seeing the state of worktrees and branches, and cleaning them up, isn't easy, the tool has failed, however well the rest works. Listing and cleaning are the core. Creating and switching come second.

## Goals (v1)

1. **List.** Show every worktree and branch in the repo and the state of each.
2. **Clean.** Remove what's safe to remove, after one confirmation.
3. **Create.** Make a new worktree and branch with a good name, plus an optional setup step.
4. **Switch.** Move the terminal into a worktree.
5. **Works with any git host.** The core relies only on git. Pull request status is an optional extra.

## Non-goals (v1)

- Claude Code or other agent integration (for example, hooking into Claude's worktree creation). Wanted later, deliberately left out to keep v1 simple.
- Grouping worktrees by Jira epic or project. Names that start with the ticket key sort related work together; an epic heading can come later if it's missed.
- Tracking which branches a worktree held in the past. One worktree per branch makes that unnecessary (see model below).
- Bitbucket and GitLab pull request lookups. The design leaves room for them, but v1 ships GitHub only.
- Deleting remote branches. Never.
- Merging, rebasing, committing, or any other git workflow. WorkTrunk does those; trees doesn't.

## The model

**One worktree per branch, with the same name.** That's the recommended way to use the tool, and what `create` does. Worktree and branch are created together and deleted together, so "which branch is in which folder" never comes up. The author considered one worktree per epic holding many story branches, and moved away from it because a worktree can only have one branch checked out at a time, and because it makes cleanup ambiguous. Setup time for a fresh worktree is low enough that per-branch is fine.

**Mismatches are tolerated, not fought.** If someone (or an agent) makes a worktree whose folder name differs from its branch, or switches branches inside a worktree, trees lists it accurately and doesn't complain or try to rename anything. The tool prevents the mess through good defaults in `create`, not through enforcement.

## Commands

Exact flags and wording are the builder's call. The shape:

- `trees` or `trees list`: the report (below).
- `trees clean`: show what's safe to delete, ask once, remove it.
- `trees create <name>`: new branch off the default branch, new worktree with the matching folder name, run the setup step if one is configured, then switch into it.
- `trees switch <name>`: move into that worktree. Matching by branch name or folder name, with partial matching, is a nice-to-have.
- `trees init <shell>`: prints the shell function that makes `switch` able to change the terminal's directory (a child process can't change its parent shell's directory, so this works like WorkTrunk's and zoxide's: the binary writes the target path and the wrapper `cd`s to it).

## The list (the most important screen)

Prototype: `~/.local/bin/trees` (Python, Bitbucket-specific). Its output, reorganized at the author's request, is the reference for layout:

```
shop-app  ~/code/shop-app, compared to origin/main

Worktrees
  ● still in use  ABC-2605-cli-move  0h ago
      branch ABC-2605-cli-beta-notice
      PR #1220 open
  ● still in use  ABC-2602-cookie-consent-script  14h ago
      branch ABC-2602-cookie-consent-script
      22 uncommitted file(s); branch itself is already merged
  ● stale, take a look  fix-startup-4b4f0d (Claude)  7d ago
      branch claude/abc-2571-checklist-61a30e
      PR #1159 declined
  ● safe to delete  ABC-2475-move  14h ago
      branch ABC-2475-no-next
      PR #1230 merged

Branches not in any worktree
  ● stale, take a look  XYZ-477-unhandled-api-error  463d ago
      PR #172 declined
  ● safe to delete  ABC-2475-move  15h ago
      PR #1217 merged

2 safe to delete. Run trees clean to remove them.
```

Decisions from the conversation:

- **Worktrees first**, each with the branch it has checked out underneath. Then a separate section for branches that aren't in any worktree. The author thinks worktree-first.
- **Plain words, not shorthand.** An earlier version showed the main checkout as `<main>` and it confused the author. Show folder names as short names relative to the repo, and the full repo path once at the top.
- When folder name and branch name match, the builder may collapse them to one line.
- Each entry gets a state, its age (time since last commit), and the reasons in plain words.
- Worktrees whose folder is missing, and worktrees with no branch checked out, are shown as such.
- The main checkout isn't listed as an item. Mention it only if something is off (for example, it's not on the default branch, or it has uncommitted changes).

### States

| State | Meaning |
| --- | --- |
| **still in use** | Has uncommitted changes, an open PR, or commits not yet merged and recent activity |
| **stale, take a look** | Not merged, and either inactive for a while (default 14 days, configurable) or its PR was closed without merging |
| **safe to delete** | Its work is fully in the default branch and its worktree (if any) has no uncommitted changes |

**Which state wins** (assistant's call, settled after the builder check). Check these in order; the first that applies decides:

1. Uncommitted changes, or an open PR: **still in use**, even if git says the branch is merged.
2. A branch with no commits of its own (for example, one `create` just made): **still in use** until it passes the stale threshold, then **stale**. It is never "safe to delete" just because it has nothing in it. Telling "no commits of its own" apart from "fast-forward merged" needs care (the branch's reflog shows where it started); the builder picks the method.
3. Merged by the git check: **safe to delete**.
4. PR closed without merging, or no commits for longer than the stale threshold: **stale**.
5. Anything else: **still in use**.

**Uncommitted changes always block deletion,** and get called out loudly, especially on a branch that's already merged ("22 uncommitted files; branch already merged"). Leftover uncommitted work in old worktrees was the most common mess in the author's repo.

### Deciding "merged" without a git host

This is what makes the tool host-independent, and it has to handle squash merges and rebase merges, where the branch's commits never appear in the default branch.

A branch counts as merged when merging it into the up-to-date default branch would change nothing. Suggested approach: `git merge-tree --write-tree <default> <branch>` and compare the resulting tree to the default branch's tree. If they're equal, every change on the branch is already there. That also handles "commits added after the PR merged" correctly: those make the trees differ, so the branch isn't safe. A plain ancestor check (`merge-base --is-ancestor`) is the fast path for regular merges. The builder may pick another method if it's more reliable. It must handle squash merges and must never call a branch merged when it has changes the default branch lacks.

The default branch is whatever `origin/HEAD` points to, falling back to `main` or `master`.

**Fetching** (assistant's call). `list` fetches with prune under a short timeout (a few seconds). If the fetch fails or times out, it uses the refs it already has and says so in one line at the top. `clean` needs a successful fetch before it removes anything; if the fetch fails, it says why and stops. A flag lets `list` skip fetching.

### Pull request status (optional)

When it's available, PR state adds detail: PR number, and open, merged, or closed. v1 supports GitHub through the `gh` CLI if it's installed and logged in. If it isn't, the tool works fully without it; PR info just doesn't appear. Build this behind a small provider interface so Bitbucket and GitLab can be added later. PR lookups run in parallel and must not make the list feel slow. The prototype took about 10 seconds with serial-ish lookups, which is too slow. There's also a flag to skip them.

A merged PR is supporting evidence, not proof. The git check above decides safety.

## Clean

- Shows exactly what will be removed (worktree folder and branch), then asks once, yes or no.
- Removes only "safe to delete" items. A flag to also include stale items, or to pick items one by one, is the builder's call.
- Refuses to remove the worktree the terminal is currently in, and says why.
- Prints how to restore each deleted branch (`git branch <name> <sha>`).
- Never forces removal of a worktree with uncommitted changes. Never touches remote branches.
- Prunes worktree records whose folders are gone.

## Create and naming

- Default: worktree folder and branch share the name given. `trees create ABC-2518-first-run-prototype` makes branch `ABC-2518-first-run-prototype` and folder `<repo>.ABC-2518-first-run-prototype` beside the repo.
- New branches start from the remote default branch after a fetch (`origin/main`), not from a possibly outdated local `main`. Assistant's call.
- If the branch already exists, locally or only on the remote (for example, to review someone's PR), `create` makes a worktree for that existing branch instead of failing, and sets up tracking for a remote branch. If the branch is already checked out in another worktree, it says where and offers to switch there. Assistant's call.
- Slashes in branch names become dashes in folder names.
- **Where folders go is a setting.** Default is beside the repo (WorkTrunk's default and the author's current layout). Alternatives people use: a folder inside the repo, or one shared folder in the home directory.
- **Name pattern is a setting, off by default.** Teams that start names with a ticket key (`ABC-2518-...`) can set a pattern so `create` nudges or requires it. Other teams use `feature/...` or `name/...`. The default just uses the name given.
- **Setup step:** optional per-repo commands to run after creating a worktree (copy `.env` files, install dependencies).

## Switch

Moves the terminal into the worktree via the shell function. An optional setting runs a command afterward, such as opening the folder in an editor. Default is just the directory change.

## Configuration

A user config file and optional per-repo settings. Format is the builder's call; TOML in `~/.config/trees/` is the obvious choice.

**Per-repo settings are personal, not shared** (author's decision). They live outside the repo or in a file git ignores, never committed. So trees never runs setup commands that someone else wrote, and v1 needs no approval step. A shared, committed config with an approval step like WorkTrunk's or direnv's can come later. Settings: worktree location, name pattern, setup commands, post-switch command, stale threshold, PR provider on or off.

## Constraints and decisions

- **Language: Go.** Assistant's call, accepted by the author: one small binary with no runtime to install, fast startup, an established release path to GitHub Releases and a Homebrew tap (for example, GoReleaser). Go isn't installed on the author's machine yet.
- **Name: trees.** Not taken in Homebrew core as of 2026-10-28. Distribution through the author's own Homebrew tap.
- **Works on macOS and Linux.** Windows is a nice-to-have.
- **Shells:** zsh and bash for v1; fish is a nice-to-have.
- **Shells out to `git`** rather than using a Go git library, so behavior matches the user's git exactly (worktrees and `merge-tree --write-tree` need git 2.38+; say so clearly if git is older).
- **Project location:** `~/Personal Projects/trees`, published as `github.com/jared-christensen/trees` (also the Go module path). Homebrew tap: `jared-christensen/homebrew-tap`, so installing is `brew install jared-christensen/tap/trees`. Assistant's call.
- Read-only by default. Only `clean` and `create` change anything.

## Left to the builder

- Exact command names, flags, aliases, and output styling (colors, symbols), within the layout above.
- Whether to add `--json` output (likely useful for later agent integration).
- Test strategy. Git behavior should be tested against real temporary repos, including a squash-merged branch, a branch with commits after its squash merge, a dirty worktree, a missing worktree folder, and a detached worktree.
- README and install instructions, including the shell setup line.
