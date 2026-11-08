# Extensions

[Documentation index](README.md)

Extensions add commands to ta. Installed commands appear in `ta --help`.
They live under `~/.ta/extensions/`. The extension interface is experimental.

## Install and inspect

Use a GitHub `owner/repository` reference or a local extension path.
Replace the example repository with the extension you want:

```sh
ta extension install owner/repository
ta extension install /path/to/extension
ta extension install --dry-run owner/repository
ta extension install --force owner/repository
ta extension list
```

`--dry-run` previews changes. `--force` replaces an existing installation.
For private repositories, export `GITHUB_TOKEN` with read access to repository contents.
Third-party repository names keep their original names.

## Run

Run the command shown by `ta extension list` as a ta subcommand.
Put global flags before the extension command and its own flags after it.
For example, an extension named `hello` would use `ta -n work hello --help`.
Check that extension's README for its options.

## Upgrade or remove

Pass an installed extension name. Omit names when upgrading to update all installed extensions:

```sh
ta extension upgrade
ta extension upgrade EXTENSION
ta extension upgrade --dry-run EXTENSION
ta extension remove EXTENSION
ta extension remove --dry-run EXTENSION
```

GitHub installations can be upgraded through ta. Local extensions are updated at their source.
Private-repository upgrades also need `GITHUB_TOKEN`.
