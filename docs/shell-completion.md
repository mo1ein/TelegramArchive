# Shell Completion

[Documentation index](README.md)

Run corresponding command to enable shell completion in all sessions:

### bash

```sh
echo "source <(ta completion bash)" >> ~/.bashrc
```

### zsh

```sh
echo "source <(ta completion zsh)" >> ~/.zshrc
```

### fish

```sh
echo "ta completion fish | source" >> ~/.config/fish/config.fish
```

### PowerShell

```powershell
Add-Content -Path $PROFILE -Value "ta completion powershell | Out-String | Invoke-Expression"
```
