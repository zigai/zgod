# zgod

Highly customizable interactive shell history search with fuzzy, regex, and glob matching.

zgod records every command you run, stores it in a local SQLite database, and gives you an interactive search interface bound to `Ctrl+R`.

## Install

```sh
go install github.com/zigai/zgod@latest
```

Or build from source:

```sh
git clone https://github.com/zigai/zgod.git
cd zgod
go build
```

## Shell setup

The easiest way to set up zgod is to use the install command:

```sh
zgod install bash   # or zsh, fish
```

This will automatically add the required configuration to your shell's config file.

If you prefer to set it up manually, add one of the following to your shell configuration file:

**Bash** (`~/.bashrc`)

```bash
eval "$(zgod init bash)"
```

**Zsh** (`~/.zshrc`)

```zsh
eval "$(zgod init zsh)"
```

**Fish** (`~/.config/fish/config.fish`)

```fish
zgod init fish | source
```

This hooks into your shell to automatically record commands and binds `Ctrl+R` to the interactive search.

## Usage

Press `Ctrl+R` in your shell to open the search interface. Start typing to filter your history. Press `Enter` to select a command, `Esc` to cancel.

### Keybindings

| Key | Action |
|---|---|
| `up` / `ctrl+p` | Move up |
| `down` / `ctrl+n` | Move down |
| `enter` | Accept selection |
| `esc` | Cancel |
| `ctrl+s` | Cycle match mode (fuzzy / regex / glob) |
| `alt+f` | Fuzzy mode |
| `alt+r` | Regex mode |
| `alt+g` | Glob mode |
| `ctrl+g` | Toggle CWD filter |
| `ctrl+d` | Toggle deduplication |
| `ctrl+f` | Toggle failed commands only |
| `?` | Help overlay |

## Configuration

zgod is configured via `~/.config/zgod/config.toml`. All fields are optional.
History is stored in a SQLite database at `~/.local/share/zgod/history.db`.

```toml
[db]
path = ""  # default: ~/.local/share/zgod/history.db

[filters]
ignore_space = true       # skip commands starting with a space
exit_code = [130]         # exit codes to skip, e.g. 130 = Ctrl+C
command_glob = []         # command glob patterns to skip, e.g. ["cd *", "ls", "exit"]
command_regex = []        # command regex patterns to skip, e.g. ["^sudo "]
directory_glob = []       # directory glob patterns to skip, e.g. ["/tmp/**"]
directory_regex = []      # directory regex patterns to skip, e.g. ["^/tmp"]

[theme]
prompt = "> "
prompt_color = "cyan"
match_color = "yellow"
selected_bg = "236"
selected_fg = ""
mode_color = "240"
border_color = ""

[display]
time_format = "relative"     # relative | absolute
duration_format = "auto"     # auto | ms | s
show_directory = false       # show directory column in search results

[keys]
mode_next = "ctrl+s"
mode_fuzzy = "alt+f"
mode_regex = "alt+r"
mode_glob = "alt+g"
toggle_cwd = "ctrl+g"
toggle_dedupe = "ctrl+d"
toggle_fails = "ctrl+f"
accept = "enter"
cancel = "esc"
up = "up"
down = "down"
help = "?"
```
