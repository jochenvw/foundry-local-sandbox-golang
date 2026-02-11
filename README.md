# 🧪 Foundry Local Sandbox

A retro-cool TUI chat app powered by **Microsoft Foundry Local** — talk to AI models running right on your machine. No cloud. No latency excuses. Just vibes. 🚀

![Running on GPU](assets/running_on_gpu.png)

![Running on NPU](assets/running_on_npu.png)

## ⚡ Quick Start

```sh
go run .
```

## 🎛 Startup Options

| Flag | Default | Description |
|------|---------|-------------|
| `-system` | `"You are a helpful AI assistant."` | System prompt |
| `-device` | `auto` | Device: `auto` · `cpu` · `gpu` · `npu` |
| `-temp` | `0.2` | Sampling temperature (0–2) |
| `-ctx` | `4096` | Context window size |
| `-mode` | `local` | Session mode label (`local` · `remote`) |

## 🕹 In-App Shortcuts

| Key | Action |
|-----|--------|
| `↑` / `↓` | Scroll through prompt history |
| `:` | Enter command mode (`:clear`, `:temp`, `:model`, `:stats`, `:export`) |
| `clear` | Reset session & history |
| `q` / `quit` / `exit` | Quit |
| `Ctrl+C` / `Ctrl+Q` | Quit |

## 🛠 Built With

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — Styling
- [Foundry Local SDK](https://github.com/microsoft/foundry-local) — On-device AI inference
